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
	oauthSessionCookie = "oauth-session"
	stateKey           = "state"
)

// GitOAuth Agent represents an agent that takes care Github authentication process such as handles
// login request from users or handles redirection from Github OAuth server.
type GitOAuthAgent struct {
	gc     *config.GitOAuthConfig
	logger *logrus.Entry
}

// Returns new GitOAUth Agent.
func NewGitOAuthAgent(config *config.GitOAuthConfig, logger *logrus.Entry) *GitOAuthAgent {
	return &GitOAuthAgent{
		gc:     config,
		logger: logger,
	}
}

// HandleLogin handles Github login request from front-end. It starts a new git oauth session and
// redirect user to Github OAuth end-point for authentication.
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

// HandleRedirect handles the redirection from Github. It exchanges the code from redirect URL for
// user access token. The access token is then saved to the cookie and the page is redirected to
// the final destination in the config, which should be the front-end.
func (ga *GitOAuthAgent) HandleRedirect(w http.ResponseWriter, r *http.Request) {
	state := r.FormValue("state")
	// Check if the state token is still valid or not.
	if !xsrftoken.Valid(state, ga.gc.Client.ClientSecret, "", "") {
		ga.serverError(w, "Validate state", errors.New("State token has expired"))
	}

	oauthSession, err := ga.gc.CookieStore.Get(r, oauthSessionCookie)
	if err != nil {
		ga.serverError(w, "Get cookie", err)
		return
	}
	// Validate the state parameter to prevent cross-site attack.
	if state == "" || state != oauthSession.Values[stateKey].(string) {
		ga.serverError(w, "Validate state", errors.New("Invalid state"))
		return
	}

	// Exchanges the code for user access token.
	code := r.FormValue("code")
	token, err := ga.gc.OAuthClient.Exchange(context.Background(), code)
	if err != nil {
		ga.serverError(w, "Exchange code for token", err)
		return
	}

	// New session that stores the token.
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

// Handles server errors.
func (ga *GitOAuthAgent) serverError(w http.ResponseWriter, action string, err error) {
	ga.logger.WithError(err).Errorf("Error %s.", action)
	msg := fmt.Sprintf("500 Internal server error %s: %v", action, err)
	http.Error(w, msg, http.StatusInternalServerError)
}
