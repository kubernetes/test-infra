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

type GitOAuthConfig struct {
	ClientId string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`

	RedirectURL string `json:"redirect_url,omitempty"`
	Scope []string `json:"scope,omitempty"`

	FinalRedirectURL string `json:"final_redirect_url,omitempty"`

	GitTokenSession string `json:"token_session,omitempty"`
	GitTokenKey string `json:"token_key,omitempty"`

	OAuthClient *oauth2.Config
	CookieStore *sessions.CookieStore
}

func (gac *GitOAuthConfig) InitGitOAuthConfig(cookie *sessions.CookieStore) {
	gac.CookieStore = cookie
	gac.OAuthClient = &oauth2.Config{
			ClientID: gac.ClientId,
			ClientSecret: gac.ClientSecret,
			RedirectURL: gac.RedirectURL,
			Scopes: gac.Scope,
			Endpoint: github.Endpoint,
		}
}
