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

package mungers

import (
	"strings"
	"testing"

	"time"

	"github.com/google/go-github/github"
	github_testing "k8s.io/contrib/mungegithub/github/testing"
	cache "k8s.io/contrib/mungegithub/mungers/flakesync"
	"k8s.io/contrib/mungegithub/mungers/sync"
	"k8s.io/contrib/test-utils/utils"
)

func makeTestFlakeManager() *FlakeManager {
	bucketUtils := utils.NewUtils("bucket", "logs")
	return &FlakeManager{
		sq:                   nil,
		config:               nil,
		googleGCSBucketUtils: bucketUtils,
	}
}

func expect(t *testing.T, actual, expected string) {
	if actual != expected {
		t.Errorf("expected `%s` to be `%s`", actual, expected)
	}
}

func expectContains(t *testing.T, haystack, needle, desc string) {
	if !strings.Contains(haystack, needle) {
		t.Errorf("%s: `%v` not in `%v`", desc, needle, haystack)
	}
}

func checkCommon(t *testing.T, source sync.IssueSource) {
	expect(t, source.ID(), "/bucket/logs/e2e-gce/123/\n")
	expectContains(t, source.Body(false), source.ID(),
		"Body() does not contain ID()")
	expectContains(t, "https://storage.googleapis.com/"+
		"bucket/logs/e2e-gce/123/\n",
		source.ID(),
		"ID() is not compatible with older IDs")
	expectContains(t, source.Body(false),
		"https://k8s-gubernator.appspot.com/build"+source.ID(),
		"Body() does not contain gubernator link")
}

func TestIndividualFlakeSource(t *testing.T) {
	fm := makeTestFlakeManager()
	flake := cache.Flake{
		Job:    "e2e-gce",
		Number: 123,
		Test:   "[k8s.io] Latency",
		Reason: "Took too long!",
	}
	source := individualFlakeSource{flake, fm}
	expect(t, source.Title(), "[k8s.io] Latency")
	checkCommon(t, &source)
}

func TestBrokenJobSource(t *testing.T) {
	fm := makeTestFlakeManager()
	result := cache.Result{
		Job:    "e2e-gce",
		Number: 123,
	}
	source := brokenJobSource{&result, fm}
	expect(t, source.Title(), "e2e-gce: broken test run")
	checkCommon(t, &source)
}

func flakecomment(id int, createdAt time.Time) *github.IssueComment {
	return github_testing.Comment(id, "k8s-bot", createdAt, "Failed: something failed")
}

func TestAutoPrioritize(t *testing.T) {
	var p0Comments []*github.IssueComment

	//simulates 50 test flakes/comments in the last 50 hours
	for i := 0; i < 50; i++ {
		p0Comments = append(p0Comments, flakecomment(1, time.Now().Add(-time.Duration(i)*time.Hour)))
	}

	testcases := []struct {
		comments       []*github.IssueComment
		issueCreatedAt time.Time
		expectPriority int
	}{
		{
			//0 flakes in the last 7 days
			comments:       []*github.IssueComment{},
			issueCreatedAt: time.Now(),
			expectPriority: 3,
		},
		{
			//1 flakes in the last 7 days
			comments: []*github.IssueComment{
				flakecomment(1, time.Now()),
			},
			issueCreatedAt: time.Now().Add(-1 * 29 * 24 * time.Hour),
			expectPriority: 3,
		},
		{
			//3 flakes in the last 7 days
			comments: []*github.IssueComment{
				flakecomment(1, time.Now()),
				flakecomment(1, time.Now().Add(-1*3*24*time.Hour)),
				flakecomment(1, time.Now().Add(-1*6*24*time.Hour)),
			},
			issueCreatedAt: time.Now().Add(-1 * 30 * 24 * time.Hour),
			expectPriority: 2,
		},
		{
			//10 flakes in the last 10 hrs
			comments: []*github.IssueComment{
				flakecomment(1, time.Now()),
				flakecomment(1, time.Now().Add(-1*time.Hour)),
				flakecomment(1, time.Now().Add(-2*time.Hour)),
				flakecomment(1, time.Now().Add(-3*time.Hour)),
				flakecomment(1, time.Now().Add(-4*time.Hour)),
				flakecomment(1, time.Now().Add(-5*time.Hour)),
				flakecomment(1, time.Now().Add(-6*time.Hour)),
				flakecomment(1, time.Now().Add(-7*time.Hour)),
				flakecomment(1, time.Now().Add(-8*time.Hour)),
				flakecomment(1, time.Now().Add(-9*time.Hour)),
			},
			issueCreatedAt: time.Now().Add(-1 * 29 * 24 * time.Hour),
			expectPriority: 1,
		},
		{
			//4 flakes, but only 2 in a week
			comments: []*github.IssueComment{
				flakecomment(1, time.Now()),
				flakecomment(1, time.Now().Add(-3*24*time.Hour)),
				flakecomment(1, time.Now().Add(-15*24*time.Hour)),
				flakecomment(1, time.Now().Add(-20*24*time.Hour)),
			},
			issueCreatedAt: time.Now().Add(-1 * 29 * 24 * time.Hour),
			expectPriority: 3,
		},
		{
			//50 flakes in a week
			comments:       p0Comments,
			issueCreatedAt: time.Now().Add(-1 * 6 * 24 * time.Hour),
			expectPriority: 0,
		},
	}
	for _, tc := range testcases {
		p := autoPrioritize(tc.comments, &tc.issueCreatedAt)
		if p.Priority() != tc.expectPriority {
			t.Errorf("Expected priority: %d, But got: %d",
				len(tc.comments), tc.expectPriority, p.Priority())
		}
	}
}

func TestPullRE(t *testing.T) {
	table := []struct {
		path   string
		expect string
	}{
		{"/kubernetes-jenkins/pr-logs/pull/27898/kubernetes-pull-build-test-e2e-gce/47123/", "27898"},
		{"kubernetes-jenkins/logs/kubernetes-e2e-gke-test/13095/", ""},
	}
	for _, tt := range table {
		got := ""
		if parts := pullRE.FindStringSubmatch(tt.path); len(parts) > 1 {
			got = parts[1]
		}
		if got != tt.expect {
			t.Errorf("Expected %v, got %v", tt.expect, got)
		}
	}
}
