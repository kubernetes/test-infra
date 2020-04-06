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
	"encoding/gob"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/net/xsrftoken"
	"golang.org/x/oauth2"
)

const mockAccessToken = "justSomeRandomSecretToken"

type mockOAuthClient struct {
	config *oauth2.Config
}

func (c mockOAuthClient) WithFinalRedirectURL(path string) (OAuthClient, error) {
	parsedURL, err := url.Parse("www.something.com")
	if err != nil {
		return nil, err
	}
	q := parsedURL.Query()
	q.Set("dest", path)
	parsedURL.RawQuery = q.Encode()
	return mockOAuthClient{&oauth2.Config{RedirectURL: parsedURL.String()}}, nil
}

func (c mockOAuthClient) Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	return &oauth2.Token{
		AccessToken: mockAccessToken,
	}, nil
}

func (c mockOAuthClient) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	return c.config.AuthCodeURL(state, opts...)
}

func getMockConfig(cookie *sessions.CookieStore) *Config {
	clientID := "mock-client-id"
	clientSecret := "mock-client-secret"
	redirectURL := "uni-test/redirect-url"
	scopes := []string{}

	return &Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       scopes,

		CookieStore: cookie,
	}
}

func createMockStateToken(config *Config) string {
	stateToken := xsrftoken.Generate(config.ClientSecret, "", "")
	state := hex.EncodeToString([]byte(stateToken))

	return state
}

func isEqual(token1 *oauth2.Token, token2 *oauth2.Token) bool {
	return token1.AccessToken == token2.AccessToken &&
		token1.Expiry == token2.Expiry &&
		token1.RefreshToken == token2.RefreshToken &&
		token1.TokenType == token2.TokenType
}

func TestHandleLogin(t *testing.T) {
	dest := "wherever"
	rerunStatus := "working"
	cookie := sessions.NewCookieStore([]byte("secret-key"))
	mockConfig := getMockConfig(cookie)
	mockLogger := logrus.WithField("uni-test", "githuboauth")
	mockAgent := NewAgent(mockConfig, mockLogger)
	mockOAuthClient := mockOAuthClient{}

	mockRequest := httptest.NewRequest(http.MethodGet, "/mock-login?dest="+dest+"?rerun="+rerunStatus, nil)
	mockResponse := httptest.NewRecorder()

	handleLoginFn := mockAgent.HandleLogin(mockOAuthClient, false)
	handleLoginFn.ServeHTTP(mockResponse, mockRequest)
	result := mockResponse.Result()
	if result.StatusCode != http.StatusFound {
		t.Errorf("Unexpected status code. Got %v, expected %v", result.StatusCode, http.StatusFound)
	}
	resultCookies := result.Cookies()
	var oauthCookie *http.Cookie
	for _, v := range resultCookies {
		if v.Name == oauthSessionCookie {
			oauthCookie = v
			break
		}
	}
	if oauthCookie == nil {
		t.Error("Cookie for oauth session not found")
	}
	decodedCookie := make(map[interface{}]interface{})
	if err := securecookie.DecodeMulti(oauthCookie.Name, oauthCookie.Value, &decodedCookie, cookie.Codecs...); err != nil {
		t.Fatalf("Cannot decoded cookie: %v", err)
	}
	state, ok := decodedCookie[stateKey].(string)
	if !ok {
		t.Fatal("Error with getting state parameter")
	}
	stateTokenRaw, err := hex.DecodeString(state)
	if err != nil {
		t.Fatal("Cannot decoding state token")
	}
	stateToken := string(stateTokenRaw)
	if !xsrftoken.Valid(stateToken, mockConfig.ClientSecret, "", "") {
		t.Error("Expect the state token is valid, found state token invalid.")
	}
	if state == "" {
		t.Error("Expect state parameter is not empty, found empty")
	}
	destCount := 0
	path := mockResponse.Header().Get("Location")
	for _, q := range strings.Split(path, "&") {
		if q == "redirect_uri=www.something.com%3Fdest%3Dwherever%253Frerun%253Dworking" {
			destCount++
		}
	}
	if destCount != 1 {
		t.Errorf("Redirect URI in path does not include correct destination. path: %s, destination: %s", path, "?dest="+dest+"?rerun=working")
	}
}

func TestHandleLogout(t *testing.T) {
	cookie := sessions.NewCookieStore([]byte("secret-key"))
	mockConfig := getMockConfig(cookie)
	mockLogger := logrus.WithField("uni-test", "githuboauth")
	mockAgent := NewAgent(mockConfig, mockLogger)
	mockOAuthClient := mockOAuthClient{}

	mockRequest := httptest.NewRequest(http.MethodGet, "/mock-logout", nil)
	_, err := cookie.New(mockRequest, tokenSession)
	if err != nil {
		t.Fatalf("Failed to create a mock token session with error: %v", err)
	}
	mockResponse := httptest.NewRecorder()

	handleLoginFn := mockAgent.HandleLogout(mockOAuthClient)
	handleLoginFn.ServeHTTP(mockResponse, mockRequest)
	result := mockResponse.Result()
	if result.StatusCode != http.StatusFound {
		t.Errorf("Unexpected status code. Got %v, expected %v", result.StatusCode, http.StatusFound)
	}
	resultCookies := result.Cookies()
	var tokenCookie *http.Cookie
	cookieCounts := 0
	for _, v := range resultCookies {
		if v.Name == tokenSession {
			tokenCookie = v
			cookieCounts++
		}
	}
	if cookieCounts != 1 {
		t.Errorf("Wrong number of %s cookie. There should be only one cookie with name %s", tokenSession, tokenSession)
	}
	if tokenCookie == nil {
		t.Error("Cookie for oauth session not found")
	}
	if tokenCookie.MaxAge != -1 {
		t.Errorf("Expect cookie MaxAge equals -1, %d", tokenCookie.MaxAge)
	}
}

func TestHandleLogoutWithLoginSession(t *testing.T) {
	cookie := sessions.NewCookieStore([]byte("secret-key"))
	mockConfig := getMockConfig(cookie)
	mockLogger := logrus.WithField("uni-test", "githuboauth")
	mockAgent := NewAgent(mockConfig, mockLogger)
	mockOAuthClient := mockOAuthClient{}

	mockRequest := httptest.NewRequest(http.MethodGet, "/mock-logout", nil)
	_, err := cookie.New(mockRequest, tokenSession)
	mocKLoginSession := &http.Cookie{
		Name: loginSession,
		Path: "/",
	}
	mockRequest.AddCookie(mocKLoginSession)
	if err != nil {
		t.Fatalf("Failed to create a mock token session with error: %v", err)
	}
	mockResponse := httptest.NewRecorder()

	handleLoginFn := mockAgent.HandleLogout(mockOAuthClient)
	handleLoginFn.ServeHTTP(mockResponse, mockRequest)
	result := mockResponse.Result()
	if result.StatusCode != http.StatusFound {
		t.Errorf("Unexpected status code. Got %v, expected %v", result.StatusCode, http.StatusFound)
	}
	resultCookies := result.Cookies()
	var loginCookie *http.Cookie
	for _, v := range resultCookies {
		if v.Name == loginSession {
			loginCookie = v
			break
		}
	}
	if loginCookie == nil {
		t.Error("Cookie for oauth session not found")
	}
	if loginCookie.MaxAge != -1 {
		t.Errorf("Expect cookie MaxAge equals -1, %d", loginCookie.MaxAge)
	}
}

type fakeAuthenticatedUserIdentifier struct {
	login string
}

func (a *fakeAuthenticatedUserIdentifier) LoginForRequester(requester, token string) (string, error) {
	return a.login, nil
}

func TestGetLogin(t *testing.T) {
	cookie := sessions.NewCookieStore([]byte("secret-key"))
	mockConfig := getMockConfig(cookie)
	mockLogger := logrus.WithField("uni-test", "githuboauth")
	mockAgent := NewAgent(mockConfig, mockLogger)
	mockToken := &oauth2.Token{AccessToken: "tokentokentoken"}
	mockRequest := httptest.NewRequest(http.MethodGet, "/someurl", nil)
	mockSession, err := sessions.GetRegistry(mockRequest).Get(cookie, "access-token-session")
	if err != nil {
		t.Fatalf("Error with getting mock session: %v", err)
	}
	mockSession.Values["access-token"] = mockToken

	login, err := mockAgent.GetLogin(mockRequest, &fakeAuthenticatedUserIdentifier{"correct-login"})
	if err != nil {
		t.Fatalf("Error getting login: %v", err)
	}
	if login != "correct-login" {
		t.Fatalf("Incorrect login: %s", login)
	}
}

func TestHandleRedirectWithInvalidState(t *testing.T) {
	gob.Register(&oauth2.Token{})
	cookie := sessions.NewCookieStore([]byte("secret-key"))
	mockConfig := getMockConfig(cookie)
	mockLogger := logrus.WithField("uni-test", "githuboauth")
	mockAgent := NewAgent(mockConfig, mockLogger)
	mockOAuthClient := mockOAuthClient{}
	mockStateToken := createMockStateToken(mockConfig)

	mockRequest := httptest.NewRequest(http.MethodGet, "/mock-login", nil)
	mockResponse := httptest.NewRecorder()
	query := mockRequest.URL.Query()
	query.Add("state", "bad-state-token")
	mockRequest.URL.RawQuery = query.Encode()
	mockSession, err := sessions.GetRegistry(mockRequest).Get(cookie, oauthSessionCookie)
	if err != nil {
		t.Fatalf("Error with getting mock session: %v", err)
	}
	mockSession.Values[stateKey] = mockStateToken

	handleLoginFn := mockAgent.HandleRedirect(mockOAuthClient, &fakeAuthenticatedUserIdentifier{""}, false)
	handleLoginFn.ServeHTTP(mockResponse, mockRequest)
	result := mockResponse.Result()

	if result.StatusCode != http.StatusInternalServerError {
		t.Errorf("Invalid status code. Got %v, expected %v", result.StatusCode, http.StatusInternalServerError)
	}
}

func TestHandleRedirectWithValidState(t *testing.T) {
	gob.Register(&oauth2.Token{})
	cookie := sessions.NewCookieStore([]byte("secret-key"))
	mockConfig := getMockConfig(cookie)
	mockLogger := logrus.WithField("uni-test", "githuboauth")
	mockAgent := NewAgent(mockConfig, mockLogger)
	mockLogin := "foo_name"
	mockOAuthClient := mockOAuthClient{}
	mockStateToken := createMockStateToken(mockConfig)

	dest := "somewhere"
	rerunStatus := "working"
	mockRequest := httptest.NewRequest(http.MethodGet, "/mock-login?dest="+dest+"?rerun="+rerunStatus, nil)
	mockResponse := httptest.NewRecorder()
	query := mockRequest.URL.Query()
	query.Add("state", mockStateToken)
	query.Add("rerun", "working")
	mockRequest.URL.RawQuery = query.Encode()

	mockSession, err := sessions.GetRegistry(mockRequest).Get(cookie, oauthSessionCookie)
	if err != nil {
		t.Fatalf("Error with getting mock session: %v", err)
	}
	mockSession.Values[stateKey] = mockStateToken

	handleLoginFn := mockAgent.HandleRedirect(mockOAuthClient, &fakeAuthenticatedUserIdentifier{mockLogin}, false)
	handleLoginFn.ServeHTTP(mockResponse, mockRequest)
	result := mockResponse.Result()
	if result.StatusCode != http.StatusFound {
		t.Errorf("Invalid status code. Got %v, expected %v", result.StatusCode, http.StatusFound)
	}
	resultCookies := result.Cookies()
	var oauthCookie *http.Cookie
	for _, v := range resultCookies {
		if v.Name == tokenSession {
			oauthCookie = v
			break
		}
	}
	if oauthCookie == nil {
		t.Fatalf("Cookie for oauth session not found")
	}
	decodedCookie := make(map[interface{}]interface{})
	if err := securecookie.DecodeMulti(oauthCookie.Name, oauthCookie.Value, &decodedCookie, cookie.Codecs...); err != nil {
		t.Fatalf("Cannot decoded cookie: %v", err)
	}
	accessTokenFromCookie, ok := decodedCookie[tokenKey].(*oauth2.Token)
	if !ok {
		t.Fatalf("Error with getting access token: %v", decodedCookie)
	}
	token := &oauth2.Token{
		AccessToken: mockAccessToken,
	}
	if !isEqual(accessTokenFromCookie, token) {
		t.Errorf("Invalid access token. Got %v, expected %v", accessTokenFromCookie, token)
	}
	var loginCookie *http.Cookie
	for _, v := range resultCookies {
		if v.Name == loginSession {
			loginCookie = v
			break
		}
	}
	if loginCookie == nil {
		t.Fatalf("Cookie for github login not found")
	}
	if loginCookie.Value != mockLogin {
		t.Errorf("Mismatch github login. Got %v, expected %v", loginCookie.Value, mockLogin)
	}
	path := mockResponse.Header().Get("Location")
	if path != "http://example.com/"+dest+"?rerun="+rerunStatus {
		t.Errorf("Incorrect final redirect URL. Actual path: %s, Expected path: /%s", path, dest+"?rerun="+rerunStatus)
	}
}
