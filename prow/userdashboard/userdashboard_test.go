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

package userdashboard

import (
	"testing"
	"net/http/httptest"
	"net/http"
	"io/ioutil"
	"reflect"
	"github.com/ghodss/yaml"
	"github.com/gorilla/sessions"
	"github.com/sirupsen/logrus"
	"github.com/shurcooL/githubql"
	"fmt"
	"k8s.io/test-infra/prow/config"
	"context"
)

type MockQueryHandler struct {
	prs []PullRequest
}

func (mh *MockQueryHandler) Query(ctx context.Context, ghc githubClient) ([]PullRequest, error){
	return mh.prs, nil
}

func newMockQueryHandler(prs []PullRequest) (*MockQueryHandler){
	return &MockQueryHandler {
		prs: prs,
	}
}

func createMockAgent(config *config.GitOAuthConfig) (*DashboardAgent) {
	return &DashboardAgent{
		gitConfig: config,
		log: logrus.WithField("unit-test", "dashboard-agent"),
	}
}

func TestServeHTTPWithoutLogin(t *testing.T) {
	mockCookieStore := sessions.NewCookieStore([]byte("secret-key"))
	mockConfig := &config.GitOAuthConfig{
		CookieStore: mockCookieStore,
		GitTokenKey: "mock-token-key",
		GitTokenSession: "mock-token-session",
	}

	mockAgent := createMockAgent(mockConfig)
	mockData := UserData {
		Login: false,
		PullRequests: nil,
	}

	rr := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/user-dashboard", nil)
	mockSession, err := mockCookieStore.New(request, mockConfig.GitTokenSession)
	if err != nil {
		t.Fatalf("Failed to create mock session: %v", err)
	}
	if err := mockSession.Save(request, rr); err != nil {
		t.Fatalf("Failed to save session: %v", err)
	}
	udHandler := mockAgent.HandleUserDashboard(mockAgent)
	udHandler.ServeHTTP(rr, request)
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
	mockCookieStore := sessions.NewCookieStore([]byte("secret-key"))
	mockConfig := &config.GitOAuthConfig{
		CookieStore: mockCookieStore,
		GitTokenKey: "mock-token-key",
		GitTokenSession: "mock-token-session",
	}

	mockAgent := createMockAgent(mockConfig)
	mockUserData := generateMockUserData()

	rr := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/user-dashboard", nil)
	mockSession, err := mockCookieStore.New(request, mockConfig.GitTokenSession)
	mockSession.Values[mockConfig.GitTokenKey] = "mock-access-token"
	if err != nil {
		t.Errorf("Error with creating mock session: %v", err)
	}
	if err := mockSession.Save(request, rr); err != nil {
		t.Fatalf("Error with saving mock session: %v", err)
	}
	mockQueryHandler := newMockQueryHandler(mockUserData.PullRequests)
	udHandler := mockAgent.HandleUserDashboard(mockQueryHandler)
	udHandler.ServeHTTP(rr, request)
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
	if !reflect.DeepEqual(dataReturned, mockUserData) {
		t.Errorf("Invalid user data. Got %v, expected %v", dataReturned, mockUserData)
	}
}

func generateMockPullRequest(numPr int) (PullRequest) {
	authorName :=  (githubql.String)(fmt.Sprintf("mock_user_login_%d", numPr))
	repoName := fmt.Sprintf("repo_%d", numPr)
	return PullRequest {
		Number: 1,
		Author: struct {
			Login githubql.String
		}{
			Login: authorName,
		},
		Repository: struct {
			Name githubql.String
			NameWithOwner  githubql.String
			Owner struct {
				Login githubql.String
		  }
		}{
			Name: (githubql.String)(repoName),
			NameWithOwner: (githubql.String)(fmt.Sprintf("%v_%v", repoName, authorName)),
			Owner: struct {
				Login githubql.String
			}	{
				Login: authorName,
			},
		},
		Labels: struct {
			Nodes []struct {
				Label Label
			}
		} {
			Nodes: []struct {
				Label Label
			} {
				{
					Label: Label{
						Id: (githubql.ID)(1),
						Name: (githubql.String)("label1"),
					},
				},
				{
					Label: Label{
						Id:   (githubql.ID)(2),
						Name: (githubql.String)("label2"),
					},
				},
			},
		},
	}
}

func generateMockUserData() (UserData) {
	prs := []PullRequest{}
	for numPr := 0; numPr < 5; numPr ++ {
		prs = append(prs, generateMockPullRequest(numPr))
	}

	return UserData{
		Login: true,
		PullRequests: prs,
	}
}
