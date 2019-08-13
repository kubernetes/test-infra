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
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"k8s.io/test-infra/prow/plugins"
)

func TestServeHTTPErrors(t *testing.T) {
	metrics := NewMetrics()
	pa := &plugins.ConfigAgent{}
	pa.Set(&plugins.Configuration{})

	getSecret := func() []byte {
		return []byte("abc")
	}

	s := &Server{
		Metrics:        metrics,
		Plugins:        pa,
		TokenGenerator: getSecret,
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
			Code: http.StatusOK,
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

		eventType string
		srcRepo   string
		plugins   map[string][]plugins.ExternalPlugin

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
	}

	for _, test := range tests {
		t.Logf("Running scenario %q", test.name)

		pa := &plugins.ConfigAgent{}
		pa.Set(&plugins.Configuration{
			ExternalPlugins: test.plugins,
		})
		s := &Server{Plugins: pa}

		gotPlugins := s.needDemux(test.eventType, test.srcRepo)
		if len(gotPlugins) != len(test.expected) {
			t.Errorf("expected plugins: %+v, got: %+v", test.expected, gotPlugins)
			continue
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
	}
}
