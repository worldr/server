// Copyright (c) 2020-present Worldr, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api4

func (api *API) InitWUser() {
	api.BaseRoutes.WUsers.Handle("/login", api.ApiHandler(login)).Methods("POST")
	api.BaseRoutes.WUsers.Handle("/logout", api.ApiHandler(logout)).Methods("POST")
}
