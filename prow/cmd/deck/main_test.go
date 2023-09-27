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
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
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
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	pluginsflagutil "k8s.io/test-infra/prow/flagutil/plugins"
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
	setTenantIDs := flagutil.Strings{}
	setTenantIDs.Set("Test")
	var testCases = []struct {
		name        string
		input       options
		expectedErr bool
	}{
		{
			name: "minimal set ok",
			input: options{
				config: configflagutil.ConfigOptions{ConfigPath: "test"},
				controllerManager: flagutil.ControllerManagerOptions{
					TimeoutListingProwJobsDefault: 30 * time.Second,
				},
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
				config: configflagutil.ConfigOptions{ConfigPath: "test"},
				controllerManager: flagutil.ControllerManagerOptions{
					TimeoutListingProwJobsDefault: 30 * time.Second,
				},
				oauthURL:              "website",
				githubOAuthConfigFile: "something",
				cookieSecretFile:      "yum",
			},
			expectedErr: false,
		},
		{
			name: "missing github config with oauth",
			input: options{
				config: configflagutil.ConfigOptions{ConfigPath: "test"},
				controllerManager: flagutil.ControllerManagerOptions{
					TimeoutListingProwJobsDefault: 30 * time.Second,
				},
				oauthURL:         "website",
				cookieSecretFile: "yum",
			},
			expectedErr: true,
		},
		{
			name: "missing cookie with oauth",
			input: options{
				config: configflagutil.ConfigOptions{ConfigPath: "test"},
				controllerManager: flagutil.ControllerManagerOptions{
					TimeoutListingProwJobsDefault: 30 * time.Second,
				},
				oauthURL:              "website",
				githubOAuthConfigFile: "something",
			},
			expectedErr: true,
		},
		{
			name: "hidden only and show hidden are mutually exclusive",
			input: options{
				config: configflagutil.ConfigOptions{ConfigPath: "test"},
				controllerManager: flagutil.ControllerManagerOptions{
					TimeoutListingProwJobsDefault: 30 * time.Second,
				},
				hiddenOnly: true,
				showHidden: true,
			},
			expectedErr: true,
		},
		{
			name: "show hidden and tenantIds are mutually exclusive",
			input: options{
				config: configflagutil.ConfigOptions{ConfigPath: "test"},
				controllerManager: flagutil.ControllerManagerOptions{
					TimeoutListingProwJobsDefault: 30 * time.Second,
				},
				hiddenOnly: false,
				showHidden: true,
				tenantIDs:  setTenantIDs,
			},
			expectedErr: true,
		},
		{
			name: "hiddenOnly and tenantIds are mutually exclusive",
			input: options{
				config: configflagutil.ConfigOptions{ConfigPath: "test"},
				controllerManager: flagutil.ControllerManagerOptions{
					TimeoutListingProwJobsDefault: 30 * time.Second,
				},
				hiddenOnly: true,
				showHidden: false,
				tenantIDs:  setTenantIDs,
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
				if body, err := io.ReadAll(resp.Body); err != nil {
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

	fakeJa := jobs.NewJobAgent(context.Background(), kc, false, true, []string{}, map[string]jobs.PodLogClient{}, fca{}.Config)
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
	body, err := io.ReadAll(resp.Body)
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
				TideGitHubConfig: config.TideGitHubConfig{
					Queries: []config.TideQuery{
						{Repos: []string{"prowapi.netes/test-infra"}},
					},
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
		cfg:          func() *config.Config { return &config.Config{} },
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
	body, err := io.ReadAll(resp.Body)
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
		cfg:          func() *config.Config { return &config.Config{} },
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
	body, err := io.ReadAll(resp.Body)
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
		body, err := io.ReadAll(resp.Body)
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
		name       string
		args       map[string]string
		del        sets.Set[string]
		koDataPath string
		expected   func(*options)
		err        bool
	}{
		{
			name: "minimal flags work",
			expected: func(o *options) {
				o.controllerManager.TimeoutListingProwJobs = 30 * time.Second
				o.controllerManager.TimeoutListingProwJobsDefault = 30 * time.Second
			},
		},
		{
			name: "default static files location",
			expected: func(o *options) {
				o.controllerManager.TimeoutListingProwJobs = 30 * time.Second
				o.controllerManager.TimeoutListingProwJobsDefault = 30 * time.Second
				o.spyglassFilesLocation = "/lenses"
				o.staticFilesLocation = "/static"
				o.templateFilesLocation = "/template"
			},
		},
		{
			name:       "ko data path",
			koDataPath: "ko-data",
			expected: func(o *options) {
				o.controllerManager.TimeoutListingProwJobs = 30 * time.Second
				o.controllerManager.TimeoutListingProwJobsDefault = 30 * time.Second
				o.spyglassFilesLocation = "ko-data/lenses"
				o.staticFilesLocation = "ko-data/static"
				o.templateFilesLocation = "ko-data/template"
			},
		},
		{
			name: "explicitly set --config-path",
			args: map[string]string{
				"--config-path": "/random/value",
			},
			expected: func(o *options) {
				o.config.ConfigPath = "/random/value"
				o.controllerManager.TimeoutListingProwJobs = 30 * time.Second
				o.controllerManager.TimeoutListingProwJobsDefault = 30 * time.Second
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
			oldKoDataPath := os.Getenv("KO_DATA_PATH")
			if err := os.Setenv("KO_DATA_PATH", tc.koDataPath); err != nil {
				t.Fatalf("Failed set env var KO_DATA_PATH: %v", err)
			}
			defer os.Setenv("KO_DATA_PATH", oldKoDataPath)

			expected := &options{
				config: configflagutil.ConfigOptions{
					ConfigPathFlagName:                    "config-path",
					JobConfigPathFlagName:                 "job-config-path",
					ConfigPath:                            "yo",
					SupplementalProwConfigsFileNameSuffix: "_prowconfig.yaml",
					InRepoConfigCacheSize:                 200,
				},
				pluginsConfig: pluginsflagutil.PluginOptions{
					SupplementalPluginsConfigsFileNameSuffix: "_pluginconfig.yaml",
				},
				githubOAuthConfigFile: "/etc/github/secret",
				cookieSecretFile:      "",
				staticFilesLocation:   "/static",
				templateFilesLocation: "/template",
				spyglassFilesLocation: "/lenses",
				github:                ghoptions,
				instrumentation:       flagutil.DefaultInstrumentationOptions(),
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
				t.Errorf("actual differs from expected: %s", cmp.Diff(actual, *expected, cmp.Exporter(func(_ reflect.Type) bool { return true })))
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
				TideGitHubConfig: config.TideGitHubConfig{
					Queries: []config.TideQuery{
						{Repos: []string{"prowapi.netes/test-infra"}},
					},
				},
			},
		},
	}
	cWithDisabledCluster := config.Config{
		ProwConfig: config.ProwConfig{
			DisabledClusters: []string{"build08", "build08", "build01"},
		},
	}
	dataC, err := yaml.Marshal(c)
	if err != nil {
		t.Fatalf("Error unmarshaling: %v", err)
	}

	testcases := []struct {
		name                string
		config              config.Config
		url                 string
		expectedBody        []byte
		expectedStatus      int
		expectedContentType string
	}{
		{
			name:                "general case",
			config:              c,
			url:                 "/config",
			expectedBody:        dataC,
			expectedStatus:      http.StatusOK,
			expectedContentType: "text/plain",
		},
		{
			name:                "unsupported key",
			config:              c,
			url:                 "/config?key=some",
			expectedBody:        []byte("getting config for key some is not supported\n"),
			expectedStatus:      http.StatusInternalServerError,
			expectedContentType: `text/plain; charset=utf-8`,
		},
		{
			name:   "no disabled clusters",
			config: c,
			url:    "/config?key=disabled-clusters",
			expectedBody: []byte(`[]
`),
			expectedStatus:      http.StatusOK,
			expectedContentType: `text/plain`,
		},
		{
			name:   "disabled clusters",
			config: cWithDisabledCluster,
			url:    "/config?key=disabled-clusters",
			expectedBody: []byte(`- build01
- build08
`),
			expectedStatus:      http.StatusOK,
			expectedContentType: `text/plain`,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			configGetter := func() *config.Config {
				return &tc.config
			}
			handler := handleConfig(configGetter, logrus.WithField("handler", "/config"))
			req, err := http.NewRequest(http.MethodGet, tc.url, nil)
			if err != nil {
				t.Fatalf("Error making request: %v", err)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tc.expectedStatus {
				t.Fatalf("Bad error code: %d", rr.Code)
			}
			if h := rr.Header().Get("Content-Type"); h != tc.expectedContentType {
				t.Fatalf("Bad Content-Type, expected: 'text/plain', got: %v", h)
			}
			resp := rr.Result()
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Error reading response body: %v", err)
			}
			if diff := cmp.Diff(string(tc.expectedBody), string(body)); diff != "" {
				t.Errorf("Error differs from expected:\n%s", diff)
			}
		})
	}

}

func TestHandlePluginConfig(t *testing.T) {
	c := plugins.Configuration{
		Plugins: plugins.Plugins{
			"org/repo": {Plugins: []string{
				"approve",
				"lgtm",
			}},
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
	body, err := io.ReadAll(resp.Body)
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
					return fmt.Errorf("expected err to be %q, was %w", expectedErrMsg, err)
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

func TestHandleGitProviderLink(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{
			name:  "github-commit",
			query: "target=commit&repo=bar&commit=abc123",
			want:  "https://github.mycompany.com/bar/commit/abc123",
		},
		{
			name:  "github-branch",
			query: "target=branch&repo=bar&branch=main",
			want:  "https://github.mycompany.com/bar/tree/main",
		},
		{
			name:  "github-pr",
			query: "target=pr&repo=bar&number=2",
			want:  "https://github.mycompany.com/bar/pull/2",
		},
		{
			name:  "github-pr-with-quote",
			query: "target=pr&repo='bar'&number=2",
			want:  "https://github.mycompany.com/bar/pull/2",
		},
		{
			name:  "github-author",
			query: "target=author&author=chaodaiG",
			want:  "https://github.mycompany.com/chaodaiG",
		},
		{
			name:  "github-author-withquote",
			query: "target=author&repo='bar'&author=chaodaiG",
			want:  "https://github.mycompany.com/chaodaiG",
		},
		{
			name:  "github-invalid",
			query: "target=invalid&repo=bar&commit=abc123",
			want:  "/",
		},
		{
			name:  "gerrit-commit",
			query: "target=commit&repo='https://foo-review.abc/bar'&commit=abc123",
			want:  "https://foo.abc/bar/+/abc123",
		},
		{
			name:  "gerrit-commit",
			query: "target=prcommit&repo='https://foo-review.abc/bar'&commit=abc123",
			want:  "https://foo.abc/bar/+/abc123",
		},
		{
			name:  "gerrit-branch",
			query: "target=branch&repo='https://foo-review.abc/bar'&branch=main",
			want:  "https://foo.abc/bar/+/refs/heads/main",
		},
		{
			name:  "gerrit-pr",
			query: "target=pr&repo='https://foo-review.abc/bar'&number=2",
			want:  "https://foo-review.abc/c/bar/+/2",
		},
		{
			name:  "gerrit-invalid",
			query: "target=invalid&repo='https://foo-review.abc/bar'&commit=abc123",
			want:  "/",
		},
	}

	ghoptions := flagutil.GitHubOptions{Host: "github.mycompany.com"}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url := fmt.Sprintf("/git-provider-link?%s", tc.query)
			req, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				t.Fatalf("Error making request: %v", err)
			}

			handler := HandleGitProviderLink(ghoptions.Host, true)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusFound {
				t.Fatalf("Bad error code: %d", rr.Code)
			}
			resp := rr.Result()
			defer resp.Body.Close()
			if want, got := tc.want, resp.Header.Get("Location"); want != got {
				t.Fatalf("Wrong URL. Want: %s, got: %s", want, got)
			}
		})
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
		{
			name: "httpError_wrapped",
			input: fmt.Errorf("wrapped error: %w", httpError{
				error:      errors.New("some error message"),
				statusCode: http.StatusGone,
			}),
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

func TestPRHistLink(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    string
		org     string
		repo    string
		number  int
		want    string
		wantErr bool
	}{
		{
			name:    "default",
			tmpl:    defaultPRHistLinkTemplate,
			org:     "org",
			repo:    "repo",
			number:  0,
			want:    "/pr-history?org=org&repo=repo&pr=0",
			wantErr: false,
		},
		{
			name:    "different-template",
			tmpl:    "/pull={{.Number}}",
			org:     "org",
			repo:    "repo",
			number:  0,
			want:    "/pull=0",
			wantErr: false,
		},
		{
			name:    "invalid-template",
			tmpl:    "doesn't matter {{.NotExist}}",
			org:     "org",
			repo:    "repo",
			number:  0,
			want:    "",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, gotErr := prHistLinkFromTemplate(tc.tmpl, tc.org, tc.repo, tc.number)
			if (tc.wantErr && (gotErr == nil)) || (!tc.wantErr && (gotErr != nil)) {
				t.Fatalf("Error mismatch. Want: %v, got: %v", tc.wantErr, gotErr)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("Template mismatch. Want: (-), got: (+). \n%s", diff)
			}
		})
	}
}
