package api4

import (
	"net/http"

	"github.com/mattermost/mattermost-server/v5/model"
)

func (api *API) InitWPosts() {
	api.BaseRoutes.WPosts.Handle("/recent", api.ApiSessionRequired(getRecentPosts)).Methods("POST")
}

// getRecentPosts returns most recent posts for given channels and respects total and per channel limits.
func getRecentPosts(c *Context, w http.ResponseWriter, r *http.Request) {
	requestData := model.RecentRequestDataFromJson(r.Body)
	if requestData == nil {
		c.SetInvalidParam("recent_request_data")
		return
	}

	for _, channelId := range requestData.ChannelIds {
		if !c.App.SessionHasPermissionToChannel(*c.App.Session(), channelId, model.PERMISSION_READ_CHANNEL) {
			c.Err = model.NewAppError("getRecentPosts", "api.w_post.get_recent_posts.not_allowed.app_error", nil, "", http.StatusBadRequest)
			return
		}
	}

	posts, err := c.App.GetRecentPosts(requestData)
	if err != nil {
		c.Err = model.NewAppError("getRecentPosts", "api.w_post.get_recent_posts.get_failed.app_error", nil, "", http.StatusInternalServerError)
		return
	}

	for i := range *posts {
		p := (*posts)[i]
		p.StripActionIntegrations()
		(*posts)[i] = c.App.PreparePostForClient(p, false, false)
	}

	response := model.RecentPostsResponseData{Content: posts}
	w.Write([]byte(response.ToJson()))
}
