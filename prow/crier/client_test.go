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

package crier

import (
	"net/http/httptest"
	"testing"

	"k8s.io/test-infra/prow/github"
)

type fakeGitHub struct {
	ics    []github.IssueComment
	status github.Status
	lastIC int
}

func (f *fakeGitHub) CreateStatus(org, repo, ref string, s github.Status) error {
	f.status = s
	return nil
}

func (f *fakeGitHub) ListIssueComments(org, repo string, number int) ([]github.IssueComment, error) {
	return f.ics, nil
}

func (f *fakeGitHub) CreateComment(org, repo string, number int, comment string) error {
	f.lastIC++
	f.ics = append(f.ics, github.IssueComment{
		ID:   f.lastIC,
		Body: comment,
		User: github.User{Login: "k8s-ci-robot"},
	})
	return nil
}

func (f *fakeGitHub) DeleteComment(org, repo string, ID int) error {
	var nics []github.IssueComment
	for _, ic := range f.ics {
		if ic.ID != ID {
			nics = append(nics, ic)
		}
	}
	f.ics = nics
	return nil
}

// Test that the server and client work nicely together.
func TestCrier(t *testing.T) {
	fghc := &fakeGitHub{}
	crierServer := NewServer(fghc)
	crierServer.notify = make(chan struct{})
	s := httptest.NewServer(crierServer)
	crierServer.Run()
	defer s.Close()

	if err := ReportToCrier(s.URL, Report{
		Context: "bla",
		State:   github.StatusPending,
	}); err != nil {
		t.Fatalf("Error reporting to crier: %v", err)
	}
	<-crierServer.notify
	if fghc.status.Context != "bla" {
		t.Errorf("Incorrect context for status: got %s want bla", fghc.status.Context)
	}

	fghc.CreateComment("", "", 0, "foo test **failed** for a thingamabob")
	ReportToCrier(s.URL, Report{
		Context: "foo test",
		State:   github.StatusSuccess,
	})
	<-crierServer.notify
	if len(fghc.ics) != 0 {
		t.Errorf("There shouldn't be any comments here: %v", fghc.ics)
	}

	ReportToCrier(s.URL, Report{
		Context: "bar test",
		State:   github.StatusFailure,
	})
	<-crierServer.notify
	if len(fghc.ics) != 1 {
		t.Errorf("There should be one comment here: %v", fghc.ics)
	}

	ReportToCrier(s.URL, Report{
		Context: "bar test",
		State:   github.StatusFailure,
	})
	<-crierServer.notify
	if len(fghc.ics) != 1 {
		t.Errorf("There should be one comment here: %v", fghc.ics)
	}
}
