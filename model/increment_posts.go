package model

import (
	"encoding/json"
	"io"
)

type ChannelWithLastPost struct {
	ChannelId  string `json:"channel_id"`
	LastPostId string `json:"last_post_id"`
}

type IncrementPostsRequest struct {
	Channels    []ChannelWithLastPost `json:"channels"`
	MaxMessages int                   `json:"max_messages"`
}

type IncrementCheckRequest struct {
	Channels []ChannelWithLastPost `json:"channels"`
}

type IncrementCheckResponse struct {
	Allow bool `json:"allow"`
}

func IncrementCheckRequestDataFromJson(data io.Reader) *IncrementCheckRequest {
	decoder := json.NewDecoder(data)
	var d IncrementCheckRequest
	err := decoder.Decode(&d)
	if err != nil {
		return nil
	}
	return &d
}

func (o *IncrementCheckResponse) ToJson() string {
	b, _ := json.Marshal(o)
	return string(b)
}
