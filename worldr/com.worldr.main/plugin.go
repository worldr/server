// Copyright (c) 2020-present Worldr, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"encoding/json"
	"net/http"

	"github.com/mattermost/mattermost-server/v5/plugin"
)

// WorldrExtension contains additions to RESTful API
type WorldrExtension struct {
	plugin.MattermostPlugin
}

func (p *WorldrExtension) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	uid := r.Header.Get("Mattermost-User-Id")
	if uid == "" {
		http.Error(w, "Not authorized", http.StatusUnauthorized)
	} else {
		execute(uid, p, c, w, r)
	}
}

func execute(uid string, p *WorldrExtension, c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")
	/*
		switch r.URL.Path {
		case "/channels/categories":
			fmt.Fprint(w, response)
		case "/channels/personal":
			fmt.Fprint(w, response)
		case "/dialog/work":
			fmt.Fprint(w, response)
		case "/dialog/global":
		default:
			http.NotFound(w, r)
		}
		return "{uri:" + r.RequestURI + "}"
	*/
	if res, err1 := p.API.GetChannelCategories(uid); err1 == nil {
		if bytes, err2 := json.Marshal(res); err2 == nil {
			w.Write(bytes)
		} else {
			http.Error(w, err2.Error(), http.StatusInternalServerError)
		}
	} else {
		http.Error(w, err1.Error(), http.StatusInternalServerError)
	}
}

func main() {
	plugin.ClientMain(&WorldrExtension{})
}
