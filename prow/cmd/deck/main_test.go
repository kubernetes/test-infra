/*
Copyright 2016 The Kubernetes Authors.

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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/sessions"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	coreapi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/client/clientset/versioned/fake"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/deck/jobs"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/githuboauth"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	_ "k8s.io/test-infra/prow/spyglass/lenses/buildlog"
	"k8s.io/test-infra/prow/spyglass/lenses/common"
	_ "k8s.io/test-infra/prow/spyglass/lenses/junit"
	_ "k8s.io/test-infra/prow/spyglass/lenses/metadata"
	"k8s.io/test-infra/prow/tide"
	"k8s.io/test-infra/prow/tide/history"
)

type fkc []prowapi.ProwJob

func (f fkc) List(ctx context.Context, pjs *prowapi.ProwJobList, _ ...ctrlruntimeclient.ListOption) error {
	pjs.Items = f
	return nil
}

type fca struct {
	c config.Config
}

func (ca fca) Config() *config.Config {
	return &ca.c
}

func TestOptions_Validate(t *testing.T) {
	var testCases = []struct {
		name        string
		input       options
		expectedErr bool
	}{
		{
			name: "minimal set ok",
			input: options{
				configPath: "test",
			},
			expectedErr: false,
		},
		{
			name:        "missing configpath",
			input:       options{},
			expectedErr: true,
		},
		{
			name: "ok with oauth",
			input: options{
				configPath:            "test",
				oauthURL:              "website",
				githubOAuthConfigFile: "something",
				cookieSecretFile:      "yum",
			},
			expectedErr: false,
		},
		{
			name: "missing github config with oauth",
			input: options{
				configPath:       "test",
				oauthURL:         "website",
				cookieSecretFile: "yum",
			},
			expectedErr: true,
		},
		{
			name: "missing cookie with oauth",
			input: options{
				configPath:            "test",
				oauthURL:              "website",
				githubOAuthConfigFile: "something",
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		err := testCase.input.Validate()
		if testCase.expectedErr && err == nil {
			t.Errorf("%s: expected an error but got none", testCase.name)
		}
		if !testCase.expectedErr && err != nil {
			t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
		}
	}
}

type flc int

func (f flc) GetJobLog(job, id, container string) ([]byte, error) {
	if job == "job" && id == "123" {
		return []byte("hello"), nil
	}
	return nil, errors.New("muahaha")
}

func TestHandleLog(t *testing.T) {
	var testcases = []struct {
		name string
		path string
		code int
	}{
		{
			name: "no job name",
			path: "",
			code: http.StatusBadRequest,
		},
		{
			name: "job but no id",
			path: "?job=job",
			code: http.StatusBadRequest,
		},
		{
			name: "id but no job",
			path: "?id=123",
			code: http.StatusBadRequest,
		},
		{
			name: "id and job, found",
			path: "?job=job&id=123",
			code: http.StatusOK,
		},
		{
			name: "id and job, not found",
			path: "?job=ohno&id=123",
			code: http.StatusNotFound,
		},
	}
	handler := handleLog(flc(0), logrus.WithField("handler", "/log"))
	for _, tc := range testcases {
		req, err := http.NewRequest(http.MethodGet, "", nil)
		if err != nil {
			t.Fatalf("Error making request: %v", err)
		}
		u, err := url.Parse(tc.path)
		if err != nil {
			t.Fatalf("Error parsing URL: %v", err)
		}
		var follow = false
		if ok, _ := strconv.ParseBool(u.Query().Get("follow")); ok {
			follow = true
		}
		req.URL = u
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != tc.code {
			t.Errorf("Wrong error code. Got %v, want %v", rr.Code, tc.code)
		} else if rr.Code == http.StatusOK {
			if follow {
				//wait a little to get the chunks
				time.Sleep(2 * time.Millisecond)
				reader := bufio.NewReader(rr.Body)
				var buf bytes.Buffer
				for {
					line, err := reader.ReadBytes('\n')
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatalf("Expecting reply with content but got error: %v", err)
					}
					buf.Write(line)
				}
				if !bytes.Contains(buf.Bytes(), []byte("hello")) {
					t.Errorf("Unexpected body: got %s.", buf.String())
				}
			} else {
				resp := rr.Result()
				defer resp.Body.Close()
				if body, err := ioutil.ReadAll(resp.Body); err != nil {
					t.Errorf("Error reading response body: %v", err)
				} else if string(body) != "hello" {
					t.Errorf("Unexpected body: got %s.", string(body))
				}
			}
		}
	}
}

// TestHandleProwJobs just checks that the results can be unmarshaled properly, have the same
func TestHandleProwJobs(t *testing.T) {
	kc := fkc{
		prowapi.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"hello": "world",
				},
				Labels: map[string]string{
					"goodbye": "world",
				},
			},
			Spec: prowapi.ProwJobSpec{
				Agent:            prowapi.KubernetesAgent,
				Job:              "job",
				DecorationConfig: &prowapi.DecorationConfig{},
				PodSpec: &coreapi.PodSpec{
					Containers: []coreapi.Container{
						{
							Name:  "test-1",
							Image: "tester1",
						},
						{
							Name:  "test-2",
							Image: "tester2",
						},
					},
				},
			},
		},
		prowapi.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"hello": "world",
				},
				Labels: map[string]string{
					"goodbye": "world",
				},
			},
			Spec: prowapi.ProwJobSpec{
				Agent:            prowapi.KubernetesAgent,
				Job:              "missing-podspec-job",
				DecorationConfig: &prowapi.DecorationConfig{},
			},
		},
	}
	fakeJa := jobs.NewJobAgent(context.Background(), kc, false, true, map[string]jobs.PodLogClient{}, fca{}.Config)
	fakeJa.Start()

	handler := handleProwJobs(fakeJa, logrus.WithField("handler", "/prowjobs.js"))
	req, err := http.NewRequest(http.MethodGet, "/prowjobs.js?omit=annotations,labels,decoration_config,pod_spec", nil)
	if err != nil {
		t.Fatalf("Error making request: %v", err)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("Bad error code: %d", rr.Code)
	}
	resp := rr.Result()
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Error reading response body: %v", err)
	}
	type prowjobItems struct {
		Items []prowapi.ProwJob `json:"items"`
	}
	var res prowjobItems
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("Error unmarshaling: %v", err)
	}
	if res.Items[0].Annotations != nil {
		t.Errorf("Failed to omit annotations correctly, expected: nil, got %v", res.Items[0].Annotations)
	}
	if res.Items[0].Labels != nil {
		t.Errorf("Failed to omit labels correctly, expected: nil, got %v", res.Items[0].Labels)
	}
	if res.Items[0].Spec.DecorationConfig != nil {
		t.Errorf("Failed to omit decoration config correctly, expected: nil, got %v", res.Items[0].Spec.DecorationConfig)
	}

	// this tests the behavior for filling a podspec with empty containers when asked to omit it
	emptyPodspec := &coreapi.PodSpec{
		Containers: []coreapi.Container{{}, {}},
	}
	if !equality.Semantic.DeepEqual(res.Items[0].Spec.PodSpec, emptyPodspec) {
		t.Errorf("Failed to omit podspec correctly\n%s", diff.ObjectReflectDiff(res.Items[0].Spec.PodSpec, emptyPodspec))
	}

	if res.Items[1].Spec.PodSpec != nil {
		t.Errorf("Failed to omit podspec correctly, expected: nil, got %v", res.Items[0].Spec.PodSpec)
	}
}

// TestProwJob just checks that the result can be unmarshaled properly, has
// the same status, and has equal spec.
func TestProwJob(t *testing.T) {
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
					{Number: 1},
				},
			},
		},
		Status: prowapi.ProwJobStatus{
			State: prowapi.PendingState,
		},
	})
	handler := handleProwJob(fakeProwJobClient.ProwV1().ProwJobs("prowjobs"), logrus.WithField("handler", "/prowjob"))
	req, err := http.NewRequest(http.MethodGet, "/prowjob?prowjob=wowsuch", nil)
	if err != nil {
		t.Fatalf("Error making request: %v", err)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("Bad error code: %d", rr.Code)
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
	if res.Spec.Job != "whoa" {
		t.Errorf("Wrong job, expected \"whoa\", got \"%s\"", res.Spec.Job)
	}
	if res.Status.State != prowapi.PendingState {
		t.Errorf("Wrong state, expected \"%v\", got \"%v\"", prowapi.PendingState, res.Status.State)
	}
}

type fakeAuthenticatedUserIdentifier struct {
	login string
}

func (a *fakeAuthenticatedUserIdentifier) LoginForRequester(requester, token string) (string, error) {
	return a.login, nil
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
			authCfgGetter := func(refs *prowapi.Refs) *prowapi.RerunAuthConfig {
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
			handler := handleRerun(fakeProwJobClient.ProwV1().ProwJobs("prowjobs"), tc.rerunCreatesJob, authCfgGetter, goa, ghc, rc, &pca, logrus.WithField("handler", "/rerun"))
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
				body, err := ioutil.ReadAll(resp.Body)
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

func TestTide(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pools := []tide.Pool{
			{
				Org: "o",
			},
		}
		b, err := json.Marshal(pools)
		if err != nil {
			t.Fatalf("Marshaling: %v", err)
		}
		fmt.Fprint(w, string(b))
	}))
	ca := &config.Agent{}
	ca.Set(&config.Config{
		ProwConfig: config.ProwConfig{
			Tide: config.Tide{
				Queries: []config.TideQuery{
					{Repos: []string{"prowapi.netes/test-infra"}},
				},
			},
		},
	})
	ta := tideAgent{
		path: s.URL,
		hiddenRepos: func() []string {
			return []string{}
		},
		updatePeriod: func() time.Duration { return time.Minute },
	}
	if err := ta.updatePools(); err != nil {
		t.Fatalf("Updating: %v", err)
	}
	if len(ta.pools) != 1 {
		t.Fatalf("Wrong number of pools. Got %d, expected 1 in %v", len(ta.pools), ta.pools)
	}
	if ta.pools[0].Org != "o" {
		t.Errorf("Wrong org in pool. Got %s, expected o in %v", ta.pools[0].Org, ta.pools)
	}
	handler := handleTidePools(ca.Config, &ta, logrus.WithField("handler", "/tide.js"))
	req, err := http.NewRequest(http.MethodGet, "/tide.js", nil)
	if err != nil {
		t.Fatalf("Error making request: %v", err)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("Bad error code: %d", rr.Code)
	}
	resp := rr.Result()
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Error reading response body: %v", err)
	}
	res := tidePools{}
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("Error unmarshaling: %v", err)
	}
	if len(res.Pools) != 1 {
		t.Fatalf("Wrong number of pools. Got %d, expected 1 in %v", len(res.Pools), res.Pools)
	}
	if res.Pools[0].Org != "o" {
		t.Errorf("Wrong org in pool. Got %s, expected o in %v", res.Pools[0].Org, res.Pools)
	}
	if len(res.Queries) != 1 {
		t.Fatalf("Wrong number of pools. Got %d, expected 1 in %v", len(res.Queries), res.Queries)
	}
	if expected := "is:pr state:open archived:false repo:\"prowapi.netes/test-infra\""; res.Queries[0] != expected {
		t.Errorf("Wrong query. Got %s, expected %s", res.Queries[0], expected)
	}
}

func TestTideHistory(t *testing.T) {
	testHist := map[string][]history.Record{
		"o/r:b": {
			{Action: "MERGE"}, {Action: "TRIGGER"},
		},
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := json.Marshal(testHist)
		if err != nil {
			t.Fatalf("Marshaling: %v", err)
		}
		fmt.Fprint(w, string(b))
	}))

	ta := tideAgent{
		path: s.URL,
		hiddenRepos: func() []string {
			return []string{}
		},
		updatePeriod: func() time.Duration { return time.Minute },
	}
	if err := ta.updateHistory(); err != nil {
		t.Fatalf("Updating: %v", err)
	}
	if !reflect.DeepEqual(ta.history, testHist) {
		t.Fatalf("Expected tideAgent history:\n%#v\n,but got:\n%#v\n", testHist, ta.history)
	}

	handler := handleTideHistory(&ta, logrus.WithField("handler", "/tide-history.js"))
	req, err := http.NewRequest(http.MethodGet, "/tide-history.js", nil)
	if err != nil {
		t.Fatalf("Error making request: %v", err)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("Bad error code: %d", rr.Code)
	}
	resp := rr.Result()
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Error reading response body: %v", err)
	}
	var res tideHistory
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("Error unmarshaling: %v", err)
	}
	if !reflect.DeepEqual(res.History, testHist) {
		t.Fatalf("Expected /tide-history.js:\n%#v\n,but got:\n%#v\n", testHist, res.History)
	}
}

func TestHelp(t *testing.T) {
	hitCount := 0
	help := pluginhelp.Help{
		AllRepos:            []string{"org/repo"},
		RepoPlugins:         map[string][]string{"org": {"plugin"}},
		RepoExternalPlugins: map[string][]string{"org/repo": {"external-plugin"}},
		PluginHelp:          map[string]pluginhelp.PluginHelp{"plugin": {Description: "plugin"}},
		ExternalPluginHelp:  map[string]pluginhelp.PluginHelp{"external-plugin": {Description: "external-plugin"}},
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount++
		b, err := json.Marshal(help)
		if err != nil {
			t.Fatalf("Marshaling: %v", err)
		}
		fmt.Fprint(w, string(b))
	}))
	ha := &helpAgent{
		path: s.URL,
	}
	handler := handlePluginHelp(ha, logrus.WithField("handler", "/plugin-help.js"))
	handleAndCheck := func() {
		req, err := http.NewRequest(http.MethodGet, "/plugin-help.js", nil)
		if err != nil {
			t.Fatalf("Error making request: %v", err)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("Bad error code: %d", rr.Code)
		}
		resp := rr.Result()
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Error reading response body: %v", err)
		}
		var res pluginhelp.Help
		if err := yaml.Unmarshal(body, &res); err != nil {
			t.Fatalf("Error unmarshaling: %v", err)
		}
		if !reflect.DeepEqual(help, res) {
			t.Errorf("Invalid plugin help. Got %v, expected %v", res, help)
		}
		if hitCount != 1 {
			t.Errorf("Expected fake hook endpoint to be hit once, but endpoint was hit %d times.", hitCount)
		}
	}
	handleAndCheck()
	handleAndCheck()
}

func Test_gatherOptions(t *testing.T) {
	cases := []struct {
		name     string
		args     map[string]string
		del      sets.String
		expected func(*options)
		err      bool
	}{
		{
			name: "minimal flags work",
		},
		{
			name: "explicitly set --config-path",
			args: map[string]string{
				"--config-path": "/random/value",
			},
			expected: func(o *options) {
				o.configPath = "/random/value"
			},
		},
		{
			name: "explicitly set both --hidden-only and --show-hidden to true",
			args: map[string]string{
				"--hidden-only": "true",
				"--show-hidden": "true",
			},
			err: true,
		},
		{
			name: "explicitly set --plugin-config",
			args: map[string]string{
				"--hidden-only": "true",
				"--show-hidden": "true",
			},
			err: true,
		},
	}
	for _, tc := range cases {
		fs := flag.NewFlagSet("fake-flags", flag.PanicOnError)
		ghoptions := flagutil.GitHubOptions{}
		ghoptions.AddFlags(fs)
		ghoptions.AllowAnonymous = true
		ghoptions.AllowDirectAccess = true
		t.Run(tc.name, func(t *testing.T) {
			expected := &options{
				configPath:            "yo",
				githubOAuthConfigFile: "/etc/github/secret",
				cookieSecretFile:      "",
				staticFilesLocation:   "/static",
				templateFilesLocation: "/template",
				spyglassFilesLocation: "/lenses",
				kubernetes:            flagutil.KubernetesOptions{},
				github:                ghoptions,
				instrumentation: flagutil.InstrumentationOptions{
					MetricsPort: flagutil.DefaultMetricsPort,
					PProfPort:   flagutil.DefaultPProfPort,
					HealthPort:  flagutil.DefaultHealthPort,
				},
			}
			if tc.expected != nil {
				tc.expected(expected)
			}

			argMap := map[string]string{
				"--config-path": "yo",
			}
			for k, v := range tc.args {
				argMap[k] = v
			}
			for k := range tc.del {
				delete(argMap, k)
			}

			var args []string
			for k, v := range argMap {
				args = append(args, k+"="+v)
			}
			fs := flag.NewFlagSet("fake-flags", flag.PanicOnError)
			actual := gatherOptions(fs, args...)
			switch err := actual.Validate(); {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("failed to receive expected error")
			case !reflect.DeepEqual(*expected, actual):
				t.Errorf("\n%#v\n!= expected\n%#v", actual, *expected)
			}
		})
	}

}

func TestHandleConfig(t *testing.T) {
	trueVal := true
	c := config.Config{
		JobConfig: config.JobConfig{
			PresubmitsStatic: map[string][]config.Presubmit{
				"org/repo": {
					{
						Reporter: config.Reporter{
							Context: "gce",
						},
						AlwaysRun: true,
					},
					{
						Reporter: config.Reporter{
							Context: "unit",
						},
						AlwaysRun: true,
					},
				},
			},
		},
		ProwConfig: config.ProwConfig{
			BranchProtection: config.BranchProtection{
				Orgs: map[string]config.Org{
					"kubernetes": {
						Policy: config.Policy{
							Protect: &trueVal,
							RequiredStatusChecks: &config.ContextPolicy{
								Strict: &trueVal,
							},
						},
					},
				},
			},
			Tide: config.Tide{
				Queries: []config.TideQuery{
					{Repos: []string{"prowapi.netes/test-infra"}},
				},
			},
		},
	}
	configGetter := func() *config.Config {
		return &c
	}
	handler := handleConfig(configGetter, logrus.WithField("handler", "/config"))
	req, err := http.NewRequest(http.MethodGet, "/config", nil)
	if err != nil {
		t.Fatalf("Error making request: %v", err)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("Bad error code: %d", rr.Code)
	}
	if h := rr.Header().Get("Content-Type"); h != "text/plain" {
		t.Fatalf("Bad Content-Type, expected: 'text/plain', got: %v", h)
	}
	resp := rr.Result()
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Error reading response body: %v", err)
	}
	var res config.Config
	if err := yaml.Unmarshal(body, &res); err != nil {
		t.Fatalf("Error unmarshaling: %v", err)
	}
	if !reflect.DeepEqual(c, res) {
		t.Errorf("Invalid config. Got %v, expected %v", res, c)
	}
}

func TestHandlePluginConfig(t *testing.T) {
	c := plugins.Configuration{
		Plugins: map[string][]string{
			"org/repo": {
				"approve",
				"lgtm",
			},
		},
		Blunderbuss: plugins.Blunderbuss{
			ExcludeApprovers: true,
		},
	}
	pluginAgent := &plugins.ConfigAgent{}
	pluginAgent.Set(&c)
	handler := handlePluginConfig(pluginAgent, logrus.WithField("handler", "/plugin-config"))
	req, err := http.NewRequest(http.MethodGet, "/config", nil)
	if err != nil {
		t.Fatalf("Error making request: %v", err)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("Bad error code: %d", rr.Code)
	}
	if h := rr.Header().Get("Content-Type"); h != "text/plain" {
		t.Fatalf("Bad Content-Type, expected: 'text/plain', got: %v", h)
	}
	resp := rr.Result()
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Error reading response body: %v", err)
	}
	var res plugins.Configuration
	if err := yaml.Unmarshal(body, &res); err != nil {
		t.Fatalf("Error unmarshaling: %v", err)
	}
	if !reflect.DeepEqual(c, res) {
		t.Errorf("Invalid config. Got %v, expected %v", res, c)
	}
}

func cfgWithLensNamed(lensName string) *config.Config {
	return &config.Config{
		ProwConfig: config.ProwConfig{
			Deck: config.Deck{
				Spyglass: config.Spyglass{
					Lenses: []config.LensFileConfig{{
						Lens: config.LensConfig{
							Name: lensName,
						},
					}},
				},
			},
		},
	}
}

func verifyCfgHasRemoteForLens(lensName string) func(*config.Config, error) error {
	return func(c *config.Config, err error) error {
		if err != nil {
			return fmt.Errorf("got unexpected error: %w", err)
		}

		var found bool
		for _, lens := range c.Deck.Spyglass.Lenses {
			if lens.Lens.Name != lensName {
				continue
			}
			found = true

			if lens.RemoteConfig == nil {
				return errors.New("remoteConfig for lens was nil")
			}

			if lens.RemoteConfig.Endpoint == "" {
				return errors.New("endpoint was unset")
			}

			if lens.RemoteConfig.ParsedEndpoint == nil {
				return errors.New("parsedEndpoint was nil")
			}
			if expected := common.DyanmicPathForLens(lensName); lens.RemoteConfig.ParsedEndpoint.Path != expected {
				return fmt.Errorf("expected parsedEndpoint.Path to be %q, was %q", expected, lens.RemoteConfig.ParsedEndpoint.Path)
			}
			if lens.RemoteConfig.ParsedEndpoint.Scheme != "http" {
				return fmt.Errorf("expected parsedEndpoint.scheme to be 'http', was %q", lens.RemoteConfig.ParsedEndpoint.Scheme)
			}
			if lens.RemoteConfig.ParsedEndpoint.Host != spyglassLocalLensListenerAddr {
				return fmt.Errorf("expected parsedEndpoint.Host to be %q, was %q", spyglassLocalLensListenerAddr, lens.RemoteConfig.ParsedEndpoint.Host)
			}
			if lens.RemoteConfig.Title == "" {
				return errors.New("expected title to be set")
			}
			if lens.RemoteConfig.Priority == nil {
				return errors.New("expected priority to be set")
			}
			if lens.RemoteConfig.HideTitle == nil {
				return errors.New("expected HideTitle to be set")
			}
		}

		if !found {
			return fmt.Errorf("no config found for lens %q", lensName)
		}

		return nil
	}

}

func TestSpyglassConfigDefaulting(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		in     *config.Config
		verify func(*config.Config, error) error
	}{
		{
			name:   "buildlog lens gets defaulted",
			in:     cfgWithLensNamed("buildlog"),
			verify: verifyCfgHasRemoteForLens("buildlog"),
		},
		{
			name:   "coverage lens gets defaulted",
			in:     cfgWithLensNamed("coverage"),
			verify: verifyCfgHasRemoteForLens("coverage"),
		},
		{
			name:   "junit lens gets defaulted",
			in:     cfgWithLensNamed("junit"),
			verify: verifyCfgHasRemoteForLens("junit"),
		},
		{
			name:   "metadata lens gets defaulted",
			in:     cfgWithLensNamed("metadata"),
			verify: verifyCfgHasRemoteForLens("metadata"),
		},
		{
			name:   "podinfo lens gets defaulted",
			in:     cfgWithLensNamed("podinfo"),
			verify: verifyCfgHasRemoteForLens("podinfo"),
		},
		{
			name:   "restcoverage lens gets defaulted",
			in:     cfgWithLensNamed("restcoverage"),
			verify: verifyCfgHasRemoteForLens("restcoverage"),
		},
		{
			name: "undef lens defaulting fails",
			in:   cfgWithLensNamed("undef"),
			verify: func(_ *config.Config, err error) error {
				expectedErrMsg := `lens "undef" has no remote_config and could not get default: invalid lens name`
				if err == nil || err.Error() != expectedErrMsg {
					return fmt.Errorf("expected err to be %q, was %v", expectedErrMsg, err)
				}
				return nil
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.verify(tc.in, spglassConfigDefaulting(tc.in)); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestHandleGitHubLink(t *testing.T) {
	ghoptions := flagutil.GitHubOptions{Host: "github.mycompany.com"}
	org, repo := "org", "repo"
	handler := HandleGitHubLink(ghoptions.Host, true)
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("/github-link?dest=%s/%s", org, repo), nil)
	if err != nil {
		t.Fatalf("Error making request: %v", err)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("Bad error code: %d", rr.Code)
	}
	resp := rr.Result()
	defer resp.Body.Close()
	actual := resp.Header.Get("Location")
	expected := fmt.Sprintf("https://%s/%s/%s", ghoptions.Host, org, repo)
	if expected != actual {
		t.Fatalf("%v", actual)
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

func TestHttpStatusForError(t *testing.T) {
	testCases := []struct {
		name           string
		input          error
		expectedStatus int
	}{
		{
			name:           "normal_error",
			input:          errors.New("some error message"),
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "httpError",
			input: httpError{
				error:      errors.New("some error message"),
				statusCode: http.StatusGone,
			},
			expectedStatus: http.StatusGone,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(nested *testing.T) {
			actual := httpStatusForError(tc.input)
			if actual != tc.expectedStatus {
				t.Fatalf("unexpected HTTP status (expected=%v, actual=%v) for error: %v", tc.expectedStatus, actual, tc.input)
			}
		})
	}
}
