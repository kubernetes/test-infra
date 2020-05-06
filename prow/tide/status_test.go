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
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/go-test/deep"
	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"k8s.io/apimachinery/pkg/util/sets"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/tide/blockers"
)

func TestExpectedStatus(t *testing.T) {
	neededLabels := []string{"need-1", "need-2", "need-a-very-super-duper-extra-not-short-at-all-label-name"}
	forbiddenLabels := []string{"forbidden-1", "forbidden-2"}
	testcases := []struct {
		name string

		baseref           string
		branchWhitelist   []string
		branchBlacklist   []string
		sameBranchReqs    bool
		labels            []string
		author            string
		firstQueryAuthor  string
		secondQueryAuthor string
		milestone         string
		contexts          []Context
		inPool            bool
		blocks            []int
		prowJobs          []runtime.Object
		requiredContexts  []string
		mergeConflicts    bool

		state string
		desc  string
	}{
		{
			name:   "in pool",
			inPool: true,

			state: github.StatusSuccess,
			desc:  statusInPool,
		},
		{
			name:              "check truncation of label list",
			author:            "batman",
			firstQueryAuthor:  "batman",
			secondQueryAuthor: "batman",
			milestone:         "v1.0",
			inPool:            false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Needs need-1, need-2 labels."),
		},
		{
			name:              "check truncation of label list is not excessive",
			labels:            append([]string{}, neededLabels[:2]...),
			author:            "batman",
			firstQueryAuthor:  "batman",
			secondQueryAuthor: "batman",
			milestone:         "v1.0",
			inPool:            false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Needs need-a-very-super-duper-extra-not-short-at-all-label-name label."),
		},
		{
			name:              "has forbidden labels",
			labels:            append(append([]string{}, neededLabels...), forbiddenLabels...),
			author:            "batman",
			firstQueryAuthor:  "batman",
			secondQueryAuthor: "batman",
			milestone:         "v1.0",
			inPool:            false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Should not have forbidden-1, forbidden-2 labels."),
		},
		{
			name:              "has one forbidden label",
			labels:            append(append([]string{}, neededLabels...), forbiddenLabels[0]),
			author:            "batman",
			firstQueryAuthor:  "batman",
			secondQueryAuthor: "batman",
			milestone:         "v1.0",
			inPool:            false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Should not have forbidden-1 label."),
		},
		{
			name:              "only mention one requirement class",
			labels:            append(append([]string{}, neededLabels[1:]...), forbiddenLabels[0]),
			author:            "batman",
			firstQueryAuthor:  "batman",
			secondQueryAuthor: "batman",
			milestone:         "v1.0",
			inPool:            false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Needs need-1 label."),
		},
		{
			name:            "against excluded branch",
			baseref:         "bad",
			branchBlacklist: []string{"bad"},
			sameBranchReqs:  true,
			labels:          neededLabels,
			inPool:          false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Merging to branch bad is forbidden."),
		},
		{
			name:            "not against included branch",
			baseref:         "bad",
			branchWhitelist: []string{"good"},
			sameBranchReqs:  true,
			labels:          neededLabels,
			inPool:          false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Merging to branch bad is forbidden."),
		},
		{
			name:              "choose query for correct branch",
			baseref:           "bad",
			branchWhitelist:   []string{"good"},
			author:            "batman",
			firstQueryAuthor:  "batman",
			secondQueryAuthor: "batman",
			milestone:         "v1.0",
			labels:            neededLabels,
			inPool:            false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Needs 1, 2, 3, 4, 5, 6, 7 labels."),
		},
		{
			name:              "only failed tide context",
			labels:            neededLabels,
			author:            "batman",
			firstQueryAuthor:  "batman",
			secondQueryAuthor: "batman",
			milestone:         "v1.0",
			contexts:          []Context{{Context: githubql.String(statusContext), State: githubql.StatusStateError}},
			inPool:            false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, ""),
		},
		{
			name:              "single bad context",
			labels:            neededLabels,
			contexts:          []Context{{Context: githubql.String("job-name"), State: githubql.StatusStateError}},
			author:            "batman",
			firstQueryAuthor:  "batman",
			secondQueryAuthor: "batman",
			milestone:         "v1.0",
			inPool:            false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Job job-name has not succeeded."),
		},
		{
			name:              "multiple bad contexts",
			labels:            neededLabels,
			author:            "batman",
			firstQueryAuthor:  "batman",
			secondQueryAuthor: "batman",
			milestone:         "v1.0",
			contexts: []Context{
				{Context: githubql.String("job-name"), State: githubql.StatusStateError},
				{Context: githubql.String("other-job-name"), State: githubql.StatusStateError},
			},
			inPool: false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Jobs job-name, other-job-name have not succeeded."),
		},
		{
			name:              "wrong author",
			labels:            neededLabels,
			author:            "robin",
			firstQueryAuthor:  "batman",
			secondQueryAuthor: "batman",
			milestone:         "v1.0",
			contexts:          []Context{{Context: githubql.String("job-name"), State: githubql.StatusStateSuccess}},
			inPool:            false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Must be by author batman."),
		},
		{
			name:              "wrong author; use lowest diff",
			labels:            neededLabels,
			author:            "robin",
			firstQueryAuthor:  "penguin",
			secondQueryAuthor: "batman",
			milestone:         "v1.0",
			contexts:          []Context{{Context: githubql.String("job-name"), State: githubql.StatusStateSuccess}},
			inPool:            false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Must be by author penguin."),
		},
		{
			name:              "wrong milestone",
			labels:            neededLabels,
			author:            "batman",
			firstQueryAuthor:  "batman",
			secondQueryAuthor: "batman",
			milestone:         "v1.1",
			contexts:          []Context{{Context: githubql.String("job-name"), State: githubql.StatusStateSuccess}},
			inPool:            false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Must be in milestone v1.0."),
		},
		{
			name:              "unknown requirement",
			labels:            neededLabels,
			author:            "batman",
			firstQueryAuthor:  "batman",
			secondQueryAuthor: "batman",
			milestone:         "v1.0",
			contexts:          []Context{{Context: githubql.String("job-name"), State: githubql.StatusStateSuccess}},
			inPool:            false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, ""),
		},
		{
			name:              "check that min diff query is used",
			labels:            []string{"3", "4", "5", "6", "7"},
			author:            "batman",
			firstQueryAuthor:  "batman",
			secondQueryAuthor: "batman",
			milestone:         "v1.0",
			inPool:            false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Needs 1, 2 labels."),
		},
		{
			name:              "check that blockers take precedence over other queries",
			labels:            []string{"3", "4", "5", "6", "7"},
			author:            "batman",
			firstQueryAuthor:  "batman",
			secondQueryAuthor: "batman",
			milestone:         "v1.0",
			inPool:            false,
			blocks:            []int{1, 2},

			state: github.StatusError,
			desc:  fmt.Sprintf(statusNotInPool, " Merging is blocked by issues 1, 2."),
		},
		{
			name:             "missing passing up-to-date context",
			inPool:           true,
			baseref:          "baseref",
			requiredContexts: []string{"foo", "bar"},
			prowJobs: []runtime.Object{
				&prowapi.ProwJob{
					ObjectMeta: metav1.ObjectMeta{Name: "123"},
					Spec: prowapi.ProwJobSpec{
						Context: "foo",
						Refs: &prowapi.Refs{
							BaseSHA: "baseref",
							Pulls:   []prowapi.Pull{{SHA: "head"}},
						},
						Type: prowapi.PresubmitJob,
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.SuccessState,
					},
				},
				&prowapi.ProwJob{
					ObjectMeta: metav1.ObjectMeta{Name: "1234"},
					Spec: prowapi.ProwJobSpec{
						Context: "bar",
						Refs: &prowapi.Refs{
							BaseSHA: "baseref",
							Pulls:   []prowapi.Pull{{SHA: "head"}},
						},
						Type: prowapi.PresubmitJob,
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.PendingState,
					},
				},
			},

			state: github.StatusPending,
			desc:  "Not mergeable. Retesting: bar",
		},
		{
			name:             "missing passing up-to-date contexts",
			inPool:           true,
			baseref:          "baseref",
			requiredContexts: []string{"foo", "bar", "baz"},
			prowJobs: []runtime.Object{
				&prowapi.ProwJob{
					ObjectMeta: metav1.ObjectMeta{Name: "123"},
					Spec: prowapi.ProwJobSpec{
						Context: "foo",
						Refs: &prowapi.Refs{
							BaseSHA: "baseref",
							Pulls:   []prowapi.Pull{{SHA: "head"}},
						},
						Type: prowapi.PresubmitJob,
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.SuccessState,
					},
				},
				&prowapi.ProwJob{
					ObjectMeta: metav1.ObjectMeta{Name: "1234"},
					Spec: prowapi.ProwJobSpec{
						Context: "bar",
						Refs: &prowapi.Refs{
							BaseSHA: "baseref",
							Pulls:   []prowapi.Pull{{SHA: "head"}},
						},
						Type: prowapi.PresubmitJob,
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.PendingState,
					},
				},
				&prowapi.ProwJob{
					ObjectMeta: metav1.ObjectMeta{Name: "12345"},
					Spec: prowapi.ProwJobSpec{
						Context: "baz",
						Refs: &prowapi.Refs{
							BaseSHA: "baseref",
							Pulls:   []prowapi.Pull{{SHA: "head"}},
						},
						Type: prowapi.PresubmitJob,
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.PendingState,
					},
				},
			},

			state: github.StatusPending,
			desc:  "Not mergeable. Retesting: bar baz",
		},
		{
			name:             "missing passing up-to-date contexts with different ordering",
			inPool:           true,
			baseref:          "baseref",
			requiredContexts: []string{"foo", "bar", "baz"},
			prowJobs: []runtime.Object{
				&prowapi.ProwJob{
					ObjectMeta: metav1.ObjectMeta{Name: "123"},
					Spec: prowapi.ProwJobSpec{
						Context: "foo",
						Refs: &prowapi.Refs{
							BaseSHA: "baseref",
							Pulls:   []prowapi.Pull{{SHA: "head"}},
						},
						Type: prowapi.PresubmitJob,
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.SuccessState,
					},
				},
				&prowapi.ProwJob{
					ObjectMeta: metav1.ObjectMeta{Name: "1234"},
					Spec: prowapi.ProwJobSpec{
						Context: "baz",
						Refs: &prowapi.Refs{
							BaseSHA: "baseref",
							Pulls:   []prowapi.Pull{{SHA: "head"}},
						},
						Type: prowapi.PresubmitJob,
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.PendingState,
					},
				},
				&prowapi.ProwJob{
					ObjectMeta: metav1.ObjectMeta{Name: "12345"},
					Spec: prowapi.ProwJobSpec{
						Context: "bar",
						Refs: &prowapi.Refs{
							BaseSHA: "baseref",
							Pulls:   []prowapi.Pull{{SHA: "head"}},
						},
						Type: prowapi.PresubmitJob,
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.PendingState,
					},
				},
			},

			state: github.StatusPending,
			desc:  "Not mergeable. Retesting: bar baz",
		},
		{
			name:    "long list of not up-to-date contexts results in shortened message",
			inPool:  true,
			baseref: "baseref",
			requiredContexts: []string{
				strings.Repeat("very-long-context", 8),
				strings.Repeat("also-long-content", 8),
			},
			prowJobs: []runtime.Object{
				&prowapi.ProwJob{
					ObjectMeta: metav1.ObjectMeta{Name: "123"},
					Spec: prowapi.ProwJobSpec{
						Context: strings.Repeat("very-long-context", 8),
						Refs: &prowapi.Refs{
							BaseSHA: "baseref",
							Pulls:   []prowapi.Pull{{SHA: "head"}},
						},
						Type: prowapi.PresubmitJob,
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.PendingState,
					},
				},
				&prowapi.ProwJob{
					ObjectMeta: metav1.ObjectMeta{Name: "1234"},
					Spec: prowapi.ProwJobSpec{
						Context: strings.Repeat("also-long-content", 8),
						Refs: &prowapi.Refs{
							BaseSHA: "baseref",
							Pulls:   []prowapi.Pull{{SHA: "head"}},
						},
						Type: prowapi.PresubmitJob,
					},
					Status: prowapi.ProwJobStatus{
						State: prowapi.PendingState,
					},
				},
			},

			state: github.StatusPending,
			desc:  "Not mergeable. Retesting 2 jobs.",
		},
		{
			name:           "mergeconflicts",
			inPool:         true,
			mergeConflicts: true,
			state:          github.StatusError,
			desc:           "Not mergeable. PR has a merge conflict.",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			secondQuery := config.TideQuery{
				Orgs:      []string{""},
				Labels:    []string{"1", "2", "3", "4", "5", "6", "7"}, // lots of requirements
				Author:    tc.secondQueryAuthor,
				Milestone: "v1.0",
			}
			if tc.sameBranchReqs {
				secondQuery.ExcludedBranches = tc.branchBlacklist
				secondQuery.IncludedBranches = tc.branchWhitelist
			}
			queriesByRepo := config.TideQueries{
				config.TideQuery{
					Orgs:             []string{""},
					ExcludedBranches: tc.branchBlacklist,
					IncludedBranches: tc.branchWhitelist,
					Labels:           neededLabels,
					MissingLabels:    forbiddenLabels,
					Author:           tc.firstQueryAuthor,
					Milestone:        "v1.0",
				},
				secondQuery,
			}.QueryMap()
			var pr PullRequest
			pr.BaseRef = struct {
				Name   githubql.String
				Prefix githubql.String
			}{
				Name: githubql.String(tc.baseref),
			}
			for _, label := range tc.labels {
				pr.Labels.Nodes = append(
					pr.Labels.Nodes,
					struct{ Name githubql.String }{Name: githubql.String(label)},
				)
			}
			pr.HeadRefOID = githubql.String("head")
			if len(tc.contexts) > 0 {
				pr.Commits.Nodes = append(
					pr.Commits.Nodes,
					struct{ Commit Commit }{
						Commit: Commit{
							Status: struct{ Contexts []Context }{
								Contexts: tc.contexts,
							},
							OID: githubql.String("head"),
						},
					},
				)
			}
			pr.Author = struct {
				Login githubql.String
			}{githubql.String(tc.author)}
			if tc.milestone != "" {
				pr.Milestone = &struct {
					Title githubql.String
				}{githubql.String(tc.milestone)}
			}
			if tc.mergeConflicts {
				pr.Mergeable = githubql.MergeableStateConflicting
			}
			var pool map[string]PullRequest
			if tc.inPool {
				pool = map[string]PullRequest{"#0": {}}
			}
			blocks := blockers.Blockers{
				Repo: map[blockers.OrgRepo][]blockers.Blocker{},
			}
			var items []blockers.Blocker
			for _, block := range tc.blocks {
				items = append(items, blockers.Blocker{Number: block})
			}
			blocks.Repo[blockers.OrgRepo{Org: "", Repo: ""}] = items

			ca := &config.Agent{}
			ca.Set(&config.Config{})
			mmc := newMergeChecker(ca.Config, &fgc{})

			sc, err := newStatusController(logrus.NewEntry(logrus.StandardLogger()), nil, newFakeManager(tc.prowJobs...), nil, nil, nil, "", mmc)
			if err != nil {
				t.Fatalf("failed to get statusController: %v", err)
			}
			ccg := func() (contextChecker, error) {
				return &config.TideContextPolicy{RequiredContexts: tc.requiredContexts}, nil
			}
			state, desc, err := sc.expectedStatus(sc.logger, queriesByRepo, &pr, pool, ccg, blocks, tc.baseref)
			if err != nil {
				t.Fatalf("error calling expectedStatus(): %v", err)
			}
			if state != tc.state {
				t.Errorf("Expected status state %q, but got %q.", string(tc.state), string(state))
			}
			if desc != tc.desc {
				t.Errorf("Expected status description %q, but got %q.", tc.desc, desc)
			}
		})
	}
}

func TestSetStatuses(t *testing.T) {
	statusNotInPoolEmpty := fmt.Sprintf(statusNotInPool, "")
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
			desc:       statusNotInPoolEmpty,

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
			desc:       statusNotInPoolEmpty,

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
		pool := make(map[string]PullRequest)
		if tc.inPool {
			pool[prKey(&pr)] = pr
		}
		fc := &fgc{
			refs: map[string]string{"/ heads/": "SHA"},
		}
		ca := &config.Agent{}
		ca.Set(&config.Config{})
		// setStatuses logs instead of returning errors.
		// Construct a logger to watch for errors to be printed.
		log := logrus.WithField("component", "tide")
		initialLog, err := log.String()
		if err != nil {
			t.Fatalf("Failed to get log output before testing: %v", err)
		}

		mmc := newMergeChecker(ca.Config, fc)
		sc, err := newStatusController(log, fc, newFakeManager(), nil, ca.Config, nil, "", mmc)
		if err != nil {
			t.Fatalf("failed to get statusController: %v", err)
		}
		sc.setStatuses([]PullRequest{pr}, pool, blockers.Blockers{}, nil, nil)
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

func TestTargetUrl(t *testing.T) {
	testcases := []struct {
		name   string
		pr     *PullRequest
		config config.Tide

		expectedURL string
	}{
		{
			name:        "no config",
			pr:          &PullRequest{},
			config:      config.Tide{},
			expectedURL: "",
		},
		{
			name:        "tide overview config",
			pr:          &PullRequest{},
			config:      config.Tide{TargetURL: "tide.com"},
			expectedURL: "tide.com",
		},
		{
			name:        "PR dashboard config and overview config",
			pr:          &PullRequest{},
			config:      config.Tide{TargetURL: "tide.com", PRStatusBaseURLs: map[string]string{"*": "pr.status.com"}},
			expectedURL: "tide.com",
		},
		{
			name: "PR dashboard config",
			pr: &PullRequest{
				Author: struct {
					Login githubql.String
				}{Login: githubql.String("author")},
				Repository: struct {
					Name          githubql.String
					NameWithOwner githubql.String
					Owner         struct {
						Login githubql.String
					}
				}{NameWithOwner: githubql.String("org/repo")},
				HeadRefName: "head",
			},
			config:      config.Tide{PRStatusBaseURLs: map[string]string{"*": "pr.status.com"}},
			expectedURL: "pr.status.com?query=is%3Apr+repo%3Aorg%2Frepo+author%3Aauthor+head%3Ahead",
		},
		{
			name: "generate link by default config",
			pr: &PullRequest{
				Author: struct {
					Login githubql.String
				}{Login: githubql.String("author")},
				Repository: struct {
					Name          githubql.String
					NameWithOwner githubql.String
					Owner         struct {
						Login githubql.String
					}
				}{
					Owner:         struct{ Login githubql.String }{Login: githubql.String("testOrg")},
					Name:          githubql.String("testRepo"),
					NameWithOwner: githubql.String("testOrg/testRepo"),
				},
				HeadRefName: "head",
			},
			config:      config.Tide{PRStatusBaseURLs: map[string]string{"*": "default.pr.status.com"}},
			expectedURL: "default.pr.status.com?query=is%3Apr+repo%3AtestOrg%2FtestRepo+author%3Aauthor+head%3Ahead",
		},
		{
			name: "generate link by org config",
			pr: &PullRequest{
				Author: struct {
					Login githubql.String
				}{Login: githubql.String("author")},
				Repository: struct {
					Name          githubql.String
					NameWithOwner githubql.String
					Owner         struct {
						Login githubql.String
					}
				}{
					Owner:         struct{ Login githubql.String }{Login: githubql.String("testOrg")},
					Name:          githubql.String("testRepo"),
					NameWithOwner: githubql.String("testOrg/testRepo"),
				},
				HeadRefName: "head",
			},
			config:      config.Tide{PRStatusBaseURLs: map[string]string{"testOrg": "byorg.pr.status.com"}},
			expectedURL: "byorg.pr.status.com?query=is%3Apr+repo%3AtestOrg%2FtestRepo+author%3Aauthor+head%3Ahead",
		},
		{
			name: "generate link by repo config",
			pr: &PullRequest{
				Author: struct {
					Login githubql.String
				}{Login: githubql.String("author")},
				Repository: struct {
					Name          githubql.String
					NameWithOwner githubql.String
					Owner         struct {
						Login githubql.String
					}
				}{
					Owner:         struct{ Login githubql.String }{Login: githubql.String("testOrg")},
					Name:          githubql.String("testRepo"),
					NameWithOwner: githubql.String("testOrg/testRepo"),
				},
				HeadRefName: "head",
			},
			config: config.Tide{PRStatusBaseURLs: map[string]string{
				"testOrg":          "byorg.pr.status.com",
				"testOrg/testRepo": "byrepo.pr.status.com"},
			},
			expectedURL: "byrepo.pr.status.com?query=is%3Apr+repo%3AtestOrg%2FtestRepo+author%3Aauthor+head%3Ahead",
		},
	}

	for _, tc := range testcases {
		log := logrus.WithField("controller", "status-update")
		c := &config.Config{ProwConfig: config.ProwConfig{Tide: tc.config}}
		if actual, expected := targetURL(c, tc.pr, log), tc.expectedURL; actual != expected {
			t.Errorf("%s: expected target URL %s but got %s", tc.name, expected, actual)
		}
	}
}

func TestOpenPRsQuery(t *testing.T) {
	var q string
	checkTok := func(tok string) {
		if !strings.Contains(q, " "+tok+" ") {
			t.Errorf("Expected query to contain \"%s\", got \"%s\"", tok, q)
		}
	}

	orgs := []string{"org", "kuber"}
	repos := []string{"k8s/k8s", "k8s/t-i"}
	exceptions := map[string]sets.String{
		"org":            sets.NewString("org/repo1", "org/repo2"),
		"irrelevant-org": sets.NewString("irrelevant-org/repo1", "irrelevant-org/repo2"),
	}

	q = " " + openPRsQuery(orgs, repos, exceptions) + " "
	checkTok("is:pr")
	checkTok("state:open")
	checkTok("org:\"org\"")
	checkTok("org:\"kuber\"")
	checkTok("repo:\"k8s/k8s\"")
	checkTok("repo:\"k8s/t-i\"")
	checkTok("-repo:\"org/repo1\"")
	checkTok("-repo:\"org/repo2\"")
}

func TestIndexFuncPassingJobs(t *testing.T) {
	testCases := []struct {
		name     string
		pj       *prowapi.ProwJob
		expected []string
	}{
		{
			name: "Jobs that are not presubmit or batch are ignored",
			pj:   getProwJob(prowapi.PeriodicJob, "org", "", "repo", "baseSHA", prowapi.SuccessState, []prowapi.Pull{{SHA: "head"}}),
		},
		{
			name: "Non-Passing jobs are ignored",
			pj:   getProwJob(prowapi.PresubmitJob, "org", "repo", "", "baseSHA", prowapi.FailureState, []prowapi.Pull{{SHA: "head"}}),
		},
		{
			name:     "Indexkey is returned for presubmit job",
			pj:       getProwJob(prowapi.PresubmitJob, "org", "repo", "", "baseSHA", prowapi.SuccessState, []prowapi.Pull{{SHA: "head"}}),
			expected: []string{"org/repo@baseSHA+head"},
		},
		{
			name:     "Indexkeys are returned for batch job",
			pj:       getProwJob(prowapi.BatchJob, "org", "repo", "", "baseSHA", prowapi.SuccessState, []prowapi.Pull{{SHA: "head"}, {SHA: "head-2"}}),
			expected: []string{"org/repo@baseSHA+head", "org/repo@baseSHA+head-2"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var results []string
			for _, result := range indexFuncPassingJobs(tc.pj) {
				results = append(results, result)
			}
			if diff := deep.Equal(tc.expected, results); diff != nil {
				t.Errorf("expected does not match result, diff: %v", diff)
			}
		})
	}
}

func TestSetStatusRespectsRequiredContexts(t *testing.T) {
	var pr PullRequest
	pr.Commits.Nodes = []struct{ Commit Commit }{{}}
	pr.Repository.NameWithOwner = githubql.String("org/repo")
	pr.Number = githubql.Int(2)
	requiredContexts := map[string][]string{"org/repo#2": {"foo", "bar"}}

	fghc := &fgc{
		refs: map[string]string{"/ heads/": "SHA"},
	}
	log := logrus.WithField("component", "tide")
	initialLog, err := log.String()
	if err != nil {
		t.Fatalf("Failed to get log output before testing: %v", err)
	}

	ca := &config.Agent{}
	ca.Set(&config.Config{})

	sc := &statusController{
		logger:       log,
		ghc:          fghc,
		config:       ca.Config,
		pjClient:     fakectrlruntimeclient.NewFakeClient(),
		mergeChecker: newMergeChecker(ca.Config, fghc),
	}
	pool := map[string]PullRequest{prKey(&pr): pr}
	sc.setStatuses([]PullRequest{pr}, pool, blockers.Blockers{}, nil, requiredContexts)
	if str, err := log.String(); err != nil {
		t.Fatalf("Failed to get log output: %v", err)
	} else if str != initialLog {
		t.Errorf("Error setting status: %s", str)
	}

	if n := len(fghc.statuses); n != 1 {
		t.Fatalf("expected exactly one status to be set, got %d", n)
	}

	expectedDescription := "Not mergeable. Retesting: bar foo"
	val, exists := fghc.statuses["//"]
	if !exists {
		t.Fatal("Status didn't get set")
	}
	if val.Description != expectedDescription {
		t.Errorf("Expected description to be %q, was %q", expectedDescription, val.Description)
	}
}

func TestNewBaseSHAGetter(t *testing.T) {
	org, repo, branch := "org", "repo", "branch"
	testCases := []struct {
		name     string
		baseSHAs map[string]string
		ghc      githubClient

		expectedSHA string
		expectErr   bool
	}{
		{
			name:        "Default to content of baseSHAs map",
			baseSHAs:    map[string]string{"org/repo:branch": "123"},
			expectedSHA: "123",
		},
		{
			name:        "BaseSHAs map has no entry, ask GitHub",
			baseSHAs:    map[string]string{},
			ghc:         &fgc{refs: map[string]string{"org/repo heads/branch": "SHA"}},
			expectedSHA: "SHA",
		},
		{
			name:      "Error is returned",
			baseSHAs:  map[string]string{},
			ghc:       &fgc{err: errors.New("some-failure")},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := newBaseSHAGetter(tc.baseSHAs, tc.ghc, org, repo, branch)()
			if err != nil && !tc.expectErr {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.expectErr {
				return
			}
			if result != tc.expectedSHA {
				t.Errorf("expected %q, got %q", tc.expectedSHA, result)
			}
			if val := tc.baseSHAs[org+"/"+repo+":"+branch]; val != tc.expectedSHA {
				t.Errorf("baseSHA in the map (%q) does not match expected(%q)", val, tc.expectedSHA)
			}
		})
	}
}
