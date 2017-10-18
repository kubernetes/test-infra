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

package mungers

import (
	"fmt"
	"net/http"
	"reflect"
	"runtime"
	"testing"

	githubapi "github.com/google/go-github/github"
	"k8s.io/test-infra/mungegithub/github"
	github_test "k8s.io/test-infra/mungegithub/github/testing"
)

const (
	helpWanted          = "help-wanted"
	open                = "open"
	sigApps             = "sig/apps"
	committeeSteering   = "committee/steering"
	wgContainerIdentity = "wg/container-identity"
	username            = "Ali"
)

func TestSigMentionHandler(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	tests := []struct {
		name     string
		issue    *githubapi.Issue
		expected []githubapi.Label
	}{
		{
			name: "ignore PRs",
			issue: &githubapi.Issue{
				State:            githubapi.String(open),
				Labels:           []githubapi.Label{{Name: githubapi.String(helpWanted)}},
				PullRequestLinks: &githubapi.PullRequestLinks{},
				Assignee:         &githubapi.User{Login: githubapi.String(username)},
				Number:           intPtr(1),
			},
			expected: []githubapi.Label{{Name: githubapi.String(helpWanted)}},
		},
		{
			name:     "issue is null",
			issue:    nil,
			expected: nil,
		},
		{
			name: "issue state is null",
			issue: &githubapi.Issue{
				State:            nil,
				Labels:           []githubapi.Label{{Name: githubapi.String(helpWanted)}},
				PullRequestLinks: nil,
				Assignee:         &githubapi.User{Login: githubapi.String(username)},
				Number:           intPtr(1),
			},
			expected: []githubapi.Label{{Name: githubapi.String(helpWanted)}},
		},
		{
			name: "issue is closed",
			issue: &githubapi.Issue{
				State:            githubapi.String("closed"),
				Labels:           []githubapi.Label{{Name: githubapi.String(helpWanted)}},
				PullRequestLinks: nil,
				Assignee:         &githubapi.User{Login: githubapi.String(username)},
				Number:           intPtr(1),
			},
			expected: []githubapi.Label{{Name: githubapi.String(helpWanted)}},
		},
		{
			name: "issue has sig/foo label, no needs-sig label",
			issue: &githubapi.Issue{
				State: githubapi.String(open),
				Labels: []githubapi.Label{{Name: githubapi.String(helpWanted)},
					{Name: githubapi.String(sigApps)}},
				PullRequestLinks: nil,
				Assignee:         &githubapi.User{Login: githubapi.String(username)},
				Number:           intPtr(1),
			},
			expected: []githubapi.Label{{Name: githubapi.String(helpWanted)},
				{Name: githubapi.String(sigApps)}},
		},
		{
			name: "issue has no sig/foo label, no needs-sig label",
			issue: &githubapi.Issue{
				State:            githubapi.String(open),
				Labels:           []githubapi.Label{{Name: githubapi.String(helpWanted)}},
				PullRequestLinks: nil,
				Assignee:         &githubapi.User{Login: githubapi.String(username)},
				Number:           intPtr(1),
			},
			expected: []githubapi.Label{{Name: githubapi.String(helpWanted)},
				{Name: githubapi.String(needsSigLabel)}},
		},
		{
			name: "issue has needs-sig label, no sig/foo label",
			issue: &githubapi.Issue{
				State: githubapi.String(open),
				Labels: []githubapi.Label{{Name: githubapi.String(helpWanted)},
					{Name: githubapi.String(needsSigLabel)}},
				PullRequestLinks: nil,
				Assignee:         &githubapi.User{Login: githubapi.String(username)},
				Number:           intPtr(1),
			},
			expected: []githubapi.Label{{Name: githubapi.String(helpWanted)},
				{Name: githubapi.String(needsSigLabel)}},
		},
		{
			name: "issue has both needs-sig label and sig/foo label",
			issue: &githubapi.Issue{
				State: githubapi.String(open),
				Labels: []githubapi.Label{{Name: githubapi.String(helpWanted)},
					{Name: githubapi.String(needsSigLabel)},
					{Name: githubapi.String(sigApps)}},
				PullRequestLinks: nil,
				Assignee:         &githubapi.User{Login: githubapi.String(username)},
				Number:           intPtr(1),
			},
			expected: []githubapi.Label{{Name: githubapi.String(helpWanted)},
				{Name: githubapi.String(sigApps)}},
		},
		{
			name: "issue has committee/foo label, no needs-sig label",
			issue: &githubapi.Issue{
				State: githubapi.String(open),
				Labels: []githubapi.Label{{Name: githubapi.String(helpWanted)},
					{Name: githubapi.String(committeeSteering)}},
				PullRequestLinks: nil,
				Assignee:         &githubapi.User{Login: githubapi.String(username)},
				Number:           intPtr(1),
			},
			expected: []githubapi.Label{{Name: githubapi.String(helpWanted)},
				{Name: githubapi.String(committeeSteering)}},
		},
		{
			name: "issue has both needs-sig label and committee/foo label",
			issue: &githubapi.Issue{
				State: githubapi.String(open),
				Labels: []githubapi.Label{{Name: githubapi.String(helpWanted)},
					{Name: githubapi.String(needsSigLabel)},
					{Name: githubapi.String(committeeSteering)}},
				PullRequestLinks: nil,
				Assignee:         &githubapi.User{Login: githubapi.String(username)},
				Number:           intPtr(1),
			},
			expected: []githubapi.Label{{Name: githubapi.String(helpWanted)},
				{Name: githubapi.String(committeeSteering)}},
		},
		{
			name: "issue has wg/foo label, no needs-sig label",
			issue: &githubapi.Issue{
				State: githubapi.String(open),
				Labels: []githubapi.Label{{Name: githubapi.String(helpWanted)},
					{Name: githubapi.String(wgContainerIdentity)}},
				PullRequestLinks: nil,
				Assignee:         &githubapi.User{Login: githubapi.String(username)},
				Number:           intPtr(1),
			},
			expected: []githubapi.Label{{Name: githubapi.String(helpWanted)},
				{Name: githubapi.String(wgContainerIdentity)}},
		},
		{
			name: "issue has both needs-sig label and wg/foo label",
			issue: &githubapi.Issue{
				State: githubapi.String(open),
				Labels: []githubapi.Label{{Name: githubapi.String(helpWanted)},
					{Name: githubapi.String(needsSigLabel)},
					{Name: githubapi.String(wgContainerIdentity)}},
				PullRequestLinks: nil,
				Assignee:         &githubapi.User{Login: githubapi.String(username)},
				Number:           intPtr(1),
			},
			expected: []githubapi.Label{{Name: githubapi.String(helpWanted)},
				{Name: githubapi.String(wgContainerIdentity)}},
		},
	}

	for testNum, test := range tests {
		client, server, mux := github_test.InitServer(t, test.issue, nil, nil, nil, nil, nil, nil)
		config := &github.Config{
			Org:     "o",
			Project: "r",
		}
		config.SetClient(client)

		mux.HandleFunc(fmt.Sprintf("/repos/o/r/issues/1/labels/%s", needsSigLabel), func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		obj := &github.MungeObject{}
		var err error

		if test.issue != nil {
			obj, err = config.GetObject(*test.issue.Number)

			if err != nil {
				t.Fatalf("%d: unable to get issue: %v", testNum, *test.issue.Number)
			}

			obj.Issue = test.issue
		}

		s := SigMentionHandler{}
		s.Munge(obj)

		if obj.Issue == nil {
			if test.expected != nil {
				t.Errorf("%d:%s: expected: %v, saw: %v", testNum, test.name, test.expected, nil)
			} else {
				continue
			}
		}

		if !reflect.DeepEqual(test.expected, obj.Issue.Labels) {
			t.Errorf("%d:%s: expected: %v, saw: %v", testNum, test.name, test.expected, obj.Issue.Labels)
		}

		server.Close()
	}
}
