// Copyright (c) 2020-present Worldr, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package model

import (
	"encoding/json"
	"io"
	"regexp"
	"strings"
)

// Structure for storing categories relations to user channels
type ChannelCategory struct {
	// This is a condensed Name: lowercased, all whitespace removed
	// Only used on the server side to distinguish categories
	Id string `json:"id"`

	// All categories are per-user
	UserId string `json:"user_id"`

	ChannelId string `json:"channel_id"`

	// The name of the category as the user sees it
	Name string `json:"name"`
	Sort int32  `json:"sort"`
}

type ChannelCategoryOrder struct {
	Name string `json:"name"`
	Sort int32  `json:"sort"`
}

type ChannelCategoryAggregated struct {
	Name     string   `json:"name"`
	Sort     int32    `json:"sort"`
	Channels []string `json:"channels"`
}

type ChannelCategoriesList []*ChannelCategory
type ChannelCategoriesAggregatedList []*ChannelCategoryAggregated

var condenseName = regexp.MustCompile(`\s+`)

func GetChannelCategoryId(name string) string {
	return condenseName.ReplaceAllString(strings.TrimSpace(strings.ToLower(name)), "-")
}

func (me *ChannelCategory) GetId() string {
	return GetChannelCategoryId(me.Name)
}

func (me *ChannelCategory) IsValidCategory() bool {
	return len(me.ChannelId) == 26 && len(me.UserId) == 26 && len(condenseName.ReplaceAllString(me.Name, "")) > 0
}

func ChannelCategoryFromJson(data io.Reader) *ChannelCategory {
	var o *ChannelCategory
	json.NewDecoder(data).Decode(&o)
	return o
}

func ChannelCategoryOrderFromJson(data io.Reader) *ChannelCategory {
	var o *ChannelCategory
	json.NewDecoder(data).Decode(&o)
	return o
}

func (o *ChannelCategory) ToJson() string {
	b, _ := json.Marshal(o)
	return string(b)
}

func (o *ChannelCategoryAggregated) ToJson() string {
	b, _ := json.Marshal(o)
	return string(b)
}

func ChannelCategoryAggregatedFromJson(data io.Reader) *ChannelCategoryAggregated {
	var o *ChannelCategoryAggregated
	json.NewDecoder(data).Decode(&o)
	return o
}

func ChannelCategoriesAggregatedListToJson(o ChannelCategoriesAggregatedList) string {
	if b, err := json.Marshal(o); err != nil {
		return "[]"
	} else {
		return string(b)
	}
}

func ChannelCategoriesAggregatedListFromJson(data io.Reader) ChannelCategoriesAggregatedList {
	var list []*ChannelCategoryAggregated
	json.NewDecoder(data).Decode(&list)
	return list
}
