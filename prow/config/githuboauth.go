/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"encoding/gob"

	"github.com/gorilla/sessions"
	"golang.org/x/oauth2"
)

// Cookie holds the secret returned from github that authenticates the user who authorized this app.
type Cookie struct {
	Secret string `json:"secret,omitempty"`
}

// GitHubOAuthConfig is a config for requesting users access tokens from GitHub API. It also has
// a Cookie Store that retains user credentials deriving from GitHub API.
type GitHubOAuthConfig struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURL  string   `json:"redirect_url"`
	Scopes       []string `json:"scopes,omitempty"`

	CookieStore *sessions.CookieStore `json:"-"`
}

// InitGitHubOAuthConfig creates an OAuthClient using GitHubOAuth config and a Cookie Store
// to retain user credentials.
func (gac *GitHubOAuthConfig) InitGitHubOAuthConfig(cookie *sessions.CookieStore) {
	gob.Register(&oauth2.Token{})
	gac.CookieStore = cookie
}
