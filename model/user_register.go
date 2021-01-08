// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package model

import (
	"encoding/json"
	"io"
)

type RegisterEmailsResponse struct {
	Successes []*User           `json:"successes"`
	Failures  map[string]string `json:"failures"`
}

// ToJson convert a RegisterEmailsResponse to a json string
func (u *RegisterEmailsResponse) ToJson() string {
	b, _ := json.Marshal(u)
	return string(b)
}

// RegisterEmailsResponseFromJson will decode the input and return a RegisterEmailsResponse
func RegisterEmailsResponseFromJson(data io.Reader) (*RegisterEmailsResponse, error) {
	var result *RegisterEmailsResponse
	if err := json.NewDecoder(data).Decode(&result); err != nil {
		return nil, err
	} else {
		return result, nil
	}
}
