// Copyright (c) 2020-present Worldr, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api4

import (
	"encoding/json"
	"net/http"

	"github.com/mattermost/mattermost-server/v5/app"
	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/store"
)

// InitWSecure initializes request handles for security purposes.
func (api *API) InitWSecure() {
	api.BaseRoutes.WSecure.Handle("/pk", api.ApiHandler(getPublicSignKey)).Methods("GET")
}

// GetSigningKey reads the certificate pinning ECDSA key from the database,
// generates one if none exists.
func GetSigningKey(s store.SystemStore) (*model.SystemAsymmetricSigningKey, *model.AppError) {
	var key *model.SystemAsymmetricSigningKey
	value, err := s.GetByName(model.SYSTEM_CERTIFICATE_SIGNING_KEY)
	if err == nil {
		if err := json.Unmarshal([]byte(value.Value), &key); err != nil {
			return nil, model.NewAppError("getPublicSignKey", "get_sign_key.db", nil, err.Error(), http.StatusInternalServerError)
		}
	}
	if key == nil {
		system, newKey, err := app.GenerateSigningKey(model.SYSTEM_CERTIFICATE_SIGNING_KEY)
		if err != nil {
			return nil, model.NewAppError("getPublicSignKey", "get_sign_key.generate", nil, err.Error(), http.StatusInternalServerError)
		}
		// If we were able to save the key, use it, otherwise respond with error.
		if appErr := s.Save(system); appErr != nil {
			return nil, appErr
		}
		key = newKey
	}
	return key, nil
}

// Get public signing key for certificate pinning
func getPublicSignKey(c *Context, w http.ResponseWriter, r *http.Request) {
	key, err := GetSigningKey(c.App.Srv().Store.System())
	if err != nil {
		c.Err = err
		return
	}
	response := &model.SigningPK{
		X: key.ECDSAKey.X.Text(16),
		Y: key.ECDSAKey.Y.Text(16),
	}
	w.Write([]byte(response.ToJson()))
}
