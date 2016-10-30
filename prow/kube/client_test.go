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

package kube

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func getClient(url string) *Client {
	return &Client{
		baseURL:   url,
		client:    &http.Client{},
		token:     "abcd",
		namespace: "ns",
	}
}

func TestListPods(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/namespaces/ns/pods" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"items": [{}, {}]}`)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	ps, err := c.ListPods(nil)
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
	if len(ps) != 2 {
		t.Error("Expected two pods.")
	}
}

func TestDeletePod(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/namespaces/ns/pods/po" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	err := c.DeletePod("po")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestGetJob(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/apis/batch/v1/namespaces/ns/jobs/jo" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"metadata": {"name": "abcd"}}`)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	jo, err := c.GetJob("jo")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
	if jo.Metadata.Name != "abcd" {
		t.Errorf("Wrong name: %s", jo.Metadata.Name)
	}
}

func TestListJobs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/apis/batch/v1/namespaces/ns/jobs" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"items": [{}, {}]}`)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	js, err := c.ListJobs(nil)
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
	if len(js) != 2 {
		t.Error("Expected two jobs.")
	}
}

func TestGetPod(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/namespaces/ns/pods/po" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"metadata": {"name": "abcd"}}`)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	po, err := c.GetPod("po")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
	if po.Metadata.Name != "abcd" {
		t.Errorf("Wrong name: %s", po.Metadata.Name)
	}
}

func TestCreatePod(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/namespaces/ns/pods" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"metadata": {"name": "abcd"}}`)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	po, err := c.CreatePod(Pod{})
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
	if po.Metadata.Name != "abcd" {
		t.Errorf("Wrong name: %s", po.Metadata.Name)
	}
}

func TestCreateJob(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/apis/batch/v1/namespaces/ns/jobs" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"metadata": {"name": "abcd"}}`)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	jo, err := c.CreateJob(Job{})
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
	if jo.Metadata.Name != "abcd" {
		t.Errorf("Wrong name: %s", jo.Metadata.Name)
	}
}

func TestDeleteJob(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/apis/batch/v1/namespaces/ns/jobs/jo" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	err := c.DeleteJob("jo")
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestPatchJob(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/apis/batch/v1/namespaces/ns/jobs/jo" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/strategic-merge-patch+json" {
			t.Errorf("Bad Content-Type: %s", r.Header.Get("Content-Type"))
		}
		fmt.Fprint(w, `{"metadata": {"name": "abcd"}}`)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	_, err := c.PatchJob("jo", Job{})
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}

func TestPatchJobStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("Bad method: %s", r.Method)
		}
		if r.URL.Path != "/apis/batch/v1/namespaces/ns/jobs/jo/status" {
			t.Errorf("Bad request path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/strategic-merge-patch+json" {
			t.Errorf("Bad Content-Type: %s", r.Header.Get("Content-Type"))
		}
		fmt.Fprint(w, `{"metadata": {"name": "abcd"}}`)
	}))
	defer ts.Close()
	c := getClient(ts.URL)
	_, err := c.PatchJobStatus("jo", Job{})
	if err != nil {
		t.Errorf("Didn't expect error: %v", err)
	}
}
