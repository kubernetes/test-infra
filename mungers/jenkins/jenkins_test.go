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

package jenkins

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
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

func TestGetLatestCompletedBuild(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		obj       *Job
		stable    bool
		expectErr bool
	}{
		{
			name: "foo",
			path: "/job/foo/lastCompletedBuild/api/json",
			obj:  &Job{Result: "UNSTABLE"},
		},
		{
			name:   "bar",
			path:   "/job/bar/lastCompletedBuild/api/json",
			obj:    &Job{Result: "SUCCESS"},
			stable: true,
		},
		{
			name:      "baz",
			expectErr: true,
		},
	}
	for _, test := range tests {
		server := httptest.NewServer(&testHandler{
			handler: func(res http.ResponseWriter, req *http.Request) {
				if test.path != req.URL.Path {
					res.WriteHeader(http.StatusNotFound)
					fmt.Fprintf(res, "Unknown path: %s", req.URL.Path)
					return
				}
				res.WriteHeader(http.StatusOK)
				res.Write(marshalOrDie(test.obj, t))
			},
		})
		client := &JenkinsClient{Host: server.URL}
		job, err := client.GetLastCompletedBuild(test.name)
		if test.expectErr {
			if err == nil {
				t.Errorf("unexpected non-error")
			}
			continue
		}
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(job, test.obj) {
			t.Errorf("expected:\n%#v\nsaw:%#v\n", test.obj, job)
		}
		stable := job.IsStable()
		if stable != test.stable {
			t.Errorf("expected stable=%v but got %v", test.stable, stable)
		}
	}
}

func TestJob(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		obj       *Queue
		expectErr bool
	}{
		{
			name: "foo",
			path: "/job/foo/api/json",
			obj: &Queue{
				Builds: []Build{
					{
						Number: 1,
						URL:    "http://foo.bar/baz",
					},
				},
			},
		},
		{
			name: "bar",
			path: "/job/bar/api/json",
			obj: &Queue{
				Builds: []Build{
					{
						Number: 2,
						URL:    "http://bar.baz/foo",
					},
				},
			},
		},
		{
			name:      "baz",
			expectErr: true,
		},
	}
	for _, test := range tests {
		server := httptest.NewServer(&testHandler{
			handler: func(res http.ResponseWriter, req *http.Request) {
				if test.path != req.URL.Path {
					res.WriteHeader(http.StatusNotFound)
					fmt.Fprintf(res, "Unknown path: %s", req.URL.Path)
					return
				}
				res.WriteHeader(http.StatusOK)
				res.Write(marshalOrDie(test.obj, t))
			},
		})
		client := &JenkinsClient{Host: server.URL}
		job, err := client.GetJob(test.name)
		if test.expectErr {
			if err == nil {
				t.Errorf("unexpected non-error")
			}
			continue
		}
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(job, test.obj) {
			t.Errorf("expected:\n%#v\nsaw:%#v\n", test.obj, job)
		}
	}
}
