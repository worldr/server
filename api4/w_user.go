package api4

import (
	"net/http"
	"time"

	"github.com/mattermost/mattermost-server/v5/app"
	"github.com/mattermost/mattermost-server/v5/audit"
	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/utils"
)

func (api *API) InitWUser() {
	// Valid session IS NOT required
	api.BaseRoutes.WUsers.Handle("/login", api.ApiHandler(loginWrapper)).Methods("POST")
	api.BaseRoutes.WUsers.Handle("/logout", api.ApiHandler(logout)).Methods("POST")

	// Valid session is required
	api.BaseRoutes.WPosts.Handle("/sessions/device", api.ApiSessionRequired(attachDevice)).Methods("PUT")
	api.BaseRoutes.WPosts.Handle("/sessions/device", api.ApiSessionRequired(detachDevice)).Methods("DELETE")
}

func loginWrapper(c *Context, w http.ResponseWriter, r *http.Request) {
	if user, err := executeLogin(c, w, r); err != nil {
		c.Err = err
	} else {
		response := model.LoginResponseWrapper{
			User:  user,
			Token: w.Header().Get("Token"),
		}
		w.Write([]byte(response.ToJson()))
	}
}

func attachDevice(c *Context, w http.ResponseWriter, r *http.Request) {
	device := model.DeviceFromJson(r.Body)

	if len(device.DeviceId) == 0 {
		c.SetInvalidParam("device_id")
		return
	}
	if len(device.PushToken) == 0 {
		c.SetInvalidParam("push_token")
		return
	}

	auditRec := c.MakeAuditRecord("attachDevice", audit.Fail)
	defer c.LogAuditRec(auditRec)
	auditRec.AddMeta("platform", device.Platform)
	auditRec.AddMeta("device_id", device.DeviceId)
	auditRec.AddMeta("push_token", device.PushToken)

	// A special case where we logout of all other sessions with the same device id
	if err := c.App.RevokeSessionsForDevice(c.App.Session().UserId, device, c.App.Session().Id); err != nil {
		c.Err = err
		return
	}

	c.App.ClearSessionCacheForUser(c.App.Session().UserId)
	c.App.Session().SetExpireInDays(*c.App.Config().ServiceSettings.SessionLengthMobileInDays)

	maxAge := *c.App.Config().ServiceSettings.SessionLengthMobileInDays * 60 * 60 * 24

	secure := false
	if app.GetProtocol(r) == "https" {
		secure = true
	}

	subpath, _ := utils.GetSubpathFromConfig(c.App.Config())

	expiresAt := time.Unix(model.GetMillis()/1000+int64(maxAge), 0)
	sessionCookie := &http.Cookie{
		Name:     model.SESSION_COOKIE_TOKEN,
		Value:    c.App.Session().Token,
		Path:     subpath,
		MaxAge:   maxAge,
		Expires:  expiresAt,
		HttpOnly: true,
		Domain:   c.App.GetCookieDomain(),
		Secure:   secure,
	}

	http.SetCookie(w, sessionCookie)

	if err := c.App.AttachDevice(c.App.Session().Id, device, c.App.Session().ExpiresAt); err != nil {
		c.Err = err
		return
	}

	auditRec.Success()
	c.LogAudit("")

	ReturnStatusOK(w)
}

func detachDevice(c *Context, w http.ResponseWriter, r *http.Request) {

}
