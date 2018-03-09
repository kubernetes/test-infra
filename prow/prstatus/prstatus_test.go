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

package prstatus

import (
	"context"
	"encoding/gob"
	"fmt"
	"golang.org/x/oauth2"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/ghodss/yaml"
	"github.com/gorilla/sessions"
	"github.com/shurcooL/githubql"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/test-infra/prow/config"
)

type MockQueryHandler struct {
	prs []PullRequest
}

func (mh *MockQueryHandler) QueryPullRequests(ctx context.Context, ghc githubClient, query string) ([]PullRequest, error) {
	return mh.prs, nil
}

func newMockQueryHandler(prs []PullRequest) *MockQueryHandler {
	return &MockQueryHandler{
		prs: prs,
	}
}

func createMockAgent(repos []string, config *config.GithubOAuthConfig) *DashboardAgent {
	return &DashboardAgent{
		repos: repos,
		goac:  config,
		log:   logrus.WithField("unit-test", "dashboard-agent"),
	}
}

func TestServeHTTPWithoutLogin(t *testing.T) {
	repos := []string{"mock/repo", "kubernetes/test-infra", "foo/bar"}
	mockCookieStore := sessions.NewCookieStore([]byte("secret-key"))
	mockConfig := &config.GithubOAuthConfig{
		CookieStore: mockCookieStore,
	}

	mockAgent := createMockAgent(repos, mockConfig)
	mockData := UserData{
		Login:        false,
		PullRequests: nil,
	}

	rr := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/pr-data.js", nil)

	prHandler := mockAgent.HandlePrStatus(mockAgent)
	prHandler.ServeHTTP(rr, request)
	if rr.Code != http.StatusOK {
		t.Fatalf("Bad status code: %d", rr.Code)
	}
	response := rr.Result()
	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("Error with reading response body: %v", err)
	}
	var dataReturned UserData
	if err := yaml.Unmarshal(body, &dataReturned); err != nil {
		t.Errorf("Error with unmarshaling response: %v", err)
	}
	if !reflect.DeepEqual(dataReturned, mockData) {
		t.Errorf("Invalid user data. Got %v, expected %v", dataReturned, mockData)
	}
}

func TestServeHTTPWithLogin(t *testing.T) {
	repos := []string{"mock/repo", "kuberentes/test-infra", "foo/bar"}
	mockCookieStore := sessions.NewCookieStore([]byte("secret-key"))
	mockConfig := &config.GithubOAuthConfig{
		CookieStore: mockCookieStore,
	}

	mockAgent := createMockAgent(repos, mockConfig)
	mockUserData := generateMockUserData()

	rr := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/pr-data.js", nil)
	mockSession, err := sessions.GetRegistry(request).Get(mockCookieStore, tokenSession)
	if err != nil {
		t.Errorf("Error with creating mock session: %v", err)
	}
	gob.Register(oauth2.Token{})
	token := &oauth2.Token{AccessToken: "secret-token", Expiry: time.Now().Add(time.Duration(24*365) * time.Hour)}
	mockSession.Values[tokenKey] = token
	mockSession.Values[loginKey] = "random_user"

	mockQueryHandler := newMockQueryHandler(mockUserData.PullRequests)
	prHandler := mockAgent.HandlePrStatus(mockQueryHandler)
	prHandler.ServeHTTP(rr, request)
	if rr.Code != http.StatusOK {
		t.Fatalf("Bad status code: %d", rr.Code)
	}
	response := rr.Result()
	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("Error with reading response body: %v", err)
	}
	var dataReturned UserData
	if err := yaml.Unmarshal(body, &dataReturned); err != nil {
		t.Errorf("Error with unmarshaling response: %v", err)
	}
	if equality.Semantic.DeepEqual(dataReturned, mockUserData) {
		t.Errorf("Invalid user data. Got %v, expected %v.", dataReturned, mockUserData)
	}
}

func generateMockPullRequest(numPr int) PullRequest {
	authorName := (githubql.String)(fmt.Sprintf("mock_user_login_%d", numPr))
	repoName := fmt.Sprintf("repo_%d", numPr)
	return PullRequest{
		Number: (githubql.Int)(numPr),
		Merged: (githubql.Boolean)(true),
		Title:  (githubql.String)("A mock pull request"),
		Author: struct {
			Login githubql.String
		}{
			Login: authorName,
		},
		BaseRef: struct {
			Name   githubql.String
			Prefix githubql.String
		}{
			Name:   githubql.String("mockBaseName"),
			Prefix: githubql.String("mockPrefix"),
		},
		HeadRefOID: githubql.String("mockHeadRefOID"),
		Repository: struct {
			Name          githubql.String
			NameWithOwner githubql.String
			Owner         struct {
				Login githubql.String
			}
		}{
			Name:          (githubql.String)(repoName),
			NameWithOwner: (githubql.String)(fmt.Sprintf("%v_%v", repoName, authorName)),
			Owner: struct {
				Login githubql.String
			}{
				Login: authorName,
			},
		},
		Labels: struct {
			Nodes []struct {
				Label Label `graphql:"... on Label"`
			}
		}{
			Nodes: []struct {
				Label Label `graphql:"... on Label"`
			}{
				{
					Label: Label{
						ID:   (githubql.ID)(1),
						Name: (githubql.String)("label1"),
					},
				},
				{
					Label: Label{
						ID:   (githubql.ID)(2),
						Name: (githubql.String)("label2"),
					},
				},
			},
		},
		Milestone: struct {
			ID     githubql.ID
			Closed githubql.Boolean
		}{
			ID:     githubql.String("mockMilestoneID"),
			Closed: githubql.Boolean(true),
		},
	}
}

func TestConstructSearchQuery(t *testing.T) {
	repos := []string{"mock/repo", "kubernetes/test-infra", "foo/bar"}
	mockCookieStore := sessions.NewCookieStore([]byte("secret-key"))
	mockConfig := &config.GithubOAuthConfig{
		CookieStore: mockCookieStore,
	}
	mockAgent := createMockAgent(repos, mockConfig)
	query := mockAgent.ConstructSearchQuery("random_username")
	mockQuery := "is:pr state:open author:random_username repo:\"mock/repo\" repo:\"kubernetes/test-infra\" repo:\"foo/bar\""
	if query != mockQuery {
		t.Errorf("Invalid query. Got: %v, expected %v", query, mockQuery)
	}
}

func generateMockUserData() UserData {
	var prs []PullRequest
	for numPr := 0; numPr < 5; numPr++ {
		prs = append(prs, generateMockPullRequest(numPr))
	}

	return UserData{
		Login:        true,
		PullRequests: prs,
	}
}
