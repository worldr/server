// Copyright (c) 2020-present Worldr, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api4

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/store"
)

func (api *API) InitWChannel() {

	// all channels
	api.BaseRoutes.WChannels.Handle("/categories", api.ApiSessionRequired(getChannelsCategories)).Methods("GET")
	api.BaseRoutes.WChannels.Handle("/personal", api.ApiSessionRequired(getPersonalChannels)).Methods("GET")
	api.BaseRoutes.WChannels.Handle("/work", api.ApiSessionRequired(getWorkChannels)).Methods("GET")
	api.BaseRoutes.WChannels.Handle("/global", api.ApiSessionRequired(getGlobalChannels)).Methods("GET")
	api.BaseRoutes.WChannels.Handle("/overview", api.ApiSessionRequired(getOverview)).Methods("GET")

	// channel with specific id
	api.BaseRoutes.WChannel.Handle("/image", api.ApiSessionRequired(getChannelImage)).Methods("GET")
}
func getChannelImage(c *Context, w http.ResponseWriter, r *http.Request) {
	c.RequireChannelId()
	if c.Err != nil {
		return
	}

	session := c.App.Session()
	isReadable := c.App.SessionHasPermissionToChannel(
		*session,
		c.Params.ChannelId,
		model.PERMISSION_READ_CHANNEL,
	)
	channel, err := c.App.GetChannel(c.Params.ChannelId)
	if err != nil {
		c.Err = err
		return
	}
	// always allow getting pictures for open chats plus the chats the user is permitted to read
	if channel.Type != "O" && !isReadable {
		c.Err = model.NewAppError("getChannelImage", "api.channel.get_image.not_allowed.app_error", nil, "", http.StatusBadRequest)
		return
	}

	etag := strconv.FormatInt(channel.LastPictureUpdate, 10)
	if c.HandleEtag(etag, "Get Channel Image", w, r) {
		return
	}

	image, err := c.App.GetChannelImage(c.Params.ChannelId)
	if err != nil {
		c.Err = err
		return
	}
	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%v, public", 24*60*60)) // 24 hrs
	w.Header().Set(model.HEADER_ETAG_SERVER, etag)
	w.Header().Set("Content-Type", "image/png")
	w.Write(image)
}

func getChannelsCategories(c *Context, w http.ResponseWriter, r *http.Request) {
	uid := c.App.Session().UserId
	cats, err := c.App.GetChannelsCategories(uid)
	if err != nil {
		c.Err = err
		return
	}
	w.Write([]byte(cats.ChannelCategoriesListToJson()))
}

func fillLastUsers(c *Context, list *model.ChannelSnapshotList) *model.AppError {
	// Get a list of distinct uids
	uids := make([]string, len(*list))[:0]
	distinct := make(map[string]bool)
	for _, v := range *list {
		m := (*v).LastMessage
		if m != nil {
			if _, exists := distinct[m.UserId]; !exists {
				distinct[m.UserId] = true
				uids = append(uids, m.UserId)
			}
		}
	}
	// Get users
	users, err := c.App.GetUsersByIds(uids, &store.UserGetByIdsOpts{})
	if err != nil {
		return err
	}
	usersMap := make(map[string]*model.User, len(users))
	for _, v := range users {
		usersMap[(*v).Id] = v
	}
	// Place the users into snapshots
	for _, v := range *list {
		if (*v).LastMessage != nil {
			(*v).LastUser = usersMap[(*v).LastMessage.UserId]
		}
	}
	return nil
}

type specChannelsGetter func(string, string) (*model.ChannelSnapshotList, *model.AppError)

func getSpecificChannels(c *Context, w http.ResponseWriter, r *http.Request, getter specChannelsGetter) {
	team, err := c.App.MainTeam()
	if err != nil {
		c.Err = err
		return
	}
	uid := c.App.Session().UserId
	if list, err := getter(team.Id, uid); err != nil {
		c.Err = err
	} else {
		if err := fillLastUsers(c, list); err != nil {
			c.Err = err
		} else {
			w.Write([]byte(list.ChannelSnapshotListToJson()))
		}
	}
}

func getPersonalChannels(c *Context, w http.ResponseWriter, r *http.Request) {
	getSpecificChannels(c, w, r, c.App.GetPersonalChannels)
}

func getWorkChannels(c *Context, w http.ResponseWriter, r *http.Request) {
	getSpecificChannels(c, w, r, c.App.GetWorkChannels)
}

func getGlobalChannels(c *Context, w http.ResponseWriter, r *http.Request) {
	getSpecificChannels(c, w, r, c.App.GetGlobalChannels)
}

func getOverview(c *Context, w http.ResponseWriter, r *http.Request) {
	team, err := c.App.MainTeam()
	if err != nil {
		c.Err = err
		return
	}
	uid := c.App.Session().UserId
	// Get channels visible to user and their members
	if channels, membersByChannel, uids, err := c.App.GetOverview(team.Id, uid); err != nil {
		c.Err = err
	} else {
		// Get users
		users, err := c.App.GetUsersByIds(*uids, &store.UserGetByIdsOpts{})
		if err != nil {
			c.Err = err
		}
		// Get statuses
		statuses, err := c.App.GetUserStatusesByIds(*uids)
		if err != nil {
			c.Err = err
		}

		o := &model.ChannelOverview{
			Channels: channels,
			Members:  membersByChannel,
			Users:    &users,
			Statuses: &statuses,
		}
		w.Write([]byte(o.ToJson()))
	}
}
