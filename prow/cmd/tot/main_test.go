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
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"

	"k8s.io/test-infra/prow/config"
)

func expectEqual(t *testing.T, msg string, have interface{}, want interface{}) {
	if !reflect.DeepEqual(have, want) {
		t.Errorf("bad %s: got %v, wanted %v",
			msg, have, want)
	}
}

func makeStore(t *testing.T) *store {
	tmp, err := os.CreateTemp("", "tot_test_")
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(tmp.Name()) // json decoding an empty file throws an error

	store, err := newStore(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}

	return store
}

func TestVend(t *testing.T) {
	store := makeStore(t)
	defer os.Remove(store.storagePath)

	expectEqual(t, "empty vend", store.vend("a"), 1)
	expectEqual(t, "second vend", store.vend("a"), 2)
	expectEqual(t, "third vend", store.vend("a"), 3)
	expectEqual(t, "second empty", store.vend("b"), 1)

	store2, err := newStore(store.storagePath)
	if err != nil {
		t.Fatal(err)
	}
	expectEqual(t, "fourth vend, different instance", store2.vend("a"), 4)
}

func TestSet(t *testing.T) {
	store := makeStore(t)
	defer os.Remove(store.storagePath)

	store.set("foo", 300)
	expectEqual(t, "peek", store.peek("foo"), 300)
	store.set("foo2", 300)
	expectEqual(t, "vend", store.vend("foo2"), 301)
	expectEqual(t, "vend", store.vend("foo2"), 302)
}

func expectResponse(t *testing.T, handler http.Handler, req *http.Request, msg, value string) {
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	expectEqual(t, "http status OK", rr.Code, 200)

	expectEqual(t, msg, rr.Body.String(), value)
}

func TestHandler(t *testing.T) {
	store := makeStore(t)
	defer os.Remove(store.storagePath)

	handler := http.HandlerFunc(store.handle)

	req, err := http.NewRequest("GET", "/vend/foo", nil)
	if err != nil {
		t.Fatal(err)
	}

	expectResponse(t, handler, req, "http vend", "1")
	expectResponse(t, handler, req, "http vend", "2")

	req, err = http.NewRequest("HEAD", "/vend/foo", nil)
	if err != nil {
		t.Fatal(err)
	}

	expectResponse(t, handler, req, "http peek", "2")

	req, err = http.NewRequest("POST", "/vend/bar", strings.NewReader("40"))
	if err != nil {
		t.Fatal(err)
	}

	expectResponse(t, handler, req, "http post", "")

	req, err = http.NewRequest("HEAD", "/vend/bar", nil)
	if err != nil {
		t.Fatal(err)
	}

	expectResponse(t, handler, req, "http vend", "40")
}

type mapHandler map[string]string

func (h mapHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "%s", h[r.URL.Path])
}

func TestFallback(t *testing.T) {
	serv := httptest.NewServer(mapHandler(map[string]string{
		"/logs/foo/latest-build.txt": "200",
		"/logs/bar/latest-build.txt": "\t300 \n",
		"/logs/baz/latest-build.txt": "asdf",
	}))
	defer serv.Close()

	store := makeStore(t)
	defer os.Remove(store.storagePath)
	store.fallbackFunc = fallbackHandler{template: serv.URL + "/logs/%s/latest-build.txt"}.get

	expectEqual(t, "vend foo 1", store.vend("foo"), 201)
	expectEqual(t, "vend foo 2", store.vend("foo"), 202)

	expectEqual(t, "vend bar", store.vend("bar"), 301)
	expectEqual(t, "vend baz", store.vend("baz"), 1)
	expectEqual(t, "vend quux", store.vend("quux"), 1)
}

func TestGetURL(t *testing.T) {
	configs := map[string]string{
		"config.yaml": "",
		"job-config.yaml": `
periodics:
- name: periodics-foo
  interval: 1m
  spec:
    containers:
    - image: localhost:5001/alpine
presubmits:
  some-org:
  - name: presubmit-foo
    spec:
      containers:
      - image: localhost:5001/alpine
postsubmits:
  some-org:
  - name: postsubmit-foo
    spec:
      containers:
      - image: localhost:5001/alpine
`,
	}
	tmpDir := t.TempDir()
	for fp, content := range configs {
		if err := os.WriteFile(path.Join(tmpDir, fp), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	c, err := config.Load(path.Join(tmpDir, "config.yaml"), path.Join(tmpDir, "job-config.yaml"), nil, "")
	if err != nil {
		t.Fatalf("Could not load config: %v", err)
	}
	tests := []struct {
		name string

		jobName  string
		template string
		c        *config.Config
		bucket   string

		expected string
	}{
		{
			name: "fallback template",

			jobName:  "pull-community-verify",
			template: "https://storage.googleapis.com/kubernetes-jenkins/logs/%s/latest-build.txt",

			expected: "https://storage.googleapis.com/kubernetes-jenkins/logs/pull-community-verify/latest-build.txt",
		},
		{
			name: "fallback bucket - presubmit",

			jobName: "presubmit-foo",
			c:       c,
			bucket:  "https://storage.googleapis.com/kubernetes-jenkins",

			expected: "https://storage.googleapis.com/kubernetes-jenkins/pr-logs/directory/presubmit-foo/latest-build.txt",
		},
		{
			name: "fallback bucket - postsubmit",

			jobName: "postsubmit-foo",
			c:       c,
			bucket:  "https://storage.googleapis.com/kubernetes-jenkins",

			expected: "https://storage.googleapis.com/kubernetes-jenkins/logs/postsubmit-foo/latest-build.txt",
		},
		{
			name: "fallback bucket - periodic",

			jobName: "periodics-foo",
			c:       c,
			bucket:  "https://storage.googleapis.com/kubernetes-jenkins",

			expected: "https://storage.googleapis.com/kubernetes-jenkins/logs/periodics-foo/latest-build.txt",
		},
		{
			name: "fallback bucket - unknown",

			jobName: "a-name-that-is-what-it-is",
			c:       c,
			bucket:  "https://storage.googleapis.com/kubernetes-jenkins",

			expected: "",
		},
		{
			name: "fallback bucket with trailing slash",

			jobName: "presubmit-foo",
			c:       c,
			bucket:  "https://storage.googleapis.com/kubernetes-jenkins/",

			expected: "https://storage.googleapis.com/kubernetes-jenkins/pr-logs/directory/presubmit-foo/latest-build.txt",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("running scenario %q", tc.name)

			var configAgent *config.Agent
			if tc.c != nil {
				configAgent = new(config.Agent)
				configAgent.Set(tc.c)
			}
			f := fallbackHandler{
				template:    tc.template,
				configAgent: configAgent,
				bucket:      tc.bucket,
			}

			if got := f.getURL(tc.jobName); got != tc.expected {
				t.Errorf("unexpected URL:\n%s\nexpected:\n%s", got, tc.expected)
			}
		})
	}
}
