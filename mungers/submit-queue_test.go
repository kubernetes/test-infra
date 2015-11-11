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

package mungers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	github_util "k8s.io/contrib/mungegithub/github"
	github_test "k8s.io/contrib/mungegithub/github/testing"
	"k8s.io/contrib/mungegithub/mungers/jenkins"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
)

var (
	_ = fmt.Printf
	_ = glog.Errorf
)

func stringPtr(val string) *string { return &val }
func boolPtr(val bool) *bool       { return &val }

const noWhitelistUser = "UserNotInWhitelist"
const whitelistUser = "WhitelistUser"

func ValidPR() *github.PullRequest {
	return github_test.PullRequest(whitelistUser, false, true, true)
}

func UnMergeablePR() *github.PullRequest {
	return github_test.PullRequest(whitelistUser, false, true, false)
}

func UndeterminedMergeablePR() *github.PullRequest {
	return github_test.PullRequest(whitelistUser, false, false, false)
}

func NonWhitelistUserPR() *github.PullRequest {
	return github_test.PullRequest(noWhitelistUser, false, true, true)
}

func BareIssue() *github.Issue {
	return github_test.Issue(whitelistUser, 1, []string{}, true)
}

func NoOKToMergeIssue() *github.Issue {
	return github_test.Issue(whitelistUser, 1, []string{"cla: yes", "lgtm"}, true)
}

func NoCLAIssue() *github.Issue {
	return github_test.Issue(whitelistUser, 1, []string{"lgtm", "ok-to-merge"}, true)
}

func NoLGTMIssue() *github.Issue {
	return github_test.Issue(whitelistUser, 1, []string{"cla: yes", "ok-to-merge"}, true)
}

func UserNotInWhitelistNoOKToMergeIssue() *github.Issue {
	return github_test.Issue(noWhitelistUser, 1, []string{"cla: yes", "lgtm"}, true)
}

func UserNotInWhitelistOKToMergeIssue() *github.Issue {
	return github_test.Issue(noWhitelistUser, 1, []string{"lgtm", "cla: yes", "ok-to-merge"}, true)
}

func DontRequireGithubE2EIssue() *github.Issue {
	return github_test.Issue(whitelistUser, 1, []string{"cla: yes", "lgtm", "e2e-not-required"}, true)
}

func OldLGTMEvents() []github.IssueEvent {
	return github_test.Events([]github_test.LabelTime{
		{"lgtm", 6},
		{"lgtm", 7},
		{"lgtm", 8},
	})
}

func NewLGTMEvents() []github.IssueEvent {
	return github_test.Events([]github_test.LabelTime{
		{"lgtm", 10},
		{"lgtm", 11},
		{"lgtm", 12},
	})
}

func OverlappingLGTMEvents() []github.IssueEvent {
	return github_test.Events([]github_test.LabelTime{
		{"lgtm", 8},
		{"lgtm", 9},
		{"lgtm", 10},
	})
}

// Commits returns a slice of github.RepositoryCommit of len==3 which
// happened at times 7, 8, 9
func Commits() []github.RepositoryCommit {
	return github_test.Commits(3, 7)
}

func SuccessStatus() *github.CombinedStatus {
	return github_test.Status("mysha", []string{claContext, shippableContext, travisContext, jenkinsCIContext, gceE2EContext}, nil, nil, nil)
}

func JenkinsCIGreenShippablePendingStatus() *github.CombinedStatus {
	return github_test.Status("mysha", []string{claContext, jenkinsCIContext, travisContext, gceE2EContext}, nil, []string{shippableContext}, nil)
}

func ShippableGreenStatus() *github.CombinedStatus {
	return github_test.Status("mysha", []string{claContext, shippableContext, travisContext, gceE2EContext}, nil, nil, nil)
}

func GithubE2EFailStatus() *github.CombinedStatus {
	return github_test.Status("mysha", []string{claContext, shippableContext, travisContext}, []string{gceE2EContext}, nil, nil)
}

func SuccessJenkins() jenkins.Job {
	return jenkins.Job{
		Result: "SUCCESS",
	}
}

func FailJenkins() jenkins.Job {
	return jenkins.Job{
		Result: "FAILED",
	}
}

func TestValidateLGTMAfterPush(t *testing.T) {
	tests := []struct {
		issueEvents []github.IssueEvent
		commits     []github.RepositoryCommit
		shouldPass  bool
	}{
		{
			issueEvents: NewLGTMEvents(), // Label >= time.Unix(10)
			commits:     Commits(),       // Modified at time.Unix(7), 8, and 9
			shouldPass:  true,
		},
		{
			issueEvents: OldLGTMEvents(), // Label <= time.Unix(8)
			commits:     Commits(),       // Modified at time.Unix(7), 8, and 9
			shouldPass:  false,
		},
		{
			issueEvents: OverlappingLGTMEvents(), // Labeled at 8, 9, and 10
			commits:     Commits(),               // Modified at time.Unix(7), 8, and 9
			shouldPass:  true,
		},
	}
	for testNum, test := range tests {
		config := &github_util.Config{}
		client, server, _ := github_test.InitServer(t, nil, nil, test.issueEvents, test.commits, nil)
		config.Org = "o"
		config.Project = "r"
		config.SetClient(client)

		obj := github_util.TestObject(config, BareIssue(), nil, nil, nil)

		if _, err := obj.GetCommits(); err != nil {
			t.Errorf("Unexpected error getting filled commits: %v", err)
		}

		if _, err := obj.GetEvents(); err != nil {
			t.Errorf("Unexpected error getting events commits: %v", err)
		}

		lastModifiedTime := obj.LastModifiedTime()
		lgtmTime := obj.LabelTime("lgtm")

		if lastModifiedTime == nil || lgtmTime == nil {
			t.Errorf("unexpected lastModifiedTime or lgtmTime == nil")
		}

		ok := !lastModifiedTime.After(*lgtmTime)

		if ok != test.shouldPass {
			t.Errorf("%d: expected: %v, saw: %v", testNum, test.shouldPass, ok)
		}
		server.Close()
	}
}

// fakeRunGithubE2ESuccess imitates the github e2e running, but indicates
// success after a short sleep
func fakeRunGithubE2ESuccess(ciStatus *github.CombinedStatus, shouldPass bool) {
	ciStatus.State = stringPtr("pending")
	for id := range ciStatus.Statuses {
		status := &ciStatus.Statuses[id]
		if *status.Context == gceE2EContext {
			status.State = stringPtr("pending")
			break
		}
	}
	// short sleep like the test is running
	time.Sleep(500 * time.Millisecond)
	ciStatus.State = stringPtr("success")
	found := false
	for id := range ciStatus.Statuses {
		status := &ciStatus.Statuses[id]
		if *status.Context == gceE2EContext {
			if shouldPass {
				status.State = stringPtr("success")
			} else {
				status.State = stringPtr("failure")
			}
			found = true
			break
		}
	}
	if !found {
		e2eStatus := github.RepoStatus{
			Context: stringPtr(gceE2EContext),
			State:   stringPtr("success"),
		}
		ciStatus.Statuses = append(ciStatus.Statuses, e2eStatus)
	}
}

func TestMunge(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	tests := []struct {
		pr               *github.PullRequest
		issue            *github.Issue
		commits          []github.RepositoryCommit
		events           []github.IssueEvent
		ciStatus         *github.CombinedStatus
		jenkinsJob       jenkins.Job
		shouldPass       bool
		mergeAfterQueued bool
		reasons          []string
	}{
		// Should pass because the entire thing was run and good
		{
			pr:         ValidPR(),
			issue:      NoOKToMergeIssue(),
			events:     NewLGTMEvents(),
			commits:    Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:   SuccessStatus(),
			jenkinsJob: SuccessJenkins(),
			shouldPass: true,
			reasons:    []string{merged},
		},
		// Should list as 'merged' but the merge should happen before it gets e2e tested
		// and we should bail early instead of waiting for a test that will never come.
		{
			pr:         ValidPR(),
			issue:      NoOKToMergeIssue(),
			events:     NewLGTMEvents(),
			commits:    Commits(),
			ciStatus:   SuccessStatus(),
			jenkinsJob: SuccessJenkins(),
			// The test should never run, but if it does, make sure it fails
			shouldPass:       false,
			mergeAfterQueued: true,
			reasons:          []string{merged},
		},
		// Should merge even though github ci failed because of dont-require-e2e
		{
			pr:         ValidPR(),
			issue:      DontRequireGithubE2EIssue(),
			ciStatus:   GithubE2EFailStatus(),
			events:     NewLGTMEvents(),
			commits:    Commits(), // Modified at time.Unix(7), 8, and 9
			jenkinsJob: SuccessJenkins(),
			reasons:    []string{merged},
		},
		// Should merge even though user not in whitelist because has ok-to-merge
		{
			pr:         ValidPR(),
			issue:      UserNotInWhitelistOKToMergeIssue(),
			ciStatus:   SuccessStatus(),
			events:     NewLGTMEvents(),
			commits:    Commits(), // Modified at time.Unix(7), 8, and 9
			jenkinsJob: SuccessJenkins(),
			shouldPass: true,
			reasons:    []string{merged},
		},
		// Fail because PR can't automatically merge
		{
			pr:      UnMergeablePR(),
			issue:   NoOKToMergeIssue(),
			reasons: []string{unmergeable},
		},
		// Fail because we don't know if PR can automatically merge
		{
			pr:      UndeterminedMergeablePR(),
			issue:   NoOKToMergeIssue(),
			reasons: []string{undeterminedMergability},
		},
		// Fail because the "cla: yes" label was not applied
		{
			pr:      ValidPR(),
			issue:   NoCLAIssue(),
			reasons: []string{noCLA},
		},
		// Fail because github CI tests have failed (or at least are not success)
		{
			pr:      NonWhitelistUserPR(),
			issue:   NoOKToMergeIssue(),
			reasons: []string{ciFailure},
		},
		// Fail because the user is not in the whitelist and we don't have "ok-to-merge"
		{
			pr:       ValidPR(),
			issue:    UserNotInWhitelistNoOKToMergeIssue(),
			ciStatus: SuccessStatus(),
			reasons:  []string{needsok},
		},
		// Fail because missing LGTM label
		{
			pr:       ValidPR(),
			issue:    NoLGTMIssue(),
			ciStatus: SuccessStatus(),
			reasons:  []string{noLGTM},
		},
		// Fail because we can't tell if LGTM was added before the last change
		{
			pr:       ValidPR(),
			issue:    NoOKToMergeIssue(),
			ciStatus: SuccessStatus(),
			reasons:  []string{unknown},
		},
		// Fail because LGTM was added before the last change
		{
			pr:       ValidPR(),
			issue:    NoOKToMergeIssue(),
			ciStatus: SuccessStatus(),
			events:   OldLGTMEvents(),
			commits:  Commits(), // Modified at time.Unix(7), 8, and 9
			reasons:  []string{lgtmEarly},
		},
		// Fail because jenkins instances are failing (whole submit queue blocks)
		{
			pr:         ValidPR(),
			issue:      NoOKToMergeIssue(),
			ciStatus:   SuccessStatus(),
			events:     NewLGTMEvents(),
			commits:    Commits(), // Modified at time.Unix(7), 8, and 9
			jenkinsJob: FailJenkins(),
			reasons:    []string{e2eFailure},
		},
		// Fail because the second run of github e2e tests failed
		{
			pr:         ValidPR(),
			issue:      NoOKToMergeIssue(),
			ciStatus:   SuccessStatus(),
			events:     NewLGTMEvents(),
			commits:    Commits(),
			jenkinsJob: SuccessJenkins(),
			reasons:    []string{ghE2EFailed},
		},
		// Should pass because the jenkins ci is green even tho shippable is pending.
		{
			pr:         ValidPR(),
			issue:      NoOKToMergeIssue(),
			events:     NewLGTMEvents(),
			commits:    Commits(),
			ciStatus:   JenkinsCIGreenShippablePendingStatus(),
			jenkinsJob: SuccessJenkins(),
			shouldPass: true,
			reasons:    []string{merged},
		},
		// Should pass because the shippable is green (no jenkins ci).
		{
			pr:         ValidPR(),
			issue:      NoOKToMergeIssue(),
			events:     NewLGTMEvents(),
			commits:    Commits(),
			ciStatus:   ShippableGreenStatus(),
			jenkinsJob: SuccessJenkins(),
			shouldPass: true,
			reasons:    []string{merged},
		},
		// When we check the reason it may be queued or it may already have failed.
		{
			pr:         ValidPR(),
			issue:      NoOKToMergeIssue(),
			ciStatus:   SuccessStatus(),
			events:     NewLGTMEvents(),
			commits:    Commits(), // Modified at time.Unix(7), 8, and 9
			jenkinsJob: SuccessJenkins(),
			shouldPass: false,
			reasons:    []string{ghE2EQueued, ghE2EFailed},
		},
		// Fail because the second run of github e2e tests failed
		{
			pr:         ValidPR(),
			issue:      NoOKToMergeIssue(),
			ciStatus:   SuccessStatus(),
			events:     NewLGTMEvents(),
			commits:    Commits(), // Modified at time.Unix(7), 8, and 9
			jenkinsJob: SuccessJenkins(),
			shouldPass: false,
			reasons:    []string{ghE2EFailed},
		},
	}
	for testNum, test := range tests {
		client, server, mux := github_test.InitServer(t, test.issue, test.pr, test.events, test.commits, test.ciStatus)

		config := &github_util.Config{}
		config.Org = "o"
		config.Project = "r"
		config.SetClient(client)
		// Don't wait so long for it to go pending or back
		d := 250 * time.Millisecond
		config.PendingWaitTime = &d

		numJenkinsCalls := 0
		// Respond with success to jenkins requests.
		mux.HandleFunc("/job/foo/lastCompletedBuild/api/json", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("Unexpected method: %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			data, err := json.Marshal(test.jenkinsJob)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			w.Write(data)

			// There is no good spot for this, but this gets called
			// before we queue the PR. So mark the PR as "merged".
			// When the sq initializes, it will check the Jenkins status,
			// so we don't want to modify the PR there. Instead we need
			// to wait until the second time we check Jenkins, which happens
			// we did the IsMerged() check.
			numJenkinsCalls = numJenkinsCalls + 1
			if numJenkinsCalls == 2 && test.mergeAfterQueued {
				test.pr.Merged = boolPtr(true)
				test.pr.Mergeable = nil
			}
		})
		mux.HandleFunc("/repos/o/r/issues/1/comments", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("Unexpected method: %s", r.Method)
			}

			type comment struct {
				Body string `json:"body"`
			}
			c := new(comment)
			json.NewDecoder(r.Body).Decode(c)
			msg := c.Body
			if strings.HasPrefix(msg, "@k8s-bot test this") {
				go fakeRunGithubE2ESuccess(test.ciStatus, test.shouldPass)
			}
			w.WriteHeader(http.StatusOK)
			data, err := json.Marshal(github.IssueComment{})
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			w.Write(data)
		})
		mux.HandleFunc("/repos/o/r/pulls/1/merge", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "PUT" {
				t.Errorf("Unexpected method: %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			data, err := json.Marshal(github.PullRequestMergeResult{})
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			w.Write(data)
			test.pr.Merged = boolPtr(true)
		})

		sq := SubmitQueue{}
		sq.RequiredStatusContexts = []string{claContext}
		sq.DontRequireE2ELabel = "e2e-not-required"
		sq.E2EStatusContext = gceE2EContext
		sq.JenkinsHost = server.URL
		sq.JenkinsJobs = []string{"foo"}
		sq.WhitelistOverride = "ok-to-merge"
		sq.Initialize(config)
		sq.EachLoop()
		sq.userWhitelist.Insert(whitelistUser)

		obj := github_util.TestObject(config, test.issue, test.pr, test.commits, test.events)
		sq.Munge(obj)
		done := make(chan bool, 1)
		go func(done chan bool) {
			for {
				reason := sq.prStatus["1"].Reason
				for _, r := range test.reasons {
					if r == reason {
						done <- true
						return
					}
				}
				time.Sleep(1 * time.Millisecond)
			}
		}(done)
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("test:%d timed out waiting expected reason=%v but got %q", testNum, test.reasons, sq.prStatus["1"].Reason)
		}
		server.Close()
	}
}
