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
	"fmt"
	"testing"

	"github.com/shurcooL/githubql"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/kube"
)

func testPullsMatchList(t *testing.T, test string, actual []pullRequest, expected []int) {
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
		var pulls []pullRequest
		for _, p := range test.pulls {
			pr := pullRequest{Number: githubql.Int(p.number)}
			pr.HeadRef.Target.OID = githubql.String(p.sha)
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
		if pending != test.pending {
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
		var pulls []pullRequest
		for _, p := range test.pullRequests {
			pulls = append(pulls, pullRequest{Number: githubql.Int(p)})
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
	refs map[string]string
}

func (f *fgc) GetRef(o, r, ref string) (string, error) {
	return f.refs[o+"/"+r+" "+ref], nil
}

func (f *fgc) Query(ctx context.Context, q interface{}, vars map[string]interface{}) error {
	return nil
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
		log: logrus.NewEntry(logrus.StandardLogger()),
		ghc: fc,
	}
	var pulls []pullRequest
	for _, p := range testPulls {
		npr := pullRequest{Number: githubql.Int(p.number)}
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
