/*
Copyright 2018 The Kubernetes Authors.

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

package adapter

import (
	"errors"
	"fmt"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/andygrunwald/go-gerrit"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
	clienttesting "k8s.io/client-go/testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowfake "k8s.io/test-infra/prow/client/clientset/versioned/fake"
	"k8s.io/test-infra/prow/config"
	reporter "k8s.io/test-infra/prow/crier/reporters/gerrit"
	"k8s.io/test-infra/prow/gerrit/client"
	"k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/kube"
)

func makeStamp(t time.Time) gerrit.Timestamp {
	return gerrit.Timestamp{Time: t}
}

var (
	timeNow   = time.Date(1234, time.May, 15, 1, 2, 3, 4, time.UTC)
	stampNow  = makeStamp(timeNow)
	trueBool  = true
	namespace = "default"
)

type fca struct {
	sync.Mutex
	c *config.Config
}

func (f *fca) Config() *config.Config {
	f.Lock()
	defer f.Unlock()
	return f.c
}

type fgc struct {
	reviews     int
	instanceMap map[string]*gerrit.AccountInfo
}

func (f *fgc) HasRelatedChanges(instance, id, revision string) (bool, error) {
	return false, nil
}

func (f *fgc) ApplyGlobalConfig(orgRepoConfigGetter func() *config.GerritOrgRepoConfigs, lastSyncTracker *client.SyncTime, cookiefilePath, tokenPathOverride string, additionalFunc func()) {

}

func (f *fgc) Authenticate(cookiefilePath, tokenPath string) {

}

func (f *fgc) QueryChanges(lastUpdate client.LastSyncState, rateLimit int) map[string][]client.ChangeInfo {
	return nil
}

func (f *fgc) QueryChangesForProject(instance, project string, lastUpdate time.Time, rateLimit int, additionalFilters ...string) ([]gerrit.ChangeInfo, error) {
	return nil, nil
}

func (f *fgc) SetReview(instance, id, revision, message string, labels map[string]string) error {
	f.reviews++
	return nil
}

func (f *fgc) GetBranchRevision(instance, project, branch string) (string, error) {
	return "abc", nil
}

func (f *fgc) Account(instance string) (*gerrit.AccountInfo, error) {
	res, ok := f.instanceMap[instance]
	if !ok {
		return nil, errors.New("not exit")
	}
	return res, nil
}

func fakeProwYAMLGetter(
	c *config.Config,
	gc git.ClientFactory,
	identifier string,
	baseBranch string,
	baseSHA string,
	headSHAs ...string) (*config.ProwYAML, error) {

	presubmits := []config.Presubmit{
		{
			JobBase: config.JobBase{
				Name:      "always-runs-inRepoConfig",
				Spec:      &v1.PodSpec{Containers: []v1.Container{{Name: "always-runs-inRepoConfig", Env: []v1.EnvVar{}}}},
				Namespace: &namespace,
			},
			Brancher: config.Brancher{
				Branches: []string{"inRepoConfig"},
			},
			AlwaysRun: true,
			Reporter: config.Reporter{
				Context:    "always-runs-inRepoConfig",
				SkipReport: true,
			},
		},
	}
	postsubmits := []config.Postsubmit{
		{
			JobBase: config.JobBase{
				Name:      "always-runs-inRepoConfig-Post",
				Spec:      &v1.PodSpec{Containers: []v1.Container{{Name: "always-runs-inRepoConfig-Post", Env: []v1.EnvVar{}}}},
				Namespace: &namespace,
			},
			Brancher: config.Brancher{
				Branches: []string{"inRepoConfig"},
			},
			AlwaysRun: &trueBool,
			Reporter: config.Reporter{
				Context:    "always-runs-inRepoConfig-Post",
				SkipReport: true,
			},
		},
	}
	if err := config.SetPostsubmitRegexes(postsubmits); err != nil {
		return nil, err
	}
	if err := config.SetPresubmitRegexes(presubmits); err != nil {
		return nil, err
	}
	res := config.ProwYAML{
		Presubmits:  presubmits,
		Postsubmits: postsubmits,
	}
	return &res, nil
}

func TestShouldTriggerJobs(t *testing.T) {
	now := time.Now()
	instance := "gke-host"
	project := "private-cloud"
	var lastUpdateTime = now.Add(-time.Hour)
	c := &Controller{
		configAgent: &config.Agent{},
	}
	presubmitTriggerRawString := "(?mi)/test\\s.*"
	c.configAgent.Set(&config.Config{ProwConfig: config.ProwConfig{Gerrit: config.Gerrit{AllowedPresubmitTriggerReRawString: presubmitTriggerRawString}}})
	presubmitTriggerRegex, err := regexp.Compile(presubmitTriggerRawString)
	if err != nil {
		t.Fatalf("failed to compile regex for allowed presubmit triggers: %s", err.Error())
	}
	c.configAgent.Config().Gerrit.AllowedPresubmitTriggerRe = &config.CopyableRegexp{Regexp: presubmitTriggerRegex}
	cases := []struct {
		name     string
		instance string
		change   gerrit.ChangeInfo
		latest   time.Time
		result   bool
	}{
		{
			name:     "trigger jobs when revision is new",
			instance: instance,
			change: gerrit.ChangeInfo{ID: "1", CurrentRevision: "10", Project: project,
				Revisions: map[string]gerrit.RevisionInfo{
					"10": {Created: makeStamp(now)},
				}},
			latest: lastUpdateTime,
			result: true,
		},
		{
			name:     "trigger jobs when comment contains test related commands",
			instance: instance,
			change: gerrit.ChangeInfo{ID: "1", CurrentRevision: "10", Project: project,
				Revisions: map[string]gerrit.RevisionInfo{
					"10": {Number: 10, Created: makeStamp(now.Add(-2 * time.Hour))},
				}, Messages: []gerrit.ChangeMessageInfo{
					{
						Date:           makeStamp(now),
						Message:        "/test all",
						RevisionNumber: 10,
					},
				}},
			latest: lastUpdateTime,
			result: true,
		},
		{
			name:     "trigger jobs when comment contains /test with custom test name",
			instance: instance,
			change: gerrit.ChangeInfo{ID: "1", CurrentRevision: "10", Project: project,
				Revisions: map[string]gerrit.RevisionInfo{
					"10": {Number: 10, Created: makeStamp(now.Add(-2 * time.Hour))},
				}, Messages: []gerrit.ChangeMessageInfo{
					{
						Date:           makeStamp(now),
						Message:        "/test integration",
						RevisionNumber: 10,
					},
				}},
			latest: lastUpdateTime,
			result: true,
		},
		{
			name:     "do not trigger when command does not conform to requirements",
			instance: instance,
			change: gerrit.ChangeInfo{ID: "1", CurrentRevision: "10", Project: project,
				Revisions: map[string]gerrit.RevisionInfo{
					"10": {Number: 10, Created: makeStamp(now.Add(-2 * time.Hour))},
				}, Messages: []gerrit.ChangeMessageInfo{
					{
						Date:           makeStamp(now),
						Message:        "LGTM",
						RevisionNumber: 10,
					},
				}},
			latest: lastUpdateTime,
			result: false,
		},
		{
			name:     "trigger jobs for merge events (in order to trigger postsubmit jobs)",
			instance: instance,
			change:   gerrit.ChangeInfo{Status: client.Merged},
			latest:   lastUpdateTime,
			result:   true,
		},
		{
			name:     "trigger jobs for previously-seen change coming out of WIP status (via ReadyForReviewMessageFixed)",
			instance: instance,
			change: gerrit.ChangeInfo{ID: "1", CurrentRevision: "10", Project: project,
				Revisions: map[string]gerrit.RevisionInfo{
					"10": {
						Number: 10,
						// The associated revision is old (predates
						// lastUpdateTime)...
						Created: makeStamp(now.Add(-2 * time.Hour))},
				}, Messages: []gerrit.ChangeMessageInfo{
					{
						Date: makeStamp(now),
						// ...but we shouldn't skip triggering jobs for it
						// because the message says this is no longer WIP.
						Message:        client.ReadyForReviewMessageFixed,
						RevisionNumber: 10,
					},
				}},
			latest: lastUpdateTime,
			result: true,
		},
		{
			name:     "trigger jobs for previously-seen change coming out of WIP status (via ReadyForReviewMessageCustomizable)",
			instance: instance,
			change: gerrit.ChangeInfo{ID: "1", CurrentRevision: "10", Project: project,
				Revisions: map[string]gerrit.RevisionInfo{
					"10": {
						Number: 10,
						// The associated revision is old (predates
						// lastUpdateTime)...
						Created: makeStamp(now.Add(-2 * time.Hour))},
				}, Messages: []gerrit.ChangeMessageInfo{
					{
						Date: makeStamp(now),
						// ...but we shouldn't skip triggering jobs for it
						// because the message says this is no longer WIP.
						Message:        client.ReadyForReviewMessageCustomizable,
						RevisionNumber: 10,
					},
				}},
			latest: lastUpdateTime,
			result: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.shouldTriggerJobs(tc.change, tc.latest); got != tc.result {
				t.Errorf("want %t, got %t", tc.result, got)
			}
		})
	}
}

func TestHandleInRepoConfigError(t *testing.T) {
	change := gerrit.ChangeInfo{ID: "1", CurrentRevision: "1"}
	instanceName := "instance"
	changeHash := fmt.Sprintf("%s%s%s", instanceName, change.ID, change.CurrentRevision)
	cases := []struct {
		name                      string
		err                       error
		allowedPresubmitTriggerRe string
		startingFailures          map[string]bool
		expectedFailures          map[string]bool
		expectedReview            bool
	}{
		{
			name:             "No error. Do not send message",
			expectedReview:   false,
			startingFailures: map[string]bool{},
			expectedFailures: map[string]bool{},
			err:              nil,
		},
		{
			name:             "First time (or previously resolved) error send review",
			err:              errors.New("InRepoConfigError"),
			expectedReview:   true,
			startingFailures: map[string]bool{},
			expectedFailures: map[string]bool{changeHash: true},
		},
		{
			name:             "second time error do not send review",
			err:              errors.New("InRepoConfigError"),
			expectedReview:   false,
			startingFailures: map[string]bool{changeHash: true},
			expectedFailures: map[string]bool{changeHash: true},
		},
		{
			name:             "Resolved error changes Failures map",
			err:              nil,
			expectedReview:   false,
			startingFailures: map[string]bool{changeHash: true},
			expectedFailures: map[string]bool{},
		},
		{
			name:                      "second time error DO send review if irrelevant changes are being skipped",
			err:                       errors.New("InRepoConfigError"),
			allowedPresubmitTriggerRe: "^/test.*",
			expectedReview:            true,
			startingFailures:          map[string]bool{changeHash: true},
			expectedFailures:          map[string]bool{changeHash: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gc := &fgc{reviews: 0}
			gerritConfig := config.Gerrit{AllowedPresubmitTriggerReRawString: tc.allowedPresubmitTriggerRe}
			if err := gerritConfig.DefaultAndValidate(); err != nil {
				t.Fatalf("Failed to default and validate the gerrit config: %v", err)
			}
			controller := &Controller{
				inRepoConfigFailuresTracker: tc.startingFailures,
				gc:                          gc,
				config:                      func() *config.Config { return &config.Config{ProwConfig: config.ProwConfig{Gerrit: gerritConfig}} },
			}

			ret := controller.handleInRepoConfigError(tc.err, instanceName, change)
			if ret != nil {
				t.Errorf("handleInRepoConfigError returned with non nil error")
			}
			if tc.expectedReview && gc.reviews == 0 {
				t.Errorf("expected a review and did not get one")
			}
			if !tc.expectedReview && gc.reviews != 0 {
				t.Error("expected no reviews and got one")
			}
			if diff := cmp.Diff(tc.expectedFailures, controller.inRepoConfigFailuresTracker, cmpopts.SortSlices(func(a, b string) bool {
				return a < b
			})); diff != "" {
				t.Fatalf("expected failures mismatch. got(+), want(-):\n%s", diff)
			}
		})
	}
}

type fakeSync struct {
	val  client.LastSyncState
	lock sync.Mutex
}

func (s *fakeSync) Current() client.LastSyncState {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.val
}

func (s *fakeSync) Update(t client.LastSyncState) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.val = t
	return nil
}

func TestCreateRefs(t *testing.T) {
	reviewHost := "https://cat-review.example.com"
	change := client.ChangeInfo{
		Number:          42,
		Project:         "meow/purr",
		CurrentRevision: "123456",
		Branch:          "master",
		Revisions: map[string]client.RevisionInfo{
			"123456": {
				Ref: "refs/changes/00/1/1",
				Commit: gerrit.CommitInfo{
					Author: gerrit.GitPersonInfo{
						Name:  "Some Cat",
						Email: "nyan@example.com",
					},
				},
			},
		},
	}
	expected := prowapi.Refs{
		Org:      "https://cat-review.example.com",
		Repo:     "meow/purr",
		BaseRef:  "master",
		BaseSHA:  "abcdef",
		CloneURI: "https://cat-review.example.com/meow/purr",
		RepoLink: "https://cat.example.com/meow/purr",
		BaseLink: "https://cat.example.com/meow/purr/+/abcdef",
		Pulls: []prowapi.Pull{
			{
				Number:     42,
				Author:     "Some Cat",
				SHA:        "123456",
				Ref:        "refs/changes/00/1/1",
				Link:       "https://cat-review.example.com/c/meow/purr/+/42",
				CommitLink: "https://cat.example.com/meow/purr/+/123456",
				AuthorLink: "https://cat-review.example.com/q/nyan@example.com",
			},
		},
	}
	actual, err := CreateRefs(reviewHost, change.Project, change.Branch, "abcdef", change)
	if err != nil {
		t.Errorf("unexpected error creating refs: %v", err)
	}
	if !equality.Semantic.DeepEqual(expected, actual) {
		t.Errorf("diff between expected and actual refs:%s", diff.ObjectReflectDiff(expected, actual))
	}
}

func TestFailedJobs(t *testing.T) {
	const (
		me      = 314159
		stan    = 666
		current = 555
		old     = 4
	)
	now := time.Now()
	message := func(msg string, patch func(*gerrit.ChangeMessageInfo)) gerrit.ChangeMessageInfo {
		var out gerrit.ChangeMessageInfo
		out.Author.AccountID = me
		out.Message = msg
		out.RevisionNumber = current
		out.Date.Time = now
		now = now.Add(time.Minute)
		if patch != nil {
			patch(&out)
		}
		return out
	}

	report := func(jobs map[string]prowapi.ProwJobState) string {
		var pjs []*prowapi.ProwJob
		for name, state := range jobs {
			var pj prowapi.ProwJob
			pj.Spec.Job = name
			pj.Status.State = state
			pj.Status.URL = "whatever"
			pjs = append(pjs, &pj)
		}
		return reporter.GenerateReport(pjs, 0).String()
	}

	cases := []struct {
		name     string
		messages []gerrit.ChangeMessageInfo
		expected sets.Set[string]
	}{
		{
			name: "basically works",
		},
		{
			name: "report parses",
			messages: []gerrit.ChangeMessageInfo{
				message("ignore this", nil),
				message(report(map[string]prowapi.ProwJobState{
					"foo":          prowapi.SuccessState,
					"should-fail":  prowapi.FailureState,
					"should-abort": prowapi.AbortedState,
				}), nil),
				message("also ignore this", nil),
			},
			expected: sets.New[string]("should-fail", "should-abort"),
		},
		{
			name: "ignore report from someone else",
			messages: []gerrit.ChangeMessageInfo{
				message(report(map[string]prowapi.ProwJobState{
					"foo":                  prowapi.SuccessState,
					"ignore-their-failure": prowapi.FailureState,
				}), func(msg *gerrit.ChangeMessageInfo) {
					msg.Author.AccountID = stan
				}),
				message(report(map[string]prowapi.ProwJobState{
					"whatever":    prowapi.SuccessState,
					"should-fail": prowapi.FailureState,
				}), nil),
			},
			expected: sets.New[string]("should-fail"),
		},
		{
			name: "ignore failures on other revisions",
			messages: []gerrit.ChangeMessageInfo{
				message(report(map[string]prowapi.ProwJobState{
					"current-pass": prowapi.SuccessState,
					"current-fail": prowapi.FailureState,
				}), nil),
				message(report(map[string]prowapi.ProwJobState{
					"old-pass": prowapi.SuccessState,
					"old-fail": prowapi.FailureState,
				}), func(msg *gerrit.ChangeMessageInfo) {
					msg.RevisionNumber = old
				}),
			},
			expected: sets.New[string]("current-fail"),
		},
		{
			name: "ignore jobs in my earlier report",
			messages: []gerrit.ChangeMessageInfo{
				message(report(map[string]prowapi.ProwJobState{
					"failed-then-pass": prowapi.FailureState,
					"old-broken":       prowapi.FailureState,
					"old-pass":         prowapi.SuccessState,
					"pass-then-failed": prowapi.SuccessState,
					"still-fail":       prowapi.FailureState,
					"still-pass":       prowapi.SuccessState,
				}), nil),
				message(report(map[string]prowapi.ProwJobState{
					"failed-then-pass": prowapi.SuccessState,
					"new-broken":       prowapi.FailureState,
					"new-pass":         prowapi.SuccessState,
					"pass-then-failed": prowapi.FailureState,
					"still-fail":       prowapi.FailureState,
					"still-pass":       prowapi.SuccessState,
				}), nil),
			},
			expected: sets.New[string]("old-broken", "new-broken", "still-fail", "pass-then-failed"),
		},
		{
			// https://en.wikipedia.org/wiki/Gravitational_redshift
			name: "handle gravitationally redshifted results",
			messages: []gerrit.ChangeMessageInfo{
				message(report(map[string]prowapi.ProwJobState{
					"earth-broken":              prowapi.FailureState,
					"earth-pass":                prowapi.SuccessState,
					"fail-earth-pass-blackhole": prowapi.FailureState,
					"pass-earth-fail-blackhole": prowapi.SuccessState,
				}), nil),
				message(report(map[string]prowapi.ProwJobState{
					"blackhole-broken":          prowapi.FailureState,
					"blackhole-pass":            prowapi.SuccessState,
					"fail-earth-pass-blackhole": prowapi.SuccessState,
					"pass-earth-fail-blackhole": prowapi.FailureState,
				}), func(change *gerrit.ChangeMessageInfo) {
					change.Date.Time = change.Date.Time.Add(-time.Hour)
				}),
			},
			expected: sets.New[string]("earth-broken", "blackhole-broken", "fail-earth-pass-blackhole"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual, expected := failedJobs(me, current, tc.messages...), tc.expected; !equality.Semantic.DeepEqual(expected, actual) {
				t.Errorf(diff.ObjectReflectDiff(expected, actual))
			}
		})
	}
}

func createTestRepoCache(t *testing.T, ca *fca) (*config.InRepoConfigCache, error) {
	// triggerJobs takes a ClientFactory. If provided a nil clientFactory it will skip inRepoConfig
	// otherwise it will get the prow yaml using the client provided. We are mocking ProwYamlGetter
	// so we are creating a localClientFactory but leaving it unpopulated.
	var cf git.ClientFactory
	var lg *localgit.LocalGit
	lg, cf, err := localgit.NewV2()
	if err != nil {
		return nil, fmt.Errorf("error making local git repo: %v", err)
	}
	defer func() {
		if err := lg.Clean(); err != nil {
			t.Errorf("Error cleaning LocalGit: %v", err)
		}
		if err := cf.Clean(); err != nil {
			t.Errorf("Error cleaning Client: %v", err)
		}
	}()

	// Initialize cache for fetching Presubmit and Postsubmit information. If
	// the cache cannot be initialized, exit with an error.
	cache, err := config.NewInRepoConfigCache(10, ca, cf)
	if err != nil {
		t.Errorf("error creating cache: %v", err)
	}
	return cache, nil
}

func TestTriggerJobs(t *testing.T) {
	testInstance := "https://gerrit"
	var testcases = []struct {
		name           string
		change         client.ChangeInfo
		instancesMap   map[string]*gerrit.AccountInfo
		instance       string
		wantError      bool
		wantSkipReport bool
		wantPjs        []*prowapi.ProwJob
	}{
		{
			name: "no presubmit Prow jobs automatically triggered from WorkInProgess change",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Status:          "NEW",
				WorkInProgress:  true,
				Revisions: map[string]gerrit.RevisionInfo{
					"1": {
						Number: 1001,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
		},
		{
			name: "no revisions errors out",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Status:          "NEW",
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantError:    true,
		},
		{
			name: "wrong project triggers no jobs",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "woof",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Created: stampNow,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
		},
		{
			name: "normal changes should trigger matching branch jobs",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Ref:     "refs/changes/00/1/1",
						Created: stampNow,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/job":                 "always-runs-all-branches",
							"prow.k8s.io/context":             "always-runs-all-branches",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/gerrit-revision":     "1",
							"created-by-prow":                 "true",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/refs.base_ref":       "",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/refs.org":            "gerrit",
						},
						Annotations: map[string]string{
							"prow.k8s.io/job":             "always-runs-all-branches",
							"prow.k8s.io/context":         "always-runs-all-branches",
							"foo":                         "bar",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/1/1",
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/job":                 "runs-on-all-but-baz-branch",
							"prow.k8s.io/context":             "runs-on-all-but-baz-branch",
							"prow.k8s.io/refs.base_ref":       "",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"created-by-prow":                 "true",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/refs.org":            "gerrit",
						},
						Annotations: map[string]string{
							"prow.k8s.io/context":         "runs-on-all-but-baz-branch",
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/job":             "runs-on-all-but-baz-branch",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/1/1",
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "instance not registered",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Ref:     "refs/changes/00/1/1",
						Created: stampNow,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance + "_notexist",
			wantError:    true,
		},
		{
			name: "jobs should trigger with correct labels",
			change: client.ChangeInfo{
				CurrentRevision: "rev42",
				Project:         "test-infra",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"rev42": {
						Ref:     "refs/changes/00/1/1",
						Created: stampNow,
						Number:  42,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/context":             "always-runs-all-branches",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/refs.base_ref":       "",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/gerrit-patchset":     "42",
							"prow.k8s.io/gerrit-revision":     "rev42",
							"created-by-prow":                 "true",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/job":                 "always-runs-all-branches",
						},
						Annotations: map[string]string{
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/job":             "always-runs-all-branches",
							"prow.k8s.io/context":         "always-runs-all-branches",
							"foo":                         "bar",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/1/1",
									SHA:        "rev42",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/rev42",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/refs.base_ref":       "",
							"prow.k8s.io/refs.pull":           "0",
							"created-by-prow":                 "true",
							"prow.k8s.io/context":             "runs-on-all-but-baz-branch",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/job":                 "runs-on-all-but-baz-branch",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/gerrit-revision":     "rev42",
							"prow.k8s.io/gerrit-patchset":     "42",
							"prow.k8s.io/type":                "presubmit",
						},
						Annotations: map[string]string{
							"prow.k8s.io/context":         "runs-on-all-but-baz-branch",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/job":             "runs-on-all-but-baz-branch",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/1/1",
									SHA:        "rev42",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/rev42",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "multiple revisions",
			change: client.ChangeInfo{
				CurrentRevision: "2",
				Project:         "test-infra",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Ref:     "refs/changes/00/2/1",
						Created: stampNow,
					},
					"2": {
						Ref:     "refs/changes/00/2/2",
						Created: stampNow,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/job":                 "always-runs-all-branches",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/context":             "always-runs-all-branches",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/gerrit-revision":     "2",
							"created-by-prow":                 "true",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.base_ref":       "",
						},
						Annotations: map[string]string{
							"prow.k8s.io/job":             "always-runs-all-branches",
							"prow.k8s.io/context":         "always-runs-all-branches",
							"prow.k8s.io/gerrit-id":       "",
							"foo":                         "bar",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/2/2",
									SHA:        "2",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/2",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/refs.pull":           "0",
							"created-by-prow":                 "true",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/context":             "runs-on-all-but-baz-branch",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/job":                 "runs-on-all-but-baz-branch",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/gerrit-revision":     "2",
							"prow.k8s.io/refs.base_ref":       "",
						},
						Annotations: map[string]string{
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/job":             "runs-on-all-but-baz-branch",
							"prow.k8s.io/context":         "runs-on-all-but-baz-branch",
							"prow.k8s.io/gerrit-id":       "",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/2/2",
									SHA:        "2",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/2",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "other-test-with-https",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "other-repo",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Ref:     "refs/changes/00/1/1",
						Created: stampNow,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/context":             "other-test",
							"prow.k8s.io/refs.pull":           "0",
							"created-by-prow":                 "true",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/refs.base_ref":       "",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/refs.repo":           "other-repo",
							"prow.k8s.io/job":                 "other-test",
							"prow.k8s.io/refs.org":            "gerrit",
						},
						Annotations: map[string]string{
							"prow.k8s.io/job":             "other-test",
							"prow.k8s.io/context":         "other-test",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "other-repo",
							RepoLink: "https://gerrit/other-repo",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/other-repo/+/abc",
							CloneURI: "https://gerrit/other-repo",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/1/1",
									SHA:        "1",
									Link:       "https://gerrit/c/other-repo/+/0",
									CommitLink: "https://gerrit/other-repo/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "merged change should trigger postsubmit",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "postsubmits-project",
				Status:          "MERGED",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Ref:     "refs/changes/00/1/1",
						Created: stampNow,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"created-by-prow":                 "true",
							"prow.k8s.io/context":             "test-bar",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/job":                 "test-bar",
							"prow.k8s.io/refs.base_ref":       "",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/refs.repo":           "postsubmits-project",
							"prow.k8s.io/type":                "postsubmit",
						},
						Annotations: map[string]string{
							"prow.k8s.io/context":         "test-bar",
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/job":             "test-bar",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "postsubmits-project",
							RepoLink: "https://gerrit/postsubmits-project",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/postsubmits-project/+/abc",
							CloneURI: "https://gerrit/postsubmits-project",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/1/1",
									SHA:        "1",
									Link:       "https://gerrit/c/postsubmits-project/+/0",
									CommitLink: "https://gerrit/postsubmits-project/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "merged change on project without postsubmits",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Status:          "MERGED",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Ref:     "refs/changes/00/1/1",
						Created: stampNow,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
		},
		{
			name: "presubmit runs when a file matches run_if_changed",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Files: map[string]client.FileInfo{
							"bee-movie-script.txt": {},
							"africa-lyrics.txt":    {},
							"important-code.go":    {},
						},
						Created: stampNow,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/job":                 "always-runs-all-branches",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/refs.base_ref":       "",
							"created-by-prow":                 "true",
							"prow.k8s.io/context":             "always-runs-all-branches",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/type":                "presubmit",
						},
						Annotations: map[string]string{
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
							"foo":                         "bar",
							"prow.k8s.io/context":         "always-runs-all-branches",
							"prow.k8s.io/job":             "always-runs-all-branches",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/job":                 "run-if-changed-all-branches",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/type":                "presubmit",
							"created-by-prow":                 "true",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/context":             "run-if-changed-all-branches",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/refs.base_ref":       "",
						},
						Annotations: map[string]string{
							"prow.k8s.io/job":             "run-if-changed-all-branches",
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/context":         "run-if-changed-all-branches",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/refs.base_ref":       "",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/job":                 "runs-on-all-but-baz-branch",
							"created-by-prow":                 "true",
							"prow.k8s.io/context":             "runs-on-all-but-baz-branch",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/gerrit-revision":     "1",
						},
						Annotations: map[string]string{
							"prow.k8s.io/job":             "runs-on-all-but-baz-branch",
							"prow.k8s.io/context":         "runs-on-all-but-baz-branch",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "presubmit runs when a file matching run_if_changed is renamed",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Files: map[string]client.FileInfo{
							"bee-movie-script.txt": {},
							"just-a-copy.bar":      {Status: "C", OldPath: "important.go"},  // Copied file doesn't affect original
							"important.bar":        {Status: "R", OldPath: "important.foo"}, // Renamed file affects original
						},
						Created: stampNow,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/job":                 "always-runs-all-branches",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/refs.base_ref":       "",
							"created-by-prow":                 "true",
							"prow.k8s.io/context":             "always-runs-all-branches",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/type":                "presubmit",
						},
						Annotations: map[string]string{
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
							"foo":                         "bar",
							"prow.k8s.io/context":         "always-runs-all-branches",
							"prow.k8s.io/job":             "always-runs-all-branches",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/job":                 "reported-job-runs-on-foo-file-change",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/type":                "presubmit",
							"created-by-prow":                 "true",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/context":             "foo-job-reported",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/refs.base_ref":       "",
						},
						Annotations: map[string]string{
							"prow.k8s.io/job":             "reported-job-runs-on-foo-file-change",
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/context":         "foo-job-reported",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/refs.base_ref":       "",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/job":                 "runs-on-all-but-baz-branch",
							"created-by-prow":                 "true",
							"prow.k8s.io/context":             "runs-on-all-but-baz-branch",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/gerrit-revision":     "1",
						},
						Annotations: map[string]string{
							"prow.k8s.io/job":             "runs-on-all-but-baz-branch",
							"prow.k8s.io/context":         "runs-on-all-but-baz-branch",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "presubmit does not run when a file matches run_if_changed but the change is WorkInProgress",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Status:          "NEW",
				WorkInProgress:  true,
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Files: map[string]client.FileInfo{
							"bee-movie-script.txt": {},
							"africa-lyrics.txt":    {},
							"important-code.go":    {},
						},
						Created: stampNow,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
		},
		{
			name: "presubmit doesn't run when no files match run_if_changed",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Files: map[string]client.FileInfo{
							"hacky-hack.sh": {},
							"README.md":     {},
							"let-it-go.txt": {},
						},
						Created: stampNow,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/job":                 "always-runs-all-branches",
							"prow.k8s.io/refs.base_ref":       "",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/context":             "always-runs-all-branches",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"created-by-prow":                 "true",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.repo":           "test-infra",
						},
						Annotations: map[string]string{
							"prow.k8s.io/context":         "always-runs-all-branches",
							"prow.k8s.io/job":             "always-runs-all-branches",
							"foo":                         "bar",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/job":                 "runs-on-all-but-baz-branch",
							"prow.k8s.io/context":             "runs-on-all-but-baz-branch",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/refs.base_ref":       "",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"created-by-prow":                 "true",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.repo":           "test-infra",
						},
						Annotations: map[string]string{
							"prow.k8s.io/job":             "runs-on-all-but-baz-branch",
							"prow.k8s.io/context":         "runs-on-all-but-baz-branch",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "presubmit run when change against matched branch",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Branch:          "pony",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Created: stampNow,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/gerrit-patchset":     "0",
							"created-by-prow":                 "true",
							"prow.k8s.io/context":             "always-runs-all-branches",
							"prow.k8s.io/job":                 "always-runs-all-branches",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/refs.base_ref":       "pony",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
						},
						Annotations: map[string]string{
							"prow.k8s.io/context":         "always-runs-all-branches",
							"prow.k8s.io/job":             "always-runs-all-branches",
							"foo":                         "bar",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseRef:  "pony",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"created-by-prow":                 "true",
							"prow.k8s.io/job":                 "runs-on-pony-branch",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/context":             "runs-on-pony-branch",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/refs.base_ref":       "pony",
						},
						Annotations: map[string]string{
							"prow.k8s.io/job":             "runs-on-pony-branch",
							"prow.k8s.io/context":         "runs-on-pony-branch",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseRef:  "pony",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"created-by-prow":                 "true",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/context":             "runs-on-all-but-baz-branch",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/job":                 "runs-on-all-but-baz-branch",
							"prow.k8s.io/refs.base_ref":       "pony",
						},
						Annotations: map[string]string{
							"prow.k8s.io/job":             "runs-on-all-but-baz-branch",
							"prow.k8s.io/context":         "runs-on-all-but-baz-branch",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseRef:  "pony",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "presubmit doesn't run when not against target branch",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Branch:          "baz",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Created: stampNow,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/job":                 "always-runs-all-branches",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/gerrit-patchset":     "0",
							"created-by-prow":                 "true",
							"prow.k8s.io/context":             "always-runs-all-branches",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/refs.base_ref":       "baz",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/gerrit-revision":     "1",
						},
						Annotations: map[string]string{
							"foo":                         "bar",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/context":         "always-runs-all-branches",
							"prow.k8s.io/job":             "always-runs-all-branches",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseRef:  "baz",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "old presubmits don't run on old revision but trigger job does because new message",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Branch:          "baz",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Number:  1,
						Created: makeStamp(timeNow.Add(-time.Hour)),
					},
				},
				Messages: []gerrit.ChangeMessageInfo{
					{
						Message:        "/test troll",
						RevisionNumber: 1,
						Date:           makeStamp(timeNow.Add(time.Hour)),
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/job":                 "trigger-regex-all-branches",
							"prow.k8s.io/refs.base_ref":       "baz",
							"prow.k8s.io/gerrit-revision":     "1",
							"created-by-prow":                 "true",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/gerrit-patchset":     "1",
							"prow.k8s.io/context":             "trigger-regex-all-branches",
							"prow.k8s.io/refs.repo":           "test-infra",
						},
						Annotations: map[string]string{
							"prow.k8s.io/job":             "trigger-regex-all-branches",
							"prow.k8s.io/context":         "trigger-regex-all-branches",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseRef:  "baz",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "unrelated comment shouldn't trigger anything",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Branch:          "baz",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Number:  1,
						Created: makeStamp(timeNow.Add(-time.Hour)),
					},
				},
				Messages: []gerrit.ChangeMessageInfo{
					{
						Message:        "/test diasghdgasudhkashdk",
						RevisionNumber: 1,
						Date:           makeStamp(timeNow.Add(time.Hour)),
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
		},
		{
			name: "trigger always run job on test all even if revision is old",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Branch:          "baz",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Number:  1,
						Created: makeStamp(timeNow.Add(-time.Hour)),
					},
				},
				Messages: []gerrit.ChangeMessageInfo{
					{
						Message:        "/test all",
						RevisionNumber: 1,
						Date:           makeStamp(timeNow.Add(time.Hour)),
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"created-by-prow":                 "true",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/job":                 "always-runs-all-branches",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.base_ref":       "baz",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/gerrit-patchset":     "1",
							"prow.k8s.io/context":             "always-runs-all-branches",
						},
						Annotations: map[string]string{
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/job":             "always-runs-all-branches",
							"prow.k8s.io/context":         "always-runs-all-branches",
							"foo":                         "bar",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseRef:  "baz",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "trigger always run job on test all even if the change is WorkInProgress",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Branch:          "baz",
				Status:          "NEW",
				WorkInProgress:  true,
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Number: 1,
					},
				},
				Messages: []gerrit.ChangeMessageInfo{
					{
						Message:        "/test all",
						RevisionNumber: 1,
						Date:           makeStamp(timeNow.Add(time.Hour)),
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/context":             "always-runs-all-branches",
							"created-by-prow":                 "true",
							"prow.k8s.io/refs.base_ref":       "baz",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/job":                 "always-runs-all-branches",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/gerrit-patchset":     "1",
						},
						Annotations: map[string]string{
							"prow.k8s.io/job":             "always-runs-all-branches",
							"prow.k8s.io/context":         "always-runs-all-branches",
							"foo":                         "bar",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseRef:  "baz",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "retest correctly triggers failed jobs",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Branch:          "retest-branch",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Number:  1,
						Created: makeStamp(timeNow.Add(-time.Hour)),
					},
				},
				Messages: []gerrit.ChangeMessageInfo{
					{
						Message:        "/retest",
						RevisionNumber: 1,
						Date:           makeStamp(timeNow.Add(time.Hour)),
					},
					{
						Message:        "Prow Status: 1 out of 2 passed\n✔️ foo-job SUCCESS - http://foo-status\n❌ bar-job FAILURE - http://bar-status",
						Author:         gerrit.AccountInfo{AccountID: 42},
						RevisionNumber: 1,
						Date:           makeStamp(timeNow.Add(-time.Hour)),
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/job":                 "bar-job",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/context":             "bar-job",
							"prow.k8s.io/gerrit-patchset":     "1",
							"created-by-prow":                 "true",
							"prow.k8s.io/refs.base_ref":       "retest-branch",
							"prow.k8s.io/gerrit-revision":     "1",
						},
						Annotations: map[string]string{
							"prow.k8s.io/job":             "bar-job",
							"prow.k8s.io/context":         "bar-job",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseRef:  "retest-branch",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "retest uses latest status and ignores earlier status",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Branch:          "retest-branch",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Number:  1,
						Created: makeStamp(timeNow.Add(-3 * time.Hour)),
					},
				},
				Messages: []gerrit.ChangeMessageInfo{
					{
						Message:        "/retest",
						RevisionNumber: 1,
						Date:           makeStamp(timeNow.Add(time.Hour)),
					},
					{
						Message:        "Prow Status: 1 out of 2 passed\n✔️ foo-job SUCCESS - http://foo-status\n❌ bar-job FAILURE - http://bar-status",
						RevisionNumber: 1,
						Author:         gerrit.AccountInfo{AccountID: 42},
						Date:           makeStamp(timeNow.Add(-time.Hour)),
					},
					{
						Message:        "Prow Status: 0 out of 2 passed\n❌️ foo-job FAILURE - http://foo-status\n❌ bar-job FAILURE - http://bar-status",
						RevisionNumber: 1,
						Author:         gerrit.AccountInfo{AccountID: 42},
						Date:           makeStamp(timeNow.Add(-2 * time.Hour)),
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/gerrit-patchset":     "1",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/refs.base_ref":       "retest-branch",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/job":                 "bar-job",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/context":             "bar-job",
							"created-by-prow":                 "true",
						},
						Annotations: map[string]string{
							"prow.k8s.io/context":         "bar-job",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/job":             "bar-job",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseRef:  "retest-branch",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "retest ignores statuses not reported by the prow account",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Branch:          "retest-branch",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Number:  1,
						Created: makeStamp(timeNow.Add(-3 * time.Hour)),
					},
				},
				Messages: []gerrit.ChangeMessageInfo{
					{
						Message:        "/retest",
						RevisionNumber: 1,
						Date:           makeStamp(timeNow.Add(time.Hour)),
					},
					{
						Message:        "Prow Status: 1 out of 2 passed\n✔️ foo-job SUCCESS - http://foo-status\n❌ bar-job FAILURE - http://bar-status",
						RevisionNumber: 1,
						Author:         gerrit.AccountInfo{AccountID: 123},
						Date:           makeStamp(timeNow.Add(-time.Hour)),
					},
					{
						Message:        "Prow Status: 0 out of 2 passed\n❌️ foo-job FAILURE - http://foo-status\n❌ bar-job FAILURE - http://bar-status",
						RevisionNumber: 1,
						Author:         gerrit.AccountInfo{AccountID: 42},
						Date:           makeStamp(timeNow.Add(-2 * time.Hour)),
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/job":                 "foo-job",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/refs.pull":           "0",
							"created-by-prow":                 "true",
							"prow.k8s.io/gerrit-patchset":     "1",
							"prow.k8s.io/context":             "foo-job",
							"prow.k8s.io/refs.base_ref":       "retest-branch",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/type":                "presubmit",
						},
						Annotations: map[string]string{
							"prow.k8s.io/job":             "foo-job",
							"prow.k8s.io/context":         "foo-job",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseRef:  "retest-branch",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/gerrit-patchset":     "1",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/context":             "bar-job",
							"prow.k8s.io/refs.base_ref":       "retest-branch",
							"prow.k8s.io/gerrit-revision":     "1",
							"created-by-prow":                 "true",
							"prow.k8s.io/job":                 "bar-job",
							"prow.k8s.io/refs.repo":           "test-infra",
						},
						Annotations: map[string]string{
							"prow.k8s.io/context":         "bar-job",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/job":             "bar-job",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseRef:  "retest-branch",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "retest does nothing if there are no latest reports",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Branch:          "retest-branch",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Number:  1,
						Created: makeStamp(timeNow.Add(-time.Hour)),
					},
				},
				Messages: []gerrit.ChangeMessageInfo{
					{
						Message:        "/retest",
						RevisionNumber: 1,
						Date:           makeStamp(timeNow),
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
		},
		{
			name: "retest uses the latest report",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Branch:          "retest-branch",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Number:  1,
						Created: makeStamp(timeNow.Add(-3 * time.Hour)),
					},
				},
				Messages: []gerrit.ChangeMessageInfo{
					{
						Message:        "/retest",
						RevisionNumber: 1,
						Date:           makeStamp(timeNow.Add(time.Hour)),
					},
					{
						Message:        "Prow Status: 1 out of 2 passed\n✔️ foo-job SUCCESS - http://foo-status\n❌ bar-job FAILURE - http://bar-status",
						RevisionNumber: 1,
						Author:         gerrit.AccountInfo{AccountID: 42},
						Date:           makeStamp(timeNow.Add(-2 * time.Hour)),
					},
					{
						Message:        "Prow Status: 0 out of 2 passed\n❌️ foo-job FAILURE - http://foo-status\n❌ bar-job FAILURE - http://bar-status",
						RevisionNumber: 1,
						Author:         gerrit.AccountInfo{AccountID: 42},
						Date:           makeStamp(timeNow.Add(-time.Hour)),
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"created-by-prow":                 "true",
							"prow.k8s.io/job":                 "foo-job",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/context":             "foo-job",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.base_ref":       "retest-branch",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/gerrit-patchset":     "1",
						},
						Annotations: map[string]string{
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/job":             "foo-job",
							"prow.k8s.io/context":         "foo-job",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseRef:  "retest-branch",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/gerrit-patchset":     "1",
							"created-by-prow":                 "true",
							"prow.k8s.io/context":             "bar-job",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.base_ref":       "retest-branch",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/job":                 "bar-job",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
						},
						Annotations: map[string]string{
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/job":             "bar-job",
							"prow.k8s.io/context":         "bar-job",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseRef:  "retest-branch",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "no comments when no jobs have Report set",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Ref:     "refs/changes/00/1/1",
						Created: stampNow,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/refs.base_ref":       "",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"created-by-prow":                 "true",
							"prow.k8s.io/job":                 "always-runs-all-branches",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/context":             "always-runs-all-branches",
							"prow.k8s.io/refs.repo":           "test-infra",
						},
						Annotations: map[string]string{
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/job":             "always-runs-all-branches",
							"prow.k8s.io/context":         "always-runs-all-branches",
							"foo":                         "bar",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/1/1",
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/refs.org":            "gerrit",
							"created-by-prow":                 "true",
							"prow.k8s.io/job":                 "runs-on-all-but-baz-branch",
							"prow.k8s.io/context":             "runs-on-all-but-baz-branch",
							"prow.k8s.io/refs.base_ref":       "",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/refs.pull":           "0",
						},
						Annotations: map[string]string{
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/job":             "runs-on-all-but-baz-branch",
							"prow.k8s.io/context":         "runs-on-all-but-baz-branch",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/1/1",
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
			wantSkipReport: true,
		},
		{
			name: "comment left when at-least 1 job has Report set",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Files: map[string]client.FileInfo{
							"a.foo": {},
						},
						Ref:     "refs/changes/00/1/1",
						Created: stampNow,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/context":             "always-runs-all-branches",
							"prow.k8s.io/refs.base_ref":       "",
							"prow.k8s.io/type":                "presubmit",
							"created-by-prow":                 "true",
							"prow.k8s.io/job":                 "always-runs-all-branches",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
						},
						Annotations: map[string]string{
							"foo":                         "bar",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/job":             "always-runs-all-branches",
							"prow.k8s.io/context":         "always-runs-all-branches",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/1/1",
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"created-by-prow":                 "true",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/job":                 "runs-on-all-but-baz-branch",
							"prow.k8s.io/refs.base_ref":       "",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/context":             "runs-on-all-but-baz-branch",
							"prow.k8s.io/refs.pull":           "0",
						},
						Annotations: map[string]string{
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/context":         "runs-on-all-but-baz-branch",
							"prow.k8s.io/job":             "runs-on-all-but-baz-branch",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/1/1",
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/refs.pull":           "0",
							"created-by-prow":                 "true",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/context":             "foo-job-reported",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.base_ref":       "",
							"prow.k8s.io/job":                 "reported-job-runs-on-foo-file-change",
							"prow.k8s.io/gerrit-revision":     "1",
						},
						Annotations: map[string]string{
							"prow.k8s.io/job":             "reported-job-runs-on-foo-file-change",
							"prow.k8s.io/context":         "foo-job-reported",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/1/1",
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "/test ? will leave a comment with the commands to trigger presubmit Prow jobs",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Status:          "NEW",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Number:  1,
						Created: makeStamp(timeNow.Add(-time.Hour)),
					},
				},
				Messages: []gerrit.ChangeMessageInfo{
					{
						Message:        "/test ?",
						RevisionNumber: 1,
						Date:           makeStamp(timeNow),
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
		},
		{
			name: "InRepoConfig Presubmits are retrieved",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Status:          "NEW",
				Branch:          "inRepoConfig",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Ref:     "refs/changes/00/1/1",
						Created: stampNow,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/job":                 "always-runs-all-branches",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"created-by-prow":                 "true",
							"prow.k8s.io/context":             "always-runs-all-branches",
							"prow.k8s.io/refs.base_ref":       "inRepoConfig",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/gerrit-revision":     "1",
						},
						Annotations: map[string]string{
							"prow.k8s.io/context":         "always-runs-all-branches",
							"foo":                         "bar",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/job":             "always-runs-all-branches",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseRef:  "inRepoConfig",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/1/1",
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/job":                 "runs-on-all-but-baz-branch",
							"prow.k8s.io/context":             "runs-on-all-but-baz-branch",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/refs.base_ref":       "inRepoConfig",
							"created-by-prow":                 "true",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/refs.pull":           "0",
						},
						Annotations: map[string]string{
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/job":             "runs-on-all-but-baz-branch",
							"prow.k8s.io/context":         "runs-on-all-but-baz-branch",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseRef:  "inRepoConfig",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/1/1",
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/job":                 "always-runs-inRepoConfig",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/gerrit-revision":     "1",
							"created-by-prow":                 "true",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/refs.base_ref":       "inRepoConfig",
							"prow.k8s.io/context":             "always-runs-inRepoConfig",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/gerrit-patchset":     "0",
						},
						Annotations: map[string]string{
							"prow.k8s.io/job":             "always-runs-inRepoConfig",
							"prow.k8s.io/context":         "always-runs-inRepoConfig",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "test-infra",
							RepoLink: "https://gerrit/test-infra",
							BaseSHA:  "abc",
							BaseRef:  "inRepoConfig",
							BaseLink: "https://gerrit/test-infra/+/abc",
							CloneURI: "https://gerrit/test-infra",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/1/1",
									SHA:        "1",
									Link:       "https://gerrit/c/test-infra/+/0",
									CommitLink: "https://gerrit/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "InRepoConfig Postsubmits are retrieved",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "postsubmits-project",
				Status:          "MERGED",
				Branch:          "inRepoConfig",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Ref:     "refs/changes/00/1/1",
						Created: stampNow,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/type":                "postsubmit",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.base_ref":       "inRepoConfig",
							"prow.k8s.io/refs.pull":           "0",
							"created-by-prow":                 "true",
							"prow.k8s.io/refs.repo":           "postsubmits-project",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/job":                 "test-bar",
							"prow.k8s.io/context":             "test-bar",
							"prow.k8s.io/gerrit-patchset":     "0",
						},
						Annotations: map[string]string{
							"prow.k8s.io/job":             "test-bar",
							"prow.k8s.io/context":         "test-bar",
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "postsubmits-project",
							RepoLink: "https://gerrit/postsubmits-project",
							BaseSHA:  "abc",
							BaseRef:  "inRepoConfig",
							BaseLink: "https://gerrit/postsubmits-project/+/abc",
							CloneURI: "https://gerrit/postsubmits-project",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/1/1",
									SHA:        "1",
									Link:       "https://gerrit/c/postsubmits-project/+/0",
									CommitLink: "https://gerrit/postsubmits-project/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/context":             "always-runs-inRepoConfig-Post",
							"prow.k8s.io/gerrit-patchset":     "0",
							"created-by-prow":                 "true",
							"prow.k8s.io/type":                "postsubmit",
							"prow.k8s.io/refs.base_ref":       "inRepoConfig",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/job":                 "always-runs-inRepoConfig-Post",
							"prow.k8s.io/refs.repo":           "postsubmits-project",
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/refs.org":            "gerrit",
						},
						Annotations: map[string]string{
							"prow.k8s.io/job":             "always-runs-inRepoConfig-Post",
							"prow.k8s.io/context":         "always-runs-inRepoConfig-Post",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "postsubmits-project",
							RepoLink: "https://gerrit/postsubmits-project",
							BaseSHA:  "abc",
							BaseRef:  "inRepoConfig",
							BaseLink: "https://gerrit/postsubmits-project/+/abc",
							CloneURI: "https://gerrit/postsubmits-project",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/1/1",
									SHA:        "1",
									Link:       "https://gerrit/c/postsubmits-project/+/0",
									CommitLink: "https://gerrit/postsubmits-project/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "InRepoConfig Presubmits are retrieved when repo name format has slash",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "kubernetes/test-infra",
				Status:          "NEW",
				Branch:          "inRepoConfig",
				Revisions: map[string]client.RevisionInfo{
					"1": {
						Ref:     "refs/changes/00/1/1",
						Created: stampNow,
					},
				},
			},
			instancesMap: map[string]*gerrit.AccountInfo{testInstance: {AccountID: 42}},
			instance:     testInstance,
			wantPjs: []*prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/refs.pull":           "0",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/job":                 "always-runs-inRepoConfig",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/context":             "always-runs-inRepoConfig",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.base_ref":       "inRepoConfig",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"created-by-prow":                 "true",
							"prow.k8s.io/type":                "presubmit",
						},
						Annotations: map[string]string{
							"prow.k8s.io/context":         "always-runs-inRepoConfig",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
							"prow.k8s.io/job":             "always-runs-inRepoConfig",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "kubernetes/test-infra",
							RepoLink: "https://gerrit/kubernetes/test-infra",
							BaseSHA:  "abc",
							BaseRef:  "inRepoConfig",
							BaseLink: "https://gerrit/kubernetes/test-infra/+/abc",
							CloneURI: "https://gerrit/kubernetes/test-infra",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/1/1",
									SHA:        "1",
									Link:       "https://gerrit/c/kubernetes/test-infra/+/0",
									CommitLink: "https://gerrit/kubernetes/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"prow.k8s.io/refs.base_ref":       "inRepoConfig",
							"prow.k8s.io/gerrit-revision":     "1",
							"prow.k8s.io/gerrit-patchset":     "0",
							"prow.k8s.io/gerrit-report-label": "Code-Review",
							"prow.k8s.io/refs.repo":           "test-infra",
							"prow.k8s.io/type":                "presubmit",
							"prow.k8s.io/job":                 "other-test",
							"prow.k8s.io/context":             "other-test",
							"prow.k8s.io/refs.org":            "gerrit",
							"prow.k8s.io/refs.pull":           "0",
							"created-by-prow":                 "true",
						},
						Annotations: map[string]string{
							"prow.k8s.io/job":             "other-test",
							"prow.k8s.io/context":         "other-test",
							"prow.k8s.io/gerrit-instance": "https://gerrit",
							"prow.k8s.io/gerrit-id":       "",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{
							Org:      "https://gerrit",
							Repo:     "kubernetes/test-infra",
							RepoLink: "https://gerrit/kubernetes/test-infra",
							BaseSHA:  "abc",
							BaseRef:  "inRepoConfig",
							BaseLink: "https://gerrit/kubernetes/test-infra/+/abc",
							CloneURI: "https://gerrit/kubernetes/test-infra",
							Pulls: []prowapi.Pull{
								{
									Ref:        "refs/changes/00/1/1",
									SHA:        "1",
									Link:       "https://gerrit/c/kubernetes/test-infra/+/0",
									CommitLink: "https://gerrit/kubernetes/test-infra/+/1",
									AuthorLink: "https://gerrit/q/",
								},
							},
						},
					},
				},
			},
		},
	}

	testInfraPresubmits := []config.Presubmit{
		{
			JobBase: config.JobBase{
				Name:        "always-runs-all-branches",
				Annotations: map[string]string{"foo": "bar"},
			},
			AlwaysRun: true,
			Reporter: config.Reporter{
				Context:    "always-runs-all-branches",
				SkipReport: true,
			},
		},
		{
			JobBase: config.JobBase{
				Name: "run-if-changed-all-branches",
			},
			RegexpChangeMatcher: config.RegexpChangeMatcher{
				RunIfChanged: "\\.go",
			},
			Reporter: config.Reporter{
				Context:    "run-if-changed-all-branches",
				SkipReport: true,
			},
		},
		{
			JobBase: config.JobBase{
				Name: "runs-on-pony-branch",
			},
			Brancher: config.Brancher{
				Branches: []string{"pony"},
			},
			AlwaysRun: true,
			Reporter: config.Reporter{
				Context:    "runs-on-pony-branch",
				SkipReport: true,
			},
		},
		{
			JobBase: config.JobBase{
				Name: "runs-on-all-but-baz-branch",
			},
			Brancher: config.Brancher{
				SkipBranches: []string{"baz"},
			},
			AlwaysRun: true,
			Reporter: config.Reporter{
				Context:    "runs-on-all-but-baz-branch",
				SkipReport: true,
			},
		},
		{
			JobBase: config.JobBase{
				Name: "trigger-regex-all-branches",
			},
			Trigger:      `.*/test\s*troll.*`,
			RerunCommand: "/test troll",
			Reporter: config.Reporter{
				Context:    "trigger-regex-all-branches",
				SkipReport: true,
			},
		},
		{
			JobBase: config.JobBase{
				Name: "foo-job",
			},
			Reporter: config.Reporter{
				Context:    "foo-job",
				SkipReport: true,
			},
		},
		{
			JobBase: config.JobBase{
				Name: "bar-job",
			},
			Reporter: config.Reporter{
				Context:    "bar-job",
				SkipReport: true,
			},
		},
		{
			JobBase: config.JobBase{
				Name: "reported-job-runs-on-foo-file-change",
			},
			RegexpChangeMatcher: config.RegexpChangeMatcher{
				RunIfChanged: "\\.foo",
			},
			Reporter: config.Reporter{
				Context: "foo-job-reported",
			},
		},
	}
	if err := config.SetPresubmitRegexes(testInfraPresubmits); err != nil {
		t.Fatalf("could not set regexes: %v", err)
	}

	cfg := &config.Config{
		JobConfig: config.JobConfig{
			ProwYAMLGetterWithDefaults: fakeProwYAMLGetter,
			ProwYAMLGetter:             fakeProwYAMLGetter,
			PresubmitsStatic: map[string][]config.Presubmit{
				"https://gerrit/test-infra": testInfraPresubmits,
				"https://gerrit/kubernetes/test-infra": {
					{
						JobBase: config.JobBase{
							Name: "other-test",
						},
						AlwaysRun: true,
						Reporter: config.Reporter{
							Context:    "other-test",
							SkipReport: true,
						},
					},
				},
				"https://gerrit/other-repo": {
					{
						JobBase: config.JobBase{
							Name: "other-test",
						},
						AlwaysRun: true,
						Reporter: config.Reporter{
							Context:    "other-test",
							SkipReport: true,
						},
					},
				},
			},
			PostsubmitsStatic: map[string][]config.Postsubmit{
				"https://gerrit/postsubmits-project": {
					{
						JobBase: config.JobBase{
							Name: "test-bar",
						},
						Reporter: config.Reporter{
							Context:    "test-bar",
							SkipReport: true,
						},
					},
				},
			},
		},
		ProwConfig: config.ProwConfig{
			PodNamespace: namespace,
			InRepoConfig: config.InRepoConfig{
				Enabled:         map[string]*bool{"*": &trueBool},
				AllowedClusters: map[string][]string{"*": {kube.DefaultClusterAlias}},
			},
		},
	}
	fca := &fca{
		c: cfg,
	}
	for _, tc := range testcases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fakeProwJobClient := prowfake.NewSimpleClientset()
			fakeLastSync := client.LastSyncState{tc.instance: map[string]time.Time{}}
			fakeLastSync[tc.instance][tc.change.Project] = timeNow.Add(-time.Minute)

			cache, err := createTestRepoCache(t, fca)
			if err != nil {
				t.Errorf("error making test repo cache %v", err)
			}

			var gc fgc
			gc.instanceMap = tc.instancesMap
			c := &Controller{
				config:                      fca.Config,
				prowJobClient:               fakeProwJobClient.ProwV1().ProwJobs("prowjobs"),
				gc:                          &gc,
				tracker:                     &fakeSync{val: fakeLastSync},
				inRepoConfigGetter:          cache,
				inRepoConfigFailuresTracker: make(map[string]bool),
			}

			err = c.triggerJobs(logrus.WithField("name", tc.name), tc.instance, tc.change)
			if tc.wantError {
				if err == nil {
					t.Fatal("Expected error, got nil.")
				}
				return
			}

			if err != nil {
				t.Fatalf("Expect no error, but got %v", err)
			}

			var gotProwjobs []*prowapi.ProwJob
			for _, action := range fakeProwJobClient.Fake.Actions() {
				switch action := action.(type) {
				case clienttesting.CreateActionImpl:
					if prowjob, ok := action.Object.(*prowapi.ProwJob); ok {
						// Comparing the entire prowjob struct is not necessary
						// in this test, so construct only ProwJob structs with
						// only necessary informations.
						gotProwjobs = append(gotProwjobs, &prowapi.ProwJob{
							ObjectMeta: metav1.ObjectMeta{
								Labels:      prowjob.Labels,
								Annotations: prowjob.Annotations,
							},
							Spec: prowapi.ProwJobSpec{
								Refs: prowjob.Spec.Refs,
							},
						})
					}
				}
			}

			// It seems that the PJs are very deterministic, consider sorting
			// them if this test becomes flaky.
			if diff := cmp.Diff(tc.wantPjs, gotProwjobs, cmpopts.SortSlices(func(a, b *prowapi.ProwJob) bool {
				if b == nil {
					return true
				}
				if a == nil {
					return false
				}
				return a.Labels["prow.k8s.io/job"] < b.Labels["prow.k8s.io/job"]
			})); diff != "" {
				t.Fatalf("%q Prowjobs mismatch. Want(-), got(+):\n%s", tc.name, diff)
			}

			if tc.wantSkipReport {
				if gc.reviews > 0 {
					t.Errorf("expected no comments, got: %d", gc.reviews)
				}
			}
		})
	}
}

func TestIsProjectExemptFromHelp(t *testing.T) {
	var testcases = []struct {
		name                   string
		projectsExemptFromHelp map[string]sets.Set[string]
		instance               string
		project                string
		expected               bool
	}{
		{
			name:                   "no project is exempt",
			projectsExemptFromHelp: map[string]sets.Set[string]{},
			instance:               "foo",
			project:                "bar",
			expected:               false,
		},
		{
			name: "the instance does not match",
			projectsExemptFromHelp: map[string]sets.Set[string]{
				"foo": sets.New[string]("bar"),
			},
			instance: "fuz",
			project:  "bar",
			expected: false,
		},
		{
			name: "the instance matches but the project does not",
			projectsExemptFromHelp: map[string]sets.Set[string]{
				"foo": sets.New[string]("baz"),
			},
			instance: "fuz",
			project:  "bar",
			expected: false,
		},
		{
			name: "the project is exempt",
			projectsExemptFromHelp: map[string]sets.Set[string]{
				"foo": sets.New[string]("bar"),
			},
			instance: "foo",
			project:  "bar",
			expected: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got := isProjectOptOutHelp(tc.projectsExemptFromHelp, tc.instance, tc.project)
			if got != tc.expected {
				t.Errorf("expected %t for IsProjectExemptFromHelp but got %t", tc.expected, got)
			}
		})
	}
}

func TestDeckLinkForPR(t *testing.T) {
	tcs := []struct {
		name         string
		deckURL      string
		refs         prowapi.Refs
		changeStatus string
		expected     string
	}{
		{
			name:         "No deck_url specified",
			changeStatus: client.New,
			expected:     "",
		},
		{
			name:    "deck_url specified, repo without slash",
			deckURL: "https://prow.k8s.io/",
			refs: prowapi.Refs{
				Org:  "gerrit-review.host.com",
				Repo: "test-infra",
				Pulls: []prowapi.Pull{
					{
						Number: 42,
					},
				},
			},
			changeStatus: client.New,
			expected:     "https://prow.k8s.io/?pull=42&repo=gerrit-review.host.com%2Ftest-infra",
		},
		{
			name:    "deck_url specified, repo with slash",
			deckURL: "https://prow.k8s.io/",
			refs: prowapi.Refs{
				Org:  "gerrit-review.host.com",
				Repo: "test/infra",
				Pulls: []prowapi.Pull{
					{
						Number: 42,
					},
				},
			},
			changeStatus: client.New,
			expected:     "https://prow.k8s.io/?pull=42&repo=gerrit-review.host.com%2Ftest%2Finfra",
		},
		{
			name:    "deck_url specified, change is merged (triggering postsubmits)",
			deckURL: "https://prow.k8s.io/",
			refs: prowapi.Refs{
				Org:  "gerrit-review.host.com",
				Repo: "test-infra",
				Pulls: []prowapi.Pull{
					{
						Number: 42,
					},
				},
			},
			changeStatus: client.Merged,
			expected:     "",
		},
	}

	for i := range tcs {
		tc := tcs[i]
		t.Run(tc.name, func(t *testing.T) {
			result, err := deckLinkForPR(tc.deckURL, tc.refs, tc.changeStatus)
			if err != nil {
				t.Errorf("unexpected error generating Deck link: %v", err)
			}
			if result != tc.expected {
				t.Errorf("expected deck link %s, but got %s", tc.expected, result)
			}
		})
	}
}
