package model

import (
	"encoding/json"
	"io"
)

type Device struct {
	Platform  string `json:"platform"`
	DeviceId  string `json:"device_id"`
	PushToken string `json:"push_token"`
}

func (o *Device) ToJson() string {
	b, _ := json.Marshal(o)
	return string(b)
}

func DeviceFromJson(data io.Reader) *Device {
	var o *Device
	json.NewDecoder(data).Decode(&o)
	return o
}

var MockDevice = &Device{
	Platform:  "apple",
	DeviceId:  "1234567890",
	PushToken: "12345678901234567890",
}
