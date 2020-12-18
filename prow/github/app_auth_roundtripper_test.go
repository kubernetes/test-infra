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
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	utilpointer "k8s.io/utils/pointer"
)

// *appsAuthError implements the error interface
var _ error = &appsAuthError{}

// *appsRoundTripper implements the http.RoundTripper interface
var _ http.RoundTripper = &appsRoundTripper{}

type fakeRoundTripper struct {
	lock     sync.Mutex
	requests []*http.Request
	// path -> response
	responses map[string]*http.Response
}

func (frt *fakeRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	frt.lock.Lock()
	defer frt.lock.Unlock()
	frt.requests = append(frt.requests, r)
	if response, found := frt.responses[r.URL.Path]; found {
		return response, nil
	}
	return &http.Response{StatusCode: 400}, nil
}

func TestAppsAuth(t *testing.T) {

	const appID = "13"
	testCases := []struct {
		name                string
		cachedAppSlug       *string
		cachedInstallations map[string]AppInstallation
		cachedTokens        map[int64]*AppInstallationToken
		doRequest           func(Client) error
		responses           map[string]*http.Response
		verifyRequests      func([]*http.Request) error
	}{
		{
			name: "App auth success",
			doRequest: func(c Client) error {
				_, err := c.GetApp()
				return err
			},
			responses: map[string]*http.Response{"/app": {
				StatusCode: 200,
				Body:       serializeOrDie(App{}),
			}},
			verifyRequests: func(r []*http.Request) error {
				if n := len(r); n != 1 {
					return fmt.Errorf("expected exactly one request, got %d", n)
				}
				if val := r[0].Header.Get("Authorization"); !strings.HasPrefix(val, "Bearer ") {
					return fmt.Errorf("expected the Authorization header %q to start with 'Bearer '", val)
				}
				if val := r[0].Header.Get("X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER"); val != appID {
					return fmt.Errorf("expected X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER header %q to have value %s", val, appID)
				}
				return nil
			},
		},
		{
			name: "App auth failure",
			doRequest: func(c Client) error {
				_, err := c.GetApp()
				if expectedMsg := "status code 401 not one of [200], body: "; err == nil || err.Error() != expectedMsg {
					return fmt.Errorf("expected error to have message %s, was %v", expectedMsg, err)
				}
				return nil
			},
			responses: map[string]*http.Response{"/app": {
				StatusCode: 401,
				Body:       ioutil.NopCloser(&bytes.Buffer{}),
			}},
			verifyRequests: func(r []*http.Request) error {
				if n := len(r); n != 1 {
					return fmt.Errorf("expected exactly one request, got %d", n)
				}
				if val := r[0].Header.Get("Authorization"); !strings.HasPrefix(val, "Bearer ") {
					return fmt.Errorf("expected the Authorization header %q to start with 'Bearer '", val)
				}
				if val := r[0].Header.Get("X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER"); val != appID {
					return fmt.Errorf("expected X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER header %q to have value %s", val, appID)
				}
				return nil
			},
		},
		{
			name:                "App installation auth success, everything served from cache",
			cachedAppSlug:       utilpointer.StringPtr("ci-app"),
			cachedInstallations: map[string]AppInstallation{"org": {ID: 1}},
			cachedTokens:        map[int64]*AppInstallationToken{1: {Token: "the-token", ExpiresAt: time.Now().Add(time.Hour)}},
			doRequest: func(c Client) error {
				_, err := c.GetOrg("org")
				return err
			},
			responses: map[string]*http.Response{"/orgs/org": {
				StatusCode: 200,
				Body:       serializeOrDie(Organization{}),
			}},
			verifyRequests: func(r []*http.Request) error {
				if n := len(r); n != 1 {
					return fmt.Errorf("expected exactly one request, got %d", n)
				}
				if val := r[0].Header.Get("Authorization"); val != "Bearer the-token" {
					return fmt.Errorf("expected the Authorization header %q to be 'Bearer the-token'", val)
				}
				expectedGHCacheHeaderValue := "ci-app - org"
				if val := r[0].Header.Get("X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER"); val != expectedGHCacheHeaderValue {
					return fmt.Errorf("expected X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER header %q to be %q", val, expectedGHCacheHeaderValue)
				}
				return nil
			},
		},
		{
			name:                "App installation auth success, new token is requested",
			cachedAppSlug:       utilpointer.StringPtr("ci-app"),
			cachedInstallations: map[string]AppInstallation{"org": {ID: 1}},
			doRequest: func(c Client) error {
				_, err := c.GetOrg("org")
				return err
			},
			responses: map[string]*http.Response{
				"/orgs/org":                          {StatusCode: 200, Body: serializeOrDie(Organization{})},
				"/app/installations/1/access_tokens": {StatusCode: 201, Body: serializeOrDie(AppInstallationToken{Token: "the-token"})},
			},
			verifyRequests: func(r []*http.Request) error {
				if n := len(r); n != 2 {
					return fmt.Errorf("expected exactly one request, got %d", n)
				}
				expectedGHCacheHeaderValue := "ci-app - org"
				if r[0].URL.Path != "/app/installations/1/access_tokens" {
					return fmt.Errorf("expected first request to request a token, but had path %s", r[0].URL.Path)
				}
				if val := r[0].Header.Get("Authorization"); !strings.HasPrefix(val, "Bearer ") {
					return fmt.Errorf("expected the Authorization header %q to start with 'Bearer '", val)
				}
				if val := r[0].Header.Get("X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER"); val != "ci-app" {
					return fmt.Errorf("expected X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER header %q to have value ci-app", val)
				}
				if val := r[1].Header.Get("Authorization"); val != "Bearer the-token" {
					return fmt.Errorf("expected the Authorization header %q to be 'Bearer the-token'", val)
				}
				if val := r[1].Header.Get("X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER"); val != expectedGHCacheHeaderValue {
					return fmt.Errorf("expected X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER header %q to be %q", val, expectedGHCacheHeaderValue)
				}
				return nil
			},
		},
		{
			name:          "App installation auth success, installations and token is requsted",
			cachedAppSlug: utilpointer.StringPtr("ci-app"),
			doRequest: func(c Client) error {
				_, err := c.GetOrg("org")
				return err
			},
			responses: map[string]*http.Response{
				"/app/installations":                 {StatusCode: 200, Body: serializeOrDie([]AppInstallation{{ID: 1, Account: User{Login: "org"}}})},
				"/app/installations/1/access_tokens": {StatusCode: 201, Body: serializeOrDie(AppInstallationToken{Token: "the-token"})},
				"/orgs/org":                          {StatusCode: 200, Body: serializeOrDie(Organization{})},
			},
			verifyRequests: func(r []*http.Request) error {
				if n := len(r); n != 3 {
					return fmt.Errorf("expected exactly three request, got %d", n)
				}
				if r[0].URL.Path != "/app/installations" {
					return fmt.Errorf("expected first request to have path '/app/installations' but had %q", r[0].URL.Path)
				}
				if val := r[0].Header.Get("Authorization"); !strings.HasPrefix(val, "Bearer ") {
					return fmt.Errorf("expected the Authorization header %q to start with 'Bearer '", val)
				}
				if val := r[0].Header.Get("X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER"); val != "ci-app" {
					return fmt.Errorf("expected X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER header %q to have value ci-app", val)
				}

				if r[1].URL.Path != "/app/installations/1/access_tokens" {
					return fmt.Errorf("expected second request to request a token, but had path %s", r[0].URL.Path)
				}
				if val := r[1].Header.Get("Authorization"); !strings.HasPrefix(val, "Bearer ") {
					return fmt.Errorf("expected the Authorization header %q to start with 'Bearer '", val)
				}
				if val := r[1].Header.Get("X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER"); val != "ci-app" {
					return fmt.Errorf("expected X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER header %q to have value ci-app", val)
				}

				expectedGHCacheHeaderValue := "ci-app - org"
				if val := r[2].Header.Get("Authorization"); val != "Bearer the-token" {
					return fmt.Errorf("expected the Authorization header %q to be 'Bearer the-token'", val)
				}
				if val := r[2].Header.Get("X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER"); val != expectedGHCacheHeaderValue {
					return fmt.Errorf("expected X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER header %q to be %q", val, expectedGHCacheHeaderValue)
				}
				return nil
			},
		},
		{
			name: "App installation auth success, slug, installations and token is requsted",
			doRequest: func(c Client) error {
				_, err := c.GetOrg("org")
				return err
			},
			responses: map[string]*http.Response{
				"/app":                               {StatusCode: 200, Body: serializeOrDie(App{Slug: "ci-app"})},
				"/app/installations":                 {StatusCode: 200, Body: serializeOrDie([]AppInstallation{{ID: 1, Account: User{Login: "org"}}})},
				"/app/installations/1/access_tokens": {StatusCode: 201, Body: serializeOrDie(AppInstallationToken{Token: "the-token"})},
				"/orgs/org":                          {StatusCode: 200, Body: serializeOrDie(Organization{})},
			},
			verifyRequests: func(r []*http.Request) error {
				if n := len(r); n != 4 {
					return fmt.Errorf("expected exactly four request, got %d", n)
				}

				if r[0].URL.Path != "/app" {
					return fmt.Errorf("expected first request to have path '/app' but had %q", r[0].URL.Path)
				}
				if val := r[0].Header.Get("Authorization"); !strings.HasPrefix(val, "Bearer ") {
					return fmt.Errorf("expected the Authorization header %q to start with 'Bearer '", val)
				}
				if val := r[0].Header.Get("X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER"); val != "13" {
					return fmt.Errorf("expected X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER header %q to have value '13'", val)
				}

				if r[1].URL.Path != "/app/installations" {
					return fmt.Errorf("expected first request to have path '/app/installations' but had %q", r[0].URL.Path)
				}
				if val := r[1].Header.Get("Authorization"); !strings.HasPrefix(val, "Bearer ") {
					return fmt.Errorf("expected the Authorization header %q to start with 'Bearer '", val)
				}
				if val := r[1].Header.Get("X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER"); val != "ci-app" {
					return fmt.Errorf("expected X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER header %q to have value ci-app", val)
				}

				if r[2].URL.Path != "/app/installations/1/access_tokens" {
					return fmt.Errorf("expected second request to request a token, but had path %s", r[0].URL.Path)
				}
				if val := r[2].Header.Get("Authorization"); !strings.HasPrefix(val, "Bearer ") {
					return fmt.Errorf("expected the Authorization header %q to start with 'Bearer '", val)
				}
				if val := r[2].Header.Get("X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER"); val != "ci-app" {
					return fmt.Errorf("expected X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER header %q to have value ci-app", val)
				}

				expectedGHCacheHeaderValue := "ci-app - org"
				if val := r[3].Header.Get("Authorization"); val != "Bearer the-token" {
					return fmt.Errorf("expected the Authorization header %q to be 'Bearer the-token'", val)
				}
				if val := r[3].Header.Get("X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER"); val != expectedGHCacheHeaderValue {
					return fmt.Errorf("expected X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER header %q to be %q", val, expectedGHCacheHeaderValue)
				}
				return nil
			},
		},
		{
			name:          "App installation request has no installation, failure",
			cachedAppSlug: utilpointer.StringPtr("ci-app"),
			doRequest: func(c Client) error {
				_, err := c.GetOrg("other-org")
				expectedErrMsgSubstr := "failed to get installation id for org other-org: the github app is not installed in organization other-org"
				if err == nil || !strings.Contains(err.Error(), expectedErrMsgSubstr) {
					return fmt.Errorf("expected error to contain string %s, was %v", expectedErrMsgSubstr, err)
				}
				return nil
			},
			responses: map[string]*http.Response{
				"/app/installations": {StatusCode: 200, Body: serializeOrDie([]AppInstallation{{ID: 1, Account: User{Login: "org"}}})},
			},
			verifyRequests: func(r []*http.Request) error {
				if n := len(r); n != 1 {
					return fmt.Errorf("expected exactly four request, got %d", n)
				}

				if val := r[0].Header.Get("Authorization"); !strings.HasPrefix(val, "Bearer ") {
					return fmt.Errorf("expected the Authorization header %q to start with 'Bearer '", val)
				}
				if val := r[0].Header.Get("X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER"); val != "ci-app" {
					return fmt.Errorf("expected X-PROW-GHCACHE-TOKEN-BUDGET-IDENTIFIER header %q to have value ci-app", val)
				}

				return nil
			},
		},
	}

	// Generate it only once. Can not be smaller, otherwise the JWT signature generation
	// fails with "message too long for RSA public key size"
	rsaKey, err := rsa.GenerateKey(rand.Reader, 512)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, ghClient := NewAppsAuthClientWithFields(logrus.Fields{}, func(b []byte) []byte { return b }, appID, func() *rsa.PrivateKey { return rsaKey }, "", "")

			if _, ok := ghClient.(*client); !ok {
				t.Fatal("ghclient is not a *client")
			}
			if _, ok := ghClient.(*client).client.(*http.Client); !ok {
				t.Fatal("the ghclients client is not a *http.Client")
			}
			if _, ok := ghClient.(*client).client.(*http.Client).Transport.(*appsRoundTripper); !ok {
				t.Fatal("the ghclients didn't get configured to use the appsRoundTripper")
			}

			roundTripper := &fakeRoundTripper{
				responses: tc.responses,
			}

			appsRoundTripper := ghClient.(*client).client.(*http.Client).Transport.(*appsRoundTripper)
			appsRoundTripper.upstream = roundTripper
			if tc.cachedAppSlug != nil {
				appsRoundTripper.appSlug = *tc.cachedAppSlug
			}
			if tc.cachedInstallations != nil {
				appsRoundTripper.installations = tc.cachedInstallations
			}
			if tc.cachedTokens != nil {
				appsRoundTripper.tokens = tc.cachedTokens
			}

			if err := tc.doRequest(ghClient); err != nil {
				t.Fatalf("Failed to do request: %v", err)
			}

			if err := tc.verifyRequests(roundTripper.requests); err != nil {
				t.Errorf("Request verification failed: %v", err)
			}
		})
	}
}

func TestAppsRoundTripperThreadSafety(t *testing.T) {
	const appID = "13"
	// Can not be smaller, otherwise the JWT signature generation
	// fails with "message too long for RSA public key size"
	rsaKey, err := rsa.GenerateKey(rand.Reader, 512)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	_, ghClient := NewAppsAuthClientWithFields(logrus.Fields{}, nil, appID, func() *rsa.PrivateKey { return rsaKey }, "", "")

	if _, ok := ghClient.(*client); !ok {
		t.Fatal("ghclient is not a *client")
	}
	if _, ok := ghClient.(*client).client.(*http.Client); !ok {
		t.Fatal("the ghclients client is not a *http.Client")
	}
	if _, ok := ghClient.(*client).client.(*http.Client).Transport.(*appsRoundTripper); !ok {
		t.Fatal("the ghclients didn't get configured to use the appsRoundTripper")
	}

	// installation and token for requests to "org" are cached, but need to be fetched for requests
	// to "other-org"
	appsRoundTripper := ghClient.(*client).client.(*http.Client).Transport.(*appsRoundTripper)
	appsRoundTripper.installations = map[string]AppInstallation{"org": {ID: 1}}
	appsRoundTripper.tokens = map[int64]*AppInstallationToken{1: {Token: "the-token", ExpiresAt: time.Now().Add(time.Hour)}}
	appsRoundTripper.upstream = &fakeRoundTripper{
		responses: map[string]*http.Response{
			"/app": {StatusCode: 200, Body: serializeOrDie(App{Slug: "ci-app"})},
			"/app/installations": {StatusCode: 200, Body: serializeOrDie([]AppInstallation{
				{ID: 1, Account: User{Login: "org"}},
				{ID: 2, Account: User{Login: "other-org"}},
			})},
			"/app/installations/2/access_tokens": {StatusCode: 201, Body: serializeOrDie(AppInstallationToken{Token: "the-other-token"})},
			"/orgs/org":                          {StatusCode: 200, Body: serializeOrDie(Organization{})},
			"/orgs/other-org":                    {StatusCode: 200, Body: serializeOrDie(Organization{})},
		},
	}

	req1Done, req2Done := make(chan struct{}), make(chan struct{})

	go func() {
		defer close(req1Done)
		if _, err := ghClient.GetOrg("org"); err != nil {
			t.Errorf("failed to get org org: %v", err)
		}
	}()

	go func() {
		defer close(req2Done)
		if _, err := ghClient.GetOrg("other-org"); err != nil {
			t.Errorf("failed to get org other-org: %v", err)
		}
	}()

	<-req1Done
	<-req2Done
}

func serializeOrDie(in interface{}) io.ReadCloser {
	rawData, err := json.Marshal(in)
	if err != nil {
		panic(fmt.Sprintf("Serialization failed: %v", err))
	}
	return ioutil.NopCloser(bytes.NewBuffer(rawData))
}
