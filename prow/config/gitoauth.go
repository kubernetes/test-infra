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
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"github.com/gorilla/sessions"
)

type Client struct {
	ClientId string `json:"client_secret,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
}

type GitOAuthConfig struct {
	RedirectURL string `json:"redirect_url,omitempty"`
	Scopes []string `json:"scopes,omitempty"`

	FinalRedirectURL string `json:"final_redirect_url,omitempty"`

	GitTokenSession string `json:"token_session,omitempty"`
	GitTokenKey string `json:"token_key,omitempty"`

	Client *Client
	OAuthClient *oauth2.Config
	CookieStore *sessions.CookieStore
}

func (gac *GitOAuthConfig) InitGitOAuthConfig(client *Client, cookie *sessions.CookieStore) {
	gac.Client = client
	gac.CookieStore = cookie
	gac.OAuthClient = &oauth2.Config{
			ClientID: gac.Client.ClientId,
			ClientSecret: gac.Client.ClientSecret,
			RedirectURL: gac.RedirectURL,
			Scopes: gac.Scopes,
			Endpoint: github.Endpoint,
		}
}
