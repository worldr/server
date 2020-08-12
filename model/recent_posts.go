package model

import (
	"encoding/json"
	"io"
)

type RecentPostsRequestData struct {
	ChannelIds         []string `json:"channel_ids"`
	MaxTotalMessages   int      `json:"max_total_messages"`
	MessagesPerChannel int      `json:"messages_per_channel"`
}

type RecentPostsResponseData struct {
	Content *PostListSimple `json:"content"`
}

func RecentRequestDataFromJson(data io.Reader) *RecentPostsRequestData {
	decoder := json.NewDecoder(data)
	var d RecentPostsRequestData
	err := decoder.Decode(&d)
	if err != nil {
		return nil
	}
	return &d
}

func (o *RecentPostsRequestData) ToJson() string {
	b, _ := json.Marshal(o)
	return string(b)
}

func (o *RecentPostsResponseData) ToJson() string {
	b, _ := json.Marshal(o)
	return string(b)
}

func RecentResponseDataFromJson(data io.Reader) *RecentPostsResponseData {
	decoder := json.NewDecoder(data)
	var d RecentPostsResponseData
	err := decoder.Decode(&d)
	if err != nil {
		return nil
	}
	return &d
}
