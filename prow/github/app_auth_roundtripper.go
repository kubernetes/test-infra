/*
Copyright 2020 The Kubernetes Authors.

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

package github

import (
	"context"
	"crypto/rsa"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	jwt "github.com/dgrijalva/jwt-go/v4"

	"k8s.io/test-infra/ghproxy/ghcache"
)

const (
	githubOrgHeaderKey = "X-PROW-GITHUB-ORG"
)

type appGitHubClient interface {
	ListAppInstallations() ([]AppInstallation, error)
	getAppInstallationToken(installationId int64) (*AppInstallationToken, error)
	GetApp() (*App, error)
}

func newAppsRoundTripper(appID string, privateKey func() *rsa.PrivateKey, upstream http.RoundTripper, githubClient appGitHubClient, v3BaseURLs []string) (*appsRoundTripper, error) {
	roundTripper := &appsRoundTripper{
		appID:             appID,
		privateKey:        privateKey,
		upstream:          upstream,
		githubClient:      githubClient,
		hostPrefixMapping: make(map[string]string, len(v3BaseURLs)),
	}
	for _, baseURL := range v3BaseURLs {
		url, err := url.Parse(baseURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse github-endpoint %s as URL: %w", baseURL, err)
		}
		roundTripper.hostPrefixMapping[url.Host] = url.Path
	}

	return roundTripper, nil
}

type appsRoundTripper struct {
	appID             string
	appSlug           string
	appSlugLock       sync.Mutex
	privateKey        func() *rsa.PrivateKey
	installationLock  sync.RWMutex
	installations     map[string]AppInstallation
	tokenLock         sync.RWMutex
	tokens            map[int64]*AppInstallationToken
	upstream          http.RoundTripper
	githubClient      appGitHubClient
	hostPrefixMapping map[string]string
}

// appsAuthError is returned by the appsRoundTripper if any issues were encountered
// trying to authorize the request. It signals the client to not retry.
type appsAuthError struct {
	error
}

func (*appsAuthError) Is(target error) bool {
	_, ok := target.(*appsAuthError)
	return ok
}

func (arr *appsRoundTripper) canonicalizedPath(url *url.URL) string {
	return strings.TrimPrefix(url.Path, arr.hostPrefixMapping[url.Host])
}

var installationPath = regexp.MustCompile(`^/repos/[^/]+/[^/]+/installation$`)

func (arr *appsRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	path := arr.canonicalizedPath(r.URL)
	// We need to use a JWT when we are getting /app/* endpoints or installation information for a particular repo
	if strings.HasPrefix(path, "/app") || installationPath.MatchString(path) {
		if err := arr.addAppAuth(r); err != nil {
			return nil, err
		}
	} else if err := arr.addAppInstallationAuth(r); err != nil {
		return nil, err
	}

	return arr.upstream.RoundTrip(r)
}

// TimeNow is exposed so that it can be mocked by unit test, to ensure that
// addAppAuth always return consistent token when needed.
// DO NOT use it in prod
var TimeNow = func() time.Time {
	return time.Now().UTC()
}

func (arr *appsRoundTripper) addAppAuth(r *http.Request) *appsAuthError {
	now := TimeNow()
	expiresAt := now.Add(10 * time.Minute)
	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, &jwt.StandardClaims{
		IssuedAt:  jwt.NewTime(float64(now.Unix())),
		ExpiresAt: jwt.NewTime(float64(expiresAt.Unix())),
		Issuer:    arr.appID,
	}).SignedString(arr.privateKey())
	if err != nil {
		return &appsAuthError{fmt.Errorf("failed to generate jwt: %w", err)}
	}

	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set(ghcache.TokenExpiryAtHeader, expiresAt.Format(time.RFC3339))

	// We call the /app endpoint to resolve the slug, so we can't set it there
	if arr.canonicalizedPath(r.URL) == "/app" {
		r.Header.Set(ghcache.TokenBudgetIdentifierHeader, arr.appID)
	} else {
		slug, err := arr.getSlug()
		if err != nil {
			return &appsAuthError{err}
		}
		r.Header.Set(ghcache.TokenBudgetIdentifierHeader, slug)
	}
	return nil
}

func extractOrgFromContext(ctx context.Context) string {
	var org string
	if v := ctx.Value(githubOrgHeaderKey); v != nil {
		org = v.(string)
	}
	return org
}

func (arr *appsRoundTripper) addAppInstallationAuth(r *http.Request) *appsAuthError {
	org := extractOrgFromContext(r.Context())
	if org == "" {
		return &appsAuthError{fmt.Errorf("BUG apps auth requested but empty org, please report this to the test-infra repo. Stack: %s", string(debug.Stack()))}
	}

	token, expiresAt, err := arr.installationTokenFor(org)
	if err != nil {
		return &appsAuthError{err}
	}

	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set(ghcache.TokenExpiryAtHeader, expiresAt.Format(time.RFC3339))
	slug, err := arr.getSlug()
	if err != nil {
		return &appsAuthError{err}
	}

	// Token budgets are set on organization level, so include it in the identifier
	// to not mess up metrics.
	r.Header.Set(ghcache.TokenBudgetIdentifierHeader, slug+" - "+org)

	return nil
}

func (arr *appsRoundTripper) installationTokenFor(org string) (string, time.Time, error) {
	installationID, err := arr.installationIDFor(org)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to get installation id for org %s: %w", org, err)
	}

	token, expiresAt, err := arr.getTokenForInstallation(installationID)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to get an installation token for org %s: %w", org, err)
	}

	return token, expiresAt, nil
}

// installationIDFor returns the installation id for the given org. Unfortunately,
// GitHub does not expose what repos in that org the app is installed in, it
// only tells us if its all repos or a subset via the repository_selection
// property.
// Ref: https://docs.github.com/en/free-pro-team@latest/rest/reference/apps#list-installations-for-the-authenticated-app
func (arr *appsRoundTripper) installationIDFor(org string) (int64, error) {
	arr.installationLock.RLock()
	id, found := arr.installations[org]
	arr.installationLock.RUnlock()
	if found {
		return id.ID, nil
	}

	arr.installationLock.Lock()
	defer arr.installationLock.Unlock()

	// Check again in case a concurrent routine updated it while we waited for the lock
	id, found = arr.installations[org]
	if found {
		return id.ID, nil
	}

	installations, err := arr.githubClient.ListAppInstallations()
	if err != nil {
		return 0, fmt.Errorf("failed to list app installations: %w", err)
	}

	installationsMap := make(map[string]AppInstallation, len(installations))
	for _, installation := range installations {
		installationsMap[installation.Account.Login] = installation
	}

	if equal := reflect.DeepEqual(arr.installations, installationsMap); equal {
		return 0, fmt.Errorf("the github app is not installed in organization %s", org)
	}
	arr.installations = installationsMap

	id, found = installationsMap[org]
	if !found {
		return 0, fmt.Errorf("the github app is not installed in organization %s", org)
	}

	return id.ID, nil
}

func (arr *appsRoundTripper) getTokenForInstallation(installation int64) (string, time.Time, error) {
	arr.tokenLock.RLock()
	token, found := arr.tokens[installation]
	arr.tokenLock.RUnlock()

	if found && token.ExpiresAt.Add(-time.Minute).After(time.Now()) {
		return token.Token, token.ExpiresAt, nil
	}

	arr.tokenLock.Lock()
	defer arr.tokenLock.Unlock()

	// Check again in case a concurrent routine got a token while we waited for the lock
	token, found = arr.tokens[installation]
	if found && token.ExpiresAt.Add(-time.Minute).After(time.Now()) {
		return token.Token, token.ExpiresAt, nil
	}

	token, err := arr.githubClient.getAppInstallationToken(installation)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to get installation token from GitHub: %w", err)
	}

	if arr.tokens == nil {
		arr.tokens = map[int64]*AppInstallationToken{}
	}
	arr.tokens[installation] = token

	return token.Token, token.ExpiresAt, nil
}

func (arr *appsRoundTripper) getSlug() (string, error) {
	arr.appSlugLock.Lock()
	defer arr.appSlugLock.Unlock()

	if arr.appSlug != "" {
		return arr.appSlug, nil
	}
	response, err := arr.githubClient.GetApp()
	if err != nil {
		return "", err
	}

	arr.appSlug = response.Slug
	return arr.appSlug, nil
}
