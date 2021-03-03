// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mattermost/mattermost-server/v5/mlog"
	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/plugin"
	"github.com/mattermost/mattermost-server/v5/store"
	"github.com/mattermost/mattermost-server/v5/utils"
)

const (
	PENDING_POST_IDS_CACHE_SIZE = 25000
	PENDING_POST_IDS_CACHE_TTL  = 30 * time.Second
	PAGE_DEFAULT                = 0
	MAX_RECENT_TOTAL            = 1000
	MAX_RECENT_PER_CHANNEL      = 30
	MAX_INCREMENT_PAGE          = 1000
	MAX_INCREMENT_TOTAL         = 1500
)

func (a *App) CreatePostAsUser(post *model.Post, currentSessionId string) (*model.Post, *model.AppError) {
	// Check that channel has not been deleted
	channel, errCh := a.Srv().Store.Channel().Get(post.ChannelId, true)
	if errCh != nil {
		err := model.NewAppError("CreatePostAsUser", "api.context.invalid_param.app_error", map[string]interface{}{"Name": "post.channel_id"}, errCh.Error(), http.StatusBadRequest)
		return nil, err
	}

	if strings.HasPrefix(post.Type, model.POST_SYSTEM_MESSAGE_PREFIX) {
		err := model.NewAppError("CreatePostAsUser", "api.context.invalid_param.app_error", map[string]interface{}{"Name": "post.type"}, "", http.StatusBadRequest)
		return nil, err
	}

	if channel.DeleteAt != 0 {
		err := model.NewAppError("createPost", "api.post.create_post.can_not_post_to_deleted.error", nil, "", http.StatusBadRequest)
		return nil, err
	}

	rp, err := a.CreatePost(post, channel, true)
	if err != nil {
		if err.Id == "api.post.create_post.root_id.app_error" ||
			err.Id == "api.post.create_post.channel_root_id.app_error" ||
			err.Id == "api.post.create_post.parent_id.app_error" {
			err.StatusCode = http.StatusBadRequest
		}

		if err.Id == "api.post.create_post.town_square_read_only" {
			user, userErr := a.Srv().Store.User().Get(post.UserId)
			if userErr != nil {
				return nil, userErr
			}

			T := utils.GetUserTranslations(user.Locale)
			a.SendEphemeralPost(
				post.UserId,
				&model.Post{
					ChannelId: channel.Id,
					ParentId:  post.ParentId,
					RootId:    post.RootId,
					UserId:    post.UserId,
					Message:   T("api.post.create_post.town_square_read_only"),
					CreateAt:  model.GetMillis() + 1,
				},
			)
		}
		return nil, err
	}

	// Update the LastViewAt only if the post does not have from_webhook prop set (eg. Zapier app)
	if _, ok := post.Props["from_webhook"]; !ok {
		if _, err := a.MarkChannelsAsViewed([]string{post.ChannelId}, post.UserId, currentSessionId); err != nil {
			mlog.Error(
				"Encountered error updating last viewed",
				mlog.String("channel_id", post.ChannelId),
				mlog.String("user_id", post.UserId),
				mlog.Err(err),
			)
		}
	}

	return rp, nil
}

func (a *App) CreatePostMissingChannel(post *model.Post, triggerWebhooks bool) (*model.Post, *model.AppError) {
	channel, err := a.Srv().Store.Channel().Get(post.ChannelId, true)
	if err != nil {
		return nil, err
	}

	return a.CreatePost(post, channel, triggerWebhooks)
}

// deduplicateCreatePost attempts to make posting idempotent within a caching window.
// We rely on the client sending the pending post id across "duplicate" requests. If there
// isn't one, we can't deduplicate, so allow creation normally.
func (a *App) deduplicateCreatePost(post *model.Post) (foundPost *model.Post, err *model.AppError) {
	if post.PendingPostId == "" {
		return nil, nil
	}

	const unknownPostId = ""

	// Query the cache atomically for the given pending post id, saving a record if
	// it hasn't previously been seen.
	value, loaded := a.Srv().seenPendingPostIdsCache.GetOrAdd(post.PendingPostId, unknownPostId, PENDING_POST_IDS_CACHE_TTL)

	// If we were the first thread to save this pending post id into the cache,
	// proceed with create post normally.
	if !loaded {
		return nil, nil
	}

	postId := value.(string)

	// If another thread saved the cache record, but hasn't yet updated it with the actual post
	// id (because it's still saving), notify the client with an error. Ideally, we'd wait
	// for the other thread, but coordinating that adds complexity to the happy path.
	if postId == unknownPostId {
		return nil, model.NewAppError("deduplicateCreatePost", "api.post.deduplicate_create_post.pending", nil, "", http.StatusInternalServerError)
	}

	// If the other thread finished creating the post, return the created post back to the
	// client, making the API call feel idempotent.
	actualPost, err := a.GetSinglePost(postId)
	if err != nil {
		return nil, model.NewAppError("deduplicateCreatePost", "api.post.deduplicate_create_post.failed_to_get", nil, err.Error(), http.StatusInternalServerError)
	}

	mlog.Debug("Deduplicated create post", mlog.String("post_id", actualPost.Id), mlog.String("pending_post_id", post.PendingPostId))

	return actualPost, nil
}

func (a *App) CreatePost(post *model.Post, channel *model.Channel, triggerWebhooks bool) (savedPost *model.Post, err *model.AppError) {
	foundPost, err := a.deduplicateCreatePost(post)
	if err != nil {
		return nil, err
	}
	if foundPost != nil {
		return foundPost, nil
	}

	// If we get this far, we've recorded the client-provided pending post id to the cache.
	// Remove it if we fail below, allowing a proper retry by the client.
	defer func() {
		if post.PendingPostId == "" {
			return
		}

		if err != nil {
			a.Srv().seenPendingPostIdsCache.Remove(post.PendingPostId)
			return
		}

		a.Srv().seenPendingPostIdsCache.AddWithExpiresInSecs(post.PendingPostId, savedPost.Id, int64(PENDING_POST_IDS_CACHE_TTL.Seconds()))
	}()

	post.SanitizeProps()

	// Get a parent post(s) if any
	var pchan chan store.StoreResult
	var ancestorId string
	if len(post.RootId) > 0 {
		ancestorId = post.RootId
	} else if len(post.ReplyToId) > 0 {
		ancestorId = post.ReplyToId
	}
	if ancestorId != "" {
		pchan = make(chan store.StoreResult, 1)
		go func() {
			// The result is either a thread of posts or a single post
			r, pErr := a.Srv().Store.Post().Get(ancestorId, len(post.ReplyToId) > 0)
			pchan <- store.StoreResult{Data: r, Err: pErr}
			close(pchan)
		}()
	}

	user, err := a.Srv().Store.User().Get(post.UserId)
	if err != nil {
		return nil, err
	}

	if user.IsBot {
		post.AddProp("from_bot", "true")
	}

	if a.License() != nil && *a.Config().TeamSettings.ExperimentalTownSquareIsReadOnly &&
		!post.IsSystemMessage() &&
		channel.Name == model.DEFAULT_CHANNEL &&
		!a.RolesGrantPermission(user.GetRoles(), model.PERMISSION_MANAGE_SYSTEM.Id) {
		return nil, model.NewAppError("createPost", "api.post.create_post.town_square_read_only", nil, "", http.StatusForbidden)
	}

	var ephemeralPost *model.Post
	if post.Type == "" && !a.HasPermissionToChannel(user.Id, channel.Id, model.PERMISSION_USE_CHANNEL_MENTIONS) {
		mention := post.DisableMentionHighlights()
		if mention != "" {
			T := utils.GetUserTranslations(user.Locale)
			ephemeralPost = &model.Post{
				UserId:    user.Id,
				RootId:    post.RootId,
				ParentId:  post.ParentId,
				ChannelId: channel.Id,
				Message:   T("model.post.channel_notifications_disabled_in_channel.message", model.StringInterface{"ChannelName": channel.Name, "Mention": mention}),
				Props:     model.StringInterface{model.POST_PROPS_MENTION_HIGHLIGHT_DISABLED: true},
			}
		}
	}

	// Verify the parent/child relationships are correct
	var parentPostList *model.PostList
	if pchan != nil {
		// We only get here if either RootId or ReplyToId is defined
		result := <-pchan

		if result.Err != nil {
			return nil, model.NewAppError("createPost", "api.post.create_post.root_id.app_error", nil, "", http.StatusBadRequest)
		}
		parentPostList = result.Data.(*model.PostList)

		var parentErr *model.AppError
		if len(post.RootId) > 0 {
			// Check threaded replies
			if post.ParentId == "" {
				post.ParentId = post.RootId
			}
			parentErr = checkThreadedReply(post, parentPostList)
		} else {
			// Check non-threaded replies
			parentErr = checkNonThreadedReply(post, parentPostList)
		}

		if parentErr != nil {
			return nil, parentErr
		}
	}

	post.Hashtags, _ = model.ParseHashtags(post.Message)

	if err = a.FillInPostProps(post, channel); err != nil {
		return nil, err
	}

	// Temporary fix so old plugins don't clobber new fields in SlackAttachment struct, see MM-13088
	if attachments, ok := post.Props["attachments"].([]*model.SlackAttachment); ok {
		jsonAttachments, err := json.Marshal(attachments)
		if err == nil {
			attachmentsInterface := []interface{}{}
			err = json.Unmarshal(jsonAttachments, &attachmentsInterface)
			post.Props["attachments"] = attachmentsInterface
		}
		if err != nil {
			mlog.Error("Could not convert post attachments to map interface.", mlog.Err(err))
		}
	}

	if pluginsEnvironment := a.GetPluginsEnvironment(); pluginsEnvironment != nil {
		var rejectionError *model.AppError
		pluginContext := a.PluginContext()
		pluginsEnvironment.RunMultiPluginHook(func(hooks plugin.Hooks) bool {
			replacementPost, rejectionReason := hooks.MessageWillBePosted(pluginContext, post)
			if rejectionReason != "" {
				id := "Post rejected by plugin. " + rejectionReason
				if rejectionReason == plugin.DismissPostError {
					id = plugin.DismissPostError
				}
				rejectionError = model.NewAppError("createPost", id, nil, "", http.StatusBadRequest)
				return false
			}
			if replacementPost != nil {
				post = replacementPost
			}

			return true
		}, plugin.MessageWillBePostedId)

		if rejectionError != nil {
			return nil, rejectionError
		}
	}

	rpost, err := a.Srv().Store.Post().Save(post)
	if err != nil {
		return nil, err
	}

	// Update the mapping from pending post id to the actual post id, for any clients that
	// might be duplicating requests.
	a.Srv().seenPendingPostIdsCache.AddWithExpiresInSecs(post.PendingPostId, rpost.Id, int64(PENDING_POST_IDS_CACHE_TTL.Seconds()))

	if pluginsEnvironment := a.GetPluginsEnvironment(); pluginsEnvironment != nil {
		a.Srv().Go(func() {
			pluginContext := a.PluginContext()
			pluginsEnvironment.RunMultiPluginHook(func(hooks plugin.Hooks) bool {
				hooks.MessageHasBeenPosted(pluginContext, rpost)
				return true
			}, plugin.MessageHasBeenPostedId)
		})
	}

	if a.IsESIndexingEnabled() {
		a.Srv().Go(func() {
			if err = a.Elasticsearch().IndexPost(rpost, channel.TeamId); err != nil {
				mlog.Error("Encountered error indexing post", mlog.String("post_id", post.Id), mlog.Err(err))
			}
		})
	}

	if a.Metrics() != nil {
		a.Metrics().IncrementPostCreate()
	}

	if len(post.FileIds) > 0 {
		if err = a.attachFilesToPost(post); err != nil {
			mlog.Error("Encountered error attaching files to post", mlog.String("post_id", post.Id), mlog.Any("file_ids", post.FileIds), mlog.Err(err))
		}

		if a.Metrics() != nil {
			a.Metrics().IncrementPostFileAttachment(len(post.FileIds))
		}
	}

	// Normally, we would let the API layer call PreparePostForClient, but we do it here since it also needs
	// to be done when we send the post over the websocket in handlePostEvents
	rpost = a.PreparePostForClient(rpost, true, false)

	if err := a.handlePostEvents(rpost, user, channel, triggerWebhooks, parentPostList); err != nil {
		mlog.Error("Failed to handle post events", mlog.Err(err))
	}

	// Send any ephemeral posts after the post is created to ensure it shows up after the latest post created
	if ephemeralPost != nil {
		a.SendEphemeralPost(post.UserId, ephemeralPost)
	}

	return rpost, nil
}

func checkThreadedReply(post *model.Post, parentPostList *model.PostList) *model.AppError {
	if len(parentPostList.Posts) == 0 || !parentPostList.IsChannelId(post.ChannelId) {
		return model.NewAppError("createPost", "api.post.create_post.channel_root_id.app_error", nil, "", http.StatusInternalServerError)
	}

	rootPost := parentPostList.Posts[post.RootId]
	if len(rootPost.RootId) > 0 {
		return model.NewAppError("createPost", "api.post.create_post.root_id.app_error", nil, "", http.StatusBadRequest)
	}

	if post.RootId != post.ParentId {
		parent := parentPostList.Posts[post.ParentId]
		if parent == nil {
			return model.NewAppError("createPost", "api.post.create_post.parent_id.app_error", nil, "", http.StatusInternalServerError)
		}
	}

	return nil
}

func checkNonThreadedReply(post *model.Post, parentPostList *model.PostList) *model.AppError {
	if len(parentPostList.Posts) != 1 {
		return model.NewAppError(
			"createPost",
			"api.post.create_post.simple_reply.not_found",
			map[string]interface{}{
				"found_posts": len(parentPostList.Posts),
				"reply_to_id": post.ReplyToId,
			},
			"failed to find the post specified as the reply target",
			http.StatusNotFound,
		)
	}
	for _, v := range parentPostList.Posts {
		if v.ChannelId != post.ChannelId {
			return model.NewAppError(
				"createPost",
				"api.post.create_post.simple_reply.channel",
				nil,
				"the post specified as the reply target belongs to a different channel",
				http.StatusBadRequest,
			)
		}
	}
	return nil
}

func (a *App) attachFilesToPost(post *model.Post) *model.AppError {
	var attachedIds []string
	for _, fileId := range post.FileIds {
		err := a.Srv().Store.FileInfo().AttachToPost(fileId, post.Id, post.UserId)
		if err != nil {
			mlog.Warn("Failed to attach file to post", mlog.String("file_id", fileId), mlog.String("post_id", post.Id), mlog.Err(err))
			continue
		}

		attachedIds = append(attachedIds, fileId)
	}

	if len(post.FileIds) != len(attachedIds) {
		// We couldn't attach all files to the post, so ensure that post.FileIds reflects what was actually attached
		post.FileIds = attachedIds

		if _, err := a.Srv().Store.Post().Overwrite(post); err != nil {
			return err
		}
	}

	return nil
}

// FillInPostProps should be invoked before saving posts to fill in properties such as
// channel_mentions.
//
// If channel is nil, FillInPostProps will look up the channel corresponding to the post.
func (a *App) FillInPostProps(post *model.Post, channel *model.Channel) *model.AppError {
	channelMentions := post.ChannelMentions()
	channelMentionsProp := make(map[string]interface{})

	if len(channelMentions) > 0 {
		if channel == nil {
			postChannel, err := a.Srv().Store.Channel().GetForPost(post.Id)
			if err != nil {
				return model.NewAppError("FillInPostProps", "api.context.invalid_param.app_error", map[string]interface{}{"Name": "post.channel_id"}, err.Error(), http.StatusBadRequest)
			}
			channel = postChannel
		}

		mentionedChannels, err := a.GetChannelsByNames(channelMentions, channel.TeamId)
		if err != nil {
			return err
		}

		for _, mentioned := range mentionedChannels {
			if mentioned.Type == model.CHANNEL_OPEN {
				channelMentionsProp[mentioned.Name] = map[string]interface{}{
					"display_name": mentioned.DisplayName,
				}
			}
		}
	}

	if len(channelMentionsProp) > 0 {
		post.AddProp("channel_mentions", channelMentionsProp)
	} else if post.Props != nil {
		delete(post.Props, "channel_mentions")
	}

	return nil
}

func (a *App) handlePostEvents(post *model.Post, user *model.User, channel *model.Channel, triggerWebhooks bool, parentPostList *model.PostList) error {
	var team *model.Team
	if len(channel.TeamId) > 0 {
		t, err := a.Srv().Store.Team().Get(channel.TeamId)
		if err != nil {
			return err
		}
		team = t
	} else {
		// Blank team for DMs
		team = &model.Team{}
	}

	a.invalidateCacheForChannel(channel)
	a.invalidateCacheForChannelPosts(channel.Id)

	if _, err := a.SendNotifications(post, team, channel, user, parentPostList); err != nil {
		return err
	}

	a.Srv().Go(func() {
		_, err := a.SendAutoResponseIfNecessary(channel, user)
		if err != nil {
			mlog.Error("Failed to send auto response", mlog.String("user_id", user.Id), mlog.String("post_id", post.Id), mlog.Err(err))
		}
	})

	if triggerWebhooks {
		a.Srv().Go(func() {
			if err := a.handleWebhookEvents(post, team, channel, user); err != nil {
				mlog.Error(err.Error())
			}
		})
	}

	return nil
}

func (a *App) SendEphemeralPost(userId string, post *model.Post) *model.Post {
	post.Type = model.POST_EPHEMERAL

	// fill in fields which haven't been specified which have sensible defaults
	if post.Id == "" {
		post.Id = model.NewId()
	}
	if post.CreateAt == 0 {
		post.CreateAt = model.GetMillis()
	}
	if post.Props == nil {
		post.Props = model.StringInterface{}
	}

	post.GenerateActionIds()
	message := model.NewWebSocketEvent(model.WEBSOCKET_EVENT_EPHEMERAL_MESSAGE, "", post.ChannelId, userId, nil)
	post = a.PreparePostForClient(post, true, false)
	post = model.AddPostActionCookies(post, a.PostActionCookieSecret())
	message.Add("post", post.ToJson())
	a.Publish(message)

	return post
}

func (a *App) UpdateEphemeralPost(userId string, post *model.Post) *model.Post {
	post.Type = model.POST_EPHEMERAL

	post.UpdateAt = model.GetMillis()
	if post.Props == nil {
		post.Props = model.StringInterface{}
	}

	post.GenerateActionIds()
	message := model.NewWebSocketEvent(model.WEBSOCKET_EVENT_POST_EDITED, "", post.ChannelId, userId, nil)
	post = a.PreparePostForClient(post, true, false)
	post = model.AddPostActionCookies(post, a.PostActionCookieSecret())
	message.Add("post", post.ToJson())
	a.Publish(message)

	return post
}

func (a *App) DeleteEphemeralPost(userId, postId string) {
	post := &model.Post{
		Id:       postId,
		UserId:   userId,
		Type:     model.POST_EPHEMERAL,
		DeleteAt: model.GetMillis(),
		UpdateAt: model.GetMillis(),
	}

	message := model.NewWebSocketEvent(model.WEBSOCKET_EVENT_POST_DELETED, "", "", userId, nil)
	message.Add("post", post.ToJson())
	a.Publish(message)
}

func (a *App) UpdatePost(post *model.Post, safeUpdate bool) (*model.Post, *model.AppError) {
	post.SanitizeProps()

	postLists, err := a.Srv().Store.Post().Get(post.Id, false)
	if err != nil {
		return nil, err
	}
	oldPost := postLists.Posts[post.Id]

	if oldPost == nil {
		err = model.NewAppError("UpdatePost", "api.post.update_post.find.app_error", nil, "id="+post.Id, http.StatusBadRequest)
		return nil, err
	}

	if oldPost.DeleteAt != 0 {
		err = model.NewAppError("UpdatePost", "api.post.update_post.permissions_details.app_error", map[string]interface{}{"PostId": post.Id}, "", http.StatusBadRequest)
		return nil, err
	}

	if oldPost.IsSystemMessage() {
		err = model.NewAppError("UpdatePost", "api.post.update_post.system_message.app_error", nil, "id="+post.Id, http.StatusBadRequest)
		return nil, err
	}

	if a.License() != nil {
		if *a.Config().ServiceSettings.PostEditTimeLimit != -1 && model.GetMillis() > oldPost.CreateAt+int64(*a.Config().ServiceSettings.PostEditTimeLimit*1000) && post.Message != oldPost.Message {
			err = model.NewAppError("UpdatePost", "api.post.update_post.permissions_time_limit.app_error", map[string]interface{}{"timeLimit": *a.Config().ServiceSettings.PostEditTimeLimit}, "", http.StatusBadRequest)
			return nil, err
		}
	}

	channel, err := a.GetChannel(oldPost.ChannelId)
	if err != nil {
		return nil, err
	}

	if channel.DeleteAt != 0 {
		return nil, model.NewAppError("UpdatePost", "api.post.update_post.can_not_update_post_in_deleted.error", nil, "", http.StatusBadRequest)
	}

	newPost := &model.Post{}
	*newPost = *oldPost

	if newPost.Message != post.Message {
		newPost.Message = post.Message
		newPost.EditAt = model.GetMillis()
		newPost.Hashtags, _ = model.ParseHashtags(post.Message)
	}

	if !safeUpdate {
		newPost.IsPinned = post.IsPinned
		newPost.HasReactions = post.HasReactions
		newPost.FileIds = post.FileIds
		newPost.Props = post.Props
	}

	// Avoid deep-equal checks if EditAt was already modified through message change
	if newPost.EditAt == oldPost.EditAt && (!oldPost.FileIds.Equals(newPost.FileIds) || !oldPost.AttachmentsEqual(newPost)) {
		newPost.EditAt = model.GetMillis()
	}

	if err = a.FillInPostProps(post, nil); err != nil {
		return nil, err
	}

	if pluginsEnvironment := a.GetPluginsEnvironment(); pluginsEnvironment != nil {
		var rejectionReason string
		pluginContext := a.PluginContext()
		pluginsEnvironment.RunMultiPluginHook(func(hooks plugin.Hooks) bool {
			newPost, rejectionReason = hooks.MessageWillBeUpdated(pluginContext, newPost, oldPost)
			return post != nil
		}, plugin.MessageWillBeUpdatedId)
		if newPost == nil {
			return nil, model.NewAppError("UpdatePost", "Post rejected by plugin. "+rejectionReason, nil, "", http.StatusBadRequest)
		}
	}

	rpost, err := a.Srv().Store.Post().Update(newPost, oldPost)
	if err != nil {
		return nil, err
	}

	if pluginsEnvironment := a.GetPluginsEnvironment(); pluginsEnvironment != nil {
		a.Srv().Go(func() {
			pluginContext := a.PluginContext()
			pluginsEnvironment.RunMultiPluginHook(func(hooks plugin.Hooks) bool {
				hooks.MessageHasBeenUpdated(pluginContext, newPost, oldPost)
				return true
			}, plugin.MessageHasBeenUpdatedId)
		})
	}

	if a.IsESIndexingEnabled() {
		a.Srv().Go(func() {
			channel, chanErr := a.Srv().Store.Channel().GetForPost(rpost.Id)
			if chanErr != nil {
				mlog.Error("Couldn't get channel for post for Elasticsearch indexing.", mlog.String("channel_id", rpost.ChannelId), mlog.String("post_id", rpost.Id))
				return
			}
			if err := a.Elasticsearch().IndexPost(rpost, channel.TeamId); err != nil {
				mlog.Error("Encountered error indexing post", mlog.String("post_id", post.Id), mlog.Err(err))
			}
		})
	}

	rpost = a.PreparePostForClient(rpost, false, true)

	message := model.NewWebSocketEvent(model.WEBSOCKET_EVENT_POST_EDITED, "", rpost.ChannelId, "", nil)
	message.Add("post", rpost.ToJson())
	a.Publish(message)

	a.invalidateCacheForChannelPosts(rpost.ChannelId)

	return rpost, nil
}

func (a *App) PatchPost(postId string, patch *model.PostPatch) (*model.Post, *model.AppError) {
	post, err := a.GetSinglePost(postId)
	if err != nil {
		return nil, err
	}

	channel, err := a.GetChannel(post.ChannelId)
	if err != nil {
		return nil, err
	}

	if channel.DeleteAt != 0 {
		err = model.NewAppError("PatchPost", "api.post.patch_post.can_not_update_post_in_deleted.error", nil, "", http.StatusBadRequest)
		return nil, err
	}

	if !a.HasPermissionToChannel(post.UserId, post.ChannelId, model.PERMISSION_USE_CHANNEL_MENTIONS) {
		patch.DisableMentionHighlights()
	}

	post.Patch(patch)

	updatedPost, err := a.UpdatePost(post, false)
	if err != nil {
		return nil, err
	}

	return updatedPost, nil
}

func (a *App) GetPostsPage(options model.GetPostsOptions) (*model.PostList, *model.AppError) {
	return a.Srv().Store.Post().GetPosts(options, false)
}

func (a *App) GetPosts(channelId string, offset int, limit int) (*model.PostList, *model.AppError) {
	return a.Srv().Store.Post().GetPosts(model.GetPostsOptions{ChannelId: channelId, Page: offset, PerPage: limit}, true)
}

func (a *App) GetPostsEtag(channelId string) string {
	return a.Srv().Store.Post().GetEtag(channelId, true)
}

func (a *App) GetPostsSince(options model.GetPostsSinceOptions) (*model.PostList, *model.AppError) {
	return a.Srv().Store.Post().GetPostsSince(options, true)
}

func (a *App) GetSinglePost(postId string) (*model.Post, *model.AppError) {
	return a.Srv().Store.Post().GetSingle(postId)
}

func (a *App) GetPostThread(postId string, skipFetchThreads bool) (*model.PostList, *model.AppError) {
	return a.Srv().Store.Post().Get(postId, skipFetchThreads)
}

func (a *App) GetFlaggedPosts(userId string, offset int, limit int) (*model.PostList, *model.AppError) {
	return a.Srv().Store.Post().GetFlaggedPosts(userId, offset, limit)
}

func (a *App) GetFlaggedPostsForTeam(userId, teamId string, offset int, limit int) (*model.PostList, *model.AppError) {
	return a.Srv().Store.Post().GetFlaggedPostsForTeam(userId, teamId, offset, limit)
}

func (a *App) GetFlaggedPostsForChannel(userId, channelId string, offset int, limit int) (*model.PostList, *model.AppError) {
	return a.Srv().Store.Post().GetFlaggedPostsForChannel(userId, channelId, offset, limit)
}

func (a *App) GetPermalinkPost(postId string, userId string) (*model.PostList, *model.AppError) {
	list, err := a.Srv().Store.Post().Get(postId, false)
	if err != nil {
		return nil, err
	}

	if len(list.Order) != 1 {
		return nil, model.NewAppError("getPermalinkTmp", "api.post_get_post_by_id.get.app_error", nil, "", http.StatusNotFound)
	}
	post := list.Posts[list.Order[0]]

	channel, err := a.GetChannel(post.ChannelId)
	if err != nil {
		return nil, err
	}

	if err = a.JoinChannel(channel, userId); err != nil {
		return nil, err
	}

	return list, nil
}

func (a *App) GetPostsBeforePost(options model.GetPostsOptions) (*model.PostList, *model.AppError) {
	return a.Srv().Store.Post().GetPostsBefore(options)
}

func (a *App) GetPostsAfterPost(options model.GetPostsOptions) (*model.PostList, *model.AppError) {
	return a.Srv().Store.Post().GetPostsAfter(options)
}

func (a *App) GetPostsAroundPost(before bool, options model.GetPostsOptions) (*model.PostList, *model.AppError) {
	if before {
		return a.Srv().Store.Post().GetPostsBefore(options)
	}
	return a.Srv().Store.Post().GetPostsAfter(options)
}

func (a *App) GetPostAfterTime(channelId string, time int64) (*model.Post, *model.AppError) {
	return a.Srv().Store.Post().GetPostAfterTime(channelId, time)
}

func (a *App) GetPostIdAfterTime(channelId string, time int64) (string, *model.AppError) {
	return a.Srv().Store.Post().GetPostIdAfterTime(channelId, time)
}

func (a *App) GetPostIdBeforeTime(channelId string, time int64) (string, *model.AppError) {
	return a.Srv().Store.Post().GetPostIdBeforeTime(channelId, time)
}

func (a *App) GetNextPostIdFromPostList(postList *model.PostList) string {
	if len(postList.Order) > 0 {
		firstPostId := postList.Order[0]
		firstPost := postList.Posts[firstPostId]
		nextPostId, err := a.GetPostIdAfterTime(firstPost.ChannelId, firstPost.CreateAt)
		if err != nil {
			mlog.Warn("GetNextPostIdFromPostList: failed in getting next post", mlog.Err(err))
		}

		return nextPostId
	}

	return ""
}

func (a *App) GetPrevPostIdFromPostList(postList *model.PostList) string {
	if len(postList.Order) > 0 {
		lastPostId := postList.Order[len(postList.Order)-1]
		lastPost := postList.Posts[lastPostId]
		previousPostId, err := a.GetPostIdBeforeTime(lastPost.ChannelId, lastPost.CreateAt)
		if err != nil {
			mlog.Warn("GetPrevPostIdFromPostList: failed in getting previous post", mlog.Err(err))
		}

		return previousPostId
	}

	return ""
}

// AddCursorIdsForPostList adds NextPostId and PrevPostId as cursor to the PostList.
// The conditional blocks ensure that it sets those cursor IDs immediately as afterPost, beforePost or empty,
// and only query to database whenever necessary.
func (a *App) AddCursorIdsForPostList(originalList *model.PostList, afterPost, beforePost string, since int64, page, perPage int) {
	prevPostIdSet := false
	prevPostId := ""
	nextPostIdSet := false
	nextPostId := ""

	if since > 0 { // "since" query to return empty NextPostId and PrevPostId
		nextPostIdSet = true
		prevPostIdSet = true
	} else if afterPost != "" {
		if page == 0 {
			prevPostId = afterPost
			prevPostIdSet = true
		}

		if len(originalList.Order) < perPage {
			nextPostIdSet = true
		}
	} else if beforePost != "" {
		if page == 0 {
			nextPostId = beforePost
			nextPostIdSet = true
		}

		if len(originalList.Order) < perPage {
			prevPostIdSet = true
		}
	}

	if !nextPostIdSet {
		nextPostId = a.GetNextPostIdFromPostList(originalList)
	}

	if !prevPostIdSet {
		prevPostId = a.GetPrevPostIdFromPostList(originalList)
	}

	originalList.NextPostId = nextPostId
	originalList.PrevPostId = prevPostId
}
func (a *App) GetPostsForChannelAroundLastUnread(channelId, userId string, limitBefore, limitAfter int, skipFetchThreads bool) (*model.PostList, *model.AppError) {
	var member *model.ChannelMember
	var err *model.AppError
	if member, err = a.GetChannelMember(channelId, userId); err != nil {
		return nil, err
	} else if member.LastViewedAt == 0 {
		return model.NewPostList(), nil
	}

	lastUnreadPostId, err := a.GetPostIdAfterTime(channelId, member.LastViewedAt)
	if err != nil {
		return nil, err
	} else if lastUnreadPostId == "" {
		return model.NewPostList(), nil
	}

	postList, err := a.GetPostThread(lastUnreadPostId, skipFetchThreads)
	if err != nil {
		return nil, err
	}
	// Reset order to only include the last unread post: if the thread appears in the centre
	// channel organically, those replies will be added below.
	postList.Order = []string{lastUnreadPostId}

	if postListBefore, err := a.GetPostsBeforePost(model.GetPostsOptions{ChannelId: channelId, PostId: lastUnreadPostId, Page: PAGE_DEFAULT, PerPage: limitBefore, SkipFetchThreads: skipFetchThreads}); err != nil {
		return nil, err
	} else if postListBefore != nil {
		postList.Extend(postListBefore)
	}

	if postListAfter, err := a.GetPostsAfterPost(model.GetPostsOptions{ChannelId: channelId, PostId: lastUnreadPostId, Page: PAGE_DEFAULT, PerPage: limitAfter - 1, SkipFetchThreads: skipFetchThreads}); err != nil {
		return nil, err
	} else if postListAfter != nil {
		postList.Extend(postListAfter)
	}

	postList.SortByCreateAt()
	return postList, nil
}

func (a *App) DeletePost(postId, deleteByID string) (*model.Post, *model.AppError) {
	post, err := a.Srv().Store.Post().GetSingle(postId)
	if err != nil {
		err.StatusCode = http.StatusBadRequest
		return nil, err
	}

	channel, err := a.GetChannel(post.ChannelId)
	if err != nil {
		return nil, err
	}

	if channel.DeleteAt != 0 {
		err := model.NewAppError("DeletePost", "api.post.delete_post.can_not_delete_post_in_deleted.error", nil, "", http.StatusBadRequest)
		return nil, err
	}

	if err := a.Srv().Store.Post().Delete(postId, model.GetMillis(), deleteByID); err != nil {
		return nil, err
	}

	message := model.NewWebSocketEvent(model.WEBSOCKET_EVENT_POST_DELETED, "", post.ChannelId, "", nil)
	message.Add("post", a.PreparePostForClient(post, false, false).ToJson())
	a.Publish(message)

	a.Srv().Go(func() {
		a.DeletePostFiles(post)
	})
	a.Srv().Go(func() {
		a.DeleteFlaggedPosts(post.Id)
	})

	if a.IsESIndexingEnabled() {
		a.Srv().Go(func() {
			if err := a.Elasticsearch().DeletePost(post); err != nil {
				mlog.Error("Encountered error deleting post", mlog.String("post_id", post.Id), mlog.Err(err))
			}
		})
	}

	a.invalidateCacheForChannelPosts(post.ChannelId)

	return post, nil
}

func (a *App) DeleteFlaggedPosts(postId string) {
	if err := a.Srv().Store.Preference().DeleteCategoryAndName(model.PREFERENCE_CATEGORY_FLAGGED_POST, postId); err != nil {
		mlog.Warn("Unable to delete flagged post preference when deleting post.", mlog.Err(err))
		return
	}
}

func (a *App) DeletePostFiles(post *model.Post) {
	if len(post.FileIds) == 0 {
		return
	}

	if _, err := a.Srv().Store.FileInfo().DeleteForPost(post.Id); err != nil {
		mlog.Warn("Encountered error when deleting files for post", mlog.String("post_id", post.Id), mlog.Err(err))
	}
}

func (a *App) parseAndFetchChannelIdByNameFromInFilter(channelName, userId, teamId string, includeDeleted bool) (*model.Channel, error) {
	if strings.HasPrefix(channelName, "@") && strings.Contains(channelName, ",") {
		var userIds []string
		users, err := a.GetUsersByUsernames(strings.Split(channelName[1:], ","), false, nil)
		if err != nil {
			return nil, err
		}
		for _, user := range users {
			userIds = append(userIds, user.Id)
		}

		channel, err := a.GetGroupChannel(userIds)
		if err != nil {
			return nil, err
		}
		return channel, nil
	}

	if strings.HasPrefix(channelName, "@") && !strings.Contains(channelName, ",") {
		user, err := a.GetUserByUsername(channelName[1:])
		if err != nil {
			return nil, err
		}
		channel, err := a.GetOrCreateDirectChannel(userId, user.Id)
		if err != nil {
			return nil, err
		}
		return channel, nil
	}

	channel, err := a.GetChannelByName(channelName, teamId, includeDeleted)
	if err != nil {
		return nil, err
	}
	return channel, nil
}

func (a *App) searchPostsInTeam(teamId string, userId string, paramsList []*model.SearchParams, modifierFun func(*model.SearchParams)) (*model.PostList, *model.AppError) {
	var wg sync.WaitGroup

	pchan := make(chan store.StoreResult, len(paramsList))

	for _, params := range paramsList {
		// Don't allow users to search for everything.
		if params.Terms == "*" {
			continue
		}
		modifierFun(params)
		wg.Add(1)

		go func(params *model.SearchParams) {
			defer wg.Done()
			postList, err := a.Srv().Store.Post().Search(teamId, userId, params)
			pchan <- store.StoreResult{Data: postList, Err: err}
		}(params)
	}

	wg.Wait()
	close(pchan)

	posts := model.NewPostList()

	for result := range pchan {
		if result.Err != nil {
			return nil, result.Err
		}
		data := result.Data.(*model.PostList)
		posts.Extend(data)
	}

	posts.SortByCreateAt()
	return posts, nil
}

func (a *App) convertChannelNamesToChannelIds(channels []string, userId string, teamId string, includeDeletedChannels bool) []string {
	for idx, channelName := range channels {
		channel, err := a.parseAndFetchChannelIdByNameFromInFilter(channelName, userId, teamId, includeDeletedChannels)
		if err != nil {
			mlog.Error("error getting channel id by name from in filter", mlog.Err(err))
			continue
		}
		channels[idx] = channel.Id
	}
	return channels
}

func (a *App) convertUserNameToUserIds(usernames []string) []string {
	for idx, username := range usernames {
		if user, err := a.GetUserByUsername(username); err != nil {
			mlog.Error("error getting user by username", mlog.String("user_name", username), mlog.Err(err))
		} else {
			usernames[idx] = user.Id
		}
	}
	return usernames
}

func (a *App) SearchPostsInTeam(teamId string, paramsList []*model.SearchParams) (*model.PostList, *model.AppError) {
	if !*a.Config().ServiceSettings.EnablePostSearch {
		return nil, model.NewAppError("SearchPostsInTeam", "store.sql_post.search.disabled", nil, fmt.Sprintf("teamId=%v", teamId), http.StatusNotImplemented)
	}
	return a.searchPostsInTeam(teamId, "", paramsList, func(params *model.SearchParams) {
		params.SearchWithoutUserId = true
	})
}

func (a *App) esSearchPostsInTeamForUser(paramsList []*model.SearchParams, userId, teamId string, isOrSearch, includeDeletedChannels bool, page, perPage int) (*model.PostSearchResults, *model.AppError) {
	finalParamsList := []*model.SearchParams{}
	includeDeleted := includeDeletedChannels && *a.Config().TeamSettings.ExperimentalViewArchivedChannels

	for _, params := range paramsList {
		params.OrTerms = isOrSearch
		// Don't allow users to search for "*"
		if params.Terms != "*" {
			// Convert channel names to channel IDs
			params.InChannels = a.convertChannelNamesToChannelIds(params.InChannels, userId, teamId, includeDeletedChannels)
			params.ExcludedChannels = a.convertChannelNamesToChannelIds(params.ExcludedChannels, userId, teamId, includeDeletedChannels)

			// Convert usernames to user IDs
			params.FromUsers = a.convertUserNameToUserIds(params.FromUsers)
			params.ExcludedUsers = a.convertUserNameToUserIds(params.ExcludedUsers)

			finalParamsList = append(finalParamsList, params)
		}
	}

	// If the processed search params are empty, return empty search results.
	if len(finalParamsList) == 0 {
		return model.MakePostSearchResults(model.NewPostList(), nil), nil
	}

	// We only allow the user to search in channels they are a member of.
	userChannels, err := a.GetChannelsForUser(teamId, userId, includeDeleted)
	if err != nil {
		mlog.Error("error getting channel for user", mlog.Err(err))
		return nil, err
	}

	postIds, matches, err := a.Elasticsearch().SearchPosts(userChannels, finalParamsList, page, perPage)
	if err != nil {
		return nil, err
	}

	// Get the posts
	postList := model.NewPostList()
	if len(postIds) > 0 {
		posts, err := a.Srv().Store.Post().GetPostsByIds(postIds)
		if err != nil {
			return nil, err
		}
		for _, p := range posts {
			if p.DeleteAt == 0 {
				postList.AddPost(p)
				postList.AddOrder(p.Id)
			}
		}
	}

	return model.MakePostSearchResults(postList, matches), nil
}

func (a *App) SearchPostsInTeamForUser(terms string, userId string, teamId string, isOrSearch bool, includeDeletedChannels bool, timeZoneOffset int, page, perPage int) (*model.PostSearchResults, *model.AppError) {
	var postSearchResults *model.PostSearchResults
	var err *model.AppError
	paramsList := model.ParseSearchParams(strings.TrimSpace(terms), timeZoneOffset)

	if !*a.Config().ServiceSettings.EnablePostSearch {
		return nil, model.NewAppError("SearchPostsInTeamForUser", "store.sql_post.search.disabled", nil, fmt.Sprintf("teamId=%v userId=%v", teamId, userId), http.StatusNotImplemented)
	}

	if a.IsESSearchEnabled() {
		postSearchResults, err = a.esSearchPostsInTeamForUser(paramsList, userId, teamId, isOrSearch, includeDeletedChannels, page, perPage)
		if err != nil {
			mlog.Error("Encountered error on SearchPostsInTeamForUser through Elasticsearch. Falling back to default search.", mlog.Err(err))
		}
	}

	if !a.IsESSearchEnabled() || err != nil {
		// Since we don't support paging for DB search, we just return nothing for later pages
		if page > 0 {
			return model.MakePostSearchResults(model.NewPostList(), nil), nil
		}

		includeDeleted := includeDeletedChannels && *a.Config().TeamSettings.ExperimentalViewArchivedChannels
		posts, err := a.searchPostsInTeam(teamId, userId, paramsList, func(params *model.SearchParams) {
			params.IncludeDeletedChannels = includeDeleted
			params.OrTerms = isOrSearch
			for idx, channelName := range params.InChannels {
				if strings.HasPrefix(channelName, "@") {
					channel, err := a.parseAndFetchChannelIdByNameFromInFilter(channelName, userId, teamId, includeDeletedChannels)
					if err != nil {
						mlog.Error("error getting channel_id by name from in filter", mlog.Err(err))
						continue
					}
					params.InChannels[idx] = channel.Name
				}
			}
			for idx, channelName := range params.ExcludedChannels {
				if strings.HasPrefix(channelName, "@") {
					channel, err := a.parseAndFetchChannelIdByNameFromInFilter(channelName, userId, teamId, includeDeletedChannels)
					if err != nil {
						mlog.Error("error getting channel_id by name from in filter", mlog.Err(err))
						continue
					}
					params.ExcludedChannels[idx] = channel.Name
				}
			}
		})
		if err != nil {
			return nil, err
		}

		postSearchResults = model.MakePostSearchResults(posts, nil)
	}

	return postSearchResults, nil
}

func (a *App) GetFileInfosForPostWithMigration(postId string) ([]*model.FileInfo, *model.AppError) {

	pchan := make(chan store.StoreResult, 1)
	go func() {
		post, err := a.Srv().Store.Post().GetSingle(postId)
		pchan <- store.StoreResult{Data: post, Err: err}
		close(pchan)
	}()

	infos, err := a.GetFileInfosForPost(postId, false)
	if err != nil {
		return nil, err
	}

	if len(infos) == 0 {
		// No FileInfos were returned so check if they need to be created for this post
		result := <-pchan
		if result.Err != nil {
			return nil, result.Err
		}
		post := result.Data.(*model.Post)

		if len(post.Filenames) > 0 {
			a.Srv().Store.FileInfo().InvalidateFileInfosForPostCache(postId, false)
			a.Srv().Store.FileInfo().InvalidateFileInfosForPostCache(postId, true)
			// The post has Filenames that need to be replaced with FileInfos
			infos = a.MigrateFilenamesToFileInfos(post)
		}
	}

	return infos, nil
}

func (a *App) GetFileInfosForPost(postId string, fromMaster bool) ([]*model.FileInfo, *model.AppError) {
	return a.Srv().Store.FileInfo().GetForPost(postId, fromMaster, false, true)
}

func (a *App) PostWithProxyAddedToImageURLs(post *model.Post) *model.Post {
	if f := a.ImageProxyAdder(); f != nil {
		return post.WithRewrittenImageURLs(f)
	}
	return post
}

func (a *App) PostWithProxyRemovedFromImageURLs(post *model.Post) *model.Post {
	if f := a.ImageProxyRemover(); f != nil {
		return post.WithRewrittenImageURLs(f)
	}
	return post
}

func (a *App) PostPatchWithProxyRemovedFromImageURLs(patch *model.PostPatch) *model.PostPatch {
	if f := a.ImageProxyRemover(); f != nil {
		return patch.WithRewrittenImageURLs(f)
	}
	return patch
}

func (a *App) ImageProxyAdder() func(string) string {
	if !*a.Config().ImageProxySettings.Enable {
		return nil
	}

	return func(url string) string {
		return a.Srv().ImageProxy.GetProxiedImageURL(url)
	}
}

func (a *App) ImageProxyRemover() (f func(string) string) {
	if !*a.Config().ImageProxySettings.Enable {
		return nil
	}

	return func(url string) string {
		return a.Srv().ImageProxy.GetUnproxiedImageURL(url)
	}
}

func (a *App) MaxPostSize() int {
	maxPostSize := a.Srv().Store.Post().GetMaxPostSize()
	if maxPostSize == 0 {
		return model.POST_MESSAGE_MAX_RUNES_V1
	}

	return maxPostSize
}

// countMentionsFromPost returns the number of posts in the post's channel that mention the user after and including the
// given post.
func (a *App) countMentionsFromPost(user *model.User, post *model.Post) (int, *model.AppError) {
	channel, err := a.GetChannel(post.ChannelId)
	if err != nil {
		return 0, err
	}

	if channel.Type == model.CHANNEL_DIRECT {
		// In a DM channel, every post made by the other user is a mention
		count, countErr := a.Srv().Store.Channel().CountPostsAfter(post.ChannelId, post.CreateAt-1, channel.GetOtherUserIdForDM(user.Id))
		if countErr != nil {
			return 0, countErr
		}

		return count, countErr
	}

	channelMember, err := a.GetChannelMember(channel.Id, user.Id)
	if err != nil {
		return 0, err
	}

	keywords := addMentionKeywordsForUser(
		map[string][]string{},
		user,
		channelMember.NotifyProps,
		&model.Status{Status: model.STATUS_ONLINE}, // Assume the user is online since they would've triggered this
		true, // Assume channel mentions are always allowed for simplicity
	)
	commentMentions := user.NotifyProps[model.COMMENTS_NOTIFY_PROP]
	checkForCommentMentions := commentMentions == model.COMMENTS_NOTIFY_ROOT || commentMentions == model.COMMENTS_NOTIFY_ANY

	// A mapping of thread root IDs to whether or not a post in that thread mentions the user
	mentionedByThread := make(map[string]bool)

	thread, err := a.GetPostThread(post.Id, false)
	if err != nil {
		return 0, err
	}

	count := 0

	if isPostMention(user, post, keywords, thread.Posts, mentionedByThread, checkForCommentMentions) {
		count += 1
	}

	page := 0
	perPage := 200
	for {
		postList, err := a.GetPostsAfterPost(model.GetPostsOptions{
			ChannelId: post.ChannelId,
			PostId:    post.Id,
			Page:      page,
			PerPage:   perPage,
		})
		if err != nil {
			return 0, err
		}

		for _, postId := range postList.Order {
			if isPostMention(user, postList.Posts[postId], keywords, postList.Posts, mentionedByThread, checkForCommentMentions) {
				count += 1
			}
		}

		if len(postList.Order) < perPage {
			break
		}

		page += 1
	}

	return count, nil
}

func isCommentMention(user *model.User, post *model.Post, otherPosts map[string]*model.Post, mentionedByThread map[string]bool) bool {
	if post.RootId == "" {
		// Not a comment
		return false
	}

	if mentioned, ok := mentionedByThread[post.RootId]; ok {
		// We've already figured out if the user was mentioned by this thread
		return mentioned
	}

	// Whether or not the user was mentioned because they started the thread
	mentioned := otherPosts[post.RootId].UserId == user.Id

	// Or because they commented on it before this post
	if !mentioned && user.NotifyProps[model.COMMENTS_NOTIFY_PROP] == model.COMMENTS_NOTIFY_ANY {
		for _, otherPost := range otherPosts {
			if otherPost.Id == post.Id {
				continue
			}

			if otherPost.RootId != post.RootId {
				continue
			}

			if otherPost.UserId == user.Id && otherPost.CreateAt < post.CreateAt {
				// Found a comment made by the user from before this post
				mentioned = true
				break
			}
		}
	}

	mentionedByThread[post.RootId] = mentioned
	return mentioned
}

func isPostMention(user *model.User, post *model.Post, keywords map[string][]string, otherPosts map[string]*model.Post, mentionedByThread map[string]bool, checkForCommentMentions bool) bool {
	// Prevent the user from mentioning themselves
	if post.UserId == user.Id && post.Props["from_webhook"] != "true" {
		return false
	}

	// Check for keyword mentions
	mentions := getExplicitMentions(post, keywords)
	if _, ok := mentions.Mentions[user.Id]; ok {
		return true
	}

	// Check for mentions caused by being added to the channel
	if post.Type == model.POST_ADD_TO_CHANNEL {
		if addedUserId, ok := post.Props[model.POST_PROPS_ADDED_USER_ID].(string); ok && addedUserId == user.Id {
			return true
		}
	}

	// Check for comment mentions
	if checkForCommentMentions && isCommentMention(user, post, otherPosts, mentionedByThread) {
		return true
	}

	return false
}

// GetRecentPosts() returns a list of most recent posts for given channels.
//
func (a *App) GetRecentPosts(request *model.RecentPostsRequestData) (*model.PostListSimple, *model.AppError) {
	if request.MaxTotalMessages > MAX_RECENT_TOTAL {
		return nil, model.NewAppError("GetRecentPosts", "app.post.get_recent_posts.total_too_big.app_error", nil, "", http.StatusBadRequest)
	}
	if request.MessagesPerChannel > MAX_RECENT_PER_CHANNEL {
		return nil, model.NewAppError("GetRecentPosts", "app.post.get_recent_posts.per_channel_too_big.app_error", nil, "", http.StatusBadRequest)
	}
	if request.MaxTotalMessages < request.MessagesPerChannel {
		return nil, model.NewAppError("GetRecentPosts", "app.post.get_recent_posts.total_less_than_per_channel.app_error", nil, "", http.StatusBadRequest)
	}

	total := len(request.ChannelIds)
	// Don't load more posts if MessagesPerChannel doesn't fit the remaining amount before reaching the limit
	lim := request.MaxTotalMessages - request.MessagesPerChannel
	processedChannels := 0
	collectedPosts := 0
	var ids []string
	var posts = []*[]model.Post{}
	// Collect posts in batches.
	// Stop when either all of the requested channels have been processed,
	// or total posts limit has been reached.
	for processedChannels < total && collectedPosts <= lim {
		// Each batch is guaranteed to fit the limit for total messages,
		// even if each channel in a batch has MessagesPerChannel messages.
		// This means it's either a channel will have a full slice of posts up to MessagesPerChannel,
		// or none at all (which means the client has to request again).
		//
		// Example: 5 channels have 20 messages each.
		// Requested: 100 messages max in total, 30 messages max per channel.
		// Result: 80 messages for the first 4 channels in the list, because there is no way
		// of guessing whether the last channel will fit the total limit or not,
		// and the method avoids requesting extra data.
		//
		batch := (request.MaxTotalMessages - collectedPosts) / request.MessagesPerChannel
		if processedChannels+batch > total {
			ids = request.ChannelIds[processedChannels:]
		} else {
			ids = request.ChannelIds[processedChannels : processedChannels+batch]
		}

		chunk, err := a.Srv().Store.Post().GetRecentPosts(&ids, request.MessagesPerChannel)
		if err != nil {
			return nil, err
		}
		posts = append(posts, chunk)
		processedChannels += len(ids)
		collectedPosts += len(*chunk)
	}

	// Copy pointers to a flattened list
	result := model.PostListSimple{}
	for i := range posts {
		// Reverse the order as the db returns posts sorted by date from the most recent to the oldest
		batch := convertPostsToPtrs(posts[i], true)
		result = append(result, *batch...)
	}
	return &result, nil
}

func convertPostsToPtrs(posts *[]model.Post, reverse bool) *[]*model.Post {
	result := make([]*model.Post, len(*posts))[:0]
	l := len(*posts)
	if reverse {
		for i := l - 1; i >= 0; i-- {
			result = append(result, &((*posts)[i]))
		}
	} else {
		for i := range *posts {
			result = append(result, &((*posts)[i]))
		}
	}
	return &result
}

// CheckIncrementPossible() returns true if it is reasonable to proceed with downloading incremental updates for given chats.
//
func (a *App) CheckIncrementPossible(request *model.IncrementCheckRequest) (bool, *model.AppError) {
	channelsWithPosts := make([]model.ChannelWithPost, 0, len(request.Channels))
	missingChannels := []string{}
	for _, v := range request.Channels {
		if len(v.PostId) == 0 {
			missingChannels = append(missingChannels, v.ChannelId)
		} else {
			channelsWithPosts = append(channelsWithPosts, v)
		}
	}

	count, err := a.Srv().Store.Post().GetPostCountAfter(&channelsWithPosts)
	if err != nil {
		return false, err
	}

	var missingCount int64 = 0
	if len(missingChannels) > 0 {
		missingCount, err = a.Srv().Store.Post().GetTotalPosts(&missingChannels)
		if err != nil {
			return false, err
		}
	}
	return count+missingCount <= MAX_INCREMENT_TOTAL, nil
}

// GetIncrementalPosts() returns a list of posts for given channels after the given last post id
// for each channel. The response is paginated and the method respects the limit of maximum
// messages per page. The client can provide a desired page size no more than MAX_INCREMENT_PAGE.
//
func (a *App) GetIncrementalPosts(request *model.IncrementPostsRequest) (*[]model.IncrementalPosts, *model.AppError) {
	if request.MaxMessages > MAX_INCREMENT_PAGE {
		return nil, model.NewAppError("GetIncrementalPosts", "app.post.get_incremental_posts.page_too_big.app_error", nil, "", http.StatusBadRequest)
	}

	// Collect channel ids for which the request doesn't have a post id
	channelsWithMissingPosts := []string{}
	for _, v := range request.Channels {
		if len(v.PostId) == 0 {
			channelsWithMissingPosts = append(channelsWithMissingPosts, v.ChannelId)
		}
	}

	// Get oldest posts for each channel that doesn't have a post id specified in the request
	oldestPosts, err := a.Srv().Store.Post().GetOldestPostsForChannels(&channelsWithMissingPosts)
	if err != nil {
		return nil, model.NewAppError("GetIncrementalPosts", "app.post.get_incremental_posts.get_oldest_posts.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	// filter non-empty channels
	channelsWithPosts := make([]model.ChannelWithPost, len(request.Channels))[:0]
	includeIds := make([]string, len(*oldestPosts))[:0]
	for _, v := range request.Channels {
		firstPost, hasFirst := (*oldestPosts)[v.ChannelId]
		if hasFirst && len(firstPost) > 0 {
			v.PostId = firstPost
			includeIds = append(includeIds, firstPost)
		}
		if len(v.PostId) != 0 {
			channelsWithPosts = append(channelsWithPosts, v)
		}
	}

	// Get the number of posts after a given post id for each non-empty channel
	counts, err := a.Srv().Store.Post().GetPostCountAfterForChannels(&channelsWithPosts)
	if err != nil {
		return nil, model.NewAppError("GetIncrementalPosts", "app.post.get_incremental_posts.posts_counting.app_error", nil, err.Error(), http.StatusInternalServerError)
	}
	// Add zero counts to the map
	for channelId, firstPostId := range *oldestPosts {
		if len(firstPostId) == 0 {
			(*counts)[channelId] = 0
		}
	}

	// Get a slice of channels that fits MaxMessages limit
	// Don't collect channels with zero counts.
	pageChannels := make([]model.ChannelWithPost, len(request.Channels))[:0]
	var trimmedChannel string = ""
	var total int
	for _, v := range request.Channels {
		channelCount := (*counts)[v.ChannelId]
		if channelCount == 0 {
			continue
		}
		if total+channelCount > request.MaxMessages {
			// Channel will have some of its messages, but not all of them fit the page limit
			channelCount = request.MaxMessages - total
			(*counts)[v.ChannelId] = channelCount
			trimmedChannel = v.ChannelId
		}
		total += channelCount
		pageChannels = append(pageChannels, v)
		if total >= request.MaxMessages {
			break
		}
	}

	// Get all the messages for all of the channels that fit the page limit
	posts, err := a.Srv().Store.Post().GetAllPostsAfter(&pageChannels, &includeIds, counts)
	if err != nil {
		return nil, model.NewAppError("GetIncrementalPosts", "app.post.get_incremental_posts.get_posts.app_error", nil, err.Error(), http.StatusInternalServerError)
	}

	result := make([]model.IncrementalPosts, len(request.Channels))[:0]
	for _, channel := range request.Channels {
		// If channel has zero posts, always add it to the results
		if c, exists := (*counts)[channel.ChannelId]; exists && c == 0 {
			result = append(result, model.IncrementalPosts{
				ChannelId: channel.ChannelId,
				Posts:     &model.PostListSimple{},
				Complete:  true,
			})
		} else if list, exists := (*posts)[channel.ChannelId]; exists {
			var batch model.PostListSimple = *list
			var trimmed bool
			if len(*list) > c && trimmedChannel == channel.ChannelId {
				// Limit the amount of messages received to the measured count.
				// The list can have more messages than previously measured in two cases:
				// 1. new posts have been published while we are here
				// 2. this is the last channel in the sequence and it has more messages than fits the page limit
				batch = (*list)[0:c]
				trimmed = true
			} else {
				trimmed = false
			}
			result = append(result, model.IncrementalPosts{
				ChannelId: channel.ChannelId,
				Posts:     &batch,
				Complete:  len(*list) >= c && !trimmed,
			})
		}
	}

	return &result, nil
}
