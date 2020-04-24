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
	"testing"
	"text/template"
	"time"

	"github.com/go-test/deep"
	"github.com/google/go-cmp/cmp"
	fuzz "github.com/google/gofuzz"
	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/equality"
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
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/tide/history"
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

			var pulls []PullRequest
			for _, p := range test.pulls {
				pr := PullRequest{
					Number:     githubql.Int(p.number),
					HeadRefOID: githubql.String(p.sha),
				}
				pulls = append(pulls, pr)
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
				inrepoconfig.Enabled = map[string]*bool{"*": utilpointer.BoolPtr(true)}
			}
			c := &Controller{
				config: func() *config.Config {
					return &config.Config{
						JobConfig: config.JobConfig{
							PresubmitsStatic: map[string][]config.Presubmit{
								"org/repo": test.presubmits,
							},
							ProwYAMLGetter: test.prowYAMLGetter,
						},
						ProwConfig: config.ProwConfig{
							InRepoConfig: inrepoconfig,
						},
					}
				},
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
		presubmits   map[int][]config.Presubmit
		pullRequests map[int]string
		prowJobs     []prowjob

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
	}

	for i, test := range tests {
		var pulls []PullRequest
		for num, sha := range test.pullRequests {
			pulls = append(
				pulls,
				PullRequest{Number: githubql.Int(num), HeadRefOID: githubql.String(sha)},
			)
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

		successes, pendings, nones, _ := accumulate(test.presubmits, pulls, pjs, logrus.NewEntry(logrus.New()))

		t.Logf("test run %d", i)
		testPullsMatchList(t, "successes", successes, test.successes)
		testPullsMatchList(t, "pendings", pendings, test.pendings)
		testPullsMatchList(t, "nones", nones, test.none)
	}
}

type fgc struct {
	err error

	prs       []PullRequest
	refs      map[string]string
	merged    int
	setStatus bool
	statuses  map[string]github.Status
	mergeErrs map[int]error

	expectedSHA    string
	combinedStatus map[string]string
	checkRuns      *github.CheckRunList
	issueComments  map[string][]github.IssueComment
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
	if err, ok := f.mergeErrs[number]; ok {
		return err
	}
	f.merged++
	return nil
}

func (f *fgc) CreateStatus(org, repo, ref string, s github.Status) error {
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

func (f *fgc) ListCheckRuns(org, repo, ref string) (*github.CheckRunList, error) {
	if f.expectedSHA != ref {
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

func (f *fgc) BotUserChecker() (func(string) bool, error) {
	return func(candidate string) bool {
		return candidate == "BotName"
	}, nil
}

func (f *fgc) ListIssueComments(org, repo string, number int) ([]github.IssueComment, error) {
	comments, _ := f.issueComments[fmt.Sprintf("%s/%s#%d", org, repo, number)]
	return comments, nil
}

func (f *fgc) CreateComment(org, repo string, number int, body string) error {
	if len(f.issueComments) == 0 {
		f.issueComments = make(map[string][]github.IssueComment)
	}
	key := fmt.Sprintf("%s/%s#%d", org, repo, number)
	f.issueComments[key] = append(f.issueComments[key], github.IssueComment{
		User: github.User{
			Login: "BotName",
		},
		Body: body,
	})
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
			baseRef: "master",
			baseSHA: "123",
		},
		{
			jobType: prowapi.BatchJob,
			org:     "k",
			repo:    "t-i",
			baseRef: "master",
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
			baseRef: "master",
			baseSHA: "abc",
		},
		{
			jobType: prowapi.PresubmitJob,
			org:     "o",
			repo:    "t-i",
			baseRef: "master",
			baseSHA: "123",
		},
		{
			jobType: prowapi.PresubmitJob,
			org:     "k",
			repo:    "other",
			baseRef: "master",
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

	log := logrus.NewEntry(logrus.StandardLogger())
	mmc := newMergeChecker(configGetter, fc)
	mgr := newFakeManager()
	mfc, err := newFailureCommenter(fc)
	if err != nil {
		t.Fatalf("Failed to initialize merge failure commenter: %v", err)
	}
	c, err := newSyncController(
		context.Background(), log, fc, mgr, configGetter, nil, nil, nil, mmc, mfc,
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
	pulls := make(map[string]PullRequest)
	for _, p := range testPulls {
		npr := PullRequest{Number: githubql.Int(p.number)}
		npr.BaseRef.Name = githubql.String(p.branch)
		npr.BaseRef.Prefix = "refs/heads/"
		npr.Repository.Name = githubql.String(p.repo)
		npr.Repository.Owner.Login = githubql.String(p.org)
		pulls[prKey(&npr)] = npr
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
			if string(pr.Repository.Owner.Login) != sp.org || string(pr.Repository.Name) != sp.repo || string(pr.BaseRef.Name) != sp.branch {
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

func TestPickBatch(t *testing.T) {
	testPickBatch(localgit.New, t)
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
	c := &Controller{
		logger: logrus.WithField("component", "tide"),
		gc:     gc,
		config: ca.Config,
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
	})
	if err != nil {
		t.Fatalf("Error from pickBatch: %v", err)
	}
	if !equality.Semantic.DeepEqual(presubmits, ca.Config().PresubmitsStatic["o/r"]) {
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
		SquashLabel: squashLabel,
		MergeLabel:  mergeLabel,
		RebaseLabel: rebaseLabel,

		MergeType: map[string]github.PullRequestMergeType{
			"o/configured-rebase":              github.MergeRebase, // GH client allows merge, rebase
			"o/configured-squash-allow-rebase": github.MergeSquash, // GH client allows merge, squash, rebase
			"o/configure-re-base":              github.MergeRebase, // GH client allows merge
		},
	}
	cfg := func() *config.Config { return &config.Config{ProwConfig: config.ProwConfig{Tide: tideConfig}} }
	mmc := newMergeChecker(cfg, &fgc{})

	testcases := []struct {
		name              string
		repo              string
		labels            []string
		conflict          bool
		expectedMethod    github.PullRequestMergeType
		expectErr         bool
		expectConflictErr bool
	}{
		{
			name:           "default method without PR label override",
			repo:           "foo",
			expectedMethod: github.MergeMerge,
		},
		{
			name:           "irrelevant PR labels ignored",
			repo:           "foo",
			labels:         []string{"unrelated"},
			expectedMethod: github.MergeMerge,
		},
		{
			name:           "default method overridden by a PR label",
			repo:           "allow-squash-nomerge",
			labels:         []string{"tide/squash"},
			expectedMethod: github.MergeSquash,
		},
		{
			name:           "use method configured for repo in tide config",
			repo:           "configured-squash-allow-rebase",
			labels:         []string{"unrelated"},
			expectedMethod: github.MergeSquash,
		},
		{
			name:           "tide config method overridden by a PR label",
			repo:           "configured-squash-allow-rebase",
			labels:         []string{"unrelated", "tide/rebase"},
			expectedMethod: github.MergeRebase,
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
			expectedMethod:    github.MergeMerge,
			expectErr:         false,
			expectConflictErr: true,
		},
		{
			name:              "squash label conflicts with merge only GH settings",
			repo:              "foo",
			labels:            []string{"tide/squash"},
			expectedMethod:    github.MergeSquash,
			expectErr:         false,
			expectConflictErr: true,
		},
		{
			name:              "rebase method tide config conflicts with merge only GH settings",
			repo:              "configure-re-base",
			labels:            []string{"unrelated"},
			expectedMethod:    github.MergeRebase,
			expectErr:         false,
			expectConflictErr: true,
		},
		{
			name:              "default method conflicts with squash only GH settings",
			repo:              "squash-nomerge",
			labels:            []string{"unrelated"},
			expectedMethod:    github.MergeMerge,
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
			}
			for _, label := range tc.labels {
				labelNode := struct{ Name githubql.String }{Name: githubql.String(label)}
				pr.Labels.Nodes = append(pr.Labels.Nodes, labelNode)
			}
			if tc.conflict {
				pr.Mergeable = githubql.MergeableStateConflicting
			}

			actual, err := prMergeMethod(tideConfig, pr)
			if err != nil {
				if !tc.expectErr {
					t.Errorf("unexpected error: %v", err)
				}
				return
			} else if tc.expectErr {
				t.Errorf("missing expected error")
				return
			}
			if tc.expectedMethod != actual {
				t.Errorf("wanted: %q, got: %q", tc.expectedMethod, actual)
			}
			reason, err := mmc.isAllowed(pr)
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

func TestTakeAction(t *testing.T) {
	testTakeAction(localgit.New, t)
}

func TestTakeActionV2(t *testing.T) {
	testTakeAction(localgit.NewV2, t)
}

func testTakeAction(clients localgit.Clients, t *testing.T) {
	sleep = func(time.Duration) {}
	defer func() { sleep = time.Sleep }()

	// PRs 0-9 exist. All are mergable, and all are passing tests.
	testcases := []struct {
		name             string
		batchPending     bool
		successes        []int
		pendings         []int
		nones            []int
		batchMerges      []int
		presubmits       map[int][]config.Presubmit
		preExistingJobs  []runtime.Object
		mergeErrs        map[int]error
		existingComments map[string][]github.IssueComment
		merged           int
		triggered        int
		triggeredBatches int
		action           Action
		expectErr        bool
		expectedComments map[string][]github.IssueComment
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
						BaseRef: "master",
						BaseSHA: "master",
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
						BaseRef: "master",
						BaseSHA: "master",
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
						BaseRef: "master",
						BaseSHA: "master",
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
		{
			name: "on merge failure, a comment is left on the PR",

			batchMerges: []int{1, 2, 101},
			mergeErrs:   map[int]error{101: errors.New("test error")},
			merged:      2,
			triggered:   0,
			action:      MergeBatch,
			expectErr:   true,
			expectedComments: map[string][]github.IssueComment{
				"o/r#101": {
					{
						User: github.User{Login: "BotName"},
						Body: fmt.Sprintf("%s The following are the error details:\n<code>\n%s\n</code>\n", mergeFailureComment(true), "test error"),
					},
				},
			},
		},
		{
			name: "do not comment on PR which already has a merge failure comment",

			batchMerges: []int{1, 2, 101},
			mergeErrs:   map[int]error{101: errors.New("test error")},
			existingComments: map[string][]github.IssueComment{
				"o/r#101": {
					{
						User: github.User{Login: "BotName"},
						Body: mergeFailureComment(true),
					},
				},
			},
			merged:    2,
			triggered: 0,
			action:    MergeBatch,
			expectErr: true,
			expectedComments: map[string][]github.IssueComment{
				"o/r#101": {
					{
						User: github.User{Login: "BotName"},
						Body: mergeFailureComment(true),
					},
				},
			},
		},
		{
			name: "do not comment on PR if tide context is required by the repo",

			batchMerges: []int{1, 2, 3},
			mergeErrs:   map[int]error{2: errors.New("test error")},
			merged:      2,
			triggered:   0,
			action:      MergeBatch,
			expectErr:   true,
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
					101: &config.TideContextPolicy{
						OptionalContexts: []string{statusContext},
					},
				},
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
					oid := githubql.String(fmt.Sprintf("origin/pr-%d", i))
					var pr PullRequest
					pr.Number = githubql.Int(i)
					pr.HeadRefOID = oid
					pr.Commits.Nodes = []struct {
						Commit Commit
					}{{Commit: Commit{OID: oid}}}
					pr.Repository.Name = "r"
					pr.Repository.Owner.Login = "o"
					sp.prs = append(sp.prs, pr)
					prs = append(prs, pr)
				}
				return prs
			}
			fgc := fgc{
				mergeErrs:     tc.mergeErrs,
				issueComments: tc.existingComments,
			}
			mfc, err := newFailureCommenter(&fgc)
			if err != nil {
				t.Fatalf("Failed to initialize merge failure commenter: %v", err)
			}
			c, err := newSyncController(
				context.Background(),
				logrus.WithField("controller", "tide"),
				&fgc,
				newFakeManager(tc.preExistingJobs...),
				ca.Config,
				gc,
				nil,
				nil,
				nil,
				mfc,
			)
			if err != nil {
				t.Fatalf("failed to construct sync controller: %v", err)
			}
			c.changedFiles = &changedFilesAgent{
				ghc:             &fgc,
				nextChangeCache: make(map[changeCacheKey][]string),
			}
			var batchPending []PullRequest
			if tc.batchPending {
				batchPending = []PullRequest{{}}
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
					batchJobs = append(batchJobs, &pj)
				}
			}

			if tc.triggered != numCreated {
				t.Errorf("Wrong number of jobs triggered. Got %d, expected %d.", numCreated, tc.triggered)
			}
			if tc.merged != fgc.merged {
				t.Errorf("Wrong number of merges. Got %d, expected %d.", fgc.merged, tc.merged)
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
			if len(tc.expectedComments) != len(fgc.issueComments) {
				t.Errorf("Expected %d comments, found %d", len(tc.expectedComments), len(fgc.issueComments))
			}
			if len(tc.expectedComments) > 0 && !reflect.DeepEqual(fgc.issueComments, tc.expectedComments) {
				t.Errorf("Expected comments %#v does not match actual comments %#v.", tc.expectedComments, fgc.issueComments)
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
	c := &Controller{
		pools: []Pool{
			{
				MissingPRs: []PullRequest{pr1},
				Action:     Merge,
			},
		},
		mergeChecker: newMergeChecker(cfg, &fgc{}),
		History:      hist,
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
		name                string
		commitContexts      []commitContext
		expectAPICall       bool
		expectChecksAPICall bool
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
			name: "no commit is head, falling back to v3 api and getting context via status api",
			commitContexts: []commitContext{
				{context: lose, sha: "shaaa"},
				{context: lose, sha: "other"},
				{context: lose, sha: "sha"},
			},
			expectAPICall: true,
		},
		{
			name: "no commit is head, falling back to v3 api and getting context via checks api",
			commitContexts: []commitContext{
				{context: lose, sha: "shaaa"},
				{context: lose, sha: "other"},
				{context: lose, sha: "sha"},
			},
			expectAPICall:       true,
			expectChecksAPICall: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Running test case %q", tc.name)
			fgc := &fgc{}
			if !tc.expectChecksAPICall {
				fgc.combinedStatus = map[string]string{win: string(githubql.StatusStateSuccess)}
			} else {
				fgc.checkRuns = &github.CheckRunList{CheckRuns: []github.CheckRun{
					{Name: win, Status: "completed", Conclusion: "neutral"},
				}}
			}
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
		})
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

func testPRWithLabels(org, repo, branch string, number int, mergeable githubql.MergeableState, labels []string) PullRequest {
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
		fgc := &fgc{
			prs: tc.prs,
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
					Queries:            []config.TideQuery{{}},
					MaxGoroutines:      4,
					StatusUpdatePeriod: &metav1.Duration{Duration: time.Second * 0},
				},
			},
		})
		hist, err := history.New(100, nil, "")
		if err != nil {
			t.Fatalf("Failed to create history client: %v", err)
		}
		mergeChecker := newMergeChecker(ca.Config, fgc)
		sc := &statusController{
			pjClient:       fakectrlruntimeclient.NewFakeClient(),
			logger:         logrus.WithField("controller", "status-update"),
			ghc:            fgc,
			gc:             nil,
			config:         ca.Config,
			newPoolPending: make(chan bool, 1),
			shutDown:       make(chan bool),
			mergeChecker:   mergeChecker,
		}
		go sc.run()
		defer sc.shutdown()
		mfc, err := newFailureCommenter(fgc)
		if err != nil {
			t.Fatalf("Failed to initialize merge failure commenter: %v", err)
		}
		c := &Controller{
			config:        ca.Config,
			ghc:           fgc,
			gc:            nil,
			prowJobClient: fakectrlruntimeclient.NewFakeClient(),
			logger:        logrus.WithField("controller", "sync"),
			sc:            sc,
			changedFiles: &changedFilesAgent{
				ghc:             fgc,
				nextChangeCache: make(map[changeCacheKey][]string),
			},
			mergeChecker:     mergeChecker,
			History:          hist,
			failureCommenter: mfc,
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
				sp.prs = append(sp.prs, pr)
			}

			configGetter := func() *config.Config { return &config.Config{} }
			mmc := newMergeChecker(configGetter, &fgc{})
			filtered := filterSubpool(nil, mmc.isAllowed, sp)
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
			t.Fatalf("Failed to get log output before testing: %v", err)
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
		prs                []PullRequest
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
						Branches: []string{"master", "dev"},
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
					Branches: []string{"master", "dev"},
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
			prs: []PullRequest{
				{Number: githubql.Int(1), HeadRefOID: githubql.String("1")},
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
			prowYAMLGetter: func(_ *config.Config, _ git.ClientFactory, _, _ string, headRefs ...string) (*config.ProwYAML, error) {
				if len(headRefs) == 1 && headRefs[0] == "1" {
					return nil, errors.New("you shall not get jobs")
				}
				return &config.ProwYAML{}, nil
			},
			prs: []PullRequest{
				{Number: githubql.Int(1), HeadRefOID: githubql.String("1")},
			},
			expectedPresubmits: map[int][]config.Presubmit{
				100: {
					{AlwaysRun: true, Reporter: config.Reporter{Context: "always"}},
				},
			},
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
			"foo/bar": {{Reporter: config.Reporter{Context: "wrong-repo"}, AlwaysRun: true}},
		})
		if tc.prowYAMLGetter != nil {
			cfg.InRepoConfig.Enabled = map[string]*bool{"*": utilpointer.BoolPtr(true)}
			cfg.ProwYAMLGetter = tc.prowYAMLGetter
		}
		cfgAgent := &config.Agent{}
		cfgAgent.Set(cfg)
		sp := &subpool{
			branch: "master",
			sha:    "master-sha",
			prs:    append(tc.prs, samplePR),
		}
		c := &Controller{
			config: cfgAgent.Config,
			ghc:    &fgc{},
			gc:     nil,
			changedFiles: &changedFilesAgent{
				ghc:             &fgc{},
				changeCache:     tc.initialChangeCache,
				nextChangeCache: make(map[changeCacheKey][]string),
			},
			mergeChecker: newMergeChecker(cfgAgent.Config, &fgc{}),
			logger:       logrus.WithField("test", tc.name),
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
		if !equality.Semantic.DeepEqual(presubmits, tc.expectedPresubmits) {
			t.Errorf("got incorrect presubmit mapping: %v\n", diff.ObjectReflectDiff(tc.expectedPresubmits, presubmits))
		}
		if got := c.changedFiles.changeCache; !reflect.DeepEqual(got, tc.expectedChangeCache) {
			t.Errorf("got incorrect file change cache: %v", diff.ObjectReflectDiff(tc.expectedChangeCache, got))
		}
	}
}

func getTemplate(name, tplStr string) *template.Template {
	tpl, _ := template.New(name).Parse(tplStr)
	return tpl
}

func TestPrepareMergeDetails(t *testing.T) {
	pr := PullRequest{
		Number:     githubql.Int(1),
		Mergeable:  githubql.MergeableStateMergeable,
		HeadRefOID: githubql.String("SHA"),
		Title:      "my commit title",
		Body:       "my commit body",
	}

	testCases := []struct {
		name        string
		tpl         config.TideMergeCommitTemplate
		pr          PullRequest
		mergeMethod github.PullRequestMergeType
		expected    github.MergeDetails
	}{{
		name:        "No commit template",
		tpl:         config.TideMergeCommitTemplate{},
		pr:          pr,
		mergeMethod: "merge",
		expected: github.MergeDetails{
			SHA:         "SHA",
			MergeMethod: "merge",
		},
	}, {
		name: "No commit template fields",
		tpl: config.TideMergeCommitTemplate{
			Title: nil,
			Body:  nil,
		},
		pr:          pr,
		mergeMethod: "merge",
		expected: github.MergeDetails{
			SHA:         "SHA",
			MergeMethod: "merge",
		},
	}, {
		name: "Static commit template",
		tpl: config.TideMergeCommitTemplate{
			Title: getTemplate("CommitTitle", "static title"),
			Body:  getTemplate("CommitBody", "static body"),
		},
		pr:          pr,
		mergeMethod: "merge",
		expected: github.MergeDetails{
			SHA:           "SHA",
			MergeMethod:   "merge",
			CommitTitle:   "static title",
			CommitMessage: "static body",
		},
	}, {
		name: "Commit template uses PullRequest fields",
		tpl: config.TideMergeCommitTemplate{
			Title: getTemplate("CommitTitle", "{{ .Number }}: {{ .Title }}"),
			Body:  getTemplate("CommitBody", "{{ .HeadRefOID }} - {{ .Body }}"),
		},
		pr:          pr,
		mergeMethod: "merge",
		expected: github.MergeDetails{
			SHA:           "SHA",
			MergeMethod:   "merge",
			CommitTitle:   "1: my commit title",
			CommitMessage: "SHA - my commit body",
		},
	}, {
		name: "Commit template uses nonexistent fields",
		tpl: config.TideMergeCommitTemplate{
			Title: getTemplate("CommitTitle", "{{ .Hello }}"),
			Body:  getTemplate("CommitBody", "{{ .World }}"),
		},
		pr:          pr,
		mergeMethod: "merge",
		expected: github.MergeDetails{
			SHA:         "SHA",
			MergeMethod: "merge",
		},
	}}

	for _, test := range testCases {
		cfg := &config.Config{}
		cfgAgent := &config.Agent{}
		cfgAgent.Set(cfg)
		c := &Controller{
			config: cfgAgent.Config,
			ghc:    &fgc{},
			logger: logrus.WithField("component", "tide"),
		}

		actual := c.prepareMergeDetails(test.tpl, test.pr, test.mergeMethod)

		if !reflect.DeepEqual(actual, test.expected) {
			t.Errorf("Case %s failed: expected %+v, got %+v", test.name, test.expected, actual)
		}
	}
}

func TestAccumulateReturnsCorrectMissingTests(t *testing.T) {
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
				Number:     githubql.Int(1),
				HeadRefOID: githubql.String("sha"),
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
				Number:     githubql.Int(1),
				HeadRefOID: githubql.String("sha"),
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
				Number:     githubql.Int(1),
				HeadRefOID: githubql.String("sha"),
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
				Number:     githubql.Int(1),
				HeadRefOID: githubql.String("sha"),
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
					Number:     githubql.Int(1),
					HeadRefOID: githubql.String("sha"),
				},
				{
					Number:     githubql.Int(2),
					HeadRefOID: githubql.String("sha"),
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
	}

	log := logrus.NewEntry(logrus.New())
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, missingSerialTests := accumulate(tc.presubmits, tc.prs, tc.pjs, log)
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
		prs            []PullRequest
		changedFiles   *changedFilesAgent
		jobs           []config.Presubmit
		prowYAMLGetter config.ProwYAMLGetter
		expected       []config.Presubmit
	}{
		{
			name: "All jobs get picked",
			prs:  []PullRequest{getPR("org", "repo", 1)},
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
			prs:  []PullRequest{getPR("org", "repo", 1)},
			jobs: []config.Presubmit{{
				AlwaysRun: true,
				Reporter:  config.Reporter{Context: "foo"},
				Brancher:  config.Brancher{Branches: []string{"master"}},
			}},
			expected: []config.Presubmit{{
				AlwaysRun: true,
				Reporter:  config.Reporter{Context: "foo"},
				Brancher:  config.Brancher{Branches: []string{"master"}},
			}},
		},
		{
			name: "Optional jobs are excluded",
			prs:  []PullRequest{getPR("org", "repo", 1)},
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
			prs: []PullRequest{
				getPR("org", "repo", 2),
				getPR("org", "repo", 1, func(pr *PullRequest) {
					pr.HeadRefOID = githubql.String("sha")
				}),
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
			prs: []PullRequest{
				getPR("org", "repo", 2),
				getPR("org", "repo", 1, func(pr *PullRequest) {
					pr.HeadRefOID = githubql.String("sha")
				}),
			},
			jobs: []config.Presubmit{
				{
					AlwaysRun: true,
					Reporter:  config.Reporter{Context: "foo"},
				},
			},
			prowYAMLGetter: prowYAMLGetterForHeadRefs([]string{"sha", ""}, []config.Presubmit{{
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
			prs: []PullRequest{
				getPR("org", "repo", 2),
				getPR("org", "repo", 1, func(pr *PullRequest) {
					pr.HeadRefOID = githubql.String("sha")
				}),
			},
			jobs: []config.Presubmit{
				{
					AlwaysRun: true,
					Reporter:  config.Reporter{Context: "foo"},
				},
			},
			prowYAMLGetter: prowYAMLGetterForHeadRefs([]string{"other-sha", ""}, []config.Presubmit{{
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
						org:    string(pr.Repository.Owner.Login),
						repo:   string(pr.Repository.Name),
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
				inrepoconfig.Enabled = map[string]*bool{"*": utilpointer.BoolPtr(true)}
			}
			c := &Controller{
				changedFiles: tc.changedFiles,
				config: func() *config.Config {
					return &config.Config{
						JobConfig: config.JobConfig{
							PresubmitsStatic: map[string][]config.Presubmit{
								"org/repo": tc.jobs,
							},
							ProwYAMLGetter: tc.prowYAMLGetter,
						},
						ProwConfig: config.ProwConfig{
							InRepoConfig: inrepoconfig,
						},
					}
				},
				logger: logrus.WithField("test", tc.name),
			}

			presubmits, err := c.presubmitsForBatch(tc.prs, "org", "repo", "baseSHA", "master")
			if err != nil {
				t.Fatalf("failed to get presubmits for batch: %v", err)
			}
			// Clear regexes, otherwise DeepEqual comparison wont work
			config.ClearCompiledRegexes(presubmits)
			if !equality.Semantic.DeepEqual(tc.expected, presubmits) {
				t.Errorf("returned presubmits do not match expected, diff: %v\n", diff.ObjectReflectDiff(tc.expected, presubmits))
			}
		})
	}
}

func TestChangedFilesAgentBatchChanges(t *testing.T) {
	testCases := []struct {
		name         string
		prs          []PullRequest
		changedFiles *changedFilesAgent
		expected     []string
	}{
		{
			name: "Single PR",
			prs: []PullRequest{
				getPR("org", "repo", 1),
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
			prs: []PullRequest{
				getPR("org", "repo", 1),
				getPR("org", "repo", 2),
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
			if !equality.Semantic.DeepEqual(result, tc.expected) {
				t.Errorf("returned changes do not match expected; diff: %v\n", diff.ObjectReflectDiff(tc.expected, result))
			}
		})
	}
}

func getPR(org, name string, number int, opts ...func(*PullRequest)) PullRequest {
	pr := PullRequest{}
	pr.Repository.Owner.Login = githubql.String(org)
	pr.Repository.NameWithOwner = githubql.String(org + "/" + name)
	pr.Repository.Name = githubql.String(name)
	pr.Number = githubql.Int(number)
	for _, opt := range opts {
		opt(&pr)
	}
	return pr
}

func TestCacheIndexFuncReturnsDifferentResultsForDifferentInputs(t *testing.T) {
	type orgRepoBranch struct{ org, repo, branch string }

	results := sets.String{}
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
	return func(_ *config.Config, _ git.ClientFactory, _, _ string, headRefs ...string) (*config.ProwYAML, error) {
		if len(headRefsToLookFor) != len(headRefs) {
			return nil, fmt.Errorf("expcted %d headrefs, got %d", len(headRefsToLookFor), len(headRefs))
		}
		var presubmits []config.Presubmit
		if sets.NewString(headRefsToLookFor...).Equal(sets.NewString(headRefs...)) {
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

func TestHasMergeFailureComment(t *testing.T) {
	testCases := []struct {
		name       string
		comments   []github.IssueComment
		hasComment bool
	}{
		{
			name: "comment author is verified against bot name",
			comments: []github.IssueComment{
				{
					Body: mergeFailureComment(true),
					User: github.User{Login: "foo"},
				},
			},
		},
		{
			name: "comment body is verified",
			comments: []github.IssueComment{
				{
					Body: "comment",
					User: github.User{Login: "BotName"},
				},
			},
		},
		{
			name: "previous failure comment is found",
			comments: []github.IssueComment{
				{
					Body: mergeFailureComment(true),
					User: github.User{Login: "BotName"},
				},
			},
			hasComment: true,
		},
		{
			name: "previous failure comment without retry is found",
			comments: []github.IssueComment{
				{
					Body: mergeFailureComment(false),
					User: github.User{Login: "BotName"},
				},
			},
			hasComment: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fgc := &fgc{
				issueComments: map[string][]github.IssueComment{
					"org/repo#1": tc.comments,
				},
			}

			mfc, err := newFailureCommenter(fgc)
			if err != nil {
				t.Fatal("Failed to initialize merge failure commenter")
			}
			pr := getPR("org", "repo", 1)
			got, err := mfc.HasMergeFailureComment(&pr)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.hasComment {
				t.Fatalf("expected %t got %t", tc.hasComment, got)
			}
		})
	}
}

func TestDeduplicateContestsDoesntLoseData(t *testing.T) {
	for i := 0; i < 100; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			context := Context{}
			fuzz.New().Fuzz(&context)
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
	}
	testCases := []struct {
		name     string
		prs      []PullRequest
		expected int
	}{
		{
			name: "no label",
			prs: []PullRequest{
				testPR("org", "repo", "A", 5, githubql.MergeableStateMergeable),
				testPR("org", "repo", "A", 3, githubql.MergeableStateMergeable),
			},
			expected: 3,
		},
		{
			name: "deflake PR",
			prs: []PullRequest{
				testPR("org", "repo", "A", 5, githubql.MergeableStateMergeable),
				testPR("org", "repo", "A", 3, githubql.MergeableStateMergeable),
				testPRWithLabels("org", "repo", "A", 7, githubql.MergeableStateMergeable, []string{"area/deflake"}),
			},
			expected: 7,
		},
		{
			name: "same label",
			prs: []PullRequest{
				testPRWithLabels("org", "repo", "A", 7, githubql.MergeableStateMergeable, []string{"area/deflake"}),
				testPRWithLabels("org", "repo", "A", 6, githubql.MergeableStateMergeable, []string{"area/deflake"}),
				testPRWithLabels("org", "repo", "A", 1, githubql.MergeableStateMergeable, []string{"area/deflake"}),
			},
			expected: 1,
		},
		{
			name: "missing one label",
			prs: []PullRequest{
				testPR("org", "repo", "A", 5, githubql.MergeableStateMergeable),
				testPR("org", "repo", "A", 3, githubql.MergeableStateMergeable),
				testPRWithLabels("org", "repo", "A", 6, githubql.MergeableStateMergeable, []string{"kind/bug"}),
			},
			expected: 3,
		},
		{
			name: "complete",
			prs: []PullRequest{
				testPR("org", "repo", "A", 5, githubql.MergeableStateMergeable),
				testPR("org", "repo", "A", 3, githubql.MergeableStateMergeable),
				testPRWithLabels("org", "repo", "A", 6, githubql.MergeableStateMergeable, []string{"kind/bug"}),
				testPRWithLabels("org", "repo", "A", 7, githubql.MergeableStateMergeable, []string{"area/deflake"}),
				testPRWithLabels("org", "repo", "A", 8, githubql.MergeableStateMergeable, []string{"kind/bug"}),
				testPRWithLabels("org", "repo", "A", 9, githubql.MergeableStateMergeable, []string{"kind/failing-test"}),
				testPRWithLabels("org", "repo", "A", 10, githubql.MergeableStateMergeable, []string{"kind/bug", "priority/critical-urgent"}),
			},
			expected: 9,
		},
	}
	alwaysTrue := func(*logrus.Entry, githubClient, PullRequest, contextChecker) bool { return true }
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, got := pickHighestPriorityPR(nil, nil, tc.prs, nil, alwaysTrue, priorities)
			if int(got.Number) != tc.expected {
				t.Errorf("got %d, expected %d", int(got.Number), tc.expected)
			}
		})
	}
}

func TestCreateMergeFailureComment(t *testing.T) {
	pr := getPR("org", "repo", 1)
	fgc := &fgc{}
	mfc, err := newFailureCommenter(fgc)
	if err != nil {
		t.Fatalf("Failed to initialize merge failure commenter: %v", err)
	}
	if err := mfc.CreateMergeFailureComment(&pr, fmt.Errorf("foo"), true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	comments, ok := fgc.issueComments[fmt.Sprintf("org/repo#1")]
	if !ok {
		t.Fatalf("no comments found for %s/%s#%d", "org", "repo", 1)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, found %d: %v", len(comments), comments)
	}
	if !strings.Contains(comments[0].Body, mergeFailureComment(true)) {
		t.Fatalf("expected comment to contain %q, found %q as comment body", mergeFailureComment(true), comments[0].Body)
	}
	if !strings.Contains(comments[0].Body, "foo") {
		t.Fatalf("expected error %q to be reported in the comment body, found %q as comment body", "foo", comments[0].Body)
	}
}
