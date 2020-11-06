package model

import (
	"fmt"
	"net/http"
)

const (
	API_URL_SUFFIX_WORLDR_V1 = "/api/worldr/v1"
	API_URL_SUFFIX_WORLDR    = API_URL_SUFFIX_WORLDR_V1
)

// This client is a shim for Worldr API handles.
// It exposes the Worldr API and delegates the actual networking to the Mattermost client.
type WClient struct {
	ApiUrl   string // for example "http://localhost:8065/api/worldr/v1"
	MMClient *Client4
}

// Worldr API client is always paired with Mattermost client.
func NewWorldrAPIClient(url string, mmClient *Client4) *WClient {
	return &WClient{
		ApiUrl:   url + API_URL_SUFFIX_WORLDR,
		MMClient: mmClient,
	}
}

//
// URLS
//

// Get absolute api url of the files handle.
func (c *WClient) GetFilesUrl() string {
	return fmt.Sprintf("%v/files", c.ApiUrl)
}

// Get absolute api url of the login handle.
func (c *WClient) GetLoginUrl() string {
	return fmt.Sprintf("%v/users/login", c.ApiUrl)
}

// Get absolute api url of the recent posts handle.
func (c *WClient) GetRecentUrl() string {
	return fmt.Sprintf("%v/posts/recent", c.ApiUrl)
}

// Get absolute api url of the admin login handle.
func (c *WClient) GetLoginByAdminUrl() string {
	return fmt.Sprintf("%v/admin/login", c.ApiUrl)
}

// Get absolute api url of the admin setup handle.
func (c *WClient) GetSetupUrl() string {
	return fmt.Sprintf("%v/admin/setup", c.ApiUrl)
}

// Get absolute api url of the initial admin creation handle.
func (c *WClient) GetCreateIntialAdminUrl() string {
	return fmt.Sprintf("%v/admin/setup/admin", c.ApiUrl)
}

// Get absolute api url of the posts reactions handle.
func (c *WClient) GetReactionsForPostsUrl() string {
	return fmt.Sprintf("%v/posts/ids/reactions", c.ApiUrl)
}

// Get absolute api url of the channel category assigning handle.
func (c *WClient) GetAssignCategoryUrl() string {
	return fmt.Sprintf("%v/channels/categories/assign", c.ApiUrl)
}

// Get absolute api url of the channel category getting handle.
func (c *WClient) GetChannelCategoriesUrl() string {
	return fmt.Sprintf("%v/channels/categories", c.ApiUrl)
}

// Get absolute api url of the channel category deleting handle.
func (c *WClient) GetRemoveCategoryFromChannelUrl() string {
	return fmt.Sprintf("%v/channels/categories/remove", c.ApiUrl)
}

// Get absolute api url of the channel category reordering handle.
func (c *WClient) GetCategoryReorderUrl() string {
	return fmt.Sprintf("%v/channels/categories/order", c.ApiUrl)
}

// Get absolute api url of the check for updates handle.
func (c *WClient) GetCheckUpdatesUrl() string {
	return fmt.Sprintf("%v/posts/updates", c.ApiUrl)
}

// Get absolute api url of the admin user active update handle.
func (c *WClient) GetUserActiveUrl(userId string) string {
	return fmt.Sprintf("%v/admin/user/%v/active", c.ApiUrl, userId)
}

// Get absolute api url of the admin revoke all sessions handle.
func (c *WClient) RevokeAllUserSessionsUrl(userId string) string {
	return fmt.Sprintf("%v/admin/user/%v/sessions/revoke", c.ApiUrl, userId)
}

// Get absolute api url of the admin get sessions handle.
func (c *WClient) GetSessionsUrl(userId string) string {
	return fmt.Sprintf("%v/admin/user/%v/sessions", c.ApiUrl, userId)
}

// Get absolute api url of the admin revoke all sessions handle.
func (c *WClient) RevokeSessionUrl(userId string) string {
	return fmt.Sprintf("%v/admin/user/%v/session/revoke", c.ApiUrl, userId)
}

//
// METHODS
//

// Creates an administrator user if no administrators are present on the server.
func (c *WClient) CreateInitialAdmin(u *User) (*User, *Response) {
	r, err := c.MMClient.DoApiPostWithUrl(c.GetCreateIntialAdminUrl(), u.ToJson(), false)
	if err != nil {
		return nil, BuildErrorResponse(r, err)
	}
	defer closeBody(r)
	return UserFromJson(r.Body), BuildResponse(r)
}

// Setup checks whether the main team and at least one admin is present on the server.
// If the main team is absent, it gets created.
func (c *WClient) Setup() (*AdminSetupStatus, *Response) {
	r, err := c.MMClient.DoApiGetWithUrl(c.GetSetupUrl(), "", false)
	if err != nil {
		return nil, BuildErrorResponse(r, err)
	}
	defer closeBody(r)
	return AdminSetupStatusFromJson(r.Body), BuildResponse(r)
}

// GetFileInfos gets all the files into objects.
func (c *WClient) GetFileInfos() ([]*FileInfo, *Response) {
	r, err := c.MMClient.DoApiGetWithUrl(c.GetFilesUrl(), "", false)
	if err != nil {
		return nil, BuildErrorResponse(r, err)
	}
	defer closeBody(r)
	return *FileInfoResponseWrapperFromJson(r.Body).Content, BuildResponse(r)
}

// Login normally and pack the response into Worldr response format.
func (c *WClient) Login(loginId string, password string) (*LoginResponseWrapper, *Response) {
	m := make(map[string]string)
	m["login_id"] = loginId
	m["password"] = password
	return c.login(m)
}

// Login normally and pack the response into Worldr response format.
func (c *WClient) LoginByAdmin(loginId string, password string) (*AdminAuthResponse, *Response) {
	m := make(map[string]string)
	m["login_id"] = loginId
	m["password"] = password
	r, err := c.MMClient.DoApiPostWithUrl(c.GetLoginByAdminUrl(), MapToJson(m), false)
	if err != nil {
		return nil, BuildErrorResponse(r, err)
	}
	defer closeBody(r)
	c.MMClient.AuthToken = r.Header.Get(HEADER_TOKEN)
	c.MMClient.AuthType = HEADER_BEARER
	return AdminAuthResponseFromJson(r.Body), BuildResponse(r)
}

func (c *WClient) login(m map[string]string) (*LoginResponseWrapper, *Response) {
	r, err := c.MMClient.DoApiPostWithUrl(c.GetLoginUrl(), MapToJson(m), false)
	if err != nil {
		return nil, BuildErrorResponse(r, err)
	}
	defer closeBody(r)
	c.MMClient.AuthToken = r.Header.Get(HEADER_TOKEN)
	c.MMClient.AuthType = HEADER_BEARER
	return LoginResponseFromJson(r.Body), BuildResponse(r)
}

func (c *WClient) GetRecentPosts(request *RecentPostsRequestData) (*RecentPostsResponseData, *Response) {
	r, err := c.MMClient.DoApiPostWithUrl(c.GetRecentUrl(), request.ToJson(), false)
	if err != nil {
		return nil, BuildErrorResponse(r, err)
	}
	defer closeBody(r)
	return RecentResponseDataFromJson(r.Body), BuildResponse(r)
}

func (c *WClient) GetReactionsForPosts(request []string) (*PostsReactionsResponseWrapper, *Response) {
	r, err := c.MMClient.DoApiPostWithUrl(c.GetReactionsForPostsUrl(), ArrayToJson(request), false)
	if err != nil {
		return nil, BuildErrorResponse(r, err)
	}
	defer closeBody(r)
	return PostsReactionsResponseWrapperFromJson(r.Body), BuildResponse(r)
}

func (c *WClient) AssignChannelCategory(request *ChannelCategory) (*ChannelCategory, *Response) {
	r, err := c.MMClient.DoApiPostWithUrl(c.GetAssignCategoryUrl(), request.ToJson(), false)
	if err != nil {
		return nil, BuildErrorResponse(r, err)
	}
	defer closeBody(r)
	return ChannelCategoryFromJson(r.Body), BuildResponse(r)
}

func (c *WClient) GetChannelCategories() ([]*ChannelCategoryAggregated, *Response) {
	r, err := c.MMClient.DoApiGetWithUrl(c.GetChannelCategoriesUrl(), "", false)
	if err != nil {
		return nil, BuildErrorResponse(r, err)
	}
	defer closeBody(r)
	return ChannelCategoriesAggregatedListFromJson(r.Body), BuildResponse(r)
}

func (c *WClient) RemoveCategoryFromChannel(request map[string]string) *Response {
	r, err := c.MMClient.DoApiPostWithUrl(c.GetRemoveCategoryFromChannelUrl(), MapToJson(request), false)
	if err != nil {
		return BuildErrorResponse(r, err)
	}
	defer closeBody(r)
	return BuildResponse(r)
}

func (c *WClient) ReorderChannelCategory(request map[string]string) *Response {
	r, err := c.MMClient.DoApiPostWithUrl(c.GetCategoryReorderUrl(), MapToJson(request), false)
	if err != nil {
		return BuildErrorResponse(r, err)
	}
	defer closeBody(r)
	return BuildResponse(r)
}

func (c *WClient) CheckUpdates(request []*ChannelWithPost) (*ChannelUpdates, *Response) {
	r, err := c.MMClient.DoApiPostWithUrl(c.GetCheckUpdatesUrl(), ChannelWithPostListToJson(request), false)
	if err != nil {
		return nil, BuildErrorResponse(r, err)
	}
	defer closeBody(r)
	if result, err := ChannelUpdatesFromJson(r.Body); err == nil {
		return result, BuildResponse(r)
	} else {
		return nil, BuildErrorResponse(r, NewAppError("CheckUpdates", "WClient.CheckUpdates", nil, "Failed to unmarshall response JSON", http.StatusInternalServerError))
	}
}

func (c *WClient) UpdateUserActive(userId string, status bool) *Response {
	request := map[string]bool{
		"active": status,
	}
	r, err := c.MMClient.DoApiPostWithUrl(c.GetUserActiveUrl(userId), MapBoolToJson(request), false)
	if err != nil {
		return BuildErrorResponse(r, err)
	}
	defer closeBody(r)
	return BuildResponse(r)
}

func (c *WClient) RevokeAllUserSessions(userId string) *Response {
	r, err := c.MMClient.DoApiPostWithUrl(c.RevokeAllUserSessionsUrl(userId), "", false)
	if err != nil {
		return BuildErrorResponse(r, err)
	}
	defer closeBody(r)
	return BuildResponse(r)
}

func (c *WClient) GetSessions(userId string) ([]*Session, *Response) {
	r, err := c.MMClient.DoApiGetWithUrl(c.GetSessionsUrl(userId), "", false)
	if err != nil {
		return nil, BuildErrorResponse(r, err)
	}
	defer closeBody(r)
	return SessionsFromJson(r.Body), BuildResponse(r)
}

func (c *WClient) RevokeSession(userId string, sessionId string) *Response {
	request := map[string]string{
		"session_id": sessionId,
	}
	r, err := c.MMClient.DoApiPostWithUrl(c.RevokeSessionUrl(userId), MapToJson(request), false)
	if err != nil {
		return BuildErrorResponse(r, err)
	}
	defer closeBody(r)
	return BuildResponse(r)
}
