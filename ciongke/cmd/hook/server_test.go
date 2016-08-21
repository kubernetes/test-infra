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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeHTTPErrors(t *testing.T) {
	s := &Server{
		HMACSecret: []byte("abc"),
	}
	// This is the SHA1 signature for payload "{}" and signature "abc"
	// echo -n '{}' | openssl dgst -sha1 -hmac abc
	const hmac string = "sha1=db5c76f4264d0ad96cf21baec394964b4b8ce580"
	const body string = "{}"
	var testcases = []struct {
		Method string
		Header map[string]string
		Body   string
		Code   int
	}{
		{
			// GET
			Method: http.MethodGet,
			Header: map[string]string{
				"X-GitHub-Event":  "ping",
				"X-Hub-Signature": hmac,
			},
			Body: body,
			Code: http.StatusMethodNotAllowed,
		},
		{
			// No event
			Method: http.MethodPost,
			Header: map[string]string{
				"X-Hub-Signature": hmac,
			},
			Body: body,
			Code: http.StatusBadRequest,
		},
		{
			// No signature
			Method: http.MethodPost,
			Header: map[string]string{
				"X-GitHub-Event": "ping",
			},
			Body: body,
			Code: http.StatusForbidden,
		},
		{
			// Bad signature
			Method: http.MethodPost,
			Header: map[string]string{
				"X-GitHub-Event":  "ping",
				"X-Hub-Signature": "this doesn't work",
			},
			Body: body,
			Code: http.StatusForbidden,
		},
		{
			// Good
			Method: http.MethodPost,
			Header: map[string]string{
				"X-GitHub-Event":  "ping",
				"X-Hub-Signature": hmac,
			},
			Body: body,
			Code: http.StatusOK,
		},
	}

	for _, tc := range testcases {
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
