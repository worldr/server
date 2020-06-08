// Copyright (c) 2020-present Worldr, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api4

import (
	"net/http"

	"github.com/mattermost/mattermost-server/v5/model"
)

func (api *API) InitWFile() {

	api.BaseRoutes.WFiles.Handle("", api.ApiSessionRequired(getWFilesInfos)).Methods("GET")

}

// Get all the FileInfos given a GetFileInfosOptions object. This is used to get
// all the data for the Worldr "File" tab.
//
// This could take a user ID as a part of the context to allow administrators
// to get files for users, if need be. For now, let us not bother.
//
// Worldr only.
func getWFilesInfos(c *Context, w http.ResponseWriter, r *http.Request) {
	opts := model.GetFileInfosOptions{UserIds: []string{c.App.Session().UserId}}
	infos, err := c.App.GetFileInfos(0, 11, &opts)
	if err != nil {
		c.Err = err
		return
	}

	// Checks that the current UserId actually has access to those files?
	// Isn't that useless? Possibly.
	for _, fileInfo := range infos {
		if fileInfo.CreatorId != c.App.Session().UserId && !c.App.SessionHasPermissionToChannelByPost(*c.App.Session(), "", model.PERMISSION_READ_CHANNEL) {
			c.SetPermissionError(model.PERMISSION_READ_CHANNEL)
			return
		}
	}

	// All is good, let's return,
	w.Header().Set("Cache-Control", "max-age=2592000, public")
	response := model.FileInfoResponseWrapper{Content: &infos}
	w.Write([]byte(response.ToJson()))
}
