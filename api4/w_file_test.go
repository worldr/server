// Copyright (c) 2020-present Worldr, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api4

import (
	"testing"
	"time"

	"github.com/mattermost/mattermost-server/v5/utils/fileutils"
	"github.com/mattermost/mattermost-server/v5/utils/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var wtestDir = ""

func init() {
	wtestDir, _ = fileutils.FindDir("tests")
}

// Test that we can get all the infos structures for all files.
func TestGetFileInfos(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()
	Client := th.Client
	//user := th.BasicUser
	channel := th.BasicChannel

	if *th.App.Config().FileSettings.DriverName == "" {
		t.Skip("skipping because no file driver is enabled")
	}

	sent, err := testutils.ReadTestFile("test.png")
	require.NoError(t, err)

	fileResp, resp := Client.UploadFile(sent, channel.Id, "test.png")
	CheckNoError(t, resp)

	fileResp, resp = Client.UploadFile(sent, channel.Id, "test.png")
	CheckNoError(t, resp)

	// Wait a bit for files to ready
	time.Sleep(3 * time.Second)

	info, resp := Client.GetFileInfos()
	CheckNoError(t, resp)

	userId := fileResp.FileInfos[0].CreatorId
	require.Equal(t, 2, len(info), "Correct number of file infos not returned")
	for _, fileInfo := range info {
		require.Equal(t, userId, fileInfo.CreatorId, "file should be assigned to user")
		require.Equal(t, "", fileInfo.PostId, "file shouldn't have a post")
		require.Equal(t, "", fileInfo.Path, "file path shouldn't have been returned to client")
		require.Equal(t, "", fileInfo.ThumbnailPath, "file thumbnail path shouldn't have been returned to client")
		require.Equal(t, "", fileInfo.PreviewPath, "file preview path shouldn't have been returned to client")
		require.Equal(t, "image/png", fileInfo.MimeType, "mime type should have been image/png")
	}

	Client.Logout()
	_, resp = Client.GetFileInfos()
	CheckUnauthorizedStatus(t, resp)

	otherUser := th.CreateUser()
	Client.Login(otherUser.Email, otherUser.Password)
	info, _ = Client.GetFileInfos()
	assert.Empty(t, info)

	Client.Logout()
	_, resp = th.SystemAdminClient.GetFileInfos()
	CheckNoError(t, resp)
}
