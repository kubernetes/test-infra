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
	_ = glog.Errorf
)

func stringPtr(val string) *string            { return &val }
func timePtr(val time.Time) *time.Time        { return &val }
func intPtr(val int) *int                     { return &val }
func boolPtr(val bool) *bool                  { return &val }
func issuePtr(val github.Issue) *github.Issue { return &val }

func TestValidateLGTMAfterPush(t *testing.T) {
	tests := []struct {
		issueEvents  []github.IssueEvent
		shouldPass   bool
		lastModified time.Time
	}{
		{
			issueEvents: []github.IssueEvent{
				{
					Event: stringPtr("labeled"),
					Label: &github.Label{
						Name: stringPtr("lgtm"),
					},
					CreatedAt: timePtr(time.Unix(10, 0)),
				},
			},
			lastModified: time.Unix(9, 0),
			shouldPass:   true,
		},
		{
			issueEvents: []github.IssueEvent{
				{
					Event: stringPtr("labeled"),
					Label: &github.Label{
						Name: stringPtr("lgtm"),
					},
					CreatedAt: timePtr(time.Unix(10, 0)),
				},
			},
			lastModified: time.Unix(11, 0),
			shouldPass:   false,
		},
		{
			issueEvents: []github.IssueEvent{
				{
					Event: stringPtr("labeled"),
					Label: &github.Label{
						Name: stringPtr("lgtm"),
					},
					CreatedAt: timePtr(time.Unix(12, 0)),
				},
				{
					Event: stringPtr("labeled"),
					Label: &github.Label{
						Name: stringPtr("lgtm"),
					},
					CreatedAt: timePtr(time.Unix(11, 0)),
				},
				{
					Event: stringPtr("labeled"),
					Label: &github.Label{
						Name: stringPtr("lgtm"),
					},
					CreatedAt: timePtr(time.Unix(10, 0)),
				},
			},
			lastModified: time.Unix(11, 0),
			shouldPass:   true,
		},
		{
			issueEvents: []github.IssueEvent{
				{
					Event: stringPtr("labeled"),
					Label: &github.Label{
						Name: stringPtr("lgtm"),
					},
					CreatedAt: timePtr(time.Unix(10, 0)),
				},
				{
					Event: stringPtr("labeled"),
					Label: &github.Label{
						Name: stringPtr("lgtm"),
					},
					CreatedAt: timePtr(time.Unix(11, 0)),
				},
				{
					Event: stringPtr("labeled"),
					Label: &github.Label{
						Name: stringPtr("lgtm"),
					},
					CreatedAt: timePtr(time.Unix(12, 0)),
				},
			},
			lastModified: time.Unix(11, 0),
			shouldPass:   true,
		},
	}
	for _, test := range tests {
		config := &github_util.Config{}
		client, server, mux := github_test.InitTest()
		config.Org = "o"
		config.Project = "r"
		config.SetClient(client)

		mux.HandleFunc(fmt.Sprintf("/repos/o/r/issues/1/events"), func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("Unexpected method: %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			data, err := json.Marshal(test.issueEvents)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			w.Write(data)

			obj := github_util.MungeObject{
				Issue: issuePtr(github.Issue{
					Number: intPtr(1),
				}),
			}
			commits, err := config.GetFilledCommits(&obj)
			if err != nil {
				t.Errorf("Unexpected error getting filled commits: %v", err)
			}
			obj.Commits = commits

			events, err := config.GetAllEventsForPR(&obj)
			if err != nil {
				t.Errorf("Unexpected error getting events commits: %v", err)
			}
			obj.Events = events

			lastModifiedTime := github_util.LastModifiedTime(&obj)
			lgtmTime := github_util.LabelTime(&obj, "lgtm")

			if lastModifiedTime == nil || lgtmTime == nil {
				t.Errorf("unexpected lastModifiedTime or lgtmTime == nil")
			}

			ok := !lastModifiedTime.After(*lgtmTime)

			if ok != test.shouldPass {
				t.Errorf("expected: %v, saw: %v", test.shouldPass, ok)
			}
		})
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

func barePR() *github.PullRequest {
	return &github.PullRequest{
		Title:   stringPtr("My title"),
		Number:  intPtr(1),
		HTMLURL: stringPtr("PR URL"),
		Head: &github.PullRequestBranch{
			SHA: stringPtr("mysha"),
		},
		User: &github.User{
			Login:     stringPtr("UserNotInWhiteList"),
			AvatarURL: stringPtr("MyAvatarURL"),
		},
		Merged: boolPtr(false),
	}
}

func mergeablePR(pr *github.PullRequest) *github.PullRequest {
	return pr
}

func userInWhiteListPR(pr *github.PullRequest) *github.PullRequest {
	pr.User.Login = stringPtr("k8s-merge-robot")
	return pr
}

func ValidPR() *github.PullRequest {
	pr := barePR()
	pr.Mergeable = boolPtr(true)
	pr = userInWhiteListPR(pr)
	return pr
}

func UnMergeablePR() *github.PullRequest {
	pr := barePR()
	pr.Mergeable = boolPtr(false)
	pr = userInWhiteListPR(pr)
	return pr
}

func UndeterminedMergeablePR() *github.PullRequest {
	pr := barePR()
	pr.Mergeable = nil
	pr = userInWhiteListPR(pr)
	return pr
}

func NonWhitelistUserPR() *github.PullRequest {
	pr := barePR()
	pr.Mergeable = boolPtr(true)
	pr = mergeablePR(pr)
	return pr
}

func bareIssue() *github.Issue {
	return &github.Issue{
		Title:   stringPtr("My title"),
		Number:  intPtr(1),
		HTMLURL: stringPtr("PR URL"),
		User: &github.User{
			Login:     stringPtr("UserNotInWhiteList"),
			AvatarURL: stringPtr("MyAvatarURL"),
		},
	}
}

func labelIssue(label string, issue *github.Issue) *github.Issue {
	l := github.Label{
		Name: stringPtr(label),
	}
	issue.Labels = append(issue.Labels, l)
	return issue
}

func NoCLAIssue() *github.Issue {
	issue := bareIssue()
	labelIssue("lgtm", issue)
	labelIssue("ok-to-merge", issue)
	return issue
}

func NoLGTMIssue() *github.Issue {
	issue := bareIssue()
	labelIssue("cla: yes", issue)
	labelIssue("ok-to-merge", issue)
	return issue
}

func NoOKToMergeIssue() *github.Issue {
	issue := bareIssue()
	labelIssue("cla: yes", issue)
	labelIssue("lgtm", issue)
	return issue
}

func DontRequireGithubE2EIssue() *github.Issue {
	issue := bareIssue()
	labelIssue("cla: yes", issue)
	labelIssue("lgtm", issue)
	labelIssue("e2e-not-required", issue)
	return issue
}

func AllLabelsIssue() *github.Issue {
	issue := bareIssue()
	labelIssue("cla: yes", issue)
	labelIssue("lgtm", issue)
	labelIssue("ok-to-merge", issue)
	return issue
}

func OldLGTMEvents() []github.IssueEvent {
	return []github.IssueEvent{
		{
			Event: stringPtr("labeled"),
			Label: &github.Label{
				Name: stringPtr("lgtm"),
			},
			CreatedAt: timePtr(time.Unix(8, 0)),
		},
	}
}

func NewLGTMEvents() []github.IssueEvent {
	return []github.IssueEvent{
		{
			Event: stringPtr("labeled"),
			Label: &github.Label{
				Name: stringPtr("lgtm"),
			},
			CreatedAt: timePtr(time.Unix(10, 0)),
		},
	}
}

func Commits() []github.RepositoryCommit {
	return []github.RepositoryCommit{
		{
			Commit: &github.Commit{
				Committer: &github.CommitAuthor{
					Date: timePtr(time.Unix(9, 0)),
				},
			},
		},
	}
}

func bareStatus() github.CombinedStatus {
	return github.CombinedStatus{}
}

func updateStatusState(status github.CombinedStatus) github.CombinedStatus {
	prioMap := map[string]int{
		"pending": 4,
		"error":   3,
		"failure": 2,
		"success": 1,
		"":        0,
	}

	backMap := map[int]string{
		4: "pending",
		3: "error",
		2: "failure",
		1: "success",
		0: "",
	}

	sint := 0
	for _, s := range status.Statuses {
		newSint := prioMap[*s.State]
		if newSint > sint {
			sint = newSint
		}
	}
	status.State = stringPtr(backMap[sint])
	return status
}

func claStatus(status github.CombinedStatus) github.CombinedStatus {
	s := github.RepoStatus{
		Context: stringPtr("cla/google"),
		State:   stringPtr("success"),
	}
	status.Statuses = append(status.Statuses, s)
	return updateStatusState(status)
}

func jenkinsCIStatus(status github.CombinedStatus, state string) github.CombinedStatus {
	s := github.RepoStatus{
		Context: stringPtr(jenkinsCIContext),
		State:   stringPtr(state),
	}
	status.Statuses = append(status.Statuses, s)
	return updateStatusState(status)
}

func shippableStatus(status github.CombinedStatus, state string) github.CombinedStatus {
	s := github.RepoStatus{
		Context: stringPtr(shippableContext),
		State:   stringPtr(state),
	}
	status.Statuses = append(status.Statuses, s)
	return updateStatusState(status)
}

func successGithubStatus(status github.CombinedStatus) github.CombinedStatus {
	s := github.RepoStatus{
		Context: stringPtr(gceE2EContext),
		State:   stringPtr("success"),
	}
	status.Statuses = append(status.Statuses, s)
	return updateStatusState(status)
}

func failGithubStatus(status github.CombinedStatus) github.CombinedStatus {
	s := github.RepoStatus{
		Context: stringPtr(gceE2EContext),
		State:   stringPtr("failure"),
	}
	status.Statuses = append(status.Statuses, s)
	return updateStatusState(status)
}

func NoCLAStatus() github.CombinedStatus {
	status := bareStatus()
	return failGithubStatus(status)
}

func GitHubE2EFailStatus() github.CombinedStatus {
	status := bareStatus()
	status = claStatus(status)
	status = jenkinsCIStatus(status, "success")
	return failGithubStatus(status)
}

func JenkinsCIGreenShippablePendingStatus() github.CombinedStatus {
	status := bareStatus()
	status = claStatus(status)
	status = shippableStatus(status, "pending")
	status = jenkinsCIStatus(status, "success")
	return successGithubStatus(status)
}

func ShippableGreenStatus() github.CombinedStatus {
	status := bareStatus()
	status = claStatus(status)
	status = shippableStatus(status, "success")
	return successGithubStatus(status)
}

func SuccessStatus() github.CombinedStatus {
	status := bareStatus()
	status = claStatus(status)
	status = jenkinsCIStatus(status, "success")
	return successGithubStatus(status)
}

func bareJenkins() jenkins.Job {
	return jenkins.Job{}
}

func SuccessJenkins() jenkins.Job {
	job := bareJenkins()
	job.Result = "SUCCESS"
	return job
}

func FailJenkins() jenkins.Job {
	job := bareJenkins()
	job.Result = "FAILED"
	return job
}

func TestMungePullRequest(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	tests := []struct {
		pr               *github.PullRequest
		issue            *github.Issue
		commits          []github.RepositoryCommit
		events           []github.IssueEvent
		ciStatus         github.CombinedStatus
		jenkinsJob       jenkins.Job
		shouldPass       bool
		mergeAfterQueued bool
		reason           string
	}{
		// Should pass because the entire thing was run and good
		{
			pr:         ValidPR(),
			issue:      NoOKToMergeIssue(),
			events:     NewLGTMEvents(),
			commits:    Commits(),
			ciStatus:   SuccessStatus(),
			jenkinsJob: SuccessJenkins(),
			shouldPass: true,
			reason:     merged,
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
			shouldPass:       true,
			mergeAfterQueued: true,
			reason:           merged,
		},
		// Should merge even though github ci failed because of dont-require-e2e
		{
			pr:         ValidPR(),
			issue:      DontRequireGithubE2EIssue(),
			ciStatus:   GitHubE2EFailStatus(),
			events:     NewLGTMEvents(),
			commits:    Commits(),
			jenkinsJob: SuccessJenkins(),
			reason:     merged,
		},
		// Fail because PR can't automatically merge
		{
			pr:     UnMergeablePR(),
			issue:  NoOKToMergeIssue(),
			reason: unmergeable,
		},
		// Fail because we don't know if PR can automatically merge
		{
			pr:     UndeterminedMergeablePR(),
			issue:  NoOKToMergeIssue(),
			reason: undeterminedMergability,
		},
		// Fail because the "cla: yes" label was not applied
		{
			pr:     ValidPR(),
			issue:  NoCLAIssue(),
			reason: noCLA,
		},
		// Fail because github CI tests have failed (or at least are not success)
		{
			pr:     NonWhitelistUserPR(),
			issue:  NoOKToMergeIssue(),
			reason: ciFailure,
		},
		// Fail because the use is not in the whitelist and we don't have "ok-to-merge"
		{
			pr:       NonWhitelistUserPR(),
			issue:    NoOKToMergeIssue(),
			ciStatus: SuccessStatus(),
			reason:   needsok,
		},
		// Fail because missing LGTM label
		{
			pr:       NonWhitelistUserPR(),
			issue:    NoLGTMIssue(),
			ciStatus: SuccessStatus(),
			reason:   noLGTM,
		},
		// Fail because we can't tell if LGTM was added before the last change
		{
			pr:       ValidPR(),
			issue:    NoOKToMergeIssue(),
			ciStatus: SuccessStatus(),
			reason:   unknown,
		},
		// Fail because LGTM was added before the last change
		{
			pr:       ValidPR(),
			issue:    AllLabelsIssue(),
			ciStatus: SuccessStatus(),
			events:   OldLGTMEvents(),
			commits:  Commits(),
			reason:   lgtmEarly,
		},
		// Fail because jenkins instances are failing (whole submit queue blocks)
		{
			pr:         ValidPR(),
			issue:      NoOKToMergeIssue(),
			ciStatus:   SuccessStatus(),
			events:     NewLGTMEvents(),
			commits:    Commits(),
			jenkinsJob: FailJenkins(),
			reason:     e2eFailure,
		},
		// This is not really a failure, we just check that it reports e2e is in progress
		{
			pr:         ValidPR(),
			issue:      AllLabelsIssue(),
			ciStatus:   SuccessStatus(),
			events:     NewLGTMEvents(),
			commits:    Commits(),
			jenkinsJob: SuccessJenkins(),
			reason:     ghE2EQueued,
		},
		// Fail because the second run of github e2e tests failed
		{
			pr:         ValidPR(),
			issue:      AllLabelsIssue(),
			ciStatus:   SuccessStatus(),
			events:     NewLGTMEvents(),
			commits:    Commits(),
			jenkinsJob: SuccessJenkins(),
			reason:     ghE2EFailed,
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
			reason:     merged,
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
			reason:     merged,
		},
	}
	for testNum, test := range tests {
		client, server, mux := github_test.InitTest()

		config := &github_util.Config{}
		config.Org = "o"
		config.Project = "r"
		config.SetClient(client)
		// Don't wait so long for it to go pending or back
		d := 250 * time.Millisecond
		config.PendingWaitTime = &d

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
		})
		mux.HandleFunc("/repos/o/r/commits/mysha/status", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("Unexpected method: %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			data, err := json.Marshal(test.ciStatus)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			w.Write(data)

			// There is no good spot for this, but this gets called
			// before we queue the PR. So mark the PR as "merged".
			if test.mergeAfterQueued {
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
				go fakeRunGithubE2ESuccess(&test.ciStatus, test.shouldPass)
			}
			w.WriteHeader(http.StatusOK)
			data, err := json.Marshal(github.IssueComment{})
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			w.Write(data)
		})
		mux.HandleFunc("/repos/o/r/pulls/1", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("Unexpected method: %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			data, err := json.Marshal(test.pr)
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
		sq.RequiredStatusContexts = []string{"cla/google"}
		sq.DontRequireE2ELabel = "e2e-not-required"
		sq.E2EStatusContext = gceE2EContext
		sq.JenkinsHost = server.URL
		sq.JenkinsJobs = []string{"foo"}
		sq.WhitelistOverride = "ok-to-merge"
		sq.Initialize(config)
		sq.EachLoop(config)
		sq.userWhitelist.Insert("k8s-merge-robot")

		obj := github_util.MungeObject{
			Issue:   test.issue,
			PR:      test.pr,
			Commits: test.commits,
			Events:  test.events,
		}
		sq.MungePullRequest(config, &obj)
		done := make(chan bool, 1)
		go func(done chan bool, reason string) {
			for sq.prStatus["1"].Reason != reason {
				time.Sleep(1 * time.Millisecond)
			}
			done <- true
		}(done, test.reason)
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("test:%d timed out waiting expected reason=%q but got %q", testNum, test.reason, sq.prStatus["1"].Reason)
		}
		server.Close()
	}
}
