package model

import (
	"encoding/json"
	"io"
)

type AdminAuthResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	User      *User  `json:"user"`
}

func (me *AdminAuthResponse) ToJson() string {
	b, _ := json.Marshal(me)
	return string(b)
}

func AdminAuthResponseFromJson(data io.Reader) *AdminAuthResponse {
	var w *AdminAuthResponse
	json.NewDecoder(data).Decode(&w)
	return w
}

type AdminUsersPage struct {
	Users   *[]*User `json:"users"`
	Total   uint64   `json:"total"`
	From    uint64   `json:"from"`
	PerPage uint64   `json:"per_page"`
}

func (me *AdminUsersPage) ToJson() string {
	b, _ := json.Marshal(me)
	return string(b)
}

type AdminTokenCheck struct {
	Valid     bool   `json:"valid"`
	ExpiresAt string `json:"expires_at"`
}

func (me *AdminTokenCheck) ToJson() string {
	b, _ := json.Marshal(me)
	return string(b)
}

type AdminSetupStatus struct {
	Team  bool `json:"team"`
	Admin bool `json:"admin"`
}

func (me *AdminSetupStatus) ToJson() string {
	b, _ := json.Marshal(me)
	return string(b)
}

func AdminSetupStatusFromJson(data io.Reader) *AdminSetupStatus {
	var w *AdminSetupStatus
	json.NewDecoder(data).Decode(&w)
	return w
}

type SigningPK struct {
	X string `json:"x"`
	Y string `json:"y"`
}

func (me *SigningPK) ToJson() string {
	b, _ := json.Marshal(me)
	return string(b)
}
