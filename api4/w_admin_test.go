// Copyright (c) 2020-present Worldr, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api4

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/stretchr/testify/assert"
)

// Login as administrator, checking privileges
func TestLoginByAdmin(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	th.CreateMainTeam()

	th.SystemAdminClient.Logout()

	adminSessionTtlHours := 1

	th.App.UpdateConfig(func(cfg *model.Config) {
		*cfg.ServiceSettings.SessionLengthAdminToolInHours = adminSessionTtlHours
	})

	t.Run("valid login", func(t *testing.T) {
		response, r := th.WSystemAdminClient.LoginByAdmin(th.SystemAdminUser.Username, th.SystemAdminUser.Password)
		CheckNoError(t, r)
		// check that we are logged in
		assert.Equal(t, th.SystemAdminUser.Id, response.User.Id)
		assert.True(t, len(response.Token) > 0)
		exp, errConv := strconv.ParseInt(response.ExpiresAt, 10, 64)
		assert.Nil(t, errConv)
		// check that we have a specific ttl
		ttl := (exp - time.Now().UnixNano()/1000000) / time.Minute.Milliseconds()
		assert.LessOrEqual(t, ttl, int64(adminSessionTtlHours*60))
	})

	t.Run("not an admin", func(t *testing.T) {
		_, r := th.WSystemAdminClient.LoginByAdmin(th.BasicUser.Username, th.BasicUser.Password)
		assert.NotNil(t, r.Error)
		assert.Equal(t, "admin_check_role", r.Error.Id)
	})
}

func TestSetupSystem1(t *testing.T) {
	th := Setup(t)
	defer th.TearDown()

	t.Run("empty server", func(t *testing.T) {
		response, r := th.WSystemAdminClient.Setup()
		CheckNoError(t, r)
		// check that we have the main team and we don't have an administrator
		assert.True(t, response.Team)
		assert.False(t, response.Admin)
	})
}

func TestSetupSystem2(t *testing.T) {
	th := Setup(t)
	defer th.TearDown()

	t.Run("create twice", func(t *testing.T) {
		response, r := th.WSystemAdminClient.Setup()
		CheckNoError(t, r)
		response, r = th.WSystemAdminClient.Setup()
		CheckNoError(t, r)
		// the main team must have been already created
		assert.True(t, response.Team)
		assert.False(t, response.Admin)
	})
}

var initialAdmin = &model.User{
	Id:       "",
	Username: "worldr",
	Password: "0123456789",
	Email:    "a@b.c",
}

var initialAdminDup = &model.User{
	Id:       "",
	Username: "worldr1",
	Password: "0123456789",
	Email:    "a1@b.c",
}

func TestCreateInitialAdmin1(t *testing.T) {
	th := Setup(t)
	th.waitForConnectivity()
	defer th.TearDown()

	t.Run("create successfully", func(t *testing.T) {
		// initial admin
		response, r := th.WSystemAdminClient.CreateInitialAdmin(initialAdmin)
		CheckNoError(t, r)
		assert.Equal(t, "worldr", response.Username)
		assert.True(t, strings.Contains(response.Roles, "system_admin"))
	})
}

func TestCreateInitialAdmin2(t *testing.T) {
	th := Setup(t)
	th.waitForConnectivity()
	defer th.TearDown()

	t.Run("create fail", func(t *testing.T) {
		// initial admin
		_, r := th.WSystemAdminClient.CreateInitialAdmin(initialAdmin)
		CheckNoError(t, r)
		// try to create initial admin again
		_, r = th.WSystemAdminClient.CreateInitialAdmin(initialAdminDup)
		assert.NotNil(t, r.Error)
		assert.Equal(t, "initial_admin.already_present", r.Error.Id)
	})
}

func TestActivateUser(t *testing.T) {
	th := Setup(t).InitBasic()
	th.waitForConnectivity()
	defer th.TearDown()

	t.Run("activate", func(t *testing.T) {
		// Try for non-admin
		r := th.WClient.UpdateUserActive(th.BasicUser.Id, false)
		CheckForbiddenStatus(t, r)
		// Deactivate
		r = th.WSystemAdminClient.UpdateUserActive(th.BasicUser.Id, false)
		CheckNoError(t, r)
		// Activate back
		r = th.WSystemAdminClient.UpdateUserActive(th.BasicUser.Id, true)
		CheckNoError(t, r)
	})
}

func TestAdminPermissions(t *testing.T) {
	th := Setup(t).InitBasic()
	th.waitForConnectivity()
	defer th.TearDown()

	t.Run("permissions", func(t *testing.T) {
		r := th.WClient.RevokeAllUserSessions(th.BasicUser.Id)
		CheckForbiddenStatus(t, r)
		_, r = th.WClient.GetSessions(th.BasicUser.Id)
		CheckForbiddenStatus(t, r)
	})
}

func TestGetSessionsByAdmin(t *testing.T) {
	th := Setup(t).InitBasic()
	th.waitForConnectivity()
	defer th.TearDown()

	t.Run("get sessions", func(t *testing.T) {
		list, r := th.WSystemAdminClient.GetSessions(th.SystemAdminUser.Id)
		CheckNoError(t, r)
		assert.NotNil(t, list, "list is nil")
		assert.True(t, len(list) > 0, "list is empty")
	})
}

func TestRevokeSession(t *testing.T) {
	th := Setup(t).InitBasic()
	th.waitForConnectivity()
	defer th.TearDown()

	t.Run("get and revoke session", func(t *testing.T) {
		list, r := th.WSystemAdminClient.GetSessions(th.BasicUser.Id)
		CheckNoError(t, r)
		assert.NotNil(t, list, "list is nil before revoke")
		assert.True(t, len(list) > 0, "list is empty before revoke")

		sessionId := list[0].Id

		r = th.WClient.RevokeSession(th.BasicUser.Id, sessionId)
		CheckForbiddenStatus(t, r)
		r = th.WSystemAdminClient.RevokeSession(th.BasicUser.Id, sessionId)
		CheckNoError(t, r)

		list, r = th.WSystemAdminClient.GetSessions(th.BasicUser.Id)
		CheckNoError(t, r)
		assert.NotNil(t, list, "list is nil after revoke")
		for _, v := range list {
			assert.NotEqual(t, sessionId, v.Id)
		}
	})
}

func TestRevokeAllSessionsByAdmin(t *testing.T) {
	th := Setup(t).InitBasic()
	th.waitForConnectivity()
	defer th.TearDown()

	t.Run("get and revoke all sessions", func(t *testing.T) {
		list, r := th.WSystemAdminClient.GetSessions(th.BasicUser.Id)
		CheckNoError(t, r)
		assert.NotNil(t, list, "list is nil before revoke")
		assert.True(t, len(list) > 0, "list is empty before revoke")

		r = th.WClient.RevokeAllUserSessions(th.BasicUser.Id)
		CheckForbiddenStatus(t, r)
		r = th.WSystemAdminClient.RevokeAllUserSessions(th.BasicUser.Id)
		CheckNoError(t, r)

		list, r = th.WSystemAdminClient.GetSessions(th.BasicUser.Id)
		CheckNoError(t, r)
		assert.NotNil(t, list, "list is nil after revoke")
		assert.Equal(t, 0, len(list), "list is expected to be empty")
	})
}
