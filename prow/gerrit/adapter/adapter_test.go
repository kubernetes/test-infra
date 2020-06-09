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
	"sync"
	"testing"
	"time"

	"github.com/andygrunwald/go-gerrit"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
	clienttesting "k8s.io/client-go/testing"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowfake "k8s.io/test-infra/prow/client/clientset/versioned/fake"
	"k8s.io/test-infra/prow/config"
	reporter "k8s.io/test-infra/prow/crier/reporters/gerrit"
	"k8s.io/test-infra/prow/gerrit/client"
)

func makeStamp(t time.Time) gerrit.Timestamp {
	return gerrit.Timestamp{Time: t}
}

var (
	timeNow  = time.Date(1234, time.May, 15, 1, 2, 3, 4, time.UTC)
	stampNow = makeStamp(timeNow)
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
	reviews int
}

func (f *fgc) QueryChanges(lastUpdate client.LastSyncState, rateLimit int) map[string][]client.ChangeInfo {
	return nil
}

func (f *fgc) SetReview(instance, id, revision, message string, labels map[string]string) error {
	f.reviews++
	return nil
}

func (f *fgc) GetBranchRevision(instance, project, branch string) (string, error) {
	return "abc", nil
}

func (f *fgc) Account(instance string) *gerrit.AccountInfo {
	return &gerrit.AccountInfo{AccountID: 42}
}

func TestMakeCloneURI(t *testing.T) {
	cases := []struct {
		name     string
		instance string
		project  string
		expected string
		err      bool
	}{
		{
			name:     "happy case",
			instance: "https://android.googlesource.com",
			project:  "platform/build",
			expected: "https://android.googlesource.com/platform/build",
		},
		{
			name:     "reject non urls",
			instance: "!!!://",
			project:  "platform/build",
			err:      true,
		},
		{
			name:     "require instance to specify host",
			instance: "android.googlesource.com",
			project:  "platform/build",
			err:      true,
		},
		{
			name:     "reject instances with paths",
			instance: "https://android.googlesource.com/platform",
			project:  "build",
			err:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := makeCloneURI(tc.instance, tc.project)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Error("failed to receive expected exception")
			case actual.String() != tc.expected:
				t.Errorf("actual %q != expected %q", actual.String(), tc.expected)
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
		Org:      "cat-review.example.com",
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
	cloneURI, err := makeCloneURI(reviewHost, change.Project)
	if err != nil {
		t.Errorf("failed to make clone URI: %v", err)
	}
	actual, err := createRefs(reviewHost, change, cloneURI, "abcdef")
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
		return reporter.GenerateReport(pjs).String()
	}

	cases := []struct {
		name     string
		messages []gerrit.ChangeMessageInfo
		expected sets.String
	}{
		{
			name: "basically works",
		},
		{
			name: "report parses",
			messages: []gerrit.ChangeMessageInfo{
				message("ignore this", nil),
				message(report(map[string]prowapi.ProwJobState{
					"foo":         prowapi.SuccessState,
					"should-fail": prowapi.FailureState,
				}), nil),
				message("also ignore this", nil),
			},
			expected: sets.NewString("should-fail"),
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
			expected: sets.NewString("should-fail"),
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
			expected: sets.NewString("current-fail"),
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
			expected: sets.NewString("old-broken", "new-broken", "still-fail", "pass-then-failed"),
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
			expected: sets.NewString("earth-broken", "blackhole-broken", "fail-earth-pass-blackhole"),
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

func TestProcessChange(t *testing.T) {
	var testcases = []struct {
		name             string
		change           client.ChangeInfo
		numPJ            int
		pjRef            string
		shouldError      bool
		shouldSkipReport bool
	}{
		{
			name: "no revisions errors out",
			change: client.ChangeInfo{
				CurrentRevision: "1",
				Project:         "test-infra",
				Status:          "NEW",
			},
			shouldError: true,
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
			numPJ: 2,
			pjRef: "refs/changes/00/1/1",
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
			numPJ: 2,
			pjRef: "refs/changes/00/2/2",
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
			numPJ: 1,
			pjRef: "refs/changes/00/1/1",
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
			numPJ: 1,
			pjRef: "refs/changes/00/1/1",
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
			numPJ: 3,
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
			numPJ: 2,
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
			numPJ: 3,
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
			numPJ: 1,
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
			numPJ: 1,
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
			numPJ: 0,
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
			numPJ: 1,
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
			numPJ: 1,
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
			numPJ: 1,
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
			numPJ: 2,
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
			numPJ: 0,
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
			numPJ: 2,
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
			numPJ:            2,
			pjRef:            "refs/changes/00/1/1",
			shouldSkipReport: true,
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
			numPJ: 3,
			pjRef: "refs/changes/00/1/1",
		},
	}

	testInfraPresubmits := []config.Presubmit{
		{
			JobBase: config.JobBase{
				Name: "always-runs-all-branches",
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

	config := &config.Config{
		JobConfig: config.JobConfig{
			PresubmitsStatic: map[string][]config.Presubmit{
				"gerrit/test-infra": testInfraPresubmits,
				"https://gerrit/other-repo": {
					{
						JobBase: config.JobBase{
							Name: "other-test",
						},
						AlwaysRun: true,
					},
				},
			},
			PostsubmitsStatic: map[string][]config.Postsubmit{
				"gerrit/postsubmits-project": {
					{
						JobBase: config.JobBase{
							Name: "test-bar",
						},
					},
				},
			},
		},
	}
	testInstance := "https://gerrit"
	fca := &fca{
		c: config,
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			tc := tc // capture range variable
			t.Parallel()

			fakeProwJobClient := prowfake.NewSimpleClientset()
			fakeLastSync := client.LastSyncState{testInstance: map[string]time.Time{}}
			fakeLastSync[testInstance][tc.change.Project] = timeNow.Add(-time.Minute)

			var gc fgc
			c := &Controller{
				config:        fca.Config,
				prowJobClient: fakeProwJobClient.ProwV1().ProwJobs("prowjobs"),
				gc:            &gc,
				tracker:       &fakeSync{val: fakeLastSync},
			}

			err := c.ProcessChange(testInstance, tc.change)
			if err != nil && !tc.shouldError {
				t.Errorf("expect no error, but got %v", err)
			} else if err == nil && tc.shouldError {
				t.Errorf("expect error, but got none")
			}

			var prowjobs []*prowapi.ProwJob
			for _, action := range fakeProwJobClient.Fake.Actions() {
				switch action := action.(type) {
				case clienttesting.CreateActionImpl:
					if prowjob, ok := action.Object.(*prowapi.ProwJob); ok {
						prowjobs = append(prowjobs, prowjob)
					}
				}
			}

			if len(prowjobs) != tc.numPJ {
				t.Errorf("should make %d prowjob, got %d", tc.numPJ, len(prowjobs))
			}

			if len(prowjobs) > 0 {
				refs := prowjobs[0].Spec.Refs
				if refs.Org != "gerrit" {
					t.Errorf("org %s != gerrit", refs.Org)
				}
				if refs.Repo != tc.change.Project {
					t.Errorf("repo %s != expected %s", refs.Repo, tc.change.Project)
				}
				if prowjobs[0].Spec.Refs.Pulls[0].Ref != tc.pjRef {
					t.Errorf("ref should be %s, got %s", tc.pjRef, prowjobs[0].Spec.Refs.Pulls[0].Ref)
				}
				if prowjobs[0].Spec.Refs.BaseSHA != "abc" {
					t.Errorf("BaseSHA should be abc, got %s", prowjobs[0].Spec.Refs.BaseSHA)
				}
			}
			if tc.shouldSkipReport {
				if gc.reviews > 0 {
					t.Errorf("expected no comments, got: %d", gc.reviews)
				}
			}
		})
	}
}
