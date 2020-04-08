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
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"k8s.io/test-infra/prow/flagutil"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/sessions"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/net/xsrftoken"
	"golang.org/x/oauth2"
	"k8s.io/test-infra/prow/github"
)

const (
	loginSession       = "github_login"
	tokenSession       = "access-token-session"
	tokenKey           = "access-token"
	oauthSessionCookie = "oauth-session"
	stateKey           = "state"
)

// Config is a config for requesting users access tokens from GitHub API. It also has
// a Cookie Store that retains user credentials deriving from GitHub API.
type Config struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURL  string   `json:"redirect_url"`
	Scopes       []string `json:"scopes,omitempty"`

	CookieStore *sessions.CookieStore `json:"-"`
}

// InitGitHubOAuthConfig creates an OAuthClient using GitHubOAuth config and a Cookie Store
// to retain user credentials.
func (c *Config) InitGitHubOAuthConfig(cookie *sessions.CookieStore) {
	// The `oauth2.Token` needs to be stored in the CookieStore with a specific encoder.
	// Since we are using `gorilla/sessions` which uses `gorilla/securecookie`,
	// it has to be registered to `encoding/gob`.
	//
	// See https://github.com/gorilla/securecookie/blob/master/doc.go#L56-L59
	gob.Register(&oauth2.Token{})
	c.CookieStore = cookie
}

// AuthenticatedUserIdentifier knows how to get the identity of an authenticated user
type AuthenticatedUserIdentifier interface {
	LoginForRequester(requester, token string) (string, error)
}

func NewAuthenticatedUserIdentifier(options *flagutil.GitHubOptions) AuthenticatedUserIdentifier {
	return &authenticatedUserIdentifier{clientFactory: options.GitHubClientWithAccessToken}
}

type authenticatedUserIdentifier struct {
	clientFactory func(accessToken string) github.Client
}

func (a *authenticatedUserIdentifier) LoginForRequester(requester, token string) (string, error) {
	return a.clientFactory(token).ForSubcomponent(requester).BotName()
}

// OAuthClient is an interface for a GitHub OAuth client.
type OAuthClient interface {
	WithFinalRedirectURL(url string) (OAuthClient, error)
	// Exchanges code from GitHub OAuth redirect for user access token.
	Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error)
	// Returns a URL to GitHub's OAuth 2.0 consent page. The state is a token to protect the user
	// from an XSRF attack.
	AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string
}

type client struct {
	*oauth2.Config
}

func NewClient(config *oauth2.Config) client {
	return client{
		config,
	}
}

func (cli client) WithFinalRedirectURL(path string) (OAuthClient, error) {
	parsedURL, err := url.Parse(cli.RedirectURL)
	if err != nil {
		return nil, err
	}
	q := parsedURL.Query()
	q.Set("dest", path)
	parsedURL.RawQuery = q.Encode()
	return NewClient(
		&oauth2.Config{
			ClientID:     cli.ClientID,
			ClientSecret: cli.ClientSecret,
			RedirectURL:  parsedURL.String(),
			Scopes:       cli.Scopes,
			Endpoint:     cli.Endpoint,
		},
	), nil
}

// Agent represents an agent that takes care GitHub authentication process such as handles
// login request from users or handles redirection from GitHub OAuth server.
type Agent struct {
	gc     *Config
	logger *logrus.Entry
}

// NewAgent returns a new GitHub OAuth Agent.
func NewAgent(config *Config, logger *logrus.Entry) *Agent {
	return &Agent{
		gc:     config,
		logger: logger,
	}
}

// HandleLogin handles GitHub login request from front-end. It starts a new git oauth session and
// redirect user to GitHub OAuth end-point for authentication.
func (ga *Agent) HandleLogin(client OAuthClient, secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		destPage := r.URL.Query().Get("dest")
		stateToken := xsrftoken.Generate(ga.gc.ClientSecret, "", "")
		state := hex.EncodeToString([]byte(stateToken))
		oauthSession, err := ga.gc.CookieStore.New(r, oauthSessionCookie)
		oauthSession.Options.Secure = secure
		oauthSession.Options.HttpOnly = true
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
		newClient, err := client.WithFinalRedirectURL(destPage)
		if err != nil {
			ga.serverError(w, "Failed to parse redirect URL", err)
		}
		redirectURL := newClient.AuthCodeURL(state, oauth2.ApprovalForce, oauth2.AccessTypeOnline)
		http.Redirect(w, r, redirectURL, http.StatusFound)
	}
}

// GetLogin returns the username of the already authenticated GitHub user.
func (ga *Agent) GetLogin(r *http.Request, identifier AuthenticatedUserIdentifier) (string, error) {
	session, err := ga.gc.CookieStore.Get(r, tokenSession)
	if err != nil {
		return "", err
	}
	token, ok := session.Values[tokenKey].(*oauth2.Token)
	if !ok || !token.Valid() {
		return "", fmt.Errorf("Could not find GitHub token")
	}
	login, err := identifier.LoginForRequester("rerun", token.AccessToken)
	if err != nil {
		return "", err
	}
	return login, nil
}

// HandleLogout handles GitHub logout request from front-end. It invalidates cookie sessions and
// redirect back to the front page.
func (ga *Agent) HandleLogout(client OAuthClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accessTokenSession, err := ga.gc.CookieStore.Get(r, tokenSession)
		if err != nil {
			ga.serverError(w, "get cookie", err)
			return
		}
		// Clear session
		accessTokenSession.Options.MaxAge = -1
		if err := accessTokenSession.Save(r, w); err != nil {
			ga.serverError(w, "Save invalidated session on log out", err)
			return
		}
		loginCookie, err := r.Cookie(loginSession)
		if err == nil {
			loginCookie.MaxAge = -1
			loginCookie.Expires = time.Now().Add(-time.Hour * 24)
			http.SetCookie(w, loginCookie)
		}
		http.Redirect(w, r, r.URL.Host, http.StatusFound)
	}
}

// HandleRedirect handles the redirection from GitHub. It exchanges the code from redirect URL for
// user access token. The access token is then saved to the cookie and the page is redirected to
// the final destination in the config, which should be the front-end.
func (ga *Agent) HandleRedirect(client OAuthClient, identifier AuthenticatedUserIdentifier, secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// This is string manipulation for clarity, and to avoid surprising parse mismatches.
		scheme := "http"
		if secure {
			scheme = "https"
		}
		finalRedirectURL := scheme + "://" + r.Host + "/" + r.URL.Query().Get("dest")

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
			ga.serverError(w, "Get secret state", fmt.Errorf("empty string or cannot convert to string. this probably means the options passed to GitHub don't match what was expected"))
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
			if gherror := r.FormValue("error"); len(gherror) > 0 {
				gherrorDescription := r.FormValue("error_description")
				gherrorURI := r.FormValue("error_uri")
				fields := logrus.Fields{
					"gh_error":             gherror,
					"gh_error_description": gherrorDescription,
					"gh_error_uri":         gherrorURI,
				}
				ga.logger.WithFields(fields).Error("GitHub passed errors in callback, token is not present")
				ga.serverError(w, "OAuth authentication with GitHub", fmt.Errorf(gherror))
			} else {
				ga.serverError(w, "Exchange code for token", err)
			}
			return
		}

		// New session that stores the token.
		session, err := ga.gc.CookieStore.New(r, tokenSession)
		session.Options.Secure = secure
		session.Options.HttpOnly = true
		if err != nil {
			ga.serverError(w, "Create new session", err)
			return
		}

		session.Values[tokenKey] = token
		if err := session.Save(r, w); err != nil {
			ga.serverError(w, "Save session", err)
			return
		}
		user, err := identifier.LoginForRequester("oauth", token.AccessToken)
		if err != nil {
			ga.serverError(w, "Get user login", err)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:    loginSession,
			Value:   user,
			Path:    "/",
			Expires: time.Now().Add(time.Hour * 24 * 30),
			Secure:  secure,
		})
		http.Redirect(w, r, finalRedirectURL, http.StatusFound)
	}
}

// Handles server errors.
func (ga *Agent) serverError(w http.ResponseWriter, action string, err error) {
	ga.logger.WithError(err).Errorf("Error %s.", action)
	msg := fmt.Sprintf("500 Internal server error %s: %v", action, err)
	http.Error(w, msg, http.StatusInternalServerError)
}
