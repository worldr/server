package model

import (
	"encoding/json"
	"io"
)

type ChannelWithPost struct {
	ChannelId string `json:"channel_id"`
	PostId    string `json:"post_id"`
}

type IncrementalPosts struct {
	ChannelId string          `json:"channel_id"`
	Posts     *PostListSimple `json:"posts"`
	Complete  bool            `json:"complete"`
}

type IncrementPostsRequest struct {
	Channels    []ChannelWithPost `json:"channels"`
	MaxMessages int               `json:"max_messages"`
}

type IncrementPostsResponse struct {
	Content *[]IncrementalPosts `json:"content"`
}

type IncrementCheckRequest struct {
	Channels []ChannelWithPost `json:"channels"`
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

func IncrementPostsRequestDataFromJson(data io.Reader) *IncrementPostsRequest {
	decoder := json.NewDecoder(data)
	var d IncrementPostsRequest
	err := decoder.Decode(&d)
	if err != nil {
		return nil
	}
	return &d
}

func (o *IncrementPostsResponse) ToJson() string {
	b, _ := json.Marshal(o)
	return string(b)
}
