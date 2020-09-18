// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPushNotification(t *testing.T) {
	t.Run("should build a push notification from JSON", func(t *testing.T) {
		msg := PushNotification{Platform: "test"}
		json := msg.ToJson()
		result, err := PushNotificationFromJson(strings.NewReader(json))

		require.Nil(t, err)
		require.Equal(t, msg.Platform, result.Platform, "ids do not match")
	})

	t.Run("should throw an error when the message is nil", func(t *testing.T) {
		_, err := PushNotificationFromJson(nil)
		require.NotNil(t, err)
		require.Equal(t, "push notification data can't be nil", err.Error())
	})

	t.Run("should throw an error when the message parsing fails", func(t *testing.T) {
		_, err := PushNotificationFromJson(strings.NewReader(""))
		require.NotNil(t, err)
		require.Equal(t, "EOF", err.Error())
	})
}

func TestPushNotificationAck(t *testing.T) {
	t.Run("should build a push notification ack from JSON", func(t *testing.T) {
		msg := PushNotificationAck{ClientPlatform: "test"}
		json := msg.ToJson()
		result, err := PushNotificationAckFromJson(strings.NewReader(json))

		require.Nil(t, err)
		require.Equal(t, msg.ClientPlatform, result.ClientPlatform, "ids do not match")
	})

	t.Run("should throw an error when the message is nil", func(t *testing.T) {
		_, err := PushNotificationAckFromJson(nil)
		require.NotNil(t, err)
		require.Equal(t, "push notification data can't be nil", err.Error())
	})

	t.Run("should throw an error when the message parsing fails", func(t *testing.T) {
		_, err := PushNotificationAckFromJson(strings.NewReader(""))
		require.NotNil(t, err)
		require.Equal(t, "EOF", err.Error())
	})
}

func TestPushNotificationServerTag(t *testing.T) {

	msg := PushNotification{ServerTag: "test"}

	tag := "changed"
	msg.SetServerTag(&tag)
	require.Equal(t, msg.ServerTag, "changed", msg.ServerTag)
	msg.ServerTag = ""

	msg.SetServerTag(nil)
	require.Equal(t, msg.ServerTag, "", msg.ServerTag)
}
