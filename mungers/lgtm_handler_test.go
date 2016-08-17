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

package mungers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"testing"

	github_util "k8s.io/contrib/mungegithub/github"
	github_test "k8s.io/contrib/mungegithub/github/testing"
	"k8s.io/contrib/mungegithub/mungers/mungerutil"
	"k8s.io/kubernetes/pkg/util/sets"

	"github.com/google/go-github/github"
)

var (
	prWithLGTM    = github_test.Issue(botName, 1, []string{lgtmLabel}, true)
	prWithoutLGTM = github_test.Issue(botName, 1, []string{}, true)
)

func TestAddLGTMIfCommented(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	tests := []struct {
		name        string
		comments    []*github.IssueComment
		issue       *github.Issue
		assignees   mungerutil.UserSet
		mustHave    []string
		mustNotHave []string
	}{
		{
			name:  "Other comments should not add LGTM.",
			issue: prWithoutLGTM,
			comments: []*github.IssueComment{
				github_test.IssueComment(1, "/comment 1", "user 1", 0),
				github_test.IssueComment(2, "/comment 2 //comment3", "user 2", 1),
			},
			assignees:   mungerutil.UserSet(sets.NewString("user 1")),
			mustHave:    []string{},
			mustNotHave: []string{lgtmLabel},
		},
		{
			name:  "/lgtm by non-assignee should not add LGTM label",
			issue: prWithoutLGTM,
			comments: []*github.IssueComment{
				github_test.IssueComment(1, "/lgtm", "user 1", 0),
				github_test.IssueComment(2, "comment 2", "user 2", 1),
			},
			assignees:   mungerutil.UserSet(sets.NewString("user 2")),
			mustHave:    []string{},
			mustNotHave: []string{lgtmLabel},
		},
		{
			name:  "/lgtm by assignee should add LGTM label",
			issue: prWithoutLGTM,
			comments: []*github.IssueComment{
				github_test.IssueComment(1, "/lgtm", "user 1", 0),
				github_test.IssueComment(2, "comment 2", "user 2", 1),
			},
			assignees:   mungerutil.UserSet(sets.NewString("user 1")),
			mustHave:    []string{lgtmLabel},
			mustNotHave: []string{},
		},
		{
			name:  "/lgtm by assignee followed by cancellation by non-assignee should add lgtm",
			issue: prWithoutLGTM,
			comments: []*github.IssueComment{
				github_test.IssueComment(1, "/lgtm", "user 1", 0),
				github_test.IssueComment(2, "/lgtm cancel", "user 2", 1),
			},
			assignees:   mungerutil.UserSet(sets.NewString("user 1")),
			mustHave:    []string{lgtmLabel},
			mustNotHave: []string{},
		},
		{
			name:  "/lgtm by assignee followed by /lgtm cancel should not add lgtm",
			issue: prWithoutLGTM,
			comments: []*github.IssueComment{
				github_test.IssueComment(1, "/lgtm", "user 1", 0),
				github_test.IssueComment(2, "/lgtm cancel", "user 2", 1),
			},
			assignees:   mungerutil.UserSet(sets.NewString("user 1", "user 2")),
			mustHave:    []string{},
			mustNotHave: []string{lgtmLabel},
		},
		{
			name:  "/lgtm followed by comment should be honored",
			issue: prWithoutLGTM,
			comments: []*github.IssueComment{
				github_test.IssueComment(1, "/lgtm //this is a comment", "user 1", 0),
			},
			assignees:   mungerutil.UserSet(sets.NewString("user 1")),
			mustHave:    []string{lgtmLabel},
			mustNotHave: []string{},
		},
		{
			name:  "/lgtm cancel by bot should be honored",
			issue: prWithoutLGTM,
			comments: []*github.IssueComment{
				github_test.IssueComment(1, "/lgtm //this is a comment", "user 1", 0),
				github_test.IssueComment(1, "/lgtm cancel //this is a bot", botName, 0),
			},
			assignees:   mungerutil.UserSet(sets.NewString("user 1")),
			mustHave:    []string{},
			mustNotHave: []string{lgtmLabel},
		},
	}

	for testNum, test := range tests {
		pr := ValidPR()
		client, server, mux := github_test.InitServer(t, test.issue, pr, nil, nil, nil, nil, nil)
		path := fmt.Sprintf("/repos/o/r/issue/%s/labels", *test.issue.Number)
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			out := []github.Label{{}}
			data, err := json.Marshal(out)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			w.Write(data)
		})

		config := &github_util.Config{}
		config.Org = "o"
		config.Project = "r"
		config.SetClient(client)

		l := LGTMHandler{}
		obj, err := config.GetObject(*test.issue.Number)
		if err != nil {
			t.Fatalf("%v", err)
		}

		l.addLGTMIfCommented(obj, test.comments, test.assignees)
		for _, lab := range test.mustHave {
			if !obj.HasLabel(lab) {
				t.Errorf("%s:%d: Did not find label %q, labels: %v", test.name, testNum, lab, obj.Issue.Labels)
			}
		}
		for _, lab := range test.mustNotHave {
			if obj.HasLabel(lab) {
				t.Errorf("%s:%d: Found label %q and should not have, labels: %v", test.name, testNum, lab, obj.Issue.Labels)
			}
		}
		server.Close()
	}
}

func TestRemoveLGTMIfCommented(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	tests := []struct {
		name        string
		comments    []*github.IssueComment
		issue       *github.Issue
		assignees   mungerutil.UserSet
		mustHave    []string
		mustNotHave []string
	}{
		{
			name:  "LGTM with no comments should be unaffected by other comments.",
			issue: prWithLGTM,
			comments: []*github.IssueComment{
				github_test.IssueComment(1, "/comment 1", "user 1", 0),
				github_test.IssueComment(2, "/comment 2 //comment 3", "user 2", 1),
			},
			assignees:   mungerutil.UserSet(sets.NewString("user 1")),
			mustHave:    []string{lgtmLabel},
			mustNotHave: []string{},
		},
		{
			name:  "/lgtm cancel followed by comment should be honored.",
			issue: prWithLGTM,
			comments: []*github.IssueComment{
				github_test.IssueComment(1, "/lgtm cancel // something // something else", "user 1", 0),
			},
			assignees:   mungerutil.UserSet(sets.NewString("user 1")),
			mustHave:    []string{},
			mustNotHave: []string{lgtmLabel},
		},
		{
			name:  "/lgtm cancel by non-assignee should be ignored.",
			issue: prWithLGTM,
			comments: []*github.IssueComment{
				github_test.IssueComment(1, "/lgtm cancel // something // something else", "user 1", 0),
			},
			assignees:   mungerutil.UserSet(sets.NewString("user 2")),
			mustHave:    []string{lgtmLabel},
			mustNotHave: []string{},
		},
		{
			name:  "/lgtm should be honored if after /lgtm cancel.",
			issue: prWithLGTM,
			comments: []*github.IssueComment{
				github_test.IssueComment(1, "/lgtm cancel // something // something else", "user 1", 0),
				github_test.IssueComment(1, "/lgtm", "user 1", 1),
			},
			assignees:   mungerutil.UserSet(sets.NewString("user 1")),
			mustHave:    []string{lgtmLabel},
			mustNotHave: []string{},
		},
		{
			name:  "/lgtm cancel by bot should be honored",
			issue: prWithoutLGTM,
			comments: []*github.IssueComment{
				github_test.IssueComment(1, "/lgtm //this is a comment", "user 1", 0),
				github_test.IssueComment(1, "/lgtm cancel //this is a bot", botName, 0),
			},
			assignees:   mungerutil.UserSet(sets.NewString("user 1")),
			mustHave:    []string{},
			mustNotHave: []string{lgtmLabel},
		},
	}

	for testNum, test := range tests {
		pr := ValidPR()
		client, server, mux := github_test.InitServer(t, test.issue, pr, nil, nil, nil, nil, nil)
		path := fmt.Sprintf("/repos/o/r/issue/%s/labels", *test.issue.Number)
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			out := []github.Label{{}}
			data, err := json.Marshal(out)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			w.Write(data)
		})

		config := &github_util.Config{}
		config.Org = "o"
		config.Project = "r"
		config.SetClient(client)

		l := LGTMHandler{}
		obj, err := config.GetObject(*test.issue.Number)
		if err != nil {
			t.Fatalf("%v", err)
		}

		l.removeLGTMIfCancelled(obj, test.comments, test.assignees)
		for _, lab := range test.mustHave {
			if !obj.HasLabel(lab) {
				t.Errorf("%s:%d: Did not find label %q, labels: %v", test.name, testNum, lab, obj.Issue.Labels)
			}
		}
		for _, lab := range test.mustNotHave {
			if obj.HasLabel(lab) {
				t.Errorf("%s:%d: Found label %q and should not have, labels: %v", test.name, testNum, lab, obj.Issue.Labels)
			}
		}
		server.Close()
	}
}
