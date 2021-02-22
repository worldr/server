package api4

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/icrowley/fake"
	"github.com/mattermost/mattermost-server/v5/audit"
	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/store/sqlstore"
	"github.com/mattermost/mattermost-server/v5/utils"
)

func (api *API) InitWAdmin() {
	// Valid session IS NOT required
	api.BaseRoutes.WAdmin.Handle("/token", api.ApiHandler(isAdminTokenValid)).Methods("POST")
	api.BaseRoutes.WAdmin.Handle("/login", api.ApiHandler(loginByAdmin)).Methods("POST")
	api.BaseRoutes.WAdmin.Handle("/setup", api.ApiHandler(getSetupStatus)).Methods("GET")
	api.BaseRoutes.WAdmin.Handle("/setup/admin", api.ApiHandler(createInitialAdmin)).Methods("POST")

	// Valid session is required
	api.BaseRoutes.WAdmin.Handle("/logout", api.ApiSessionRequired(logout)).Methods("POST")
	api.BaseRoutes.WAdmin.Handle("/users", api.ApiSessionRequired(getAllUsersByAdmin)).Methods("GET")
	api.BaseRoutes.WAdmin.Handle("/users/onboard/emails", api.ApiSessionRequired(registerUsersWithEmails)).Methods("POST")
	api.BaseRoutes.WAdmin.Handle("/admins", api.ApiSessionRequired(getAllAdmins)).Methods("GET")
	api.BaseRoutes.WAdmin.Handle("/user/{user_id:[A-Za-z0-9]+}", api.ApiSessionRequired(getUser)).Methods("GET")
	api.BaseRoutes.WAdmin.Handle("/user/{user_id:[A-Za-z0-9]+}/roles", api.ApiSessionRequired(changeUserRolesByAdmin)).Methods("PUT")
	api.BaseRoutes.WAdmin.Handle("/user/{user_id:[A-Za-z0-9]+}/password", api.ApiSessionRequired(changeUserPasswordByAdmin)).Methods("PUT")
	api.BaseRoutes.WAdmin.Handle("/user/create", api.ApiSessionRequired(createUserByAdmin)).Methods("POST")
	api.BaseRoutes.WAdmin.Handle("/user/{user_id:[A-Za-z0-9]+}/active", api.ApiSessionRequired(updateUserActiveByAdmin)).Methods("POST")
	api.BaseRoutes.WAdmin.Handle("/user/{user_id:[A-Za-z0-9]+}/sessions", api.ApiSessionRequired(getUserSessionsByAdmin)).Methods("GET")
	api.BaseRoutes.WAdmin.Handle("/user/{user_id:[A-Za-z0-9]+}/sessions/revoke", api.ApiSessionRequired(revokeAllUserSessionsByAdmin)).Methods("POST")
	api.BaseRoutes.WAdmin.Handle("/user/{user_id:[A-Za-z0-9]+}/session/revoke", api.ApiSessionRequired(revokeSessionByAdmin)).Methods("POST")

	api.BaseRoutes.WAdmin.Handle("/config", api.ApiSessionRequired(getConfigurableValues)).Methods("GET")
	api.BaseRoutes.WAdmin.Handle("/config", api.ApiSessionRequired(setConfigurableValues)).Methods("PUT")
}

// Checks and side-effects context with appropriate error if the check is negative.
func isSystemAdmin(c *Context, where, errorId string) bool {
	if c.IsSystemAdmin() {
		return true
	}
	c.Err = model.NewAppError(where, errorId, nil, "user is not an administrator", http.StatusForbidden)
	return false
}

func getTeamAndAdmin(c *Context) (*model.Team, *model.User, int64, *model.AppError) {
	team, err := c.App.MainTeam()
	if err != nil {
		auditRec1 := c.MakeAuditRecord("createMainTeam", audit.Attempt)
		defer c.LogAuditRec(auditRec1)
		team, err = c.App.CreateTeam(&model.Team{
			DisplayName: sqlstore.MAIN_TEAM_NAME,
			Name:        sqlstore.MAIN_TEAM_NAME,
			Type:        model.TEAM_INVITE,
		})
		var auditRec2 *audit.Record
		if err != nil {
			auditRec2 = c.MakeAuditRecord("createMainTeam", audit.Fail)
		} else {
			auditRec2 = c.MakeAuditRecord("createMainTeam", audit.Success)
		}
		defer c.LogAuditRec(auditRec2)
	}

	if err != nil {
		err = model.NewAppError("getTeamAndAdmin", "main_team", nil, err.Error(), http.StatusInternalServerError)
		return nil, nil, 0, err
	}

	userGetOptions := &model.UserGetOptions{
		Role:    model.SYSTEM_ADMIN_ROLE_ID,
		Sort:    "createat",
		Page:    0,
		PerPage: 1,
	}
	profiles, err := c.App.GetUsersPage(userGetOptions, c.IsSystemAdmin())

	userCountOptions := model.UserCountOptions{
		IncludeBotAccounts: true,
		IncludeDeleted:     true,
	}
	totalUsers, _ := c.App.Srv().Store.User().Count(userCountOptions)

	if err != nil || len(profiles) == 0 {
		return team, nil, totalUsers, err
	}

	return team, profiles[0], totalUsers, nil
}

// getSetupStatus() Checks whether the main team is present in the db and
// that it has a system administrator.
func getSetupStatus(c *Context, w http.ResponseWriter, r *http.Request) {
	team, admin, _, err := getTeamAndAdmin(c)
	if err != nil {
		c.Err = err
		return
	}
	response := model.AdminSetupStatus{Team: team != nil, Admin: admin != nil}
	w.Write([]byte(response.ToJson()))
}

func createInitialAdmin(c *Context, w http.ResponseWriter, r *http.Request) {
	team, admin, totalUsers, err := getTeamAndAdmin(c)
	if err != nil || team == nil {
		msg := "main team should be available by the time the initial admin gets created"
		if err != nil {
			msg += ": " + err.Error()
		}
		c.Err = model.NewAppError(
			"createInitialAdmin",
			"main_team.not_available",
			nil,
			msg,
			http.StatusInternalServerError,
		)
		return
	}

	if admin == nil {
		if totalUsers > 0 {
			c.Err = model.NewAppError(
				"createMainAdmin",
				"initial_admin.users_already_present",
				nil,
				"at least one non-administrator user is already present on the server",
				http.StatusInternalServerError,
			)
			return
		}
		auditRec1 := c.MakeAuditRecord("createMainAdmin", audit.Attempt)
		defer c.LogAuditRec(auditRec1)

		user := model.UserFromJson(r.Body)
		if user == nil {
			c.SetInvalidParam("user")
			return
		}
		user.SanitizeInput(c.IsSystemAdmin())
		user.EmailVerified = true

		user.Roles = model.SYSTEM_ADMIN_ROLE_ID + " " + model.SYSTEM_USER_ROLE_ID

		// create the user
		admin, err = c.App.CreateUser(user)
		var auditRec2 *audit.Record
		if err != nil {
			auditRec2 = c.MakeAuditRecord("createMainAdmin", audit.Fail)
		} else {
			auditRec2 = c.MakeAuditRecord("createMainAdmin", audit.Success)
		}
		defer c.LogAuditRec(auditRec2)
	} else {
		err = model.NewAppError(
			"createMainAdmin",
			"initial_admin.already_present",
			nil,
			"at least one administrator is already present on the server",
			http.StatusInternalServerError,
		)
	}
	if err != nil {
		c.Err = err
		return
	}

	// Add to main team
	_, err = c.App.AddTeamMember(team.Id, admin.Id)
	var auditRec3 *audit.Record
	if err != nil {
		auditRec3 = c.MakeAuditRecord("addMainAdminToTeam", audit.Fail)
	} else {
		auditRec3 = c.MakeAuditRecord("addMainAdminToTeam", audit.Success)
	}
	defer c.LogAuditRec(auditRec3)

	if err != nil {
		c.Err = err
		return
	}

	// Success
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(admin.ToJson()))
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
		c.Err = model.NewAppError("loginForAdmins", "admin_check_role", nil, err.Error(), err.StatusCode)
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
	user.EmailVerified = true

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
	if executeUpdatePassword(c, r) {
		revokeAllSessionsForUser(c, w, r)
	}
}

func updateUserActiveByAdmin(c *Context, w http.ResponseWriter, r *http.Request) {
	if !isSystemAdmin(c, "updateUserActive", "admin_change_user_active") {
		return
	}
	updateUserActive(c, w, r)
}

func revokeAllUserSessionsByAdmin(c *Context, w http.ResponseWriter, r *http.Request) {
	if !isSystemAdmin(c, "revokeAllUserSessions", "admin_revoke_all_user_sessions") {
		return
	}
	revokeAllSessionsForUser(c, w, r)
}

func revokeSessionByAdmin(c *Context, w http.ResponseWriter, r *http.Request) {
	if !isSystemAdmin(c, "revokeSession", "admin_revoke_session") {
		return
	}
	revokeSession(c, w, r)
}

func getUserSessionsByAdmin(c *Context, w http.ResponseWriter, r *http.Request) {
	if !isSystemAdmin(c, "getUserSessions", "admin_get_sessions") {
		return
	}
	getSessions(c, w, r)
}

func registerUsersWithEmails(c *Context, w http.ResponseWriter, r *http.Request) {
	if !isSystemAdmin(c, "registerUsersWithEmails", "admin_register_emails") {
		return
	}
	team, err1 := c.App.MainTeam()
	if err1 != nil {
		c.Err = err1
		return
	}

	emails := model.ArrayFromJson(r.Body)
	if len(emails) == 0 {
		c.Err = model.NewAppError("registerUsersWithEmails", "admin_register_emails", nil, "no emails to register", http.StatusBadRequest)
		return
	}

	successes := make([]*model.User, 0, len(emails))
	failures := make(map[string]string, len(emails))

	regName := regexp.MustCompile(`[._-]`)

	uids := make([]string, 0, len(emails))

	for _, email := range emails {
		email = strings.ToLower(strings.Trim(email, ", \n\t\r"))
		if !model.IsValidEmailAddress(email) {
			failures[email] = "Email is invalid"
			continue
		}

		username := strings.Split(email, "@")[0]
		if !model.IsValidUsername(username) {
			failures[email] = "Unable to use email part before @ as a username"
			continue
		}
		firstLast := regName.Split(username, -1)
		first, last := firstLast[0], ""
		first = fmt.Sprintf("%v%v", strings.ToUpper(first[0:1]), first[1:])
		if len(firstLast) > 1 {
			for i := 1; i < len(firstLast); i++ {
				firstLast[i] = fmt.Sprintf("%v%v", strings.ToUpper(firstLast[i][0:1]), firstLast[i][1:])
			}
			last = strings.Join(firstLast[1:], " ")
		}

		password := fmt.Sprintf("Worldr-%v", fake.CharactersN(5))
		user := model.User{
			Username:      username,
			FirstName:     first,
			LastName:      last,
			Email:         email,
			Password:      password,
			EmailVerified: true,
		}

		ruser, err := executeCreateUser(c, &user, "", "")
		if err != nil {
			failures[email] = fmt.Sprintf("%v", err.Message)
		} else {
			successes = append(successes, ruser)
			// This method of registration returns the password to the caller.
			// This may change in the future.
			ruser.Password = password
			uids = append(uids, ruser.Id)
		}
	}

	// Automatically add to main team
	_, err := c.App.AddTeamMembers(team.Id, uids, c.App.Session().UserId, false)
	if err != nil {
		c.Err = err
		return
	}

	response := model.RegisterEmailsResponse{
		Successes: successes,
		Failures:  failures,
	}
	w.Write([]byte(response.ToJson()))
}

func getConfigurableValues(c *Context, w http.ResponseWriter, r *http.Request) {
	if !isSystemAdmin(c, "getConfigurableValues", "admin_get_configurable") {
		return
	}

	local := c.App.Config().EmailSettings
	// Don't give out the password. If it's set, send a * placeholder,
	// if it's not set, send an empty string.
	var pass string
	if local.SMTPPassword != nil && len(*local.SMTPPassword) > 0 {
		pass = "*"
	} else {
		pass = ""
	}
	exposed := &model.EmailSettingsExposed{
		EmailAddress:       local.FeedbackEmail,
		EmailFrom:          local.FeedbackName,
		SMTPServer:         local.SMTPServer,
		SMTPPort:           local.SMTPPort,
		ConnectionSecurity: local.ConnectionSecurity,
		VerifyCertificate:  model.NewBool(!*local.SkipServerCertificateVerification),
		EnableSMTPAuth:     local.EnableSMTPAuth,
		SMTPUsername:       local.SMTPUsername,
		SMTPPassword:       model.NewString(pass),
	}

	response := model.Configurable{
		Email: exposed,
	}
	w.Write([]byte(response.ToJson()))
}

func setConfigurableValues(c *Context, w http.ResponseWriter, r *http.Request) {
	if !isSystemAdmin(c, "setConfigurableValues", "admin_set_configurable") {
		return
	}

	local := c.App.Config()
	incoming, err := model.ConfigurableFromJson(r.Body)
	if err != nil {
		c.Err = model.NewAppError(
			"setConfigurableValues",
			"admin_set_configurable_request",
			nil,
			err.Error(),
			http.StatusBadRequest,
		)
		return
	}

	auditRec := c.MakeAuditRecord("patchConfig", audit.Fail)
	defer c.LogAuditRec(auditRec)

	// If a new password has been set, replace it in the config.
	// If it hasn't been set, leave the old one.
	var pass *string
	if incoming.Email.SMTPPassword != nil && len(*incoming.Email.SMTPPassword) > 0 {
		pass = incoming.Email.SMTPPassword
	} else {
		pass = local.EmailSettings.SMTPPassword
	}
	var verify bool
	if incoming.Email.VerifyCertificate != nil {
		verify = *incoming.Email.VerifyCertificate
	} else {
		verify = false
	}
	patch := &model.Config{
		EmailSettings: model.EmailSettings{
			FeedbackEmail:                     incoming.Email.EmailAddress,
			ReplyToAddress:                    incoming.Email.EmailAddress,
			FeedbackName:                      incoming.Email.EmailFrom,
			SMTPServer:                        incoming.Email.SMTPServer,
			SMTPPort:                          incoming.Email.SMTPPort,
			ConnectionSecurity:                incoming.Email.ConnectionSecurity,
			SkipServerCertificateVerification: model.NewBool(!verify),
			EnableSMTPAuth:                    incoming.Email.EnableSMTPAuth,
			SMTPUsername:                      incoming.Email.SMTPUsername,
			SMTPPassword:                      pass,
		},
	}

	merged, mergeErr := utils.Merge(local, patch, nil)
	if mergeErr != nil {
		c.Err = model.NewAppError(
			"setConfigurableValues",
			"admin_set_configurable_merge",
			nil,
			mergeErr.Error(),
			http.StatusInternalServerError,
		)
		return
	}
	updated := merged.(model.Config)

	errValid := updated.IsValid()
	if errValid != nil {
		c.Err = errValid
		return
	}

	errSave := c.App.SaveConfig(&updated, true)
	if errSave != nil {
		c.Err = errSave
		return
	}

	auditRec.Success()

	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	getConfigurableValues(c, w, r)
}
