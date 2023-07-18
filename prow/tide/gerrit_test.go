/*
Copyright 2022 The Kubernetes Authors.

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
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/andygrunwald/go-gerrit"
	"github.com/google/go-cmp/cmp"
	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/types"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/tide/blockers"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ gerritClient = (*fakeGerritClient)(nil)

type fakeGerritClient struct {
	// map{org: map{project: []changes}}
	changes map[string]map[string][]gerrit.ChangeInfo
}

func newFakeGerritClient() *fakeGerritClient {
	return &fakeGerritClient{
		changes: make(map[string]map[string][]gerrit.ChangeInfo),
	}
}

func (f *fakeGerritClient) QueryChangesForProject(instance, project string, lastUpdate time.Time, rateLimit int, addtionalFilters ...string) ([]gerrit.ChangeInfo, error) {
	if f.changes == nil || f.changes[instance] == nil || f.changes[instance][project] == nil {
		return nil, errors.New("queries project doesn't exist")
	}

	return f.changes[instance][project], nil
}

func (f *fakeGerritClient) GetChange(instance, id string, addtionalFields ...string) (*gerrit.ChangeInfo, error) {
	if f.changes == nil || f.changes[instance] == nil {
		return nil, errors.New("instance not exist")
	}
	for _, prs := range f.changes[instance] {
		for _, pr := range prs {
			if pr.ID == id {
				return &pr, nil
			}
		}
	}
	return nil, errors.New("change not exist")
}

func (f *fakeGerritClient) GetBranchRevision(instance, project, branch string) (string, error) {
	return "abc", nil
}

func (f *fakeGerritClient) SubmitChange(instance, id string, wait bool) (*gerrit.ChangeInfo, error) {
	return f.GetChange(instance, id)
}

func (f *fakeGerritClient) SetReview(instance, id, revision, message string, _ map[string]string) error {
	change, err := f.GetChange(instance, id)
	if err != nil {
		return fmt.Errorf("change not found: %v", err)
	}
	revNum, err := strconv.Atoi(revision)
	if err != nil {
		return fmt.Errorf("failed converting revision '%s' to int: %v", revision, err)
	}
	change.Messages = append(change.Messages, gerrit.ChangeMessageInfo{
		RevisionNumber: revNum,
		Message:        message,
	})

	return nil
}

func (f *fakeGerritClient) addChanges(instance, project string, changes []gerrit.ChangeInfo) {
	if _, ok := f.changes[instance]; !ok {
		f.changes[instance] = make(map[string][]gerrit.ChangeInfo)
	}
	if _, ok := f.changes[instance][project]; !ok {
		f.changes[instance][project] = []gerrit.ChangeInfo{}
	}
	f.changes[instance][project] = append(f.changes[instance][project], changes...)
}

func TestGerritQueryParam(t *testing.T) {
	tests := []struct {
		name  string
		optIn bool
		want  string
	}{
		{
			name:  "default",
			optIn: false,
			want:  "status:open+-is:wip+is:submittable+-label:Prow-Auto-Submit=-1+label:Prow-Auto-Submit",
		},
		{
			name:  "opt-in",
			optIn: true,
			want:  "status:open+-is:wip+is:submittable+-label:Prow-Auto-Submit=-1",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if want, got := tc.want, gerritQueryParam(tc.optIn); want != got {
				t.Errorf("Wrong query param. Want: %s, got: %s", want, got)
			}
		})
	}
}

func TestQuery(t *testing.T) {
	tests := []struct {
		name    string
		queries config.GerritOrgRepoConfigs
		prs     map[string]map[string][]gerrit.ChangeInfo
		expect  map[string]CodeReviewCommon
		wantErr bool
	}{
		{
			name: "single",
			queries: config.GerritOrgRepoConfigs{
				{
					Org:   "foo1",
					Repos: []string{"bar1"},
				},
			},
			prs: map[string]map[string][]gerrit.ChangeInfo{
				"foo1": {
					"bar1": {
						gerrit.ChangeInfo{
							Number:  1,
							Project: "bar1",
						},
					},
				},
			},
			expect: map[string]CodeReviewCommon{
				"foo1/bar1#1": *CodeReviewCommonFromGerrit(&gerrit.ChangeInfo{Number: 1, Project: "bar1"}, "foo1"),
			},
		},
		{
			name: "multiple",
			queries: config.GerritOrgRepoConfigs{
				{
					Org:   "foo1",
					Repos: []string{"bar1", "bar2"},
				},
				{
					Org:   "foo2",
					Repos: []string{"bar3", "bar4"},
				},
			},
			prs: map[string]map[string][]gerrit.ChangeInfo{
				"foo1": {
					"bar1": {
						gerrit.ChangeInfo{
							Number:  1,
							Project: "bar1",
						},
					},
					"bar2": {
						gerrit.ChangeInfo{
							Number:  2,
							Project: "bar2",
						},
					},
				},
				"foo2": {
					"bar3": {
						gerrit.ChangeInfo{
							Number:  1,
							Project: "bar3",
						},
					},
					"bar4": {
						gerrit.ChangeInfo{
							Number:  2,
							Project: "bar4",
						},
					},
				},
			},
			expect: map[string]CodeReviewCommon{
				"foo1/bar1#1": *CodeReviewCommonFromGerrit(&gerrit.ChangeInfo{Number: 1, Project: "bar1"}, "foo1"),
				"foo1/bar2#2": *CodeReviewCommonFromGerrit(&gerrit.ChangeInfo{Number: 2, Project: "bar2"}, "foo1"),
				"foo2/bar3#1": *CodeReviewCommonFromGerrit(&gerrit.ChangeInfo{Number: 1, Project: "bar3"}, "foo2"),
				"foo2/bar4#2": *CodeReviewCommonFromGerrit(&gerrit.ChangeInfo{Number: 2, Project: "bar4"}, "foo2"),
			},
		},
		{
			name: "not-configured",
			queries: config.GerritOrgRepoConfigs{
				{
					Org:   "foo5",
					Repos: []string{"bar1", "bar2"},
				},
				{
					Org:   "foo6",
					Repos: []string{"bar3", "bar4"},
				},
			},
			prs: map[string]map[string][]gerrit.ChangeInfo{
				"foo1": {
					"bar1": {
						gerrit.ChangeInfo{
							Number:  1,
							Project: "bar1",
						},
					},
					"bar2": {
						gerrit.ChangeInfo{
							Number:  2,
							Project: "bar2",
						},
					},
				},
				"foo2": {
					"bar3": {
						gerrit.ChangeInfo{
							Number:  1,
							Project: "bar3",
						},
					},
					"bar4": {
						gerrit.ChangeInfo{
							Number:  2,
							Project: "bar4",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "no-pr",
			queries: config.GerritOrgRepoConfigs{
				{
					Org:   "foo1",
					Repos: []string{"bar1"},
				},
			},
			prs: map[string]map[string][]gerrit.ChangeInfo{
				"foo1": {
					"bar1": {},
				},
			},
			expect: map[string]CodeReviewCommon{},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{
				ProwConfig: config.ProwConfig{
					Tide: config.Tide{
						Gerrit: &config.TideGerritConfig{
							Queries: tc.queries,
						},
					},
				},
			}

			fc := newGerritProvider(logrus.WithContext(context.Background()), func() *config.Config { return &cfg }, nil, nil, "", "", 0, 0)
			fgc := newFakeGerritClient()

			for instance, projs := range tc.prs {
				for project, changes := range projs {
					fgc.addChanges(instance, project, changes)
				}
			}
			fc.gc = fgc

			got, err := fc.Query()
			if (tc.wantErr && err == nil) || (!tc.wantErr && err != nil) {
				t.Fatalf("Error mismatch. Want: %v, got: %v", tc.wantErr, err)
			}
			if diff := cmp.Diff(tc.expect, got); diff != "" {
				t.Fatalf("Query result mismatch. Want(-), got(+):\n%s", diff)
			}
		})
	}
}

func TestBlocker(t *testing.T) {
	fc := &GerritProvider{}
	want := blockers.Blockers{}
	var wantErr error
	got, gotErr := fc.blockers()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Blocker mismatch. Want(-), got(+):\n%s", diff)
	}
	if wantErr != gotErr {
		t.Errorf("Error mismatch. Want: %v, got: %v", wantErr, gotErr)
	}
}

func TestIsAllowedToMerge(t *testing.T) {
	tests := []struct {
		name      string
		mergeable string
		want      string
		wantErr   error
	}{
		{
			name:      "conflict",
			mergeable: string(githubql.MergeableStateConflicting),
			want:      "PR has a merge conflict.",
		},
		{
			name:      "normal",
			mergeable: string(githubql.MergeableStateMergeable),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fc := &GerritProvider{}
			got, gotErr := fc.isAllowedToMerge(&CodeReviewCommon{Mergeable: tc.mergeable})

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Blocker mismatch. Want(-), got(+):\n%s", diff)
			}
			if tc.wantErr != gotErr {
				t.Errorf("Error mismatch. Want: %v, got: %v", tc.wantErr, gotErr)
			}
		})
	}
}

func TestGetRef(t *testing.T) {
	fgc := newFakeGerritClient()
	fc := &GerritProvider{gc: fgc}
	got, _ := fc.GetRef("", "", "")

	want := "abc"
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Blocker mismatch. Want(-), got(+):\n%s", diff)
	}
}

func TestGerritHeadContexts(t *testing.T) {
	tests := []struct {
		name    string
		jobs    []prowapi.ProwJob
		want    []Context
		wantErr error
	}{
		{
			name: "normal",
			jobs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "not-important-1",
						Namespace: "prowjobs",
						Labels: map[string]string{
							kube.GerritRevision:   "abc123",
							kube.ProwJobTypeLabel: string(prowapi.PresubmitJob),
							kube.OrgLabel:         "foo1",
							kube.RepoLabel:        "bar1",
							kube.PullLabel:        "1",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Type:    prowapi.PresubmitJob,
						Job:     "job-1",
						Context: "job-1",
						Refs: &prowapi.Refs{
							BaseSHA: "def123",
						},
					},
					Status: prowapi.ProwJobStatus{
						State:       prowapi.SuccessState,
						Description: "desc",
					},
				},
			},
			want: []Context{
				{
					Context:     "job-1",
					Description: "desc\u2001\u2001\u2001\u2001\u2001\u2001\u2001\u2001\u2001\u2001\u2001\u2001\u2001\u2001\u2001\u2001\u2001\u2001\u2001\u2001\u2001\xe2\x80\x81\u2001\u2001\u2001\u2001\u2001\u2001\u2001\u2001\u2001 BaseSHA:def123",
					State:       "success",
				},
			},
		},
		{
			name: "periodic",
			jobs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "not-important-1",
						Namespace: "prowjobs",
						Labels: map[string]string{
							kube.GerritRevision:   "abc123",
							kube.ProwJobTypeLabel: string(prowapi.PeriodicJob),
							kube.OrgLabel:         "foo1",
							kube.RepoLabel:        "bar1",
							kube.PullLabel:        "1",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Type:    prowapi.PeriodicJob,
						Job:     "job-1",
						Context: "job-1",
						Refs: &prowapi.Refs{
							BaseSHA: "def123",
						},
					},
					Status: prowapi.ProwJobStatus{
						State:       prowapi.SuccessState,
						Description: "desc",
					},
				},
			},
		},
		{
			name: "wrong-org",
			jobs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "not-important-1",
						Namespace: "prowjobs",
						Labels: map[string]string{
							kube.GerritRevision:   "abc123",
							kube.ProwJobTypeLabel: string(prowapi.PresubmitJob),
							kube.OrgLabel:         "foo2",
							kube.RepoLabel:        "bar1",
							kube.PullLabel:        "1",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Type:    prowapi.PresubmitJob,
						Job:     "job-1",
						Context: "job-1",
						Refs: &prowapi.Refs{
							BaseSHA: "def123",
						},
					},
					Status: prowapi.ProwJobStatus{
						State:       prowapi.SuccessState,
						Description: "desc",
					},
				},
			},
		},
		{
			name: "wrong-repo",
			jobs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "not-important-1",
						Namespace: "prowjobs",
						Labels: map[string]string{
							kube.GerritRevision:   "abc123",
							kube.ProwJobTypeLabel: string(prowapi.PresubmitJob),
							kube.OrgLabel:         "foo1",
							kube.RepoLabel:        "bar2",
							kube.PullLabel:        "1",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Type:    prowapi.PresubmitJob,
						Job:     "job-1",
						Context: "job-1",
						Refs: &prowapi.Refs{
							BaseSHA: "def123",
						},
					},
					Status: prowapi.ProwJobStatus{
						State:       prowapi.SuccessState,
						Description: "desc",
					},
				},
			},
		},
		{
			name: "wrong-revision",
			jobs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "not-important-1",
						Namespace: "prowjobs",
						Labels: map[string]string{
							kube.GerritRevision:   "abc456",
							kube.ProwJobTypeLabel: string(prowapi.PresubmitJob),
							kube.OrgLabel:         "foo1",
							kube.RepoLabel:        "bar1",
							kube.PullLabel:        "1",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Type:    prowapi.PresubmitJob,
						Job:     "job-1",
						Context: "job-1",
						Refs: &prowapi.Refs{
							BaseSHA: "def123",
						},
					},
					Status: prowapi.ProwJobStatus{
						State:       prowapi.SuccessState,
						Description: "desc",
					},
				},
			},
		},
		{
			name: "wrong-pull",
			jobs: []prowapi.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "not-important-1",
						Namespace: "prowjobs",
						Labels: map[string]string{
							kube.GerritRevision:   "abc123",
							kube.ProwJobTypeLabel: string(prowapi.PresubmitJob),
							kube.OrgLabel:         "foo1",
							kube.RepoLabel:        "bar1",
							kube.PullLabel:        "2",
						},
					},
					Spec: prowapi.ProwJobSpec{
						Type:    prowapi.PresubmitJob,
						Job:     "job-1",
						Context: "job-1",
						Refs: &prowapi.Refs{
							BaseSHA: "def123",
						},
					},
					Status: prowapi.ProwJobStatus{
						State:       prowapi.SuccessState,
						Description: "desc",
					},
				},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var jobs []runtime.Object
			for _, job := range tc.jobs {
				job := job
				complete := metav1.NewTime(time.Now().Add(-time.Millisecond))
				if job.Status.State != prowapi.PendingState && job.Status.State != prowapi.TriggeredState {
					job.Status.CompletionTime = &complete
				}
				jobs = append(jobs, &job)
			}

			fpjc := fakectrlruntimeclient.NewFakeClient(jobs...)
			fc := &GerritProvider{pjclientset: fpjc}

			got, gotErr := fc.headContexts(&CodeReviewCommon{
				HeadRefOID: "abc123",
				Org:        "foo1",
				Repo:       "bar1",
				Number:     1,
			})

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Blocker mismatch. Want(-), got(+):\n%s", diff)
			}
			if tc.wantErr != gotErr {
				t.Errorf("Error mismatch. Want: %v, got: %v", tc.wantErr, gotErr)
			}
		})
	}
}

func TestMergePR(t *testing.T) {
	tests := []struct {
		name          string
		subpool       subpool
		clientChanges map[string]map[string][]gerrit.ChangeInfo
		prs           []gerrit.ChangeInfo
		wantErr       error
	}{
		{
			name: "single",
			subpool: subpool{
				org:  "org",
				repo: "repo",
			},
			clientChanges: map[string]map[string][]gerrit.ChangeInfo{
				"org": {
					"repo": {
						{
							ID: "abc123",
						},
					},
				},
			},
			prs: []gerrit.ChangeInfo{
				{
					ID: "abc123",
				},
			},
			wantErr: nil,
		},
		{
			name: "multiple",
			subpool: subpool{
				org:  "org",
				repo: "repo",
			},
			clientChanges: map[string]map[string][]gerrit.ChangeInfo{
				"org": {
					"repo": {
						{
							ID: "abc123",
						},
						{
							ID: "def456",
						},
					},
				},
			},
			prs: []gerrit.ChangeInfo{
				{
					ID: "abc123",
				},
				{
					ID: "def456",
				},
			},
			wantErr: nil,
		},
		{
			name: "single-error",
			subpool: subpool{
				org:  "org",
				repo: "repo",
			},
			// Empty changes results in SubmitChange error.
			clientChanges: map[string]map[string][]gerrit.ChangeInfo{},
			prs: []gerrit.ChangeInfo{
				{
					ID: "abc123",
				},
			},
			wantErr: errors.New("failed submitting change 'org' from org 'abc123': instance not exist"),
		},
		{
			name: "multiple-error",
			subpool: subpool{
				org:  "org",
				repo: "repo",
			},
			// Empty changes results in SubmitChange error.
			clientChanges: map[string]map[string][]gerrit.ChangeInfo{},
			prs: []gerrit.ChangeInfo{
				{
					ID: "abc123",
				},
				{
					ID: "def456",
				},
			},
			wantErr: errors.New("[failed submitting change 'org' from org 'abc123': instance not exist, failed submitting change 'org' from org 'def456': instance not exist]"),
		},
		{
			name: "partial-error",
			subpool: subpool{
				org:  "org",
				repo: "repo",
			},
			clientChanges: map[string]map[string][]gerrit.ChangeInfo{
				"org": {
					"repo": {
						{
							ID: "abc123",
						},
					},
				},
			},
			prs: []gerrit.ChangeInfo{
				{
					ID: "abc123",
				},
				{
					ID: "def456",
				},
			},
			wantErr: errors.New("failed submitting change 'org' from org 'def456': change not exist"),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fgc := newFakeGerritClient()
			fgc.changes = tc.clientChanges
			cfg := config.Config{
				ProwConfig: config.ProwConfig{
					Gerrit: config.Gerrit{
						DeckURL: "http://foo.bar",
					},
				},
			}
			fc := &GerritProvider{
				logger: logrus.WithContext(context.Background()),
				gc:     fgc,
				cfg:    func() *config.Config { return &cfg },
			}

			var prsToMerge []CodeReviewCommon
			for _, pr := range tc.prs {
				prsToMerge = append(prsToMerge, *CodeReviewCommonFromGerrit(&pr, tc.subpool.org))
			}

			_, gotErr := fc.mergePRs(tc.subpool, prsToMerge, nil)
			if tc.wantErr == nil {
				if gotErr != nil {
					t.Fatalf("Error mismatch. Want nil, got: %v", gotErr)
				}
				return
			}
			if gotErr == nil {
				t.Fatalf("Error mismatch. Want %v, got nil", tc.wantErr)
			}
			if tc.wantErr.Error() != gotErr.Error() {
				t.Fatalf("Error not matching. Want: %v, got: %v", tc.wantErr, gotErr)
			}
		})
	}
}

func TestGetTideContextPolicy(t *testing.T) {
	tests := []struct {
		name       string
		pr         gerrit.ChangeInfo
		cloneURI   string
		presubmits map[string][]config.Presubmit
		want       contextChecker
		wantErr    error
	}{
		{
			name: "base",
			pr: gerrit.ChangeInfo{
				Project:         "bar1",
				Branch:          "main",
				CurrentRevision: "abc123",
				Labels: map[string]gerrit.LabelInfo{
					"Verified": {
						Optional: false,
					},
				},
			},
			presubmits: map[string][]config.Presubmit{
				"https://foo1/bar1": {
					{
						Reporter: config.Reporter{Context: "job-1"},
						JobBase: config.JobBase{
							Labels: map[string]string{
								"prow.k8s.io/gerrit-report-label": "Verified",
							},
						},
					},
				},
			},
			want: &gerritContextChecker{},
		},
		{
			name: "no-job",
			pr: gerrit.ChangeInfo{
				Project:         "bar1",
				Branch:          "main",
				CurrentRevision: "abc123",
				Labels: map[string]gerrit.LabelInfo{
					"Verified": {
						Optional: false,
					},
				},
			},
			presubmits: map[string][]config.Presubmit{
				"https://foo1/bar1": {},
			},
			want: &gerritContextChecker{},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{JobConfig: config.JobConfig{PresubmitsStatic: tc.presubmits}}
			fc := &GerritProvider{cfg: func() *config.Config { return &cfg }}

			got, gotErr := fc.GetTideContextPolicy("foo1", tc.pr.Project, tc.pr.Branch, nil, CodeReviewCommonFromGerrit(&tc.pr, "foo1"))

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Blocker mismatch. Want(-), got(+):\n%s", diff)
			}
			if tc.wantErr != gotErr {
				t.Errorf("Error mismatch. Want: %v, got: %v", tc.wantErr, gotErr)
			}
		})
	}
}

func TestPrMergeMethod(t *testing.T) {
	tests := []struct {
		name string
		pr   gerrit.ChangeInfo
		want types.PullRequestMergeType
	}{
		{
			name: "MERGE_IF_NECESSARY",
			pr: gerrit.ChangeInfo{
				SubmitType: "MERGE_IF_NECESSARY",
			},
			want: types.MergeIfNecessary,
		},
		{
			name: "FAST_FORWARD_ONLY",
			pr: gerrit.ChangeInfo{
				SubmitType: "FAST_FORWARD_ONLY",
			},
			want: types.MergeMerge,
		},
		{
			name: "REBASE_IF_NECESSARY",
			pr: gerrit.ChangeInfo{
				SubmitType: "REBASE_IF_NECESSARY",
			},
			want: types.MergeRebase,
		},
		{
			name: "REBASE_ALWAYS",
			pr: gerrit.ChangeInfo{
				SubmitType: "REBASE_ALWAYS",
			},
			want: types.MergeRebase,
		},
		{
			name: "MERGE_ALWAYS",
			pr: gerrit.ChangeInfo{
				SubmitType: "MERGE_ALWAYS",
			},
			want: types.MergeMerge,
		},
		{
			name: "NOT_EXIST",
			pr: gerrit.ChangeInfo{
				SubmitType: "NOT_EXIST",
			},
			want: types.MergeMerge,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fc := &GerritProvider{}

			got := fc.prMergeMethod(CodeReviewCommonFromGerrit(&tc.pr, "foo1"))
			if got == nil {
				t.Error("Multiple conflicting merge methods assigned.")
			} else {
				if diff := cmp.Diff(tc.want, *got); diff != "" {
					t.Errorf("Blocker mismatch. Want(-), got(+):\n%s", diff)
				}
			}
		})
	}
}

func TestJobIsRequiredByTide(t *testing.T) {
	tests := []struct {
		name string
		ps   *config.Presubmit
		crc  *CodeReviewCommon
		want bool
	}{
		{
			name: "default",
			ps:   &config.Presubmit{},
			crc: &CodeReviewCommon{
				Gerrit: &gerrit.ChangeInfo{Labels: map[string]gerrit.LabelInfo{}},
			},
		},
		{
			name: "run-before-merge",
			ps:   &config.Presubmit{RunBeforeMerge: true},
			crc: &CodeReviewCommon{
				Gerrit: &gerrit.ChangeInfo{Labels: map[string]gerrit.LabelInfo{}},
			},
			want: true,
		},
		{
			name: "required-label",
			ps: &config.Presubmit{JobBase: config.JobBase{Labels: map[string]string{
				"prow.k8s.io/gerrit-report-label": "Verified-By-Prow",
			}}},
			crc: &CodeReviewCommon{
				Gerrit: &gerrit.ChangeInfo{Labels: map[string]gerrit.LabelInfo{"Verified-By-Prow": {}}},
			},
			want: true,
		},
		{
			name: "optional-label",
			ps: &config.Presubmit{JobBase: config.JobBase{Labels: map[string]string{
				"prow.k8s.io/gerrit-report-label": "Verified-By-Prow",
			}}},
			crc: &CodeReviewCommon{
				Gerrit: &gerrit.ChangeInfo{Labels: map[string]gerrit.LabelInfo{"Verified-By-Prow": {Optional: true}}},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fc := &GerritProvider{}
			if want, got := tc.want, fc.jobIsRequiredByTide(tc.ps, tc.crc); want != got {
				t.Errorf("Wrong. Want: %v, got: %v", want, got)
			}
		})
	}
}
