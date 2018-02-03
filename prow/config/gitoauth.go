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
	"golang.org/x/oauth2/github"
)

// Client holds client id and client secret for the git app, using in deck, which query pull
// requests on behave of users.
type Client struct {
	ClientId     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
}

// GitOAuthConfig is a config for requesting users access tokens from Github API. It also has
// a Cookie Store that retains user credentials deriving from Github API.
type GitOAuthConfig struct {
	RedirectURL string   `json:"redirect_url,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`

	FinalRedirectURL string `json:"final_redirect_url,omitempty"`

	GitTokenSession string `json:"token_session,omitempty"`
	GitTokenKey     string `json:"token_key,omitempty"`

	Client      *Client
	OAuthClient *oauth2.Config
	CookieStore *sessions.CookieStore
}

// Initialise a GitOAuthConfig. It creates a OAuthClient using GitOAuth config and a Cookie Store
// to retain user credentials.
func (gac *GitOAuthConfig) InitGitOAuthConfig(client *Client, cookie *sessions.CookieStore) {
	gob.Register(&oauth2.Token{})
	gac.Client = client
	gac.CookieStore = cookie
	gac.OAuthClient = &oauth2.Config{
		ClientID:     client.ClientId,
		ClientSecret: client.ClientSecret,
		RedirectURL:  gac.RedirectURL,
		Scopes:       gac.Scopes,
		Endpoint:     github.Endpoint,
	}
}
