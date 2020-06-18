// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package app

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-server/v5/einterfaces/mocks"
	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/plugin/plugintest/mock"
	"github.com/mattermost/mattermost-server/v5/store/storetest"
	storemocks "github.com/mattermost/mattermost-server/v5/store/storetest/mocks"
)

func TestCreatePostDeduplicate(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	t.Run("duplicate create post is idempotent", func(t *testing.T) {
		pendingPostId := model.NewId()
		post, err := th.App.CreatePostAsUser(&model.Post{
			UserId:        th.BasicUser.Id,
			ChannelId:     th.BasicChannel.Id,
			Message:       "message",
			PendingPostId: pendingPostId,
		}, "")
		require.Nil(t, err)
		require.Equal(t, "message", post.Message)

		duplicatePost, err := th.App.CreatePostAsUser(&model.Post{
			UserId:        th.BasicUser.Id,
			ChannelId:     th.BasicChannel.Id,
			Message:       "message",
			PendingPostId: pendingPostId,
		}, "")
		require.Nil(t, err)
		require.Equal(t, post.Id, duplicatePost.Id, "should have returned previously created post id")
		require.Equal(t, "message", duplicatePost.Message)
	})

	t.Run("post rejected by plugin leaves cache ready for non-deduplicated try", func(t *testing.T) {
		setupPluginApiTest(t, `
			package main

			import (
				"github.com/mattermost/mattermost-server/v5/plugin"
				"github.com/mattermost/mattermost-server/v5/model"
			)

			type MyPlugin struct {
				plugin.MattermostPlugin
				allow bool
			}

			func (p *MyPlugin) MessageWillBePosted(c *plugin.Context, post *model.Post) (*model.Post, string) {
				if !p.allow {
					p.allow = true
					return nil, "rejected"
				}

				return nil, ""
			}

			func main() {
				plugin.ClientMain(&MyPlugin{})
			}
		`, `{"id": "testrejectfirstpost", "backend": {"executable": "backend.exe"}}`, "testrejectfirstpost", th.App)

		pendingPostId := model.NewId()
		post, err := th.App.CreatePostAsUser(&model.Post{
			UserId:        th.BasicUser.Id,
			ChannelId:     th.BasicChannel.Id,
			Message:       "message",
			PendingPostId: pendingPostId,
		}, "")
		require.NotNil(t, err)
		require.Equal(t, "Post rejected by plugin. rejected", err.Id)
		require.Nil(t, post)

		duplicatePost, err := th.App.CreatePostAsUser(&model.Post{
			UserId:        th.BasicUser.Id,
			ChannelId:     th.BasicChannel.Id,
			Message:       "message",
			PendingPostId: pendingPostId,
		}, "")
		require.Nil(t, err)
		require.Equal(t, "message", duplicatePost.Message)
	})

	t.Run("slow posting after cache entry blocks duplicate request", func(t *testing.T) {
		setupPluginApiTest(t, `
			package main

			import (
				"github.com/mattermost/mattermost-server/v5/plugin"
				"github.com/mattermost/mattermost-server/v5/model"
				"time"
			)

			type MyPlugin struct {
				plugin.MattermostPlugin
				instant bool
			}

			func (p *MyPlugin) MessageWillBePosted(c *plugin.Context, post *model.Post) (*model.Post, string) {
				if !p.instant {
					p.instant = true
					time.Sleep(3 * time.Second)
				}

				return nil, ""
			}

			func main() {
				plugin.ClientMain(&MyPlugin{})
			}
		`, `{"id": "testdelayfirstpost", "backend": {"executable": "backend.exe"}}`, "testdelayfirstpost", th.App)

		var post *model.Post
		pendingPostId := model.NewId()

		wg := sync.WaitGroup{}

		// Launch a goroutine to make the first CreatePost call that will get delayed
		// by the plugin above.
		wg.Add(1)
		go func() {
			defer wg.Done()
			var err error
			post, err = th.App.CreatePostAsUser(&model.Post{
				UserId:        th.BasicUser.Id,
				ChannelId:     th.BasicChannel.Id,
				Message:       "plugin delayed",
				PendingPostId: pendingPostId,
			}, "")
			require.Nil(t, err)
			require.Equal(t, post.Message, "plugin delayed")
		}()

		// Give the goroutine above a chance to start and get delayed by the plugin.
		time.Sleep(2 * time.Second)

		// Try creating a duplicate post
		duplicatePost, err := th.App.CreatePostAsUser(&model.Post{
			UserId:        th.BasicUser.Id,
			ChannelId:     th.BasicChannel.Id,
			Message:       "plugin delayed",
			PendingPostId: pendingPostId,
		}, "")
		require.NotNil(t, err)
		require.Equal(t, "api.post.deduplicate_create_post.pending", err.Id)
		require.Nil(t, duplicatePost)

		// Wait for the first CreatePost to finish to ensure assertions are made.
		wg.Wait()
	})

	t.Run("duplicate create post after cache expires is not idempotent", func(t *testing.T) {
		pendingPostId := model.NewId()
		post, err := th.App.CreatePostAsUser(&model.Post{
			UserId:        th.BasicUser.Id,
			ChannelId:     th.BasicChannel.Id,
			Message:       "message",
			PendingPostId: pendingPostId,
		}, "")
		require.Nil(t, err)
		require.Equal(t, "message", post.Message)

		time.Sleep(PENDING_POST_IDS_CACHE_TTL)

		duplicatePost, err := th.App.CreatePostAsUser(&model.Post{
			UserId:        th.BasicUser.Id,
			ChannelId:     th.BasicChannel.Id,
			Message:       "message",
			PendingPostId: pendingPostId,
		}, "")
		require.Nil(t, err)
		require.NotEqual(t, post.Id, duplicatePost.Id, "should have created new post id")
		require.Equal(t, "message", duplicatePost.Message)
	})
}

func TestAttachFilesToPost(t *testing.T) {
	t.Run("should attach files", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		info1, err := th.App.Srv().Store.FileInfo().Save(&model.FileInfo{
			CreatorId: th.BasicUser.Id,
			Path:      "path.txt",
		})
		require.Nil(t, err)

		info2, err := th.App.Srv().Store.FileInfo().Save(&model.FileInfo{
			CreatorId: th.BasicUser.Id,
			Path:      "path.txt",
		})
		require.Nil(t, err)

		post := th.BasicPost
		post.FileIds = []string{info1.Id, info2.Id}

		err = th.App.attachFilesToPost(post)
		assert.Nil(t, err)

		infos, err := th.App.GetFileInfosForPost(post.Id, false)
		assert.Nil(t, err)
		assert.Len(t, infos, 2)
	})

	t.Run("should update File.PostIds after failing to add files", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		info1, err := th.App.Srv().Store.FileInfo().Save(&model.FileInfo{
			CreatorId: th.BasicUser.Id,
			Path:      "path.txt",
			PostId:    model.NewId(),
		})
		require.Nil(t, err)

		info2, err := th.App.Srv().Store.FileInfo().Save(&model.FileInfo{
			CreatorId: th.BasicUser.Id,
			Path:      "path.txt",
		})
		require.Nil(t, err)

		post := th.BasicPost
		post.FileIds = []string{info1.Id, info2.Id}

		err = th.App.attachFilesToPost(post)
		assert.Nil(t, err)

		infos, err := th.App.GetFileInfosForPost(post.Id, false)
		assert.Nil(t, err)
		assert.Len(t, infos, 1)
		assert.Equal(t, info2.Id, infos[0].Id)

		updated, err := th.App.GetSinglePost(post.Id)
		require.Nil(t, err)
		assert.Len(t, updated.FileIds, 1)
		assert.Contains(t, updated.FileIds, info2.Id)
	})
}

func TestUpdatePostEditAt(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	post := &model.Post{}
	*post = *th.BasicPost

	post.IsPinned = true
	saved, err := th.App.UpdatePost(post, true)
	require.Nil(t, err)
	assert.Equal(t, saved.EditAt, post.EditAt, "shouldn't have updated post.EditAt when pinning post")
	*post = *saved

	time.Sleep(time.Millisecond * 100)

	post.Message = model.NewId()
	saved, err = th.App.UpdatePost(post, true)
	require.Nil(t, err)
	assert.NotEqual(t, saved.EditAt, post.EditAt, "should have updated post.EditAt when updating post message")

	time.Sleep(time.Millisecond * 200)
}

func TestUpdatePostTimeLimit(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	post := &model.Post{}
	*post = *th.BasicPost

	th.App.SetLicense(model.NewTestLicense())

	th.App.UpdateConfig(func(cfg *model.Config) {
		*cfg.ServiceSettings.PostEditTimeLimit = -1
	})
	_, err := th.App.UpdatePost(post, true)
	require.Nil(t, err)

	th.App.UpdateConfig(func(cfg *model.Config) {
		*cfg.ServiceSettings.PostEditTimeLimit = 1000000000
	})
	post.Message = model.NewId()

	_, err = th.App.UpdatePost(post, true)
	require.Nil(t, err, "should allow you to edit the post")

	th.App.UpdateConfig(func(cfg *model.Config) {
		*cfg.ServiceSettings.PostEditTimeLimit = 1
	})
	post.Message = model.NewId()
	_, err = th.App.UpdatePost(post, true)
	require.NotNil(t, err, "should fail on update old post")

	th.App.UpdateConfig(func(cfg *model.Config) {
		*cfg.ServiceSettings.PostEditTimeLimit = -1
	})
}

func TestUpdatePostInArchivedChannel(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	archivedChannel := th.CreateChannel(th.BasicTeam)
	post := th.CreatePost(archivedChannel)
	th.App.DeleteChannel(archivedChannel, "")

	_, err := th.App.UpdatePost(post, true)
	require.NotNil(t, err)
	require.Equal(t, "api.post.update_post.can_not_update_post_in_deleted.error", err.Id)
}

func TestPostReplyToPostWhereRootPosterLeftChannel(t *testing.T) {
	// This test ensures that when replying to a root post made by a user who has since left the channel, the reply
	// post completes successfully. This is a regression test for PLT-6523.
	th := Setup(t).InitBasic()
	defer th.TearDown()

	channel := th.BasicChannel
	userInChannel := th.BasicUser2
	userNotInChannel := th.BasicUser
	rootPost := th.BasicPost

	_, err := th.App.AddUserToChannel(userInChannel, channel)
	require.Nil(t, err)

	err = th.App.RemoveUserFromChannel(userNotInChannel.Id, "", channel)
	require.Nil(t, err)
	replyPost := model.Post{
		Message:       "asd",
		ChannelId:     channel.Id,
		RootId:        rootPost.Id,
		ParentId:      rootPost.Id,
		PendingPostId: model.NewId() + ":" + fmt.Sprint(model.GetMillis()),
		UserId:        userInChannel.Id,
		CreateAt:      0,
	}

	_, err = th.App.CreatePostAsUser(&replyPost, "")
	require.Nil(t, err)
}

func TestPostAttachPostToChildPost(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	channel := th.BasicChannel
	user := th.BasicUser
	rootPost := th.BasicPost

	replyPost1 := model.Post{
		Message:       "reply one",
		ChannelId:     channel.Id,
		RootId:        rootPost.Id,
		ParentId:      rootPost.Id,
		PendingPostId: model.NewId() + ":" + fmt.Sprint(model.GetMillis()),
		UserId:        user.Id,
		CreateAt:      0,
	}

	res1, err := th.App.CreatePostAsUser(&replyPost1, "")
	require.Nil(t, err)

	replyPost2 := model.Post{
		Message:       "reply two",
		ChannelId:     channel.Id,
		RootId:        res1.Id,
		ParentId:      res1.Id,
		PendingPostId: model.NewId() + ":" + fmt.Sprint(model.GetMillis()),
		UserId:        user.Id,
		CreateAt:      0,
	}

	_, err = th.App.CreatePostAsUser(&replyPost2, "")
	assert.Equalf(t, err.StatusCode, http.StatusBadRequest, "Expected BadRequest error, got %v", err)

	replyPost3 := model.Post{
		Message:       "reply three",
		ChannelId:     channel.Id,
		RootId:        rootPost.Id,
		ParentId:      rootPost.Id,
		PendingPostId: model.NewId() + ":" + fmt.Sprint(model.GetMillis()),
		UserId:        user.Id,
		CreateAt:      0,
	}

	_, err = th.App.CreatePostAsUser(&replyPost3, "")
	assert.Nil(t, err)
}

func TestPostChannelMentions(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	channel := th.BasicChannel
	user := th.BasicUser

	channelToMention, err := th.App.CreateChannel(&model.Channel{
		DisplayName: "Mention Test",
		Name:        "mention-test",
		Type:        model.CHANNEL_OPEN,
		TeamId:      th.BasicTeam.Id,
	}, false)
	require.Nil(t, err)
	defer th.App.PermanentDeleteChannel(channelToMention)

	_, err = th.App.AddUserToChannel(user, channel)
	require.Nil(t, err)

	post := &model.Post{
		Message:       fmt.Sprintf("hello, ~%v!", channelToMention.Name),
		ChannelId:     channel.Id,
		PendingPostId: model.NewId() + ":" + fmt.Sprint(model.GetMillis()),
		UserId:        user.Id,
		CreateAt:      0,
	}

	result, err := th.App.CreatePostAsUser(post, "")
	require.Nil(t, err)
	assert.Equal(t, map[string]interface{}{
		"mention-test": map[string]interface{}{
			"display_name": "Mention Test",
		},
	}, result.Props["channel_mentions"])

	post.Message = fmt.Sprintf("goodbye, ~%v!", channelToMention.Name)
	result, err = th.App.UpdatePost(post, false)
	require.Nil(t, err)
	assert.Equal(t, map[string]interface{}{
		"mention-test": map[string]interface{}{
			"display_name": "Mention Test",
		},
	}, result.Props["channel_mentions"])
}

func TestImageProxy(t *testing.T) {
	th := SetupWithStoreMock(t)
	defer th.TearDown()

	mockStore := th.App.Srv().Store.(*storemocks.Store)
	mockUserStore := storemocks.UserStore{}
	mockUserStore.On("Count", mock.Anything).Return(int64(10), nil)
	mockPostStore := storemocks.PostStore{}
	mockPostStore.On("GetMaxPostSize").Return(65535, nil)
	mockSystemStore := storemocks.SystemStore{}
	mockSystemStore.On("GetByName", "InstallationDate").Return(&model.System{Name: "InstallationDate", Value: "10"}, nil)
	mockStore.On("User").Return(&mockUserStore)
	mockStore.On("Post").Return(&mockPostStore)
	mockStore.On("System").Return(&mockSystemStore)

	th.App.UpdateConfig(func(cfg *model.Config) {
		*cfg.ServiceSettings.SiteURL = "http://mymattermost.com"
	})

	for name, tc := range map[string]struct {
		ProxyType       string
		ProxyURL        string
		ProxyOptions    string
		ImageURL        string
		ProxiedImageURL string
	}{
		"atmos/camo": {
			ProxyType:       model.IMAGE_PROXY_TYPE_ATMOS_CAMO,
			ProxyURL:        "https://127.0.0.1",
			ProxyOptions:    "foo",
			ImageURL:        "http://mydomain.com/myimage",
			ProxiedImageURL: "http://mymattermost.com/api/v4/image?url=http%3A%2F%2Fmydomain.com%2Fmyimage",
		},
		"atmos/camo_SameSite": {
			ProxyType:       model.IMAGE_PROXY_TYPE_ATMOS_CAMO,
			ProxyURL:        "https://127.0.0.1",
			ProxyOptions:    "foo",
			ImageURL:        "http://mymattermost.com/myimage",
			ProxiedImageURL: "http://mymattermost.com/myimage",
		},
		"atmos/camo_PathOnly": {
			ProxyType:       model.IMAGE_PROXY_TYPE_ATMOS_CAMO,
			ProxyURL:        "https://127.0.0.1",
			ProxyOptions:    "foo",
			ImageURL:        "/myimage",
			ProxiedImageURL: "/myimage",
		},
		"atmos/camo_EmptyImageURL": {
			ProxyType:       model.IMAGE_PROXY_TYPE_ATMOS_CAMO,
			ProxyURL:        "https://127.0.0.1",
			ProxyOptions:    "foo",
			ImageURL:        "",
			ProxiedImageURL: "",
		},
		"local": {
			ProxyType:       model.IMAGE_PROXY_TYPE_LOCAL,
			ImageURL:        "http://mydomain.com/myimage",
			ProxiedImageURL: "http://mymattermost.com/api/v4/image?url=http%3A%2F%2Fmydomain.com%2Fmyimage",
		},
		"local_SameSite": {
			ProxyType:       model.IMAGE_PROXY_TYPE_LOCAL,
			ImageURL:        "http://mymattermost.com/myimage",
			ProxiedImageURL: "http://mymattermost.com/myimage",
		},
		"local_PathOnly": {
			ProxyType:       model.IMAGE_PROXY_TYPE_LOCAL,
			ImageURL:        "/myimage",
			ProxiedImageURL: "/myimage",
		},
		"local_EmptyImageURL": {
			ProxyType:       model.IMAGE_PROXY_TYPE_LOCAL,
			ImageURL:        "",
			ProxiedImageURL: "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			th.App.UpdateConfig(func(cfg *model.Config) {
				cfg.ImageProxySettings.Enable = model.NewBool(true)
				cfg.ImageProxySettings.ImageProxyType = model.NewString(tc.ProxyType)
				cfg.ImageProxySettings.RemoteImageProxyOptions = model.NewString(tc.ProxyOptions)
				cfg.ImageProxySettings.RemoteImageProxyURL = model.NewString(tc.ProxyURL)
			})

			post := &model.Post{
				Id:      model.NewId(),
				Message: "![foo](" + tc.ImageURL + ")",
			}

			list := model.NewPostList()
			list.Posts[post.Id] = post

			assert.Equal(t, "![foo]("+tc.ProxiedImageURL+")", th.App.PostWithProxyAddedToImageURLs(post).Message)

			assert.Equal(t, "![foo]("+tc.ImageURL+")", th.App.PostWithProxyRemovedFromImageURLs(post).Message)
			post.Message = "![foo](" + tc.ProxiedImageURL + ")"
			assert.Equal(t, "![foo]("+tc.ImageURL+")", th.App.PostWithProxyRemovedFromImageURLs(post).Message)

			if tc.ImageURL != "" {
				post.Message = "![foo](" + tc.ImageURL + " =500x200)"
				assert.Equal(t, "![foo]("+tc.ProxiedImageURL+" =500x200)", th.App.PostWithProxyAddedToImageURLs(post).Message)
				assert.Equal(t, "![foo]("+tc.ImageURL+" =500x200)", th.App.PostWithProxyRemovedFromImageURLs(post).Message)
				post.Message = "![foo](" + tc.ProxiedImageURL + " =500x200)"
				assert.Equal(t, "![foo]("+tc.ImageURL+" =500x200)", th.App.PostWithProxyRemovedFromImageURLs(post).Message)
			}
		})
	}
}

func TestMaxPostSize(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Description         string
		StoreMaxPostSize    int
		ExpectedMaxPostSize int
	}{
		{
			"Max post size less than model.model.POST_MESSAGE_MAX_RUNES_V1 ",
			0,
			model.POST_MESSAGE_MAX_RUNES_V1,
		},
		{
			"4000 rune limit",
			4000,
			4000,
		},
		{
			"16383 rune limit",
			16383,
			16383,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.Description, func(t *testing.T) {
			t.Parallel()

			mockStore := &storetest.Store{}
			defer mockStore.AssertExpectations(t)

			mockStore.PostStore.On("GetMaxPostSize").Return(testCase.StoreMaxPostSize)

			app := App{
				srv: &Server{
					Store: mockStore,
				},
			}

			assert.Equal(t, testCase.ExpectedMaxPostSize, app.MaxPostSize())
		})
	}
}

func TestDeletePostWithFileAttachments(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	// Create a post with a file attachment.
	teamId := th.BasicTeam.Id
	channelId := th.BasicChannel.Id
	userId := th.BasicUser.Id
	filename := "test"
	data := []byte("abcd")

	info1, err := th.App.DoUploadFile(time.Date(2007, 2, 4, 1, 2, 3, 4, time.Local), teamId, channelId, userId, filename, data)
	require.Nil(t, err)
	defer func() {
		th.App.Srv().Store.FileInfo().PermanentDelete(info1.Id)
		th.App.RemoveFile(info1.Path)
	}()

	post := &model.Post{
		Message:       "asd",
		ChannelId:     channelId,
		PendingPostId: model.NewId() + ":" + fmt.Sprint(model.GetMillis()),
		UserId:        userId,
		CreateAt:      0,
		FileIds:       []string{info1.Id},
	}

	post, err = th.App.CreatePost(post, th.BasicChannel, false)
	assert.Nil(t, err)

	// Delete the post.
	post, err = th.App.DeletePost(post.Id, userId)
	assert.Nil(t, err)

	// Wait for the cleanup routine to finish.
	time.Sleep(time.Millisecond * 100)

	// Check that the file can no longer be reached.
	_, err = th.App.GetFileInfo(info1.Id)
	assert.NotNil(t, err)
}

func TestDeletePostInArchivedChannel(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	archivedChannel := th.CreateChannel(th.BasicTeam)
	post := th.CreatePost(archivedChannel)
	th.App.DeleteChannel(archivedChannel, "")

	_, err := th.App.DeletePost(post.Id, "")
	require.NotNil(t, err)
	require.Equal(t, "api.post.delete_post.can_not_delete_post_in_deleted.error", err.Id)
}

func TestCreatePost(t *testing.T) {
	t.Run("call PreparePostForClient before returning", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		th.App.UpdateConfig(func(cfg *model.Config) {
			*cfg.ServiceSettings.SiteURL = "http://mymattermost.com"
			*cfg.ImageProxySettings.Enable = true
			*cfg.ImageProxySettings.ImageProxyType = "atmos/camo"
			*cfg.ImageProxySettings.RemoteImageProxyURL = "https://127.0.0.1"
			*cfg.ImageProxySettings.RemoteImageProxyOptions = "foo"
		})

		imageURL := "http://mydomain.com/myimage"
		proxiedImageURL := "http://mymattermost.com/api/v4/image?url=http%3A%2F%2Fmydomain.com%2Fmyimage"

		post := &model.Post{
			ChannelId: th.BasicChannel.Id,
			Message:   "![image](" + imageURL + ")",
			UserId:    th.BasicUser.Id,
		}

		rpost, err := th.App.CreatePost(post, th.BasicChannel, false)
		require.Nil(t, err)
		assert.Equal(t, "![image]("+proxiedImageURL+")", rpost.Message)
	})

	t.Run("Sets prop MENTION_HIGHLIGHT_DISABLED when it should", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()
		th.AddUserToChannel(th.BasicUser, th.BasicChannel)

		t.Run("Does not set prop when user has USE_CHANNEL_MENTIONS", func(t *testing.T) {
			postWithNoMention := &model.Post{
				ChannelId: th.BasicChannel.Id,
				Message:   "This post does not have mentions",
				UserId:    th.BasicUser.Id,
			}
			rpost, err := th.App.CreatePost(postWithNoMention, th.BasicChannel, false)
			require.Nil(t, err)
			assert.Equal(t, rpost.Props, model.StringInterface{})

			postWithMention := &model.Post{
				ChannelId: th.BasicChannel.Id,
				Message:   "This post has @here mention @all",
				UserId:    th.BasicUser.Id,
			}
			rpost, err = th.App.CreatePost(postWithMention, th.BasicChannel, false)
			require.Nil(t, err)
			assert.Equal(t, rpost.Props, model.StringInterface{})
		})

		t.Run("Sets prop when post has mentions and user does not have USE_CHANNEL_MENTIONS", func(t *testing.T) {
			th.RemovePermissionFromRole(model.PERMISSION_USE_CHANNEL_MENTIONS.Id, model.CHANNEL_USER_ROLE_ID)

			postWithNoMention := &model.Post{
				ChannelId: th.BasicChannel.Id,
				Message:   "This post does not have mentions",
				UserId:    th.BasicUser.Id,
			}
			rpost, err := th.App.CreatePost(postWithNoMention, th.BasicChannel, false)
			require.Nil(t, err)
			assert.Equal(t, rpost.Props, model.StringInterface{})

			postWithMention := &model.Post{
				ChannelId: th.BasicChannel.Id,
				Message:   "This post has @here mention @all",
				UserId:    th.BasicUser.Id,
			}
			rpost, err = th.App.CreatePost(postWithMention, th.BasicChannel, false)
			require.Nil(t, err)
			assert.Equal(t, rpost.Props[model.POST_PROPS_MENTION_HIGHLIGHT_DISABLED], true)

			th.AddPermissionToRole(model.PERMISSION_USE_CHANNEL_MENTIONS.Id, model.CHANNEL_USER_ROLE_ID)
		})
	})
}

func TestPatchPost(t *testing.T) {
	t.Run("call PreparePostForClient before returning", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		th.App.UpdateConfig(func(cfg *model.Config) {
			*cfg.ServiceSettings.SiteURL = "http://mymattermost.com"
			*cfg.ImageProxySettings.Enable = true
			*cfg.ImageProxySettings.ImageProxyType = "atmos/camo"
			*cfg.ImageProxySettings.RemoteImageProxyURL = "https://127.0.0.1"
			*cfg.ImageProxySettings.RemoteImageProxyOptions = "foo"
		})

		imageURL := "http://mydomain.com/myimage"
		proxiedImageURL := "http://mymattermost.com/api/v4/image?url=http%3A%2F%2Fmydomain.com%2Fmyimage"

		post := &model.Post{
			ChannelId: th.BasicChannel.Id,
			Message:   "![image](http://mydomain/anotherimage)",
			UserId:    th.BasicUser.Id,
		}

		rpost, err := th.App.CreatePost(post, th.BasicChannel, false)
		require.Nil(t, err)
		assert.NotEqual(t, "![image]("+proxiedImageURL+")", rpost.Message)

		patch := &model.PostPatch{
			Message: model.NewString("![image](" + imageURL + ")"),
		}

		rpost, err = th.App.PatchPost(rpost.Id, patch)
		require.Nil(t, err)
		assert.Equal(t, "![image]("+proxiedImageURL+")", rpost.Message)
	})

	t.Run("Sets Prop MENTION_HIGHLIGHT_DISABLED when it should", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		th.AddUserToChannel(th.BasicUser, th.BasicChannel)

		post := &model.Post{
			ChannelId: th.BasicChannel.Id,
			Message:   "This post does not have mentions",
			UserId:    th.BasicUser.Id,
		}

		rpost, err := th.App.CreatePost(post, th.BasicChannel, false)
		require.Nil(t, err)

		t.Run("Does not set prop when user has USE_CHANNEL_MENTIONS", func(t *testing.T) {
			patchWithNoMention := &model.PostPatch{Message: model.NewString("This patch has no channel mention")}

			rpost, err = th.App.PatchPost(rpost.Id, patchWithNoMention)
			require.Nil(t, err)
			assert.Equal(t, rpost.Props, model.StringInterface{})

			patchWithMention := &model.PostPatch{Message: model.NewString("This patch has a mention now @here")}

			rpost, err = th.App.PatchPost(rpost.Id, patchWithMention)
			require.Nil(t, err)
			assert.Equal(t, rpost.Props, model.StringInterface{})
		})

		t.Run("Sets prop when user does not have USE_CHANNEL_MENTIONS", func(t *testing.T) {
			th.RemovePermissionFromRole(model.PERMISSION_USE_CHANNEL_MENTIONS.Id, model.CHANNEL_USER_ROLE_ID)

			patchWithNoMention := &model.PostPatch{Message: model.NewString("This patch still does not have a mention")}
			rpost, err = th.App.PatchPost(rpost.Id, patchWithNoMention)
			require.Nil(t, err)
			assert.Equal(t, rpost.Props, model.StringInterface{})

			patchWithMention := &model.PostPatch{Message: model.NewString("This patch has a mention now @here")}

			rpost, err = th.App.PatchPost(rpost.Id, patchWithMention)
			require.Nil(t, err)
			assert.Equal(t, rpost.Props[model.POST_PROPS_MENTION_HIGHLIGHT_DISABLED], true)

			th.AddPermissionToRole(model.PERMISSION_USE_CHANNEL_MENTIONS.Id, model.CHANNEL_USER_ROLE_ID)
		})
	})
}

func TestPatchPostInArchivedChannel(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	archivedChannel := th.CreateChannel(th.BasicTeam)
	post := th.CreatePost(archivedChannel)
	th.App.DeleteChannel(archivedChannel, "")

	_, err := th.App.PatchPost(post.Id, &model.PostPatch{IsPinned: model.NewBool(true)})
	require.NotNil(t, err)
	require.Equal(t, "api.post.patch_post.can_not_update_post_in_deleted.error", err.Id)
}

func TestUpdatePost(t *testing.T) {
	t.Run("call PreparePostForClient before returning", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		th.App.UpdateConfig(func(cfg *model.Config) {
			*cfg.ServiceSettings.SiteURL = "http://mymattermost.com"
			*cfg.ImageProxySettings.Enable = true
			*cfg.ImageProxySettings.ImageProxyType = "atmos/camo"
			*cfg.ImageProxySettings.RemoteImageProxyURL = "https://127.0.0.1"
			*cfg.ImageProxySettings.RemoteImageProxyOptions = "foo"
		})

		imageURL := "http://mydomain.com/myimage"
		proxiedImageURL := "http://mymattermost.com/api/v4/image?url=http%3A%2F%2Fmydomain.com%2Fmyimage"

		post := &model.Post{
			ChannelId: th.BasicChannel.Id,
			Message:   "![image](http://mydomain/anotherimage)",
			UserId:    th.BasicUser.Id,
		}

		rpost, err := th.App.CreatePost(post, th.BasicChannel, false)
		require.Nil(t, err)
		assert.NotEqual(t, "![image]("+proxiedImageURL+")", rpost.Message)

		post.Id = rpost.Id
		post.Message = "![image](" + imageURL + ")"

		rpost, err = th.App.UpdatePost(post, false)
		require.Nil(t, err)
		assert.Equal(t, "![image]("+proxiedImageURL+")", rpost.Message)
	})
}

func TestSearchPostsInTeamForUser(t *testing.T) {
	perPage := 5
	searchTerm := "searchTerm"

	setup := func(t *testing.T, enableElasticsearch bool) (*TestHelper, []*model.Post) {
		th := Setup(t).InitBasic()

		posts := make([]*model.Post, 7)
		for i := 0; i < cap(posts); i++ {
			post, err := th.App.CreatePost(&model.Post{
				UserId:    th.BasicUser.Id,
				ChannelId: th.BasicChannel.Id,
				Message:   searchTerm,
			}, th.BasicChannel, false)

			require.Nil(t, err)

			posts[i] = post
		}

		if enableElasticsearch {
			th.App.SetLicense(model.NewTestLicense("elastic_search"))

			th.App.UpdateConfig(func(cfg *model.Config) {
				*cfg.ElasticsearchSettings.EnableIndexing = true
				*cfg.ElasticsearchSettings.EnableSearching = true
			})
		} else {
			th.App.UpdateConfig(func(cfg *model.Config) {
				*cfg.ElasticsearchSettings.EnableSearching = false
			})
		}

		return th, posts
	}

	t.Run("should return everything as first page of posts from database", func(t *testing.T) {
		th, posts := setup(t, false)
		defer th.TearDown()

		page := 0

		results, err := th.App.SearchPostsInTeamForUser(searchTerm, th.BasicUser.Id, th.BasicTeam.Id, false, false, 0, page, perPage)

		assert.Nil(t, err)
		assert.Equal(t, []string{
			posts[6].Id,
			posts[5].Id,
			posts[4].Id,
			posts[3].Id,
			posts[2].Id,
			posts[1].Id,
			posts[0].Id,
		}, results.Order)
	})

	t.Run("should not return later pages of posts from database", func(t *testing.T) {
		th, _ := setup(t, false)
		defer th.TearDown()

		page := 1

		results, err := th.App.SearchPostsInTeamForUser(searchTerm, th.BasicUser.Id, th.BasicTeam.Id, false, false, 0, page, perPage)

		assert.Nil(t, err)
		assert.Equal(t, []string{}, results.Order)
	})

	t.Run("should return first page of posts from ElasticSearch", func(t *testing.T) {
		th, posts := setup(t, true)
		defer th.TearDown()

		page := 0
		resultsPage := []string{
			posts[6].Id,
			posts[5].Id,
			posts[4].Id,
			posts[3].Id,
			posts[2].Id,
		}

		es := &mocks.ElasticsearchInterface{}
		es.On("SearchPosts", mock.Anything, mock.Anything, page, perPage).Return(resultsPage, nil, nil)
		th.App.elasticsearch = es

		results, err := th.App.SearchPostsInTeamForUser(searchTerm, th.BasicUser.Id, th.BasicTeam.Id, false, false, 0, page, perPage)

		assert.Nil(t, err)
		assert.Equal(t, resultsPage, results.Order)
		es.AssertExpectations(t)
	})

	t.Run("should return later pages of posts from ElasticSearch", func(t *testing.T) {
		th, posts := setup(t, true)
		defer th.TearDown()

		page := 1
		resultsPage := []string{
			posts[1].Id,
			posts[0].Id,
		}

		es := &mocks.ElasticsearchInterface{}
		es.On("SearchPosts", mock.Anything, mock.Anything, page, perPage).Return(resultsPage, nil, nil)
		th.App.elasticsearch = es

		results, err := th.App.SearchPostsInTeamForUser(searchTerm, th.BasicUser.Id, th.BasicTeam.Id, false, false, 0, page, perPage)

		assert.Nil(t, err)
		assert.Equal(t, resultsPage, results.Order)
		es.AssertExpectations(t)
	})

	t.Run("should fall back to database if ElasticSearch fails on first page", func(t *testing.T) {
		th, posts := setup(t, true)
		defer th.TearDown()

		page := 0

		es := &mocks.ElasticsearchInterface{}
		es.On("SearchPosts", mock.Anything, mock.Anything, page, perPage).Return(nil, nil, &model.AppError{})
		th.App.elasticsearch = es

		results, err := th.App.SearchPostsInTeamForUser(searchTerm, th.BasicUser.Id, th.BasicTeam.Id, false, false, 0, page, perPage)

		assert.Nil(t, err)
		assert.Equal(t, []string{
			posts[6].Id,
			posts[5].Id,
			posts[4].Id,
			posts[3].Id,
			posts[2].Id,
			posts[1].Id,
			posts[0].Id,
		}, results.Order)
		es.AssertExpectations(t)
	})

	t.Run("should return nothing if ElasticSearch fails on later pages", func(t *testing.T) {
		th, _ := setup(t, true)
		defer th.TearDown()

		page := 1

		es := &mocks.ElasticsearchInterface{}
		es.On("SearchPosts", mock.Anything, mock.Anything, page, perPage).Return(nil, nil, &model.AppError{})
		th.App.elasticsearch = es

		results, err := th.App.SearchPostsInTeamForUser(searchTerm, th.BasicUser.Id, th.BasicTeam.Id, false, false, 0, page, perPage)

		assert.Nil(t, err)
		assert.Equal(t, []string{}, results.Order)
		es.AssertExpectations(t)
	})
}

func TestCountMentionsFromPost(t *testing.T) {
	t.Run("should not count posts without mentions", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		user1 := th.BasicUser
		user2 := th.BasicUser2

		channel := th.CreateChannel(th.BasicTeam)
		th.AddUserToChannel(user2, channel)

		post1, err := th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "test",
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "test2",
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "test3",
		}, channel, false)
		require.Nil(t, err)

		count, err := th.App.countMentionsFromPost(user2, post1)

		assert.Nil(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("should count keyword mentions", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		user1 := th.BasicUser
		user2 := th.BasicUser2

		channel := th.CreateChannel(th.BasicTeam)
		th.AddUserToChannel(user2, channel)

		user2.NotifyProps[model.MENTION_KEYS_NOTIFY_PROP] = "apple"

		post1, err := th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   fmt.Sprintf("@%s", user2.Username),
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "test2",
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "apple",
		}, channel, false)
		require.Nil(t, err)

		// post1 and post3 should mention the user

		count, err := th.App.countMentionsFromPost(user2, post1)

		assert.Nil(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("should count channel-wide mentions when enabled", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		user1 := th.BasicUser
		user2 := th.BasicUser2

		channel := th.CreateChannel(th.BasicTeam)
		th.AddUserToChannel(user2, channel)

		user2.NotifyProps[model.CHANNEL_MENTIONS_NOTIFY_PROP] = "true"

		post1, err := th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "test",
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "@channel",
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "@all",
		}, channel, false)
		require.Nil(t, err)

		// post2 and post3 should mention the user

		count, err := th.App.countMentionsFromPost(user2, post1)

		assert.Nil(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("should not count channel-wide mentions when disabled for user", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		user1 := th.BasicUser
		user2 := th.BasicUser2

		channel := th.CreateChannel(th.BasicTeam)
		th.AddUserToChannel(user2, channel)

		user2.NotifyProps[model.CHANNEL_MENTIONS_NOTIFY_PROP] = "false"

		post1, err := th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "test",
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "@channel",
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "@all",
		}, channel, false)
		require.Nil(t, err)

		count, err := th.App.countMentionsFromPost(user2, post1)

		assert.Nil(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("should not count channel-wide mentions when disabled for channel", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		user1 := th.BasicUser
		user2 := th.BasicUser2

		channel := th.CreateChannel(th.BasicTeam)
		th.AddUserToChannel(user2, channel)

		user2.NotifyProps[model.CHANNEL_MENTIONS_NOTIFY_PROP] = "true"

		_, err := th.App.UpdateChannelMemberNotifyProps(map[string]string{
			model.IGNORE_CHANNEL_MENTIONS_NOTIFY_PROP: model.IGNORE_CHANNEL_MENTIONS_ON,
		}, channel.Id, user2.Id)
		require.Nil(t, err)

		post1, err := th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "test",
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "@channel",
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "@all",
		}, channel, false)
		require.Nil(t, err)

		count, err := th.App.countMentionsFromPost(user2, post1)

		assert.Nil(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("should count comment mentions when using COMMENTS_NOTIFY_ROOT", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		user1 := th.BasicUser
		user2 := th.BasicUser2

		channel := th.CreateChannel(th.BasicTeam)
		th.AddUserToChannel(user2, channel)

		user2.NotifyProps[model.COMMENTS_NOTIFY_PROP] = model.COMMENTS_NOTIFY_ROOT

		post1, err := th.App.CreatePost(&model.Post{
			UserId:    user2.Id,
			ChannelId: channel.Id,
			Message:   "test",
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			RootId:    post1.Id,
			Message:   "test2",
		}, channel, false)
		require.Nil(t, err)
		post3, err := th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "test3",
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user2.Id,
			ChannelId: channel.Id,
			RootId:    post3.Id,
			Message:   "test4",
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			RootId:    post3.Id,
			Message:   "test5",
		}, channel, false)
		require.Nil(t, err)

		// post2 should mention the user

		count, err := th.App.countMentionsFromPost(user2, post1)

		assert.Nil(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("should count comment mentions when using COMMENTS_NOTIFY_ANY", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		user1 := th.BasicUser
		user2 := th.BasicUser2

		channel := th.CreateChannel(th.BasicTeam)
		th.AddUserToChannel(user2, channel)

		user2.NotifyProps[model.COMMENTS_NOTIFY_PROP] = model.COMMENTS_NOTIFY_ANY

		post1, err := th.App.CreatePost(&model.Post{
			UserId:    user2.Id,
			ChannelId: channel.Id,
			Message:   "test",
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			RootId:    post1.Id,
			Message:   "test2",
		}, channel, false)
		require.Nil(t, err)
		post3, err := th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "test3",
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user2.Id,
			ChannelId: channel.Id,
			RootId:    post3.Id,
			Message:   "test4",
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			RootId:    post3.Id,
			Message:   "test5",
		}, channel, false)
		require.Nil(t, err)

		// post2 and post5 should mention the user

		count, err := th.App.countMentionsFromPost(user2, post1)

		assert.Nil(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("should count mentions caused by being added to the channel", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		user1 := th.BasicUser
		user2 := th.BasicUser2

		channel := th.CreateChannel(th.BasicTeam)
		th.AddUserToChannel(user2, channel)

		post1, err := th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "test",
			Type:      model.POST_ADD_TO_CHANNEL,
			Props: map[string]interface{}{
				model.POST_PROPS_ADDED_USER_ID: model.NewId(),
			},
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "test2",
			Type:      model.POST_ADD_TO_CHANNEL,
			Props: map[string]interface{}{
				model.POST_PROPS_ADDED_USER_ID: user2.Id,
			},
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "test3",
			Type:      model.POST_ADD_TO_CHANNEL,
			Props: map[string]interface{}{
				model.POST_PROPS_ADDED_USER_ID: user2.Id,
			},
		}, channel, false)
		require.Nil(t, err)

		// should be mentioned by post2 and post3

		count, err := th.App.countMentionsFromPost(user2, post1)

		assert.Nil(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("should return the number of posts made by the other user for a direct channel", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		user1 := th.BasicUser
		user2 := th.BasicUser2

		channel, err := th.App.createDirectChannel(user1.Id, user2.Id)
		require.Nil(t, err)

		post1, err := th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "test",
		}, channel, false)
		require.Nil(t, err)

		_, err = th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "test2",
		}, channel, false)
		require.Nil(t, err)

		count, err := th.App.countMentionsFromPost(user2, post1)

		assert.Nil(t, err)
		assert.Equal(t, 2, count)

		count, err = th.App.countMentionsFromPost(user1, post1)

		assert.Nil(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("should not count mentions from the before the given post", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		user1 := th.BasicUser
		user2 := th.BasicUser2

		channel := th.CreateChannel(th.BasicTeam)
		th.AddUserToChannel(user2, channel)

		_, err := th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   fmt.Sprintf("@%s", user2.Username),
		}, channel, false)
		require.Nil(t, err)
		post2, err := th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "test2",
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   fmt.Sprintf("@%s", user2.Username),
		}, channel, false)
		require.Nil(t, err)

		// post1 and post3 should mention the user, but we only count post3

		count, err := th.App.countMentionsFromPost(user2, post2)

		assert.Nil(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("should not count mentions from the user's own posts", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		user1 := th.BasicUser
		user2 := th.BasicUser2

		channel := th.CreateChannel(th.BasicTeam)
		th.AddUserToChannel(user2, channel)

		post1, err := th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   fmt.Sprintf("@%s", user2.Username),
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user2.Id,
			ChannelId: channel.Id,
			Message:   fmt.Sprintf("@%s", user2.Username),
		}, channel, false)
		require.Nil(t, err)

		// post2 should mention the user

		count, err := th.App.countMentionsFromPost(user2, post1)

		assert.Nil(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("should include comments made before the given post when counting comment mentions", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		user1 := th.BasicUser
		user2 := th.BasicUser2

		channel := th.CreateChannel(th.BasicTeam)
		th.AddUserToChannel(user2, channel)

		user2.NotifyProps[model.COMMENTS_NOTIFY_PROP] = model.COMMENTS_NOTIFY_ANY

		post1, err := th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "test1",
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user2.Id,
			ChannelId: channel.Id,
			RootId:    post1.Id,
			Message:   "test2",
		}, channel, false)
		require.Nil(t, err)
		post3, err := th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "test3",
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			RootId:    post1.Id,
			Message:   "test4",
		}, channel, false)
		require.Nil(t, err)

		// post4 should mention the user

		count, err := th.App.countMentionsFromPost(user2, post3)

		assert.Nil(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("should count mentions from the user's webhook posts", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		user1 := th.BasicUser
		user2 := th.BasicUser2

		channel := th.CreateChannel(th.BasicTeam)
		th.AddUserToChannel(user2, channel)

		post1, err := th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   "test1",
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user2.Id,
			ChannelId: channel.Id,
			Message:   fmt.Sprintf("@%s", user2.Username),
		}, channel, false)
		require.Nil(t, err)
		_, err = th.App.CreatePost(&model.Post{
			UserId:    user2.Id,
			ChannelId: channel.Id,
			Message:   fmt.Sprintf("@%s", user2.Username),
			Props: map[string]interface{}{
				"from_webhook": "true",
			},
		}, channel, false)
		require.Nil(t, err)

		// post3 should mention the user

		count, err := th.App.countMentionsFromPost(user2, post1)

		assert.Nil(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("should count multiple pages of mentions", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		user1 := th.BasicUser
		user2 := th.BasicUser2

		channel := th.CreateChannel(th.BasicTeam)
		th.AddUserToChannel(user2, channel)

		numPosts := 215

		post1, err := th.App.CreatePost(&model.Post{
			UserId:    user1.Id,
			ChannelId: channel.Id,
			Message:   fmt.Sprintf("@%s", user2.Username),
		}, channel, false)
		require.Nil(t, err)

		for i := 0; i < numPosts-1; i++ {
			_, err = th.App.CreatePost(&model.Post{
				UserId:    user1.Id,
				ChannelId: channel.Id,
				Message:   fmt.Sprintf("@%s", user2.Username),
			}, channel, false)
			require.Nil(t, err)
		}

		// Every post should mention the user

		count, err := th.App.countMentionsFromPost(user2, post1)

		assert.Nil(t, err)
		assert.Equal(t, numPosts, count)
	})
}

type ChannelList []*model.Channel

func (a ChannelList) Len() int           { return len(a) }
func (a ChannelList) Less(i, j int) bool { return a[i].Id < a[j].Id }
func (a ChannelList) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

// Create some channels and posts for incremental and recent posts tests
func populateChannels(
	th *TestHelper,
	channelsCount int,
	postsPerChannel int,
	usersCount int,
) (*[]*model.Channel, *[]*model.User, *map[string][]string) {
	team := th.CreateTeam()

	users := make([]*model.User, usersCount)[:0]
	for i := 0; i < usersCount; i++ {
		u := th.CreateUser()
		users = append(users, u)
		th.LinkUserToTeam(u, team)
	}

	channels := make([]*model.Channel, channelsCount)
	posts := make(map[string][]string, channelsCount)
	for i := 0; i < channelsCount; i++ {
		c := th.CreateChannel(team)
		channels[i] = c
		for _, u := range users {
			th.AddUserToChannel(u, c)
		}

		list := make([]string, postsPerChannel)
		posts[c.Id] = list
		for j := 0; j < postsPerChannel; j++ {
			p := th.CreatePost(c)
			list[j] = p.Id
		}
	}

	var l ChannelList = channels
	sort.Sort(l)

	return &channels, &users, &posts
}

func TestGetRecentPosts(t *testing.T) {
	// regular case
	t.Run("get recent posts", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		channels, _, posts := populateChannels(th, 3, 30, 2)
		receivedPosts, err := th.App.GetRecentPosts(&model.RecentPostsRequestData{
			ChannelIds: []string{
				(*channels)[0].Id,
				(*channels)[1].Id,
				(*channels)[2].Id,
			},
			MaxTotalMessages:   100,
			MessagesPerChannel: 30,
		})
		assert.Nil(t, err)
		assert.Equal(t, 90, len(*receivedPosts), "wrong number of posts")

		var expected []string
		for i, v := range *receivedPosts {
			expected = (*posts)[v.ChannelId]
			assert.Equal(t, expected[i%30], v.Id, "post ids mismatch")
		}
	})

	t.Run("get recent posts for channels with more messages", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		channels, _, posts := populateChannels(th, 3, 50, 2)
		receivedPosts, err := th.App.GetRecentPosts(&model.RecentPostsRequestData{
			ChannelIds: []string{
				(*channels)[0].Id,
				(*channels)[1].Id,
				(*channels)[2].Id,
			},
			MaxTotalMessages:   80,
			MessagesPerChannel: 30,
		})
		assert.Nil(t, err)
		assert.Equal(t, 60, len(*receivedPosts), "wrong number of posts")

		var expected []string
		for i, v := range *receivedPosts {
			expected = (*posts)[v.ChannelId]
			assert.Equal(t, expected[20+i%30], v.Id, "post ids mismatch")
		}
	})

	t.Run("get recent posts on a big number of channels with limit", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		channels, _, posts := populateChannels(th, 50, 20, 2)
		channelIds := make([]string, 50)
		for i := 0; i < 50; i++ {
			channelIds[i] = (*channels)[i].Id
		}
		receivedPosts, err := th.App.GetRecentPosts(&model.RecentPostsRequestData{
			ChannelIds:         channelIds,
			MaxTotalMessages:   200, // 37 channels should not fit the limit
			MessagesPerChannel: 15,  // 13 channels should fit the limit
		})
		assert.Nil(t, err)
		assert.Equal(t, 195, len(*receivedPosts), "wrong number of posts")

		receivedChannels := make(map[string]bool, 13)
		var expected []string
		for i, v := range *receivedPosts {
			receivedChannels[v.ChannelId] = true
			expected = (*posts)[v.ChannelId]
			assert.Equal(t, expected[5+i%15], v.Id, "post ids mismatch")
		}

		for i := 13; i < 50; i++ {
			channelId := (*channels)[i].Id
			_, exists := receivedChannels[channelId]
			assert.False(t, exists, "received unexpected channel")
		}
	})

	t.Run("get recent posts from channels that have less messages than requested max", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()
		channels, _, posts := populateChannels(th, 5, 20, 2)
		receivedPosts, err := th.App.GetRecentPosts(&model.RecentPostsRequestData{
			ChannelIds: []string{
				(*channels)[0].Id,
				(*channels)[1].Id,
				(*channels)[2].Id,
				(*channels)[3].Id,
				(*channels)[4].Id,
			},
			MaxTotalMessages:   100, // one channel should not fit
			MessagesPerChannel: 30,  // max messages is more than the channels have
		})
		assert.Nil(t, err)
		assert.Equal(t, 80, len(*receivedPosts), "wrong number of posts")

		var expected []string
		for i, v := range *receivedPosts {
			expected = (*posts)[v.ChannelId]
			assert.Equal(t, expected[i%20], v.Id, "post ids mismatch")
		}
	})

	t.Run("request too much per channel", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		channels, _, _ := populateChannels(th, 3, 50, 2)
		_, err := th.App.GetRecentPosts(&model.RecentPostsRequestData{
			ChannelIds: []string{
				(*channels)[0].Id,
				(*channels)[1].Id,
				(*channels)[2].Id,
			},
			MaxTotalMessages:   80,
			MessagesPerChannel: MAX_RECENT_PER_CHANNEL + 1, // above the allowed maximum, expect error
		})
		assert.NotNil(t, err)
		assert.Equal(t, "app.post.get_recent_posts.per_channel_too_big.app_error", err.Id, "wrong error id")
	})

	t.Run("request too much total messages", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		channels, _, _ := populateChannels(th, 3, 50, 2)
		_, err := th.App.GetRecentPosts(&model.RecentPostsRequestData{
			ChannelIds: []string{
				(*channels)[0].Id,
				(*channels)[1].Id,
				(*channels)[2].Id,
			},
			MaxTotalMessages:   MAX_RECENT_TOTAL + 1, // above the allowed maximum, expect error
			MessagesPerChannel: 30,
		})
		assert.NotNil(t, err)
		assert.Equal(t, "app.post.get_recent_posts.total_too_big.app_error", err.Id, "wrong error id")
	})
}

func TestCheckIncrementPossible(t *testing.T) {
	t.Run("client has no posts for channels", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		channels, _, _ := populateChannels(th, 4, 500, 2)
		allow, err := th.App.CheckIncrementPossible(&model.IncrementCheckRequest{
			Channels: []model.ChannelWithPost{
				{ChannelId: (*channels)[0].Id},
				{ChannelId: (*channels)[1].Id},
				{ChannelId: (*channels)[2].Id},
				{ChannelId: (*channels)[3].Id},
			},
		})
		assert.Nil(t, err)
		assert.False(t, allow, "negative result expected")
	})

	t.Run("client has some posts for channels but update is not possible", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		channels, _, posts := populateChannels(th, 4, 500, 2)
		// Arrange that the client has MAX_INCREMENT_TOTAL + 1 posts to download.
		// This should be considered too much for incremental update.
		allow, err := th.App.CheckIncrementPossible(&model.IncrementCheckRequest{
			Channels: []model.ChannelWithPost{
				{
					ChannelId: (*channels)[0].Id,
					PostId:    (*posts)[(*channels)[0].Id][99], // 400 posts after
				},
				{
					ChannelId: (*channels)[1].Id,
					PostId:    (*posts)[(*channels)[1].Id][99], // 400 posts after
				},
				{
					ChannelId: (*channels)[2].Id,
					PostId:    (*posts)[(*channels)[2].Id][99], // 400 posts after
				},
				{
					ChannelId: (*channels)[3].Id,
					PostId:    (*posts)[(*channels)[3].Id][198], // 301 posts after for a total of 1501
				},
			},
		})
		assert.Nil(t, err)
		assert.False(t, allow, "negative result expected")
	})

	t.Run("client has some posts for channels and update is possible", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		channels, _, posts := populateChannels(th, 4, 500, 2)
		// Arrange that the client has MAX_INCREMENT_TOTAL posts to download.
		// This should be considered Ok for incremental update.
		allow, err := th.App.CheckIncrementPossible(&model.IncrementCheckRequest{
			Channels: []model.ChannelWithPost{
				{
					ChannelId: (*channels)[0].Id,
					PostId:    (*posts)[(*channels)[0].Id][99], // 400 posts after
				},
				{
					ChannelId: (*channels)[1].Id,
					PostId:    (*posts)[(*channels)[1].Id][99], // 400 posts after
				},
				{
					ChannelId: (*channels)[2].Id,
					PostId:    (*posts)[(*channels)[2].Id][99], // 400 posts after
				},
				{
					ChannelId: (*channels)[3].Id,
					PostId:    (*posts)[(*channels)[3].Id][199], // 300 posts after for a total of 1500
				},
			},
		})
		assert.Nil(t, err)
		assert.True(t, allow, "positive result expected")
	})
}

func TestGetIncrementalPosts(t *testing.T) {
	t.Run("get incremental posts complete", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		channels, _, posts := populateChannels(th, 3, 50, 2)
		increment, err := th.App.GetIncrementalPosts(&model.IncrementPostsRequest{
			Channels: []model.ChannelWithPost{
				{
					ChannelId: (*channels)[0].Id,
					PostId:    (*posts)[(*channels)[0].Id][24],
				},
				{
					ChannelId: (*channels)[1].Id,
					PostId:    (*posts)[(*channels)[1].Id][24],
				},
				{
					ChannelId: (*channels)[2].Id,
					PostId:    (*posts)[(*channels)[2].Id][24],
				},
			},
			MaxMessages: 75, // all posts should fit exactly
		})
		assert.Nil(t, err)
		assert.Equal(t, 3, len(*increment), "wrong number of channels")
		for i, v := range *increment {
			assert.Equal(t, (*channels)[i].Id, v.ChannelId, "unexpected channel id")
			assert.Equal(t, 25, len(*v.Posts), "wrong number of posts")
			assert.True(t, v.Complete, "incremental update should be complete")
			expectedPosts := (*posts)[v.ChannelId]
			for j := 0; j < 25; j++ {
				assert.Equal(t, expectedPosts[25+j], (*v.Posts)[j].Id, "unexpected post id")
			}
		}
	})

	t.Run("get incremental posts incomplete", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		channels, _, posts := populateChannels(th, 3, 50, 2)
		increment, err := th.App.GetIncrementalPosts(&model.IncrementPostsRequest{
			Channels: []model.ChannelWithPost{
				{
					ChannelId: (*channels)[0].Id,
					PostId:    (*posts)[(*channels)[0].Id][24],
				},
				{
					ChannelId: (*channels)[1].Id,
					PostId:    (*posts)[(*channels)[1].Id][24],
				},
				{
					ChannelId: (*channels)[2].Id,
					PostId:    (*posts)[(*channels)[2].Id][24],
				},
			},
			MaxMessages: 60, // the last chanel should have an incomplete update
		})
		assert.Nil(t, err)
		assert.Equal(t, 3, len(*increment), "wrong number of channels")
		for i, v := range *increment {
			assert.Equal(t, (*channels)[i].Id, v.ChannelId, "unexpected channel id")
			if i != 2 {
				// complete channels
				assert.Equal(t, 25, len(*v.Posts), "wrong number of posts")
				assert.True(t, v.Complete, "incremental update should be complete")
				expectedPosts := (*posts)[v.ChannelId]
				for j := 0; j < 25; j++ {
					assert.Equal(t, expectedPosts[25+j], (*v.Posts)[j].Id, "unexpected post id")
				}
			} else {
				// incomplete channel
				assert.Equal(t, 10, len(*v.Posts), "wrong number of posts")
				assert.False(t, v.Complete, "incremental update should be incomplete")
				expectedPosts := (*posts)[v.ChannelId]
				for j := 0; j < 10; j++ {
					assert.Equal(t, expectedPosts[25+j], (*v.Posts)[j].Id, "unexpected post id")
				}
			}
		}
	})

	t.Run("get incremental posts one channel is empty", func(t *testing.T) {
		th := Setup(t).InitBasic()
		defer th.TearDown()

		channels, _, posts := populateChannels(th, 3, 50, 2)
		emptyChannels, _, _ := populateChannels(th, 1, 0, 2)
		request := model.IncrementPostsRequest{
			Channels: []model.ChannelWithPost{
				{
					ChannelId: (*channels)[0].Id,
					PostId:    (*posts)[(*channels)[0].Id][24],
				},
				{
					ChannelId: (*emptyChannels)[0].Id,
				},
				{
					ChannelId: (*channels)[1].Id,
					PostId:    (*posts)[(*channels)[1].Id][24],
				},
				{
					ChannelId: (*channels)[2].Id,
					PostId:    (*posts)[(*channels)[2].Id][24],
				},
			},
			MaxMessages: 60, // the last chanel should have an incomplete update
		}
		increment, err := th.App.GetIncrementalPosts(&request)
		assert.Nil(t, err)
		assert.Equal(t, 4, len(*increment), "wrong number of channels")
		for i, v := range *increment {
			assert.Equal(t, request.Channels[i].ChannelId, v.ChannelId, "unexpected channel id")
			if i == 1 {
				// should be empty
				assert.Equal(t, 0, len(*v.Posts), "wrong number of posts")
				assert.True(t, v.Complete, "incremental update should be complete")
			} else if i == 0 || i == 2 {
				// complete channels
				assert.Equal(t, 25, len(*v.Posts), "wrong number of posts")
				assert.True(t, v.Complete, "incremental update should be complete")
				expectedPosts := (*posts)[v.ChannelId]
				for j := 0; j < 25; j++ {
					assert.Equal(t, expectedPosts[25+j], (*v.Posts)[j].Id, "unexpected post id")
				}
			} else {
				// incomplete channel
				assert.Equal(t, 10, len(*v.Posts), "wrong number of posts")
				assert.False(t, v.Complete, "incremental update should be incomplete")
				expectedPosts := (*posts)[v.ChannelId]
				for j := 0; j < 10; j++ {
					assert.Equal(t, expectedPosts[25+j], (*v.Posts)[j].Id, "unexpected post id")
				}
			}
		}
	})
}
