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
	"io"
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
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/githuboauth"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/plugins"
	"sigs.k8s.io/yaml"
)

func getPresubmitConfig() *config.Config {
	presubmitJobs := []config.Presubmit{
		{
			JobBase: config.JobBase{
				Name: "whoa",
				Labels: map[string]string{
					"foo": "foo",
				},
				Annotations: map[string]string{
					"foo": "foo",
				},
				RerunAuthConfig: &prowapi.RerunAuthConfig{
					AllowAnyone:   false,
					GitHubUsers:   []string{"authorized", "alsoauthorized"},
					GitHubTeamIDs: []int{42},
				},
			},
			Brancher: config.Brancher{
				Branches: []string{"master"},
			},
		},
	}
	config.SetPresubmitRegexes(presubmitJobs)
	return &config.Config{
		JobConfig: config.JobConfig{
			PresubmitsStatic: map[string][]config.Presubmit{"org/repo": presubmitJobs},
		},
	}
}

func getPeriodicConfig() *config.Config {
	return &config.Config{
		JobConfig: config.JobConfig{
			Periodics: []config.Periodic{{
				JobBase: config.JobBase{
					Name: "whoa",
					Labels: map[string]string{
						"foo": "foo",
					},
					Annotations: map[string]string{
						"foo": "foo",
					},
					RerunAuthConfig: &prowapi.RerunAuthConfig{
						AllowAnyone:   false,
						GitHubUsers:   []string{"authorized", "alsoauthorized"},
						GitHubTeamIDs: []int{42},
					},
				},
			}},
		},
	}
}

// TestRerun just checks that the result can be unmarshaled properly, has an
// updated status, and has equal spec.
func TestRerun(t *testing.T) {
	testCases := []struct {
		name                string
		login               string
		authorized          []string
		allowAnyone         bool
		rerunCreatesJob     bool
		shouldCreateProwJob bool
		httpCode            int
		httpMethod          string
	}{
		{
			name:                "Handler returns ProwJob",
			login:               "authorized",
			authorized:          []string{"authorized", "alsoauthorized"},
			allowAnyone:         false,
			rerunCreatesJob:     true,
			shouldCreateProwJob: true,
			httpCode:            http.StatusOK,
			httpMethod:          http.MethodPost,
		},
		{
			name:                "User not authorized to create prow job",
			login:               "random-dude",
			authorized:          []string{"authorized", "alsoauthorized"},
			allowAnyone:         false,
			rerunCreatesJob:     true,
			shouldCreateProwJob: false,
			httpCode:            http.StatusOK,
			httpMethod:          http.MethodPost,
		},
		{
			name:                "RerunCreatesJob set to false, should not create prow job",
			login:               "authorized",
			authorized:          []string{"authorized", "alsoauthorized"},
			allowAnyone:         true,
			rerunCreatesJob:     false,
			shouldCreateProwJob: false,
			httpCode:            http.StatusOK,
			httpMethod:          http.MethodGet,
		},
		{
			name:                "Allow anyone set to true, creates job",
			login:               "ugh",
			authorized:          []string{"authorized", "alsoauthorized"},
			allowAnyone:         true,
			rerunCreatesJob:     true,
			shouldCreateProwJob: true,
			httpCode:            http.StatusOK,
			httpMethod:          http.MethodPost,
		},
		{
			name:                "Direct rerun disabled, post request",
			login:               "authorized",
			authorized:          []string{"authorized", "alsoauthorized"},
			allowAnyone:         true,
			rerunCreatesJob:     false,
			shouldCreateProwJob: false,
			httpCode:            http.StatusMethodNotAllowed,
			httpMethod:          http.MethodPost,
		},
		{
			name:                "User permitted on specific job",
			login:               "authorized",
			authorized:          []string{},
			allowAnyone:         false,
			rerunCreatesJob:     true,
			shouldCreateProwJob: true,
			httpCode:            http.StatusOK,
			httpMethod:          http.MethodPost,
		},
		{
			name:                "User on permitted team",
			login:               "sig-lead",
			authorized:          []string{},
			allowAnyone:         false,
			rerunCreatesJob:     true,
			shouldCreateProwJob: true,
			httpCode:            http.StatusOK,
			httpMethod:          http.MethodPost,
		},
		{
			name:                "Org member permitted for presubmits",
			login:               "org-member",
			authorized:          []string{},
			allowAnyone:         false,
			rerunCreatesJob:     true,
			shouldCreateProwJob: true,
			httpCode:            http.StatusOK,
			httpMethod:          http.MethodPost,
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
					State: prowapi.PendingState,
				},
			})
			authCfgGetter := func(refs *prowapi.ProwJobSpec) *prowapi.RerunAuthConfig {
				return &prowapi.RerunAuthConfig{
					AllowAnyone: tc.allowAnyone,
					GitHubUsers: tc.authorized,
				}
			}

			req, err := http.NewRequest(tc.httpMethod, "/rerun?prowjob=wowsuch", nil)
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
			cfg := func() *config.Config { return &config.Config{} }
			handler := handleRerun(cfg, fakeProwJobClient.ProwV1().ProwJobs("prowjobs"), tc.rerunCreatesJob, authCfgGetter, goa, ghc, rc, &pca, logrus.WithField("handler", "/rerun"))
			handler.ServeHTTP(rr, req)
			if rr.Code != tc.httpCode {
				t.Fatalf("Bad error code: %d", rr.Code)
			}

			if tc.shouldCreateProwJob {
				pjs, err := fakeProwJobClient.ProwV1().ProwJobs("prowjobs").List(context.Background(), metav1.ListOptions{})
				if err != nil {
					t.Fatalf("failed to list prowjobs: %v", err)
				}
				if numPJs := len(pjs.Items); numPJs != 2 {
					t.Errorf("expected to get two prowjobs, got %d", numPJs)
				}

			} else if !tc.rerunCreatesJob && tc.httpCode == http.StatusOK {
				resp := rr.Result()
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("Error reading response body: %v", err)
				}
				var res prowapi.ProwJob
				if err := yaml.Unmarshal(body, &res); err != nil {
					t.Fatalf("Error unmarshaling: %v", err)
				}
				if res.Spec.Job != "whoa" {
					t.Errorf("Wrong job, expected \"whoa\", got \"%s\"", res.Spec.Job)
				}
				if res.Status.State != prowapi.TriggeredState {
					t.Errorf("Wrong state, expected \"%v\", got \"%v\"", prowapi.TriggeredState, res.Status.State)
				}
			}
		})
	}
}

// TestLatestRerun just checks that the result can be unmarshaled properly, has an
// updated status, and has equal spec.
func TestLatestRerun(t *testing.T) {
	testCases := []struct {
		name                string
		pjType              prowapi.ProwJobType
		config              *config.Config
		login               string
		authorized          []string
		allowAnyone         bool
		rerunCreatesJob     bool
		shouldCreateProwJob bool
		reported            bool
		httpCode            int
		httpMethod          string
	}{
		{
			name:                "Handler returns Presubmit ProwJob",
			pjType:              prowapi.PresubmitJob,
			config:              getPresubmitConfig(),
			login:               "authorized",
			authorized:          []string{"authorized", "alsoauthorized"},
			allowAnyone:         false,
			rerunCreatesJob:     true,
			shouldCreateProwJob: true,
			reported:            false,
			httpCode:            http.StatusOK,
			httpMethod:          http.MethodPost,
		},
		{
			name:                "Handler returns Periodic ProwJob",
			pjType:              prowapi.PeriodicJob,
			config:              getPeriodicConfig(),
			login:               "authorized",
			authorized:          []string{"authorized", "alsoauthorized"},
			allowAnyone:         false,
			rerunCreatesJob:     true,
			shouldCreateProwJob: true,
			reported:            false,
			httpCode:            http.StatusOK,
			httpMethod:          http.MethodPost,
		},
		{
			name:                "User not authorized to create prow job",
			pjType:              prowapi.PresubmitJob,
			config:              getPresubmitConfig(),
			login:               "random-dude",
			authorized:          []string{"authorized", "alsoauthorized"},
			allowAnyone:         false,
			rerunCreatesJob:     true,
			shouldCreateProwJob: false,
			reported:            false,
			httpCode:            http.StatusOK,
			httpMethod:          http.MethodPost,
		},
		{
			name:                "RerunCreatesJob set to false, should not create prow job",
			pjType:              prowapi.PresubmitJob,
			config:              getPresubmitConfig(),
			login:               "authorized",
			authorized:          []string{"authorized", "alsoauthorized"},
			allowAnyone:         true,
			rerunCreatesJob:     false,
			shouldCreateProwJob: false,
			reported:            true,
			httpCode:            http.StatusOK,
			httpMethod:          http.MethodGet,
		},
		{
			name:                "Allow anyone set to true, creates job",
			pjType:              prowapi.PresubmitJob,
			config:              getPresubmitConfig(),
			login:               "ugh",
			authorized:          []string{"authorized", "alsoauthorized"},
			allowAnyone:         true,
			rerunCreatesJob:     true,
			shouldCreateProwJob: true,
			reported:            false,
			httpCode:            http.StatusOK,
			httpMethod:          http.MethodPost,
		},
		{
			name:                "Direct rerun disabled, post request",
			pjType:              prowapi.PresubmitJob,
			config:              getPresubmitConfig(),
			login:               "authorized",
			authorized:          []string{"authorized", "alsoauthorized"},
			allowAnyone:         true,
			rerunCreatesJob:     false,
			shouldCreateProwJob: false,
			httpCode:            http.StatusMethodNotAllowed,
			httpMethod:          http.MethodPost,
		},
		{
			name:                "User permitted on specific job",
			pjType:              prowapi.PresubmitJob,
			config:              getPresubmitConfig(),
			login:               "authorized",
			authorized:          []string{},
			allowAnyone:         false,
			rerunCreatesJob:     true,
			shouldCreateProwJob: true,
			reported:            false,
			httpCode:            http.StatusOK,
			httpMethod:          http.MethodPost,
		},
		{
			name:                "User on permitted team",
			pjType:              prowapi.PresubmitJob,
			config:              getPresubmitConfig(),
			login:               "sig-lead",
			authorized:          []string{},
			allowAnyone:         false,
			rerunCreatesJob:     true,
			shouldCreateProwJob: true,
			reported:            false,
			httpCode:            http.StatusOK,
			httpMethod:          http.MethodPost,
		},
		{
			name:                "Org member permitted for presubmits",
			pjType:              prowapi.PresubmitJob,
			config:              getPresubmitConfig(),
			login:               "org-member",
			authorized:          []string{},
			allowAnyone:         false,
			rerunCreatesJob:     true,
			shouldCreateProwJob: true,
			reported:            false,
			httpCode:            http.StatusOK,
			httpMethod:          http.MethodPost,
		},
		{
			name:                "Org member not permitted for periodic",
			pjType:              prowapi.PeriodicJob,
			config:              getPeriodicConfig(),
			login:               "org-member",
			authorized:          []string{},
			allowAnyone:         false,
			rerunCreatesJob:     true,
			shouldCreateProwJob: false,
			reported:            false,
			httpCode:            http.StatusOK,
			httpMethod:          http.MethodPost,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeProwJobClient := fake.NewSimpleClientset(&prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wowsuch",
					Namespace: "prowjobs",
					Labels: map[string]string{
						kube.GerritReportLabel: "foo",
						"random":               "foo",
					},
					Annotations: map[string]string{
						kube.GerritID: "foo",
						"random":      "foo",
					},
				},
				Spec: prowapi.ProwJobSpec{
					Job:  "whoa",
					Type: tc.pjType,
					Refs: &prowapi.Refs{
						Org:  "org",
						Repo: "repo",
						Pulls: []prowapi.Pull{
							{
								Number: 1,
								Author: tc.login,
							},
						},
						BaseSHA: "foo",
						BaseRef: "master",
					},
					RerunAuthConfig: &prowapi.RerunAuthConfig{
						AllowAnyone: false,
						GitHubUsers: []string{"random", "random"},
					},
				},
				Status: prowapi.ProwJobStatus{
					State: prowapi.PendingState,
				},
			})
			authCfgGetter := func(refs *prowapi.ProwJobSpec) *prowapi.RerunAuthConfig {
				return &prowapi.RerunAuthConfig{
					AllowAnyone: tc.allowAnyone,
					GitHubUsers: tc.authorized,
				}
			}

			req, err := http.NewRequest(tc.httpMethod, "/rerun?mode=latest&prowjob=wowsuch", nil)
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
			cfg := func() *config.Config {
				return tc.config
			}
			handler := handleRerun(cfg, fakeProwJobClient.ProwV1().ProwJobs("prowjobs"), tc.rerunCreatesJob, authCfgGetter, goa, ghc, rc, &pca, logrus.WithField("handler", "/rerun"))
			handler.ServeHTTP(rr, req)
			if rr.Code != tc.httpCode {
				t.Fatalf("Bad error code: %d", rr.Code)
			}

			if tc.shouldCreateProwJob {
				pjs, err := fakeProwJobClient.ProwV1().ProwJobs("prowjobs").List(context.Background(), metav1.ListOptions{})
				if err != nil {
					t.Fatalf("failed to list prowjobs: %v", err)
				}
				if numPJs := len(pjs.Items); numPJs != 2 {
					t.Errorf("expected to get two prowjobs, got %d", numPJs)
				}
			} else if !tc.rerunCreatesJob && tc.httpCode == http.StatusOK {
				expectedProwJob := prowapi.ProwJob{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ProwJob",
						APIVersion: "prow.k8s.io/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"created-by-prow":                 "true",
							"foo":                             "foo",
							"prow.k8s.io/context":             "",
							"prow.k8s.io/gerrit-report-label": "foo",
							"prow.k8s.io/job":                 "whoa",
							"prow.k8s.io/refs.base_ref":       "master",
							"prow.k8s.io/refs.org":            "org",
							"prow.k8s.io/refs.pull":           "1",
							"prow.k8s.io/refs.repo":           "repo",
							"prow.k8s.io/type":                string(tc.pjType),
						},
						Annotations: map[string]string{
							"foo":                   "foo",
							"prow.k8s.io/context":   "",
							"prow.k8s.io/gerrit-id": "foo",
							"prow.k8s.io/job":       "whoa",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Job:  "whoa",
						Type: tc.pjType,
						Refs: &prowapi.Refs{
							Org:  "org",
							Repo: "repo",
							Pulls: []prowapi.Pull{
								{
									Number: 1,
									Author: tc.login,
								},
							},
							BaseSHA: "foo",
							BaseRef: "master",
						},
						Report: tc.reported,
						RerunAuthConfig: &prowapi.RerunAuthConfig{
							AllowAnyone:   false,
							GitHubUsers:   tc.authorized,
							GitHubTeamIDs: []int{42},
						},
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.TriggeredState,
					},
				}

				resp := rr.Result()
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("Error reading response body: %v", err)
				}
				var res prowapi.ProwJob
				if err := yaml.Unmarshal(body, &res); err != nil {
					t.Fatalf("Error unmarshaling: %v", err)
				}
				// These two fields are undeterministic so there are being set to the default
				res.Status.StartTime = metav1.Time{}
				res.ObjectMeta.Name = ""
				if diff := cmp.Diff(expectedProwJob, res); diff != "" {
					t.Fatalf("Job mismatch. Want: (-), got: (+). \n%s", diff)
				}
			}
		})
	}
}

func TestCanTriggerJob(t *testing.T) {
	t.Parallel()
	org := "org"
	trustedUser := "trusted"
	untrustedUser := "untrusted"

	pcfg := &plugins.Configuration{
		Triggers: []plugins.Trigger{{Repos: []string{org}}},
	}
	pcfgGetter := func() *plugins.Configuration { return pcfg }

	ghc := fakegithub.NewFakeClient()
	ghc.OrgMembers = map[string][]string{org: {trustedUser}}

	pj := prowapi.ProwJob{
		Spec: prowapi.ProwJobSpec{
			Refs: &prowapi.Refs{
				Org:   org,
				Repo:  "repo",
				Pulls: []prowapi.Pull{{Author: trustedUser}},
			},
			Type: prowapi.PresubmitJob,
		},
	}
	testCases := []struct {
		name          string
		user          string
		expectAllowed bool
	}{
		{
			name:          "Unauthorized user can not rerun",
			user:          untrustedUser,
			expectAllowed: false,
		},
		{
			name:          "Authorized user can re-run",
			user:          trustedUser,
			expectAllowed: true,
		},
	}

	log := logrus.NewEntry(logrus.StandardLogger())
	for _, tc := range testCases {
		result, err := canTriggerJob(tc.user, pj, nil, ghc, pcfgGetter, log)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if result != tc.expectAllowed {
			t.Errorf("got result %t, expected %t", result, tc.expectAllowed)
		}
	}
}
