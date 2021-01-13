package model

import (
	"encoding/json"
	"io"
)

// CompanyConfig reflects the structure of publicly available company information
type CompanyConfig struct {
	Aliases    []string `json:"aliases"`
	Deployment string   `json:"deployment"`
	Key        string   `json:"key,omitempty"`
	KeyVersion int      `json:"key_version,omitempty"`
	Name       string   `json:"name"`
	Server     string   `json:"server"`
}

func (me *CompanyConfig) ToJson() string {
	b, _ := json.Marshal(me)
	return string(b)
}

func CompanyConfigFromJson(data io.Reader) (*CompanyConfig, error) {
	var o *CompanyConfig
	err := json.NewDecoder(data).Decode(&o)
	return o, err
}

func (me *CompanyConfig) SetDefaults() {
	me.Aliases = []string{"local"}
	me.Deployment = "local"
	me.Name = "Local"
	me.Server = "localhost:8065"
}
