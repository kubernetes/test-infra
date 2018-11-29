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
	"encoding/json"
	"errors"
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

	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/tide"
)

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

func (f flc) GetJobLog(job, id string) ([]byte, error) {
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
	handler := handleLog(flc(0))
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

type fpjc kube.ProwJob

func (fc *fpjc) GetProwJob(name string) (kube.ProwJob, error) {
	return kube.ProwJob(*fc), nil
}

// TestRerun just checks that the result can be unmarshaled properly, has an
// updated status, and has equal spec.
func TestRerun(t *testing.T) {
	fc := fpjc(kube.ProwJob{
		Spec: kube.ProwJobSpec{
			Job:  "whoa",
			Type: kube.PresubmitJob,
			Refs: &kube.Refs{
				Org:  "org",
				Repo: "repo",
				Pulls: []kube.Pull{
					{Number: 1},
				},
			},
		},
		Status: kube.ProwJobStatus{
			State: kube.PendingState,
		},
	})
	handler := handleRerun(&fc)
	req, err := http.NewRequest(http.MethodGet, "/rerun?prowjob=wowsuch", nil)
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
	var res kube.ProwJob
	if err := yaml.Unmarshal(body, &res); err != nil {
		t.Fatalf("Error unmarshaling: %v", err)
	}
	if res.Spec.Job != "whoa" {
		t.Errorf("Wrong job, expected \"whoa\", got \"%s\"", res.Spec.Job)
	}
	if res.Status.State != kube.TriggeredState {
		t.Errorf("Wrong state, expected \"%v\", got \"%v\"", kube.TriggeredState, res.Status.State)
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
		fmt.Fprintf(w, string(b))
	}))
	ca := &config.Agent{}
	ca.Set(&config.Config{
		ProwConfig: config.ProwConfig{
			Tide: config.Tide{
				Queries: []config.TideQuery{
					{Repos: []string{"kubernetes/test-infra"}},
				},
			},
		},
	})
	ta := tideAgent{
		path:         s.URL,
		updatePeriod: func() time.Duration { return time.Minute },
	}
	if err := ta.update(); err != nil {
		t.Fatalf("Updating: %v", err)
	}
	if len(ta.pools) != 1 {
		t.Fatalf("Wrong number of pools. Got %d, expected 1 in %v", len(ta.pools), ta.pools)
	}
	if ta.pools[0].Org != "o" {
		t.Errorf("Wrong org in pool. Got %s, expected o in %v", ta.pools[0].Org, ta.pools)
	}
	handler := handleTide(ca, &ta)
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
	res := tideData{}
	if err := yaml.Unmarshal(body, &res); err != nil {
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
	if expected := "is:pr state:open repo:\"kubernetes/test-infra\""; res.Queries[0] != expected {
		t.Errorf("Wrong query. Got %s, expected %s", res.Queries[0], expected)
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
		fmt.Fprintf(w, string(b))
	}))
	ha := &helpAgent{
		path: s.URL,
	}
	handler := handlePluginHelp(ha)
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
