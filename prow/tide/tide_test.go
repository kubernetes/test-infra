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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shurcooL/githubql"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
)

func testPullsMatchList(t *testing.T, test string, actual []PullRequest, expected []int) {
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
	type pull struct {
		number int
		sha    string
	}
	type prowjob struct {
		prs   []pull
		job   string
		state kube.ProwJobState
	}
	tests := []struct {
		name       string
		presubmits []string
		pulls      []pull
		prowJobs   []prowjob

		merges  []int
		pending bool
	}{
		{
			name: "no batches running",
		},
		{
			name:       "batch pending",
			presubmits: []string{"foo", "bar", "baz"},
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs:   []prowjob{{job: "foo", state: kube.PendingState, prs: []pull{{1, "a"}}}},
			pending:    true,
		},
		{
			name:       "batch pending, successful previous run",
			presubmits: []string{"foo", "bar", "baz"},
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs: []prowjob{
				{job: "foo", state: kube.PendingState, prs: []pull{{1, "a"}}},
				{job: "foo", state: kube.SuccessState, prs: []pull{{2, "b"}}},
				{job: "bar", state: kube.SuccessState, prs: []pull{{2, "b"}}},
				{job: "baz", state: kube.SuccessState, prs: []pull{{2, "b"}}},
			},
			pending: true,
		},
		{
			name:       "successful run",
			presubmits: []string{"foo", "bar", "baz"},
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs: []prowjob{
				{job: "foo", state: kube.SuccessState, prs: []pull{{2, "b"}}},
				{job: "bar", state: kube.SuccessState, prs: []pull{{2, "b"}}},
				{job: "baz", state: kube.SuccessState, prs: []pull{{2, "b"}}},
			},
			merges: []int{2},
		},
		{
			name:       "successful run, multiple PRs",
			presubmits: []string{"foo", "bar", "baz"},
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs: []prowjob{
				{job: "foo", state: kube.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "bar", state: kube.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "baz", state: kube.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
			},
			merges: []int{1, 2},
		},
		{
			name:       "successful run, failures in past",
			presubmits: []string{"foo", "bar", "baz"},
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs: []prowjob{
				{job: "foo", state: kube.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "bar", state: kube.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "baz", state: kube.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "foo", state: kube.FailureState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "baz", state: kube.FailureState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "foo", state: kube.FailureState, prs: []pull{{1, "c"}, {2, "b"}}},
			},
			merges: []int{1, 2},
		},
		{
			name:       "failures",
			presubmits: []string{"foo", "bar", "baz"},
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs: []prowjob{
				{job: "foo", state: kube.FailureState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "bar", state: kube.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "baz", state: kube.FailureState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "foo", state: kube.FailureState, prs: []pull{{1, "c"}, {2, "b"}}},
			},
		},
	}
	for _, test := range tests {
		var pulls []PullRequest
		for _, p := range test.pulls {
			pr := PullRequest{Number: githubql.Int(p.number)}
			pr.Commits.Nodes = []struct {
				Commit Commit
			}{{Commit: Commit{OID: githubql.String(p.sha)}}}
			pulls = append(pulls, pr)
		}
		var pjs []kube.ProwJob
		for _, pj := range test.prowJobs {
			npj := kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Job:  pj.job,
					Type: kube.BatchJob,
				},
				Status: kube.ProwJobStatus{State: pj.state},
			}
			for _, pr := range pj.prs {
				npj.Spec.Refs.Pulls = append(npj.Spec.Refs.Pulls, kube.Pull{
					Number: pr.number,
					SHA:    pr.sha,
				})
			}
			pjs = append(pjs, npj)
		}
		merges, pending := accumulateBatch(test.presubmits, pulls, pjs)
		if (len(pending) > 0) != test.pending {
			t.Errorf("For case \"%s\", got wrong pending.", test.name)
		}
		testPullsMatchList(t, test.name, merges, test.merges)
	}
}

func TestAccumulate(t *testing.T) {
	type prowjob struct {
		prNumber int
		job      string
		state    kube.ProwJobState
	}
	tests := []struct {
		presubmits   []string
		pullRequests []int
		prowJobs     []prowjob

		successes []int
		pendings  []int
		none      []int
	}{
		{
			presubmits:   []string{"job1", "job2"},
			pullRequests: []int{1, 2, 3, 4, 5, 6, 7},
			prowJobs: []prowjob{
				{2, "job1", kube.PendingState},
				{3, "job1", kube.PendingState},
				{3, "job2", kube.TriggeredState},
				{4, "job1", kube.FailureState},
				{4, "job2", kube.PendingState},
				{5, "job1", kube.PendingState},
				{5, "job2", kube.FailureState},
				{5, "job2", kube.PendingState},
				{6, "job1", kube.SuccessState},
				{6, "job2", kube.PendingState},
				{7, "job1", kube.SuccessState},
				{7, "job2", kube.SuccessState},
				{7, "job1", kube.FailureState},
			},

			successes: []int{7},
			pendings:  []int{3, 5, 6},
			none:      []int{1, 2, 4},
		},
		{
			presubmits:   []string{"job1", "job2", "job3", "job4"},
			pullRequests: []int{7},
			prowJobs: []prowjob{
				{7, "job1", kube.SuccessState},
				{7, "job2", kube.FailureState},
				{7, "job3", kube.FailureState},
				{7, "job4", kube.FailureState},
				{7, "job3", kube.FailureState},
				{7, "job4", kube.FailureState},
				{7, "job2", kube.SuccessState},
				{7, "job3", kube.SuccessState},
				{7, "job4", kube.FailureState},
			},

			successes: []int{},
			pendings:  []int{},
			none:      []int{7},
		},
		{
			presubmits:   []string{"job1", "job2", "job3", "job4"},
			pullRequests: []int{7},
			prowJobs: []prowjob{
				{7, "job1", kube.FailureState},
				{7, "job2", kube.FailureState},
				{7, "job3", kube.FailureState},
				{7, "job4", kube.FailureState},
				{7, "job3", kube.FailureState},
				{7, "job4", kube.FailureState},
				{7, "job2", kube.FailureState},
				{7, "job3", kube.FailureState},
				{7, "job4", kube.FailureState},
			},

			successes: []int{},
			pendings:  []int{},
			none:      []int{7},
		},
		{
			presubmits:   []string{"job1", "job2", "job3", "job4"},
			pullRequests: []int{7},
			prowJobs: []prowjob{
				{7, "job1", kube.SuccessState},
				{7, "job2", kube.FailureState},
				{7, "job3", kube.FailureState},
				{7, "job4", kube.FailureState},
				{7, "job3", kube.FailureState},
				{7, "job4", kube.FailureState},
				{7, "job2", kube.SuccessState},
				{7, "job3", kube.SuccessState},
				{7, "job4", kube.SuccessState},
				{7, "job1", kube.FailureState},
			},

			successes: []int{7},
			pendings:  []int{},
			none:      []int{},
		},
		{
			presubmits:   []string{"job1", "job2", "job3", "job4"},
			pullRequests: []int{7},
			prowJobs: []prowjob{
				{7, "job1", kube.SuccessState},
				{7, "job2", kube.FailureState},
				{7, "job3", kube.FailureState},
				{7, "job4", kube.FailureState},
				{7, "job3", kube.FailureState},
				{7, "job4", kube.FailureState},
				{7, "job2", kube.SuccessState},
				{7, "job3", kube.SuccessState},
				{7, "job4", kube.PendingState},
				{7, "job1", kube.FailureState},
			},

			successes: []int{},
			pendings:  []int{7},
			none:      []int{},
		},
	}

	for i, test := range tests {
		var pulls []PullRequest
		for _, p := range test.pullRequests {
			pulls = append(pulls, PullRequest{Number: githubql.Int(p)})
		}
		var pjs []kube.ProwJob
		for _, pj := range test.prowJobs {
			pjs = append(pjs, kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Job:  pj.job,
					Type: kube.PresubmitJob,
					Refs: kube.Refs{Pulls: []kube.Pull{{Number: pj.prNumber}}},
				},
				Status: kube.ProwJobStatus{State: pj.state},
			})
		}

		successes, pendings, nones := accumulate(test.presubmits, pulls, pjs)

		t.Logf("test run %d", i)
		testPullsMatchList(t, "successes", successes, test.successes)
		testPullsMatchList(t, "pendings", pendings, test.pendings)
		testPullsMatchList(t, "nones", nones, test.none)
	}
}

type fgc struct {
	refs      map[string]string
	merged    int
	setStatus bool
}

func (f *fgc) GetRef(o, r, ref string) (string, error) {
	return f.refs[o+"/"+r+" "+ref], nil
}

func (f *fgc) Query(ctx context.Context, q interface{}, vars map[string]interface{}) error {
	return nil
}

func (f *fgc) Merge(org, repo string, number int, details github.MergeDetails) error {
	if details.SHA == "uh oh" {
		return errors.New("invalid sha")
	}
	f.merged++
	return nil
}

func (f *fgc) CreateStatus(org, repo, ref string, s github.Status) error {
	switch s.State {
	case github.StatusSuccess, github.StatusError, github.StatusPending, github.StatusFailure:
		f.setStatus = true
		return nil
	}
	return fmt.Errorf("invalid 'state' value: %q", s.State)
}

func TestSetStatuses(t *testing.T) {
	testcases := []struct {
		name string

		inPool     bool
		hasContext bool
		state      githubql.StatusState
		desc       string

		shouldSet bool
	}{
		{
			name: "in pool with proper context",

			inPool:     true,
			hasContext: true,
			state:      githubql.StatusStateSuccess,
			desc:       statusInPool,

			shouldSet: false,
		},
		{
			name: "in pool without context",

			inPool:     true,
			hasContext: false,

			shouldSet: true,
		},
		{
			name: "in pool with improper context",

			inPool:     true,
			hasContext: true,
			state:      githubql.StatusStateSuccess,
			desc:       statusNotInPool,

			shouldSet: true,
		},
		{
			name: "in pool with wrong state",

			inPool:     true,
			hasContext: true,
			state:      githubql.StatusStatePending,
			desc:       statusInPool,

			shouldSet: true,
		},
		{
			name: "not in pool with proper context",

			inPool:     false,
			hasContext: true,
			state:      githubql.StatusStatePending,
			desc:       statusNotInPool,

			shouldSet: false,
		},
		{
			name: "not in pool with improper context",

			inPool:     false,
			hasContext: true,
			state:      githubql.StatusStatePending,
			desc:       statusInPool,

			shouldSet: true,
		},
		{
			name: "not in pool with no context",

			inPool:     false,
			hasContext: false,

			shouldSet: true,
		},
	}
	for _, tc := range testcases {
		var pr PullRequest
		pr.Commits.Nodes = []struct{ Commit Commit }{{}}
		if tc.hasContext {
			pr.Commits.Nodes[0].Commit.Status.Contexts = []Context{
				{
					Context:     githubql.String(statusContext),
					State:       tc.state,
					Description: githubql.String(tc.desc),
				},
			}
		}
		var pool []PullRequest
		if tc.inPool {
			pool = []PullRequest{pr}
		}
		fc := &fgc{}
		ca := &config.Agent{}
		ca.Set(&config.Config{})
		// setStatuses logs instead of returning errors.
		// Construct a logger to watch for errors to be printed.
		log := logrus.WithField("component", "tide")
		initialLog, err := log.String()
		if err != nil {
			t.Fatalf("Failed to get log output before testing: %v", err)
		}

		c := &Controller{ghc: fc, ca: ca, logger: log}
		c.setStatuses([]PullRequest{pr}, pool)
		if str, err := log.String(); err != nil {
			t.Fatalf("For case %s: failed to get log output: %v", tc.name, err)
		} else if str != initialLog {
			t.Errorf("For case %s: error setting status: %s", tc.name, str)
		}
		if tc.shouldSet && !fc.setStatus {
			t.Errorf("For case %s: should set but didn't", tc.name)
		} else if !tc.shouldSet && fc.setStatus {
			t.Errorf("For case %s: should not set but did", tc.name)
		}
	}
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
			branch: "master",
		},
		{
			org:    "k",
			repo:   "t-i",
			number: 6,
			branch: "master",
		},
		{
			org:    "k",
			repo:   "k",
			number: 123,
			branch: "master",
		},
		{
			org:    "k",
			repo:   "k",
			number: 1000,
			branch: "release-1.6",
		},
	}
	testPJs := []struct {
		jobType kube.ProwJobType
		org     string
		repo    string
		baseRef string
		baseSHA string
	}{
		{
			jobType: kube.PresubmitJob,
			org:     "k",
			repo:    "t-i",
			baseRef: "master",
			baseSHA: "123",
		},
		{
			jobType: kube.BatchJob,
			org:     "k",
			repo:    "t-i",
			baseRef: "master",
			baseSHA: "123",
		},
		{
			jobType: kube.PeriodicJob,
		},
		{
			jobType: kube.PresubmitJob,
			org:     "k",
			repo:    "t-i",
			baseRef: "patch",
			baseSHA: "123",
		},
		{
			jobType: kube.PresubmitJob,
			org:     "k",
			repo:    "t-i",
			baseRef: "master",
			baseSHA: "abc",
		},
		{
			jobType: kube.PresubmitJob,
			org:     "o",
			repo:    "t-i",
			baseRef: "master",
			baseSHA: "123",
		},
		{
			jobType: kube.PresubmitJob,
			org:     "k",
			repo:    "other",
			baseRef: "master",
			baseSHA: "123",
		},
	}
	fc := &fgc{
		refs: map[string]string{"k/t-i heads/master": "123"},
	}
	c := &Controller{
		ghc: fc,
	}
	var pulls []PullRequest
	for _, p := range testPulls {
		npr := PullRequest{Number: githubql.Int(p.number)}
		npr.BaseRef.Name = githubql.String(p.branch)
		npr.BaseRef.Prefix = "refs/heads/"
		npr.Repository.Name = githubql.String(p.repo)
		npr.Repository.Owner.Login = githubql.String(p.org)
		pulls = append(pulls, npr)
	}
	var pjs []kube.ProwJob
	for _, pj := range testPJs {
		pjs = append(pjs, kube.ProwJob{
			Spec: kube.ProwJobSpec{
				Type: pj.jobType,
				Refs: kube.Refs{
					Org:     pj.org,
					Repo:    pj.repo,
					BaseRef: pj.baseRef,
					BaseSHA: pj.baseSHA,
				},
			},
		})
	}
	sps, err := c.dividePool(pulls, pjs)
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
			t.Errorf("For subpool %s, got sha %s, expected %s.", name, sp.sha, sha)
		}
		if len(sp.prs) == 0 {
			t.Errorf("Subpool %s has no PRs.", name)
		}
		for _, pr := range sp.prs {
			if string(pr.Repository.Owner.Login) != sp.org || string(pr.Repository.Name) != sp.repo || string(pr.BaseRef.Name) != sp.branch {
				t.Errorf("PR in wrong subpool. Got PR %+v in subpool %s.", pr, name)
			}
		}
		for _, pj := range sp.pjs {
			if pj.Spec.Type != kube.PresubmitJob && pj.Spec.Type != kube.BatchJob {
				t.Errorf("PJ with bad type in subpool %s: %+v", name, pj)
			}
			if pj.Spec.Refs.Org != sp.org || pj.Spec.Refs.Repo != sp.repo || pj.Spec.Refs.BaseRef != sp.branch || pj.Spec.Refs.BaseSHA != sp.sha {
				t.Errorf("PJ in wrong subpool. Got PJ %+v in subpool %s.", pj, name)
			}
		}
	}
}

func TestPickBatch(t *testing.T) {
	lg, gc, err := localgit.New()
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

		included bool
	}{
		{
			files:    map[string][]byte{"bar": []byte("ok")},
			success:  true,
			included: true,
		},
		{
			files:    map[string][]byte{"foo": []byte("ok")},
			success:  true,
			included: true,
		},
		{
			files:    map[string][]byte{"bar": []byte("conflicts with 0")},
			success:  true,
			included: false,
		},
		{
			files:    map[string][]byte{"qux": []byte("ok")},
			success:  false,
			included: false,
		},
		{
			files:    map[string][]byte{"bazel": []byte("ok")},
			success:  true,
			included: true,
		},
	}
	sp := subpool{
		org:    "o",
		repo:   "r",
		branch: "master",
		sha:    "master",
	}
	for i, testpr := range testprs {
		if err := lg.CheckoutNewBranch("o", "r", fmt.Sprintf("pr-%d", i)); err != nil {
			t.Fatalf("Error checking out new branch: %v", err)
		}
		if err := lg.AddCommit("o", "r", testpr.files); err != nil {
			t.Fatalf("Error adding commit: %v", err)
		}
		if err := lg.Checkout("o", "r", "master"); err != nil {
			t.Fatalf("Error checking out master: %v", err)
		}
		var pr PullRequest
		pr.Number = githubql.Int(i)
		pr.Commits.Nodes = []struct {
			Commit Commit
		}{{Commit: Commit{OID: githubql.String(fmt.Sprintf("origin/pr-%d", i))}}}
		pr.Commits.Nodes[0].Commit.Status.Contexts = append(pr.Commits.Nodes[0].Commit.Status.Contexts, Context{State: githubql.StatusStateSuccess})
		if !testpr.success {
			pr.Commits.Nodes[0].Commit.Status.Contexts[0].State = githubql.StatusStateFailure
		}
		sp.prs = append(sp.prs, pr)
	}
	c := &Controller{
		gc: gc,
	}
	prs, err := c.pickBatch(sp)
	if err != nil {
		t.Fatalf("Error from pickBatch: %v", err)
	}
	for i, testpr := range testprs {
		var found bool
		for _, pr := range prs {
			if int(pr.Number) == i {
				found = true
				break
			}
		}
		if found && !testpr.included {
			t.Errorf("PR %d should not be picked.", i)
		} else if !found && testpr.included {
			t.Errorf("PR %d should be picked.", i)
		}
	}
}

type fkc struct {
	createdJobs []kube.ProwJob
}

func (c *fkc) ListProwJobs(string) ([]kube.ProwJob, error) {
	return nil, nil
}

func (c *fkc) CreateProwJob(pj kube.ProwJob) (kube.ProwJob, error) {
	c.createdJobs = append(c.createdJobs, pj)
	return pj, nil
}

func TestTakeAction(t *testing.T) {
	// PRs 0-9 exist. All are mergable, and all are passing tests.
	testcases := []struct {
		name string

		batchPending bool
		successes    []int
		pendings     []int
		nones        []int
		batchMerges  []int

		merged            int
		triggered         int
		triggered_batches int
		action            Action
	}{
		{
			name: "no prs to test, should do nothing",

			batchPending: true,
			successes:    []int{},
			pendings:     []int{},
			nones:        []int{},
			batchMerges:  []int{},

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

			merged:            0,
			triggered:         1,
			triggered_batches: 1,
			action:            TriggerBatch,
		},
		{
			name: "one PR, should not trigger batch",

			batchPending: false,
			successes:    []int{},
			pendings:     []int{},
			nones:        []int{0},
			batchMerges:  []int{},

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

			merged:    3,
			triggered: 0,
			action:    MergeBatch,
		},
	}

	for _, tc := range testcases {
		ca := &config.Agent{}
		ca.Set(&config.Config{
			Presubmits: map[string][]config.Presubmit{
				"o/r": {
					{
						Name:      "foo",
						AlwaysRun: true,
					},
				},
			},
		})
		lg, gc, err := localgit.New()
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
			org:    "o",
			repo:   "r",
			branch: "master",
			sha:    "master",
		}
		genPulls := func(nums []int) []PullRequest {
			var prs []PullRequest
			for _, i := range nums {
				if err := lg.CheckoutNewBranch("o", "r", fmt.Sprintf("pr-%d", i)); err != nil {
					t.Fatalf("Error checking out new branch: %v", err)
				}
				if err := lg.AddCommit("o", "r", map[string][]byte{fmt.Sprintf("%d", i): []byte("WOW")}); err != nil {
					t.Fatalf("Error adding commit: %v", err)
				}
				if err := lg.Checkout("o", "r", "master"); err != nil {
					t.Fatalf("Error checking out master: %v", err)
				}
				var pr PullRequest
				pr.Number = githubql.Int(i)
				pr.HeadRef.Target.OID = githubql.String(fmt.Sprintf("origin/pr-%d", i))
				pr.Commits.Nodes = []struct {
					Commit Commit
				}{{Commit: Commit{OID: githubql.String("uh oh")}}}
				sp.prs = append(sp.prs, pr)
				prs = append(prs, pr)
			}
			return prs
		}
		var fkc fkc
		var fgc fgc
		c := &Controller{
			logger: logrus.WithField("controller", "tide"),
			gc:     gc,
			ghc:    &fgc,
			ca:     ca,
			kc:     &fkc,
		}
		var batchPending []PullRequest
		if tc.batchPending {
			batchPending = []PullRequest{{}}
		}
		t.Logf("Test case: %s", tc.name)
		if act, _, err := c.takeAction(sp, batchPending, genPulls(tc.successes), genPulls(tc.pendings), genPulls(tc.nones), genPulls(tc.batchMerges)); err != nil {
			t.Errorf("Error in takeAction: %v", err)
			continue
		} else if act != tc.action {
			t.Errorf("Wrong action. Got %v, wanted %v.", act, tc.action)
		}
		if tc.triggered != len(fkc.createdJobs) {
			t.Errorf("Wrong number of jobs triggered. Got %d, expected %d.", len(fkc.createdJobs), tc.triggered)
		}
		if tc.merged != fgc.merged {
			t.Errorf("Wrong number of merges. Got %d, expected %d.", fgc.merged, tc.merged)
		}
		// Ensure that the correct number of batch jobs were triggered
		batches := 0
		for _, job := range fkc.createdJobs {
			if (len(job.Spec.Refs.Pulls) > 1) != (job.Spec.Type == kube.BatchJob) {
				t.Error("Found a batch job that doesn't contain multiple pull refs!")
			}
			if len(job.Spec.Refs.Pulls) > 1 {
				batches++
			}
		}
		if tc.triggered_batches != batches {
			t.Errorf("Wrong number of batches triggered. Got %d, expected %d.", batches, tc.triggered_batches)
		}
	}
}

func TestServeHTTP(t *testing.T) {
	c := &Controller{
		pools: []Pool{
			{
				Action: Merge,
			},
		},
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
		t.Errorf("JSON decoding error: %v", err)
	}
	if len(pools) != 1 {
		t.Errorf("Wrong number of pools. Got %d, want 1.", len(pools))
	}
	if pools[0].Action != Merge {
		t.Errorf("Wrong action. Got %v, want %v.", pools[0].Action, Merge)
	}
}
