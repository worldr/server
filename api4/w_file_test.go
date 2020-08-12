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

	if *th.App.Config().FileSettings.DriverName == "" {
		t.Skip("skipping because no file driver is enabled")
	}

	sent, err := testutils.ReadTestFile("test.png")
	require.NoError(t, err)

	fileResp, resp := th.Client.UploadFile(sent, th.BasicChannel.Id, "test.png")
	CheckNoError(t, resp)

	fileResp, resp = th.Client.UploadFile(sent, th.BasicChannel.Id, "test.png")
	CheckNoError(t, resp)

	// Wait a bit for files to ready
	time.Sleep(3 * time.Second)

	info, resp := th.WClient.GetFileInfos()
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

	th.Client.Logout()
	_, resp = th.WClient.GetFileInfos()
	CheckUnauthorizedStatus(t, resp)

	otherUser := th.CreateUser()
	th.Client.Login(otherUser.Email, otherUser.Password)
	info, _ = th.WClient.GetFileInfos()
	assert.Empty(t, info)

	th.Client.Logout()
	_, resp = th.WSystemAdminClient.GetFileInfos()
	CheckNoError(t, resp)
}
