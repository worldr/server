package api4

import (
	"net/http"

	"github.com/mattermost/mattermost-server/v5/model"
)

func (api *API) InitWUser() {
	api.BaseRoutes.WUsers.Handle("/login", api.ApiHandler(loginWrapper)).Methods("POST")
	api.BaseRoutes.WUsers.Handle("/logout", api.ApiHandler(logout)).Methods("POST")
}

func loginWrapper(c *Context, w http.ResponseWriter, r *http.Request) {
	if user, err := executeLogin(c, w, r); err != nil {
		c.Err = err
	} else {
		response := model.UserResponseWrapper{Content: user}
		w.Write([]byte(response.ToJson()))
	}
}
