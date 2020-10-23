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

type ChannelSync struct {
	Channel *ChannelSnapshot     `json:"channel"`
	Members *ChannelMembersShort `json:"members"`
}

type ChannelUpdates struct {
	Added       *[]string               `json:"added"`         // Channels the users became a member of
	Removed     *[]string               `json:"demoved"`       // Channels the user is no longer a member of (channel deleted or user kicked/left)
	Updated     *[]string               `json:"updated"`       // Channels with new content
	ChannelById map[string]*ChannelSync `json:"channel_by_id"` // Info on channels listed in removed and updated lists
}

func (o *ChannelSync) ToJson() string {
	b, _ := json.Marshal(o)
	return string(b)
}

func ChannelSyncFromJson(data io.Reader) (*ChannelSync, error) {
	decoder := json.NewDecoder(data)
	var o ChannelSync
	err := decoder.Decode(&o)
	if err != nil {
		return nil, err
	}
	return &o, err
}

func (o *ChannelUpdates) ToJson() string {
	b, _ := json.Marshal(o)
	return string(b)
}

func ChannelUpdatesFromJson(data io.Reader) (*ChannelUpdates, error) {
	decoder := json.NewDecoder(data)
	var o ChannelUpdates
	err := decoder.Decode(&o)
	if err != nil {
		return nil, err
	}
	return &o, err
}

func (o *ChannelWithPost) ToJson() string {
	b, _ := json.Marshal(o)
	return string(b)
}

func ChannelWithPostListToJson(list []*ChannelWithPost) string {
	if b, err := json.Marshal(list); err != nil {
		return "[]"
	} else {
		return string(b)
	}
}

func ChannelWithPostListFromJson(data io.Reader) (*[]ChannelWithPost, error) {
	decoder := json.NewDecoder(data)
	var list []ChannelWithPost
	err := decoder.Decode(&list)
	if err != nil {
		return nil, err
	}
	return &list, err
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
