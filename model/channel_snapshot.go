// Copyright (c) 2020-present Worldr, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package model

import (
	"encoding/json"
)

type ChannelSnapshot struct {
	Channel     *Channel     `json:"channel"`
	LastMessage *Post        `json:"last_message"`
	LastUser    *User        `json:"last_user"`
	Info        *ChannelInfo `json:"info"`
}

type ChannelSnapshotList []*ChannelSnapshot

func (o *ChannelSnapshot) ToJson() string {
	b, _ := json.Marshal(o)
	return string(b)
}

func (o *ChannelSnapshotList) ChannelSnapshotListToJson() string {
	if b, err := json.Marshal(o); err != nil {
		return "[]"
	} else {
		return string(b)
	}
}
