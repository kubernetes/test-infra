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

	"github.com/golang/glog"
	"github.com/google/go-github/github"
)

var (
	_ = fmt.Printf
	_ = glog.Errorf
)

func TestReleaseNoteLabel(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	tests := []struct {
		name        string
		issue       *github.Issue
		body        string
		branch      string
		secondIssue *github.Issue
		mustHave    []string
		mustNotHave []string
	}{
		{
			name:        "LGTM with release-note",
			issue:       github_test.Issue(botName, 1, []string{"lgtm", releaseNote}, true),
			mustHave:    []string{"lgtm", releaseNote},
			mustNotHave: []string{releaseNoteLabelNeeded},
		},
		{
			name:        "LGTM with release-note-none",
			issue:       github_test.Issue(botName, 1, []string{"lgtm", releaseNoteNone}, true),
			mustHave:    []string{"lgtm", releaseNoteNone},
			mustNotHave: []string{releaseNoteLabelNeeded},
		},
		{
			name:        "LGTM with release-note-action-required",
			issue:       github_test.Issue(botName, 1, []string{"lgtm", releaseNoteActionRequired}, true),
			mustHave:    []string{"lgtm", releaseNoteActionRequired},
			mustNotHave: []string{releaseNoteLabelNeeded},
		},
		{
			name:        "LGTM with release-note-experimental",
			issue:       github_test.Issue(botName, 1, []string{"lgtm", releaseNoteExperimental}, true),
			mustHave:    []string{"lgtm", releaseNoteExperimental},
			mustNotHave: []string{releaseNoteLabelNeeded},
		},
		{
			name:        "LGTM with release-note-label-needed",
			issue:       github_test.Issue(botName, 1, []string{"lgtm", releaseNoteLabelNeeded}, true),
			mustHave:    []string{releaseNoteLabelNeeded},
			mustNotHave: []string{"lgtm"},
		},
		{
			name:        "LGTM only",
			issue:       github_test.Issue(botName, 1, []string{"lgtm"}, true),
			mustHave:    []string{releaseNoteLabelNeeded},
			mustNotHave: []string{"lgtm"},
		},
		{
			name:     "No labels",
			issue:    github_test.Issue(botName, 1, []string{}, true),
			mustHave: []string{releaseNoteLabelNeeded},
		},
		{
			name:     "release-note",
			issue:    github_test.Issue(botName, 1, []string{releaseNote}, true),
			mustHave: []string{releaseNote},
		},
		{
			name:     "release-note-none",
			issue:    github_test.Issue(botName, 1, []string{releaseNoteNone}, true),
			mustHave: []string{releaseNoteNone},
		},
		{
			name:     "release-note-action-required",
			issue:    github_test.Issue(botName, 1, []string{releaseNoteActionRequired}, true),
			mustHave: []string{releaseNoteActionRequired},
		},
		{
			name:     "release-note-experimental",
			issue:    github_test.Issue(botName, 1, []string{releaseNoteExperimental}, true),
			mustHave: []string{releaseNoteExperimental},
		},
		{
			name:        "release-note and release-note-label-needed",
			issue:       github_test.Issue(botName, 1, []string{releaseNote, releaseNoteLabelNeeded}, true),
			mustHave:    []string{releaseNote},
			mustNotHave: []string{releaseNoteLabelNeeded},
		},
		{
			name:        "release-note-none and release-note-label-needed",
			issue:       github_test.Issue(botName, 1, []string{releaseNoteNone, releaseNoteLabelNeeded}, true),
			mustHave:    []string{releaseNoteNone},
			mustNotHave: []string{releaseNoteLabelNeeded},
		},
		{
			name:        "release-note-action-required and release-note-label-needed",
			issue:       github_test.Issue(botName, 1, []string{releaseNoteActionRequired, releaseNoteLabelNeeded}, true),
			mustHave:    []string{releaseNoteActionRequired},
			mustNotHave: []string{releaseNoteLabelNeeded},
		},
		{
			name:        "release-note-experimental and release-note-label-needed",
			issue:       github_test.Issue(botName, 1, []string{releaseNoteExperimental, releaseNoteLabelNeeded}, true),
			mustHave:    []string{releaseNoteExperimental},
			mustNotHave: []string{releaseNoteLabelNeeded},
		},
		{
			name:        "do not add needs label when parent PR has releaseNote label",
			branch:      "release-1.2",
			issue:       github_test.Issue(botName, 1, []string{}, true),
			body:        "Cherry pick of #2 on release-1.2.",
			secondIssue: github_test.Issue(botName, 2, []string{releaseNote}, true),
			mustNotHave: []string{releaseNoteLabelNeeded},
		},
		{
			name:        "do not touch LGTM on non-master when parent PR has releaseNote label",
			branch:      "release-1.2",
			issue:       github_test.Issue(botName, 1, []string{"lgtm"}, true),
			body:        "Cherry pick of #2 on release-1.2.",
			secondIssue: github_test.Issue(botName, 2, []string{releaseNote}, true),
			mustHave:    []string{"lgtm"},
			mustNotHave: []string{releaseNoteLabelNeeded},
		},
		{
			name:        "add needs label when parent PR does not have releaseNote label",
			branch:      "release-1.2",
			issue:       github_test.Issue(botName, 1, []string{}, true),
			body:        "Cherry pick of #2 on release-1.2.",
			secondIssue: github_test.Issue(botName, 2, []string{releaseNoteNone}, true),
			mustHave:    []string{releaseNoteLabelNeeded},
		},
		{
			name:        "remove LGTM on non-master when parent PR has releaseNote label",
			branch:      "release-1.2",
			issue:       github_test.Issue(botName, 1, []string{"lgtm"}, true),
			body:        "Cherry pick of #2 on release-1.2.",
			secondIssue: github_test.Issue(botName, 2, []string{releaseNoteNone}, true),
			mustHave:    []string{releaseNoteLabelNeeded},
			mustNotHave: []string{"lgtm"},
		},
	}
	for testNum, test := range tests {
		pr := ValidPR()
		if test.branch != "" {
			pr.Base.Ref = &test.branch
		}
		test.issue.Body = &test.body
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
		if test.secondIssue != nil {
			path = fmt.Sprintf("/repos/o/r/issues/%d", *test.secondIssue.Number)
			mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
				data, err := json.Marshal(test.secondIssue)
				if err != nil {
					t.Errorf("%v", err)
				}
				if r.Method != "GET" {
					t.Errorf("Unexpected method: expected: GET got: %s", r.Method)
				}
				w.WriteHeader(http.StatusOK)
				w.Write(data)
			})
		}

		config := &github_util.Config{}
		config.Org = "o"
		config.Project = "r"
		config.SetClient(client)

		r := ReleaseNoteLabel{}
		err := r.Initialize(config, nil)
		if err != nil {
			t.Fatalf("%v", err)
		}

		err = r.EachLoop()
		if err != nil {
			t.Fatalf("%v", err)
		}

		obj, err := config.GetObject(*test.issue.Number)
		if err != nil {
			t.Fatalf("%v", err)
		}

		r.Munge(obj)

		for _, l := range test.mustHave {
			if !obj.HasLabel(l) {
				t.Errorf("%s:%d: Did not find label %q, labels: %v", test.name, testNum, l, obj.Issue.Labels)
			}
		}
		for _, l := range test.mustNotHave {
			if obj.HasLabel(l) {
				t.Errorf("%s:%d: Found label %q and should not have, labels: %v", test.name, testNum, l, obj.Issue.Labels)
			}
		}
		server.Close()
	}
}
