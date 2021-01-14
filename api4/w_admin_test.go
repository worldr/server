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
		assert.Equal(t, initialAdmin.Username, response.Username)
		assert.True(t, strings.Contains(response.Roles, model.SYSTEM_ADMIN_ROLE_ID))
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

func TestRegisterUsersWithEmails(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	type userChecker func(i int, user *model.User)

	checkSuccess := func(emails []string, checkUser userChecker) {
		result, r := th.WSystemAdminClient.RegisterUsersWithEmails(emails)
		CheckNoError(t, r)
		assert.NotNil(t, result, "response says ok, but result data is nil")
		assert.Equal(t, 0, len(result.Failures), "no failures are expected")
		assert.Equal(t, len(emails), len(result.Successes), "not all emails were successfully registered")
		for i, v := range result.Successes {
			assert.Equal(t, emails[i], v.Email, "registered email doesn't match")
			assert.True(t, len(v.Password) > 0, "this form of registration is expected to return passwords")
			checkUser(i, v)
		}
	}

	checkFailure := func(emails []string, failures int, successes int) {
		result, r := th.WSystemAdminClient.RegisterUsersWithEmails(emails)
		CheckNoError(t, r)
		assert.NotNil(t, result, "response says ok, but result data is nil")
		assert.Equal(t, failures, len(result.Failures), "unexpected number of failures")
		assert.Equal(t, successes, len(result.Successes), "unexpected number of successes")
	}

	t.Run("register users with emails – successes", func(t *testing.T) {
		checkSuccess(
			[]string{"a1@example.com"},
			func(i int, user *model.User) {
				assert.Equal(t, "A1", user.FirstName, "unexpected first name")
				assert.Equal(t, "", user.LastName, "unexpected last name")
			},
		)
		checkSuccess(
			[]string{"a2@example.com", "a.b@example.com", "c_d@example.com", "e-f@example.com", "a.b_c@example.com", "c_d.e@example.com", "e-f-g@example.com", "ccc_ddd.eee@example.com", "eee-fff_ggg@example.com"},
			func(i int, user *model.User) {
				switch i {
				case 0:
					assert.Equal(t, "A2", user.FirstName, "unexpected first name")
					assert.Equal(t, "", user.LastName, "unexpected last name")
				case 1:
					assert.Equal(t, "A", user.FirstName, "unexpected first name")
					assert.Equal(t, "B", user.LastName, "unexpected last name")
				case 2:
					assert.Equal(t, "C", user.FirstName, "unexpected first name")
					assert.Equal(t, "D", user.LastName, "unexpected last name")
				case 3:
					assert.Equal(t, "E", user.FirstName, "unexpected first name")
					assert.Equal(t, "F", user.LastName, "unexpected last name")
				case 4:
					assert.Equal(t, "A", user.FirstName, "unexpected first name")
					assert.Equal(t, "B C", user.LastName, "unexpected last name")
				case 5:
					assert.Equal(t, "C", user.FirstName, "unexpected first name")
					assert.Equal(t, "D E", user.LastName, "unexpected last name")
				case 6:
					assert.Equal(t, "E", user.FirstName, "unexpected first name")
					assert.Equal(t, "F G", user.LastName, "unexpected last name")
				case 7:
					assert.Equal(t, "Ccc", user.FirstName, "unexpected first name")
					assert.Equal(t, "Ddd Eee", user.LastName, "unexpected last name")
				case 8:
					assert.Equal(t, "Eee", user.FirstName, "unexpected first name")
					assert.Equal(t, "Fff Ggg", user.LastName, "unexpected last name")
				}
			},
		)
	})
	t.Run("register users with emails – failures", func(t *testing.T) {
		checkFailure([]string{"x@example"}, 1, 0)
		checkFailure([]string{"y_example.com"}, 1, 0)
		checkFailure([]string{"x@example.com", "y@example.com", "y@example"}, 1, 2)
		checkFailure([]string{"x@example.com", "y@example.com"}, 2, 0)
	})
}
