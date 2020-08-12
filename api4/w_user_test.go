package api4

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoginW(t *testing.T) {
	th := Setup(t).InitBasic()
	defer th.TearDown()

	th.CreateMainTeam()

	th.Client.Logout()

	t.Run("valid login", func(t *testing.T) {
		response, r := th.WClient.Login(th.BasicUser.Username, th.BasicUser.Password)
		CheckNoError(t, r)
		assert.Equal(t, response.User.Id, th.BasicUser.Id)
		assert.True(t, len(response.Token) > 0)
	})

	t.Run("invalid password", func(t *testing.T) {
		_, r := th.WClient.Login(th.BasicUser.Username, th.BasicUser.Password+"1")
		assert.NotNil(t, r.Error)
		assert.Equal(t, "api.user.check_user_password.invalid.app_error", r.Error.Id)
	})

	t.Run("invalid username", func(t *testing.T) {
		_, r := th.WClient.Login(th.BasicUser.Username+"1", th.BasicUser.Password)
		assert.NotNil(t, r.Error)
		assert.Equal(t, "store.sql_user.get_for_login.app_error", r.Error.Id)
	})
}
