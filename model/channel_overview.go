// Copyright (c) 2020-present Worldr, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package model

import (
	"encoding/json"
)

type ChannelOverview struct {
	Channels *ChannelList                     `json:"channels"`
	Members  *map[string]*ChannelMembersShort `json:"members"`
	Users    *[]*User                         `json:"users"`
	Statuses *[]*Status                       `json:"statuses"`
}

func (o *ChannelOverview) ToJson() string {
	b, _ := json.Marshal(o)
	return string(b)
}
