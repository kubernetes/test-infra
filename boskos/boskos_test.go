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
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/crds"
	"k8s.io/test-infra/boskos/ranch"
)

func MakeTestRanch(resources []common.Resource) *ranch.Ranch {
	resourceClient := crds.NewTestResourceClient()
	s, _ := ranch.NewStorage(crds.NewCRDStorage(resourceClient), "")
	for _, r := range resources {
		s.AddResource(r)
	}
	r, _ := ranch.NewRanch("", s)
	return r
}

func TestAcquire(t *testing.T) {
	var testcases = []struct {
		name      string
		resources []common.Resource
		path      string
		code      int
		method    string
	}{
		{
			name:      "reject get method",
			resources: []common.Resource{},
			path:      "?type=t&state=s&dest=d&owner=o",
			code:      http.StatusMethodNotAllowed,
			method:    http.MethodGet,
		},
		{
			name:      "reject request no arg",
			resources: []common.Resource{},
			path:      "",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "reject request missing type",
			resources: []common.Resource{},
			path:      "?state=s&dest=d&owner=o",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "reject request missing state",
			resources: []common.Resource{},
			path:      "?type=t&dest=d&owner=o",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "reject request missing owner",
			resources: []common.Resource{},
			path:      "?type=t&state=s&dest=d",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "reject request missing dest",
			resources: []common.Resource{},
			path:      "?type=t&state=s&owner=o",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "ranch has no resource",
			resources: []common.Resource{},
			path:      "?type=t&state=s&dest=d&owner=o",
			code:      http.StatusNotFound,
			method:    http.MethodPost,
		},
		{
			name: "no match type",
			resources: []common.Resource{
				{
					Name:  "res",
					Type:  "wrong",
					State: "s",
					Owner: "",
				},
			},
			path:   "?type=t&state=s&dest=d&owner=o",
			code:   http.StatusNotFound,
			method: http.MethodPost,
		},
		{
			name: "no match state",
			resources: []common.Resource{
				{
					Name:  "res",
					Type:  "t",
					State: "wrong",
					Owner: "",
				},
			},
			path:   "?type=t&state=s&dest=d&owner=o",
			code:   http.StatusNotFound,
			method: http.MethodPost,
		},
		{
			name: "busy",
			resources: []common.Resource{
				{
					Name:  "res",
					Type:  "t",
					State: "s",
					Owner: "user",
				},
			},
			path:   "?type=t&state=s&dest=d&owner=o",
			code:   http.StatusNotFound,
			method: http.MethodPost,
		},
		{
			name: "ok",
			resources: []common.Resource{
				{
					Name:  "res",
					Type:  "t",
					State: "s",
					Owner: "",
				},
			},
			path:   "?type=t&state=s&dest=d&owner=o",
			code:   http.StatusOK,
			method: http.MethodPost,
		},
	}

	for _, tc := range testcases {
		c := MakeTestRanch(tc.resources)
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
			var data common.Resource
			json.Unmarshal(rr.Body.Bytes(), &data)
			if data.Name != "res" {
				t.Errorf("%s - Got res %v, expect res", tc.name, data.Name)
			}

			if data.State != "d" {
				t.Errorf("%s - Got state %v, expect d", tc.name, data.State)
			}

			resources, err := c.Storage.GetResources()
			if err != nil {
				t.Error("cannot get resources")
				continue
			}
			if resources[0].Owner != "o" {
				t.Errorf("%s - Wrong owner. Got %v, expect o", tc.name, resources[0].Owner)
			}
		}
	}
}

func TestRelease(t *testing.T) {
	var testcases = []struct {
		name      string
		resources []common.Resource
		path      string
		code      int
		method    string
	}{
		{
			name:      "reject get method",
			resources: []common.Resource{},
			path:      "?name=res&dest=d&owner=foo",
			code:      http.StatusMethodNotAllowed,
			method:    http.MethodGet,
		},
		{
			name:      "reject request no arg",
			resources: []common.Resource{},
			path:      "",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "reject request missing name",
			resources: []common.Resource{},
			path:      "?dest=d&owner=foo",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "reject request missing dest",
			resources: []common.Resource{},
			path:      "?name=res&owner=foo",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "reject request missing owner",
			resources: []common.Resource{},
			path:      "?name=res&dest=d",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "ranch has no resource",
			resources: []common.Resource{},
			path:      "?name=res&dest=d&owner=foo",
			code:      http.StatusNotFound,
			method:    http.MethodPost,
		},
		{
			name: "wrong owner",
			resources: []common.Resource{
				{
					Name:  "res",
					Type:  "t",
					State: "s",
					Owner: "merlin",
				},
			},
			path:   "?name=res&dest=d&owner=foo",
			code:   http.StatusUnauthorized,
			method: http.MethodPost,
		},
		{
			name: "no match name",
			resources: []common.Resource{
				{
					Name:  "foo",
					Type:  "t",
					State: "s",
					Owner: "merlin",
				},
			},
			path:   "?name=res&dest=d&owner=merlin",
			code:   http.StatusNotFound,
			method: http.MethodPost,
		},
		{
			name: "ok",
			resources: []common.Resource{
				{
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
		c := MakeTestRanch(tc.resources)
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
			resources, err := c.Storage.GetResources()
			if err != nil {
				t.Error("cannot get resources")
				continue
			}
			if resources[0].State != "d" {
				t.Errorf("%s - Wrong state. Got %v, expect d", tc.name, resources[0].State)
			}

			if resources[0].Owner != "" {
				t.Errorf("%s - Wrong owner. Got %v, expect empty", tc.name, resources[0].Owner)
			}
		}
	}
}

func TestReset(t *testing.T) {
	var testcases = []struct {
		name       string
		resources  []common.Resource
		path       string
		code       int
		method     string
		hasContent bool
	}{
		{
			name:      "reject get method",
			resources: []common.Resource{},
			path:      "?type=t&state=s&expire=10m&dest=d",
			code:      http.StatusMethodNotAllowed,
			method:    http.MethodGet,
		},
		{
			name:      "reject request no arg",
			resources: []common.Resource{},
			path:      "",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "reject request missing type",
			resources: []common.Resource{},
			path:      "?state=s&expire=10m&dest=d",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "reject request missing state",
			resources: []common.Resource{},
			path:      "?type=t&expire=10m&dest=d",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "reject request missing expire",
			resources: []common.Resource{},
			path:      "?type=t&state=s&dest=d",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "reject request missing dest",
			resources: []common.Resource{},
			path:      "?type=t&state=s&expire=10m",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "reject request bad expire",
			resources: []common.Resource{},
			path:      "?type=t&state=s&expire=woooo&dest=d",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name: "empty - has no owner",
			resources: []common.Resource{
				{
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
			resources: []common.Resource{
				{
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
			resources: []common.Resource{
				{
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
			resources: []common.Resource{
				{
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
			resources: []common.Resource{
				{
					Name:       "res",
					Type:       "t",
					State:      "s",
					Owner:      "user",
					LastUpdate: time.Now().Add(-time.Minute * 20),
				},
			},
			path:       "?type=t&state=s&expire=10m&dest=d",
			code:       http.StatusOK,
			method:     http.MethodPost,
			hasContent: true,
		},
	}

	for _, tc := range testcases {
		c := MakeTestRanch(tc.resources)
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
			if !tc.hasContent {
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
		resources []common.Resource
		path      string
		code      int
		method    string
	}{
		{
			name:      "reject get method",
			resources: []common.Resource{},
			path:      "?name=foo",
			code:      http.StatusMethodNotAllowed,
			method:    http.MethodGet,
		},
		{
			name:      "reject request no arg",
			resources: []common.Resource{},
			path:      "",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "reject request missing name",
			resources: []common.Resource{},
			path:      "?state=s&owner=merlin",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "reject request missing owner",
			resources: []common.Resource{},
			path:      "?name=res&state=s",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "reject request missing state",
			resources: []common.Resource{},
			path:      "?name=res&owner=merlin",
			code:      http.StatusBadRequest,
			method:    http.MethodPost,
		},
		{
			name:      "ranch has no resource",
			resources: []common.Resource{},
			path:      "?name=res&state=s&owner=merlin",
			code:      http.StatusNotFound,
			method:    http.MethodPost,
		},
		{
			name: "wrong owner",
			resources: []common.Resource{
				{
					Name:  "res",
					Type:  "t",
					State: "s",
					Owner: "evil",
				},
			},
			path:   "?name=res&state=s&owner=merlin",
			code:   http.StatusUnauthorized,
			method: http.MethodPost,
		},
		{
			name: "wrong state",
			resources: []common.Resource{
				{
					Name:  "res",
					Type:  "t",
					State: "s",
					Owner: "merlin",
				},
			},
			path:   "?name=res&state=d&owner=merlin",
			code:   http.StatusConflict,
			method: http.MethodPost,
		},
		{
			name: "no matched resource",
			resources: []common.Resource{
				{
					Name:  "res",
					Type:  "t",
					State: "s",
					Owner: "merlin",
				},
			},
			path:   "?name=foo&state=s&owner=merlin",
			code:   http.StatusNotFound,
			method: http.MethodPost,
		},
		{
			name: "ok",
			resources: []common.Resource{
				{
					Name:       "res",
					Type:       "t",
					State:      "s",
					Owner:      "merlin",
					LastUpdate: FakeNow,
				},
			},
			path:   "?name=res&state=s&owner=merlin",
			code:   http.StatusOK,
			method: http.MethodPost,
		},
		{
			name: "ok",
			resources: []common.Resource{
				{
					Name:       "res",
					Type:       "t",
					State:      "s",
					Owner:      "merlin",
					LastUpdate: FakeNow,
				},
			},
			path:   "?name=res&state=s&owner=merlin",
			code:   http.StatusOK,
			method: http.MethodPost,
		},
	}

	for _, tc := range testcases {
		c := MakeTestRanch(tc.resources)
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
			resources, err := c.Storage.GetResources()
			if err != nil {
				t.Error("cannot get resources")
				continue
			}
			if resources[0].LastUpdate == FakeNow {
				t.Errorf("%s - Timestamp is not updated!", tc.name)
			}
		}
	}
}

func TestGetMetric(t *testing.T) {
	var testcases = []struct {
		name      string
		resources []common.Resource
		path      string
		code      int
		method    string
		expect    common.Metric
	}{
		{
			name:      "reject none-get method",
			resources: []common.Resource{},
			path:      "?type=t",
			code:      http.StatusMethodNotAllowed,
			method:    http.MethodPost,
		},
		{
			name:      "reject request no type",
			resources: []common.Resource{},
			path:      "",
			code:      http.StatusBadRequest,
			method:    http.MethodGet,
		},
		{
			name:      "ranch has no resource",
			resources: []common.Resource{},
			path:      "?type=t",
			code:      http.StatusNotFound,
			method:    http.MethodGet,
		},
		{
			name: "wrong type",
			resources: []common.Resource{
				{
					Name:  "res",
					Type:  "t",
					State: "s",
					Owner: "evil",
				},
			},
			path:   "?type=foo",
			code:   http.StatusNotFound,
			method: http.MethodGet,
		},
		{
			name: "ok",
			resources: []common.Resource{
				{
					Name:  "res",
					Type:  "t",
					State: "s",
					Owner: "merlin",
				},
			},
			path:   "?type=t",
			code:   http.StatusOK,
			method: http.MethodGet,
			expect: common.Metric{
				Type: "t",
				Current: map[string]int{
					"s": 1,
				},
				Owners: map[string]int{
					"merlin": 1,
				},
			},
		},
	}

	for _, tc := range testcases {
		c := MakeTestRanch(tc.resources)
		handler := handleMetric(c)
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
			var metric common.Metric
			if err := json.Unmarshal(rr.Body.Bytes(), &metric); err != nil {
				t.Errorf("%s - Fail to unmarshal body - %s", tc.name, err)
			}
			if !reflect.DeepEqual(metric, tc.expect) {
				t.Errorf("%s - wrong metric, got %v, want %v", tc.name, metric, tc.expect)
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
		handler := handleDefault(nil)
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

func TestConfig(t *testing.T) {
	resources, err := ranch.ParseConfig("resources.yaml")
	if err != nil {
		t.Errorf("parseConfig error: %v", err)
	}

	if len(resources) == 0 {
		t.Errorf("empty data")
	}
	resourceNames := map[string]bool{}

	for _, p := range resources {
		if p.Name == "" {
			t.Errorf("empty resource name: %v", p.Name)
		}

		if _, ok := resourceNames[p.Name]; ok {
			t.Errorf("duplicated resource name: %v", p.Name)
		} else {
			resourceNames[p.Name] = true
		}
	}
}
