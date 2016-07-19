/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kubernetes/test-infra/ciongke/github"
	"github.com/kubernetes/test-infra/ciongke/github/fakegithub"
	"github.com/kubernetes/test-infra/ciongke/kube"
	"github.com/kubernetes/test-infra/ciongke/kube/fakekube"
)

func TestServeHTTPErrors(t *testing.T) {
	s := &Server{
		Events:     make(chan Event),
		HMACSecret: []byte("abc"),
	}
	// This is the SHA1 signature for payload "{}" and signature "abc"
	// echo -n '{}' | openssl dgst -sha1 -hmac abc
	const hmac string = "sha1=db5c76f4264d0ad96cf21baec394964b4b8ce580"
	const body string = "{}"
	var testcases = []struct {
		Method string
		Header map[string]string
		Body   string
		Code   int
		Type   string
	}{
		{
			// GET
			Method: http.MethodGet,
			Header: map[string]string{
				"X-GitHub-Event":  "ping",
				"X-Hub-Signature": hmac,
			},
			Body: body,
			Code: http.StatusMethodNotAllowed,
			Type: "ping",
		},
		{
			// No event
			Method: http.MethodPost,
			Header: map[string]string{
				"X-Hub-Signature": hmac,
			},
			Body: body,
			Code: http.StatusBadRequest,
			Type: "ping",
		},
		{
			// No signature
			Method: http.MethodPost,
			Header: map[string]string{
				"X-GitHub-Event": "ping",
			},
			Body: body,
			Code: http.StatusForbidden,
			Type: "ping",
		},
		{
			// Bad signature
			Method: http.MethodPost,
			Header: map[string]string{
				"X-GitHub-Event":  "ping",
				"X-Hub-Signature": "this doesn't work",
			},
			Body: body,
			Code: http.StatusForbidden,
			Type: "ping",
		},
		{
			// Good
			Method: http.MethodPost,
			Header: map[string]string{
				"X-GitHub-Event":  "ping",
				"X-Hub-Signature": hmac,
			},
			Body: body,
			Code: http.StatusOK,
			Type: "ping",
		},
	}

	for _, tc := range testcases {
		w := httptest.NewRecorder()
		r, err := http.NewRequest(tc.Method, "", strings.NewReader(tc.Body))
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range tc.Header {
			r.Header.Set(k, v)
		}
		s.ServeHTTP(w, r)
		if w.Code != tc.Code {
			t.Errorf("For test case: %+v\nExpected code %v, got code %v", tc, tc.Code, w.Code)
		} else if tc.Code == http.StatusOK {
			e := <-s.Events
			if e.Type != tc.Type {
				t.Errorf("For test case: %+v\nExpected type %v, got type %v", tc, tc.Type, e.Type)
			}
			if string(e.Payload) != tc.Body {
				t.Errorf("For test case: %+v\nExpected payload %v, got payload %v", tc, tc.Body, string(e.Payload))
			}
		}
	}
}

func TestDeletePR(t *testing.T) {
	c := &fakekube.FakeClient{
		Jobs: []kube.Job{
			{
				// Delete this one.
				Metadata: kube.ObjectMeta{
					Name: "r-pr-3-abcd",
					Labels: map[string]string{
						"pr":   "3",
						"repo": "r",
					},
				},
			},
			{
				// Different PR.
				Metadata: kube.ObjectMeta{
					Name: "r-pr-4-qwer",
					Labels: map[string]string{
						"pr":   "4",
						"repo": "r",
					},
				},
			},
			{
				// Different repo.
				Metadata: kube.ObjectMeta{
					Name: "q-pr-3-wxyz",
					Labels: map[string]string{
						"pr":   "3",
						"repo": "q",
					},
				},
			},
		},
		Pods: []kube.Pod{
			{
				// Delete this one.
				Metadata: kube.ObjectMeta{
					Name: "r-pr-3-abcd-test",
					Labels: map[string]string{
						"job-name": "r-pr-3-abcd",
					},
				},
			},
			{
				// Different job.
				Metadata: kube.ObjectMeta{
					Name: "r-pr-4-qwer-test",
					Labels: map[string]string{
						"job-name": "r-pr-4-qwer",
					},
				},
			},
		},
	}
	s := &Server{
		KubeClient: c,
	}
	s.deletePR("r", 3)
	if len(c.DeletedJobs) == 0 {
		t.Error("Job for PR 3 not deleted.")
	} else if len(c.DeletedJobs) > 1 {
		t.Error("Too many jobs deleted.")
	}
	if len(c.DeletedPods) == 0 {
		t.Error("Pod for PR 3 not deleted.")
	} else if len(c.DeletedPods) > 1 {
		t.Error("Too many pods deleted.")
	}
}

func TestUntrusted(t *testing.T) {
	c := &fakekube.FakeClient{}
	g := &fakegithub.FakeClient{}
	s := &Server{
		KubeClient:   c,
		GitHubClient: g,
	}
	s.handlePullRequestEvent(github.PullRequestEvent{
		Action: "opened",
		Number: 4,
		PullRequest: github.PullRequest{
			Number: 4,
			User: github.User{
				Login: "untrustworthy",
			},
		},
	})
	if len(c.Jobs) > 0 || len(c.Pods) > 0 {
		t.Error("Should not have created a job for an untrustworthy person.")
	}
}

func TestBuildPR(t *testing.T) {
	c := &fakekube.FakeClient{}
	s := &Server{
		KubeClient: c,
	}
	s.buildPR(github.PullRequest{
		Number: 4,
		User:   github.User{"login"},
		Base: github.PullRequestBranch{
			Ref: "master",
			SHA: "1234567890",
			Repo: github.Repo{
				Owner:   github.User{"repo-owner"},
				Name:    "repo-name",
				HTMLURL: "https://github.com/repo-owner/repo-name",
			},
		},
		Head: github.PullRequestBranch{
			Ref: "pr-branch",
			SHA: "abcdefghij",
			Repo: github.Repo{
				Owner:   github.User{"pr-owner"},
				Name:    "pr-repo-name",
				HTMLURL: "https://github.com/pr-owner/pr-repo-name",
			},
		},
	})
	if len(c.Jobs) != 1 {
		t.Fatal("No job was created.")
	}
	j := c.Jobs[0]
	if j.Metadata.Name != "repo-name-pr-4-abcdefgh" {
		t.Errorf("Job name %s isn't expected repo-name-pr-4-abcdefgh.")
	}
	if j.Metadata.Labels["repo"] != "repo-name" {
		t.Errorf("Label \"repo\" %s isn't expected repo-name.", j.Metadata.Labels["repo"])
	}
	if j.Metadata.Labels["pr"] != "4" {
		t.Errorf("Label \"pr\" %s isn't expected 4.", j.Metadata.Labels["repo"])
	}
}
