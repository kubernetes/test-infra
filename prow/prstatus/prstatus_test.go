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
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/gorilla/sessions"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/githuboauth"
)

type MockQueryHandler struct {
	prs        []PullRequest
	contextMap map[int][]Context
}

func (mh *MockQueryHandler) queryPullRequests(ctx context.Context, ghc githubQuerier, query string) ([]PullRequest, error) {
	return mh.prs, nil
}

func (mh *MockQueryHandler) getHeadContexts(ghc githubStatusFetcher, pr PullRequest) ([]Context, error) {
	return mh.contextMap[int(pr.Number)], nil
}

type fgc struct {
	combinedStatus *github.CombinedStatus
	checkruns      *github.CheckRunList
	botName        string
}

func (c fgc) Query(context.Context, interface{}, map[string]interface{}) error {
	return nil
}

func (c fgc) GetCombinedStatus(org, repo, ref string) (*github.CombinedStatus, error) {
	return c.combinedStatus, nil
}

func (c fgc) ListCheckRuns(org, repo, ref string) (*github.CheckRunList, error) {
	if c.checkruns != nil {
		return c.checkruns, nil
	}
	return &github.CheckRunList{}, nil
}

func (c fgc) BotUser() (*github.UserData, error) {
	if c.botName == "error" {
		return nil, errors.New("injected BotUser() error")
	}
	return &github.UserData{Login: c.botName}, nil
}

func newGitHubClientCreator(tokenUsers map[string]fgc) githubClientCreator {
	return func(accessToken string) GitHubClient {
		who, ok := tokenUsers[accessToken]
		if !ok {
			panic("unexpected access token: " + accessToken)
		}
		return who
	}
}

func newMockQueryHandler(prs []PullRequest, contextMap map[int][]Context) *MockQueryHandler {
	return &MockQueryHandler{
		prs:        prs,
		contextMap: contextMap,
	}
}

func createMockAgent(repos []string, config *githuboauth.Config) *DashboardAgent {
	return &DashboardAgent{
		repos: repos,
		goac:  config,
		log:   logrus.WithField("unit-test", "dashboard-agent"),
	}
}

func TestHandlePrStatusWithoutLogin(t *testing.T) {
	repos := []string{"mock/repo", "kubernetes/test-infra", "foo/bar"}
	mockCookieStore := sessions.NewCookieStore([]byte("secret-key"))
	mockConfig := &githuboauth.Config{
		CookieStore: mockCookieStore,
	}
	mockAgent := createMockAgent(repos, mockConfig)
	mockData := UserData{
		Login: false,
	}

	rr := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/pr-data.js", nil)

	mockQueryHandler := newMockQueryHandler(nil, nil)

	ghClientCreator := newGitHubClientCreator(map[string]fgc{"should-not-find-me": {}})
	prHandler := mockAgent.HandlePrStatus(mockQueryHandler, ghClientCreator)
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

func TestHandlePrStatusWithInvalidToken(t *testing.T) {
	logrus.SetLevel(logrus.ErrorLevel)
	repos := []string{"mock/repo", "kubernetes/test-infra", "foo/bar"}
	mockCookieStore := sessions.NewCookieStore([]byte("secret-key"))
	mockConfig := &githuboauth.Config{
		CookieStore: mockCookieStore,
	}
	mockAgent := createMockAgent(repos, mockConfig)
	mockQueryHandler := newMockQueryHandler([]PullRequest{}, map[int][]Context{})

	rr := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/pr-data.js", nil)
	request.AddCookie(&http.Cookie{Name: tokenSession, Value: "garbage"})
	ghClientCreator := newGitHubClientCreator(map[string]fgc{"should-not-find-me": {}})
	prHandler := mockAgent.HandlePrStatus(mockQueryHandler, ghClientCreator)
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

	expectedData := UserData{Login: false}
	if !reflect.DeepEqual(dataReturned, expectedData) {
		t.Fatalf("Invalid user data. Got %v, expected %v.", dataReturned, expectedData)
	}
}

func TestHandlePrStatusWithLogin(t *testing.T) {
	repos := []string{"mock/repo", "kubernetes/test-infra", "foo/bar"}
	mockCookieStore := sessions.NewCookieStore([]byte("secret-key"))
	mockConfig := &githuboauth.Config{
		CookieStore: mockCookieStore,
	}
	mockAgent := createMockAgent(repos, mockConfig)

	testCases := []struct {
		prs          []PullRequest
		contextMap   map[int][]Context
		expectedData UserData
	}{
		{
			prs:        []PullRequest{},
			contextMap: map[int][]Context{},
			expectedData: UserData{
				Login: true,
			},
		},
		{
			prs: []PullRequest{
				{
					Number: 0,
					Title:  "random pull request",
				},
				{
					Number: 1,
					Title:  "This is a test",
				},
				{
					Number: 2,
					Title:  "test pull request",
				},
			},
			contextMap: map[int][]Context{
				0: {
					{
						Context:     "gofmt-job",
						Description: "job succeed",
						State:       "SUCCESS",
					},
				},
				1: {
					{
						Context:     "verify-bazel-job",
						Description: "job failed",
						State:       "FAILURE",
					},
				},
				2: {
					{
						Context:     "gofmt-job",
						Description: "job succeed",
						State:       "SUCCESS",
					},
					{
						Context:     "verify-bazel-job",
						Description: "job failed",
						State:       "FAILURE",
					},
				},
			},
			expectedData: UserData{
				Login: true,
				PullRequestsWithContexts: []PullRequestWithContexts{
					{
						PullRequest: PullRequest{
							Number: 0,
							Title:  "random pull request",
						},
						Contexts: []Context{
							{
								Context:     "gofmt-job",
								Description: "job succeed",
								State:       "SUCCESS",
							},
						},
					},
					{
						PullRequest: PullRequest{
							Number: 1,
							Title:  "This is a test",
						},
						Contexts: []Context{
							{
								Context:     "verify-bazel-job",
								Description: "job failed",
								State:       "FAILURE",
							},
						},
					},
					{
						PullRequest: PullRequest{
							Number: 2,
							Title:  "test pull request",
						},
						Contexts: []Context{
							{
								Context:     "gofmt-job",
								Description: "job succeed",
								State:       "SUCCESS",
							},
							{
								Context:     "verify-bazel-job",
								Description: "job failed",
								State:       "FAILURE",
							},
						},
					},
				},
			},
		},
	}
	for id, testcase := range testCases {
		t.Run(strconv.Itoa(id), func(t *testing.T) {
			rr := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/pr-data.js", nil)
			mockSession, err := sessions.GetRegistry(request).Get(mockCookieStore, tokenSession)
			if err != nil {
				t.Errorf("Error with creating mock session: %v", err)
			}
			gob.Register(oauth2.Token{})
			const (
				accessToken = "secret-token"
				botName     = "random_user"
			)
			token := &oauth2.Token{AccessToken: accessToken, Expiry: time.Now().Add(time.Duration(24*365) * time.Hour)}
			mockSession.Values[tokenKey] = token
			mockSession.Values[loginKey] = botName
			mockQueryHandler := newMockQueryHandler(testcase.prs, testcase.contextMap)
			ghClientCreator := newGitHubClientCreator(map[string]fgc{accessToken: {botName: botName}})
			prHandler := mockAgent.HandlePrStatus(mockQueryHandler, ghClientCreator)
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
			if !reflect.DeepEqual(dataReturned, testcase.expectedData) {
				t.Fatalf("Invalid user data. Got %v, expected %v.", dataReturned, testcase.expectedData)
			}
			t.Logf("Passed")
		})
	}
}

func TestGetHeadContexts(t *testing.T) {
	repos := []string{"mock/repo", "kubernetes/test-infra", "foo/bar"}
	mockCookieStore := sessions.NewCookieStore([]byte("secret-key"))
	mockConfig := &githuboauth.Config{
		CookieStore: mockCookieStore,
	}
	mockAgent := createMockAgent(repos, mockConfig)
	testCases := []struct {
		combinedStatus   *github.CombinedStatus
		checkruns        *github.CheckRunList
		expectedContexts []Context
	}{
		{
			combinedStatus:   &github.CombinedStatus{},
			expectedContexts: []Context{},
		},
		{
			combinedStatus: &github.CombinedStatus{
				Statuses: []github.Status{
					{
						State:       "FAILURE",
						Description: "job failed",
						Context:     "gofmt-job",
					},
					{
						State:       "SUCCESS",
						Description: "job succeed",
						Context:     "k8s-job",
					},
					{
						State:       "PENDING",
						Description: "triggered",
						Context:     "test-job",
					},
				},
			},
			expectedContexts: []Context{
				{
					State:       "FAILURE",
					Context:     "gofmt-job",
					Description: "job failed",
				},
				{
					State:       "SUCCESS",
					Description: "job succeed",
					Context:     "k8s-job",
				},
				{
					State:       "PENDING",
					Description: "triggered",
					Context:     "test-job",
				},
			},
		},
		{
			combinedStatus: &github.CombinedStatus{
				Statuses: []github.Status{
					{
						State:       "FAILURE",
						Description: "job failed",
						Context:     "gofmt-job",
					},
					{
						State:       "SUCCESS",
						Description: "job succeed",
						Context:     "k8s-job",
					},
					{
						State:       "PENDING",
						Description: "triggered",
						Context:     "test-job",
					},
				},
			},
			checkruns: &github.CheckRunList{
				CheckRuns: []github.CheckRun{
					{Name: "incomplete-checkrun"},
					{Name: "neutral-is-considered-success-checkrun", CompletedAt: "2000 BC", Conclusion: "neutral"},
					{Name: "success-checkrun", CompletedAt: "1900 BC", Conclusion: "success"},
					{Name: "failure-checkrun", CompletedAt: "1800 BC", Conclusion: "failure"},
				},
			},
			expectedContexts: []Context{
				{
					State:       "FAILURE",
					Context:     "gofmt-job",
					Description: "job failed",
				},
				{
					State:       "SUCCESS",
					Description: "job succeed",
					Context:     "k8s-job",
				},
				{
					State:       "PENDING",
					Description: "triggered",
					Context:     "test-job",
				},
				{
					State:   "PENDING",
					Context: "incomplete-checkrun",
				},
				{
					State:   "SUCCESS",
					Context: "neutral-is-considered-success-checkrun",
				},
				{
					State:   "SUCCESS",
					Context: "success-checkrun",
				},
				{
					State:   "FAILURE",
					Context: "failure-checkrun",
				},
			},
		},
	}
	for id, testcase := range testCases {
		t.Run(strconv.Itoa(id), func(t *testing.T) {
			contexts, err := mockAgent.getHeadContexts(&fgc{
				combinedStatus: testcase.combinedStatus,
				checkruns:      testcase.checkruns,
			}, PullRequest{})
			if err != nil {
				t.Fatalf("Error with getting head contexts")
			}
			if diff := cmp.Diff(contexts, testcase.expectedContexts); diff != "" {
				t.Fatalf("contexts differ from expected: %s", diff)
			}
		})
	}
}

func TestConstructSearchQuery(t *testing.T) {
	repos := []string{"mock/repo", "kubernetes/test-infra", "foo/bar"}
	mockCookieStore := sessions.NewCookieStore([]byte("secret-key"))
	mockConfig := &githuboauth.Config{
		CookieStore: mockCookieStore,
	}
	mockAgent := createMockAgent(repos, mockConfig)
	query := mockAgent.ConstructSearchQuery("random_username")
	mockQuery := "is:pr state:open author:random_username repo:\"mock/repo\" repo:\"kubernetes/test-infra\" repo:\"foo/bar\""
	if query != mockQuery {
		t.Errorf("Invalid query. Got: %v, expected %v", query, mockQuery)
	}
}
