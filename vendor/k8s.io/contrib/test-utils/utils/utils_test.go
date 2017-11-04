/*
Copyright 2015 The Kubernetes Authors.

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

package utils

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetPathToJenkinsGoogleBucket(t *testing.T) {
	const (
		bucket  = "kubernetes-jenkins"
		dir     = "logs"
		pullDir = "pr-logs"
		pullKey = "pull"
	)
	table := []struct {
		job    string
		build  int
		expect string
	}{
		{
			job:    "kubernetes-gce-e2e",
			build:  1458,
			expect: "/kubernetes-jenkins/logs/kubernetes-gce-e2e/1458/",
		},
		{
			job:    "kubernetes-pull-build-test-e2e-gce",
			build:  46924,
			expect: "/kubernetes-jenkins/pr-logs/pull/27898/kubernetes-pull-build-test-e2e-gce/46924/",
		},
	}

	m := http.NewServeMux()
	m.HandleFunc(
		"/kubernetes-jenkins/pr-logs/directory/kubernetes-pull-build-test-e2e-gce/46924.txt",
		func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "gs://kubernetes-jenkins/pr-logs/pull/27898/kubernetes-pull-build-test-e2e-gce/46924\n")
		},
	)
	m.HandleFunc(
		"/",
		func(w http.ResponseWriter, req *http.Request) {
			t.Errorf("Unexpected request to %v", req.URL.String())
			http.NotFound(w, req)
		},
	)
	server := httptest.NewServer(m)
	defer server.Close()

	for _, tt := range table {
		u := NewWithPresubmitDetection(bucket, dir, pullKey, pullDir)
		u.bucket = NewTestBucket(bucket, server.URL)
		out := u.GetPathToJenkinsGoogleBucket(tt.job, tt.build)
		if out != tt.expect {
			t.Errorf("Expected %v but got %v", tt.expect, out)
		}
	}
}
