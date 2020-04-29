// Copyright (c) 2020-present Worldr, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package model

import (
	"encoding/json"
)

type ChannelInfo struct {
	Id           string `json:"id"`
	Members      int    `json:"members"`
	MsgCount     int    `json:"msg_count"`
	MentionCount int    `json:"mention_count"`
}

type ChannelInfoList []*ChannelInfo

func (o *ChannelInfo) ToJson() string {
	b, _ := json.Marshal(o)
	return string(b)
}
