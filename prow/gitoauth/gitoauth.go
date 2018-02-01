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

package gitoauth

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/net/xsrftoken"
	"golang.org/x/oauth2"
	"k8s.io/test-infra/prow/config"
)

const (
	redirectURLKey     = "redirect-url"
	oauthSessionCookie = "oauth-session"
	stateKey           = "state"
)

type GitOAuthAgent struct {
	gc     *config.GitOAuthConfig
	logger *logrus.Entry
}

func NewGitOAuthAgent(config *config.GitOAuthConfig, logger *logrus.Entry) *GitOAuthAgent {
	return &GitOAuthAgent{
		gc:     config,
		logger: logger,
	}
}

func (ga *GitOAuthAgent) HandleLogin(w http.ResponseWriter, r *http.Request) {
	state := xsrftoken.Generate(ga.gc.Client.ClientSecret, "", "")

	oauthSession, err := ga.gc.CookieStore.New(r, oauthSessionCookie)
	oauthSession.Options.MaxAge = 10 * 60

	if err != nil {
		ga.serverError(w, "Create new oauth session", err)
		return
	}

	oauthSession.Values[stateKey] = state

	if err := oauthSession.Save(r, w); err != nil {
		ga.serverError(w, "Save oauth session", err)
		return
	}

	redirectURL := ga.gc.OAuthClient.AuthCodeURL(state, oauth2.ApprovalForce, oauth2.AccessTypeOnline)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (ga *GitOAuthAgent) HandleRedirect(w http.ResponseWriter, r *http.Request) {
	state := r.FormValue("state")
	if !xsrftoken.Valid(state, ga.gc.Client.ClientSecret, "", "") {
		ga.serverError(w, "Validate state", errors.New("State token has expired"))
	}

	oauthSession, err := ga.gc.CookieStore.Get(r, oauthSessionCookie)
	if err != nil {
		ga.serverError(w, "Get cookie", err)
		return
	}

	if state == "" || state != oauthSession.Values[stateKey].(string) {
		ga.serverError(w, "Validate state", errors.New("Invalid state"))
		return
	}

	code := r.FormValue("code")
	token, err := ga.gc.OAuthClient.Exchange(context.Background(), code)
	if err != nil {
		ga.serverError(w, "Exchange code for token", err)
		return
	}

	session, err := ga.gc.CookieStore.New(r, ga.gc.GitTokenSession)
	if err != nil {
		ga.serverError(w, "Create new session", err)
		return
	}

	session.Values[ga.gc.GitTokenKey] = token
	if err := session.Save(r, w); err != nil {
		ga.serverError(w, "Save session", err)
		return
	}
	http.Redirect(w, r, ga.gc.FinalRedirectURL, http.StatusFound)
}

func (ga *GitOAuthAgent) serverError(w http.ResponseWriter, action string, err error) {
	ga.logger.WithError(err).Errorf("Error %s.", action)
	msg := fmt.Sprintf("500 Internal server error %s: %v", action, err)
	http.Error(w, msg, http.StatusInternalServerError)
}
