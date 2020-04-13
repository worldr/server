// Copyright (c) 2020-present Worldr, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api4

import (
	"net/http"
)

func (api *API) InitWChannel() {
	api.BaseRoutes.WChannels.Handle("/categories", api.ApiSessionRequired(getChannelsCategories)).Methods("GET")
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
