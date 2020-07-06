package model

import (
	"encoding/json"
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
