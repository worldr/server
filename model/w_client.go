package model

import (
	"fmt"
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

//
// METHODS
//

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
