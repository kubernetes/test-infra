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
	"k8s.io/test-infra/prow/config"
	"net/http"
	"golang.org/x/net/xsrftoken"
	"fmt"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"golang.org/x/net/context"
)

const redirectURLKey = "redirect-url"

type GitOAuthAgent struct {
	gc *config.GitOAuthConfig
	logger *logrus.Entry
}

func NewGitOAuthAgent(config *config.GitOAuthConfig, logger *logrus.Entry) (*GitOAuthAgent){
	return &GitOAuthAgent{
		gc: config,
		logger: logger,
	}
}

func (ga *GitOAuthAgent) HandleLogin(w http.ResponseWriter, r *http.Request) {
	sessionId := xsrftoken.Generate(ga.gc.Client.ClientSecret, "","")

	oauthSession, err := ga.gc.CookieStore.New(r, sessionId)
	oauthSession.Options.MaxAge = 10 * 60

	if  err != nil {
		ga.serverError(w, "Create new oauth session", err)
		return
	}

	oauthSession.Values[redirectURLKey] = ga.gc.FinalRedirectURL

	if err := oauthSession.Save(r, w); err != nil {
		ga.serverError(w, "Save oauth session", err)
		return
	}

	redirectURL := ga.gc.OAuthClient.AuthCodeURL(sessionId, oauth2.ApprovalForce, oauth2.AccessTypeOnline)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (ga *GitOAuthAgent) HandleRedirect(w http.ResponseWriter, r *http.Request) {
	sessionId := r.FormValue("state")
	if !xsrftoken.Valid(sessionId, ga.gc.Client.ClientSecret, "", "") {
		ga.serverError(w, "Get session token", error("State token expired"))
		return
	}

	oauthSession, err := ga.gc.CookieStore.Get(r, r.FormValue("state"))
	if err != nil {
		ga.serverError(w, "Validate state parameter", err)
		return
	}

	finalRedirectUrl := oauthSession.Values[redirectURLKey].(string)
	if finalRedirectUrl != ga.gc.FinalRedirectURL {
		ga.serverError(w, "Validate OAuth session", error("Invalid OAuth session"))
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
	http.Redirect(w, r, finalRedirectUrl, http.StatusFound)
}

func (ga *GitOAuthAgent) serverError(w http.ResponseWriter, action string, err error) {
	ga.logger.WithError(err).Errorf("Error %s.", action)
	msg := fmt.Sprintf("500 Internal server error %s: %v", action, err)
	http.Error(w, msg, http.StatusInternalServerError)
}

