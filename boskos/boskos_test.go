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

package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func MakeFakeClient(resources []Resource) *Ranch {
	newRanch := &Ranch{
		Resources: resources,
	}

	return newRanch
}

func TestAcquire(t *testing.T) {
	var testcases = []struct {
		name      string
		resources []Resource
		path      string
		code      int
		method    string
	}{
		{
			name:      "get",
			resources: []Resource{},
			path:      "?type=t&state=s&owner=o",
			code:      http.StatusMethodNotAllowed,
			method:    http.MethodGet,
		},
		{
			name:      "no arg",
			resources: []Resource{},
			path:      "",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "missing type",
			resources: []Resource{},
			path:      "?state=s&owner=o",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "missing state",
			resources: []Resource{},
			path:      "?type=t&owner=o",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "missing owner",
			resources: []Resource{},
			path:      "?type=t&state=s",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "no resource",
			resources: []Resource{},
			path:      "?type=t&state=s&owner=o",
			code:      http.StatusConflict,
			method:    http.MethodPost,
		},
		{
			name: "no match type",
			resources: []Resource{
				Resource{
					Name:  "res",
					Type:  "wrong",
					State: "s",
					Owner: "",
				},
			},
			path:   "?type=t&state=s&owner=o",
			code:   http.StatusConflict,
			method: http.MethodPost,
		},
		{
			name: "no match state",
			resources: []Resource{
				Resource{
					Name:  "res",
					Type:  "t",
					State: "wrong",
					Owner: "",
				},
			},
			path:   "?type=t&state=s&owner=o",
			code:   http.StatusConflict,
			method: http.MethodPost,
		},
		{
			name: "busy",
			resources: []Resource{
				Resource{
					Name:  "res",
					Type:  "t",
					State: "s",
					Owner: "user",
				},
			},
			path:   "?type=t&state=s&owner=o",
			code:   http.StatusConflict,
			method: http.MethodPost,
		},
		{
			name: "ok",
			resources: []Resource{
				Resource{
					Name:  "res",
					Type:  "t",
					State: "s",
					Owner: "",
				},
			},
			path:   "?type=t&state=s&owner=o",
			code:   http.StatusOK,
			method: http.MethodPost,
		},
	}

	for _, tc := range testcases {
		c := MakeFakeClient(tc.resources)
		handler := handleAcquire(c)
		req, err := http.NewRequest(tc.method, "", nil)
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
			t.Errorf("%s - Wrong error code. Got %v, expect %v", tc.name, rr.Code, tc.code)
		}

		if rr.Code == http.StatusOK {
			var data Resource
			json.Unmarshal(rr.Body.Bytes(), &data)
			if data.Name != "res" {
				t.Errorf("%s - Got res %v, expect res", tc.name, data.Name)
			}

			if c.Resources[0].Owner != "o" {
				t.Errorf("%s - Wrong owner. Got %v, expect o", tc.name, c.Resources[0].Owner)
			}
		}
	}
}

func TestRelease(t *testing.T) {
	var testcases = []struct {
		name      string
		resources []Resource
		path      string
		code      int
		method    string
	}{
		{
			name:      "get",
			resources: []Resource{},
			path:      "?name=res&dest=d&owner=foo",
			code:      http.StatusMethodNotAllowed,
			method:    http.MethodGet,
		},
		{
			name:      "no arg",
			resources: []Resource{},
			path:      "",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "missing name",
			resources: []Resource{},
			path:      "?dest=d&owner=foo",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "missing dest",
			resources: []Resource{},
			path:      "?name=res&owner=foo",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "missing owner",
			resources: []Resource{},
			path:      "?name=res&dest=d",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "no resource",
			resources: []Resource{},
			path:      "?name=res&dest=d&owner=foo",
			code:      http.StatusConflict,
			method:    http.MethodPost,
		},
		{
			name: "wrong owner",
			resources: []Resource{
				Resource{
					Name:  "res",
					Type:  "t",
					State: "s",
					Owner: "merlin",
				},
			},
			path:   "?name=res&dest=d&owner=foo",
			code:   http.StatusConflict,
			method: http.MethodPost,
		},
		{
			name: "no match name",
			resources: []Resource{
				Resource{
					Name:  "foo",
					Type:  "t",
					State: "s",
					Owner: "merlin",
				},
			},
			path:   "?name=res&dest=d&owner=merlin",
			code:   http.StatusConflict,
			method: http.MethodPost,
		},
		{
			name: "ok",
			resources: []Resource{
				Resource{
					Name:  "res",
					Type:  "t",
					State: "s",
					Owner: "merlin",
				},
			},
			path:   "?name=res&dest=d&owner=merlin",
			code:   http.StatusOK,
			method: http.MethodPost,
		},
	}

	for _, tc := range testcases {
		c := MakeFakeClient(tc.resources)
		handler := handleRelease(c)
		req, err := http.NewRequest(tc.method, "", nil)
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
			t.Errorf("%s - Wrong error code. Got %v, expect %v", tc.name, rr.Code, tc.code)
		}

		if rr.Code == http.StatusOK {
			if c.Resources[0].State != "d" {
				t.Errorf("%s - Wrong state. Got %v, expect d", tc.name, c.Resources[0].State)
			}

			if c.Resources[0].Owner != "" {
				t.Errorf("%s - Wrong owner. Got %v, expect empty", tc.name, c.Resources[0].Owner)
			}
		}
	}
}

func TestReset(t *testing.T) {
	var testcases = []struct {
		name      string
		resources []Resource
		path      string
		code      int
		method    string
	}{
		{
			name:      "get",
			resources: []Resource{},
			path:      "?type=t&state=s&expire=10m&dest=d",
			code:      http.StatusMethodNotAllowed,
			method:    http.MethodGet,
		},
		{
			name:      "no arg",
			resources: []Resource{},
			path:      "",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "missing type",
			resources: []Resource{},
			path:      "?state=s&expire=10m&dest=d",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "missing state",
			resources: []Resource{},
			path:      "?type=t&expire=10m&dest=d",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "missing expire",
			resources: []Resource{},
			path:      "?type=t&state=s&dest=d",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "missing dest",
			resources: []Resource{},
			path:      "?type=t&state=s&expire=10m",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "bad expire",
			resources: []Resource{},
			path:      "?type=t&state=s&expire=woooo&dest=d",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name: "empty - has owner",
			resources: []Resource{
				Resource{
					Name:       "res",
					Type:       "t",
					State:      "s",
					Owner:      "",
					LastUpdate: time.Now().Add(-time.Minute * 20),
				},
			},
			path:   "?type=t&state=s&expire=10m&dest=d",
			code:   http.StatusOK,
			method: http.MethodPost,
		},
		{
			name: "empty - not expire",
			resources: []Resource{
				Resource{
					Name:       "res",
					Type:       "t",
					State:      "s",
					Owner:      "",
					LastUpdate: time.Now(),
				},
			},
			path:   "?type=t&state=s&expire=10m&dest=d",
			code:   http.StatusOK,
			method: http.MethodPost,
		},
		{
			name: "empty - no match type",
			resources: []Resource{
				Resource{
					Name:       "res",
					Type:       "wrong",
					State:      "s",
					Owner:      "",
					LastUpdate: time.Now().Add(-time.Minute * 20),
				},
			},
			path:   "?type=t&state=s&expire=10m&dest=d",
			code:   http.StatusOK,
			method: http.MethodPost,
		},
		{
			name: "empty - no match state",
			resources: []Resource{
				Resource{
					Name:       "res",
					Type:       "t",
					State:      "wrong",
					Owner:      "",
					LastUpdate: time.Now().Add(-time.Minute * 20),
				},
			},
			path:   "?type=t&state=s&expire=10m&dest=d",
			code:   http.StatusOK,
			method: http.MethodPost,
		},
		{
			name: "ok",
			resources: []Resource{
				Resource{
					Name:       "res",
					Type:       "t",
					State:      "s",
					Owner:      "user",
					LastUpdate: time.Now().Add(-time.Minute * 20),
				},
			},
			path:   "?type=t&state=s&expire=10m&dest=d",
			code:   http.StatusOK,
			method: http.MethodPost,
		},
	}

	for _, tc := range testcases {
		c := MakeFakeClient(tc.resources)
		handler := handleReset(c)
		req, err := http.NewRequest(tc.method, "", nil)
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
			t.Errorf("%s - Wrong error code. Got %v, expect %v", tc.name, rr.Code, tc.code)
		}

		if rr.Code == http.StatusOK {
			rmap := make(map[string]string)
			json.Unmarshal(rr.Body.Bytes(), &rmap)
			if strings.HasPrefix(tc.name, "empty") {
				if len(rmap) != 0 {
					t.Errorf("%s - Expect empty map. Got %v", tc.name, rmap)
				}
			} else {
				if owner, ok := rmap["res"]; !ok || owner != "user" {
					t.Errorf("%s - Expect res - user. Got %v", tc.name, rmap)
				}
			}
		}
	}
}

func TestUpdate(t *testing.T) {
	FakeNow := time.Now()

	var testcases = []struct {
		name      string
		resources []Resource
		path      string
		code      int
		method    string
	}{
		{
			name:      "get",
			resources: []Resource{},
			path:      "?name=foo",
			code:      http.StatusMethodNotAllowed,
			method:    http.MethodGet,
		},
		{
			name:      "no arg",
			resources: []Resource{},
			path:      "",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "no resource",
			resources: []Resource{},
			path:      "?name=res&owner=merlin",
			code:      http.StatusConflict,
			method:    http.MethodPost,
		},
		{
			name: "wrong owner",
			resources: []Resource{
				Resource{
					Name:  "res",
					Type:  "t",
					State: "s",
					Owner: "evil",
				},
			},
			path:   "?name=res&owner=merlin",
			code:   http.StatusConflict,
			method: http.MethodPost,
		},
		{
			name: "no matched resource",
			resources: []Resource{
				Resource{
					Name:  "res",
					Type:  "t",
					State: "s",
					Owner: "merlin",
				},
			},
			path:   "?name=foo&owner=merlin",
			code:   http.StatusConflict,
			method: http.MethodPost,
		},
		{
			name: "ok",
			resources: []Resource{
				Resource{
					Name:       "res",
					Type:       "t",
					State:      "s",
					Owner:      "merlin",
					LastUpdate: FakeNow,
				},
			},
			path:   "?name=res&owner=merlin",
			code:   http.StatusOK,
			method: http.MethodPost,
		},
	}

	for _, tc := range testcases {
		c := MakeFakeClient(tc.resources)
		handler := handleUpdate(c)
		req, err := http.NewRequest(tc.method, "", nil)
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
			t.Errorf("%s - Wrong error code. Got %v, expect %v", tc.name, rr.Code, tc.code)
		}

		if rr.Code == http.StatusOK {
			if c.Resources[0].LastUpdate == FakeNow {
				t.Errorf("%s - Timestamp is not updated!", tc.name)
			}
		}
	}
}

func TestDefault(t *testing.T) {
	var testcases = []struct {
		name string
		code int
	}{
		{
			name: "empty",
			code: http.StatusOK,
		},
	}

	for _, tc := range testcases {
		handler := handleDefault()
		req, err := http.NewRequest(http.MethodGet, "", nil)
		if err != nil {
			t.Fatalf("Error making request: %v", err)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != tc.code {
			t.Errorf("%s - Wrong error code. Got %v, expect %v", tc.name, rr.Code, tc.code)
		}
	}
}

func TestProjectConfig(t *testing.T) {
	file, err := ioutil.ReadFile("resources.json")
	if err != nil {
		t.Errorf("ReadFile error: %v\n", err)
	}

	var data []Resource
	err = json.Unmarshal(file, &data)
	if err != nil {
		t.Errorf("Unmarshal error: %v\n", err)
	}

	if len(data) == 0 {
		t.Errorf("Empty data!")
	}

	for _, p := range data {
		if p.Name == "" {
			t.Errorf("Empty project name: %v\n", p.Name)
		}

		if !strings.Contains(p.Name, "FAKE") { // placeholder, change it to a valid pattern later.
			t.Errorf("Invalid project: %v\n", p.Name)
		}
	}
}
