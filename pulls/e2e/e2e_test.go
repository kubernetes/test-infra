/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"k8s.io/contrib/mungegithub/pulls/jenkins"
)

type testHandler struct {
	handler func(http.ResponseWriter, *http.Request)
}

func (t *testHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	t.handler(res, req)
}

func marshalOrDie(obj interface{}, t *testing.T) []byte {
	data, err := json.Marshal(obj)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	return data
}

func TestCheckBuilds(t *testing.T) {
	tests := []struct {
		paths          map[string][]byte
		expectStable   bool
		expectedStatus map[string]string
	}{
		{
			paths: map[string][]byte{
				"/job/foo/lastCompletedBuild/api/json": marshalOrDie(jenkins.Job{
					Result: "SUCCESS",
				}, t),
				"/job/bar/lastCompletedBuild/api/json": marshalOrDie(jenkins.Job{
					Result: "SUCCESS",
				}, t),
			},
			expectStable:   true,
			expectedStatus: map[string]string{"foo": "Stable", "bar": "Stable"},
		},
		{
			paths: map[string][]byte{
				"/job/foo/lastCompletedBuild/api/json": marshalOrDie(jenkins.Job{
					Result: "SUCCESS",
				}, t),
				"/job/bar/lastCompletedBuild/api/json": marshalOrDie(jenkins.Job{
					Result: "UNSTABLE",
				}, t),
			},
			expectStable:   false,
			expectedStatus: map[string]string{"foo": "Stable", "bar": "Not Stable"},
		},
		{
			paths: map[string][]byte{
				"/job/foo/lastCompletedBuild/api/json": marshalOrDie(jenkins.Job{
					Result: "SUCCESS",
				}, t),
				"/job/bar/lastCompletedBuild/api/json": marshalOrDie(jenkins.Job{
					Result: "FAILURE",
				}, t),
			},
			expectStable:   false,
			expectedStatus: map[string]string{"foo": "Stable", "bar": "Not Stable"},
		},
		{
			paths: map[string][]byte{
				"/job/foo/lastCompletedBuild/api/json": marshalOrDie(jenkins.Job{
					Result: "FAILURE",
				}, t),
				"/job/bar/lastCompletedBuild/api/json": marshalOrDie(jenkins.Job{
					Result: "SUCCESS",
				}, t),
			},
			expectStable:   false,
			expectedStatus: map[string]string{"foo": "Not Stable", "bar": "Stable"},
		},
	}
	for _, test := range tests {
		server := httptest.NewServer(&testHandler{
			handler: func(res http.ResponseWriter, req *http.Request) {
				data, found := test.paths[req.URL.Path]
				if !found {
					res.WriteHeader(http.StatusNotFound)
					fmt.Fprintf(res, "Unknown path: %s", req.URL.Path)
					return
				}
				res.WriteHeader(http.StatusOK)
				res.Write(data)
			},
		})
		e2e := &E2ETester{
			JenkinsHost: server.URL,
			JenkinsJobs: []string{
				"foo",
				"bar",
			},
			State: &ExternalState{
				BuildStatus: map[string]string{},
			},
		}
		stable := e2e.checkBuilds()
		if stable != test.expectStable {
			t.Errorf("expected: %v, saw: %v", test.expectStable, stable)
		}
		if !reflect.DeepEqual(test.expectedStatus, e2e.State.BuildStatus) {
			t.Errorf("expected: %v, saw: %v", test.expectedStatus, e2e.State.BuildStatus)
		}
	}
}

func TestMsg(t *testing.T) {
	e2e := &E2ETester{State: &ExternalState{}}
	for i := 1; i <= 50; i++ {
		e2e.msg("FOO: %d", i)
		if len(e2e.State.Message) != i {
			t.Errorf("unexpected message list length. expected %d, saw %d.", i, len(e2e.State.Message))
		}
		expectedMsg := fmt.Sprintf("FOO: %d", i)
		if !strings.Contains(e2e.State.Message[i-1], expectedMsg) {
			t.Errorf("expected: %s, saw: %s", expectedMsg, e2e.State.Message[i-1])
		}
	}
	// test clipping
	e2e.msg("FOO: 51")
	if len(e2e.State.Message) != 50 {
		t.Errorf("expected to clip at 50, len is %d", len(e2e.State.Message))
	}
	if !strings.Contains(e2e.State.Message[49], "FOO: 51") {
		t.Errorf("expected to find FOO: 51, found: %s", e2e.State.Message[49])
	}
	if !strings.Contains(e2e.State.Message[0], "FOO: 2") {
		t.Errorf("expected to find FOO: 2, found: %s", e2e.State.Message[49])
	}
}
