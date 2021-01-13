// Copyright (c) 2020-present Worldr, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api4

import (
	"net/http"
	"strconv"

	"github.com/mattermost/mattermost-server/v5/model"
)

// InitWSecure initializes request handles for security purposes.
func (api *API) InitWSecure() {
	api.BaseRoutes.WSecure.Handle("/pk", api.ApiHandler(getPublicSignKey)).Methods("GET")
	api.BaseRoutes.WSecure.Handle("/cert", api.ApiHandler(getCertSignature)).Methods("GET")
	api.BaseRoutes.WSecure.Handle("/company", api.ApiSessionRequired(getCompanyInfo)).Methods("GET")
}

// Get public signing key for certificate pinning
func getPublicSignKey(c *Context, w http.ResponseWriter, r *http.Request) {
	key, err := c.App.Srv().CertSigningKey()
	if err != nil {
		c.Err = err
		return
	}
	s, err := c.App.Srv().CertSignature()
	if err != nil {
		c.Err = err
		return
	}
	response := &model.VersionedValue{
		Value:     key.Public,
		Version:   strconv.Itoa(key.Version),
		Signature: s.Signature,
	}
	w.Write([]byte(response.ToJson()))
}

// Get certificate Ed25519 signature for certificate pinning
func getCertSignature(c *Context, w http.ResponseWriter, r *http.Request) {
	s, err := c.App.Srv().CertSignature()
	if err != nil {
		c.Err = err
		return
	}
	w.Write([]byte(s.ToJson()))
}

func getCompanyInfo(c *Context, w http.ResponseWriter, r *http.Request) {
	s, err := c.App.Srv().CompanyConfig()
	if err != nil {
		c.Err = err
		return
	}
	key, err := c.App.Srv().CertSigningKey()
	if err != nil {
		c.Err = err
		return
	}
	s.Key = key.Public
	s.KeyVersion = key.Version
	w.Write([]byte(s.ToJson()))
}
