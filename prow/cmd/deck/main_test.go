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
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

type flc int

func (f flc) GetLog(name string) ([]byte, error) {
	if name == "pn" {
		return []byte("hello"), nil
	} else {
		return nil, errors.New("muahaha")
	}
}

func TestHandleLog(t *testing.T) {
	var testcases = []struct {
		name string
		path string
		code int
	}{
		{
			name: "no pod name",
			path: "",
			code: http.StatusBadRequest,
		},
		{
			name: "pod name with unescaped slashes",
			path: "?pod=qwer/abc",
			code: http.StatusBadRequest,
		},
		{
			name: "pod name with escaped slashes",
			path: "?pod=" + url.QueryEscape("qwer/abc"),
			code: http.StatusBadRequest,
		},
		{
			name: "pod name with escaped #",
			path: "?pod=" + url.QueryEscape("abc#"),
			code: http.StatusBadRequest,
		},
		{
			name: "pod that doesn't exist",
			path: "?pod=doesnotexist",
			code: http.StatusNotFound,
		},
		{
			name: "pod that does exist",
			path: "?pod=pn",
			code: http.StatusOK,
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
		req.URL = u
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != tc.code {
			t.Errorf("Wrong error code. Got %v, want %v", rr.Code, tc.code)
		} else if rr.Code == http.StatusOK {
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
