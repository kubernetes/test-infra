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

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"k8s.io/test-infra/boskos/client"
	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/crds"
	"k8s.io/test-infra/boskos/ranch"
)

var update = flag.Bool("update", false, "If the fixtures should be updated")

func init() {
	// Don't actually sleep in tests
	client.SleepFunc = func(_ time.Duration) {}
}

// json does not serialized time with nanosecond precision
func now() time.Time {
	format := "2006-01-02 15:04:05.000"
	now, _ := time.Parse(format, format)
	return now
}

var (
	fakeNow = now()
	testTTL = time.Millisecond
)

func MakeTestRanch(resources []runtime.Object) *ranch.Ranch {
	const ns = "test"
	for _, obj := range resources {
		obj.(metav1.Object).SetNamespace(ns)
	}
	client := &onceConflictingClient{Client: fakectrlruntimeclient.NewFakeClient(resources...)}
	s := ranch.NewTestingStorage(client, ns, func() time.Time { return fakeNow })
	r, _ := ranch.NewRanch("", s, testTTL)
	return r
}

func TestAcquire(t *testing.T) {
	var testcases = []struct {
		name      string
		resources []runtime.Object
		path      string
		code      int
		method    string
	}{
		{
			name:   "reject get method",
			path:   "?type=t&state=s&dest=d&owner=o",
			code:   http.StatusMethodNotAllowed,
			method: http.MethodGet,
		},
		{
			name:   "reject request no arg",
			path:   "",
			code:   http.StatusBadRequest,
			method: http.MethodPost,
		},
		{
			name:   "reject request missing type",
			path:   "?state=s&dest=d&owner=o",
			code:   http.StatusBadRequest,
			method: http.MethodPost,
		},
		{
			name:   "reject request missing state",
			path:   "?type=t&dest=d&owner=o",
			code:   http.StatusBadRequest,
			method: http.MethodPost,
		},
		{
			name:   "reject request missing owner",
			path:   "?type=t&state=s&dest=d",
			code:   http.StatusBadRequest,
			method: http.MethodPost,
		},
		{
			name:   "reject request missing dest",
			path:   "?type=t&state=s&owner=o",
			code:   http.StatusBadRequest,
			method: http.MethodPost,
		},
		{
			name:   "ranch has no resource",
			path:   "?type=t&state=s&dest=d&owner=o",
			code:   http.StatusNotFound,
			method: http.MethodPost,
		},
		{
			name: "no match type",
			resources: []runtime.Object{&crds.ResourceObject{
				ObjectMeta: metav1.ObjectMeta{
					Name: "res",
				},
				Spec: crds.ResourceSpec{
					Type: "wrong",
				},
				Status: crds.ResourceStatus{
					State: "s",
				},
			}},
			path:   "?type=t&state=s&dest=d&owner=o",
			code:   http.StatusNotFound,
			method: http.MethodPost,
		},
		{
			name: "no match state",
			resources: []runtime.Object{&crds.ResourceObject{
				ObjectMeta: metav1.ObjectMeta{
					Name: "res",
				},
				Spec: crds.ResourceSpec{
					Type: "t",
				},
				Status: crds.ResourceStatus{
					State: "wrong",
				},
			}},
			path:   "?type=t&state=s&dest=d&owner=o",
			code:   http.StatusNotFound,
			method: http.MethodPost,
		},
		{
			name: "busy",
			resources: []runtime.Object{&crds.ResourceObject{
				ObjectMeta: metav1.ObjectMeta{
					Name: "res",
				},
				Spec: crds.ResourceSpec{
					Type: "t",
				},
				Status: crds.ResourceStatus{
					State: "s",
					Owner: "user",
				},
			}},
			path:   "?type=t&state=s&dest=d&owner=o",
			code:   http.StatusNotFound,
			method: http.MethodPost,
		},
		{
			name: "ok",
			resources: []runtime.Object{&crds.ResourceObject{
				ObjectMeta: metav1.ObjectMeta{
					Name: "res",
				},
				Spec: crds.ResourceSpec{
					Type: "t",
				},
				Status: crds.ResourceStatus{
					State: "s",
				},
			}},
			path:   "?type=t&state=s&dest=d&owner=o",
			code:   http.StatusOK,
			method: http.MethodPost,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
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
				responseData := rr.Body.Bytes()
				if err := compareWithFixture(t.Name(), responseData); err != nil {
					t.Errorf("response does not match fixture: %v", err)
				}
				var data common.Resource
				if err := json.Unmarshal(responseData, &data); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if data.Name != "res" {
					t.Errorf("%s - Got res %v, expect res", tc.name, data.Name)
				}

				if data.State != "d" {
					t.Errorf("%s - Got state %v, expect d", tc.name, data.State)
				}

				resources, err := c.Storage.GetResources()
				if err != nil {
					t.Fatalf("error getting resource: %v", err)
				}
				if resources.Items[0].Status.Owner != "o" {
					t.Errorf("%s - Wrong owner. Got %v, expect o", tc.name, resources.Items[0].Status.Owner)
				}
			}
		})
	}
}

func TestRelease(t *testing.T) {
	var testcases = []struct {
		name      string
		resources []runtime.Object
		path      string
		code      int
		method    string
	}{
		{
			name:   "reject get method",
			path:   "?name=res&dest=d&owner=foo",
			code:   http.StatusMethodNotAllowed,
			method: http.MethodGet,
		},
		{
			name:   "reject request no arg",
			path:   "",
			code:   http.StatusBadRequest,
			method: http.MethodPost,
		},
		{
			name:   "reject request missing name",
			path:   "?dest=d&owner=foo",
			code:   http.StatusBadRequest,
			method: http.MethodPost,
		},
		{
			name:   "reject request missing dest",
			path:   "?name=res&owner=foo",
			code:   http.StatusBadRequest,
			method: http.MethodPost,
		},
		{
			name:   "reject request missing owner",
			path:   "?name=res&dest=d",
			code:   http.StatusBadRequest,
			method: http.MethodPost,
		},
		{
			name:   "ranch has no resource",
			path:   "?name=res&dest=d&owner=foo",
			code:   http.StatusNotFound,
			method: http.MethodPost,
		},
		{
			name: "wrong owner",
			resources: []runtime.Object{&crds.ResourceObject{
				ObjectMeta: metav1.ObjectMeta{
					Name: "res",
				},
				Spec: crds.ResourceSpec{
					Type: "t",
				},
				Status: crds.ResourceStatus{
					State: "s",
					Owner: "merlin",
				},
			}},
			path:   "?name=res&dest=d&owner=foo",
			code:   http.StatusUnauthorized,
			method: http.MethodPost,
		},
		{
			name: "no match name",
			resources: []runtime.Object{&crds.ResourceObject{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: crds.ResourceSpec{
					Type: "t",
				},
				Status: crds.ResourceStatus{
					State: "s",
					Owner: "merlin",
				},
			}},
			path:   "?name=res&dest=d&owner=merlin",
			code:   http.StatusNotFound,
			method: http.MethodPost,
		},
		{
			name: "ok",
			resources: []runtime.Object{&crds.ResourceObject{
				ObjectMeta: metav1.ObjectMeta{
					Name: "res",
				},
				Spec: crds.ResourceSpec{
					Type: "t",
				},
				Status: crds.ResourceStatus{
					State: "s",
					Owner: "merlin",
				},
			}},
			path:   "?name=res&dest=d&owner=merlin",
			code:   http.StatusOK,
			method: http.MethodPost,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
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
				if err := compareWithFixture(t.Name(), rr.Body.Bytes()); err != nil {
					t.Errorf("response does not match fixture: %v", err)
				}
				resources, err := c.Storage.GetResources()
				if err != nil {
					t.Fatalf("error getting resource: %v", err)
				}
				if resources.Items[0].Status.State != "d" {
					t.Errorf("%s - Wrong state. Got %v, expect d", tc.name, resources.Items[0].Status.State)
				}

				if resources.Items[0].Status.Owner != "" {
					t.Errorf("%s - Wrong owner. Got %v, expect empty", tc.name, resources.Items[0].Status.Owner)
				}
			}
		})
	}
}

func TestReset(t *testing.T) {
	var testcases = []struct {
		name       string
		resources  []runtime.Object
		path       string
		code       int
		method     string
		hasContent bool
	}{
		{
			name:   "reject get method",
			path:   "?type=t&state=s&expire=10m&dest=d",
			code:   http.StatusMethodNotAllowed,
			method: http.MethodGet,
		},
		{
			name:   "reject request no arg",
			path:   "",
			code:   http.StatusBadRequest,
			method: http.MethodPost,
		},
		{
			name:   "reject request missing type",
			path:   "?state=s&expire=10m&dest=d",
			code:   http.StatusBadRequest,
			method: http.MethodPost,
		},
		{
			name:   "reject request missing state",
			path:   "?type=t&expire=10m&dest=d",
			code:   http.StatusBadRequest,
			method: http.MethodPost,
		},
		{
			name:   "reject request missing expire",
			path:   "?type=t&state=s&dest=d",
			code:   http.StatusBadRequest,
			method: http.MethodPost,
		},
		{
			name:   "reject request missing dest",
			path:   "?type=t&state=s&expire=10m",
			code:   http.StatusBadRequest,
			method: http.MethodPost,
		},
		{
			name:   "reject request bad expire",
			path:   "?type=t&state=s&expire=woooo&dest=d",
			code:   http.StatusBadRequest,
			method: http.MethodPost,
		},
		{
			name: "empty - has no owner",
			resources: []runtime.Object{&crds.ResourceObject{
				ObjectMeta: metav1.ObjectMeta{
					Name: "res",
				},
				Spec: crds.ResourceSpec{
					Type: "t",
				},
				Status: crds.ResourceStatus{
					State:      "s",
					LastUpdate: time.Now().Add(-time.Minute * 20),
				},
			}},
			path:   "?type=t&state=s&expire=10m&dest=d",
			code:   http.StatusOK,
			method: http.MethodPost,
		},
		{
			name: "empty - not expire",
			resources: []runtime.Object{&crds.ResourceObject{
				ObjectMeta: metav1.ObjectMeta{
					Name: "res",
				},
				Spec: crds.ResourceSpec{
					Type: "t",
				},
				Status: crds.ResourceStatus{
					State:      "s",
					LastUpdate: time.Now(),
				},
			}},
			path:   "?type=t&state=s&expire=10m&dest=d",
			code:   http.StatusOK,
			method: http.MethodPost,
		},
		{
			name: "empty - no match type",
			resources: []runtime.Object{&crds.ResourceObject{
				ObjectMeta: metav1.ObjectMeta{
					Name: "res",
				},
				Spec: crds.ResourceSpec{
					Type: "wrong",
				},
				Status: crds.ResourceStatus{
					State:      "s",
					LastUpdate: time.Now().Add(-time.Minute * 20),
				},
			}},
			path:   "?type=t&state=s&expire=10m&dest=d",
			code:   http.StatusOK,
			method: http.MethodPost,
		},
		{
			name: "empty - no match state",
			resources: []runtime.Object{&crds.ResourceObject{
				ObjectMeta: metav1.ObjectMeta{
					Name: "res",
				},
				Spec: crds.ResourceSpec{
					Type: "t",
				},
				Status: crds.ResourceStatus{
					State:      "wrong",
					LastUpdate: time.Now().Add(-time.Minute * 20),
				},
			}},
			path:   "?type=t&state=s&expire=10m&dest=d",
			code:   http.StatusOK,
			method: http.MethodPost,
		},
		{
			name: "ok",
			resources: []runtime.Object{&crds.ResourceObject{
				ObjectMeta: metav1.ObjectMeta{
					Name: "res",
				},
				Spec: crds.ResourceSpec{
					Type: "t",
				},
				Status: crds.ResourceStatus{
					State:      "s",
					Owner:      "user",
					LastUpdate: time.Now().Add(-time.Minute * 20),
				},
			}},
			path:       "?type=t&state=s&expire=10m&dest=d",
			code:       http.StatusOK,
			method:     http.MethodPost,
			hasContent: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
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
				responseData := rr.Body.Bytes()
				if err := compareWithFixture(t.Name(), responseData); err != nil {
					t.Errorf("response does not match fixture: %v", err)
				}
				rmap := make(map[string]string)
				json.Unmarshal(responseData, &rmap)
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
		})
	}
}

func TestUpdate(t *testing.T) {
	FakeNow := time.Now()

	var testcases = []struct {
		name      string
		resources []runtime.Object
		path      string
		code      int
		method    string
	}{
		{
			name:   "reject get method",
			path:   "?name=foo",
			code:   http.StatusMethodNotAllowed,
			method: http.MethodGet,
		},
		{
			name:   "reject request no arg",
			path:   "",
			code:   http.StatusBadRequest,
			method: http.MethodPost,
		},
		{
			name:   "reject request missing name",
			path:   "?state=s&owner=merlin",
			code:   http.StatusBadRequest,
			method: http.MethodPost,
		},
		{
			name:   "reject request missing owner",
			path:   "?name=res&state=s",
			code:   http.StatusBadRequest,
			method: http.MethodPost,
		},
		{
			name:   "reject request missing state",
			path:   "?name=res&owner=merlin",
			code:   http.StatusBadRequest,
			method: http.MethodPost,
		},
		{
			name:   "ranch has no resource",
			path:   "?name=res&state=s&owner=merlin",
			code:   http.StatusNotFound,
			method: http.MethodPost,
		},
		{
			name: "wrong owner",
			resources: []runtime.Object{&crds.ResourceObject{
				ObjectMeta: metav1.ObjectMeta{
					Name: "res",
				},
				Spec: crds.ResourceSpec{
					Type: "t",
				},
				Status: crds.ResourceStatus{
					State: "s",
					Owner: "evil",
				},
			}},
			path:   "?name=res&state=s&owner=merlin",
			code:   http.StatusUnauthorized,
			method: http.MethodPost,
		},
		{
			name: "wrong state",
			resources: []runtime.Object{&crds.ResourceObject{
				ObjectMeta: metav1.ObjectMeta{
					Name: "res",
				},
				Spec: crds.ResourceSpec{
					Type: "t",
				},
				Status: crds.ResourceStatus{
					State: "s",
					Owner: "merlin",
				},
			}},
			path:   "?name=res&state=d&owner=merlin",
			code:   http.StatusConflict,
			method: http.MethodPost,
		},
		{
			name: "no matched resource",
			resources: []runtime.Object{&crds.ResourceObject{
				ObjectMeta: metav1.ObjectMeta{
					Name: "res",
				},
				Spec: crds.ResourceSpec{
					Type: "t",
				},
				Status: crds.ResourceStatus{
					State: "s",
					Owner: "merlin",
				},
			}},
			path:   "?name=foo&state=s&owner=merlin",
			code:   http.StatusNotFound,
			method: http.MethodPost,
		},
		{
			name: "ok",
			resources: []runtime.Object{&crds.ResourceObject{
				ObjectMeta: metav1.ObjectMeta{
					Name: "res",
				},
				Spec: crds.ResourceSpec{
					Type: "t",
				},
				Status: crds.ResourceStatus{
					State:      "s",
					Owner:      "merlin",
					LastUpdate: FakeNow,
				},
			}},
			path:   "?name=res&state=s&owner=merlin",
			code:   http.StatusOK,
			method: http.MethodPost,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
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
				if err := compareWithFixture(t.Name(), rr.Body.Bytes()); err != nil {
					t.Errorf("response does not match fixture: %v", err)
				}
				resources, err := c.Storage.GetResources()
				if err != nil {
					t.Fatalf("error getting resources: %v", err)
				}
				if resources.Items[0].Status.LastUpdate == FakeNow {
					t.Errorf("%s - Timestamp is not updated!", tc.name)
				}
			}
		})
	}
}

func TestGetMetric(t *testing.T) {
	var testcases = []struct {
		name      string
		resources []runtime.Object
		path      string
		code      int
		method    string
		expect    common.Metric
	}{
		{
			name:   "reject none-get method",
			path:   "?type=t",
			code:   http.StatusMethodNotAllowed,
			method: http.MethodPost,
		},
		{
			name:   "reject request no type",
			path:   "",
			code:   http.StatusBadRequest,
			method: http.MethodGet,
		},
		{
			name:   "ranch has no resource",
			path:   "?type=t",
			code:   http.StatusNotFound,
			method: http.MethodGet,
		},
		{
			name: "wrong type",
			resources: []runtime.Object{&crds.ResourceObject{
				ObjectMeta: metav1.ObjectMeta{
					Name: "res",
				},
				Spec: crds.ResourceSpec{
					Type: "t",
				},
				Status: crds.ResourceStatus{
					State: "s",
					Owner: "evil",
				},
			}},
			path:   "?type=foo",
			code:   http.StatusNotFound,
			method: http.MethodGet,
		},
		{
			name: "ok",
			resources: []runtime.Object{&crds.ResourceObject{
				ObjectMeta: metav1.ObjectMeta{
					Name: "res",
				},
				Spec: crds.ResourceSpec{
					Type: "t",
				},
				Status: crds.ResourceStatus{
					State: "s",
					Owner: "merlin",
				},
			}},
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

func compareWithFixture(testName string, actualData []byte) error {
	goldenFile := fmt.Sprintf("testdata/%s.golden", strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(testName, " ", "_"), "/", "-")))
	if *update {
		ioutil.WriteFile(goldenFile, actualData, 0644)
	}
	expected, err := ioutil.ReadFile(goldenFile)
	if err != nil {
		return fmt.Errorf("error reading fixture %q: %v", goldenFile, err)
	}
	if !bytes.Equal(expected, actualData) {
		return fmt.Errorf("fixture %s\n%s\n does not match received data\n%s\n. If this is expeted, please re-run the test with `-update` to update the fixture", goldenFile, string(expected), string(actualData))
	}

	return nil
}

// onceConflictingClient returns an IsConflict error on the first Update request it receives. It
// is used to verify that there is retrying for conflicts in place.
type onceConflictingClient struct {
	didConflict bool
	ctrlruntimeclient.Client
}

func (occ *onceConflictingClient) Update(ctx context.Context, obj runtime.Object, opts ...ctrlruntimeclient.UpdateOption) error {
	if !occ.didConflict {
		occ.didConflict = true
		return kerrors.NewConflict(schema.GroupResource{}, "obj", errors.New("conflicting as requested"))
	}
	return occ.Client.Update(ctx, obj, opts...)
}
