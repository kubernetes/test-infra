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
	"reflect"
	"testing"

	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
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
	jobSet := sets.NewString("foo", "bar", "baz")
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
		presubmits map[int]sets.String
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
			presubmits: map[int]sets.String{1: sets.NewString("foo"), 2: sets.NewString("foo")},
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs:   []prowjob{{job: "foo", state: kube.PendingState, prs: []pull{{1, "a"}}}},
			pending:    true,
		},
		{
			name:       "pending batch missing presubmits is ignored",
			presubmits: map[int]sets.String{1: jobSet},
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs:   []prowjob{{job: "foo", state: kube.PendingState, prs: []pull{{1, "a"}}}},
		},
		{
			name:       "batch pending, successful previous run",
			presubmits: map[int]sets.String{1: jobSet, 2: jobSet},
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs: []prowjob{
				{job: "foo", state: kube.PendingState, prs: []pull{{1, "a"}}},
				{job: "bar", state: kube.SuccessState, prs: []pull{{1, "a"}}},
				{job: "baz", state: kube.SuccessState, prs: []pull{{1, "a"}}},
				{job: "foo", state: kube.SuccessState, prs: []pull{{2, "b"}}},
				{job: "bar", state: kube.SuccessState, prs: []pull{{2, "b"}}},
				{job: "baz", state: kube.SuccessState, prs: []pull{{2, "b"}}},
			},
			pending: true,
			merges:  []int{2},
		},
		{
			name:       "successful run",
			presubmits: map[int]sets.String{1: jobSet, 2: jobSet},
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
			presubmits: map[int]sets.String{1: jobSet, 2: jobSet},
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
			presubmits: map[int]sets.String{1: jobSet, 2: jobSet},
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
			presubmits: map[int]sets.String{1: jobSet, 2: jobSet},
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs: []prowjob{
				{job: "foo", state: kube.FailureState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "bar", state: kube.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "baz", state: kube.FailureState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "foo", state: kube.FailureState, prs: []pull{{1, "c"}, {2, "b"}}},
			},
		},
		{
			name:       "missing job required by one PR",
			presubmits: map[int]sets.String{1: jobSet, 2: jobSet.Union(sets.NewString("boo"))},
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs: []prowjob{
				{job: "foo", state: kube.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "bar", state: kube.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "baz", state: kube.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
			},
		},
		{
			name:       "successful run with PR that requires additional job",
			presubmits: map[int]sets.String{1: jobSet, 2: jobSet.Union(sets.NewString("boo"))},
			pulls:      []pull{{1, "a"}, {2, "b"}},
			prowJobs: []prowjob{
				{job: "foo", state: kube.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "bar", state: kube.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "baz", state: kube.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
				{job: "boo", state: kube.SuccessState, prs: []pull{{1, "a"}, {2, "b"}}},
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
			presubmits: map[int]sets.String{2: jobSet},
			pulls:      []pull{{2, "b"}},
			prowJobs: []prowjob{
				{job: "foo", state: kube.PendingState, prs: []pull{{1, "a"}}},
				{job: "foo", state: kube.SuccessState, prs: []pull{{2, "b"}}},
				{job: "bar", state: kube.SuccessState, prs: []pull{{2, "b"}}},
				{job: "baz", state: kube.SuccessState, prs: []pull{{2, "b"}}},
			},
			pending: false,
			merges:  []int{2},
		},
	}
	for _, test := range tests {
		var pulls []PullRequest
		for _, p := range test.pulls {
			pr := PullRequest{
				Number:     githubql.Int(p.number),
				HeadRefOID: githubql.String(p.sha),
			}
			pulls = append(pulls, pr)
		}
		var pjs []kube.ProwJob
		for _, pj := range test.prowJobs {
			npj := kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Job:     pj.job,
					Context: pj.job,
					Type:    kube.BatchJob,
					Refs:    new(kube.Refs),
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
		merges, pending := accumulateBatch(test.presubmits, pulls, pjs, logrus.NewEntry(logrus.New()))
		if (len(pending) > 0) != test.pending {
			t.Errorf("For case \"%s\", got wrong pending.", test.name)
		}
		testPullsMatchList(t, test.name, merges, test.merges)
	}
}

func TestAccumulate(t *testing.T) {
	jobSet := sets.NewString("job1", "job2")
	type prowjob struct {
		prNumber int
		job      string
		state    kube.ProwJobState
		sha      string
	}
	tests := []struct {
		presubmits   map[int]sets.String
		pullRequests map[int]string
		prowJobs     []prowjob

		successes []int
		pendings  []int
		none      []int
	}{
		{
			pullRequests: map[int]string{1: "", 2: "", 3: "", 4: "", 5: "", 6: "", 7: ""},
			presubmits: map[int]sets.String{
				1: jobSet,
				2: jobSet,
				3: jobSet,
				4: jobSet,
				5: jobSet,
				6: jobSet,
				7: jobSet,
			},
			prowJobs: []prowjob{
				{2, "job1", kube.PendingState, ""},
				{3, "job1", kube.PendingState, ""},
				{3, "job2", kube.TriggeredState, ""},
				{4, "job1", kube.FailureState, ""},
				{4, "job2", kube.PendingState, ""},
				{5, "job1", kube.PendingState, ""},
				{5, "job2", kube.FailureState, ""},
				{5, "job2", kube.PendingState, ""},
				{6, "job1", kube.SuccessState, ""},
				{6, "job2", kube.PendingState, ""},
				{7, "job1", kube.SuccessState, ""},
				{7, "job2", kube.SuccessState, ""},
				{7, "job1", kube.FailureState, ""},
			},

			successes: []int{7},
			pendings:  []int{3, 5, 6},
			none:      []int{1, 2, 4},
		},
		{
			pullRequests: map[int]string{7: ""},
			presubmits:   map[int]sets.String{7: sets.NewString("job1", "job2", "job3", "job4")},
			prowJobs: []prowjob{
				{7, "job1", kube.SuccessState, ""},
				{7, "job2", kube.FailureState, ""},
				{7, "job3", kube.FailureState, ""},
				{7, "job4", kube.FailureState, ""},
				{7, "job3", kube.FailureState, ""},
				{7, "job4", kube.FailureState, ""},
				{7, "job2", kube.SuccessState, ""},
				{7, "job3", kube.SuccessState, ""},
				{7, "job4", kube.FailureState, ""},
			},

			successes: []int{},
			pendings:  []int{},
			none:      []int{7},
		},
		{
			pullRequests: map[int]string{7: ""},
			presubmits:   map[int]sets.String{7: sets.NewString("job1", "job2", "job3", "job4")},
			prowJobs: []prowjob{
				{7, "job1", kube.FailureState, ""},
				{7, "job2", kube.FailureState, ""},
				{7, "job3", kube.FailureState, ""},
				{7, "job4", kube.FailureState, ""},
				{7, "job3", kube.FailureState, ""},
				{7, "job4", kube.FailureState, ""},
				{7, "job2", kube.FailureState, ""},
				{7, "job3", kube.FailureState, ""},
				{7, "job4", kube.FailureState, ""},
			},

			successes: []int{},
			pendings:  []int{},
			none:      []int{7},
		},
		{
			pullRequests: map[int]string{7: ""},
			presubmits:   map[int]sets.String{7: sets.NewString("job1", "job2", "job3", "job4")},
			prowJobs: []prowjob{
				{7, "job1", kube.SuccessState, ""},
				{7, "job2", kube.FailureState, ""},
				{7, "job3", kube.FailureState, ""},
				{7, "job4", kube.FailureState, ""},
				{7, "job3", kube.FailureState, ""},
				{7, "job4", kube.FailureState, ""},
				{7, "job2", kube.SuccessState, ""},
				{7, "job3", kube.SuccessState, ""},
				{7, "job4", kube.SuccessState, ""},
				{7, "job1", kube.FailureState, ""},
			},

			successes: []int{7},
			pendings:  []int{},
			none:      []int{},
		},
		{
			pullRequests: map[int]string{7: ""},
			presubmits:   map[int]sets.String{7: sets.NewString("job1", "job2", "job3", "job4")},
			prowJobs: []prowjob{
				{7, "job1", kube.SuccessState, ""},
				{7, "job2", kube.FailureState, ""},
				{7, "job3", kube.FailureState, ""},
				{7, "job4", kube.FailureState, ""},
				{7, "job3", kube.FailureState, ""},
				{7, "job4", kube.FailureState, ""},
				{7, "job2", kube.SuccessState, ""},
				{7, "job3", kube.SuccessState, ""},
				{7, "job4", kube.PendingState, ""},
				{7, "job1", kube.FailureState, ""},
			},

			successes: []int{},
			pendings:  []int{7},
			none:      []int{},
		},
		{
			presubmits:   map[int]sets.String{7: sets.NewString("job1")},
			pullRequests: map[int]string{7: "new", 8: "new"},
			prowJobs: []prowjob{
				{7, "job1", kube.SuccessState, "old"},
				{7, "job1", kube.FailureState, "new"},
				{8, "job1", kube.FailureState, "old"},
				{8, "job1", kube.SuccessState, "new"},
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
	}

	for i, test := range tests {
		var pulls []PullRequest
		for num, sha := range test.pullRequests {
			pulls = append(
				pulls,
				PullRequest{Number: githubql.Int(num), HeadRefOID: githubql.String(sha)},
			)
		}
		var pjs []kube.ProwJob
		for _, pj := range test.prowJobs {
			pjs = append(pjs, kube.ProwJob{
				Spec: kube.ProwJobSpec{
					Job:     pj.job,
					Context: pj.job,
					Type:    kube.PresubmitJob,
					Refs:    &kube.Refs{Pulls: []kube.Pull{{Number: pj.prNumber, SHA: pj.sha}}},
				},
				Status: kube.ProwJobStatus{State: pj.state},
			})
		}

		successes, pendings, nones := accumulate(test.presubmits, pulls, pjs, logrus.NewEntry(logrus.New()))

		t.Logf("test run %d", i)
		testPullsMatchList(t, "successes", successes, test.successes)
		testPullsMatchList(t, "pendings", pendings, test.pendings)
		testPullsMatchList(t, "nones", nones, test.none)
	}
}

type fgc struct {
	prs       []PullRequest
	refs      map[string]string
	merged    int
	setStatus bool

	expectedSHA    string
	combinedStatus map[string]string
}

func (f *fgc) GetRef(o, r, ref string) (string, error) {
	return f.refs[o+"/"+r+" "+ref], nil
}

func (f *fgc) Query(ctx context.Context, q interface{}, vars map[string]interface{}) error {
	sq, ok := q.(*searchQuery)
	if !ok {
		return errors.New("unexpected query type")
	}
	for _, pr := range f.prs {
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

func (f *fgc) GetCombinedStatus(org, repo, ref string) (*github.CombinedStatus, error) {
	if f.expectedSHA != ref {
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
		ghc:    fc,
		logger: logrus.WithField("component", "tide"),
	}
	pulls := make(map[string]PullRequest)
	for _, p := range testPulls {
		npr := PullRequest{Number: githubql.Int(p.number)}
		npr.BaseRef.Name = githubql.String(p.branch)
		npr.BaseRef.Prefix = "refs/heads/"
		npr.Repository.Name = githubql.String(p.repo)
		npr.Repository.Owner.Login = githubql.String(p.org)
		pulls[prKey(&npr)] = npr
	}
	var pjs []kube.ProwJob
	for _, pj := range testPJs {
		pjs = append(pjs, kube.ProwJob{
			Spec: kube.ProwJobSpec{
				Type: pj.jobType,
				Refs: &kube.Refs{
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
			files:    map[string][]byte{"qux": []byte("ok")},
			success:  false,
			number:   6,
			included: false,
		},
		{
			files:    map[string][]byte{"bazel": []byte("ok")},
			success:  true,
			number:   7,
			included: false, // batch of 5 smallest excludes this
		},
		{
			files:    map[string][]byte{"other": []byte("ok")},
			success:  true,
			number:   5,
			included: true,
		},
		{
			files:    map[string][]byte{"changes": []byte("ok")},
			success:  true,
			number:   4,
			included: true,
		},
		{
			files:    map[string][]byte{"something": []byte("ok")},
			success:  true,
			number:   3,
			included: true,
		},
	}
	sp := subpool{
		log:    logrus.WithField("component", "tide"),
		org:    "o",
		repo:   "r",
		branch: "master",
		sha:    "master",
	}
	for _, testpr := range testprs {
		if err := lg.CheckoutNewBranch("o", "r", fmt.Sprintf("pr-%d", testpr.number)); err != nil {
			t.Fatalf("Error checking out new branch: %v", err)
		}
		if err := lg.AddCommit("o", "r", testpr.files); err != nil {
			t.Fatalf("Error adding commit: %v", err)
		}
		if err := lg.Checkout("o", "r", "master"); err != nil {
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
		sp.prs = append(sp.prs, pr)
	}
	ca := &config.Agent{}
	ca.Set(&config.Config{})
	c := &Controller{
		logger: logrus.WithField("component", "tide"),
		gc:     gc,
		ca:     ca,
	}
	prs, err := c.pickBatch(sp, &config.TideContextPolicy{})
	if err != nil {
		t.Fatalf("Error from pickBatch: %v", err)
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
		presubmits   map[int]sets.String

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
			presubmits:   map[int]sets.String{100: sets.NewString("foo", "if-changed")},

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
			presubmits:   map[int]sets.String{100: sets.NewString("foo", "if-changed")},

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
			presubmits:   map[int]sets.String{100: sets.NewString("foo", "if-changed")},

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
			presubmits:   map[int]sets.String{100: sets.NewString("foo", "if-changed")},

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
			presubmits:   map[int]sets.String{100: sets.NewString("foo", "if-changed")},

			merged:           0,
			triggered:        1,
			triggeredBatches: 1,
			action:           TriggerBatch,
		},
		{
			name: "one PR, should not trigger batch",

			batchPending: false,
			successes:    []int{},
			pendings:     []int{},
			nones:        []int{0},
			batchMerges:  []int{},
			presubmits:   map[int]sets.String{100: sets.NewString("foo", "if-changed")},

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
			presubmits:   map[int]sets.String{100: sets.NewString("foo", "if-changed")},

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
			presubmits:   map[int]sets.String{100: sets.NewString("foo", "if-changed")},

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
			presubmits:   map[int]sets.String{100: sets.NewString("foo", "if-changed")},

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
	}

	for _, tc := range testcases {
		ca := &config.Agent{}
		cfg := &config.Config{}
		if err := cfg.SetPresubmits(
			map[string][]config.Presubmit{
				"o/r": {
					{
						Context:      "foo",
						Trigger:      "/test all",
						RerunCommand: "/test all",
						AlwaysRun:    true,
					},
					{
						Context:      "if-changed",
						Trigger:      "/test if-changed",
						RerunCommand: "/test if-changed",
						RegexpChangeMatcher: config.RegexpChangeMatcher{
							RunIfChanged: "CHANGED",
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
				tc.presubmits[i] = sets.NewString("foo")
			}
		}
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
			log:               logrus.WithField("component", "tide"),
			presubmitContexts: tc.presubmits,
			cc:                &config.TideContextPolicy{},
			org:               "o",
			repo:              "r",
			branch:            "master",
			sha:               "master",
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
				oid := githubql.String(fmt.Sprintf("origin/pr-%d", i))
				var pr PullRequest
				pr.Number = githubql.Int(i)
				pr.HeadRefOID = oid
				pr.Commits.Nodes = []struct {
					Commit Commit
				}{{Commit: Commit{OID: oid}}}
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
			ca:     ca,
			ghc:    &fgc,
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
		if tc.triggeredBatches != batches {
			t.Errorf("Wrong number of batches triggered. Got %d, expected %d.", batches, tc.triggeredBatches)
		}
	}
}

func TestServeHTTP(t *testing.T) {
	pr1 := PullRequest{}
	pr1.Commits.Nodes = append(pr1.Commits.Nodes, struct{ Commit Commit }{})
	pr1.Commits.Nodes[0].Commit.Status.Contexts = []Context{{
		Context:     githubql.String("coverage/coveralls"),
		Description: githubql.String("Coverage increased (+0.1%) to 27.599%"),
	}}
	c := &Controller{
		pools: []Pool{
			{
				MissingPRs: []PullRequest{pr1},
				Action:     Merge,
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
		t.Fatalf("JSON decoding error: %v", err)
	}
	if !reflect.DeepEqual(c.pools, pools) {
		t.Errorf("Received pools %v do not match original pools %v.", pools, c.pools)
	}
}

func TestHeadContexts(t *testing.T) {
	type commitContext struct {
		// one context per commit for testing
		context string
		sha     string
	}

	win := "win"
	lose := "lose"
	headSHA := "head"
	testCases := []struct {
		name           string
		commitContexts []commitContext
		expectAPICall  bool
	}{
		{
			name: "first commit is head",
			commitContexts: []commitContext{
				{context: win, sha: headSHA},
				{context: lose, sha: "other"},
				{context: lose, sha: "sha"},
			},
		},
		{
			name: "last commit is head",
			commitContexts: []commitContext{
				{context: lose, sha: "shaaa"},
				{context: lose, sha: "other"},
				{context: win, sha: headSHA},
			},
		},
		{
			name: "no commit is head",
			commitContexts: []commitContext{
				{context: lose, sha: "shaaa"},
				{context: lose, sha: "other"},
				{context: lose, sha: "sha"},
			},
			expectAPICall: true,
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test case %q", tc.name)
		fgc := &fgc{combinedStatus: map[string]string{win: string(githubql.StatusStateSuccess)}}
		if tc.expectAPICall {
			fgc.expectedSHA = headSHA
		}
		pr := &PullRequest{HeadRefOID: githubql.String(headSHA)}
		for _, ctx := range tc.commitContexts {
			commit := Commit{
				Status: struct{ Contexts []Context }{
					Contexts: []Context{
						{
							Context: githubql.String(ctx.context),
						},
					},
				},
				OID: githubql.String(ctx.sha),
			}
			pr.Commits.Nodes = append(pr.Commits.Nodes, struct{ Commit Commit }{commit})
		}

		contexts, err := headContexts(logrus.WithField("component", "tide"), fgc, pr)
		if err != nil {
			t.Fatalf("Unexpected error from headContexts: %v", err)
		}
		if len(contexts) != 1 || string(contexts[0].Context) != win {
			t.Errorf("Expected exactly 1 %q context, but got: %#v", win, contexts)
		}
	}
}

func testPR(org, repo, branch string, number int, mergeable githubql.MergeableState) PullRequest {
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
	return pr
}

func TestSync(t *testing.T) {
	mergeableA := testPR("org", "repo", "A", 5, githubql.MergeableStateMergeable)
	unmergeableA := testPR("org", "repo", "A", 6, githubql.MergeableStateConflicting)
	unmergeableB := testPR("org", "repo", "B", 7, githubql.MergeableStateConflicting)
	unknownA := testPR("org", "repo", "A", 8, githubql.MergeableStateUnknown)

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
				SuccessPRs: []PullRequest{mergeableA},
				Action:     Merge,
				Target:     []PullRequest{mergeableA},
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
				SuccessPRs: []PullRequest{unknownA},
				Action:     Merge,
				Target:     []PullRequest{unknownA},
			}},
		},
		{
			name: "1 mergeable, 1 unmergeable (different pools)",
			prs:  []PullRequest{mergeableA, unmergeableB},
			expectedPools: []Pool{{
				Org:        "org",
				Repo:       "repo",
				Branch:     "A",
				SuccessPRs: []PullRequest{mergeableA},
				Action:     Merge,
				Target:     []PullRequest{mergeableA},
			}},
		},
		{
			name: "1 mergeable, 1 unmergeable (same pool)",
			prs:  []PullRequest{mergeableA, unmergeableA},
			expectedPools: []Pool{{
				Org:        "org",
				Repo:       "repo",
				Branch:     "A",
				SuccessPRs: []PullRequest{mergeableA},
				Action:     Merge,
				Target:     []PullRequest{mergeableA},
			}},
		},
		{
			name: "1 mergeable PR (satisfies multiple queries)",
			prs:  []PullRequest{mergeableA, mergeableA},
			expectedPools: []Pool{{
				Org:        "org",
				Repo:       "repo",
				Branch:     "A",
				SuccessPRs: []PullRequest{mergeableA},
				Action:     Merge,
				Target:     []PullRequest{mergeableA},
			}},
		},
	}

	for _, tc := range testcases {
		t.Logf("Starting case %q...", tc.name)
		fgc := &fgc{prs: tc.prs}
		fkc := &fkc{}
		ca := &config.Agent{}
		ca.Set(&config.Config{
			ProwConfig: config.ProwConfig{
				Tide: config.Tide{
					Queries:       []config.TideQuery{{}},
					MaxGoroutines: 4,
				},
			},
		})
		sc := &statusController{
			logger:         logrus.WithField("controller", "status-update"),
			ghc:            fgc,
			ca:             ca,
			newPoolPending: make(chan bool, 1),
			shutDown:       make(chan bool),
		}
		go sc.run()
		defer sc.shutdown()
		c := &Controller{
			ca:     ca,
			ghc:    fgc,
			kc:     fkc,
			logger: logrus.WithField("controller", "sync"),
			sc:     sc,
			changedFiles: &changedFilesAgent{
				ghc:             fgc,
				nextChangeCache: make(map[changeCacheKey][]string),
			},
		}

		if err := c.Sync(); err != nil {
			t.Errorf("Unexpected error from 'Sync()': %v.", err)
			continue
		}
		if len(tc.expectedPools) != len(c.pools) {
			t.Errorf("Tide pools did not match expected. Got %#v, expected %#v.", c.pools, tc.expectedPools)
			continue
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
	}
}

func TestFilterSubpool(t *testing.T) {
	presubmits := map[int]sets.String{
		1: sets.NewString("pj-a"),
		2: sets.NewString("pj-a", "pj-b"),
	}

	trueVar := true
	cc := &config.TideContextPolicy{
		RequiredContexts:    []string{"pj-a", "pj-b", "other-a"},
		OptionalContexts:    []string{"tide", "pj-c"},
		SkipUnknownContexts: &trueVar,
	}

	type pr struct {
		number    int
		mergeable bool
		contexts  []Context
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
				org:               "org",
				repo:              "repo",
				branch:            "branch",
				presubmitContexts: presubmits,
				cc:                cc,
				log:               logrus.WithFields(logrus.Fields{"org": "org", "repo": "repo", "branch": "branch"}),
			}
			for _, pull := range tc.prs {
				pr := PullRequest{
					Number: githubql.Int(pull.number),
				}
				pr.Commits.Nodes = []struct{ Commit Commit }{
					{
						Commit{
							Status: struct{ Contexts []Context }{
								Contexts: pull.contexts,
							},
						},
					},
				}
				if !pull.mergeable {
					pr.Mergeable = githubql.MergeableStateConflicting
				}
				sp.prs = append(sp.prs, pr)
			}

			filtered := filterSubpool(nil, sp)
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
		name             string
		passing          bool
		config           config.TideContextPolicy
		combinedContexts map[string]string
	}{
		{
			name:             "empty policy - success (trust combined status)",
			passing:          true,
			combinedContexts: map[string]string{"c1": success, "c2": success, statusContext: failure},
		},
		{
			name:             "empty policy - failure because of failed context c4 (trust combined status)",
			passing:          false,
			combinedContexts: map[string]string{"c1": success, "c2": success, "c3": failure, statusContext: failure},
		},
		{
			name:    "passing (trust combined status)",
			passing: true,
			config: config.TideContextPolicy{
				RequiredContexts:    []string{"c1", "c2", "c3"},
				SkipUnknownContexts: &no,
			},
			combinedContexts: map[string]string{"c1": success, "c2": success, "c3": success, statusContext: failure},
		},
		{
			name:    "failing because of missing required check c3",
			passing: false,
			config: config.TideContextPolicy{
				RequiredContexts: []string{"c1", "c2", "c3"},
			},
			combinedContexts: map[string]string{"c1": success, "c2": success, statusContext: failure},
		},
		{
			name:             "failing because of failed context c2",
			passing:          false,
			combinedContexts: map[string]string{"c1": success, "c2": failure},
			config: config.TideContextPolicy{
				RequiredContexts: []string{"c1", "c2", "c3"},
				OptionalContexts: []string{"c4"},
			},
		},
		{
			name:    "passing because of failed context c4 is optional",
			passing: true,

			combinedContexts: map[string]string{"c1": success, "c2": success, "c3": success, "c4": failure},
			config: config.TideContextPolicy{
				RequiredContexts: []string{"c1", "c2", "c3"},
				OptionalContexts: []string{"c4"},
			},
		},
		{
			name:    "skipping unknown contexts - failing because of missing required context c3",
			passing: false,
			config: config.TideContextPolicy{
				RequiredContexts:    []string{"c1", "c2", "c3"},
				SkipUnknownContexts: &yes,
			},
			combinedContexts: map[string]string{"c1": success, "c2": success, statusContext: failure},
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
		},
	}

	for _, tc := range testCases {
		ghc := &fgc{
			combinedStatus: tc.combinedContexts,
			expectedSHA:    headSHA}
		log := logrus.WithField("component", "tide")
		_, err := log.String()
		if err != nil {
			t.Errorf("Failed to get log output before testing: %v", err)
			t.FailNow()
		}
		pr := PullRequest{HeadRefOID: githubql.String(headSHA)}
		passing := isPassingTests(log, ghc, pr, &tc.config)
		if passing != tc.passing {
			t.Errorf("%s: Expected %t got %t", tc.name, tc.passing, passing)
		}
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

		expectedPresubmits  map[int]sets.String
		expectedChangeCache map[changeCacheKey][]string
	}{
		{
			name: "no matching presubmits",
			presubmits: []config.Presubmit{
				{
					Context: "always",
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "foo",
					},
				},
				{
					Context: "never",
				},
			},
			expectedChangeCache: map[changeCacheKey][]string{{number: 100, sha: "sha"}: {"CHANGED"}},
			expectedPresubmits:  map[int]sets.String{},
		},
		{
			name:               "no presubmits",
			presubmits:         []config.Presubmit{},
			expectedPresubmits: map[int]sets.String{},
		},
		{
			name: "no matching presubmits (check cache eviction)",
			presubmits: []config.Presubmit{
				{
					Context: "never",
				},
			},
			initialChangeCache: map[changeCacheKey][]string{{number: 100, sha: "sha"}: {"FILE"}},
			expectedPresubmits: map[int]sets.String{},
		},
		{
			name: "no matching presubmits (check cache retention)",
			presubmits: []config.Presubmit{
				{
					Context: "always",
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "foo",
					},
				},
				{
					Context: "never",
				},
			},
			initialChangeCache:  map[changeCacheKey][]string{{number: 100, sha: "sha"}: {"FILE"}},
			expectedChangeCache: map[changeCacheKey][]string{{number: 100, sha: "sha"}: {"FILE"}},
			expectedPresubmits:  map[int]sets.String{},
		},
		{
			name: "always_run",
			presubmits: []config.Presubmit{
				{
					Context:   "always",
					AlwaysRun: true,
				},
				{
					Context: "never",
				},
			},
			expectedPresubmits: map[int]sets.String{100: sets.NewString("always")},
		},
		{
			name: "runs against branch",
			presubmits: []config.Presubmit{
				{
					Context:   "presubmit",
					AlwaysRun: true,
					Brancher: config.Brancher{
						Branches: []string{"master", "dev"},
					},
				},
				{
					Context: "never",
				},
			},
			expectedPresubmits: map[int]sets.String{100: sets.NewString("presubmit")},
		},
		{
			name: "doesn't run against branch",
			presubmits: []config.Presubmit{
				{
					Context:   "presubmit",
					AlwaysRun: true,
					Brancher: config.Brancher{
						Branches: []string{"release", "dev"},
					},
				},
				{
					Context:   "always",
					AlwaysRun: true,
				},
				{
					Context: "never",
				},
			},
			expectedPresubmits: map[int]sets.String{100: sets.NewString("always")},
		},
		{
			name: "run_if_changed (uncached)",
			presubmits: []config.Presubmit{
				{
					Context: "presubmit",
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "^CHANGE.$",
					},
				},
				{
					Context:   "always",
					AlwaysRun: true,
				},
				{
					Context: "never",
				},
			},
			expectedPresubmits:  map[int]sets.String{100: sets.NewString("presubmit", "always")},
			expectedChangeCache: map[changeCacheKey][]string{{number: 100, sha: "sha"}: {"CHANGED"}},
		},
		{
			name: "run_if_changed (cached)",
			presubmits: []config.Presubmit{
				{
					Context: "presubmit",
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "^FIL.$",
					},
				},
				{
					Context:   "always",
					AlwaysRun: true,
				},
				{
					Context: "never",
				},
			},
			initialChangeCache:  map[changeCacheKey][]string{{number: 100, sha: "sha"}: {"FILE"}},
			expectedPresubmits:  map[int]sets.String{100: sets.NewString("presubmit", "always")},
			expectedChangeCache: map[changeCacheKey][]string{{number: 100, sha: "sha"}: {"FILE"}},
		},
		{
			name: "run_if_changed (cached) (skippable)",
			presubmits: []config.Presubmit{
				{
					Context: "presubmit",
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "^CHANGE.$",
					},
				},
				{
					Context:   "always",
					AlwaysRun: true,
				},
				{
					Context: "never",
				},
			},
			initialChangeCache:  map[changeCacheKey][]string{{number: 100, sha: "sha"}: {"FILE"}},
			expectedPresubmits:  map[int]sets.String{100: sets.NewString("always")},
			expectedChangeCache: map[changeCacheKey][]string{{number: 100, sha: "sha"}: {"FILE"}},
		},
	}

	for _, tc := range testcases {
		t.Logf("Starting test case: %q", tc.name)

		if tc.initialChangeCache == nil {
			tc.initialChangeCache = map[changeCacheKey][]string{}
		}
		if tc.expectedChangeCache == nil {
			tc.expectedChangeCache = map[changeCacheKey][]string{}
		}

		cfg := &config.Config{}
		cfg.SetPresubmits(map[string][]config.Presubmit{
			"/":       tc.presubmits,
			"foo/bar": {{Context: "wrong-repo", AlwaysRun: true}},
		})
		cfgAgent := &config.Agent{}
		cfgAgent.Set(cfg)
		sp := &subpool{
			branch: "master",
			prs:    []PullRequest{samplePR},
		}
		c := &Controller{
			ca:  cfgAgent,
			ghc: &fgc{},
			changedFiles: &changedFilesAgent{
				ghc:             &fgc{},
				changeCache:     tc.initialChangeCache,
				nextChangeCache: make(map[changeCacheKey][]string),
			},
		}
		presubmits, err := c.presubmitsByPull(sp)
		if err != nil {
			t.Fatalf("unexpected error from presubmitsByPull: %v", err)
		}
		c.changedFiles.prune()
		if !reflect.DeepEqual(presubmits, tc.expectedPresubmits) {
			t.Errorf("expected presubmit mapping: %v,\nbut got %v\n", tc.expectedPresubmits, presubmits)
		}
		if got := c.changedFiles.changeCache; !reflect.DeepEqual(got, tc.expectedChangeCache) {
			t.Errorf("expected file change cache: %v,\nbut got %v\n", tc.expectedChangeCache, got)
		}
	}
}
