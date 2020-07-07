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

package githubeventserver

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/plugins"
)

func TestServeHTTPErrors(t *testing.T) {
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

	serveMuxHandler := &serveMuxHandler{
		hmacTokenGenerator: getSecret,
		log:                logrus.NewEntry(logrus.New()),
		metrics:            NewMetrics(),
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
		serveMuxHandler.ServeHTTP(w, r)
		if w.Code != tc.Code {
			t.Errorf("For test case: %+v\nExpected code %v, got code %v", tc, tc.Code, w.Code)
		}
	}
}

func TestGetExternalPluginsForEvent(t *testing.T) {
	testCases := []struct {
		name string

		eventType string
		org       string
		repo      string
		plugins   map[string][]plugins.ExternalPlugin

		expected []plugins.ExternalPlugin
	}{
		{
			name: "we have variety",

			eventType: "issue_comment",
			org:       "kubernetes",
			repo:      "test-infra",
			plugins: map[string][]plugins.ExternalPlugin{
				"kubernetes/test-infra": {
					{
						Name:   "sandwich",
						Events: []string{"pull_request"},
					},
					{
						Name:   "coffee",
						Events: []string{"pull_request"},
					},
					{
						Name:   "beer",
						Events: []string{"issue_comment", "issues"},
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
						Name:   "water",
						Events: []string{"pull_request"},
					},
					{
						Name:   "chocolate",
						Events: []string{"pull_request", "issue_comment", "issues"},
					},
				},
			},

			expected: []plugins.ExternalPlugin{
				{
					Name:   "chocolate",
					Events: []string{"pull_request", "issue_comment", "issues"},
				},
				{
					Name:   "beer",
					Events: []string{"issue_comment", "issues"},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := &serveMuxHandler{externalPlugins: tc.plugins}
			gotPlugins := s.getExternalPluginsForEvent(tc.org, tc.repo, tc.eventType)

			if !reflect.DeepEqual(tc.expected, gotPlugins) {
				t.Errorf("expected plugin: %+v, got: %+v", tc.expected, gotPlugins)
			}

		})
	}
}
