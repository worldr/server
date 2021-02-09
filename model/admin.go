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

type VersionedValue struct {
	Value     string `json:"value,omitempty"`
	Version   string `json:"version"`
	Signature string `json:"signature"` // server certificate signature
}

func (me *VersionedValue) ToJson() string {
	b, _ := json.Marshal(me)
	return string(b)
}

func VersionedValueFromJson(data io.Reader) *VersionedValue {
	var w *VersionedValue
	json.NewDecoder(data).Decode(&w)
	return w
}

type Configurable struct {
	Email *EmailSettingsExposed `json:"email,omitempty"`
}

func (o *Configurable) ToJson() string {
	b, _ := json.Marshal(o)
	return string(b)
}

func ConfigurableFromJson(data io.Reader) (*Configurable, error) {
	var o *Configurable
	err := json.NewDecoder(data).Decode(&o)
	return o, err
}
