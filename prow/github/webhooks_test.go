/*
Copyright 2017 The Kubernetes Authors.

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

package github

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateWebhook(t *testing.T) {
	getSecret := func() []byte {
		return []byte("abc")
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
		ValidateWebhook(w, r, getSecret())
		if w.Code != tc.Code {
			t.Errorf("For test case: %+v\nExpected code %v, got code %v", tc, tc.Code, w.Code)
		}
	}
}
