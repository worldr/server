package model

import (
	"encoding/json"
	"io"
)

type LoginData struct {
	LoginId            string  `json:"login_id"`
	Password           string  `json:"password"`
	Device             *Device `json:"device"`
	Id                 string  `json:"id"`
	MfaToken           string  `json:"token"`
	LdapOnly           bool    `json:"ldap_only"`
	UseAdminSessionTtl bool
}

func (o *LoginData) ToJson() string {
	b, _ := json.Marshal(o)
	return string(b)
}

func LoginDataFromJson(data io.Reader) (*LoginData, error) {
	var o *LoginData
	if err := json.NewDecoder(data).Decode(&o); err != nil {
		return nil, err
	} else {
		return o, nil
	}
}
