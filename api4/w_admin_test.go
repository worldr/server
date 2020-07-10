// Copyright (c) 2020-present Worldr, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api4

import (
	"strconv"
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
