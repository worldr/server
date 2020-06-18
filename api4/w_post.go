package api4

import (
	"net/http"

	"github.com/mattermost/mattermost-server/v5/model"
)

func (api *API) InitWPosts() {
	api.BaseRoutes.WPosts.Handle("/recent", api.ApiSessionRequired(getRecentPosts)).Methods("POST")
	api.BaseRoutes.WPosts.Handle("/increment/check", api.ApiSessionRequired(checkIncrementPossible)).Methods("POST")
	api.BaseRoutes.WPosts.Handle("/increment", api.ApiSessionRequired(getIncrementalUpdate)).Methods("POST")
}

// getRecentPosts() returns most recent posts for given channels and respects total and per channel limits.
//
func getRecentPosts(c *Context, w http.ResponseWriter, r *http.Request) {
	requestData := model.RecentRequestDataFromJson(r.Body)
	if requestData == nil {
		c.SetInvalidParam("recent_request_data")
		return
	}

	for _, channelId := range requestData.ChannelIds {
		if !c.App.SessionHasPermissionToChannel(*c.App.Session(), channelId, model.PERMISSION_READ_CHANNEL) {
			c.Err = model.NewAppError("getRecentPosts", "api.w_post.get_recent_posts.not_allowed.app_error", nil, "no permission for channel", http.StatusBadRequest)
			return
		}
	}

	posts, err := c.App.GetRecentPosts(requestData)
	if err != nil {
		c.Err = model.NewAppError("getRecentPosts", "api.w_post.get_recent_posts.get_failed.app_error", nil, err.Message, http.StatusInternalServerError)
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

// checkIncrementPossible() returns true if it is reasonable to get the data for given channels
// in incremental manner. That is, the client can download all the messages accumulated
// since last visit in a few requests. If this is not the case, checkIncrementPossible()
// returns false and this means the client is required to wipe local data and resort
// to getting most recent posts only.
//
// checkIncrementPossible() respects total and per channel limits.
func checkIncrementPossible(c *Context, w http.ResponseWriter, r *http.Request) {
	requestData := model.IncrementCheckRequestDataFromJson(r.Body)
	if requestData == nil {
		c.SetInvalidParam("increment_possible_request_data")
		return
	}

	for _, ch := range requestData.Channels {
		if !c.App.SessionHasPermissionToChannel(*c.App.Session(), ch.ChannelId, model.PERMISSION_READ_CHANNEL) {
			c.Err = model.NewAppError(
				"checkIncrementPossible",
				"api.w_post.check_increment_posts.not_allowed.app_error",
				nil,
				"no permission for channel",
				http.StatusBadRequest,
			)
			return
		}
	}

	allow, err := c.App.CheckIncrementPossible(requestData)
	if err != nil {
		c.Err = model.NewAppError(
			"checkIncrementPossible",
			"api.w_post.check_increment_posts.check_failed.app_error",
			nil,
			err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	response := model.IncrementCheckResponse{Allow: allow}
	w.Write([]byte(response.ToJson()))
}

// getIncrementalUpdate() returns a list of posts for given channels from the last post
// the client is aware of to the most recent post in the channel.
//
// The method is intended for use when the client app starts after a period of abscence
// of the logged-in user. The client passes a list of channel ids along with a last post id
// for each channel the client currently has.
//
// getIncrementalUpdate() respects the order of channels given in the request.
//
// getIncrementalUpdate() returns no more than MAX_INCREMENT_PAGE messages.
// This means, not all of the requested channels may have messages in the response.
// Those should be requested again.
//
func getIncrementalUpdate(c *Context, w http.ResponseWriter, r *http.Request) {
	requestData := model.IncrementPostsRequestDataFromJson(r.Body)
	if requestData == nil {
		c.SetInvalidParam("increment_posts_request_data")
		return
	}

	for _, ch := range requestData.Channels {
		if !c.App.SessionHasPermissionToChannel(*c.App.Session(), ch.ChannelId, model.PERMISSION_READ_CHANNEL) {
			c.Err = model.NewAppError(
				"getIncrementalUpdate",
				"api.w_post.get_incremental_update.not_allowed.app_error",
				nil,
				"no permission for channel",
				http.StatusBadRequest,
			)
			return
		}
	}

	posts, err := c.App.GetIncrementalPosts(requestData)
	if err != nil {
		c.Err = model.NewAppError(
			"getIncrementalUpdate",
			"api.w_post.get_incremental_posts.get_failed.app_error",
			nil,
			err.Error(),
			http.StatusInternalServerError,
		)
		return
	}

	for _, v := range *posts {
		for i := range *v.Posts {
			p := (*v.Posts)[i]
			p.StripActionIntegrations()
			(*v.Posts)[i] = c.App.PreparePostForClient(p, false, false)
		}
	}

	response := model.IncrementPostsResponse{Content: posts}
	w.Write([]byte(response.ToJson()))
}
