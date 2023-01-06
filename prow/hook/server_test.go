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

package hook

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"k8s.io/test-infra/prow/githubeventserver"
	"k8s.io/test-infra/prow/plugins"
)

func TestServeHTTPErrors(t *testing.T) {
	metrics := githubeventserver.NewMetrics()
	pa := &plugins.ConfigAgent{}
	pa.Set(&plugins.Configuration{})

	getSecret := func() []byte {
		var repoLevelSecret = `
'*':
  - value: abc
    created_at: 2019-10-02T15:00:00Z
  - value: key2
    created_at: 2020-10-02T15:00:00Z
foo/bar:
  - value: 123abc
    created_at: 2019-10-02T15:00:00Z
  - value: key6
    created_at: 2020-10-02T15:00:00Z
`
		return []byte(repoLevelSecret)
	}

	s := &Server{
		Metrics:        metrics,
		Plugins:        pa,
		TokenGenerator: getSecret,
		RepoEnabled:    func(org, repo string) bool { return true },
	}
	// This is the SHA1 signature for payload "{}" and signature "abc"
	// echo -n '{}' | openssl dgst -sha1 -hmac abc
	const hmac string = "sha1=db5c76f4264d0ad96cf21baec394964b4b8ce580"
	const body string = "{}"
	var testcases = []struct {
		name string

		Method string
		Header map[string]string
		Body   string
		Code   int
	}{
		{
			name: "Delete",

			Method: http.MethodDelete,
			Header: map[string]string{
				"X-GitHub-Event":    "ping",
				"X-GitHub-Delivery": "I am unique",
				"X-Hub-Signature":   hmac,
				"content-type":      "application/json",
			},
			Body: body,
			Code: http.StatusMethodNotAllowed,
		},
		{
			name: "No event",

			Method: http.MethodPost,
			Header: map[string]string{
				"X-GitHub-Delivery": "I am unique",
				"X-Hub-Signature":   hmac,
				"content-type":      "application/json",
			},
			Body: body,
			Code: http.StatusBadRequest,
		},
		{
			name: "No content type",

			Method: http.MethodPost,
			Header: map[string]string{
				"X-GitHub-Event":    "ping",
				"X-GitHub-Delivery": "I am unique",
				"X-Hub-Signature":   hmac,
			},
			Body: body,
			Code: http.StatusBadRequest,
		},
		{
			name: "No event guid",

			Method: http.MethodPost,
			Header: map[string]string{
				"X-GitHub-Event":  "ping",
				"X-Hub-Signature": hmac,
				"content-type":    "application/json",
			},
			Body: body,
			Code: http.StatusBadRequest,
		},
		{
			name: "No signature",

			Method: http.MethodPost,
			Header: map[string]string{
				"X-GitHub-Event":    "ping",
				"X-GitHub-Delivery": "I am unique",
				"content-type":      "application/json",
			},
			Body: body,
			Code: http.StatusForbidden,
		},
		{
			name: "Bad signature",

			Method: http.MethodPost,
			Header: map[string]string{
				"X-GitHub-Event":    "ping",
				"X-GitHub-Delivery": "I am unique",
				"X-Hub-Signature":   "this doesn't work",
				"content-type":      "application/json",
			},
			Body: body,
			Code: http.StatusForbidden,
		},
		{
			name: "Good",

			Method: http.MethodPost,
			Header: map[string]string{
				"X-GitHub-Event":    "ping",
				"X-GitHub-Delivery": "I am unique",
				"X-Hub-Signature":   hmac,
				"content-type":      "application/json",
			},
			Body: body,
			Code: http.StatusOK,
		},
		{
			name: "Good, again",

			Method: http.MethodGet,
			Header: map[string]string{
				"content-type": "application/json",
			},
			Body: body,
			Code: http.StatusMethodNotAllowed,
		},
	}

	for _, tc := range testcases {
		t.Logf("Running scenario %q", tc.name)

		w := httptest.NewRecorder()
		r, err := http.NewRequest(tc.Method, "", strings.NewReader(tc.Body))
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range tc.Header {
			r.Header.Set(k, v)
		}
		s.ServeHTTP(w, r)
		if w.Code != tc.Code {
			t.Errorf("For test case: %+v\nExpected code %v, got code %v", tc, tc.Code, w.Code)
		}
	}
}

func TestNeedDemux(t *testing.T) {
	tests := []struct {
		name string

		eventType   string
		srcRepo     string
		repoEnabled func(org, repo string) bool
		plugins     map[string][]plugins.ExternalPlugin

		expected []plugins.ExternalPlugin
	}{
		{
			name: "no external plugins",

			eventType: "issue_comment",
			srcRepo:   "kubernetes/test-infra",
			plugins:   nil,

			expected: nil,
		},
		{
			name: "we have variety",

			eventType: "issue_comment",
			srcRepo:   "kubernetes/test-infra",
			plugins: map[string][]plugins.ExternalPlugin{
				"kubernetes/test-infra": {
					{
						Name:   "sandwich",
						Events: []string{"pull_request"},
					},
					{
						Name: "coffee",
					},
				},
				"kubernetes/kubernetes": {
					{
						Name:   "gumbo",
						Events: []string{"issue_comment"},
					},
				},
				"kubernetes": {
					{
						Name:   "chicken",
						Events: []string{"push"},
					},
					{
						Name: "water",
					},
					{
						Name:   "chocolate",
						Events: []string{"pull_request", "issue_comment", "issues"},
					},
				},
			},

			expected: []plugins.ExternalPlugin{
				{
					Name: "coffee",
				},
				{
					Name: "water",
				},
				{
					Name:   "chocolate",
					Events: []string{"pull_request", "issue_comment", "issues"},
				},
			},
		},
		{
			name: "external plugins handling other events",

			eventType: "repository",
			srcRepo:   "kubernetes/test-infra",
			plugins: map[string][]plugins.ExternalPlugin{
				"kubernetes/test-infra": {
					{
						Name: "coffee",
					},
				},
				"kubernetes/kubernetes": {
					{
						Name:   "gumbo",
						Events: []string{"issue_comment"},
					},
				},
				"kubernetes": {
					{
						Name:   "chicken",
						Events: []string{"repository"},
					},
					{
						Name: "water",
					},
					{
						Name:   "chocolate",
						Events: []string{"pull_request", "issue_comment", "repository"},
					},
				},
			},

			expected: []plugins.ExternalPlugin{
				{
					Name: "coffee",
				},
				{
					Name: "water",
				},
				{
					Name:   "chicken",
					Events: []string{"repository"},
				},
				{
					Name:   "chocolate",
					Events: []string{"pull_request", "issue_comment", "repository"},
				},
			},
		},
		{
			name: "we have variety but disabled that repo",

			eventType: "issue_comment",
			srcRepo:   "kubernetes/test-infra",
			repoEnabled: func(org, repo string) bool {
				if org == "kubernetes" && repo == "test-infra" {
					return false
				}
				return true
			},
			plugins: map[string][]plugins.ExternalPlugin{
				"kubernetes/test-infra": {
					{
						Name:   "sandwich",
						Events: []string{"pull_request"},
					},
					{
						Name: "coffee",
					},
				},
				"kubernetes/kubernetes": {
					{
						Name:   "gumbo",
						Events: []string{"issue_comment"},
					},
				},
				"kubernetes": {
					{
						Name:   "chicken",
						Events: []string{"push"},
					},
					{
						Name: "water",
					},
					{
						Name:   "chocolate",
						Events: []string{"pull_request", "issue_comment", "issues"},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			t.Logf("Running scenario %q", test.name)

			pa := &plugins.ConfigAgent{}
			pa.Set(&plugins.Configuration{
				ExternalPlugins: test.plugins,
			})

			if test.repoEnabled == nil {
				test.repoEnabled = func(_, _ string) bool { return true }
			}
			s := &Server{Plugins: pa, RepoEnabled: test.repoEnabled}

			gotPlugins := s.needDemux(test.eventType, test.srcRepo)
			if len(gotPlugins) != len(test.expected) {
				t.Fatalf("expected plugins: %+v, got: %+v", test.expected, gotPlugins)
			}
			for _, expected := range test.expected {
				var found bool
				for _, got := range gotPlugins {
					if got.Name != expected.Name {
						continue
					}
					if !reflect.DeepEqual(expected, got) {
						t.Errorf("expected plugin: %+v, got: %+v", expected, got)
					}
					found = true
				}
				if !found {
					t.Errorf("expected plugins: %+v, got: %+v", test.expected, gotPlugins)
					break
				}
			}
		})
	}
}

type roundTripFunc func(req *http.Request) *http.Response

// RoundTrip .
func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

// newTestClient returns *http.Client with Transport replaced to avoid making real calls
func newTestClient(fn roundTripFunc) *http.Client {
	return &http.Client{
		Transport: fn,
	}
}

func TestDemuxEvent(t *testing.T) {

	getSecret := func() []byte {
		var repoLevelSecret = `
'*':
  - value: abc
    created_at: 2019-10-02T15:00:00Z
  - value: key2
    created_at: 2020-10-02T15:00:00Z
foo/bar:
  - value: 123abc
    created_at: 2019-10-02T15:00:00Z
  - value: key6
    created_at: 2020-10-02T15:00:00Z
`
		return []byte(repoLevelSecret)
	}

	externalPlugins := map[string][]plugins.ExternalPlugin{
		"kubernetes/test-infra": {
			{
				Name:     "coffee",
				Endpoint: "/coffee",
			},
		},
		"kubernetes/kubernetes": {
			{
				Name:     "gumbo",
				Endpoint: "/gumbo",
				Events:   []string{"issue_comment"},
			},
		},
		"kubernetes": {
			{
				Name:     "chicken",
				Endpoint: "/chicken",
				Events:   []string{"repository"},
			},
			{
				Name:     "water",
				Endpoint: "/water",
			},
			{
				Name:     "chocolate",
				Endpoint: "/chocolate",
				Events:   []string{"pull_request", "issue_comment", "repository"},
			},
			{
				Name:     "unknown_event_handler",
				Endpoint: "/unknown",
				Events:   []string{"unknown_event"},
			},
		},
	}

	// This is the SHA1 signature for payload "$BODY" and signature "abc"
	// echo -n $BODY | openssl dgst -sha1 -hmac abc
	const hmac string = "sha1=d5f926df2d39006bdb5b6acb18f8fcdebad7a052"
	const body string = `{
  "action": "edited",
  "changes": {
    "default_branch": {
      "from": "master"
    }
  },
  "repository": {
    "full_name": "kubernetes/test-infra",
    "default_branch": "master"
  }
}`

	metrics := githubeventserver.NewMetrics()
	pa := &plugins.ConfigAgent{}
	pa.Set(&plugins.Configuration{
		ExternalPlugins: externalPlugins,
	})

	var testcases = []struct {
		name string

		Method string
		Header map[string]string
		Body   string

		ExpectedDispatch []string
	}{
		{
			name: "Repository event",

			Method: http.MethodPost,
			Header: map[string]string{
				"X-GitHub-Event":    "repository",
				"X-GitHub-Delivery": "I am unique",
				"X-Hub-Signature":   hmac,
				"content-type":      "application/json",
			},
			Body: body,

			ExpectedDispatch: []string{"/coffee", "/water", "/chicken", "/chocolate"},
		},
		{
			name: "Issue comment event",

			Method: http.MethodPost,
			Header: map[string]string{
				"X-GitHub-Event":    "issue_comment",
				"X-GitHub-Delivery": "I am unique",
				"X-Hub-Signature":   hmac,
				"content-type":      "application/json",
			},
			Body: body,

			ExpectedDispatch: []string{"/coffee", "/water", "/chocolate"},
		},
		{
			name: "Unknown event type gets dispatched to external plugin",

			Method: http.MethodPost,
			Header: map[string]string{
				"X-GitHub-Event":    "unknown_event",
				"X-GitHub-Delivery": "I am unique",
				"X-Hub-Signature":   hmac,
				"content-type":      "application/json",
			},
			Body: body,

			ExpectedDispatch: []string{"/coffee", "/water", "/unknown"},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Running scenario %q", tc.name)

			var calledExternalPlugins []string
			var m sync.Mutex

			client := newTestClient(func(req *http.Request) *http.Response {
				m.Lock()
				calledExternalPlugins = append(calledExternalPlugins, req.URL.String())
				m.Unlock()
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`OK`)),
					Header:     make(http.Header),
				}
			})

			s := &Server{
				Metrics:        metrics,
				Plugins:        pa,
				TokenGenerator: getSecret,
				RepoEnabled:    func(org, repo string) bool { return true },
				c:              *client,
			}
			w := httptest.NewRecorder()
			r, err := http.NewRequest(tc.Method, "", strings.NewReader(tc.Body))
			if err != nil {
				t.Fatal(err)
			}
			for k, v := range tc.Header {
				r.Header.Set(k, v)
			}
			s.ServeHTTP(w, r)
			s.wg.Wait()

			if diff := cmp.Diff(tc.ExpectedDispatch, calledExternalPlugins, cmpopts.SortSlices(func(a, b string) bool {
				return a < b
			})); diff != "" {
				t.Fatalf("Expected plugins calls mismatch. got(+), want(-):\n%s", diff)
			}
		})
	}
}
