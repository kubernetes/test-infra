/*
Copyright 2015 The Kubernetes Authors.

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
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	utilclock "k8s.io/apimachinery/pkg/util/clock"

	"k8s.io/contrib/test-utils/utils"
	"k8s.io/test-infra/mungegithub/features"
	github_util "k8s.io/test-infra/mungegithub/github"
	github_test "k8s.io/test-infra/mungegithub/github/testing"
	"k8s.io/test-infra/mungegithub/mungeopts"
	"k8s.io/test-infra/mungegithub/mungers/e2e"
	fake_e2e "k8s.io/test-infra/mungegithub/mungers/e2e/fake"
	"k8s.io/test-infra/mungegithub/mungers/mungerutil"
	"k8s.io/test-infra/mungegithub/options"
	"k8s.io/test-infra/mungegithub/sharedmux"

	"github.com/google/go-github/github"
)

func stringPtr(val string) *string    { return &val }
func boolPtr(val bool) *bool          { return &val }
func intPtr(val int) *int             { return &val }
func slicePtr(val []string) *[]string { return &val }

const (
	someUserName        = "someUserName"
	doNotMergeMilestone = "some-milestone-you-should-not-merge"

	notRequiredReTestContext1 = "someNotRequiredForRetest1"
	notRequiredReTestContext2 = "someNotRequiredForRetest2"
	requiredReTestContext1    = "someRequiredRetestContext1"
	requiredReTestContext2    = "someRequiredRetestContext2"
)

var (
	someJobNames = []string{"foo", "bar"}
)

func ValidPR() *github.PullRequest {
	return github_test.PullRequest(someUserName, false, true, true)
}

func UnMergeablePR() *github.PullRequest {
	return github_test.PullRequest(someUserName, false, true, false)
}

func UndeterminedMergeablePR() *github.PullRequest {
	return github_test.PullRequest(someUserName, false, false, false)
}

func MasterCommit() *github.RepositoryCommit {
	masterSHA := "mastersha"
	return &github.RepositoryCommit{
		SHA: &masterSHA,
	}
}

func BareIssue() *github.Issue {
	return github_test.Issue(someUserName, 1, []string{}, true)
}

func LGTMIssue() *github.Issue {
	return github_test.Issue(someUserName, 1, []string{cncfClaYesLabel, lgtmLabel}, true)
}

func LGTMApprovedIssue() *github.Issue {
	return github_test.Issue(someUserName, 1, []string{cncfClaYesLabel, lgtmLabel, approvedLabel}, true)
}

func LGTMApprovedCLAHumanApprovedIssue() *github.Issue {
	return github_test.Issue(someUserName, 1, []string{claHumanLabel, lgtmLabel, approvedLabel}, true)
}

func CriticalFixLGTMApprovedIssue() *github.Issue {
	return github_test.Issue(someUserName, 1, []string{criticalFixLabel, cncfClaYesLabel, lgtmLabel, approvedLabel}, true)
}

func OnlyApprovedIssue() *github.Issue {
	return github_test.Issue(someUserName, 1, []string{cncfClaYesLabel, approvedLabel}, true)
}

func DoNotMergeIssue() *github.Issue {
	return github_test.Issue(someUserName, 1, []string{cncfClaYesLabel, lgtmLabel, approvedLabel, doNotMergeLabel}, true)
}

func CherrypickUnapprovedIssue() *github.Issue {
	return github_test.Issue(someUserName, 1, []string{cncfClaYesLabel, lgtmLabel, approvedLabel, cherrypickUnapprovedLabel}, true)
}

func BlockedPathsIssue() *github.Issue {
	return github_test.Issue(someUserName, 1, []string{cncfClaYesLabel, lgtmLabel, approvedLabel, blockedPathsLabel}, true)
}

func MissingReleaseNoteIssue() *github.Issue {
	return github_test.Issue(someUserName, 1, []string{cncfClaYesLabel, lgtmLabel, approvedLabel, releaseNoteLabelNeeded}, true)
}

func WorkInProgressIssue() *github.Issue {
	return github_test.Issue(someUserName, 1, []string{cncfClaYesLabel, lgtmLabel, approvedLabel, wipLabel}, true)
}

func HoldLabelIssue() *github.Issue {
	return github_test.Issue(someUserName, 1, []string{cncfClaYesLabel, lgtmLabel, approvedLabel, holdLabel}, true)
}

func AdditionalLabelIssue(label string) *github.Issue {
	return github_test.Issue(someUserName, 1, []string{cncfClaYesLabel, lgtmLabel, approvedLabel, label}, true)
}

func DoNotMergeMilestoneIssue() *github.Issue {
	issue := github_test.Issue(someUserName, 1, []string{cncfClaYesLabel, lgtmLabel}, true)
	milestone := &github.Milestone{
		Title: stringPtr(doNotMergeMilestone),
	}
	issue.Milestone = milestone
	return issue
}

func NoCLAIssue() *github.Issue {
	return github_test.Issue(someUserName, 1, []string{lgtmLabel}, true)
}

func NoLGTMIssue() *github.Issue {
	return github_test.Issue(someUserName, 1, []string{cncfClaYesLabel}, true)
}

func NoRetestIssue() *github.Issue {
	return github_test.Issue(someUserName, 1, []string{cncfClaYesLabel, lgtmLabel, approvedLabel, retestNotRequiredLabel}, true)
}

func OldLGTMEvents() []*github.IssueEvent {
	return github_test.Events([]github_test.LabelTime{
		{User: "bob", Label: approvedLabel, Time: 20},
		{User: "bob", Label: lgtmLabel, Time: 6},
		{User: "bob", Label: lgtmLabel, Time: 7},
		{User: "bob", Label: lgtmLabel, Time: 8},
	})
}

func NewLGTMEvents() []*github.IssueEvent {
	return github_test.Events([]github_test.LabelTime{
		{User: "bob", Label: approvedLabel, Time: 20},
		{User: "bob", Label: lgtmLabel, Time: 10},
		{User: "bob", Label: lgtmLabel, Time: 11},
		{User: "bob", Label: lgtmLabel, Time: 12},
	})
}

func OverlappingLGTMEvents() []*github.IssueEvent {
	return github_test.Events([]github_test.LabelTime{
		{User: "bob", Label: approvedLabel, Time: 20},
		{User: "bob", Label: lgtmLabel, Time: 8},
		{User: "bob", Label: lgtmLabel, Time: 9},
		{User: "bob", Label: lgtmLabel, Time: 10},
	})
}

func OldApprovedEvents() []*github.IssueEvent {
	return github_test.Events([]github_test.LabelTime{
		{User: "bob", Label: approvedLabel, Time: 6},
		{User: "bob", Label: lgtmLabel, Time: 10},
		{User: "bob", Label: lgtmLabel, Time: 11},
		{User: "bob", Label: lgtmLabel, Time: 12},
	})
}

// Commits returns a slice of github.RepositoryCommit of len==3 which
// happened at times 7, 8, 9
func Commits() []*github.RepositoryCommit {
	return github_test.Commits(3, 7)
}

func SuccessStatus() *github.CombinedStatus {
	return github_test.Status("mysha", []string{requiredReTestContext1, requiredReTestContext2, notRequiredReTestContext1, notRequiredReTestContext2}, nil, nil, nil)
}

func RetestFailStatus() *github.CombinedStatus {
	return github_test.Status("mysha", []string{requiredReTestContext1, notRequiredReTestContext1, notRequiredReTestContext2}, []string{requiredReTestContext2}, nil, nil)
}

func NoRetestFailStatus() *github.CombinedStatus {
	return github_test.Status("mysha", []string{requiredReTestContext1, requiredReTestContext2, notRequiredReTestContext1}, []string{notRequiredReTestContext2}, nil, nil)
}

func LastBuildNumber() int {
	return 42
}

func SuccessGCS() utils.FinishedFile {
	return utils.FinishedFile{
		Result:    "SUCCESS",
		Timestamp: uint64(time.Now().Unix()),
	}
}

func FailGCS() utils.FinishedFile {
	return utils.FinishedFile{
		Result:    "FAILURE",
		Timestamp: uint64(time.Now().Unix()),
	}
}

func getJUnit(testsNo int, failuresNo int) []byte {
	return []byte(fmt.Sprintf("%v\n<testsuite tests=\"%v\" failures=\"%v\" time=\"1234\">\n</testsuite>",
		e2e.ExpectedXMLHeader, testsNo, failuresNo))
}

func getTestSQ(startThreads bool, config *github_util.Config, server *httptest.Server) *SubmitQueue {
	// TODO: Remove this line when we fix the plumbing regarding the fake/real e2e tester.
	sharedmux.Admin = sharedmux.NewConcurrentMux(http.NewServeMux())
	sq := new(SubmitQueue)
	sq.opts = options.New()

	feats := &features.Features{
		Server: &features.ServerFeature{
			Enabled: false,
		},
	}

	sq.GateApproved = true
	sq.GateCLA = true
	sq.NonBlockingJobNames = someJobNames
	sq.DoNotMergeMilestones = []string{doNotMergeMilestone}
	sq.ClaYesLabels = []string{cncfClaYesLabel, claHumanLabel}

	mungeopts.RequiredContexts.Merge = []string{notRequiredReTestContext1, notRequiredReTestContext2}
	mungeopts.RequiredContexts.Retest = []string{requiredReTestContext1, requiredReTestContext2}
	mungeopts.PRMaxWaitTime = 2 * time.Hour

	sq.githubE2EQueue = map[int]*github_util.MungeObject{}
	sq.githubE2EPollTime = time.Millisecond

	sq.clock = utilclock.NewFakeClock(time.Time{})
	sq.lastMergeTime = sq.clock.Now()
	sq.lastE2EStable = true
	sq.prStatus = map[string]submitStatus{}
	sq.lgtmTimeCache = mungerutil.NewLabelTimeCache(lgtmLabel)

	sq.startTime = sq.clock.Now()
	sq.healthHistory = make([]healthRecord, 0)

	sq.e2e = &fake_e2e.FakeE2ETester{JobNames: sq.NonBlockingJobNames}

	if startThreads {
		sq.internalInitialize(config, feats, server.URL)
		sq.EachLoop()
	}
	return sq
}

func TestQueueOrder(t *testing.T) {
	timeBase := time.Now()
	time2 := timeBase.Add(6 * time.Minute).Unix()
	time3 := timeBase.Add(5 * time.Minute).Unix()
	time4 := timeBase.Add(4 * time.Minute).Unix()
	time5 := timeBase.Add(3 * time.Minute).Unix()
	time6 := timeBase.Add(2 * time.Minute).Unix()
	labelEvents := map[int][]github_test.LabelTime{
		2: {{User: "me", Label: lgtmLabel, Time: time2}},
		3: {{User: "me", Label: lgtmLabel, Time: time3}},
		4: {{User: "me", Label: lgtmLabel, Time: time4}},
		5: {{User: "me", Label: lgtmLabel, Time: time5}},
		6: {{User: "me", Label: lgtmLabel, Time: time6}},
	}

	tests := []struct {
		name          string
		issues        []*github.Issue
		issueToEvents map[int][]github_test.LabelTime
		expected      []int
	}{
		{
			name: "Just prNum",
			issues: []*github.Issue{
				github_test.Issue(someUserName, 2, nil, true),
				github_test.Issue(someUserName, 3, nil, true),
				github_test.Issue(someUserName, 4, nil, true),
				github_test.Issue(someUserName, 5, nil, true),
			},
			issueToEvents: labelEvents,
			expected:      []int{5, 4, 3, 2},
		},
		{
			name: "With a priority label",
			issues: []*github.Issue{
				github_test.Issue(someUserName, 2, []string{multirebaseLabel}, true),
				github_test.Issue(someUserName, 3, []string{multirebaseLabel}, true),
				github_test.Issue(someUserName, 4, []string{criticalFixLabel}, true),
				github_test.Issue(someUserName, 5, nil, true),
			},
			issueToEvents: labelEvents,
			expected:      []int{4, 3, 2, 5},
		},
		{
			name: "With two priority labels",
			issues: []*github.Issue{
				github_test.Issue(someUserName, 2, []string{fixLabel, criticalFixLabel}, true),
				github_test.Issue(someUserName, 3, []string{fixLabel}, true),
				github_test.Issue(someUserName, 4, []string{criticalFixLabel}, true),
				github_test.Issue(someUserName, 5, nil, true),
			},
			issueToEvents: labelEvents,
			expected:      []int{4, 2, 3, 5},
		},
		{
			name: "With unrelated labels",
			issues: []*github.Issue{
				github_test.Issue(someUserName, 2, []string{fixLabel, criticalFixLabel}, true),
				github_test.Issue(someUserName, 3, []string{fixLabel, "kind/design"}, true),
				github_test.Issue(someUserName, 4, []string{criticalFixLabel}, true),
				github_test.Issue(someUserName, 5, []string{lgtmLabel, "kind/new-api"}, true),
			},
			issueToEvents: labelEvents,
			expected:      []int{4, 2, 3, 5},
		},
		{
			name: "With invalid priority label",
			issues: []*github.Issue{
				github_test.Issue(someUserName, 2, []string{fixLabel, criticalFixLabel}, true),
				github_test.Issue(someUserName, 3, []string{fixLabel, "kind/design", "priority/high"}, true),
				github_test.Issue(someUserName, 4, []string{criticalFixLabel, "priorty/bob"}, true),
				github_test.Issue(someUserName, 5, nil, true),
			},
			issueToEvents: labelEvents,
			expected:      []int{4, 2, 3, 5},
		},
		{
			name: "Unlabeled counts as below",
			issues: []*github.Issue{
				github_test.Issue(someUserName, 2, nil, true),
				github_test.Issue(someUserName, 3, []string{blocksOthersLabel}, true),
				github_test.Issue(someUserName, 4, []string{multirebaseLabel}, true),
				github_test.Issue(someUserName, 5, nil, true),
			},
			issueToEvents: labelEvents,
			expected:      []int{4, 3, 5, 2},
		},
		{
			name: "retestNotRequiredLabel counts above everything except criticalFixLabel",
			issues: []*github.Issue{
				github_test.Issue(someUserName, 2, nil, true),
				github_test.Issue(someUserName, 3, []string{blocksOthersLabel}, true),
				github_test.Issue(someUserName, 4, []string{fixLabel}, true),
				github_test.Issue(someUserName, 5, []string{criticalFixLabel}, true),
				github_test.Issue(someUserName, 6, []string{blocksOthersLabel, retestNotRequiredLabel}, true),
			},
			issueToEvents: labelEvents,
			expected:      []int{5, 6, 4, 3, 2},
		},
	}
	for testNum, test := range tests {
		config := &github_util.Config{Org: "o", Project: "r"}
		client, server, mux := github_test.InitServer(t, nil, nil, github_test.MultiIssueEvents(test.issueToEvents, "labeled"), nil, nil, nil, nil)
		config.SetClient(client)
		sq := getTestSQ(false, config, server)
		for i := range test.issues {
			issue := test.issues[i]
			github_test.ServeIssue(t, mux, issue)

			issueNum := *issue.Number
			obj, err := config.GetObject(issueNum)
			if err != nil {
				t.Fatalf("%d:%q unable to get issue: %v", testNum, test.name, err)
			}
			sq.githubE2EQueue[issueNum] = obj
		}
		actual := sq.orderedE2EQueue()
		if len(actual) != len(test.expected) {
			t.Fatalf("%d:%q len(actual):%v != len(expected):%v", testNum, test.name, actual, test.expected)
		}
		for i, a := range actual {
			e := test.expected[i]
			if a != e {
				t.Errorf("%d:%q a[%d]:%d != e[%d]:%d", testNum, test.name, i, a, i, e)
			}
		}
		server.Close()
	}
}

func TestValidateLGTMAfterPush(t *testing.T) {
	tests := []struct {
		issueEvents []*github.IssueEvent
		commits     []*github.RepositoryCommit
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
		config := &github_util.Config{Org: "o", Project: "r"}
		client, server, _ := github_test.InitServer(t, nil, nil, test.issueEvents, test.commits, nil, nil, nil)
		config.SetClient(client)

		obj := github_util.NewTestObject(config, BareIssue(), nil, nil, nil)

		if _, ok := obj.GetCommits(); !ok {
			t.Errorf("Unexpected error getting filled commits")
		}

		if _, ok := obj.GetEvents(); !ok {
			t.Errorf("Unexpected error getting events commits")
		}

		lastModifiedTime, ok1 := obj.LastModifiedTime()
		lgtmTime, ok2 := obj.LabelTime(lgtmLabel)

		if !ok1 || !ok2 || lastModifiedTime == nil || lgtmTime == nil {
			t.Errorf("unexpected lastModifiedTime or lgtmTime == nil")
		}

		ok := !lastModifiedTime.After(*lgtmTime)

		if ok != test.shouldPass {
			t.Errorf("%d: expected: %v, saw: %v", testNum, test.shouldPass, ok)
		}
		server.Close()
	}
}

func setStatus(status *github.RepoStatus, success bool) {
	if success {
		status.State = stringPtr("success")
	} else {
		status.State = stringPtr("failure")
	}
}

func addStatus(context string, success bool, ciStatus *github.CombinedStatus) {
	status := github.RepoStatus{
		Context: stringPtr(context),
	}
	setStatus(&status, success)
	ciStatus.Statuses = append(ciStatus.Statuses, status)
}

// fakeRunGithubE2ESuccess imitates jenkins running
func fakeRunGithubE2ESuccess(ciStatus *github.CombinedStatus, context1Pass, context2Pass bool) {
	ciStatus.State = stringPtr("pending")
	for id := range ciStatus.Statuses {
		status := &ciStatus.Statuses[id]
		if *status.Context == requiredReTestContext1 || *status.Context == requiredReTestContext2 {
			status.State = stringPtr("pending")
		}
	}
	// short sleep like the test is running
	time.Sleep(500 * time.Millisecond)
	if context1Pass && context2Pass {
		ciStatus.State = stringPtr("success")
	}
	foundContext1 := false
	foundContext2 := false
	for id := range ciStatus.Statuses {
		status := &ciStatus.Statuses[id]
		if *status.Context == requiredReTestContext1 {
			setStatus(status, context1Pass)
			foundContext1 = true
		}
		if *status.Context == requiredReTestContext2 {
			setStatus(status, context2Pass)
			foundContext2 = true
		}
	}
	if !foundContext1 {
		addStatus(jenkinsE2EContext, context1Pass, ciStatus)
	}
	if !foundContext2 {
		addStatus(jenkinsUnitContext, context2Pass, ciStatus)
	}
}

func TestSubmitQueue(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Since we testing, don't rateLimit api calls. Go hog wild
	github_util.SetCombinedStatusLifetime(1)

	tests := []struct {
		name             string // because when the fail, counting is hard
		pr               *github.PullRequest
		issue            *github.Issue
		commits          []*github.RepositoryCommit
		events           []*github.IssueEvent
		additionalLabels []string
		blockingLabels   []string
		ciStatus         *github.CombinedStatus
		lastBuildNumber  int
		gcsResult        utils.FinishedFile
		retest1Pass      bool
		retest2Pass      bool
		mergeAfterQueued bool
		reason           string
		state            string // what the github status context should be for the PR HEAD

		emergencyMergeStop bool
		isMerged           bool

		imHeadSHA      string
		imBaseSHA      string
		masterCommit   *github.RepositoryCommit
		retestsAvoided int // desired output
	}{
		// Should pass because the entire thing was run and good (with cncf-cla: yes)
		{
			name:            "Test1",
			pr:              ValidPR(),
			issue:           LGTMApprovedIssue(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        SuccessStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			retest1Pass:     true,
			retest2Pass:     true,
			reason:          merged,
			state:           "success",
			isMerged:        true,
		},
		// Should pass because the entire thing was run and good (with cla: human-approved)
		{
			name:            "Test1",
			pr:              ValidPR(),
			issue:           LGTMApprovedCLAHumanApprovedIssue(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        SuccessStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			retest1Pass:     true,
			retest2Pass:     true,
			reason:          merged,
			state:           "success",
			isMerged:        true,
		},
		{
			name:            "Test1+NoLgtm",
			pr:              ValidPR(),
			issue:           OnlyApprovedIssue(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        SuccessStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			retest1Pass:     true,
			retest2Pass:     true,
			reason:          noLGTM,
			state:           "pending",
			isMerged:        false,
		},
		// Entire thing was run and good, but emergency merge stop in progress
		{
			name:               "Test1+emergencyStop",
			pr:                 ValidPR(),
			issue:              LGTMApprovedIssue(),
			events:             NewLGTMEvents(),
			commits:            Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:           SuccessStatus(),
			lastBuildNumber:    LastBuildNumber(),
			gcsResult:          SuccessGCS(),
			retest1Pass:        true,
			retest2Pass:        true,
			emergencyMergeStop: true,
			isMerged:           false,
			reason:             e2eFailure,
			state:              "success",
		},
		// Should pass without running tests because we had a previous run.
		// TODO: Add a proper test to make sure we don't shuffle queue when we can just merge a PR
		{
			name:            "Test1+prevsuccess",
			pr:              ValidPR(),
			issue:           LGTMApprovedIssue(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        SuccessStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			retest1Pass:     true,
			retest2Pass:     true,
			reason:          merged,
			state:           "success",
			isMerged:        true,
			retestsAvoided:  1,
			imHeadSHA:       "mysha", // Set by ValidPR
			imBaseSHA:       "mastersha",
			masterCommit:    MasterCommit(),
		},
		// Should list as 'merged' but the merge should happen before it gets e2e tested
		// and we should bail early instead of waiting for a test that will never come.
		{
			name:            "Test2",
			pr:              ValidPR(),
			issue:           LGTMApprovedIssue(),
			events:          NewLGTMEvents(),
			commits:         Commits(),
			ciStatus:        SuccessStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			// The test should never run, but if it does, make sure it fails
			mergeAfterQueued: true,
			reason:           mergedByHand,
			state:            "success",
		},
		// Should merge even though retest1Pass would have failed before of `retestNotRequiredLabel`
		{
			name:            "merge because of retestNotRequired",
			pr:              ValidPR(),
			issue:           NoRetestIssue(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        SuccessStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			retest1Pass:     false,
			retest2Pass:     false,
			reason:          mergedSkippedRetest,
			state:           "success",
			isMerged:        true,
		},
		// Fail because PR can't automatically merge
		{
			name:   "Test5",
			pr:     UnMergeablePR(),
			issue:  LGTMApprovedIssue(),
			reason: unmergeable,
			state:  "pending",
			// To avoid false errors in logs
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
		},
		// Fail because we don't know if PR can automatically merge
		{
			name:   "Test6",
			pr:     UndeterminedMergeablePR(),
			issue:  LGTMApprovedIssue(),
			reason: undeterminedMergability,
			state:  "pending",
			// To avoid false errors in logs
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
		},
		// Fail because the cncfClaYesLabel or claHumanLabel label was not applied
		{
			name:   "Test7",
			pr:     ValidPR(),
			issue:  NoCLAIssue(),
			reason: fmt.Sprintf("%s %q", noCLA, []string{cncfClaYesLabel, claHumanLabel}),
			state:  "pending",
			// To avoid false errors in logs
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
		},
		// Fail because github CI tests have failed (or at least are not success)
		{
			name:   "Test8",
			pr:     ValidPR(),
			issue:  LGTMApprovedIssue(),
			reason: fmt.Sprintf(ciFailureFmt, notRequiredReTestContext1),
			state:  "pending",
			// To avoid false errors in logs
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
		},
		// Fail because missing LGTM label
		{
			name:     "Test10",
			pr:       ValidPR(),
			issue:    NoLGTMIssue(),
			ciStatus: SuccessStatus(),
			reason:   noLGTM,
			state:    "pending",
			// To avoid false errors in logs
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
		},
		// Fail because we can't tell if LGTM was added before the last change
		{
			name:     "Test11",
			pr:       ValidPR(),
			issue:    LGTMApprovedIssue(),
			ciStatus: SuccessStatus(),
			reason:   unknown,
			state:    "failure",
			// To avoid false errors in logs
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
		},
		// Fail because LGTM was added before the last change
		{
			name:     "Test12",
			pr:       ValidPR(),
			issue:    LGTMApprovedIssue(),
			ciStatus: SuccessStatus(),
			events:   OldLGTMEvents(),
			commits:  Commits(), // Modified at time.Unix(7), 8, and 9
			reason:   lgtmEarly,
			state:    "pending",
		},
		// Fail because jenkins instances are failing (whole submit queue blocks)
		{
			name:            "Test13",
			pr:              ValidPR(),
			issue:           LGTMApprovedIssue(),
			ciStatus:        SuccessStatus(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       FailGCS(),
			reason:          ghE2EQueued,
			state:           "success",
		},
		// Fail because the second run of github e2e tests failed
		{
			name:            "Test14",
			pr:              ValidPR(),
			issue:           LGTMApprovedIssue(),
			ciStatus:        SuccessStatus(),
			events:          NewLGTMEvents(),
			commits:         Commits(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			reason:          ghE2EFailed,
			state:           "pending",
		},
		// When we check the reason it may be queued or it may already have failed.
		{
			name:            "Test15",
			pr:              ValidPR(),
			issue:           LGTMApprovedIssue(),
			ciStatus:        SuccessStatus(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			reason:          ghE2EQueued,
			// The state is unpredictable. When it goes on the queue it is success.
			// When it fails the build it is pending. So state depends on how far along
			// this were when we checked. Thus just don't check it...
			state: "",
		},
		// Fail because the second run of github e2e tests failed
		{
			name:            "Test16",
			pr:              ValidPR(),
			issue:           LGTMApprovedIssue(),
			ciStatus:        SuccessStatus(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			reason:          ghE2EFailed,
			state:           "pending",
		},
		{
			name:            "Fail because E2E pass, but unit test fail",
			pr:              ValidPR(),
			issue:           LGTMApprovedIssue(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        SuccessStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			retest1Pass:     true,
			retest2Pass:     false,
			reason:          ghE2EFailed,
			state:           "pending",
		},
		{
			name:            "Fail because E2E fail, but unit test pass",
			pr:              ValidPR(),
			issue:           LGTMApprovedIssue(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        SuccessStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			retest1Pass:     false,
			retest2Pass:     true,
			reason:          ghE2EFailed,
			state:           "pending",
		},
		{
			name:            "Fail because missing release note label is present",
			pr:              ValidPR(),
			issue:           MissingReleaseNoteIssue(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        SuccessStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			retest1Pass:     true,
			retest2Pass:     true,
			reason:          noMergeMessage(releaseNoteLabelNeeded),
			state:           "pending",
		},
		{
			name:            "Fail because hold label is present",
			pr:              ValidPR(),
			issue:           HoldLabelIssue(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        SuccessStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			retest1Pass:     true,
			retest2Pass:     true,
			reason:          noMergeMessage(holdLabel),
			state:           "pending",
		},
		{
			name:             "Fail because kind/blocker label is required but missing",
			pr:               ValidPR(),
			issue:            LGTMApprovedIssue(),
			additionalLabels: []string{"kind/blocker"},
			events:           NewLGTMEvents(),
			commits:          Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:         SuccessStatus(),
			lastBuildNumber:  LastBuildNumber(),
			gcsResult:        SuccessGCS(),
			retest1Pass:      true,
			retest2Pass:      true,
			reason:           noAdditionalLabelMessage("kind/blocker"),
			state:            "pending",
		},
		{
			name:             "Merge kind/blocker PR",
			pr:               ValidPR(),
			issue:            AdditionalLabelIssue("kind/blocker"),
			additionalLabels: []string{"kind/blocker"},
			events:           NewLGTMEvents(),
			commits:          Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:         SuccessStatus(),
			lastBuildNumber:  LastBuildNumber(),
			gcsResult:        SuccessGCS(),
			retest1Pass:      true,
			retest2Pass:      true,
			reason:           merged,
			state:            "success",
			isMerged:         true,
		},
		{
			name:            "Fail because vendor-update label is required to be missing but exists",
			pr:              ValidPR(),
			issue:           AdditionalLabelIssue("vendor-update"),
			blockingLabels:  []string{"vendor-update"},
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        SuccessStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			retest1Pass:     true,
			retest2Pass:     true,
			reason:          noMergeMessage("vendor-update"),
			state:           "pending",
		},
		{
			name:            "Fail because do not merge label is present",
			pr:              ValidPR(),
			issue:           DoNotMergeIssue(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        SuccessStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			retest1Pass:     true,
			retest2Pass:     true,
			reason:          noMergeMessage(doNotMergeLabel),
			state:           "pending",
		},
		{
			name:            "Fail because cherrypick unapproved label is present",
			pr:              ValidPR(),
			issue:           CherrypickUnapprovedIssue(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        SuccessStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			retest1Pass:     true,
			retest2Pass:     true,
			reason:          noMergeMessage(cherrypickUnapprovedLabel),
			state:           "pending",
		},
		{
			name:            "Fail because blocked paths label is present",
			pr:              ValidPR(),
			issue:           BlockedPathsIssue(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        SuccessStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			retest1Pass:     true,
			retest2Pass:     true,
			reason:          noMergeMessage(blockedPathsLabel),
			state:           "pending",
		},
		{
			name:            "Fail because work-in-progress label is present",
			pr:              ValidPR(),
			issue:           WorkInProgressIssue(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        SuccessStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			retest1Pass:     true,
			retest2Pass:     true,
			reason:          noMergeMessage(wipLabel),
			state:           "pending",
		},
		// Should fail because the 'do-not-merge-milestone' is set.
		{
			name:            "Do Not Merge Milestone Set",
			pr:              ValidPR(),
			issue:           DoNotMergeMilestoneIssue(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        SuccessStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			retest1Pass:     true,
			retest2Pass:     true,
			reason:          unmergeableMilestone,
			state:           "pending",
		},
		{
			name:            "Fail because retest status fail",
			pr:              ValidPR(),
			issue:           LGTMApprovedIssue(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        RetestFailStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			retest1Pass:     true,
			retest2Pass:     true,
			reason:          fmt.Sprintf(ciFailureFmt, requiredReTestContext2),
			state:           "pending",
		},
		{
			name:            "Fail because noretest status fail",
			pr:              ValidPR(),
			issue:           LGTMApprovedIssue(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        NoRetestFailStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			retest1Pass:     true,
			retest2Pass:     true,
			reason:          fmt.Sprintf(ciFailureFmt, notRequiredReTestContext2),
			state:           "pending",
		},
		{
			name:            "Approval Can Happen Before Code Changes",
			pr:              ValidPR(),
			issue:           LGTMApprovedIssue(),
			events:          OldApprovedEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        SuccessStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       SuccessGCS(),
			retest1Pass:     true,
			retest2Pass:     true,
			reason:          merged,
			state:           "success",
			isMerged:        true,
			retestsAvoided:  1,
			imHeadSHA:       "mysha", // Set by ValidPR
			imBaseSHA:       "mastersha",
			masterCommit:    MasterCommit(),
		},
		{
			name:            "criticalFixLabel should merge even though jenkins GCS fail",
			pr:              ValidPR(),
			issue:           CriticalFixLGTMApprovedIssue(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        SuccessStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       FailGCS(),
			retest1Pass:     true,
			retest2Pass:     true,
			reason:          merged,
			state:           "success",
			isMerged:        true,
		},
		{
			name:            "criticalFixLabel but should fail if e2e's fail",
			pr:              ValidPR(),
			issue:           CriticalFixLGTMApprovedIssue(),
			events:          NewLGTMEvents(),
			commits:         Commits(), // Modified at time.Unix(7), 8, and 9
			ciStatus:        SuccessStatus(),
			lastBuildNumber: LastBuildNumber(),
			gcsResult:       FailGCS(),
			retest1Pass:     false,
			retest2Pass:     true,
			reason:          ghE2EFailed,
			state:           "pending",
		},
	}
	for testNum := range tests {
		test := &tests[testNum]
		t.Logf("---------Starting test %v (%v)---------------------", testNum, test.name)
		issueNum := testNum + 1
		issueNumStr := strconv.Itoa(issueNum)

		test.issue.Number = &issueNum
		client, server, mux := github_test.InitServer(t, test.issue, test.pr, test.events, test.commits, test.ciStatus, test.masterCommit, nil)

		config := &github_util.Config{Org: "o", Project: "r"}
		config.SetClient(client)
		// Don't wait so long for retries (pending, mergeability)
		config.BaseWaitTime = time.Millisecond

		stateSet := ""
		wasMerged := false

		for _, job := range someJobNames {
			numTestChecks := 0
			var testChecksLock sync.Mutex
			path := fmt.Sprintf("/bucket/logs/%s/latest-build.txt", job)
			mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("Unexpected method: %s", r.Method)
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(strconv.Itoa(test.lastBuildNumber)))

				// There is no good spot for this, but this gets called
				// before we queue the PR. So mark the PR as "merged".
				// When the sq initializes, it will check the Jenkins status,
				// so we don't want to modify the PR there. Instead we need
				// to wait until the second time we check Jenkins, which happens
				// we did the IsMerged() check.
				testChecksLock.Lock()
				defer testChecksLock.Unlock()
				numTestChecks = numTestChecks + 1
				if numTestChecks == 2 && test.mergeAfterQueued {
					test.pr.Merged = boolPtr(true)
					test.pr.Mergeable = nil
				}
			})
			path = fmt.Sprintf("/bucket/logs/%s/%v/finished.json", job, test.lastBuildNumber)
			mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("Unexpected method: %s", r.Method)
				}
				w.WriteHeader(http.StatusOK)
				data, err := json.Marshal(test.gcsResult)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				w.Write(data)
			})
		}

		path := fmt.Sprintf("/repos/o/r/issues/%d/comments", issueNum)
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" {
				c := new(github.IssueComment)
				json.NewDecoder(r.Body).Decode(c)
				msg := *c.Body
				if strings.HasPrefix(msg, "/test all") {
					go fakeRunGithubE2ESuccess(test.ciStatus, test.retest1Pass, test.retest2Pass)
				}
				w.WriteHeader(http.StatusOK)
				data, err := json.Marshal(github.IssueComment{})
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				w.Write(data)
				return
			}
			if r.Method == "GET" {
				w.WriteHeader(http.StatusOK)
				data, err := json.Marshal([]github.IssueComment{})
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				w.Write(data)
				return
			}
			t.Errorf("Unexpected method: %s", r.Method)
		})
		path = fmt.Sprintf("/repos/o/r/pulls/%d/merge", issueNum)
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
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
			wasMerged = true
		})
		path = fmt.Sprintf("/repos/o/r/statuses/%s", *test.pr.Head.SHA)
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("Unexpected method: %s", r.Method)
			}
			decoder := json.NewDecoder(r.Body)
			var status github.RepoStatus
			err := decoder.Decode(&status)
			if err != nil {
				t.Errorf("Unable to decode status: %v", err)
			}

			stateSet = *status.State

			data, err := json.Marshal(status)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			w.Write(data)
		})

		sq := getTestSQ(true, config, server)
		sq.setEmergencyMergeStop(test.emergencyMergeStop)
		sq.AdditionalRequiredLabels = test.additionalLabels
		sq.BlockingLabels = test.blockingLabels

		obj := github_util.NewTestObject(config, test.issue, test.pr, test.commits, test.events)
		if test.imBaseSHA != "" && test.imHeadSHA != "" {
			sq.interruptedObj = &submitQueueInterruptedObject{obj, test.imHeadSHA, test.imBaseSHA}
		}
		sq.Munge(obj)
		done := make(chan bool, 1)
		go func(done chan bool) {
			for {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%d:%q panic'd likely writing to 'done' channel", testNum, test.name)
					}
				}()

				reason := func() string {
					sq.Mutex.Lock()
					defer sq.Mutex.Unlock()
					return sq.prStatus[issueNumStr].Reason
				}

				if reason() == test.reason {
					done <- true
					return
				}
				found := false
				sq.Lock()
				for _, status := range sq.statusHistory {
					if status.Reason == test.reason {
						found = true
						break
					}
				}
				sq.Unlock()
				if found {
					done <- true
					return
				}
				time.Sleep(1 * time.Millisecond)
			}
		}(done)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Errorf("%d:%q timed out waiting expected reason=%q but got prStatus:%q history:%v", testNum, test.name, test.reason, sq.prStatus[issueNumStr].Reason, sq.statusHistory)
		}
		close(done)
		server.Close()

		if test.state != "" && test.state != stateSet {
			t.Errorf("%d:%q state set to %q but expected %q", testNum, test.name, stateSet, test.state)
		}
		if test.isMerged != wasMerged {
			t.Errorf("%d:%q PR merged = %v but wanted %v", testNum, test.name, wasMerged, test.isMerged)
		}
		if e, a := test.retestsAvoided, int(sq.retestsAvoided); e != a {
			t.Errorf("%d:%q expected %v tests avoided but got %v", testNum, test.name, e, a)
		}
	}
}

func TestCalcMergeRate(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	tests := []struct {
		name     string // because when it fails, counting is hard
		preRate  float64
		interval time.Duration
		expected func(float64) bool
	}{
		{
			name:     "0One",
			preRate:  0,
			interval: time.Duration(time.Hour),
			expected: func(rate float64) bool {
				return rate > 10 && rate < 11
			},
		},
		{
			name:     "24One",
			preRate:  24,
			interval: time.Duration(time.Hour),
			expected: func(rate float64) bool {
				return rate == float64(24)
			},
		},
		{
			name:     "24Two",
			preRate:  24,
			interval: time.Duration(2 * time.Hour),
			expected: func(rate float64) bool {
				return rate > 17 && rate < 18
			},
		},
		{
			name:     "24HalfHour",
			preRate:  24,
			interval: time.Duration(time.Hour) / 2,
			expected: func(rate float64) bool {
				return rate > 31 && rate < 32
			},
		},
		{
			name:     "24Three",
			preRate:  24,
			interval: time.Duration(3 * time.Hour),
			expected: func(rate float64) bool {
				return rate > 14 && rate < 15
			},
		},
		{
			name:     "24Then24",
			preRate:  24,
			interval: time.Duration(24 * time.Hour),
			expected: func(rate float64) bool {
				return rate > 2 && rate < 3
			},
		},
		{
			name:     "24fast",
			preRate:  24,
			interval: time.Duration(4 * time.Minute),
			expected: func(rate float64) bool {
				// Should be no change
				return rate == 24
			},
		},
	}
	for testNum, test := range tests {
		sq := getTestSQ(false, nil, nil)
		clock := sq.clock.(*utilclock.FakeClock)
		sq.mergeRate = test.preRate
		clock.Step(test.interval)
		sq.updateMergeRate()
		if !test.expected(sq.mergeRate) {
			t.Errorf("%d:%s: expected() failed: rate:%v", testNum, test.name, sq.mergeRate)
		}
	}
}

func TestCalcMergeRateWithTail(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	tests := []struct {
		name     string // because when it fails, counting is hard
		preRate  float64
		interval time.Duration
		expected func(float64) bool
	}{
		{
			name:     "ZeroPlusZero",
			preRate:  0,
			interval: time.Duration(0),
			expected: func(rate float64) bool {
				return rate == float64(0)
			},
		},
		{
			name:     "0OneHour",
			preRate:  0,
			interval: time.Duration(time.Hour),
			expected: func(rate float64) bool {
				return rate == 0
			},
		},
		{
			name:     "TinyOneHour",
			preRate:  .001,
			interval: time.Duration(time.Hour),
			expected: func(rate float64) bool {
				return rate == .001
			},
		},
		{
			name:     "TwentyFourPlusHalfHour",
			preRate:  24,
			interval: time.Duration(time.Hour) / 2,
			expected: func(rate float64) bool {
				return rate == 24
			},
		},
		{
			name:     "TwentyFourPlusOneHour",
			preRate:  24,
			interval: time.Duration(time.Hour),
			expected: func(rate float64) bool {
				return rate == 24
			},
		},
		{
			name:     "TwentyFourPlusTwoHour",
			preRate:  24,
			interval: time.Duration(2 * time.Hour),
			expected: func(rate float64) bool {
				return rate > 17 && rate < 18
			},
		},
		{
			name:     "TwentyFourPlusFourHour",
			preRate:  24,
			interval: time.Duration(4 * time.Hour),
			expected: func(rate float64) bool {
				return rate > 12 && rate < 13
			},
		},
		{
			name:     "TwentyFourPlusTwentyFourHour",
			preRate:  24,
			interval: time.Duration(24 * time.Hour),
			expected: func(rate float64) bool {
				return rate > 2 && rate < 3
			},
		},
		{
			name:     "TwentyFourPlusTiny",
			preRate:  24,
			interval: time.Duration(time.Nanosecond),
			expected: func(rate float64) bool {
				return rate == 24
			},
		},
		{
			name:     "TwentyFourPlusHuge",
			preRate:  24,
			interval: time.Duration(1024 * time.Hour),
			expected: func(rate float64) bool {
				return rate > 0 && rate < 1
			},
		},
	}
	for testNum, test := range tests {
		sq := getTestSQ(false, nil, nil)
		sq.mergeRate = test.preRate
		clock := sq.clock.(*utilclock.FakeClock)
		clock.Step(test.interval)
		rate := sq.calcMergeRateWithTail()
		if !test.expected(rate) {
			t.Errorf("%d:%s: %v", testNum, test.name, rate)
		}
	}
}

func TestHealth(t *testing.T) {
	sq := getTestSQ(false, nil, nil)
	sq.updateHealth()
	sq.updateHealth()
	if len(sq.healthHistory) != 2 {
		t.Errorf("Wrong length healthHistory after calling updateHealth: %v", sq.healthHistory)
	}
	if sq.health.TotalLoops != 2 || sq.health.NumStable != 2 || len(sq.health.NumStablePerJob) != 2 {
		t.Errorf("Wrong number of stable loops after calling updateHealth: %v", sq.health)
	}
	for _, stable := range sq.health.NumStablePerJob {
		if stable != 2 {
			t.Errorf("Wrong number of stable loops for a job: %v", sq.health.NumStablePerJob)
		}
	}
	sq.healthHistory[0].Time = time.Now().AddDate(0, 0, -3)
	sq.healthHistory[1].Time = time.Now().AddDate(0, 0, -2)
	sq.updateHealth()
	if len(sq.healthHistory) != 1 {
		t.Errorf("updateHealth didn't truncate old entries: %v", sq.healthHistory)
	}
}

func TestHealthSVG(t *testing.T) {
	sq := getTestSQ(false, nil, nil)
	e2e := sq.e2e.(*fake_e2e.FakeE2ETester)

	for _, state := range []struct {
		mergePossible bool
		expected      string
		notStable     []string
	}{
		{true, "running", nil},
		{false, "blocked</text>", nil},
		{false, "blocked by kubemark-500", []string{"kubernetes-kubemark-500"}},
		{false, "blocked by a, b, c, ...", []string{"a", "b", "c", "d"}},
	} {
		sq.health.MergePossibleNow = state.mergePossible
		e2e.NotStableJobNames = state.notStable
		res := string(sq.getHealthSVG())
		if !strings.Contains(res, state.expected) {
			t.Errorf("SVG doesn't contain `%s`: %v", state.expected, res)
		}
	}
}
