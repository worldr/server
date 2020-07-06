package api4

import (
	"net/http"
	"strconv"

	"github.com/mattermost/mattermost-server/v5/model"
)

func (api *API) InitWAdmin() {
	// Valid session IS NOT required
	api.BaseRoutes.WAdmin.Handle("/token", api.ApiHandler(isAdminTokenValid)).Methods("POST")
	api.BaseRoutes.WAdmin.Handle("/login", api.ApiHandler(loginByAdmin)).Methods("POST")

	// Valid session is required
	api.BaseRoutes.WAdmin.Handle("/logout", api.ApiSessionRequired(logout)).Methods("POST")
	api.BaseRoutes.WAdmin.Handle("/users", api.ApiSessionRequired(getAllUsersByAdmin)).Methods("GET")
	api.BaseRoutes.WAdmin.Handle("/admins", api.ApiSessionRequired(getAllAdmins)).Methods("GET")
	api.BaseRoutes.WAdmin.Handle("/user/{user_id:[A-Za-z0-9]+}", api.ApiSessionRequired(getUser)).Methods("GET")
	api.BaseRoutes.WAdmin.Handle("/user/{user_id:[A-Za-z0-9]+}/roles", api.ApiSessionRequired(changeUserRolesByAdmin)).Methods("PUT")
	api.BaseRoutes.WAdmin.Handle("/user/{user_id:[A-Za-z0-9]+}/password", api.ApiSessionRequired(changeUserPasswordByAdmin)).Methods("PUT")
	api.BaseRoutes.WAdmin.Handle("/user/create", api.ApiSessionRequired(createUserByAdmin)).Methods("POST")
}

// Checks and side-effects context with appropriate error if the check is negative.
func isSystemAdmin(c *Context, where, errorId string) bool {
	if c.IsSystemAdmin() {
		return true
	}
	c.Err = model.NewAppError(where, errorId, nil, "user is not an administrator", http.StatusForbidden)
	return false
}

// Checks user id and device id first. If no such combination is found
// in sessions – just respond negative. If the combination exists, checks the token.
// If the latter exists and is equal to the supplied, returns true and expiration time.
func isAdminTokenValid(c *Context, w http.ResponseWriter, r *http.Request) {
	props := model.MapFromJson(r.Body)

	userId, exists := props["user_id"]
	if !exists || len(userId) == 0 {
		c.Err = model.NewAppError("isTokenValid", "admin_token_valid", nil, "invalid paramter", http.StatusBadRequest)
		return
	}
	deviceId, exists := props["device_id"]
	if !exists || len(deviceId) == 0 {
		c.Err = model.NewAppError("isTokenValid", "admin_token_valid", nil, "invalid paramter", http.StatusBadRequest)
		return
	}
	token, exists := props["token"]
	if !exists || len(token) == 0 {
		c.Err = model.NewAppError("isTokenValid", "admin_token_valid", nil, "invalid paramter", http.StatusBadRequest)
		return
	}
	sessions, err := c.App.Srv().Store.Session().GetSessionsWithDeviceId(userId, deviceId)
	if err != nil {
		c.Err = model.NewAppError("isTokenValid", "admin_token_valid", nil, err.Error(), http.StatusInternalServerError)
		return
	}
	response := model.AdminTokenCheck{}
	if len(sessions) == 1 {
		response.Valid = sessions[0].Token == token
		response.ExpiresAt = strconv.FormatInt(sessions[0].ExpiresAt, 10)
		c.App.AttachSessionCookies(w, r)
	} else {
		response.Valid = false
	}
	w.Write([]byte(response.ToJson()))
}

// Only allows users with system_admin role to log in. The TTL of created session
// is different from regular log in procedure and is much shorte: see ServiceSettings.SessionLengthAdminToolInHours.
func loginByAdmin(c *Context, w http.ResponseWriter, r *http.Request) {
	team, err := c.App.MainTeam()
	if err != nil {
		c.Err = model.NewAppError("loginForAdmins", "admin_get_main_team", nil, err.Error(), http.StatusInternalServerError)
		return
	}

	props := model.MapFromJson(r.Body)
	loginId, exists := props["login_id"]
	if !exists || len(loginId) == 0 {
		c.Err = model.NewAppError("loginForAdmins", "admin_login_param", nil, "missing login parameter", http.StatusBadRequest)
		return
	}

	isAdmin, err := c.App.IsAdminUsername(team.Id, loginId)

	if err != nil {
		c.Err = model.NewAppError("loginForAdmins", "admin_check_role", nil, err.Error(), http.StatusInternalServerError)
		return
	}
	if !isAdmin {
		c.Err = model.NewAppError("loginForAdmins", "admin_check_role", nil, "user is not an administrator", http.StatusForbidden)
		return
	}

	props[model.USE_ADMIN_SESSION_TTL] = "true"

	if user, err := executeLoginWithProps(c, w, r, props); err == nil {
		c.App.AttachSessionCookies(w, r)
		response := model.AdminAuthResponse{
			Token:     w.Header().Get("Token"),
			ExpiresAt: strconv.FormatInt(c.App.Session().ExpiresAt, 10),
			User:      user,
		}
		w.Write([]byte(response.ToJson()))
	}
}

// Get full list of admins, no need to check additional permissions as this is intended for use by system admins.
func getAllAdmins(c *Context, w http.ResponseWriter, r *http.Request) {
	if !isSystemAdmin(c, "getAllAdmins", "admin_get_all_admins") {
		return
	}

	userGetOptions := &model.UserGetOptions{
		Role:    model.SYSTEM_ADMIN_ROLE_ID,
		Sort:    "username",
		Page:    0,
		PerPage: 100,
	}

	var profiles []*model.User
	etag := ""

	profiles, err := c.App.GetUsersPage(userGetOptions, c.IsSystemAdmin())
	if err != nil {
		c.Err = err
		return
	}

	if len(etag) > 0 {
		w.Header().Set(model.HEADER_ETAG_SERVER, etag)
	}
	c.App.UpdateLastActivityAtIfNeeded(*c.App.Session())
	w.Write([]byte(model.UserListToJson(profiles)))
}

// Paginated users list, short form of User structure.
func getAllUsersByAdmin(c *Context, w http.ResponseWriter, r *http.Request) {
	if !isSystemAdmin(c, "getAllUsers", "admin_get_all_users_role") {
		return
	}

	from, err := strconv.Atoi(r.URL.Query().Get("from"))
	if err != nil {
		c.Err = model.NewAppError("getAllUsers", "admin_get_all_users_param", nil, "invalid parameter from", http.StatusBadRequest)
		return
	}
	perPage, err := strconv.Atoi(r.URL.Query().Get("per_page"))
	if err != nil {
		c.Err = model.NewAppError("getAllUsers", "admin_get_all_users_param", nil, "invalid parameter per_page", http.StatusBadRequest)
		return
	}

	users, total, err1 := c.App.GetAllUsersPaginated(uint64(from), uint64(perPage))

	if err1 != nil {
		c.Err = model.NewAppError("getAllUsers", "admin_get_all_users_db", nil, err1.Error(), http.StatusBadRequest)
		return
	}
	for _, v := range users {
		v.Sanitize(map[string]bool{})
	}

	response := model.AdminUsersPage{
		Users:   &users,
		Total:   total,
		From:    uint64(from),
		PerPage: uint64(perPage),
	}
	w.Write([]byte(response.ToJson()))
}

// Creates a user and add it to the main team.
func createUserByAdmin(c *Context, w http.ResponseWriter, r *http.Request) {
	if !isSystemAdmin(c, "createUser", "admin_create_user") {
		return
	}

	team, err := c.App.MainTeam()
	if err != nil {
		c.Err = err
		return
	}

	user := model.UserFromJson(r.Body)
	if user == nil {
		c.SetInvalidParam("user")
		return
	}
	user.SanitizeInput(c.IsSystemAdmin())

	// Create user, but do not write a response
	ruser, err := executeCreateUser(c, user, "", "")
	if err != nil {
		c.Err = err
		return
	}

	// Automatically add to main team
	_, err = c.App.AddTeamMember(team.Id, user.Id)
	if err != nil {
		c.Err = err
		return
	}

	// Success
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(ruser.ToJson()))
}

func changeUserRolesByAdmin(c *Context, w http.ResponseWriter, r *http.Request) {
	if !isSystemAdmin(c, "changeUserRoles", "admin_change_user_roles") {
		return
	}
	if c.Params.UserId == model.ME || c.Params.UserId == c.App.Session().UserId {
		c.Err = model.NewAppError("changeUserRoles", "admin_change_user_roles.self", nil, "can't change your own roles", http.StatusForbidden)
		return
	}
	updateUserRoles(c, w, r)
}

func changeUserPasswordByAdmin(c *Context, w http.ResponseWriter, r *http.Request) {
	if !isSystemAdmin(c, "changeUserPassword", "admin_change_user_password") {
		return
	}
	updatePassword(c, w, r)
}
