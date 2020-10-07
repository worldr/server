// Copyright (c) 2020-present Worldr, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api4

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/store"
)

// InitWSecure initializes request handles for security purposes.
func (api *API) InitWSecure() {
	api.BaseRoutes.WSecure.Handle("/pk", api.ApiHandler(getPublicSignKey)).Methods("GET")
}

// GetCertSigningKey reads the certificate pinning Ed25519 key from the database,
// generates one if none exists.
func GetCertSigningKey(s store.SystemStore) (*model.SystemEd25519Key, *model.AppError) {
	var key *model.SystemEd25519Key
	value, err := s.GetByName(model.SYSTEM_CERTIFICATE_SIGNING_KEY)
	if err == nil {
		if err := json.Unmarshal([]byte(value.Value), &key); err != nil {
			return nil, model.NewAppError("getPublicSignKey", "get_sign_key.db", nil, err.Error(), http.StatusInternalServerError)
		}
	}
	if key == nil {
		pk, sk, keyErr := ed25519.GenerateKey(nil)
		if keyErr != nil {
			return nil, model.NewAppError("getPublicSignKey", "get_sign_key.generate", nil, keyErr.Error(), http.StatusInternalServerError)
		}
		key = &model.SystemEd25519Key{
			Public:  hex.EncodeToString(pk),
			Secret:  hex.EncodeToString(sk),
			Version: 1,
		}
		system := &model.System{
			Name: model.SYSTEM_CERTIFICATE_SIGNING_KEY,
		}
		v, keyErr := json.Marshal(key)
		if keyErr != nil {
			return nil, model.NewAppError("getPublicSignKey", "get_sign_key.serialise", nil, keyErr.Error(), http.StatusInternalServerError)
		}
		system.Value = string(v)

		// If we were able to save the key, use it, otherwise respond with error.
		if appErr := s.Save(system); appErr != nil {
			return nil, appErr
		}
	}
	return key, nil
}

// Get public signing key for certificate pinning
func getPublicSignKey(c *Context, w http.ResponseWriter, r *http.Request) {
	key, err := GetCertSigningKey(c.App.Srv().Store.System())
	if err != nil {
		c.Err = err
		return
	}
	response := &model.SigningPK{
		Key: key.Public,
	}
	w.Write([]byte(response.ToJson()))
}
