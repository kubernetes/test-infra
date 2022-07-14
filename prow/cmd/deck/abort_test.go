/*
Copyright 2022 The Kubernetes Authors.

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

package main

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/gorilla/sessions"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/client/clientset/versioned/fake"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/githuboauth"
	"k8s.io/test-infra/prow/plugins"
	"sigs.k8s.io/yaml"
)

// TestAbort that an aborted job has an updated status and
// that permissions were granted appropriately
func TestAbort(t *testing.T) {
	testCases := []struct {
		name        string
		login       string
		authorized  []string
		allowAnyone bool
		jobState    prowapi.ProwJobState
		httpCode    int
		httpMethod  string
	}{
		{
			name:        "Abort on triggered state",
			login:       "authorized",
			authorized:  []string{"authorized", "alsoauthorized"},
			allowAnyone: false,
			jobState:    prowapi.TriggeredState,
			httpCode:    http.StatusOK,
			httpMethod:  http.MethodPost,
		},
		{
			name:        "Abort on pending state",
			login:       "authorized",
			authorized:  []string{"authorized", "alsoauthorized"},
			allowAnyone: false,
			jobState:    prowapi.PendingState,
			httpCode:    http.StatusOK,
			httpMethod:  http.MethodPost,
		},
		{
			name:        "Attempt to abort on success state",
			login:       "authorized",
			authorized:  []string{"authorized", "alsoauthorized"},
			allowAnyone: false,
			jobState:    prowapi.SuccessState,
			httpCode:    http.StatusBadRequest,
			httpMethod:  http.MethodPost,
		},
		{
			name:        "Attempt to abort on aborted state",
			login:       "authorized",
			authorized:  []string{"authorized", "alsoauthorized"},
			allowAnyone: false,
			jobState:    prowapi.AbortedState,
			httpCode:    http.StatusBadRequest,
			httpMethod:  http.MethodPost,
		},
		{
			name:        "User not authorized to abort job",
			login:       "random-dude",
			authorized:  []string{"authorized", "alsoauthorized"},
			allowAnyone: false,
			jobState:    prowapi.PendingState,
			httpCode:    http.StatusUnauthorized,
			httpMethod:  http.MethodPost,
		},
		{
			name:        "Allow anyone set to true, abort job",
			login:       "ugh",
			authorized:  []string{"authorized", "alsoauthorized"},
			allowAnyone: true,
			jobState:    prowapi.PendingState,
			httpCode:    http.StatusOK,
			httpMethod:  http.MethodPost,
		},
		{
			name:        "User permitted to abort on specific job",
			login:       "authorized",
			authorized:  []string{},
			allowAnyone: false,
			jobState:    prowapi.PendingState,
			httpCode:    http.StatusOK,
			httpMethod:  http.MethodPost,
		},
		{
			name:        "User on permitted team",
			login:       "sig-lead",
			authorized:  []string{},
			allowAnyone: false,
			jobState:    prowapi.PendingState,
			httpCode:    http.StatusOK,
			httpMethod:  http.MethodPost,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeProwJobClient := fake.NewSimpleClientset(&prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wowsuch",
					Namespace: "prowjobs",
				},
				Spec: prowapi.ProwJobSpec{
					Job:  "whoa",
					Type: prowapi.PresubmitJob,
					Refs: &prowapi.Refs{
						Org:  "org",
						Repo: "repo",
						Pulls: []prowapi.Pull{
							{
								Number: 1,
								Author: tc.login,
							},
						},
					},
					RerunAuthConfig: &prowapi.RerunAuthConfig{
						AllowAnyone:   false,
						GitHubUsers:   []string{"authorized", "alsoauthorized"},
						GitHubTeamIDs: []int{42},
					},
				},
				Status: prowapi.ProwJobStatus{
					State: tc.jobState,
				},
			})
			authCfgGetter := func(refs *prowapi.Refs, cluster string) *prowapi.RerunAuthConfig {
				return &prowapi.RerunAuthConfig{
					AllowAnyone: tc.allowAnyone,
					GitHubUsers: tc.authorized,
				}
			}

			req, err := http.NewRequest(tc.httpMethod, "/abort?prowjob=wowsuch", nil)
			if err != nil {
				t.Fatalf("Error making request: %v", err)
			}
			req.AddCookie(&http.Cookie{
				Name:    "github_login",
				Value:   tc.login,
				Path:    "/",
				Expires: time.Now().Add(time.Hour * 24 * 30),
				Secure:  true,
			})
			mockCookieStore := sessions.NewCookieStore([]byte("secret-key"))
			session, err := sessions.GetRegistry(req).Get(mockCookieStore, "access-token-session")
			if err != nil {
				t.Fatalf("Error making access token session: %v", err)
			}
			session.Values["access-token"] = &oauth2.Token{AccessToken: "validtoken"}

			rr := httptest.NewRecorder()
			mockConfig := &githuboauth.Config{
				CookieStore: mockCookieStore,
			}
			goa := githuboauth.NewAgent(mockConfig, &logrus.Entry{})
			ghc := &fakeAuthenticatedUserIdentifier{login: tc.login}
			rc := fakegithub.NewFakeClient()
			rc.OrgMembers = map[string][]string{"org": {"org-member"}}
			pca := plugins.NewFakeConfigAgent()
			handler := handleAbort(fakeProwJobClient.ProwV1().ProwJobs("prowjobs"), authCfgGetter, goa, ghc, rc, &pca, logrus.WithField("handler", "/abort"))
			handler.ServeHTTP(rr, req)
			if rr.Code != tc.httpCode {
				t.Fatalf("Bad error code: %d", rr.Code)
			}

			if tc.httpCode == http.StatusOK {
				pj, err := fakeProwJobClient.ProwV1().ProwJobs("prowjobs").Get(context.TODO(), "wowsuch", metav1.GetOptions{})
				if err != nil {
					t.Fatalf("Job not found: %v", err)
				}
				resp := rr.Result()
				defer resp.Body.Close()
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("Error reading response body: %v", err)
				}
				var res prowapi.ProwJob
				if err := yaml.Unmarshal(body, &res); err != nil {
					t.Fatalf("Error unmarshaling: %v", err)
				}
				if !res.Complete() {
					t.Error("Job is not complete, was expected to be complete")
				}
				if res.Status.State != prowapi.AbortedState {
					t.Errorf("Wrong state, expected \"%v\", got \"%v\"", prowapi.AbortedState, res.Status.State)
				}
				if diff := cmp.Diff(res, *pj); diff != "" {
					t.Fatalf("Job mismatch. Want: (-), got: (+). \n%s", diff)
				}
			}
		})
	}
}
