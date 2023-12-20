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

package tide

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"text/template"
	"time"

	"github.com/go-test/deep"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	fuzz "github.com/google/gofuzz"
	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
	utilpointer "k8s.io/utils/pointer"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/git/types"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/tide/history"
)

func init() {
	// Debugging tests without this isn't fun
	logrus.SetLevel(logrus.DebugLevel)
}

var defaultBranch = localgit.DefaultBranch("")

func testPullsMatchList(t *testing.T, test string, actual []CodeReviewCommon, expected []int) {
	if len(actual) != len(expected) {
		t.Errorf("Wrong size for case %s. Got PRs %+v, wanted numbers %v.", test, actual, expected)
		return
	}
	for _, pr := range actual {
		var found bool
		n1 := int(pr.Number)
		for _, n2 := range expected {
			if n1 == n2 {
				found = true
			}
		}
		if !found {
			t.Errorf("For case %s, found PR %d but shouldn't have.", test, n1)
		}
	}
}

func TestAccumulateBatch(t *testing.T) {
	jobSet := []config.Presubmit{
		{
			Reporter: config.Reporter{Context: "foo"},
		},
		{
			Reporter: config.Reporter{Context: "bar"},
		},
		{
			Reporter: config.Reporter{Context: "baz"},
		},
	}
	type pull struct {
		number int
		sha    string
	}
	type prowjob struct {
		prs   []pull
		job   string
		state prowapi.ProwJobState
	}
	tests := []struct {
		name           string
		presubmits     []config.Presubmit
		pulls          []pull
		prowJobs       []prowjob
		prowYAMLGetter config.ProwYAMLGetter

		merges  []int
		pending bool
	}{
		{
			name: "no batches running",
		},
		{
			name: "batch pending",
			presubmits: []config.Presubmit{
				{Reporter: config.Reporter{Context: "foo"}},
			},
			pulls:    []pull{{1, "a"}, {2, "b"}},
			prowJobs: []prowjob{{job: "foo", state: prowapi.PendingState, prs: []pull{{1, "a"}}}},
			pending:  true,
		},
		{
			name:       "pending batch missing presubmits is ignored",
			presubmits: jobSet,
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs:   []prowjob{{job: "foo", state: prowapi.PendingState, prs: []pull{{1, "a"}}}},
		},
		{
			name:       "batch pending, successful previous run",
			presubmits: jobSet,
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs: []prowjob{
				{job: "foo", state: prowapi.PendingState, prs: []pull{{1, "a"}}},
				{job: "bar", state: prowapi.SuccessState, prs: []pull{{1, "a"}}},
				{job: "baz", state: prowapi.SuccessState, prs: []pull{{1, "a"}}},
				{job: "foo", state: prowapi.SuccessState, prs: []pull{{2, "b"}}},
				{job: "bar", state: prowapi.SuccessState, prs: []pull{{2, "b"}}},
				{job: "baz", state: prowapi.SuccessState, prs: []pull{{2, "b"}}},
			},
			pending: true,
			merges:  []int{2},
		},
		{
			name:       "successful run",
			presubmits: jobSet,
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs: []prowjob{
				{job: "foo", state: prowapi.SuccessState, prs: []pull{{2, "b"}}},
				{job: "bar", state: prowapi.SuccessState, prs: []pull{{2, "b"}}},
				{job: "baz", state: prowapi.SuccessState, prs: []pull{{2, "b"}}},
			},
			merges: []int{2},
		},
		{
			name:       "successful run, multiple PRs",
			presubmits: jobSet,
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs: []prowjob{
				{job: "foo", state: prowapi.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "bar", state: prowapi.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "baz", state: prowapi.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
			},
			merges: []int{1, 2},
		},
		{
			name:       "successful run, failures in past",
			presubmits: jobSet,
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs: []prowjob{
				{job: "foo", state: prowapi.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "bar", state: prowapi.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "baz", state: prowapi.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "foo", state: prowapi.FailureState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "baz", state: prowapi.FailureState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "foo", state: prowapi.FailureState, prs: []pull{{1, "c"}, {2, "b"}}},
			},
			merges: []int{1, 2},
		},
		{
			name:       "failures",
			presubmits: jobSet,
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs: []prowjob{
				{job: "foo", state: prowapi.FailureState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "bar", state: prowapi.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "baz", state: prowapi.FailureState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "foo", state: prowapi.FailureState, prs: []pull{{1, "c"}, {2, "b"}}},
			},
		},
		{
			name:       "missing job required by one PR",
			presubmits: jobSet,
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs: []prowjob{
				{job: "foo", state: prowapi.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "bar", state: prowapi.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "baz", state: prowapi.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
			},
			prowYAMLGetter: prowYAMLGetterForHeadRefs([]string{"a", "b"}, []config.Presubmit{{
				AlwaysRun: true,
				Reporter:  config.Reporter{Context: "boo"},
			}}),
		},
		{
			name:       "successful run with PR that requires additional job",
			presubmits: jobSet,
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs: []prowjob{
				{job: "foo", state: prowapi.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "bar", state: prowapi.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "baz", state: prowapi.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "boo", state: prowapi.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
			},
			merges: []int{1, 2},
		},
		{
			name:    "no presubmits",
			pulls:   []pull{{1, "a"}, {2, "b"}},
			pending: false,
		},
		{
			name:       "pending batch with PR that left pool, successful previous run",
			presubmits: jobSet,
			pulls:      []pull{{2, "b"}},
			prowJobs: []prowjob{
				{job: "foo", state: prowapi.PendingState, prs: []pull{{1, "a"}}},
				{job: "foo", state: prowapi.SuccessState, prs: []pull{{2, "b"}}},
				{job: "bar", state: prowapi.SuccessState, prs: []pull{{2, "b"}}},
				{job: "baz", state: prowapi.SuccessState, prs: []pull{{2, "b"}}},
			},
			pending: false,
			merges:  []int{2},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			var pulls []CodeReviewCommon
			for _, p := range test.pulls {
				pr := PullRequest{
					Number:     githubql.Int(p.number),
					HeadRefOID: githubql.String(p.sha),
				}
				pulls = append(pulls, *CodeReviewCommonFromPullRequest(&pr))
			}
			var pjs []prowapi.ProwJob
			for _, pj := range test.prowJobs {
				npj := prowapi.ProwJob{
					Spec: prowapi.ProwJobSpec{
						Job:     pj.job,
						Context: pj.job,
						Type:    prowapi.BatchJob,
						Refs:    new(prowapi.Refs),
					},
					Status: prowapi.ProwJobStatus{State: pj.state},
				}
				for _, pr := range pj.prs {
					npj.Spec.Refs.Pulls = append(npj.Spec.Refs.Pulls, prowapi.Pull{
						Number: pr.number,
						SHA:    pr.sha,
					})
				}
				pjs = append(pjs, npj)
			}
			for idx := range test.presubmits {
				test.presubmits[idx].AlwaysRun = true
			}

			inrepoconfig := config.InRepoConfig{}
			if test.prowYAMLGetter != nil {
				inrepoconfig.Enabled = map[string]*bool{"*": utilpointer.Bool(true)}
			}
			cfg := func() *config.Config {
				return &config.Config{
					JobConfig: config.JobConfig{
						PresubmitsStatic: map[string][]config.Presubmit{
							"org/repo": test.presubmits,
						},
						ProwYAMLGetterWithDefaults: test.prowYAMLGetter,
					},
					ProwConfig: config.ProwConfig{
						InRepoConfig: inrepoconfig,
					},
				}
			}
			c := &syncController{
				config:       cfg,
				provider:     newGitHubProvider(logrus.WithContext(context.Background()), nil, nil, cfg, nil, false),
				changedFiles: &changedFilesAgent{},
				logger:       logrus.WithField("test", test.name),
			}
			merges, pending := c.accumulateBatch(subpool{org: "org", repo: "repo", prs: pulls, pjs: pjs, log: logrus.WithField("test", test.name)})
			if (len(pending) > 0) != test.pending {
				t.Errorf("For case \"%s\", got wrong pending.", test.name)
			}
			testPullsMatchList(t, test.name, merges, test.merges)
		})
	}
}

func TestAccumulate(t *testing.T) {

	const baseSHA = "8d287a3aeae90fd0aef4a70009c715712ff302cd"
	jobSet := []config.Presubmit{
		{
			Reporter: config.Reporter{
				Context: "job1",
			},
		},
		{
			Reporter: config.Reporter{
				Context: "job2",
			},
		},
	}
	type prowjob struct {
		prNumber int
		job      string
		state    prowapi.ProwJobState
		sha      string
	}
	tests := []struct {
		name                string
		presubmits          map[int][]config.Presubmit
		pullRequests        map[int]string
		pullRequestModifier func(*PullRequest)
		prowJobs            []prowjob

		successes []int
		pendings  []int
		none      []int
	}{
		{
			pullRequests: map[int]string{1: "", 2: "", 3: "", 4: "", 5: "", 6: "", 7: ""},
			presubmits: map[int][]config.Presubmit{
				1: jobSet,
				2: jobSet,
				3: jobSet,
				4: jobSet,
				5: jobSet,
				6: jobSet,
				7: jobSet,
			},
			prowJobs: []prowjob{
				{2, "job1", prowapi.PendingState, ""},
				{3, "job1", prowapi.PendingState, ""},
				{3, "job2", prowapi.TriggeredState, ""},
				{4, "job1", prowapi.FailureState, ""},
				{4, "job2", prowapi.PendingState, ""},
				{5, "job1", prowapi.PendingState, ""},
				{5, "job2", prowapi.FailureState, ""},
				{5, "job2", prowapi.PendingState, ""},
				{6, "job1", prowapi.SuccessState, ""},
				{6, "job2", prowapi.PendingState, ""},
				{7, "job1", prowapi.SuccessState, ""},
				{7, "job2", prowapi.SuccessState, ""},
				{7, "job1", prowapi.FailureState, ""},
			},

			successes: []int{7},
			pendings:  []int{3, 5, 6},
			none:      []int{1, 2, 4},
		},
		{
			pullRequests: map[int]string{7: ""},
			presubmits: map[int][]config.Presubmit{
				7: {
					{Reporter: config.Reporter{Context: "job1"}},
					{Reporter: config.Reporter{Context: "job2"}},
					{Reporter: config.Reporter{Context: "job3"}},
					{Reporter: config.Reporter{Context: "job4"}},
				},
			},
			prowJobs: []prowjob{
				{7, "job1", prowapi.SuccessState, ""},
				{7, "job2", prowapi.FailureState, ""},
				{7, "job3", prowapi.FailureState, ""},
				{7, "job4", prowapi.FailureState, ""},
				{7, "job3", prowapi.FailureState, ""},
				{7, "job4", prowapi.FailureState, ""},
				{7, "job2", prowapi.SuccessState, ""},
				{7, "job3", prowapi.SuccessState, ""},
				{7, "job4", prowapi.FailureState, ""},
			},

			successes: []int{},
			pendings:  []int{},
			none:      []int{7},
		},
		{
			pullRequests: map[int]string{7: ""},
			presubmits: map[int][]config.Presubmit{
				7: {
					{Reporter: config.Reporter{Context: "job1"}},
					{Reporter: config.Reporter{Context: "job2"}},
					{Reporter: config.Reporter{Context: "job3"}},
					{Reporter: config.Reporter{Context: "job4"}},
				},
			},
			prowJobs: []prowjob{
				{7, "job1", prowapi.FailureState, ""},
				{7, "job2", prowapi.FailureState, ""},
				{7, "job3", prowapi.FailureState, ""},
				{7, "job4", prowapi.FailureState, ""},
				{7, "job3", prowapi.FailureState, ""},
				{7, "job4", prowapi.FailureState, ""},
				{7, "job2", prowapi.FailureState, ""},
				{7, "job3", prowapi.FailureState, ""},
				{7, "job4", prowapi.FailureState, ""},
			},

			successes: []int{},
			pendings:  []int{},
			none:      []int{7},
		},
		{
			pullRequests: map[int]string{7: ""},
			presubmits: map[int][]config.Presubmit{
				7: {
					{Reporter: config.Reporter{Context: "job1"}},
					{Reporter: config.Reporter{Context: "job2"}},
					{Reporter: config.Reporter{Context: "job3"}},
					{Reporter: config.Reporter{Context: "job4"}},
				},
			},
			prowJobs: []prowjob{
				{7, "job1", prowapi.SuccessState, ""},
				{7, "job2", prowapi.FailureState, ""},
				{7, "job3", prowapi.FailureState, ""},
				{7, "job4", prowapi.FailureState, ""},
				{7, "job3", prowapi.FailureState, ""},
				{7, "job4", prowapi.FailureState, ""},
				{7, "job2", prowapi.SuccessState, ""},
				{7, "job3", prowapi.SuccessState, ""},
				{7, "job4", prowapi.SuccessState, ""},
				{7, "job1", prowapi.FailureState, ""},
			},

			successes: []int{7},
			pendings:  []int{},
			none:      []int{},
		},
		{
			pullRequests: map[int]string{7: ""},
			presubmits: map[int][]config.Presubmit{
				7: {
					{Reporter: config.Reporter{Context: "job1"}},
					{Reporter: config.Reporter{Context: "job2"}},
					{Reporter: config.Reporter{Context: "job3"}},
					{Reporter: config.Reporter{Context: "job4"}},
				},
			},
			prowJobs: []prowjob{
				{7, "job1", prowapi.SuccessState, ""},
				{7, "job2", prowapi.FailureState, ""},
				{7, "job3", prowapi.FailureState, ""},
				{7, "job4", prowapi.FailureState, ""},
				{7, "job3", prowapi.FailureState, ""},
				{7, "job4", prowapi.FailureState, ""},
				{7, "job2", prowapi.SuccessState, ""},
				{7, "job3", prowapi.SuccessState, ""},
				{7, "job4", prowapi.PendingState, ""},
				{7, "job1", prowapi.FailureState, ""},
			},

			successes: []int{},
			pendings:  []int{7},
			none:      []int{},
		},
		{
			presubmits: map[int][]config.Presubmit{
				7: {
					{Reporter: config.Reporter{Context: "job1"}},
				},
			},
			pullRequests: map[int]string{7: "new", 8: "new"},
			prowJobs: []prowjob{
				{7, "job1", prowapi.SuccessState, "old"},
				{7, "job1", prowapi.FailureState, "new"},
				{8, "job1", prowapi.FailureState, "old"},
				{8, "job1", prowapi.SuccessState, "new"},
			},

			successes: []int{8},
			pendings:  []int{},
			none:      []int{7},
		},
		{
			pullRequests: map[int]string{7: "new", 8: "new"},
			prowJobs:     []prowjob{},

			successes: []int{8, 7},
			pendings:  []int{},
			none:      []int{},
		},
		{
			name:         "Results from successful status context for which we do not have a prowjob anymore are considered",
			presubmits:   map[int][]config.Presubmit{1: {{Reporter: config.Reporter{Context: "job1"}}}},
			pullRequests: map[int]string{1: "headsha"},
			pullRequestModifier: func(pr *PullRequest) {
				pr.Commits.Nodes = []struct{ Commit Commit }{{
					Commit: Commit{
						OID: githubql.String("headsha"),
						Status: CommitStatus{Contexts: []Context{{
							Context:     githubql.String("job1"),
							Description: githubql.String("Job succeeded. BaseSHA:" + baseSHA),
							State:       githubql.StatusStateSuccess,
						}}}},
				}}
			},

			successes: []int{1},
		},
		{
			name:         "Results from successful status context for wrong baseSHA is ignored",
			presubmits:   map[int][]config.Presubmit{1: {{Reporter: config.Reporter{Context: "job1"}}}},
			pullRequests: map[int]string{1: "headsha"},
			pullRequestModifier: func(pr *PullRequest) {
				pr.Commits.Nodes = []struct{ Commit Commit }{{
					Commit: Commit{
						OID: githubql.String("headsha"),
						Status: CommitStatus{Contexts: []Context{{
							Context:     githubql.String("job1"),
							Description: githubql.String("Job succeeded. BaseSHA:c22a32add1a36daf3b16af3762b3922e70c9626a"),
							State:       githubql.StatusStateSuccess,
						}}}},
				}}
			},

			none: []int{1},
		},
		{
			name:         "Results from failed status context for which we do not have a prowjob anymore are irrelevant",
			presubmits:   map[int][]config.Presubmit{1: {{Reporter: config.Reporter{Context: "job1"}}}},
			pullRequests: map[int]string{1: "headsha"},
			pullRequestModifier: func(pr *PullRequest) {
				pr.Commits.Nodes = []struct{ Commit Commit }{{
					Commit: Commit{
						OID: githubql.String("headsha"),
						Status: CommitStatus{Contexts: []Context{{
							Context:     githubql.String("job1"),
							Description: githubql.String("Job succeeded. BaseSHA:" + baseSHA),
							State:       githubql.StatusStateFailure,
						}}}},
				}}
			},

			none: []int{1},
		},
		{
			name:         "Successful status context and prowjob, success",
			presubmits:   map[int][]config.Presubmit{1: {{Reporter: config.Reporter{Context: "job1"}}}},
			pullRequests: map[int]string{1: "headsha"},
			pullRequestModifier: func(pr *PullRequest) {
				pr.Commits.Nodes = []struct{ Commit Commit }{{
					Commit: Commit{
						OID: githubql.String("headsha"),
						Status: CommitStatus{Contexts: []Context{{
							Context:     githubql.String("job1"),
							Description: githubql.String("Job succeeded. BaseSHA:" + baseSHA),
							State:       githubql.StatusStateSuccess,
						}}}},
				}}
			},
			prowJobs: []prowjob{{1, "job1", prowapi.SuccessState, "headsha"}},

			successes: []int{1},
		},
		{
			name:         "Successful status context, failed prowjob, success",
			presubmits:   map[int][]config.Presubmit{1: {{Reporter: config.Reporter{Context: "job1"}}}},
			pullRequests: map[int]string{1: "headsha"},
			pullRequestModifier: func(pr *PullRequest) {
				pr.Commits.Nodes = []struct{ Commit Commit }{{
					Commit: Commit{
						OID: githubql.String("headsha"),
						Status: CommitStatus{Contexts: []Context{{
							Context:     githubql.String("job1"),
							Description: githubql.String("Job succeeded. BaseSHA:" + baseSHA),
							State:       githubql.StatusStateSuccess,
						}}}},
				}}
			},
			prowJobs: []prowjob{{1, "job1", prowapi.FailureState, "headsha"}},

			successes: []int{1},
		},
		{
			name:         "Failed status context, successful prowjob, success",
			presubmits:   map[int][]config.Presubmit{1: {{Reporter: config.Reporter{Context: "job1"}}}},
			pullRequests: map[int]string{1: "headsha"},
			pullRequestModifier: func(pr *PullRequest) {
				pr.Commits.Nodes = []struct{ Commit Commit }{{
					Commit: Commit{
						OID: githubql.String("headsha"),
						Status: CommitStatus{Contexts: []Context{{
							Context:     githubql.String("job1"),
							Description: githubql.String("Job succeeded. BaseSHA:" + baseSHA),
							State:       githubql.StatusStateFailure,
						}}}},
				}}
			},
			prowJobs: []prowjob{{1, "job1", prowapi.SuccessState, "headsha"}},

			successes: []int{1},
		},
		{
			name:         "Failed status context and prowjob, failure",
			presubmits:   map[int][]config.Presubmit{1: {{Reporter: config.Reporter{Context: "job1"}}}},
			pullRequests: map[int]string{1: "headsha"},
			pullRequestModifier: func(pr *PullRequest) {
				pr.Commits.Nodes = []struct{ Commit Commit }{{
					Commit: Commit{
						OID: githubql.String("headsha"),
						Status: CommitStatus{Contexts: []Context{{
							Context:     githubql.String("job1"),
							Description: githubql.String("Job succeeded. BaseSHA:" + baseSHA),
							State:       githubql.StatusStateFailure,
						}}}},
				}}
			},
			prowJobs: []prowjob{{1, "job1", prowapi.FailureState, "headsha"}},

			none: []int{1},
		},
		{
			name: "Mixture of results from status context and prowjobs",
			presubmits: map[int][]config.Presubmit{1: {
				{Reporter: config.Reporter{Context: "job1"}},
				{Reporter: config.Reporter{Context: "job2"}},
			}},
			pullRequests: map[int]string{1: "headsha"},
			pullRequestModifier: func(pr *PullRequest) {
				pr.Commits.Nodes = []struct{ Commit Commit }{{
					Commit: Commit{
						OID: githubql.String("headsha"),
						Status: CommitStatus{Contexts: []Context{{
							Context:     githubql.String("job1"),
							Description: githubql.String("Job succeeded. BaseSHA:" + baseSHA),
							State:       githubql.StatusStateSuccess,
						}}}},
				}}
			},
			prowJobs: []prowjob{{1, "job2", prowapi.SuccessState, "headsha"}},

			successes: []int{1},
		},
	}

	for i, test := range tests {
		if test.name == "" {
			test.name = strconv.Itoa(i)
		}
		t.Run(test.name, func(t *testing.T) {
			syncCtrl := &syncController{
				provider: &GitHubProvider{ghc: &fgc{}, logger: logrus.NewEntry(logrus.New())},
				logger:   logrus.NewEntry(logrus.New()),
			}
			var pulls []CodeReviewCommon
			for num, sha := range test.pullRequests {
				newPull := PullRequest{Number: githubql.Int(num), HeadRefOID: githubql.String(sha)}
				if test.pullRequestModifier != nil {
					test.pullRequestModifier(&newPull)
				}
				pulls = append(pulls, *CodeReviewCommonFromPullRequest(&newPull))
			}
			var pjs []prowapi.ProwJob
			for _, pj := range test.prowJobs {
				pjs = append(pjs, prowapi.ProwJob{
					Spec: prowapi.ProwJobSpec{
						Job:     pj.job,
						Context: pj.job,
						Type:    prowapi.PresubmitJob,
						Refs:    &prowapi.Refs{Pulls: []prowapi.Pull{{Number: pj.prNumber, SHA: pj.sha}}},
					},
					Status: prowapi.ProwJobStatus{State: pj.state},
				})
			}

			successes, pendings, nones, _ := syncCtrl.accumulate(test.presubmits, pulls, pjs, baseSHA)

			t.Logf("test run %d", i)
			testPullsMatchList(t, "successes", successes, test.successes)
			testPullsMatchList(t, "pendings", pendings, test.pendings)
			testPullsMatchList(t, "nones", nones, test.none)
		})
	}
}

type fgc struct {
	err  error
	lock sync.Mutex

	prs        map[string][]PullRequest
	refs       map[string]string
	merged     int
	setStatus  bool
	statuses   map[string]github.Status
	mergeErrs  map[int]error
	queryCalls int

	expectedSHA          string
	skipExpectedShaCheck bool
	combinedStatus       map[string]string
	checkRuns            *github.CheckRunList
}

func (f *fgc) GetRepo(o, r string) (github.FullRepo, error) {
	repo := github.FullRepo{}
	if strings.Contains(r, "squash") {
		repo.AllowSquashMerge = true
	}
	if strings.Contains(r, "rebase") {
		repo.AllowRebaseMerge = true
	}
	if !strings.Contains(r, "nomerge") {
		repo.AllowMergeCommit = true
	}
	return repo, nil
}

func (f *fgc) GetRef(o, r, ref string) (string, error) {
	return f.refs[o+"/"+r+" "+ref], f.err
}

func (f *fgc) QueryWithGitHubAppsSupport(ctx context.Context, q interface{}, vars map[string]interface{}, org string) error {
	sq, ok := q.(*searchQuery)
	if !ok {
		return errors.New("unexpected query type")
	}

	f.lock.Lock()
	defer f.lock.Unlock()
	f.queryCalls++

	for _, pr := range f.prs[org] {
		sq.Search.Nodes = append(
			sq.Search.Nodes,
			struct {
				PullRequest PullRequest `graphql:"... on PullRequest"`
			}{PullRequest: pr},
		)
	}
	return nil
}

func (f *fgc) Merge(org, repo string, number int, details github.MergeDetails) error {
	if err, ok := f.mergeErrs[number]; ok {
		return err
	}
	f.merged++
	return nil
}

func (f *fgc) CreateStatus(org, repo, ref string, s github.Status) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	switch s.State {
	case github.StatusSuccess, github.StatusError, github.StatusPending, github.StatusFailure:
		if f.statuses == nil {
			f.statuses = map[string]github.Status{}
		}
		f.statuses[org+"/"+repo+"/"+ref] = s
		f.setStatus = true
		return nil
	}
	return fmt.Errorf("invalid 'state' value: %q", s.State)
}

func (f *fgc) GetCombinedStatus(org, repo, ref string) (*github.CombinedStatus, error) {
	if !f.skipExpectedShaCheck && f.expectedSHA != ref {
		return nil, errors.New("bad combined status request: incorrect sha")
	}
	var statuses []github.Status
	for c, s := range f.combinedStatus {
		statuses = append(statuses, github.Status{Context: c, State: s})
	}
	return &github.CombinedStatus{
			Statuses: statuses,
		},
		nil
}

func (f *fgc) ListCheckRuns(org, repo, ref string) (*github.CheckRunList, error) {
	if !f.skipExpectedShaCheck && f.expectedSHA != ref {
		return nil, errors.New("bad combined status request: incorrect sha")
	}
	if f.checkRuns != nil {
		return f.checkRuns, nil
	}
	return &github.CheckRunList{}, nil
}

func (f *fgc) GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error) {
	if number != 100 {
		return nil, nil
	}
	return []github.PullRequestChange{
			{
				Filename: "CHANGED",
			},
		},
		nil
}

// TestDividePool ensures that subpools returned by dividePool satisfy a few
// important invariants.
func TestDividePool(t *testing.T) {
	testPulls := []struct {
		org    string
		repo   string
		number int
		branch string
	}{
		{
			org:    "k",
			repo:   "t-i",
			number: 5,
			branch: defaultBranch,
		},
		{
			org:    "k",
			repo:   "t-i",
			number: 6,
			branch: defaultBranch,
		},
		{
			org:    "k",
			repo:   "k",
			number: 123,
			branch: defaultBranch,
		},
		{
			org:    "k",
			repo:   "k",
			number: 1000,
			branch: "release-1.6",
		},
	}
	testPJs := []struct {
		jobType prowapi.ProwJobType
		org     string
		repo    string
		baseRef string
		baseSHA string
	}{
		{
			jobType: prowapi.PresubmitJob,
			org:     "k",
			repo:    "t-i",
			baseRef: defaultBranch,
			baseSHA: "123",
		},
		{
			jobType: prowapi.BatchJob,
			org:     "k",
			repo:    "t-i",
			baseRef: defaultBranch,
			baseSHA: "123",
		},
		{
			jobType: prowapi.PeriodicJob,
		},
		{
			jobType: prowapi.PresubmitJob,
			org:     "k",
			repo:    "t-i",
			baseRef: "patch",
			baseSHA: "123",
		},
		{
			jobType: prowapi.PresubmitJob,
			org:     "k",
			repo:    "t-i",
			baseRef: defaultBranch,
			baseSHA: "abc",
		},
		{
			jobType: prowapi.PresubmitJob,
			org:     "o",
			repo:    "t-i",
			baseRef: defaultBranch,
			baseSHA: "123",
		},
		{
			jobType: prowapi.PresubmitJob,
			org:     "k",
			repo:    "other",
			baseRef: defaultBranch,
			baseSHA: "123",
		},
	}
	fc := &fgc{
		refs: map[string]string{
			"k/t-i heads/master":    "123",
			"k/k heads/master":      "456",
			"k/k heads/release-1.6": "789",
		},
	}

	configGetter := func() *config.Config {
		return &config.Config{
			ProwConfig: config.ProwConfig{
				ProwJobNamespace: "default",
			},
		}
	}

	mmc := newMergeChecker(configGetter, fc)
	log := logrus.NewEntry(logrus.StandardLogger())
	ghProvider := newGitHubProvider(log, fc, nil, configGetter, mmc, false)
	mgr := newFakeManager()
	c, err := newSyncController(
		context.Background(),
		log,
		mgr,
		ghProvider,
		configGetter,
		nil,
		nil,
		false,
		&statusUpdate{
			dontUpdateStatus: &threadSafePRSet{},
			newPoolPending:   make(chan bool),
		},
	)
	if err != nil {
		t.Fatalf("failed to construct sync controller: %v", err)
	}
	for idx, pj := range testPJs {
		prowjob := &prowapi.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pj-%d", idx),
				Namespace: "default",
			},
			Spec: prowapi.ProwJobSpec{
				Type: pj.jobType,
				Refs: &prowapi.Refs{
					Org:     pj.org,
					Repo:    pj.repo,
					BaseRef: pj.baseRef,
					BaseSHA: pj.baseSHA,
				},
			},
		}
		if err := mgr.GetClient().Create(context.Background(), prowjob); err != nil {
			t.Fatalf("failed to create prowjob: %v", err)
		}
	}
	pulls := make(map[string]CodeReviewCommon)
	for _, p := range testPulls {
		npr := PullRequest{Number: githubql.Int(p.number)}
		npr.BaseRef.Name = githubql.String(p.branch)
		npr.BaseRef.Prefix = "refs/heads/"
		npr.Repository.Name = githubql.String(p.repo)
		npr.Repository.Owner.Login = githubql.String(p.org)
		crc := CodeReviewCommonFromPullRequest(&npr)
		pulls[prKey(crc)] = *crc
	}
	sps, err := c.dividePool(pulls)
	if err != nil {
		t.Fatalf("Error dividing pool: %v", err)
	}
	if len(sps) == 0 {
		t.Error("No subpools.")
	}
	for _, sp := range sps {
		name := fmt.Sprintf("%s/%s %s", sp.org, sp.repo, sp.branch)
		sha := fc.refs[sp.org+"/"+sp.repo+" heads/"+sp.branch]
		if sp.sha != sha {
			t.Errorf("For subpool %s, got sha %q, expected %q.", name, sp.sha, sha)
		}
		if len(sp.prs) == 0 {
			t.Errorf("Subpool %s has no PRs.", name)
		}
		for _, pr := range sp.prs {
			if pr.Org != sp.org || pr.Repo != sp.repo || pr.BaseRefName != sp.branch {
				t.Errorf("PR in wrong subpool. Got PR %+v in subpool %s.", pr, name)
			}
		}
		for _, pj := range sp.pjs {
			if pj.Spec.Type != prowapi.PresubmitJob && pj.Spec.Type != prowapi.BatchJob {
				t.Errorf("PJ with bad type in subpool %s: %+v", name, pj)
			}
			referenceRef := &prowapi.Refs{
				Org:     sp.org,
				Repo:    sp.repo,
				BaseRef: sp.branch,
				BaseSHA: sp.sha,
			}
			if diff := deep.Equal(pj.Spec.Refs, referenceRef); diff != nil {
				t.Errorf("Got PJ with wrong refs, diff: %v", diff)
			}
		}
	}
}

func TestPickBatchV2(t *testing.T) {
	testPickBatch(localgit.NewV2, t)
}

func testPickBatch(clients localgit.Clients, t *testing.T) {
	lg, gc, err := clients()
	if err != nil {
		t.Fatalf("Error making local git: %v", err)
	}
	defer gc.Clean()
	defer lg.Clean()
	if err := lg.MakeFakeRepo("o", "r"); err != nil {
		t.Fatalf("Error making fake repo: %v", err)
	}
	if err := lg.AddCommit("o", "r", map[string][]byte{"foo": []byte("foo")}); err != nil {
		t.Fatalf("Adding initial commit: %v", err)
	}
	testprs := []struct {
		files   map[string][]byte
		success bool
		number  int

		included bool
	}{
		{
			files:    map[string][]byte{"bar": []byte("ok")},
			success:  true,
			number:   0,
			included: true,
		},
		{
			files:    map[string][]byte{"foo": []byte("ok")},
			success:  true,
			number:   1,
			included: true,
		},
		{
			files:    map[string][]byte{"bar": []byte("conflicts with 0")},
			success:  true,
			number:   2,
			included: false,
		},
		{
			files:    map[string][]byte{"something": []byte("ok")},
			success:  true,
			number:   3,
			included: true,
		},
		{
			files:    map[string][]byte{"changes": []byte("ok")},
			success:  true,
			number:   4,
			included: true,
		},
		{
			files:    map[string][]byte{"other": []byte("ok")},
			success:  true,
			number:   5,
			included: false, // excluded by context policy
		},
		{
			files:    map[string][]byte{"qux": []byte("ok")},
			success:  false,
			number:   6,
			included: false,
		},
		{
			files:    map[string][]byte{"bazel": []byte("ok")},
			success:  true,
			number:   7,
			included: true,
		},
		{
			files:    map[string][]byte{"bazel": []byte("ok")},
			success:  true,
			number:   8,
			included: false, // batch of 5 smallest excludes this
		},
	}
	sp := subpool{
		log:    logrus.WithField("component", "tide"),
		org:    "o",
		repo:   "r",
		branch: defaultBranch,
		sha:    defaultBranch,
	}
	for _, testpr := range testprs {
		if err := lg.CheckoutNewBranch("o", "r", fmt.Sprintf("pr-%d", testpr.number)); err != nil {
			t.Fatalf("Error checking out new branch: %v", err)
		}
		if err := lg.AddCommit("o", "r", testpr.files); err != nil {
			t.Fatalf("Error adding commit: %v", err)
		}
		if err := lg.Checkout("o", "r", defaultBranch); err != nil {
			t.Fatalf("Error checking out master: %v", err)
		}
		oid := githubql.String(fmt.Sprintf("origin/pr-%d", testpr.number))
		var pr PullRequest
		pr.Number = githubql.Int(testpr.number)
		pr.HeadRefOID = oid
		pr.Commits.Nodes = []struct {
			Commit Commit
		}{{Commit: Commit{OID: oid}}}
		pr.Commits.Nodes[0].Commit.Status.Contexts = append(pr.Commits.Nodes[0].Commit.Status.Contexts, Context{State: githubql.StatusStateSuccess})
		if !testpr.success {
			pr.Commits.Nodes[0].Commit.Status.Contexts[0].State = githubql.StatusStateFailure
		}
		sp.prs = append(sp.prs, *CodeReviewCommonFromPullRequest(&pr))
	}
	ca := &config.Agent{}
	ca.Set(&config.Config{
		ProwConfig: config.ProwConfig{
			Tide: config.Tide{
				BatchSizeLimitMap: map[string]int{"*": 5},
			},
		},
		JobConfig: config.JobConfig{
			PresubmitsStatic: map[string][]config.Presubmit{
				"o/r": {{
					AlwaysRun: true,
					JobBase: config.JobBase{
						Name: "my-presubmit",
					},
				}},
			},
		},
	})
	logger := logrus.WithField("component", "tide")
	ghProvider := &GitHubProvider{cfg: ca.Config, gc: gc, mergeChecker: newMergeChecker(ca.Config, &fgc{}), logger: logger}
	c := &syncController{
		logger:       logger,
		provider:     ghProvider,
		config:       ca.Config,
		pickNewBatch: pickNewBatch(gc, ca.Config, ghProvider),
	}
	prs, presubmits, err := c.pickBatch(sp, map[int]contextChecker{
		0: &config.TideContextPolicy{},
		1: &config.TideContextPolicy{},
		2: &config.TideContextPolicy{},
		3: &config.TideContextPolicy{},
		4: &config.TideContextPolicy{},
		// Test if scoping of ContextPolicy works correctly
		5: &config.TideContextPolicy{RequiredContexts: []string{"context-from-context-checker"}},
		6: &config.TideContextPolicy{},
		7: &config.TideContextPolicy{},
		8: &config.TideContextPolicy{},
	}, c.pickNewBatch)
	if err != nil {
		t.Fatalf("Error from pickBatch: %v", err)
	}
	if !apiequality.Semantic.DeepEqual(presubmits, ca.Config().PresubmitsStatic["o/r"]) {
		t.Errorf("resolving presubmits failed, diff:\n%v\n", diff.ObjectReflectDiff(presubmits, ca.Config().PresubmitsStatic["o/r"]))
	}
	for _, testpr := range testprs {
		var found bool
		for _, pr := range prs {
			if int(pr.Number) == testpr.number {
				found = true
				break
			}
		}
		if found && !testpr.included {
			t.Errorf("PR %d should not be picked.", testpr.number)
		} else if !found && testpr.included {
			t.Errorf("PR %d should be picked.", testpr.number)
		}
	}
}

func TestMergeMethodCheckerAndPRMergeMethod(t *testing.T) {
	squashLabel := "tide/squash"
	mergeLabel := "tide/merge"
	rebaseLabel := "tide/rebase"

	tideConfig := config.Tide{
		TideGitHubConfig: config.TideGitHubConfig{
			SquashLabel: squashLabel,
			MergeLabel:  mergeLabel,
			RebaseLabel: rebaseLabel,

			MergeType: map[string]config.TideOrgMergeType{
				"o/configured-rebase":              {MergeType: types.MergeRebase}, // GH client allows merge, rebase
				"o/configured-squash-allow-rebase": {MergeType: types.MergeSquash}, // GH client allows merge, squash, rebase
				"o/configure-re-base":              {MergeType: types.MergeRebase}, // GH client allows merge
			},
		},
	}
	cfg := func() *config.Config { return &config.Config{ProwConfig: config.ProwConfig{Tide: tideConfig}} }
	mmc := newMergeChecker(cfg, &fgc{})

	testcases := []struct {
		name              string
		repo              string
		labels            []string
		conflict          bool
		expectedMethod    types.PullRequestMergeType
		expectErr         bool
		expectConflictErr bool
	}{
		{
			name:           "default method without PR label override",
			repo:           "foo",
			expectedMethod: types.MergeMerge,
		},
		{
			name:           "irrelevant PR labels ignored",
			repo:           "foo",
			labels:         []string{"unrelated"},
			expectedMethod: types.MergeMerge,
		},
		{
			name:           "default method overridden by a PR label",
			repo:           "allow-squash-nomerge",
			labels:         []string{"tide/squash"},
			expectedMethod: types.MergeSquash,
		},
		{
			name:           "use method configured for repo in tide config",
			repo:           "configured-squash-allow-rebase",
			labels:         []string{"unrelated"},
			expectedMethod: types.MergeSquash,
		},
		{
			name:           "tide config method overridden by a PR label",
			repo:           "configured-squash-allow-rebase",
			labels:         []string{"unrelated", "tide/rebase"},
			expectedMethod: types.MergeRebase,
		},
		{
			name:      "multiple merge method PR labels should not merge",
			repo:      "foo",
			labels:    []string{"tide/squash", "tide/rebase"},
			expectErr: true,
		},
		{
			name:              "merge conflict",
			repo:              "foo",
			labels:            []string{"unrelated"},
			conflict:          true,
			expectedMethod:    types.MergeMerge,
			expectErr:         false,
			expectConflictErr: true,
		},
		{
			name:              "squash label conflicts with merge only GH settings",
			repo:              "foo",
			labels:            []string{"tide/squash"},
			expectedMethod:    types.MergeSquash,
			expectErr:         false,
			expectConflictErr: true,
		},
		{
			name:              "rebase method tide config conflicts with merge only GH settings",
			repo:              "configure-re-base",
			labels:            []string{"unrelated"},
			expectedMethod:    types.MergeRebase,
			expectErr:         false,
			expectConflictErr: true,
		},
		{
			name:              "default method conflicts with squash only GH settings",
			repo:              "squash-nomerge",
			labels:            []string{"unrelated"},
			expectedMethod:    types.MergeMerge,
			expectErr:         false,
			expectConflictErr: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			pr := &PullRequest{
				Repository: struct {
					Name          githubql.String
					NameWithOwner githubql.String
					Owner         struct {
						Login githubql.String
					}
				}{
					Name: githubql.String(tc.repo),
					Owner: struct {
						Login githubql.String
					}{
						Login: githubql.String("o"),
					},
				},
				Labels: struct {
					Nodes []struct{ Name githubql.String }
				}{
					Nodes: []struct{ Name githubql.String }{},
				},
				CanBeRebased: true,
			}
			for _, label := range tc.labels {
				labelNode := struct{ Name githubql.String }{Name: githubql.String(label)}
				pr.Labels.Nodes = append(pr.Labels.Nodes, labelNode)
			}
			if tc.conflict {
				pr.Mergeable = githubql.MergeableStateConflicting
			}

			actual := mmc.prMergeMethod(tideConfig, CodeReviewCommonFromPullRequest(pr))
			if actual == nil {
				if !tc.expectErr {
					t.Errorf("multiple merge methods are not allowed")
				}
				return
			} else if tc.expectErr {
				t.Errorf("missing expected error")
				return
			}
			if tc.expectedMethod != *actual {
				t.Errorf("wanted: %q, got: %q", tc.expectedMethod, *actual)
			}
			reason, err := mmc.isAllowedToMerge(CodeReviewCommonFromPullRequest(pr))
			if err != nil {
				t.Errorf("unexpected processing error: %v", err)
			} else if reason != "" {
				if !tc.expectConflictErr {
					t.Errorf("unexpected merge method conflict error: %v", err)
				}
				return
			} else if tc.expectConflictErr {
				t.Errorf("missing expected merge method conflict error")
				return
			}
		})
	}
}

func TestRebaseMergeMethodIsAllowed(t *testing.T) {
	orgName := "fake-org"
	repoName := "fake-repo"
	tideConfig := config.Tide{
		TideGitHubConfig: config.TideGitHubConfig{
			MergeType: map[string]config.TideOrgMergeType{
				fmt.Sprintf("%s/%s", orgName, repoName): {MergeType: types.MergeRebase},
			},
		},
	}
	cfg := func() *config.Config { return &config.Config{ProwConfig: config.ProwConfig{Tide: tideConfig}} }
	mmc := newMergeChecker(cfg, &fgc{})
	mmc.cache = map[config.OrgRepo]map[types.PullRequestMergeType]bool{
		{Org: orgName, Repo: repoName}: {
			types.MergeRebase: true,
		},
	}

	testCases := []struct {
		name                string
		expectedMergeOutput string
		prCanBeRebased      bool
	}{
		{
			name:                "Merging PR using rebase successfully",
			expectedMergeOutput: "",
			prCanBeRebased:      true,
		},
		{
			name:                "Merging PR using rebase but it is not allowed",
			expectedMergeOutput: "PR can't be rebased",
			prCanBeRebased:      false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pr := &PullRequest{
				Repository: struct {
					Name          githubql.String
					NameWithOwner githubql.String
					Owner         struct {
						Login githubql.String
					}
				}{
					Name: githubql.String(repoName),
					Owner: struct {
						Login githubql.String
					}{
						Login: githubql.String(orgName),
					},
				},
				Labels: struct {
					Nodes []struct{ Name githubql.String }
				}{
					Nodes: []struct{ Name githubql.String }{},
				},
				CanBeRebased: githubql.Boolean(tc.prCanBeRebased),
			}

			mergeOutput, err := mmc.isAllowedToMerge(CodeReviewCommonFromPullRequest(pr))
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			} else {
				if mergeOutput != tc.expectedMergeOutput {
					t.Errorf("Expected merge output \"%s\" but got \"%s\"\n", tc.expectedMergeOutput, mergeOutput)
				}
			}
		})
	}
}

func TestTakeActionV2(t *testing.T) {
	testTakeAction(localgit.NewV2, t)
}

func testTakeAction(clients localgit.Clients, t *testing.T) {
	sleep = func(time.Duration) {}
	defer func() { sleep = time.Sleep }()

	// PRs 0-9 exist. All are mergable, and all are passing tests.
	testcases := []struct {
		name string

		batchPending    bool
		successes       []int
		pendings        []int
		nones           []int
		batchMerges     []int
		presubmits      map[int][]config.Presubmit
		preExistingJobs []runtime.Object
		mergeErrs       map[int]error

		merged           int
		triggered        int
		triggeredBatches int
		action           Action
	}{
		{
			name: "no prs to test, should do nothing",

			batchPending: true,
			successes:    []int{},
			pendings:     []int{},
			nones:        []int{},
			batchMerges:  []int{},
			presubmits: map[int][]config.Presubmit{
				100: {
					{Reporter: config.Reporter{Context: "foo"}},
					{Reporter: config.Reporter{Context: "if-changed"}},
				},
			},
			merged:    0,
			triggered: 0,
			action:    Wait,
		},
		{
			name: "pending batch, pending serial, nothing to do",

			batchPending: true,
			successes:    []int{},
			pendings:     []int{1},
			nones:        []int{0, 2},
			batchMerges:  []int{},
			presubmits: map[int][]config.Presubmit{
				100: {
					{Reporter: config.Reporter{Context: "foo"}},
					{Reporter: config.Reporter{Context: "if-changed"}},
				},
			},
			merged:    0,
			triggered: 0,
			action:    Wait,
		},
		{
			name: "pending batch, successful serial, nothing to do",

			batchPending: true,
			successes:    []int{1},
			pendings:     []int{},
			nones:        []int{0, 2},
			batchMerges:  []int{},
			presubmits: map[int][]config.Presubmit{
				100: {
					{Reporter: config.Reporter{Context: "foo"}},
					{Reporter: config.Reporter{Context: "if-changed"}},
				},
			},
			merged:    0,
			triggered: 0,
			action:    Wait,
		},
		{
			name: "pending batch, should trigger serial",

			batchPending: true,
			successes:    []int{},
			pendings:     []int{},
			nones:        []int{0, 1, 2},
			batchMerges:  []int{},
			presubmits: map[int][]config.Presubmit{
				100: {
					{Reporter: config.Reporter{Context: "foo"}},
					{Reporter: config.Reporter{Context: "if-changed"}},
				},
			},
			merged:    0,
			triggered: 1,
			action:    Trigger,
		},
		{
			name: "no pending batch, should trigger batch",

			batchPending: false,
			successes:    []int{},
			pendings:     []int{0},
			nones:        []int{1, 2, 3},
			batchMerges:  []int{},
			presubmits: map[int][]config.Presubmit{
				100: {
					{Reporter: config.Reporter{Context: "foo"}},
					{Reporter: config.Reporter{Context: "if-changed"}},
				},
			},
			merged:           0,
			triggered:        2,
			triggeredBatches: 2,
			action:           TriggerBatch,
		},
		{
			name: "one PR, should not trigger batch",

			batchPending: false,
			successes:    []int{},
			pendings:     []int{},
			nones:        []int{0},
			batchMerges:  []int{},
			presubmits: map[int][]config.Presubmit{
				100: {
					{Reporter: config.Reporter{Context: "foo"}},
					{Reporter: config.Reporter{Context: "if-changed"}},
				},
			},
			merged:    0,
			triggered: 1,
			action:    Trigger,
		},
		{
			name: "successful PR, should merge",

			batchPending: false,
			successes:    []int{0},
			pendings:     []int{},
			nones:        []int{1, 2, 3},
			batchMerges:  []int{},
			presubmits: map[int][]config.Presubmit{
				100: {
					{Reporter: config.Reporter{Context: "foo"}},
					{Reporter: config.Reporter{Context: "if-changed"}},
				},
			},
			merged:    1,
			triggered: 0,
			action:    Merge,
		},
		{
			name: "successful batch, should merge",

			batchPending: false,
			successes:    []int{0, 1},
			pendings:     []int{2, 3},
			nones:        []int{4, 5},
			batchMerges:  []int{6, 7, 8},
			presubmits: map[int][]config.Presubmit{
				100: {
					{Reporter: config.Reporter{Context: "foo"}},
					{Reporter: config.Reporter{Context: "if-changed"}},
				},
			},
			merged:    3,
			triggered: 0,
			action:    MergeBatch,
		},
		{
			name: "one PR that triggers RunIfChangedJob",

			batchPending: false,
			successes:    []int{},
			pendings:     []int{},
			nones:        []int{100},
			batchMerges:  []int{},
			presubmits: map[int][]config.Presubmit{
				100: {
					{Reporter: config.Reporter{Context: "foo"}},
					{Reporter: config.Reporter{Context: "if-changed"}},
				},
			},
			merged:    0,
			triggered: 2,
			action:    Trigger,
		},
		{
			name: "no presubmits, merge",

			batchPending: false,
			successes:    []int{5, 4},
			pendings:     []int{},
			nones:        []int{},
			batchMerges:  []int{},

			merged:    1,
			triggered: 0,
			action:    Merge,
		},
		{
			name: "no presubmits, wait",

			batchPending: false,
			successes:    []int{},
			pendings:     []int{},
			nones:        []int{},
			batchMerges:  []int{},

			merged:    0,
			triggered: 0,
			action:    Wait,
		},
		{
			name: "no pending serial or batch, should trigger batch",

			batchPending: false,
			successes:    []int{},
			pendings:     []int{},
			nones:        []int{1, 2, 3},
			batchMerges:  []int{},
			presubmits: map[int][]config.Presubmit{
				100: {
					{Reporter: config.Reporter{Context: "foo"}},
					{Reporter: config.Reporter{Context: "if-changed"}},
				},
			},
			merged:           0,
			triggered:        2,
			triggeredBatches: 2,
			action:           TriggerBatch,
		},
		{
			name: "no pending serial or batch, should trigger batch and omit pre-existing running job",

			batchPending: false,
			successes:    []int{},
			pendings:     []int{},
			nones:        []int{1, 2, 3},
			batchMerges:  []int{},
			presubmits: map[int][]config.Presubmit{
				100: {
					{Reporter: config.Reporter{Context: "foo"}},
					{Reporter: config.Reporter{Context: "if-changed"}},
				},
			},
			preExistingJobs: []runtime.Object{&prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{Name: "my-job", Namespace: "pj-ns"},
				Spec: prowapi.ProwJobSpec{
					Job:  "bar",
					Type: prowapi.BatchJob,
					Refs: &prowapi.Refs{
						Org:     "o",
						Repo:    "r",
						BaseRef: defaultBranch,
						BaseSHA: defaultBranch,
						Pulls: []prowapi.Pull{
							{Number: 1, SHA: "origin/pr-1"},
							{Number: 3, SHA: "origin/pr-3"},
							{Number: 2, SHA: "origin/pr-2"},
						},
					},
				},
			}},
			merged:           0,
			triggered:        1,
			triggeredBatches: 1,
			action:           TriggerBatch,
		},
		{
			name: "no pending serial or batch, should trigger batch and omit pre-existing success job",

			batchPending: false,
			successes:    []int{},
			pendings:     []int{},
			nones:        []int{1, 2, 3},
			batchMerges:  []int{},
			presubmits: map[int][]config.Presubmit{
				100: {
					{Reporter: config.Reporter{Context: "foo"}},
					{Reporter: config.Reporter{Context: "if-changed"}},
				},
			},
			preExistingJobs: []runtime.Object{&prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{Name: "my-job", Namespace: "pj-ns"},
				Spec: prowapi.ProwJobSpec{
					Job:  "bar",
					Type: prowapi.BatchJob,
					Refs: &prowapi.Refs{
						Org:     "o",
						Repo:    "r",
						BaseRef: defaultBranch,
						BaseSHA: defaultBranch,
						Pulls: []prowapi.Pull{
							{Number: 1, SHA: "origin/pr-1"},
							{Number: 3, SHA: "origin/pr-3"},
							{Number: 2, SHA: "origin/pr-2"},
						},
					},
				},
				Status: prowapi.ProwJobStatus{
					State:          prowapi.SuccessState,
					CompletionTime: &metav1.Time{Time: time.Unix(10, 0)},
				},
			}},
			merged:           0,
			triggered:        1,
			triggeredBatches: 1,
			action:           TriggerBatch,
		},
		{
			name: "no pending serial or batch, should trigger batch and ignore pre-existing failure job",

			batchPending: false,
			successes:    []int{},
			pendings:     []int{},
			nones:        []int{1, 2, 3},
			batchMerges:  []int{},
			presubmits: map[int][]config.Presubmit{
				100: {
					{Reporter: config.Reporter{Context: "foo"}},
					{Reporter: config.Reporter{Context: "if-changed"}},
				},
			},
			preExistingJobs: []runtime.Object{&prowapi.ProwJob{
				ObjectMeta: metav1.ObjectMeta{Name: "my-job", Namespace: "pj-ns"},
				Spec: prowapi.ProwJobSpec{
					Job:  "bar",
					Type: prowapi.BatchJob,
					Refs: &prowapi.Refs{
						Org:     "o",
						Repo:    "r",
						BaseRef: defaultBranch,
						BaseSHA: defaultBranch,
						Pulls: []prowapi.Pull{
							{Number: 1, SHA: "origin/pr-1"},
							{Number: 3, SHA: "origin/pr-3"},
							{Number: 2, SHA: "origin/pr-2"},
						},
					},
				},
				Status: prowapi.ProwJobStatus{
					State:          prowapi.FailureState,
					CompletionTime: &metav1.Time{Time: time.Unix(10, 0)},
				},
			}},
			merged:           0,
			triggered:        2,
			triggeredBatches: 2,
			action:           TriggerBatch,
		},
		{
			name: "pending batch, no serial, should trigger serial",

			batchPending: true,
			successes:    []int{},
			pendings:     []int{},
			nones:        []int{1, 2, 3},
			batchMerges:  []int{},
			presubmits: map[int][]config.Presubmit{
				100: {
					{Reporter: config.Reporter{Context: "foo"}},
					{Reporter: config.Reporter{Context: "if-changed"}},
				},
			},
			merged:    0,
			triggered: 1,
			action:    Trigger,
		},
		{
			name: "batch merge errors but continues if a PR is unmergeable",

			batchMerges: []int{1, 2, 3},
			mergeErrs:   map[int]error{2: github.UnmergablePRError("test error")},
			merged:      2,
			triggered:   0,
			action:      MergeBatch,
		},
		{
			name: "batch merge errors but continues if a PR has changed",

			batchMerges: []int{1, 2, 3},
			mergeErrs:   map[int]error{2: github.ModifiedHeadError("test error")},
			merged:      2,
			triggered:   0,
			action:      MergeBatch,
		},
		{
			name: "batch merge errors but continues on unknown error",

			batchMerges: []int{1, 2, 3},
			mergeErrs:   map[int]error{2: errors.New("test error")},
			merged:      2,
			triggered:   0,
			action:      MergeBatch,
		},
		{
			name: "batch merge stops on auth error",

			batchMerges: []int{1, 2, 3},
			mergeErrs:   map[int]error{2: github.UnauthorizedToPushError("test error")},
			merged:      1,
			triggered:   0,
			action:      MergeBatch,
		},
		{
			name: "batch merge stops on invalid merge method error",

			batchMerges: []int{1, 2, 3},
			mergeErrs:   map[int]error{2: github.MergeCommitsForbiddenError("test error")},
			merged:      1,
			triggered:   0,
			action:      MergeBatch,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ca := &config.Agent{}
			pjNamespace := "pj-ns"
			cfg := &config.Config{ProwConfig: config.ProwConfig{ProwJobNamespace: pjNamespace}}
			if err := cfg.SetPresubmits(
				map[string][]config.Presubmit{
					"o/r": {
						{
							Reporter:     config.Reporter{Context: "foo"},
							Trigger:      "/test all",
							RerunCommand: "/test all",
							AlwaysRun:    true,
						},
						{
							JobBase: config.JobBase{
								Name: "bar",
							},
							Reporter:     config.Reporter{Context: "bar"},
							Trigger:      "/test bar",
							RerunCommand: "/test bar",
							AlwaysRun:    true,
						},
						{
							Reporter:     config.Reporter{Context: "if-changed"},
							Trigger:      "/test if-changed",
							RerunCommand: "/test if-changed",
							RegexpChangeMatcher: config.RegexpChangeMatcher{
								RunIfChanged: "CHANGED",
							},
						},
						{
							Reporter:     config.Reporter{Context: "if-changed"},
							Trigger:      "/test if-changed",
							RerunCommand: "/test if-changed",
							RegexpChangeMatcher: config.RegexpChangeMatcher{
								SkipIfOnlyChanged: "CHANGED1",
							},
						},
					},
				},
			); err != nil {
				t.Fatalf("failed to set presubmits: %v", err)
			}
			ca.Set(cfg)
			if len(tc.presubmits) > 0 {
				for i := 0; i <= 8; i++ {
					tc.presubmits[i] = []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}}
				}
			}
			lg, gc, err := clients()
			if err != nil {
				t.Fatalf("Error making local git: %v", err)
			}
			defer gc.Clean()
			defer lg.Clean()
			if err := lg.MakeFakeRepo("o", "r"); err != nil {
				t.Fatalf("Error making fake repo: %v", err)
			}
			if err := lg.AddCommit("o", "r", map[string][]byte{"foo": []byte("foo")}); err != nil {
				t.Fatalf("Adding initial commit: %v", err)
			}

			sp := subpool{
				log:        logrus.WithField("component", "tide"),
				presubmits: tc.presubmits,
				cc: map[int]contextChecker{
					0:   &config.TideContextPolicy{},
					1:   &config.TideContextPolicy{},
					2:   &config.TideContextPolicy{},
					3:   &config.TideContextPolicy{},
					4:   &config.TideContextPolicy{},
					5:   &config.TideContextPolicy{},
					6:   &config.TideContextPolicy{},
					7:   &config.TideContextPolicy{},
					8:   &config.TideContextPolicy{},
					100: &config.TideContextPolicy{},
				},
				org:    "o",
				repo:   "r",
				branch: defaultBranch,
				sha:    defaultBranch,
			}
			genPulls := func(nums []int) []CodeReviewCommon {
				var prs []CodeReviewCommon
				for _, i := range nums {
					if err := lg.CheckoutNewBranch("o", "r", fmt.Sprintf("pr-%d", i)); err != nil {
						t.Fatalf("Error checking out new branch: %v", err)
					}
					if err := lg.AddCommit("o", "r", map[string][]byte{fmt.Sprintf("%d", i): []byte("WOW")}); err != nil {
						t.Fatalf("Error adding commit: %v", err)
					}
					if err := lg.Checkout("o", "r", defaultBranch); err != nil {
						t.Fatalf("Error checking out master: %v", err)
					}
					oid := githubql.String(fmt.Sprintf("origin/pr-%d", i))
					var pr PullRequest
					pr.Number = githubql.Int(i)
					pr.HeadRefOID = oid
					pr.Commits.Nodes = []struct {
						Commit Commit
					}{{Commit: Commit{OID: oid}}}
					sp.prs = append(sp.prs, *CodeReviewCommonFromPullRequest(&pr))
					prs = append(prs, *CodeReviewCommonFromPullRequest(&pr))
				}
				return prs
			}
			fgc := fgc{mergeErrs: tc.mergeErrs}
			log := logrus.WithField("controller", "tide")
			ghProvider := newGitHubProvider(log, &fgc, gc, ca.Config, nil, false)
			c, err := newSyncController(
				context.Background(),
				log,
				newFakeManager(tc.preExistingJobs...),
				ghProvider,
				ca.Config,
				gc,
				nil,
				false,
				&statusUpdate{
					dontUpdateStatus: &threadSafePRSet{},
					newPoolPending:   make(chan bool),
				},
			)
			if err != nil {
				t.Fatalf("failed to construct sync controller: %v", err)
			}
			c.changedFiles = &changedFilesAgent{
				provider:        ghProvider,
				nextChangeCache: make(map[changeCacheKey][]string),
			}
			var batchPending []CodeReviewCommon
			if tc.batchPending {
				batchPending = []CodeReviewCommon{{}}
			}
			if act, _, _ := c.takeAction(sp, batchPending, genPulls(tc.successes), genPulls(tc.pendings), genPulls(tc.nones), genPulls(tc.batchMerges), sp.presubmits); act != tc.action {
				t.Errorf("Wrong action. Got %v, wanted %v.", act, tc.action)
			}

			prowJobs := &prowapi.ProwJobList{}
			if err := c.prowJobClient.List(context.Background(), prowJobs); err != nil {
				t.Fatalf("failed to list ProwJobs: %v", err)
			}
			var filteredProwJobs []prowapi.ProwJob
			// Filter out the ones we passed in
			for _, job := range prowJobs.Items {
				var preExists bool
				for _, preExistingJob := range tc.preExistingJobs {
					if reflect.DeepEqual(*preExistingJob.(*prowapi.ProwJob), job) {
						preExists = true
					}
				}
				if !preExists {
					filteredProwJobs = append(filteredProwJobs, job)
				}

			}
			numCreated := len(filteredProwJobs)

			var batchJobs []*prowapi.ProwJob
			for _, pj := range filteredProwJobs {
				if pj.Namespace != pjNamespace {
					t.Errorf("prowjob %q didn't have expected namespace %q but %q", pj.Name, pjNamespace, pj.Namespace)
				}
				if pj.Spec.Type == prowapi.BatchJob {
					pj := pj
					batchJobs = append(batchJobs, &pj)
				}
			}

			if tc.triggered != numCreated {
				t.Errorf("Wrong number of jobs triggered. Got %d, expected %d.", numCreated, tc.triggered)
			}
			if tc.merged != fgc.merged {
				t.Errorf("Wrong number of merges. Got %d, expected %d.", fgc.merged, tc.merged)
			}
			if n := len(c.statusUpdate.dontUpdateStatus.data); n != tc.merged+len(tc.mergeErrs) {
				t.Errorf("expected %d entries in the dontUpdateStatus map, got %d", tc.merged+len(tc.mergeErrs), n)
			}
			// Ensure that the correct number of batch jobs were triggered
			if tc.triggeredBatches != len(batchJobs) {
				t.Errorf("Wrong number of batches triggered. Got %d, expected %d.", len(batchJobs), tc.triggeredBatches)
			}
			for _, job := range batchJobs {
				if len(job.Spec.Refs.Pulls) <= 1 {
					t.Error("Found a batch job that doesn't contain multiple pull refs!")
				}
			}
		})
	}
}

func TestServeHTTP(t *testing.T) {
	pr1 := PullRequest{}
	pr1.Commits.Nodes = append(pr1.Commits.Nodes, struct{ Commit Commit }{})
	pr1.Commits.Nodes[0].Commit.Status.Contexts = []Context{{
		Context:     githubql.String("coverage/coveralls"),
		Description: githubql.String("Coverage increased (+0.1%) to 27.599%"),
	}}
	hist, err := history.New(100, nil, "")
	if err != nil {
		t.Fatalf("Failed to create history client: %v", err)
	}
	cfg := func() *config.Config { return &config.Config{} }
	c := &syncController{
		pools: []Pool{
			{
				MissingPRs: []CodeReviewCommon{*CodeReviewCommonFromPullRequest(&pr1)},
				Action:     Merge,
			},
		},
		provider: &GitHubProvider{
			mergeChecker: newMergeChecker(cfg, &fgc{}),
		},
		History: hist,
	}
	s := httptest.NewServer(c)
	defer s.Close()
	resp, err := http.Get(s.URL)
	if err != nil {
		t.Errorf("GET error: %v", err)
	}
	defer resp.Body.Close()
	var pools []Pool
	if err := json.NewDecoder(resp.Body).Decode(&pools); err != nil {
		t.Fatalf("JSON decoding error: %v", err)
	}
	if !reflect.DeepEqual(c.pools, pools) {
		t.Errorf("Received pools %v do not match original pools %v.", pools, c.pools)
	}
}

func testPR(org, repo, branch string, number int, mergeable githubql.MergeableState) *PullRequest {
	pr := PullRequest{
		Number:     githubql.Int(number),
		Mergeable:  mergeable,
		HeadRefOID: githubql.String("SHA"),
	}
	pr.Repository.Owner.Login = githubql.String(org)
	pr.Repository.Name = githubql.String(repo)
	pr.Repository.NameWithOwner = githubql.String(fmt.Sprintf("%s/%s", org, repo))
	pr.BaseRef.Name = githubql.String(branch)

	pr.Commits.Nodes = append(pr.Commits.Nodes, struct{ Commit Commit }{
		Commit{
			Status: struct{ Contexts []Context }{
				Contexts: []Context{
					{
						Context: githubql.String("context"),
						State:   githubql.StatusStateSuccess,
					},
				},
			},
			OID: githubql.String("SHA"),
		},
	})
	return &pr
}

func testPRWithLabels(org, repo, branch string, number int, mergeable githubql.MergeableState, labels []string) *PullRequest {
	pr := testPR(org, repo, branch, number, mergeable)
	for _, label := range labels {
		labelNode := struct{ Name githubql.String }{Name: githubql.String(label)}
		pr.Labels.Nodes = append(pr.Labels.Nodes, labelNode)
	}
	return pr
}

func TestSync(t *testing.T) {
	sleep = func(time.Duration) {}
	defer func() { sleep = time.Sleep }()

	mergeableA := *testPR("org", "repo", "A", 5, githubql.MergeableStateMergeable)
	unmergeableA := *testPR("org", "repo", "A", 6, githubql.MergeableStateConflicting)
	unmergeableB := *testPR("org", "repo", "B", 7, githubql.MergeableStateConflicting)
	unknownA := *testPR("org", "repo", "A", 8, githubql.MergeableStateUnknown)

	testcases := []struct {
		name string
		prs  []PullRequest

		expectedPools []Pool
	}{
		{
			name:          "no PRs",
			prs:           []PullRequest{},
			expectedPools: []Pool{},
		},
		{
			name: "1 mergeable PR",
			prs:  []PullRequest{mergeableA},
			expectedPools: []Pool{{
				Org:        "org",
				Repo:       "repo",
				Branch:     "A",
				SuccessPRs: []CodeReviewCommon{*CodeReviewCommonFromPullRequest(&mergeableA)},
				Action:     Merge,
				Target:     []CodeReviewCommon{*CodeReviewCommonFromPullRequest(&mergeableA)},
				TenantIDs:  []string{},
			}},
		},
		{
			name:          "1 unmergeable PR",
			prs:           []PullRequest{unmergeableA},
			expectedPools: []Pool{},
		},
		{
			name: "1 unknown PR",
			prs:  []PullRequest{unknownA},
			expectedPools: []Pool{{
				Org:        "org",
				Repo:       "repo",
				Branch:     "A",
				SuccessPRs: []CodeReviewCommon{*CodeReviewCommonFromPullRequest(&unknownA)},
				Action:     Merge,
				Target:     []CodeReviewCommon{*CodeReviewCommonFromPullRequest(&unknownA)},
				TenantIDs:  []string{},
			}},
		},
		{
			name: "1 mergeable, 1 unmergeable (different pools)",
			prs:  []PullRequest{mergeableA, unmergeableB},
			expectedPools: []Pool{{
				Org:        "org",
				Repo:       "repo",
				Branch:     "A",
				SuccessPRs: []CodeReviewCommon{*CodeReviewCommonFromPullRequest(&mergeableA)},
				Action:     Merge,
				Target:     []CodeReviewCommon{*CodeReviewCommonFromPullRequest(&mergeableA)},
				TenantIDs:  []string{},
			}},
		},
		{
			name: "1 mergeable, 1 unmergeable (same pool)",
			prs:  []PullRequest{mergeableA, unmergeableA},
			expectedPools: []Pool{{
				Org:        "org",
				Repo:       "repo",
				Branch:     "A",
				SuccessPRs: []CodeReviewCommon{*CodeReviewCommonFromPullRequest(&mergeableA)},
				Action:     Merge,
				Target:     []CodeReviewCommon{*CodeReviewCommonFromPullRequest(&mergeableA)},
				TenantIDs:  []string{},
			}},
		},
		{
			name: "1 mergeable PR (satisfies multiple queries)",
			prs:  []PullRequest{mergeableA, mergeableA},
			expectedPools: []Pool{{
				Org:        "org",
				Repo:       "repo",
				Branch:     "A",
				SuccessPRs: []CodeReviewCommon{*CodeReviewCommonFromPullRequest(&mergeableA)},
				Action:     Merge,
				Target:     []CodeReviewCommon{*CodeReviewCommonFromPullRequest(&mergeableA)},
				TenantIDs:  []string{},
			}},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			fgc := &fgc{
				prs: map[string][]PullRequest{"": tc.prs},
				refs: map[string]string{
					"org/repo heads/A": "SHA",
					"org/repo A":       "SHA",
					"org/repo heads/B": "SHA",
					"org/repo B":       "SHA",
				},
			}
			ca := &config.Agent{}
			ca.Set(&config.Config{
				ProwConfig: config.ProwConfig{
					Tide: config.Tide{
						MaxGoroutines: 4,
						TideGitHubConfig: config.TideGitHubConfig{
							Queries:            []config.TideQuery{{}},
							StatusUpdatePeriod: &metav1.Duration{Duration: time.Second * 0},
						},
					},
				},
			})
			hist, err := history.New(100, nil, "")
			if err != nil {
				t.Fatalf("Failed to create history client: %v", err)
			}
			mergeChecker := newMergeChecker(ca.Config, fgc)
			sc := &statusController{
				pjClient: fakectrlruntimeclient.NewFakeClient(),
				logger:   logrus.WithField("controller", "status-update"),
				ghc:      fgc,
				gc:       nil,
				config:   ca.Config,
				shutDown: make(chan bool),
				statusUpdate: &statusUpdate{
					dontUpdateStatus: &threadSafePRSet{},
					newPoolPending:   make(chan bool),
				},
			}
			go sc.run()
			defer sc.shutdown()
			log := logrus.WithField("controller", "sync")
			ghProvider := newGitHubProvider(log, fgc, nil, ca.Config, mergeChecker, false)
			c := &syncController{
				config:        ca.Config,
				provider:      ghProvider,
				prowJobClient: fakectrlruntimeclient.NewFakeClient(),
				logger:        log,
				changedFiles: &changedFilesAgent{
					provider:        ghProvider,
					nextChangeCache: make(map[changeCacheKey][]string),
				},
				History: hist,
				statusUpdate: &statusUpdate{
					dontUpdateStatus: &threadSafePRSet{},
					newPoolPending:   make(chan bool),
				},
			}

			if err := c.Sync(); err != nil {
				t.Fatalf("Unexpected error from 'Sync()': %v.", err)
			}
			if len(tc.expectedPools) != len(c.pools) {
				t.Fatalf("Tide pools did not match expected. Got %#v, expected %#v.", c.pools, tc.expectedPools)
			}
			for _, expected := range tc.expectedPools {
				var match *Pool
				for i, actual := range c.pools {
					if expected.Org == actual.Org && expected.Repo == actual.Repo && expected.Branch == actual.Branch {
						match = &c.pools[i]
					}
				}
				if match == nil {
					t.Errorf("Failed to find expected pool %s/%s %s.", expected.Org, expected.Repo, expected.Branch)
				} else if !reflect.DeepEqual(*match, expected) {
					t.Errorf("Expected pool %#v does not match actual pool %#v.", expected, *match)
				}
			}
		})
	}
}

func TestFilterSubpool(t *testing.T) {
	presubmits := map[int][]config.Presubmit{
		1: {{Reporter: config.Reporter{Context: "pj-a"}}},
		2: {{Reporter: config.Reporter{Context: "pj-a"}}, {Reporter: config.Reporter{Context: "pj-b"}}},
	}

	trueVar := true
	cc := map[int]contextChecker{
		1: &config.TideContextPolicy{
			RequiredContexts:    []string{"pj-a", "pj-b", "other-a"},
			OptionalContexts:    []string{"tide", "pj-c"},
			SkipUnknownContexts: &trueVar,
		},
		2: &config.TideContextPolicy{
			RequiredContexts:    []string{"pj-a", "pj-b", "other-a"},
			OptionalContexts:    []string{"tide", "pj-c"},
			SkipUnknownContexts: &trueVar,
		},
	}

	type pr struct {
		number    int
		mergeable bool
		contexts  []Context
		checkRuns []CheckRun
	}
	tcs := []struct {
		name string

		prs         []pr
		expectedPRs []int // Empty indicates no subpool should be returned.
	}{
		{
			name: "one mergeable passing PR (omitting optional context)",
			prs: []pr{
				{
					number:    1,
					mergeable: true,
					contexts: []Context{
						{
							Context: githubql.String("pj-a"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("pj-b"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("other-a"),
							State:   githubql.StatusStateSuccess,
						},
					},
				},
			},
			expectedPRs: []int{1},
		},
		{
			name: "one mergeable passing PR (omitting optional context), checkrun is considered",
			prs: []pr{
				{
					number:    1,
					mergeable: true,
					contexts: []Context{
						{
							Context: githubql.String("pj-a"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("pj-b"),
							State:   githubql.StatusStateSuccess,
						},
					},
					checkRuns: []CheckRun{{
						Name:       githubql.String("other-a"),
						Status:     checkRunStatusCompleted,
						Conclusion: githubql.String(githubql.StatusStateSuccess),
					}},
				},
			},
			expectedPRs: []int{1},
		},
		{
			name: "one mergeable passing PR (omitting optional context), neutral checkrun is considered success",
			prs: []pr{
				{
					number:    1,
					mergeable: true,
					contexts: []Context{
						{
							Context: githubql.String("pj-a"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("pj-b"),
							State:   githubql.StatusStateSuccess,
						},
					},
					checkRuns: []CheckRun{{
						Name:       githubql.String("other-a"),
						Status:     checkRunStatusCompleted,
						Conclusion: checkRunConclusionNeutral,
					}},
				},
			},
			expectedPRs: []int{1},
		},
		{
			name: "Incomplete checkrun throws the pr out",
			prs: []pr{
				{
					number:    1,
					mergeable: true,
					contexts: []Context{
						{
							Context: githubql.String("pj-a"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("pj-b"),
							State:   githubql.StatusStateSuccess,
						},
					},
					checkRuns: []CheckRun{{
						Name:       githubql.String("other-a"),
						Conclusion: githubql.String(githubql.StatusStateSuccess),
					}},
				},
			},
		},
		{
			name: "Failing checkrun throws the pr out",
			prs: []pr{
				{
					number:    1,
					mergeable: true,
					contexts: []Context{
						{
							Context: githubql.String("pj-a"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("pj-b"),
							State:   githubql.StatusStateSuccess,
						},
					},
					checkRuns: []CheckRun{{
						Name:       githubql.String("other-a"),
						Status:     checkRunStatusCompleted,
						Conclusion: githubql.String(githubql.StatusStateFailure),
					}},
				},
			},
		},
		{
			name: "one unmergeable passing PR",
			prs: []pr{
				{
					number:    1,
					mergeable: false,
					contexts: []Context{
						{
							Context: githubql.String("pj-a"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("pj-b"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("other-a"),
							State:   githubql.StatusStateSuccess,
						},
					},
				},
			},
			expectedPRs: []int{},
		},
		{
			name: "one mergeable PR pending non-PJ context (consider failing)",
			prs: []pr{
				{
					number:    2,
					mergeable: true,
					contexts: []Context{
						{
							Context: githubql.String("pj-a"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("pj-b"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("other-a"),
							State:   githubql.StatusStatePending,
						},
					},
				},
			},
			expectedPRs: []int{},
		},
		{
			name: "one mergeable PR pending PJ context (consider in pool)",
			prs: []pr{
				{
					number:    2,
					mergeable: true,
					contexts: []Context{
						{
							Context: githubql.String("pj-a"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("pj-b"),
							State:   githubql.StatusStatePending,
						},
						{
							Context: githubql.String("other-a"),
							State:   githubql.StatusStateSuccess,
						},
					},
				},
			},
			expectedPRs: []int{2},
		},
		{
			name: "one mergeable PR failing PJ context (consider failing)",
			prs: []pr{
				{
					number:    2,
					mergeable: true,
					contexts: []Context{
						{
							Context: githubql.String("pj-a"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("pj-b"),
							State:   githubql.StatusStateFailure,
						},
						{
							Context: githubql.String("other-a"),
							State:   githubql.StatusStateSuccess,
						},
					},
				},
			},
			expectedPRs: []int{},
		},
		{
			name: "one mergeable PR missing PJ context (consider failing)",
			prs: []pr{
				{
					number:    2,
					mergeable: true,
					contexts: []Context{
						{
							Context: githubql.String("pj-b"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("other-a"),
							State:   githubql.StatusStateSuccess,
						},
					},
				},
			},
			expectedPRs: []int{},
		},
		{
			name: "one mergeable PR failing unknown context (consider in pool)",
			prs: []pr{
				{
					number:    2,
					mergeable: true,
					contexts: []Context{
						{
							Context: githubql.String("pj-a"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("pj-b"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("other-a"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("unknown"),
							State:   githubql.StatusStateFailure,
						},
					},
				},
			},
			expectedPRs: []int{2},
		},
		{
			name: "one PR failing non-PJ required context; one PR successful (should not prune pool)",
			prs: []pr{
				{
					number:    1,
					mergeable: true,
					contexts: []Context{
						{
							Context: githubql.String("pj-a"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("pj-b"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("other-a"),
							State:   githubql.StatusStateFailure,
						},
					},
				},
				{
					number:    2,
					mergeable: true,
					contexts: []Context{
						{
							Context: githubql.String("pj-a"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("pj-b"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("other-a"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("unknown"),
							State:   githubql.StatusStateSuccess,
						},
					},
				},
			},
			expectedPRs: []int{2},
		},
		{
			name: "two successful PRs",
			prs: []pr{
				{
					number:    1,
					mergeable: true,
					contexts: []Context{
						{
							Context: githubql.String("pj-a"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("pj-b"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("other-a"),
							State:   githubql.StatusStateSuccess,
						},
					},
				},
				{
					number:    2,
					mergeable: true,
					contexts: []Context{
						{
							Context: githubql.String("pj-a"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("pj-b"),
							State:   githubql.StatusStateSuccess,
						},
						{
							Context: githubql.String("other-a"),
							State:   githubql.StatusStateSuccess,
						},
					},
				},
			},
			expectedPRs: []int{1, 2},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			sp := &subpool{
				org:        "org",
				repo:       "repo",
				branch:     "branch",
				presubmits: presubmits,
				cc:         cc,
				log:        logrus.WithFields(logrus.Fields{"org": "org", "repo": "repo", "branch": "branch"}),
			}
			for _, pull := range tc.prs {
				pr := PullRequest{
					Number: githubql.Int(pull.number),
				}
				var checkRunNodes []CheckRunNode
				for _, checkRun := range pull.checkRuns {
					checkRunNodes = append(checkRunNodes, CheckRunNode{CheckRun: checkRun})
				}
				pr.Commits.Nodes = []struct{ Commit Commit }{
					{
						Commit{
							Status: struct{ Contexts []Context }{
								Contexts: pull.contexts,
							},
							StatusCheckRollup: StatusCheckRollup{
								Contexts: StatusCheckRollupContext{
									Nodes: checkRunNodes,
								},
							},
						},
					},
				}
				if !pull.mergeable {
					pr.Mergeable = githubql.MergeableStateConflicting
				}
				sp.prs = append(sp.prs, *CodeReviewCommonFromPullRequest(&pr))
			}

			configGetter := func() *config.Config { return &config.Config{} }
			mmc := newMergeChecker(configGetter, &fgc{})
			provider := &GitHubProvider{
				cfg:          configGetter,
				mergeChecker: mmc,
				logger:       logrus.WithContext(context.Background()),
			}
			filtered := filterSubpool(provider, mmc.isAllowedToMerge, sp)
			if len(tc.expectedPRs) == 0 {
				if filtered != nil {
					t.Fatalf("Expected subpool to be pruned, but got: %v", filtered)
				}
				return
			}
			if filtered == nil {
				t.Fatalf("Expected subpool to have %d prs, but it was pruned.", len(tc.expectedPRs))
			}
			if got := prNumbers(filtered.prs); !reflect.DeepEqual(got, tc.expectedPRs) {
				t.Errorf("Expected filtered pool to have PRs %v, but got %v.", tc.expectedPRs, got)
			}
		})
	}
}

func TestIsPassing(t *testing.T) {
	yes := true
	no := false
	headSHA := "head"
	success := string(githubql.StatusStateSuccess)
	failure := string(githubql.StatusStateFailure)
	testCases := []struct {
		name              string
		passing           bool
		config            config.TideContextPolicy
		combinedContexts  map[string]string
		availableContexts []string
		failedContexts    []string
	}{
		{
			name:              "empty policy - success (trust combined status)",
			passing:           true,
			combinedContexts:  map[string]string{"c1": success, "c2": success, statusContext: failure},
			availableContexts: []string{"c1", "c2", statusContext},
		},
		{
			name:              "empty policy - failure because of failed context c4 (trust combined status)",
			passing:           false,
			combinedContexts:  map[string]string{"c1": success, "c2": success, "c3": failure, statusContext: failure},
			availableContexts: []string{"c1", "c2", "c3", statusContext},
			failedContexts:    []string{"c3"},
		},
		{
			name:    "passing (trust combined status)",
			passing: true,
			config: config.TideContextPolicy{
				RequiredContexts:    []string{"c1", "c2", "c3"},
				SkipUnknownContexts: &no,
			},
			combinedContexts:  map[string]string{"c1": success, "c2": success, "c3": success, statusContext: failure},
			availableContexts: []string{"c1", "c2", "c3", statusContext},
		},
		{
			name:    "failing because of missing required check c3",
			passing: false,
			config: config.TideContextPolicy{
				RequiredContexts: []string{"c1", "c2", "c3"},
			},
			combinedContexts:  map[string]string{"c1": success, "c2": success, statusContext: failure},
			availableContexts: []string{"c1", "c2", statusContext},
			failedContexts:    []string{"c3"},
		},
		{
			name:             "failing because of failed context c2",
			passing:          false,
			combinedContexts: map[string]string{"c1": success, "c2": failure},
			config: config.TideContextPolicy{
				RequiredContexts: []string{"c1", "c2", "c3"},
				OptionalContexts: []string{"c4"},
			},
			availableContexts: []string{"c1", "c2"},
			failedContexts:    []string{"c2", "c3"},
		},
		{
			name:    "passing because of failed context c4 is optional",
			passing: true,

			combinedContexts: map[string]string{"c1": success, "c2": success, "c3": success, "c4": failure},
			config: config.TideContextPolicy{
				RequiredContexts: []string{"c1", "c2", "c3"},
				OptionalContexts: []string{"c4"},
			},
			availableContexts: []string{"c1", "c2", "c3", "c4"},
		},
		{
			name:    "skipping unknown contexts - failing because of missing required context c3",
			passing: false,
			config: config.TideContextPolicy{
				RequiredContexts:    []string{"c1", "c2", "c3"},
				SkipUnknownContexts: &yes,
			},
			combinedContexts:  map[string]string{"c1": success, "c2": success, statusContext: failure},
			availableContexts: []string{"c1", "c2", statusContext},
			failedContexts:    []string{"c3"},
		},
		{
			name:             "skipping unknown contexts - failing because c2 is failing",
			passing:          false,
			combinedContexts: map[string]string{"c1": success, "c2": failure},
			config: config.TideContextPolicy{
				RequiredContexts:    []string{"c1", "c2"},
				OptionalContexts:    []string{"c4"},
				SkipUnknownContexts: &yes,
			},
			availableContexts: []string{"c1", "c2"},
			failedContexts:    []string{"c2"},
		},
		{
			name:             "skipping unknown contexts - passing because c4 is optional",
			passing:          true,
			combinedContexts: map[string]string{"c1": success, "c2": success, "c3": success, "c4": failure},
			config: config.TideContextPolicy{
				RequiredContexts:    []string{"c1", "c3"},
				OptionalContexts:    []string{"c4"},
				SkipUnknownContexts: &yes,
			},
			availableContexts: []string{"c1", "c2", "c3", "c4"},
		},
		{
			name:    "skipping unknown contexts - passing because c4 is optional and c5 is unknown",
			passing: true,

			combinedContexts: map[string]string{"c1": success, "c2": success, "c3": success, "c4": failure, "c5": failure},
			config: config.TideContextPolicy{
				RequiredContexts:    []string{"c1", "c3"},
				OptionalContexts:    []string{"c4"},
				SkipUnknownContexts: &yes,
			},
			availableContexts: []string{"c1", "c2", "c3", "c4", "c5"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			ghc := &fgc{
				combinedStatus: tc.combinedContexts,
				expectedSHA:    headSHA}
			log := logrus.WithField("component", "tide")
			hook := test.NewGlobal()
			_, err := log.String()
			if err != nil {
				t.Fatalf("Failed to get log output before testing: %v", err)
			}
			syncCtl := &syncController{provider: &GitHubProvider{ghc: ghc, logger: log}}
			pr := PullRequest{HeadRefOID: githubql.String(headSHA)}
			passing := syncCtl.isPassingTests(log, CodeReviewCommonFromPullRequest(&pr), &tc.config)
			if passing != tc.passing {
				t.Errorf("%s: Expected %t got %t", tc.name, tc.passing, passing)
			}

			// The last entry is used as the hook captures 2 different logs.
			// The required fields are available in the last entry and are validated.
			logFields := hook.LastEntry().Data
			assert.Equal(t, logFields["context_names"], tc.availableContexts)
			assert.Equal(t, logFields["failed_context_names"], tc.failedContexts)
			assert.Equal(t, logFields["total_context_count"], len(tc.availableContexts))
			assert.Equal(t, logFields["failed_context_count"], len(tc.failedContexts))
			if tc.passing {
				c := &syncController{
					provider:      &GitHubProvider{ghc: ghc, logger: log},
					prowJobClient: fakectrlruntimeclient.NewFakeClient(),
					config:        func() *config.Config { return &config.Config{} },
				}
				// isRetestEligible is more lenient than isPassingTests, which means we expect it to allow
				// everything that is allowed by isPassingTests. The reverse might not be true.
				if !c.isRetestEligible(log, CodeReviewCommonFromPullRequest(&pr), &tc.config) {
					t.Error("expected pr to be batch testing eligible, wasn't the case")
				}
			}
		})
	}
}

func TestPresubmitsByPull(t *testing.T) {
	samplePR := PullRequest{
		Number:     githubql.Int(100),
		HeadRefOID: githubql.String("sha"),
	}
	testcases := []struct {
		name string

		initialChangeCache map[changeCacheKey][]string
		presubmits         []config.Presubmit
		prs                []CodeReviewCommon
		prowYAMLGetter     config.ProwYAMLGetter

		expectedPresubmits  map[int][]config.Presubmit
		expectedChangeCache map[changeCacheKey][]string
	}{
		{
			name: "no matching presubmits",
			presubmits: []config.Presubmit{
				{
					Reporter: config.Reporter{Context: "always"},
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "foo",
					},
				},
				{
					Reporter: config.Reporter{Context: "never"},
				},
			},
			expectedChangeCache: map[changeCacheKey][]string{{number: 100, sha: "sha"}: {"CHANGED"}},
			expectedPresubmits:  map[int][]config.Presubmit{},
		},
		{
			name:               "no presubmits",
			presubmits:         []config.Presubmit{},
			expectedPresubmits: map[int][]config.Presubmit{},
		},
		{
			name: "no matching presubmits (check cache eviction)",
			presubmits: []config.Presubmit{
				{
					Reporter: config.Reporter{Context: "never"},
				},
			},
			initialChangeCache: map[changeCacheKey][]string{{number: 100, sha: "sha"}: {"FILE"}},
			expectedPresubmits: map[int][]config.Presubmit{},
		},
		{
			name: "no matching presubmits (check cache retention)",
			presubmits: []config.Presubmit{
				{
					Reporter: config.Reporter{Context: "always"},
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "foo",
					},
				},
				{
					Reporter: config.Reporter{Context: "never"},
				},
			},
			initialChangeCache:  map[changeCacheKey][]string{{number: 100, sha: "sha"}: {"FILE"}},
			expectedChangeCache: map[changeCacheKey][]string{{number: 100, sha: "sha"}: {"FILE"}},
			expectedPresubmits:  map[int][]config.Presubmit{},
		},
		{
			name: "always_run",
			presubmits: []config.Presubmit{
				{
					Reporter:  config.Reporter{Context: "always"},
					AlwaysRun: true,
				},
				{
					Reporter: config.Reporter{Context: "never"},
				},
			},
			expectedPresubmits: map[int][]config.Presubmit{100: {{
				Reporter:  config.Reporter{Context: "always"},
				AlwaysRun: true,
			}}},
		},
		{
			name: "runs against branch",
			presubmits: []config.Presubmit{
				{
					Reporter:  config.Reporter{Context: "presubmit"},
					AlwaysRun: true,
					Brancher: config.Brancher{
						Branches: []string{defaultBranch, "dev"},
					},
				},
				{
					Reporter: config.Reporter{Context: "never"},
				},
			},
			expectedPresubmits: map[int][]config.Presubmit{100: {{
				Reporter:  config.Reporter{Context: "presubmit"},
				AlwaysRun: true,
				Brancher: config.Brancher{
					Branches: []string{defaultBranch, "dev"},
				},
			}}},
		},
		{
			name: "doesn't run against branch",
			presubmits: []config.Presubmit{
				{
					Reporter:  config.Reporter{Context: "presubmit"},
					AlwaysRun: true,
					Brancher: config.Brancher{
						Branches: []string{"release", "dev"},
					},
				},
				{
					Reporter:  config.Reporter{Context: "always"},
					AlwaysRun: true,
				},
				{
					Reporter: config.Reporter{Context: "never"},
				},
			},
			expectedPresubmits: map[int][]config.Presubmit{100: {{
				Reporter:  config.Reporter{Context: "always"},
				AlwaysRun: true,
			}}},
		},
		{
			name: "no-always-run-no-trigger",
			presubmits: []config.Presubmit{
				{
					Reporter:  config.Reporter{Context: "presubmit"},
					AlwaysRun: false,
					Brancher: config.Brancher{
						Branches: []string{defaultBranch, "dev"},
					},
				},
				{
					Reporter: config.Reporter{Context: "never"},
				},
			},
			expectedPresubmits: map[int][]config.Presubmit{},
		},
		{
			name: "no-always-run-no-trigger-tide-wants-it",
			presubmits: []config.Presubmit{
				{
					Reporter:  config.Reporter{Context: "presubmit"},
					AlwaysRun: false,
					Brancher: config.Brancher{
						Branches: []string{defaultBranch, "dev"},
					},
					RunBeforeMerge: true,
				},
				{
					Reporter: config.Reporter{Context: "never"},
				},
			},
			expectedPresubmits: map[int][]config.Presubmit{100: {{
				Reporter:       config.Reporter{Context: "presubmit"},
				AlwaysRun:      false,
				RunBeforeMerge: true,
				Brancher: config.Brancher{
					Branches: []string{defaultBranch, "dev"},
				},
			}}},
		},
		{
			name: "brancher-not-match-when-tide-wants-it",
			presubmits: []config.Presubmit{
				{
					Reporter:  config.Reporter{Context: "presubmit"},
					AlwaysRun: false,
					Brancher: config.Brancher{
						Branches: []string{"release", "dev"},
					},
					RunBeforeMerge: true,
				},
				{
					Reporter: config.Reporter{Context: "never"},
				},
			},
			expectedPresubmits: map[int][]config.Presubmit{},
		},
		{
			name: "run_if_changed (uncached)",
			presubmits: []config.Presubmit{
				{
					Reporter: config.Reporter{Context: "presubmit"},
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "^CHANGE.$",
					},
				},
				{
					Reporter:  config.Reporter{Context: "always"},
					AlwaysRun: true,
				},
				{
					Reporter: config.Reporter{Context: "never"},
				},
			},
			expectedPresubmits: map[int][]config.Presubmit{100: {{
				Reporter: config.Reporter{Context: "presubmit"},
				RegexpChangeMatcher: config.RegexpChangeMatcher{
					RunIfChanged: "^CHANGE.$",
				},
			}, {
				Reporter:  config.Reporter{Context: "always"},
				AlwaysRun: true,
			}}},
			expectedChangeCache: map[changeCacheKey][]string{{number: 100, sha: "sha"}: {"CHANGED"}},
		},
		{
			name: "run_if_changed (cached)",
			presubmits: []config.Presubmit{
				{
					Reporter: config.Reporter{Context: "presubmit"},
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "^FIL.$",
					},
				},
				{
					Reporter:  config.Reporter{Context: "always"},
					AlwaysRun: true,
				},
				{
					Reporter: config.Reporter{Context: "never"},
				},
			},
			initialChangeCache: map[changeCacheKey][]string{{number: 100, sha: "sha"}: {"FILE"}},
			expectedPresubmits: map[int][]config.Presubmit{100: {{
				Reporter: config.Reporter{Context: "presubmit"},
				RegexpChangeMatcher: config.RegexpChangeMatcher{
					RunIfChanged: "^FIL.$",
				},
			},
				{
					Reporter:  config.Reporter{Context: "always"},
					AlwaysRun: true,
				}}},
			expectedChangeCache: map[changeCacheKey][]string{{number: 100, sha: "sha"}: {"FILE"}},
		},
		{
			name: "run_if_changed (cached) (skippable)",
			presubmits: []config.Presubmit{
				{
					Reporter: config.Reporter{Context: "presubmit"},
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "^CHANGE.$",
					},
				},
				{
					Reporter:  config.Reporter{Context: "always"},
					AlwaysRun: true,
				},
				{
					Reporter: config.Reporter{Context: "never"},
				},
			},
			initialChangeCache: map[changeCacheKey][]string{{number: 100, sha: "sha"}: {"FILE"}},
			expectedPresubmits: map[int][]config.Presubmit{100: {{
				Reporter:  config.Reporter{Context: "always"},
				AlwaysRun: true,
			}}},
			expectedChangeCache: map[changeCacheKey][]string{{number: 100, sha: "sha"}: {"FILE"}},
		},
		{
			name: "inrepoconfig presubmits get only added to the corresponding pull",
			presubmits: []config.Presubmit{{
				AlwaysRun: true,
				Reporter:  config.Reporter{Context: "always"},
			}},
			prowYAMLGetter: prowYAMLGetterForHeadRefs([]string{"1"}, []config.Presubmit{{
				AlwaysRun: true,
				Reporter:  config.Reporter{Context: "inrepoconfig"},
			}}),
			prs: []CodeReviewCommon{
				{Number: 1, HeadRefOID: "1"},
			},
			expectedPresubmits: map[int][]config.Presubmit{
				1: {
					{AlwaysRun: true, Reporter: config.Reporter{Context: "always"}},
					{AlwaysRun: true, Reporter: config.Reporter{Context: "inrepoconfig"}},
				},
				100: {
					{AlwaysRun: true, Reporter: config.Reporter{Context: "always"}},
				},
			},
		},
		{
			name: "broken inrepoconfig doesn't break the whole subpool",
			presubmits: []config.Presubmit{{
				AlwaysRun: true,
				Reporter:  config.Reporter{Context: "always"},
			}},
			prowYAMLGetter: func(_ *config.Config, _ git.ClientFactory, _, _, _ string, headRefs ...string) (*config.ProwYAML, error) {
				if len(headRefs) == 1 && headRefs[0] == "1" {
					return nil, errors.New("you shall not get jobs")
				}
				return &config.ProwYAML{}, nil
			},
			prs: []CodeReviewCommon{
				{Number: 1, HeadRefOID: "1"},
			},
			expectedPresubmits: map[int][]config.Presubmit{
				100: {
					{AlwaysRun: true, Reporter: config.Reporter{Context: "always"}},
				},
			},
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.initialChangeCache == nil {
				tc.initialChangeCache = map[changeCacheKey][]string{}
			}
			if tc.expectedChangeCache == nil {
				tc.expectedChangeCache = map[changeCacheKey][]string{}
			}

			cfg := &config.Config{}
			cfg.SetPresubmits(map[string][]config.Presubmit{
				"/":       tc.presubmits,
				"foo/bar": {{Reporter: config.Reporter{Context: "wrong-repo"}, AlwaysRun: true}},
			})
			if tc.prowYAMLGetter != nil {
				cfg.InRepoConfig.Enabled = map[string]*bool{"*": utilpointer.Bool(true)}
				cfg.ProwYAMLGetterWithDefaults = tc.prowYAMLGetter
			}
			cfgAgent := &config.Agent{}
			cfgAgent.Set(cfg)
			sp := &subpool{
				branch: defaultBranch,
				sha:    "master-sha",
				prs:    append(tc.prs, *CodeReviewCommonFromPullRequest(&samplePR)),
			}
			log := logrus.WithField("test", tc.name)
			ghProvider := newGitHubProvider(log, &fgc{}, nil, cfgAgent.Config, newMergeChecker(cfgAgent.Config, &fgc{}), false)
			c := &syncController{
				config:   cfgAgent.Config,
				provider: ghProvider,
				changedFiles: &changedFilesAgent{
					provider:        ghProvider,
					changeCache:     tc.initialChangeCache,
					nextChangeCache: make(map[changeCacheKey][]string),
				},
				logger: log,
			}
			presubmits, err := c.presubmitsByPull(sp)
			if err != nil {
				t.Fatalf("unexpected error from presubmitsByPull: %v", err)
			}
			c.changedFiles.prune()
			// for equality we need to clear the compiled regexes
			for _, jobs := range presubmits {
				config.ClearCompiledRegexes(jobs)
			}
			if !apiequality.Semantic.DeepEqual(presubmits, tc.expectedPresubmits) {
				t.Errorf("got incorrect presubmit mapping: %v\n", diff.ObjectReflectDiff(tc.expectedPresubmits, presubmits))
			}
			if got := c.changedFiles.changeCache; !reflect.DeepEqual(got, tc.expectedChangeCache) {
				t.Errorf("got incorrect file change cache: %v", diff.ObjectReflectDiff(tc.expectedChangeCache, got))
			}
		})
	}
}

func getTemplate(name, tplStr string) *template.Template {
	tpl, _ := template.New(name).Parse(tplStr)
	return tpl
}

func TestAccumulateReturnsCorrectMissingTests(t *testing.T) {
	const baseSHA = "8d287a3aeae90fd0aef4a70009c715712ff302cd"

	testCases := []struct {
		name               string
		presubmits         map[int][]config.Presubmit
		prs                []PullRequest
		pjs                []prowapi.ProwJob
		expectedPresubmits map[int][]config.Presubmit
	}{
		{
			name: "All presubmits missing, no changes",
			prs: []PullRequest{{
				Number:     1,
				HeadRefOID: "sha",
			}},
			presubmits: map[int][]config.Presubmit{1: {{
				Reporter: config.Reporter{
					Context: "my-presubmit",
				},
			}}},
			expectedPresubmits: map[int][]config.Presubmit{
				1: {{Reporter: config.Reporter{Context: "my-presubmit"}}},
			},
		},
		{
			name: "All presubmits successful, no retesting needed",
			prs: []PullRequest{{
				Number:     1,
				HeadRefOID: "sha",
			}},
			pjs: []prowapi.ProwJob{{
				Spec: prowapi.ProwJobSpec{
					Type: prowapi.PresubmitJob,
					Refs: &prowapi.Refs{
						Pulls: []prowapi.Pull{{
							Number: 1,
							SHA:    "sha",
						}},
					},
					Context: "my-presubmit",
				},
				Status: prowapi.ProwJobStatus{State: prowapi.SuccessState},
			}},
			presubmits: map[int][]config.Presubmit{
				1: {{Reporter: config.Reporter{Context: "my-presubmit"}}},
			},
		},
		{
			name: "All presubmits pending, no retesting needed",
			prs: []PullRequest{{
				Number:     1,
				HeadRefOID: "sha",
			}},
			pjs: []prowapi.ProwJob{{
				Spec: prowapi.ProwJobSpec{
					Type: prowapi.PresubmitJob,
					Refs: &prowapi.Refs{
						Pulls: []prowapi.Pull{{
							Number: 1,
							SHA:    "sha",
						}},
					},
					Context: "my-presubmit",
				},
				Status: prowapi.ProwJobStatus{State: prowapi.PendingState},
			}},
			presubmits: map[int][]config.Presubmit{
				1: {{Reporter: config.Reporter{Context: "my-presubmit"}}}},
		},
		{
			name: "One successful, one pending, one missing, one failing, only missing and failing remain",
			prs: []PullRequest{{
				Number:     1,
				HeadRefOID: "sha",
			}},
			pjs: []prowapi.ProwJob{
				{
					Spec: prowapi.ProwJobSpec{
						Type: prowapi.PresubmitJob,
						Refs: &prowapi.Refs{
							Pulls: []prowapi.Pull{{
								Number: 1,
								SHA:    "sha",
							}},
						},
						Context: "my-successful-presubmit",
					},
					Status: prowapi.ProwJobStatus{State: prowapi.SuccessState},
				},
				{
					Spec: prowapi.ProwJobSpec{
						Type: prowapi.PresubmitJob,
						Refs: &prowapi.Refs{
							Pulls: []prowapi.Pull{{
								Number: 1,
								SHA:    "sha",
							}},
						},
						Context: "my-pending-presubmit",
					},
					Status: prowapi.ProwJobStatus{State: prowapi.PendingState},
				},
				{
					Spec: prowapi.ProwJobSpec{
						Type: prowapi.PresubmitJob,
						Refs: &prowapi.Refs{
							Pulls: []prowapi.Pull{{
								Number: 1,
								SHA:    "sha",
							}},
						},
						Context: "my-failing-presubmit",
					},
					Status: prowapi.ProwJobStatus{State: prowapi.FailureState},
				},
			},
			presubmits: map[int][]config.Presubmit{
				1: {
					{Reporter: config.Reporter{Context: "my-successful-presubmit"}},
					{Reporter: config.Reporter{Context: "my-pending-presubmit"}},
					{Reporter: config.Reporter{Context: "my-failing-presubmit"}},
					{Reporter: config.Reporter{Context: "my-missing-presubmit"}},
				}},
			expectedPresubmits: map[int][]config.Presubmit{
				1: {
					{Reporter: config.Reporter{Context: "my-failing-presubmit"}},
					{Reporter: config.Reporter{Context: "my-missing-presubmit"}},
				}},
		},
		{
			name: "Two prs, each with one successful, one pending, one missing, one failing, only missing and failing remain",
			prs: []PullRequest{
				{
					Number:     1,
					HeadRefOID: "sha",
				},
				{
					Number:     2,
					HeadRefOID: "sha",
				},
			},
			pjs: []prowapi.ProwJob{
				{
					Spec: prowapi.ProwJobSpec{
						Type: prowapi.PresubmitJob,
						Refs: &prowapi.Refs{
							Pulls: []prowapi.Pull{{
								Number: 1,
								SHA:    "sha",
							}},
						},
						Context: "my-successful-presubmit",
					},
					Status: prowapi.ProwJobStatus{State: prowapi.SuccessState},
				},
				{
					Spec: prowapi.ProwJobSpec{
						Type: prowapi.PresubmitJob,
						Refs: &prowapi.Refs{
							Pulls: []prowapi.Pull{{
								Number: 1,
								SHA:    "sha",
							}},
						},
						Context: "my-pending-presubmit",
					},
					Status: prowapi.ProwJobStatus{State: prowapi.PendingState},
				},
				{
					Spec: prowapi.ProwJobSpec{
						Type: prowapi.PresubmitJob,
						Refs: &prowapi.Refs{
							Pulls: []prowapi.Pull{{
								Number: 1,
								SHA:    "sha",
							}},
						},
						Context: "my-failing-presubmit",
					},
					Status: prowapi.ProwJobStatus{State: prowapi.FailureState},
				},
				{
					Spec: prowapi.ProwJobSpec{
						Type: prowapi.PresubmitJob,
						Refs: &prowapi.Refs{
							Pulls: []prowapi.Pull{{
								Number: 2,
								SHA:    "sha",
							}},
						},
						Context: "my-successful-presubmit",
					},
					Status: prowapi.ProwJobStatus{State: prowapi.SuccessState},
				},
				{
					Spec: prowapi.ProwJobSpec{
						Type: prowapi.PresubmitJob,
						Refs: &prowapi.Refs{
							Pulls: []prowapi.Pull{{
								Number: 2,
								SHA:    "sha",
							}},
						},
						Context: "my-pending-presubmit",
					},
					Status: prowapi.ProwJobStatus{State: prowapi.PendingState},
				},
				{
					Spec: prowapi.ProwJobSpec{
						Type: prowapi.PresubmitJob,
						Refs: &prowapi.Refs{
							Pulls: []prowapi.Pull{{
								Number: 2,
								SHA:    "sha",
							}},
						},
						Context: "my-failing-presubmit",
					},
					Status: prowapi.ProwJobStatus{State: prowapi.FailureState},
				},
			},
			presubmits: map[int][]config.Presubmit{
				1: {
					{Reporter: config.Reporter{Context: "my-successful-presubmit"}},
					{Reporter: config.Reporter{Context: "my-pending-presubmit"}},
					{Reporter: config.Reporter{Context: "my-failing-presubmit"}},
					{Reporter: config.Reporter{Context: "my-missing-presubmit"}},
				},
				2: {
					{Reporter: config.Reporter{Context: "my-successful-presubmit"}},
					{Reporter: config.Reporter{Context: "my-pending-presubmit"}},
					{Reporter: config.Reporter{Context: "my-failing-presubmit"}},
					{Reporter: config.Reporter{Context: "my-missing-presubmit"}},
				},
			},
			expectedPresubmits: map[int][]config.Presubmit{
				1: {
					{Reporter: config.Reporter{Context: "my-failing-presubmit"}},
					{Reporter: config.Reporter{Context: "my-missing-presubmit"}},
				},
				2: {
					{Reporter: config.Reporter{Context: "my-failing-presubmit"}},
					{Reporter: config.Reporter{Context: "my-missing-presubmit"}},
				},
			},
		},
		{
			name:       "Result from successful context gets respected",
			presubmits: map[int][]config.Presubmit{1: {{Reporter: config.Reporter{Context: "job-1"}}}},
			prs: []PullRequest{{
				Number:     1,
				HeadRefOID: "headsha",
				Commits: Commits{Nodes: []struct{ Commit Commit }{{Commit: Commit{
					OID: githubql.String("headsha"),
					Status: CommitStatus{Contexts: []Context{{
						Context:     githubql.String("job-1"),
						Description: githubql.String("Job succeeded. BaseSHA:" + baseSHA),
						State:       githubql.StatusStateSuccess,
					}}},
				}}}}}},
		},
		{
			name:       "Result from successful context gets respected with deprecated baseha delimiter",
			presubmits: map[int][]config.Presubmit{1: {{Reporter: config.Reporter{Context: "job-1"}}}},
			prs: []PullRequest{{
				Number:     1,
				HeadRefOID: "headsha",
				Commits: Commits{Nodes: []struct{ Commit Commit }{{Commit: Commit{
					OID: githubql.String("headsha"),
					Status: CommitStatus{Contexts: []Context{{
						Context:     githubql.String("job-1"),
						Description: githubql.String("Job succeeded. Basesha:" + baseSHA),
						State:       githubql.StatusStateSuccess,
					}}},
				}}}}}},
		},
		{
			name:       "Result from failed context gets ignored",
			presubmits: map[int][]config.Presubmit{1: {{Reporter: config.Reporter{Context: "job-1"}}}},
			prs: []PullRequest{{
				Number:     1,
				HeadRefOID: "headsha",
				Commits: Commits{Nodes: []struct{ Commit Commit }{{Commit: Commit{
					OID: githubql.String("headsha"),
					Status: CommitStatus{Contexts: []Context{{
						Context:     githubql.String("job-1"),
						Description: githubql.String("Job succeeded. BaseSHA:" + baseSHA),
						State:       githubql.StatusStateFailure,
					}}},
				}}}}}},
			expectedPresubmits: map[int][]config.Presubmit{1: {{Reporter: config.Reporter{Context: "job-1"}}}},
		},
	}

	log := logrus.NewEntry(logrus.New())
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var crcs []CodeReviewCommon
			for _, pr := range tc.prs {
				crc := CodeReviewCommonFromPullRequest(&pr)
				crcs = append(crcs, *crc)
			}
			syncCtrl := &syncController{
				provider: &GitHubProvider{
					ghc:    &fgc{},
					logger: log,
				},
				logger: log,
			}
			_, _, _, missingSerialTests := syncCtrl.accumulate(tc.presubmits, crcs, tc.pjs, baseSHA)
			// Apiequality treats nil slices/maps equal to a zero length slice/map, keeping us from
			// the burden of having to always initialize them
			if !apiequality.Semantic.DeepEqual(tc.expectedPresubmits, missingSerialTests) {
				t.Errorf("expected \n%v\n to be \n%v\n", missingSerialTests, tc.expectedPresubmits)
			}
		})
	}
}

func TestPresubmitsForBatch(t *testing.T) {
	testCases := []struct {
		name           string
		prs            []CodeReviewCommon
		changedFiles   *changedFilesAgent
		jobs           []config.Presubmit
		prowYAMLGetter config.ProwYAMLGetter
		expected       []config.Presubmit
	}{
		{
			name: "All jobs get picked",
			prs:  []CodeReviewCommon{*CodeReviewCommonFromPullRequest(getPR("org", "repo", 1))},
			jobs: []config.Presubmit{{
				AlwaysRun: true,
				Reporter:  config.Reporter{Context: "foo"},
			}},
			expected: []config.Presubmit{{
				AlwaysRun: true,
				Reporter:  config.Reporter{Context: "foo"},
			}},
		},
		{
			name: "Jobs with branchconfig get picked",
			prs:  []CodeReviewCommon{*CodeReviewCommonFromPullRequest(getPR("org", "repo", 1))},
			jobs: []config.Presubmit{{
				AlwaysRun: true,
				Reporter:  config.Reporter{Context: "foo"},
				Brancher:  config.Brancher{Branches: []string{defaultBranch}},
			}},
			expected: []config.Presubmit{{
				AlwaysRun: true,
				Reporter:  config.Reporter{Context: "foo"},
				Brancher:  config.Brancher{Branches: []string{defaultBranch}},
			}},
		},
		{
			name: "Optional jobs are excluded",
			prs:  []CodeReviewCommon{*CodeReviewCommonFromPullRequest(getPR("org", "repo", 1))},
			jobs: []config.Presubmit{
				{
					AlwaysRun: true,
					Reporter:  config.Reporter{Context: "foo"},
				},
				{
					Reporter: config.Reporter{Context: "bar"},
				},
			},
			expected: []config.Presubmit{{
				AlwaysRun: true,
				Reporter:  config.Reporter{Context: "foo"},
			}},
		},
		{
			name: "Jobs that are required by any of the PRs get included",
			prs: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(getPR("org", "repo", 2)),
				*CodeReviewCommonFromPullRequest(getPR("org", "repo", 1, func(pr *PullRequest) {
					pr.HeadRefOID = githubql.String("sha")
				})),
			},
			jobs: []config.Presubmit{{
				RegexpChangeMatcher: config.RegexpChangeMatcher{
					RunIfChanged: "/very-important",
				},
				Reporter: config.Reporter{Context: "foo"},
			}},
			changedFiles: &changedFilesAgent{
				changeCache: map[changeCacheKey][]string{
					{org: "org", repo: "repo", number: 1, sha: "sha"}: {"/very-important"},
					{org: "org", repo: "repo", number: 2}:             {},
				},
				nextChangeCache: map[changeCacheKey][]string{},
			},
			expected: []config.Presubmit{{
				RegexpChangeMatcher: config.RegexpChangeMatcher{
					RunIfChanged: "/very-important",
				},
				Reporter: config.Reporter{Context: "foo"},
			}},
		},
		{
			name: "Inrepoconfig jobs get included if headref matches",
			prs: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(getPR("org", "repo", 2, func(pr *PullRequest) {
					pr.HeadRefOID = githubql.String("sha2")
				})),
				*CodeReviewCommonFromPullRequest(getPR("org", "repo", 1, func(pr *PullRequest) {
					pr.HeadRefOID = githubql.String("sha1")
				})),
			},
			jobs: []config.Presubmit{
				{
					AlwaysRun: true,
					Reporter:  config.Reporter{Context: "foo"},
				},
			},
			prowYAMLGetter: prowYAMLGetterForHeadRefs([]string{"sha1", "sha2"}, []config.Presubmit{{
				AlwaysRun: true,
				Reporter:  config.Reporter{Context: "bar"},
			}}),
			expected: []config.Presubmit{
				{
					AlwaysRun: true,
					Reporter:  config.Reporter{Context: "foo"},
				},
				{
					AlwaysRun: true,
					Reporter:  config.Reporter{Context: "bar"},
				},
			},
		},
		{
			name: "Inrepoconfig jobs do not get included if headref doesnt match",
			prs: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(getPR("org", "repo", 2, func(pr *PullRequest) {
					pr.HeadRefOID = githubql.String("sha2")
				})),
				*CodeReviewCommonFromPullRequest(getPR("org", "repo", 1, func(pr *PullRequest) {
					pr.HeadRefOID = githubql.String("sha1")
				})),
			},
			jobs: []config.Presubmit{
				{
					AlwaysRun: true,
					Reporter:  config.Reporter{Context: "foo"},
				},
			},
			prowYAMLGetter: prowYAMLGetterForHeadRefs([]string{"other-sha", "sha2"}, []config.Presubmit{{
				AlwaysRun: true,
				Reporter:  config.Reporter{Context: "bar"},
			}}),
			expected: []config.Presubmit{
				{
					AlwaysRun: true,
					Reporter:  config.Reporter{Context: "foo"},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			if tc.changedFiles == nil {
				tc.changedFiles = &changedFilesAgent{
					changeCache: map[changeCacheKey][]string{},
				}
				for _, pr := range tc.prs {
					key := changeCacheKey{
						org:    pr.Org,
						repo:   pr.Repo,
						number: int(pr.Number),
						sha:    string(pr.HeadRefOID),
					}
					tc.changedFiles.changeCache[key] = []string{}
				}
			}

			if err := config.SetPresubmitRegexes(tc.jobs); err != nil {
				t.Fatalf("failed to set presubmit regexes: %v", err)
			}

			inrepoconfig := config.InRepoConfig{}
			if tc.prowYAMLGetter != nil {
				inrepoconfig.Enabled = map[string]*bool{"*": utilpointer.Bool(true)}
			}
			cfg := func() *config.Config {
				return &config.Config{
					JobConfig: config.JobConfig{
						PresubmitsStatic: map[string][]config.Presubmit{
							"org/repo": tc.jobs,
						},
						ProwYAMLGetterWithDefaults: tc.prowYAMLGetter,
					},
					ProwConfig: config.ProwConfig{
						InRepoConfig: inrepoconfig,
					},
				}
			}
			c := &syncController{
				provider:     newGitHubProvider(logrus.WithField("test", tc.name), nil, nil, cfg, nil, false),
				changedFiles: tc.changedFiles,
				config:       cfg,
				logger:       logrus.WithField("test", tc.name),
			}

			presubmits, err := c.presubmitsForBatch(tc.prs, "org", "repo", "baseSHA", defaultBranch)
			if err != nil {
				t.Fatalf("failed to get presubmits for batch: %v", err)
			}
			// Clear regexes, otherwise DeepEqual comparison wont work
			config.ClearCompiledRegexes(presubmits)
			if !apiequality.Semantic.DeepEqual(tc.expected, presubmits) {
				t.Errorf("returned presubmits do not match expected, diff: %v\n", diff.ObjectReflectDiff(tc.expected, presubmits))
			}
		})
	}
}

func TestChangedFilesAgentBatchChanges(t *testing.T) {
	testCases := []struct {
		name         string
		prs          []CodeReviewCommon
		changedFiles *changedFilesAgent
		expected     []string
	}{
		{
			name: "Single PR",
			prs: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(getPR("org", "repo", 1)),
			},
			changedFiles: &changedFilesAgent{
				changeCache: map[changeCacheKey][]string{
					{org: "org", repo: "repo", number: 1}: {"foo"},
				},
			},
			expected: []string{"foo"},
		},
		{
			name: "Multiple PRs, changes are de-duplicated",
			prs: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(getPR("org", "repo", 1)),
				*CodeReviewCommonFromPullRequest(getPR("org", "repo", 2)),
			},
			changedFiles: &changedFilesAgent{
				changeCache: map[changeCacheKey][]string{
					{org: "org", repo: "repo", number: 1}: {"foo"},
					{org: "org", repo: "repo", number: 2}: {"foo", "bar"},
				},
			},
			expected: []string{"bar", "foo"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.changedFiles.nextChangeCache = map[changeCacheKey][]string{}

			result, err := tc.changedFiles.batchChanges(tc.prs)()
			if err != nil {
				t.Fatalf("fauked to get changed files: %v", err)
			}
			if !apiequality.Semantic.DeepEqual(result, tc.expected) {
				t.Errorf("returned changes do not match expected; diff: %v\n", diff.ObjectReflectDiff(tc.expected, result))
			}
		})
	}
}

func getPR(org, name string, number int, opts ...func(*PullRequest)) *PullRequest {
	pr := PullRequest{}
	pr.Repository.Owner.Login = githubql.String(org)
	pr.Repository.NameWithOwner = githubql.String(org + "/" + name)
	pr.Repository.Name = githubql.String(name)
	pr.Number = githubql.Int(number)
	for _, opt := range opts {
		opt(&pr)
	}
	return &pr
}

func TestCacheIndexFuncReturnsDifferentResultsForDifferentInputs(t *testing.T) {
	type orgRepoBranch struct{ org, repo, branch string }

	results := sets.Set[string]{}
	inputs := []orgRepoBranch{
		{"org-a", "repo-a", "branch-a"},
		{"org-a", "repo-a", "branch-b"},
		{"org-a", "repo-b", "branch-a"},
		{"org-b", "repo-a", "branch-a"},
	}
	for _, input := range inputs {
		pj := getProwJob(prowapi.PresubmitJob, input.org, input.repo, input.branch, "123", "", nil)
		idx := cacheIndexFunc(pj)
		if n := len(idx); n != 1 {
			t.Fatalf("expected to get exactly one index back, got %d", n)
		}
		if results.Has(idx[0]) {
			t.Errorf("got duplicate idx %q", idx)
		}
		results.Insert(idx[0])
	}
}

func TestCacheIndexFunc(t *testing.T) {
	testCases := []struct {
		name           string
		prowjob        *prowapi.ProwJob
		expectedResult string
	}{
		{
			name:    "Wrong type, no result",
			prowjob: &prowapi.ProwJob{},
		},
		{
			name:    "No refs, no result",
			prowjob: getProwJob(prowapi.PresubmitJob, "", "", "", "", "", nil),
		},
		{
			name:           "presubmit job",
			prowjob:        getProwJob(prowapi.PresubmitJob, "org", "repo", "master", "123", "", nil),
			expectedResult: "org/repo:master@123",
		},
		{
			name:           "Batch job",
			prowjob:        getProwJob(prowapi.BatchJob, "org", "repo", "next", "1234", "", nil),
			expectedResult: "org/repo:next@1234",
		},
	}

	for idx := range testCases {
		tc := testCases[idx]
		t.Run(tc.name, func(t *testing.T) {
			result := cacheIndexFunc(tc.prowjob)
			if n := len(result); n > 1 {
				t.Errorf("expected at most one result, got %d", n)
			}

			var resultString string
			if len(result) == 1 {
				resultString = result[0]
			}

			if resultString != tc.expectedResult {
				t.Errorf("Expected result %q, got result %q", tc.expectedResult, resultString)
			}
		})
	}
}

func getProwJob(pjtype prowapi.ProwJobType, org, repo, branch, sha string, state prowapi.ProwJobState, pulls []prowapi.Pull) *prowapi.ProwJob {
	pj := &prowapi.ProwJob{}
	pj.Spec.Type = pjtype
	if org != "" || repo != "" || branch != "" || sha != "" {
		pj.Spec.Refs = &prowapi.Refs{
			Org:     org,
			Repo:    repo,
			BaseRef: branch,
			BaseSHA: sha,
			Pulls:   pulls,
		}
	}
	pj.Status.State = state
	return pj
}

func newFakeManager(objs ...runtime.Object) *fakeManager {
	client := &indexingClient{
		Client:     fakectrlruntimeclient.NewFakeClient(objs...),
		indexFuncs: map[string]ctrlruntimeclient.IndexerFunc{},
	}
	return &fakeManager{
		client: client,
		fakeFieldIndexer: &fakeFieldIndexer{
			client: client,
		},
	}
}

type fakeManager struct {
	client *indexingClient
	*fakeFieldIndexer
}

type fakeFieldIndexer struct {
	client *indexingClient
}

func (fi *fakeFieldIndexer) IndexField(_ context.Context, _ ctrlruntimeclient.Object, field string, extractValue ctrlruntimeclient.IndexerFunc) error {
	fi.client.indexFuncs[field] = extractValue
	return nil
}

func (fm *fakeManager) GetClient() ctrlruntimeclient.Client {
	return fm.client
}

func (fm *fakeManager) GetFieldIndexer() ctrlruntimeclient.FieldIndexer {
	return fm.fakeFieldIndexer
}

type indexingClient struct {
	ctrlruntimeclient.Client
	indexFuncs map[string]ctrlruntimeclient.IndexerFunc
}

func (c *indexingClient) List(ctx context.Context, list ctrlruntimeclient.ObjectList, opts ...ctrlruntimeclient.ListOption) error {
	if err := c.Client.List(ctx, list, opts...); err != nil {
		return err
	}

	listOpts := &ctrlruntimeclient.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOpts)
	}

	if listOpts.FieldSelector == nil {
		return nil
	}

	if n := len(listOpts.FieldSelector.Requirements()); n == 0 {
		return nil
	} else if n > 1 {
		return fmt.Errorf("the indexing client supports at most one field selector requirement, got %d", n)
	}

	indexKey := listOpts.FieldSelector.Requirements()[0].Field
	if indexKey == "" {
		return nil
	}

	indexFunc, ok := c.indexFuncs[indexKey]
	if !ok {
		return fmt.Errorf("no index with key %q found", indexKey)
	}

	pjList, ok := list.(*prowapi.ProwJobList)
	if !ok {
		return errors.New("indexes are only supported for ProwJobLists")
	}

	result := prowapi.ProwJobList{}
	for _, pj := range pjList.Items {
		for _, indexVal := range indexFunc(&pj) {
			logrus.Infof("indexVal: %q, requirementVal: %q, match: %t", indexVal, listOpts.FieldSelector.Requirements()[0].Value, indexVal == listOpts.FieldSelector.Requirements()[0].Value)
			if indexVal == listOpts.FieldSelector.Requirements()[0].Value {
				result.Items = append(result.Items, pj)
			}
		}
	}

	*pjList = result
	return nil
}

func prowYAMLGetterForHeadRefs(headRefsToLookFor []string, ps []config.Presubmit) config.ProwYAMLGetter {
	return func(_ *config.Config, _ git.ClientFactory, _, _, _ string, headRefs ...string) (*config.ProwYAML, error) {
		if len(headRefsToLookFor) != len(headRefs) {
			return nil, fmt.Errorf("expcted %d headrefs, got %d", len(headRefsToLookFor), len(headRefs))
		}
		var presubmits []config.Presubmit
		if sets.New[string](headRefsToLookFor...).Equal(sets.New[string](headRefs...)) {
			presubmits = ps
		}
		return &config.ProwYAML{
			Presubmits: presubmits,
		}, nil
	}
}

func TestNonFailedBatchByBaseAndPullsIndexFunc(t *testing.T) {
	successFullBatchJob := func(mods ...func(*prowapi.ProwJob)) *prowapi.ProwJob {
		pj := &prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Type: prowapi.BatchJob,
				Job:  "my-job",
				Refs: &prowapi.Refs{
					Org:     "org",
					Repo:    "repo",
					BaseRef: "master",
					BaseSHA: "base-sha",
					Pulls: []prowapi.Pull{
						{
							Number: 1,
							SHA:    "1",
						},
						{
							Number: 2,
							SHA:    "2",
						},
					},
				},
			},
			Status: prowapi.ProwJobStatus{
				State:          prowapi.SuccessState,
				CompletionTime: &metav1.Time{},
			},
		}

		for _, mod := range mods {
			mod(pj)
		}

		return pj
	}
	const defaultIndexKey = "my-job|org|repo|master|base-sha|1|1|2|2"

	testCases := []struct {
		name     string
		pj       *prowapi.ProwJob
		expected []string
	}{
		{
			name:     "Basic success",
			pj:       successFullBatchJob(),
			expected: []string{defaultIndexKey},
		},
		{
			name: "Pulls reordered, same index",
			pj: successFullBatchJob(func(pj *prowapi.ProwJob) {
				pj.Spec.Refs.Pulls = []prowapi.Pull{
					pj.Spec.Refs.Pulls[1],
					pj.Spec.Refs.Pulls[0],
				}
			}),
			expected: []string{defaultIndexKey},
		},
		{
			name: "Not completed, state is ignored",
			pj: successFullBatchJob(func(pj *prowapi.ProwJob) {
				pj.Status.CompletionTime = nil
				pj.Status.State = prowapi.TriggeredState
			}),
			expected: []string{defaultIndexKey},
		},
		{
			name: "Different name, different index",
			pj: successFullBatchJob(func(pj *prowapi.ProwJob) {
				pj.Spec.Job = "my-other-job"
			}),
			expected: []string{"my-other-job|org|repo|master|base-sha|1|1|2|2"},
		},
		{
			name: "Not a batch, ignored",
			pj: successFullBatchJob(func(pj *prowapi.ProwJob) {
				pj.Spec.Type = prowapi.PresubmitJob
			}),
		},
		{
			name: "No refs, ignored",
			pj: successFullBatchJob(func(pj *prowapi.ProwJob) {
				pj.Spec.Refs = nil
			}),
		},
	}

	for _, tc := range testCases {
		result := nonFailedBatchByNameBaseAndPullsIndexFunc(tc.pj)
		if diff := deep.Equal(result, tc.expected); diff != nil {
			t.Errorf("Result differs from expected, diff: %v", diff)
		}
	}
}

func TestCheckRunNodesToContexts(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		checkRuns []CheckRun
		expected  []Context
	}{
		{
			name:      "Empty checkrun is ignored",
			checkRuns: []CheckRun{{}},
		},
		{
			name:      "Incomplete checkrun is considered pending",
			checkRuns: []CheckRun{{Name: githubql.String("some-job"), Status: githubql.String("queued")}},
			expected:  []Context{{Context: "some-job", State: githubql.StatusStatePending}},
		},
		{
			name:      "Neutral checkrun is considered success",
			checkRuns: []CheckRun{{Name: githubql.String("some-job"), Status: checkRunStatusCompleted, Conclusion: checkRunConclusionNeutral}},
			expected:  []Context{{Context: "some-job", State: githubql.StatusStateSuccess}},
		},
		{
			name:      "Successful checkrun is considered success",
			checkRuns: []CheckRun{{Name: githubql.String("some-job"), Status: checkRunStatusCompleted, Conclusion: githubql.String(githubql.StatusStateSuccess)}},
			expected:  []Context{{Context: "some-job", State: githubql.StatusStateSuccess}},
		},
		{
			name:      "Other checkrun conclusion is considered failure",
			checkRuns: []CheckRun{{Name: githubql.String("some-job"), Status: checkRunStatusCompleted, Conclusion: "unclear"}},
			expected:  []Context{{Context: "some-job", State: githubql.StatusStateFailure}},
		},
		{
			name: "Multiple checkruns are translated correctly",
			checkRuns: []CheckRun{
				{Name: githubql.String("some-job"), Status: checkRunStatusCompleted, Conclusion: checkRunConclusionNeutral},
				{Name: githubql.String("another-job"), Status: checkRunStatusCompleted, Conclusion: checkRunConclusionNeutral},
			},
			expected: []Context{
				{Context: "another-job", State: githubql.StatusStateSuccess},
				{Context: "some-job", State: githubql.StatusStateSuccess},
			},
		},
		{
			name: "De-duplicate checkruns, success > everything",
			checkRuns: []CheckRun{
				{Name: githubql.String("some-job"), Status: checkRunStatusCompleted, Conclusion: githubql.String("FAILURE")},
				{Name: githubql.String("some-job"), Status: checkRunStatusCompleted, Conclusion: githubql.String("ERROR")},
				{Name: githubql.String("some-job"), Status: checkRunStatusCompleted},
				{Name: githubql.String("some-job"), Status: checkRunStatusCompleted, Conclusion: githubql.String(githubql.StatusStateSuccess)},
			},
			expected: []Context{
				{Context: "some-job", State: githubql.StatusStateSuccess},
			},
		},
		{
			name: "De-duplicate checkruns, neutral > everything",
			checkRuns: []CheckRun{
				{Name: githubql.String("some-job"), Status: checkRunStatusCompleted, Conclusion: githubql.String("FAILURE")},
				{Name: githubql.String("some-job"), Status: checkRunStatusCompleted, Conclusion: githubql.String("ERROR")},
				{Name: githubql.String("some-job"), Status: checkRunStatusCompleted},
				{Name: githubql.String("some-job"), Status: checkRunStatusCompleted, Conclusion: checkRunConclusionNeutral},
			},
			expected: []Context{
				{Context: "some-job", State: githubql.StatusStateSuccess},
			},
		},
		{
			name: "De-duplicate checkruns, pending > failure",
			checkRuns: []CheckRun{
				{Name: githubql.String("some-job"), Status: checkRunStatusCompleted, Conclusion: githubql.String("FAILURE")},
				{Name: githubql.String("some-job"), Status: checkRunStatusCompleted, Conclusion: githubql.String("ERROR")},
				{Name: githubql.String("some-job"), Status: checkRunStatusCompleted},
				{Name: githubql.String("some-job")},
			},
			expected: []Context{
				{Context: "some-job", State: githubql.StatusStatePending},
			},
		},
		{
			name: "De-duplicate checkruns, only failures",
			checkRuns: []CheckRun{
				{Name: githubql.String("some-job"), Status: checkRunStatusCompleted, Conclusion: githubql.String("FAILURE")},
				{Name: githubql.String("some-job"), Status: checkRunStatusCompleted, Conclusion: githubql.String("ERROR")},
				{Name: githubql.String("some-job"), Status: checkRunStatusCompleted},
			},
			expected: []Context{
				{Context: "some-job", State: githubql.StatusStateFailure},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Shuffle the checkruns to make sure we don't rely on slice order
			rand.Shuffle(len(tc.checkRuns), func(i, j int) {
				tc.checkRuns[i], tc.checkRuns[j] = tc.checkRuns[j], tc.checkRuns[i]
			})

			var checkRunNodes []CheckRunNode
			for _, checkRun := range tc.checkRuns {
				checkRunNodes = append(checkRunNodes, CheckRunNode{CheckRun: checkRun})
			}

			result := checkRunNodesToContexts(logrus.New().WithField("test", tc.name), checkRunNodes)
			sort.Slice(result, func(i, j int) bool {
				return result[i].Context+result[i].Description+githubql.String(result[i].State) < result[j].Context+result[j].Description+githubql.String(result[j].State)
			})

			if diff := cmp.Diff(result, tc.expected); diff != "" {
				t.Errorf("actual result differs from expected: %s", diff)
			}
		})
	}
}

func TestDeduplicateContestsDoesntLoseData(t *testing.T) {
	seed := time.Now().UnixNano()
	// Print the seed so failures can easily be reproduced
	t.Logf("Seed: %d", seed)
	fuzzer := fuzz.NewWithSeed(seed)
	for i := 0; i < 100; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			context := Context{}
			fuzzer.Fuzz(&context)
			res := deduplicateContexts([]Context{context})
			if diff := cmp.Diff(context, res[0]); diff != "" {
				t.Errorf("deduplicateContexts lost data, new object differs: %s", diff)
			}
		})
	}
}

func TestPickSmallestPassingNumber(t *testing.T) {
	priorities := []config.TidePriority{
		{Labels: []string{"kind/failing-test"}},
		{Labels: []string{"area/deflake"}},
		{Labels: []string{"kind/bug", "priority/critical-urgent"}},
		{Labels: []string{"kind/feature,kind/enhancement,kind/undefined"}},
	}
	testCases := []struct {
		name     string
		prs      []CodeReviewCommon
		expected int
	}{
		{
			name: "no label",
			prs: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(testPR("org", "repo", "A", 5, githubql.MergeableStateMergeable)),
				*CodeReviewCommonFromPullRequest(testPR("org", "repo", "A", 3, githubql.MergeableStateMergeable)),
			},
			expected: 3,
		},
		{
			name: "any of given label alternatives",
			prs: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(testPRWithLabels("org", "repo", "A", 3, githubql.MergeableStateMergeable, []string{"kind/enhancement", "kind/undefined"})),
				*CodeReviewCommonFromPullRequest(testPRWithLabels("org", "repo", "A", 1, githubql.MergeableStateMergeable, []string{"kind/enhancement"})),
			},
			expected: 1,
		},
		{
			name: "deflake PR",
			prs: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(testPR("org", "repo", "A", 5, githubql.MergeableStateMergeable)),
				*CodeReviewCommonFromPullRequest(testPR("org", "repo", "A", 3, githubql.MergeableStateMergeable)),
				*CodeReviewCommonFromPullRequest(testPRWithLabels("org", "repo", "A", 7, githubql.MergeableStateMergeable, []string{"area/deflake"})),
			},
			expected: 7,
		},
		{
			name: "same label",
			prs: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(testPRWithLabels("org", "repo", "A", 7, githubql.MergeableStateMergeable, []string{"area/deflake"})),
				*CodeReviewCommonFromPullRequest(testPRWithLabels("org", "repo", "A", 6, githubql.MergeableStateMergeable, []string{"area/deflake"})),
				*CodeReviewCommonFromPullRequest(testPRWithLabels("org", "repo", "A", 1, githubql.MergeableStateMergeable, []string{"area/deflake"})),
			},
			expected: 1,
		},
		{
			name: "missing one label",
			prs: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(testPR("org", "repo", "A", 5, githubql.MergeableStateMergeable)),
				*CodeReviewCommonFromPullRequest(testPR("org", "repo", "A", 3, githubql.MergeableStateMergeable)),
				*CodeReviewCommonFromPullRequest(testPRWithLabels("org", "repo", "A", 6, githubql.MergeableStateMergeable, []string{"kind/bug"})),
			},
			expected: 3,
		},
		{
			name: "complete",
			prs: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(testPR("org", "repo", "A", 5, githubql.MergeableStateMergeable)),
				*CodeReviewCommonFromPullRequest(testPR("org", "repo", "A", 3, githubql.MergeableStateMergeable)),
				*CodeReviewCommonFromPullRequest(testPRWithLabels("org", "repo", "A", 6, githubql.MergeableStateMergeable, []string{"kind/bug"})),
				*CodeReviewCommonFromPullRequest(testPRWithLabels("org", "repo", "A", 7, githubql.MergeableStateMergeable, []string{"area/deflake"})),
				*CodeReviewCommonFromPullRequest(testPRWithLabels("org", "repo", "A", 8, githubql.MergeableStateMergeable, []string{"kind/bug"})),
				*CodeReviewCommonFromPullRequest(testPRWithLabels("org", "repo", "A", 9, githubql.MergeableStateMergeable, []string{"kind/failing-test"})),
				*CodeReviewCommonFromPullRequest(testPRWithLabels("org", "repo", "A", 10, githubql.MergeableStateMergeable, []string{"kind/bug", "priority/critical-urgent"})),
			},
			expected: 9,
		},
	}
	alwaysTrue := func(*logrus.Entry, *CodeReviewCommon, contextChecker) bool { return true }
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, got := pickHighestPriorityPR(nil, tc.prs, nil, alwaysTrue, priorities)
			if int(got.Number) != tc.expected {
				t.Errorf("got %d, expected %d", int(got.Number), tc.expected)
			}
		})
	}
}

func TestQueryShardsByOrgWhenAppsAuthIsEnabledOnly(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                     string
		usesGitHubAppsAuth       bool
		prs                      map[string][]PullRequest
		expectedNumberOfApiCalls int
	}{
		{
			name:               "Apps auth is used, one call per org",
			usesGitHubAppsAuth: true,
			prs: map[string][]PullRequest{
				"org":       {*testPR("org", "repo", "A", 5, githubql.MergeableStateMergeable)},
				"other-org": {*testPR("other-org", "repo", "A", 5, githubql.MergeableStateMergeable)},
			},
			expectedNumberOfApiCalls: 2,
		},
		{
			name:               "Apps auth is unused, one call for all orgs",
			usesGitHubAppsAuth: false,
			prs: map[string][]PullRequest{"": {
				*testPR("org", "repo", "A", 5, githubql.MergeableStateMergeable),
				*testPR("other-org", "repo", "A", 5, githubql.MergeableStateMergeable),
			}},
			expectedNumberOfApiCalls: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provider := &GitHubProvider{
				cfg: func() *config.Config {
					return &config.Config{ProwConfig: config.ProwConfig{Tide: config.Tide{
						TideGitHubConfig: config.TideGitHubConfig{Queries: []config.TideQuery{{Orgs: []string{"org", "other-org"}}}}}}}
				},
				ghc:                &fgc{prs: tc.prs},
				usesGitHubAppsAuth: tc.usesGitHubAppsAuth,
				logger:             logrus.WithField("test", tc.name),
			}

			prs, err := provider.Query()
			if err != nil {
				t.Fatalf("query() failed: %v", err)
			}
			if n := len(prs); n != 2 {
				t.Errorf("expected to get two prs back, got %d", n)
			}
			if diff := cmp.Diff(tc.expectedNumberOfApiCalls, provider.ghc.(*fgc).queryCalls); diff != "" {
				t.Errorf("expectedNumberOfApiCallsByOrg differs from actual: %s", diff)
			}
		})
	}
}

func TestPickBatchPrefersBatchesWithPreexistingJobs(t *testing.T) {
	t.Parallel()
	const org, repo = "org", "repo"
	tests := []struct {
		name                         string
		subpool                      func(*subpool)
		prsFailingContextCheck       sets.Set[int]
		maxBatchSize                 int
		prioritizeExistingBatchesMap map[string]bool

		expectedPullRequests []CodeReviewCommon
	}{
		{
			name:    "No pre-existing jobs, new batch is picked",
			subpool: func(sp *subpool) { sp.pjs = nil },
			expectedPullRequests: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(&PullRequest{Number: 99, HeadRefOID: "pr-from-new-batch-func"}),
			},
		},
		{
			name:    "Batch with pre-existing success jobs exists and is picked",
			subpool: func(sp *subpool) {},
			expectedPullRequests: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(&PullRequest{
					Number:     githubql.Int(1),
					HeadRefOID: githubql.String("1"),
					Commits: struct{ Nodes []struct{ Commit Commit } }{
						Nodes: []struct{ Commit Commit }{
							{Commit: Commit{Status: CommitStatus{Contexts: []Context{}}, OID: "1"}}},
					},
				}),
				*CodeReviewCommonFromPullRequest(&PullRequest{
					Number:     githubql.Int(2),
					HeadRefOID: githubql.String("2"),
					Commits: struct{ Nodes []struct{ Commit Commit } }{
						Nodes: []struct{ Commit Commit }{
							{Commit: Commit{Status: CommitStatus{Contexts: []Context{}}, OID: "2"}}},
					},
				}),
				*CodeReviewCommonFromPullRequest(&PullRequest{
					Number:     githubql.Int(3),
					HeadRefOID: githubql.String("3"),
					Commits: struct{ Nodes []struct{ Commit Commit } }{
						Nodes: []struct{ Commit Commit }{
							{Commit: Commit{Status: CommitStatus{Contexts: []Context{}}, OID: "3"}}},
					},
				}),
				*CodeReviewCommonFromPullRequest(&PullRequest{
					Number:     githubql.Int(4),
					HeadRefOID: githubql.String("4"),
					Commits: struct{ Nodes []struct{ Commit Commit } }{
						Nodes: []struct{ Commit Commit }{
							{Commit: Commit{Status: CommitStatus{Contexts: []Context{}}, OID: "4"}}},
					},
				}),
			},
		},
		{
			name:                         "Batch with pre-existing success jobs exists but PrioritizeExistingBatches is disabled globally, new batch is picked",
			subpool:                      func(sp *subpool) {},
			prioritizeExistingBatchesMap: map[string]bool{"*": false},
			expectedPullRequests: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(&PullRequest{Number: 99, HeadRefOID: "pr-from-new-batch-func"}),
			},
		},
		{
			name:                         "Batch with pre-existing success jobs exists but PrioritizeExistingBatches is disabled for org, new batch is picked",
			subpool:                      func(sp *subpool) {},
			prioritizeExistingBatchesMap: map[string]bool{org: false},
			expectedPullRequests: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(&PullRequest{Number: 99, HeadRefOID: "pr-from-new-batch-func"}),
			},
		},
		{
			name:                         "Batch with pre-existing success jobs exists but PrioritizeExistingBatches is disabled for repo, new batch is picked",
			subpool:                      func(sp *subpool) {},
			prioritizeExistingBatchesMap: map[string]bool{org + "/" + repo: false},
			expectedPullRequests: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(&PullRequest{Number: 99, HeadRefOID: "pr-from-new-batch-func"}),
			},
		},
		{
			name:                   "Batch with pre-existing success job exists but one fails context check, new batch is picked",
			subpool:                func(sp *subpool) {},
			prsFailingContextCheck: sets.New[int](1),
			expectedPullRequests: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(&PullRequest{Number: 99, HeadRefOID: "pr-from-new-batch-func"}),
			},
		},
		{
			name:         "Batch with pre-existing success job exists but is bigger than maxBatchSize, new batch is picked",
			subpool:      func(sp *subpool) {},
			maxBatchSize: 3,
			expectedPullRequests: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(&PullRequest{Number: 99, HeadRefOID: "pr-from-new-batch-func"}),
			},
		},
		{
			name:    "Batch with pre-existing success job exists but one PR is outdated, new batch is picked",
			subpool: func(sp *subpool) { sp.prs[0].HeadRefOID = "new-sha" },
			expectedPullRequests: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(&PullRequest{Number: 99, HeadRefOID: "pr-from-new-batch-func"}),
			},
		},
		{
			name:    "Batchjobs exist but is failed, new batch is picked",
			subpool: func(sp *subpool) { sp.pjs[0].Status.State = prowapi.FailureState },
			expectedPullRequests: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(&PullRequest{Number: 99, HeadRefOID: "pr-from-new-batch-func"}),
			},
		},
		{
			name: "Batch with pre-existing success jobs and batch with pre-existing pending jobs exists, batch with success jobs is picked",
			subpool: func(sp *subpool) {
				sp.pjs = append(sp.pjs, *sp.pjs[0].DeepCopy())
				sp.pjs[0].Spec.Refs.Pulls = []prowapi.Pull{{Number: 1, SHA: "1"}, {Number: 2, SHA: "2"}}

				sp.pjs[1].Status.State = prowapi.PendingState
				sp.pjs[1].Spec.Refs.Pulls = []prowapi.Pull{{Number: 3, SHA: "3"}, {Number: 4, SHA: "4"}}
			},
			expectedPullRequests: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(&PullRequest{
					Number:     githubql.Int(1),
					HeadRefOID: githubql.String("1"),
					Commits: struct{ Nodes []struct{ Commit Commit } }{
						Nodes: []struct{ Commit Commit }{
							{Commit: Commit{Status: CommitStatus{Contexts: []Context{}}, OID: "1"}}},
					},
				}),
				*CodeReviewCommonFromPullRequest(&PullRequest{
					Number:     githubql.Int(2),
					HeadRefOID: githubql.String("2"),
					Commits: struct{ Nodes []struct{ Commit Commit } }{
						Nodes: []struct{ Commit Commit }{
							{Commit: Commit{Status: CommitStatus{Contexts: []Context{}}, OID: "2"}}},
					},
				}),
			},
		},
		{
			name:    "Batch with pre-existing pending jobs exists and is picked",
			subpool: func(sp *subpool) { sp.pjs[0].Status.State = prowapi.PendingState },
			expectedPullRequests: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(&PullRequest{
					Number:     githubql.Int(1),
					HeadRefOID: githubql.String("1"),
					Commits: struct{ Nodes []struct{ Commit Commit } }{
						Nodes: []struct{ Commit Commit }{
							{Commit: Commit{Status: CommitStatus{Contexts: []Context{}}, OID: "1"}}},
					},
				}),
				*CodeReviewCommonFromPullRequest(&PullRequest{
					Number:     githubql.Int(2),
					HeadRefOID: githubql.String("2"),
					Commits: struct{ Nodes []struct{ Commit Commit } }{
						Nodes: []struct{ Commit Commit }{
							{Commit: Commit{Status: CommitStatus{Contexts: []Context{}}, OID: "2"}}},
					},
				}),
				*CodeReviewCommonFromPullRequest(&PullRequest{
					Number:     githubql.Int(3),
					HeadRefOID: githubql.String("3"),
					Commits: struct{ Nodes []struct{ Commit Commit } }{
						Nodes: []struct{ Commit Commit }{
							{Commit: Commit{Status: CommitStatus{Contexts: []Context{}}, OID: "3"}}},
					},
				}),
				*CodeReviewCommonFromPullRequest(&PullRequest{
					Number:     githubql.Int(4),
					HeadRefOID: githubql.String("4"),
					Commits: struct{ Nodes []struct{ Commit Commit } }{
						Nodes: []struct{ Commit Commit }{
							{Commit: Commit{Status: CommitStatus{Contexts: []Context{}}, OID: "4"}}},
					},
				}),
			},
		},
		{
			name: "Multiple success batches exists, the one with the highest number of tests is picked",
			subpool: func(sp *subpool) {
				sp.pjs = append(sp.pjs, *sp.pjs[0].DeepCopy(), *sp.pjs[0].DeepCopy())

				sp.pjs[1].Spec.Refs.Pulls = []prowapi.Pull{{Number: 3, SHA: "3"}, {Number: 4, SHA: "4"}}
				sp.pjs[2].Spec.Refs.Pulls = []prowapi.Pull{{Number: 3, SHA: "3"}, {Number: 4, SHA: "4"}}
			},
			expectedPullRequests: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(&PullRequest{
					Number:     githubql.Int(3),
					HeadRefOID: githubql.String("3"),
					Commits: struct{ Nodes []struct{ Commit Commit } }{
						Nodes: []struct{ Commit Commit }{
							{Commit: Commit{Status: CommitStatus{Contexts: []Context{}}, OID: "3"}}},
					},
				}),
				*CodeReviewCommonFromPullRequest(&PullRequest{
					Number:     githubql.Int(4),
					HeadRefOID: githubql.String("4"),
					Commits: struct{ Nodes []struct{ Commit Commit } }{
						Nodes: []struct{ Commit Commit }{
							{Commit: Commit{Status: CommitStatus{Contexts: []Context{}}, OID: "4"}}},
					},
				}),
			},
		},
		{
			name: "Multiple pending batches exist, the one with the highest number of tests is picked",
			subpool: func(sp *subpool) {
				sp.pjs[0].Status.State = prowapi.PendingState
				sp.pjs = append(sp.pjs, *sp.pjs[0].DeepCopy(), *sp.pjs[0].DeepCopy())

				sp.pjs[1].Spec.Refs.Pulls = []prowapi.Pull{{Number: 3, SHA: "3"}, {Number: 4, SHA: "4"}}
				sp.pjs[2].Spec.Refs.Pulls = []prowapi.Pull{{Number: 3, SHA: "3"}, {Number: 4, SHA: "4"}}
			},
			expectedPullRequests: []CodeReviewCommon{
				*CodeReviewCommonFromPullRequest(&PullRequest{
					Number:     githubql.Int(3),
					HeadRefOID: githubql.String("3"),
					Commits: struct{ Nodes []struct{ Commit Commit } }{
						Nodes: []struct{ Commit Commit }{
							{Commit: Commit{Status: CommitStatus{Contexts: []Context{}}, OID: "3"}}},
					},
				}),
				*CodeReviewCommonFromPullRequest(&PullRequest{
					Number:     githubql.Int(4),
					HeadRefOID: githubql.String("4"),
					Commits: struct{ Nodes []struct{ Commit Commit } }{
						Nodes: []struct{ Commit Commit }{
							{Commit: Commit{Status: CommitStatus{Contexts: []Context{}}, OID: "4"}}},
					},
				}),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sp := subpool{
				org:  org,
				repo: repo,
				log:  logrus.WithField("test", tc.name),
				prs: []CodeReviewCommon{
					*CodeReviewCommonFromPullRequest(&PullRequest{Number: 1, HeadRefOID: "1"}),
					*CodeReviewCommonFromPullRequest(&PullRequest{Number: 2, HeadRefOID: "2"}),
					*CodeReviewCommonFromPullRequest(&PullRequest{Number: 3, HeadRefOID: "3"}),
					*CodeReviewCommonFromPullRequest(&PullRequest{Number: 4, HeadRefOID: "4"}),
					*CodeReviewCommonFromPullRequest(&PullRequest{Number: 5, HeadRefOID: "5"}),
				},
				pjs: []prowapi.ProwJob{{
					Spec: prowapi.ProwJobSpec{
						Refs: &prowapi.Refs{Pulls: []prowapi.Pull{
							{Number: 1, SHA: "1"},
							{Number: 2, SHA: "2"},
							{Number: 3, SHA: "3"},
							{Number: 4, SHA: "4"},
						}},
						Type: prowapi.BatchJob,
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.SuccessState,
					},
				}},
			}
			tc.subpool(&sp)

			contextCheckers := make(map[int]contextChecker, len(sp.prs))
			for _, pr := range sp.prs {
				cc := &config.TideContextPolicy{}
				if tc.prsFailingContextCheck.Has(int(pr.Number)) {
					cc.RequiredContexts = []string{"guaranteed-absent"}
				}
				contextCheckers[int(pr.Number)] = cc
			}

			newBatchFunc := func(sp subpool, candidates []CodeReviewCommon, maxBatchSize int) ([]CodeReviewCommon, error) {
				return []CodeReviewCommon{
					*CodeReviewCommonFromPullRequest(&PullRequest{Number: 99, HeadRefOID: "pr-from-new-batch-func"})}, nil
			}

			cfg := func() *config.Config {
				return &config.Config{ProwConfig: config.ProwConfig{
					Tide: config.Tide{
						BatchSizeLimitMap:            map[string]int{"*": tc.maxBatchSize},
						PrioritizeExistingBatchesMap: tc.prioritizeExistingBatchesMap,
					}},
				}
			}

			logger := logrus.WithField("test", tc.name)
			ghc := &fgc{skipExpectedShaCheck: true}
			c := &syncController{
				logger: logrus.WithField("test", tc.name),
				config: cfg,
				provider: &GitHubProvider{
					cfg:    cfg,
					logger: logger,
					ghc:    ghc,
				},
			}
			prs, _, err := c.pickBatch(sp, contextCheckers, newBatchFunc)
			if err != nil {
				t.Fatalf("pickBatch failed: %v", err)
			}
			if diff := cmp.Diff(tc.expectedPullRequests, prs); diff != "" {
				t.Errorf("expected pull requests differ from actual: %s", diff)
			}
		})

	}
}

func TestTenantIDs(t *testing.T) {
	tests := []struct {
		name     string
		pjs      []prowapi.ProwJob
		expected []string
	}{
		{
			name:     "no PJs",
			pjs:      []prowapi.ProwJob{},
			expected: []string{},
		},
		{
			name: "one PJ",
			pjs: []prowapi.ProwJob{
				{
					Spec: prowapi.ProwJobSpec{
						ProwJobDefault: &prowapi.ProwJobDefault{
							TenantID: "test",
						},
					},
				},
			},
			expected: []string{"test"},
		},
		{
			name: "multiple PJs with same ID",
			pjs: []prowapi.ProwJob{
				{
					Spec: prowapi.ProwJobSpec{
						ProwJobDefault: &prowapi.ProwJobDefault{
							TenantID: "test",
						},
					},
				},
				{
					Spec: prowapi.ProwJobSpec{
						ProwJobDefault: &prowapi.ProwJobDefault{
							TenantID: "test",
						},
					},
				},
			},
			expected: []string{"test"},
		},
		{
			name: "multiple PJs with different ID",
			pjs: []prowapi.ProwJob{
				{
					Spec: prowapi.ProwJobSpec{
						ProwJobDefault: &prowapi.ProwJobDefault{
							TenantID: "test",
						},
					},
				},
				{
					Spec: prowapi.ProwJobSpec{
						ProwJobDefault: &prowapi.ProwJobDefault{
							TenantID: "other",
						},
					},
				},
			},
			expected: []string{"test", "other"},
		},
		{
			name: "no tenantID in prowJob",
			pjs: []prowapi.ProwJob{
				{
					Spec: prowapi.ProwJobSpec{
						ProwJobDefault: &prowapi.ProwJobDefault{
							TenantID: "test",
						},
					},
				},
				{
					Spec: prowapi.ProwJobSpec{
						ProwJobDefault: &prowapi.ProwJobDefault{},
					},
				},
			},
			expected: []string{"test", ""},
		},
		{
			name: "no pjDefault in prowJob",
			pjs: []prowapi.ProwJob{
				{
					Spec: prowapi.ProwJobSpec{
						ProwJobDefault: &prowapi.ProwJobDefault{
							TenantID: "test",
						},
					},
				},
				{
					Spec: prowapi.ProwJobSpec{},
				},
			},
			expected: []string{"test", ""},
		},
		{
			name: "multiple no tenant PJs",
			pjs: []prowapi.ProwJob{
				{
					Spec: prowapi.ProwJobSpec{
						ProwJobDefault: &prowapi.ProwJobDefault{
							TenantID: "",
						},
					},
				},
				{
					Spec: prowapi.ProwJobSpec{},
				},
			},
			expected: []string{""},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sp := subpool{pjs: tc.pjs}
			if diff := cmp.Diff(tc.expected, sp.TenantIDs(), cmpopts.SortSlices(func(x, y string) bool { return strings.Compare(x, y) > 0 })); diff != "" {
				t.Errorf("expected tenantIDs differ from actual: %s", diff)
			}
		})
	}
}

func TestSetTideStatusSuccess(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name string
		pr   PullRequest

		expectApiCall bool
	}{
		{
			name:          "Status is set",
			expectApiCall: true,
		},
		{
			name: "PR already has tide status set to success, no api call is made",
			pr:   PullRequest{Commits: struct{ Nodes []struct{ Commit Commit } }{Nodes: []struct{ Commit Commit }{{Commit: Commit{Status: CommitStatus{Contexts: []Context{{Context: "tide", State: githubql.StatusState("success")}}}}}}}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ghc := &fgc{}
			crc := CodeReviewCommonFromPullRequest(&tc.pr)
			err := setTideStatusSuccess(*crc, ghc, &config.Config{}, logrus.WithField("test", tc.name))
			if err != nil {
				t.Fatalf("failed to set status: %v", err)
			}

			if ghc.setStatus != tc.expectApiCall {
				t.Errorf("expected CreateStatusApiCall: %t, got CreateStatusApiCall: %t", tc.expectApiCall, ghc.setStatus)
			}
		})
	}
}

// TestBatchPickingConsidersPRThatIsCurrentlyBeingSeriallyRetested verifies the following sequence of events:
// 1. Tide creates a serial retest run for a passing PR
// 2. The status contexts on the PR get updated to pending
// 3. A second PR becomes eligible
// 4. Tide creates a batch of the first and the second PR
func TestBatchPickingConsidersPRThatIsCurrentlyBeingSeriallyRetested(t *testing.T) {
	t.Parallel()
	configGetter := func() *config.Config {
		return &config.Config{
			ProwConfig: config.ProwConfig{
				Tide: config.Tide{
					MaxGoroutines: 1,
					TideGitHubConfig: config.TideGitHubConfig{
						Queries: config.TideQueries{{}},
					},
				},
			},
			JobConfig: config.JobConfig{PresubmitsStatic: map[string][]config.Presubmit{
				"/": {{AlwaysRun: true, Reporter: config.Reporter{Context: "mandatory-job"}}},
			}},
		}
	}
	ghc := &fgc{}
	mmc := newMergeChecker(configGetter, ghc)
	mgr := newFakeManager()
	log := logrus.WithField("test", t.Name())
	history, err := history.New(1, nil, "")
	if err != nil {
		t.Fatalf("failed to construct history: %v", err)
	}
	ghProvider := newGitHubProvider(log, ghc, nil, configGetter, mmc, false)
	c, err := newSyncController(
		context.Background(),
		log,
		mgr,
		ghProvider,
		configGetter,
		nil,
		history,
		false,
		&statusUpdate{
			dontUpdateStatus: &threadSafePRSet{},
			newPoolPending:   make(chan bool),
		},
	)
	if err != nil {
		t.Fatalf("failed to construct sync controller: %v", err)
	}
	c.pickNewBatch = func(sp subpool, candidates []CodeReviewCommon, maxBatchSize int) ([]CodeReviewCommon, error) {
		return candidates, nil
	}

	// Add a successful PR to github
	initialPR := PullRequest{}
	initialPR.Commits.Nodes = append(initialPR.Commits.Nodes, struct{ Commit Commit }{
		Commit: Commit{Status: CommitStatus{Contexts: []Context{
			{
				Context: githubql.String("mandatory-job"),
				State:   githubql.StatusStateSuccess,
			},
			{
				Context: githubql.String(statusContext),
				State:   githubql.StatusStatePending,
			},
		}}},
	})
	ghc.prs = map[string][]PullRequest{"": {initialPR}}

	// sync, this creates a new serial retest prowjob
	if err := c.Sync(); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	// Ensure there is actually the retest job
	var pjs prowapi.ProwJobList
	if err := c.prowJobClient.List(c.ctx, &pjs); err != nil {
		t.Fatalf("failed to list prowjobs: %v", err)
	}
	if n := len(pjs.Items); n != 1 {
		t.Fatalf("expected a prowjob to be created, but client had %d items", n)
	}

	// Update the context on the PR to pending just like crier would
	for idx, ctx := range initialPR.Commits.Nodes[0].Commit.Status.Contexts {
		if pjs.Items[0].Spec.Context == string(ctx.Context) {
			initialPR.Commits.Nodes[0].Commit.Status.Contexts[idx].State = githubql.StatusStatePending
		}
	}

	// Add a second PR that also needs retesting to GitHub
	secondPR := PullRequest{Number: githubql.Int(1)}
	secondPR.Commits.Nodes = append(secondPR.Commits.Nodes, struct{ Commit Commit }{
		Commit: Commit{Status: CommitStatus{Contexts: []Context{
			{
				Context: githubql.String("mandatory-job"),
				State:   githubql.StatusStateSuccess,
			},
			{
				Context: githubql.String(statusContext),
				State:   githubql.StatusStatePending,
			},
		}}},
	})
	ghc.prs[""] = append(ghc.prs[""], secondPR)

	// sync again
	if err := c.Sync(); err != nil {
		t.Fatalf("failed to sync: %v", err)
	}

	// verify we have a batch prowjob
	if err := c.prowJobClient.List(c.ctx, &pjs); err != nil {
		t.Fatalf("failed to list prowjobs: %v", err)
	}
	for _, pj := range pjs.Items {
		if pj.Spec.Type == prowapi.BatchJob {
			return
		}
	}

	t.Errorf("expected to find a batch prwjob, but wasn't the case. ProwJobs: %+v", pjs.Items)
}

func TestIsBatchCandidateEligible(t *testing.T) {
	t.Parallel()

	const (
		requiredContextName = "required-context"
		optionalContextName = "optional-context"
	)

	tcs := []struct {
		name          string
		pjManipulator func(**prowapi.ProwJob)
		prManipulator func(*PullRequest)

		expected bool
	}{
		{
			name:     "Is eligible",
			expected: true,
		},
		{
			name: "Successful context doesn't require prowjob",
			prManipulator: func(pr *PullRequest) {
				pr.Commits.Nodes[0].Commit.Status.Contexts[0].State = githubql.StatusStateSuccess
			},
			pjManipulator: func(pj **prowapi.ProwJob) { *pj = nil },
			expected:      true,
		},
		{
			name:     "Optional failed context is ignored",
			expected: true,
			prManipulator: func(pr *PullRequest) {
				pr.Commits.Nodes[0].Commit.Status.Contexts = append(pr.Commits.Nodes[0].Commit.Status.Contexts, Context{
					Context: githubql.String(optionalContextName),
					State:   githubql.StatusStateFailure,
				})
			},
		},
		{
			name:     "Tides own context is ignored",
			expected: true,
			prManipulator: func(pr *PullRequest) {
				pr.Commits.Nodes[0].Commit.Status.Contexts = append(pr.Commits.Nodes[0].Commit.Status.Contexts, Context{
					Context: githubql.String(statusContext),
					State:   githubql.StatusStateFailure,
				})
			},
		},
		{
			name:          "Has missing required context, not eligible",
			prManipulator: func(pr *PullRequest) { pr.Commits.Nodes[0].Commit.Status.Contexts = nil },
		},
		{
			name: "Has failed context, not eligible",
			prManipulator: func(pr *PullRequest) {
				pr.Commits.Nodes[0].Commit.Status.Contexts[0].State = githubql.StatusStateFailure
			},
		},
		{
			name: "Has error context, not eligible",
			prManipulator: func(pr *PullRequest) {
				pr.Commits.Nodes[0].Commit.Status.Contexts[0].State = githubql.StatusStateError
			},
		},
		{
			name:          "No prowjob, not eligible",
			pjManipulator: func(pj **prowapi.ProwJob) { *pj = nil },
		},
		{
			name:          "Pj doesn't have created by tide label, not eligible",
			pjManipulator: func(pj **prowapi.ProwJob) { (*pj).Labels["created-by-tide"] = "wrong" },
		},
		{
			name:          "Pj doesn't have presubmit label, not eligible",
			pjManipulator: func(pj **prowapi.ProwJob) { (*pj).Labels[kube.ProwJobTypeLabel] = "wrong" },
		},
		{
			name:          "PJ doesn't have org label, not eligible",
			pjManipulator: func(pj **prowapi.ProwJob) { (*pj).Labels[kube.OrgLabel] = "wrong" },
		},
		{
			name:          "PJ doesn't have repo label, not eligible",
			pjManipulator: func(pj **prowapi.ProwJob) { (*pj).Labels[kube.RepoLabel] = "wrong" },
		},
		{
			name:          "Pj doesn't have baseref label, not eligible",
			pjManipulator: func(pj **prowapi.ProwJob) { (*pj).Labels[kube.BaseRefLabel] = "wrong" },
		},
		{
			name:          "Pj doesn't have pull label, not eligible",
			pjManipulator: func(pj **prowapi.ProwJob) { (*pj).Labels[kube.PullLabel] = "wrong" },
		},
		{
			name:          "pj doesn't have context label, not eligible",
			pjManipulator: func(pj **prowapi.ProwJob) { (*pj).Labels[kube.ContextAnnotation] = "wrong" },
		},
		{
			name:          "Pj is for wrong headref, not eligible",
			pjManipulator: func(pj **prowapi.ProwJob) { (*pj).Spec.Refs.Pulls[0].SHA = "wrong" },
		},
	}

	newPR := func() PullRequest {
		pr := PullRequest{}
		pr.Commits.Nodes = append(pr.Commits.Nodes, struct{ Commit Commit }{Commit: Commit{Status: CommitStatus{Contexts: []Context{
			{Context: githubql.String(requiredContextName), State: githubql.StatusStatePending},
		}}}})
		return pr
	}
	newProwJob := func() *prowapi.ProwJob {
		return &prowapi.ProwJob{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"created-by-tide":      "true",
				kube.ProwJobTypeLabel:  "presubmit",
				kube.OrgLabel:          "",
				kube.RepoLabel:         "",
				kube.BaseRefLabel:      "",
				kube.PullLabel:         "0",
				kube.ContextAnnotation: requiredContextName,
			}},
			Spec: prowapi.ProwJobSpec{Refs: &prowapi.Refs{Pulls: []prowapi.Pull{{}}}},
		}
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			pj := newProwJob()
			pr := newPR()

			if tc.prManipulator != nil {
				tc.prManipulator(&pr)
			}
			if tc.pjManipulator != nil {
				tc.pjManipulator(&pj)
			}

			var initObjects []runtime.Object
			if pj != nil {
				initObjects = append(initObjects, pj)
			}

			cfg := func() *config.Config { return &config.Config{} }
			c := &syncController{
				config:        cfg,
				provider:      &GitHubProvider{cfg: cfg},
				ctx:           context.Background(),
				prowJobClient: fakectrlruntimeclient.NewFakeClient(initObjects...),
			}

			cc := &config.TideContextPolicy{
				RequiredContexts: []string{requiredContextName},
				OptionalContexts: []string{optionalContextName},
			}

			if actual := c.isRetestEligible(logrus.WithField("tc", tc.name), CodeReviewCommonFromPullRequest(&pr), cc); actual != tc.expected {
				t.Errorf("expected result %t, got %t", tc.expected, actual)
			}
		})
	}
}

// TestSerialRetestingConsidersPRThatIsCurrentlyBeingSRetested verifies the following sequence of events:
// 1. Tide creates a serial retest run for a passing PR
// 2. The status context on the PR gets updated to pending
// 3. Another PR gets merged and changed the baseSHA, for example because it already had up-to-date tests but was missing labels
// 4. Tide will again trigger serial retests for the passing PR (The runs from step 1 will be deleted by Plank)
func TestSerialRetestingConsidersPRThatIsCurrentlyBeingSRetested(t *testing.T) {
	t.Parallel()
	configGetter := func() *config.Config {
		return &config.Config{
			ProwConfig: config.ProwConfig{
				Tide: config.Tide{
					MaxGoroutines: 1,
					TideGitHubConfig: config.TideGitHubConfig{
						Queries: config.TideQueries{{}},
					},
				},
			},
			JobConfig: config.JobConfig{PresubmitsStatic: map[string][]config.Presubmit{
				"/": {{AlwaysRun: true, Reporter: config.Reporter{Context: "mandatory-job"}}},
			}},
		}
	}
	ghc := &fgc{}
	mmc := newMergeChecker(configGetter, ghc)
	mgr := newFakeManager()
	log := logrus.WithField("test", t.Name())
	history, err := history.New(1, nil, "")
	if err != nil {
		t.Fatalf("failed to construct history: %v", err)
	}
	ghProvider := newGitHubProvider(log, ghc, nil, configGetter, mmc, false)
	c, err := newSyncController(
		context.Background(),
		log,
		mgr,
		ghProvider,
		configGetter,
		nil,
		history,
		false,
		&statusUpdate{
			dontUpdateStatus: &threadSafePRSet{},
			newPoolPending:   make(chan bool),
		},
	)
	if err != nil {
		t.Fatalf("failed to construct sync controller: %v", err)
	}

	// Add a successful PR to github
	initialPR := PullRequest{}
	initialPR.Commits.Nodes = append(initialPR.Commits.Nodes, struct{ Commit Commit }{
		Commit: Commit{Status: CommitStatus{Contexts: []Context{
			{
				Context: githubql.String("mandatory-job"),
				State:   githubql.StatusStateSuccess,
			},
			{
				Context: githubql.String(statusContext),
				State:   githubql.StatusStatePending,
			},
		}}},
	})
	ghc.prs = map[string][]PullRequest{"": {initialPR}}

	// sync, this creates a new serial retest prowjob
	if err := c.Sync(); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	// ensure there is actually the retest job
	var pjs prowapi.ProwJobList
	if err := c.prowJobClient.List(c.ctx, &pjs); err != nil {
		t.Fatalf("failed to list prowjobs: %v", err)
	}
	if n := len(pjs.Items); n != 1 {
		t.Errorf("expected to find exactly one prowjob, got %d from list %+v", n, pjs)
	}

	// Update the context on the PR to pending just like crier would
	for idx, ctx := range initialPR.Commits.Nodes[0].Commit.Status.Contexts {
		if pjs.Items[0].Spec.Context == string(ctx.Context) {
			initialPR.Commits.Nodes[0].Commit.Status.Contexts[idx].State = githubql.StatusStatePending
		}
	}

	// Update the sha of the pool
	ghc.refs = map[string]string{"/ ": "new-base-sha"}

	// sync, this creates another serial retest prowjob
	if err := c.Sync(); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// ensure we have the two retest prowjobs
	if err := c.prowJobClient.List(c.ctx, &pjs); err != nil {
		t.Fatalf("failed to list prowjobs: %v", err)
	}
	if n := len(pjs.Items); n != 2 {
		t.Errorf("expected to find exactly two prowjobs, got %d from list %+v", n, pjs)
	}

}
