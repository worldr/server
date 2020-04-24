// Copyright (c) 2020-present Worldr, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package model

import (
	"encoding/json"
	"io"
)

type ChannelCategory struct {
	UserId    string `json:"user_id"`
	ChannelId string `json:"channel_id"`
	Name      string `json:"name"`
	Sort      int32  `json:"sort"`
}

type ChannelCategoriesList []*ChannelCategory

func (o *ChannelCategory) ToJson() string {
	b, _ := json.Marshal(o)
	return string(b)
}

func ChannelCategoryFromJson(data io.Reader) *ChannelCategory {
	var o *ChannelCategory
	json.NewDecoder(data).Decode(&o)
	return o
}

func (o *ChannelCategoriesList) ChannelCategoriesListToJson() string {
	if b, err := json.Marshal(o); err != nil {
		return "[]"
	} else {
		return string(b)
	}
}
