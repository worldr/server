// Copyright (c) 2020-present Worldr, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api4

import (
	"net/http"

	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/store"
)

func (api *API) InitWChannel() {
	api.BaseRoutes.WChannels.Handle("/categories", api.ApiSessionRequired(getChannelsCategories)).Methods("GET")
	api.BaseRoutes.WChannels.Handle("/personal", api.ApiSessionRequired(getPersonalChannels)).Methods("GET")
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
	for i, v := range *list {
		m := (*v).LastMessage
		if m != nil {
			if _, exists := distinct[m.UserId]; !exists {
				distinct[m.UserId] = true
				uids = uids[:len(uids)+1]
				uids[i] = m.UserId
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

func getPersonalChannels(c *Context, w http.ResponseWriter, r *http.Request) {
	team, err := c.App.MainTeam()
	if err != nil {
		c.Err = err
		return
	}
	uid := c.App.Session().UserId
	if list, err := c.App.GetPersonalChannels(team.Id, uid); err != nil {
		c.Err = err
	} else {
		if err := fillLastUsers(c, list); err != nil {
			c.Err = err
		} else {
			w.Write([]byte(list.ChannelSnapshotListToJson()))
		}
	}
}
