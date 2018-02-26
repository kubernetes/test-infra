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

package githuboauth

import (
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/net/xsrftoken"
	"golang.org/x/oauth2"

	"k8s.io/test-infra/prow/config"
)

const (
	tokenSession       = "access-token-session"
	tokenKey           = "access-token"
	oauthSessionCookie = "oauth-session"
	stateKey           = "state"
)

type OAuthClient interface {
	// Exchanges code from github oauth redirect for user access token.
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)
	// Returns a URL to OAuth 2.0 github's consent page. The state is a token to protect user from
	// XSRF attack.
	AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string
}

// GithubOAuth Agent represents an agent that takes care Github authentication process such as handles
// login request from users or handles redirection from Github OAuth server.
type GithubOAuthAgent struct {
	gc     *config.GithubOAuthConfig
	logger *logrus.Entry
}

// Returns new GithubOAUth Agent.
func NewGithubOAuthAgent(config *config.GithubOAuthConfig, logger *logrus.Entry) *GithubOAuthAgent {
	return &GithubOAuthAgent{
		gc:     config,
		logger: logger,
	}
}

// HandleLogin handles Github login request from front-end. It starts a new git oauth session and
// redirect user to Github OAuth end-point for authentication.
func (ga *GithubOAuthAgent) HandleLogin(client OAuthClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stateToken := xsrftoken.Generate(ga.gc.ClientSecret, "", "")
		state := hex.EncodeToString([]byte(stateToken))
		oauthSession, err := ga.gc.CookieStore.New(r, oauthSessionCookie)
		if err != nil {
			ga.serverError(w, "Creating new OAuth session", err)
			return
		}
		oauthSession.Options.MaxAge = 10 * 60
		oauthSession.Values[stateKey] = state

		if err := oauthSession.Save(r, w); err != nil {
			ga.serverError(w, "Save oauth session", err)
			return
		}

		redirectURL := client.AuthCodeURL(state, oauth2.ApprovalForce, oauth2.AccessTypeOnline)
		http.Redirect(w, r, redirectURL, http.StatusFound)
	}
}

// HandleRedirect handles the redirection from Github. It exchanges the code from redirect URL for
// user access token. The access token is then saved to the cookie and the page is redirected to
// the final destination in the config, which should be the front-end.
func (ga *GithubOAuthAgent) HandleRedirect(client OAuthClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := r.FormValue("state")
		stateTokenRaw, err := hex.DecodeString(state)
		if err != nil {
			ga.serverError(w, "Decode state", fmt.Errorf("error with decoding state"))
		}
		stateToken := string(stateTokenRaw)
		// Check if the state token is still valid or not.
		if !xsrftoken.Valid(stateToken, ga.gc.ClientSecret, "", "") {
			ga.serverError(w, "Validate state", fmt.Errorf("state token has expired"))
			return
		}

		oauthSession, err := ga.gc.CookieStore.Get(r, oauthSessionCookie)
		if err != nil {
			ga.serverError(w, "Get cookie", err)
			return
		}
		secretState, ok := oauthSession.Values[stateKey].(string)
		if !ok {
			ga.serverError(w, "Get secret state", fmt.Errorf("cannot convert secret state to string"))
			return
		}
		// Validate the state parameter to prevent cross-site attack.
		if state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(secretState)) != 1 {
			ga.serverError(w, "Validate state", fmt.Errorf("invalid state"))
			return
		}

		// Exchanges the code for user access token.
		code := r.FormValue("code")
		token, err := client.Exchange(context.Background(), code)
		if err != nil {
			ga.serverError(w, "Exchange code for token", err)
			return
		}

		// New session that stores the token.
		session, err := ga.gc.CookieStore.New(r, tokenSession)
		if err != nil {
			ga.serverError(w, "Create new session", err)
			return
		}

		session.Values[tokenKey] = token
		if err := session.Save(r, w); err != nil {
			ga.serverError(w, "Save session", err)
			return
		}
		http.Redirect(w, r, ga.gc.FinalRedirectURL, http.StatusFound)
	}
}

// Handles server errors.
func (ga *GithubOAuthAgent) serverError(w http.ResponseWriter, action string, err error) {
	ga.logger.WithError(err).Errorf("Error %s.", action)
	msg := fmt.Sprintf("500 Internal server error %s: %v", action, err)
	http.Error(w, msg, http.StatusInternalServerError)
}
