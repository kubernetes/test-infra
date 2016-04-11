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
	"strconv"
	"testing"

	"k8s.io/contrib/mungegithub/mungers/jenkins"
	"k8s.io/contrib/test-utils/utils"
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

func TestCheckJenkinsBuilds(t *testing.T) {
	tests := []struct {
		paths          map[string][]byte
		expectStable   bool
		expectedStatus map[string]BuildInfo
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
			expectedStatus: map[string]BuildInfo{"foo": {"Stable", ""}, "bar": {"Stable", ""}},
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
			expectedStatus: map[string]BuildInfo{"foo": {"Stable", ""}, "bar": {"Not Stable", ""}},
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
			expectedStatus: map[string]BuildInfo{"foo": {"Stable", ""}, "bar": {"Not Stable", ""}},
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
			expectedStatus: map[string]BuildInfo{"foo": {"Not Stable", ""}, "bar": {"Stable", ""}},
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
		e2e := &RealE2ETester{
			JenkinsHost: server.URL,
			JobNames: []string{
				"foo",
				"bar",
			},
			BuildStatus: map[string]BuildInfo{},
		}
		stable := e2e.Stable()
		if stable != test.expectStable {
			t.Errorf("expected: %v, saw: %v", test.expectStable, stable)
		}
		if !reflect.DeepEqual(test.expectedStatus, e2e.BuildStatus) {
			t.Errorf("expected: %v, saw: %v", test.expectedStatus, e2e.BuildStatus)
		}
	}
}

func TestCheckGCSBuilds(t *testing.T) {
	latestBuildNumberFoo := 42
	latestBuildNumberBar := 44
	tests := []struct {
		paths             map[string][]byte
		expectStable      bool
		expectedLastBuild int
		expectedStatus    map[string]BuildInfo
	}{
		{
			paths: map[string][]byte{
				"/foo/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberFoo)),
				fmt.Sprintf("/foo/%v/finished.json", latestBuildNumberFoo): marshalOrDie(utils.FinishedFile{
					Result:    "SUCCESS",
					Timestamp: 1234,
				}, t),
				"/bar/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberBar)),
				fmt.Sprintf("/bar/%v/finished.json", latestBuildNumberBar): marshalOrDie(utils.FinishedFile{
					Result:    "SUCCESS",
					Timestamp: 1234,
				}, t),
			},
			expectStable: true,
			expectedStatus: map[string]BuildInfo{
				"foo": {Status: "Stable", ID: "42"},
				"bar": {Status: "Stable", ID: "44"},
			},
		},
		{
			paths: map[string][]byte{
				"/foo/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberFoo)),
				fmt.Sprintf("/foo/%v/finished.json", latestBuildNumberFoo): marshalOrDie(utils.FinishedFile{
					Result:    "SUCCESS",
					Timestamp: 1234,
				}, t),
				"/bar/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberBar)),
				fmt.Sprintf("/bar/%v/finished.json", latestBuildNumberBar): marshalOrDie(utils.FinishedFile{
					Result:    "UNSTABLE",
					Timestamp: 1234,
				}, t),
			},
			expectStable: false,
			expectedStatus: map[string]BuildInfo{
				"foo": {Status: "Stable", ID: "42"},
				"bar": {Status: "Not Stable", ID: "44"},
			},
		},
		{
			paths: map[string][]byte{
				"/foo/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberFoo)),
				fmt.Sprintf("/foo/%v/finished.json", latestBuildNumberFoo): marshalOrDie(utils.FinishedFile{
					Result:    "SUCCESS",
					Timestamp: 1234,
				}, t),
				"/bar/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberBar)),
				fmt.Sprintf("/bar/%v/finished.json", latestBuildNumberBar): marshalOrDie(utils.FinishedFile{
					Result:    "FAILURE",
					Timestamp: 1234,
				}, t),
			},
			expectStable: false,
			expectedStatus: map[string]BuildInfo{
				"foo": {Status: "Stable", ID: "42"},
				"bar": {Status: "Not Stable", ID: "44"},
			},
		},
		{
			paths: map[string][]byte{
				"/foo/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberFoo)),
				fmt.Sprintf("/foo/%v/finished.json", latestBuildNumberFoo): marshalOrDie(utils.FinishedFile{
					Result:    "FAILURE",
					Timestamp: 1234,
				}, t),
				"/bar/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberBar)),
				fmt.Sprintf("/bar/%v/finished.json", latestBuildNumberBar): marshalOrDie(utils.FinishedFile{
					Result:    "UNSTABLE",
					Timestamp: 1234,
				}, t),
			},
			expectStable: false,
			expectedStatus: map[string]BuildInfo{
				"foo": {Status: "Not Stable", ID: "42"},
				"bar": {Status: "Not Stable", ID: "44"},
			},
		},
		{
			paths: map[string][]byte{
				"/foo/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberFoo)),
				fmt.Sprintf("/foo/%v/finished.json", latestBuildNumberFoo): marshalOrDie(utils.FinishedFile{
					Result:    "UNSTABLE",
					Timestamp: 1234,
				}, t),
				"/bar/latest-build.txt": []byte(strconv.Itoa(latestBuildNumberBar)),
				fmt.Sprintf("/bar/%v/finished.json", latestBuildNumberBar): marshalOrDie(utils.FinishedFile{
					Result:    "SUCCESS",
					Timestamp: 1234,
				}, t),
			},
			expectStable: false,
			expectedStatus: map[string]BuildInfo{
				"foo": {Status: "Not Stable", ID: "42"},
				"bar": {Status: "Stable", ID: "44"},
			},
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
		e2e := &RealE2ETester{
			JenkinsHost: server.URL,
			JobNames: []string{
				"foo",
				"bar",
			},
			BuildStatus:          map[string]BuildInfo{},
			GoogleGCSBucketUtils: utils.NewUtils(server.URL),
		}
		stable := e2e.GCSBasedStable()
		if stable != test.expectStable {
			t.Errorf("expected: %v, saw: %v", test.expectStable, stable)
		}
		if !reflect.DeepEqual(test.expectedStatus, e2e.BuildStatus) {
			t.Errorf("expected: %v, saw: %v", test.expectedStatus, e2e.BuildStatus)
		}
	}
}
