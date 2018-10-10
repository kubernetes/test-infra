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
	"fmt"
	"strings"
	"testing"

	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
)

func TestExpectedStatus(t *testing.T) {
	neededLabels := []string{"need-1", "need-2", "need-a-very-super-duper-extra-not-short-at-all-label-name"}
	forbiddenLabels := []string{"forbidden-1", "forbidden-2"}
	testcases := []struct {
		name string

		baseref         string
		branchWhitelist []string
		branchBlacklist []string
		sameBranchReqs  bool
		labels          []string
		milestone       string
		contexts        []Context
		inPool          bool

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
			name:      "check truncation of label list",
			milestone: "v1.0",
			inPool:    false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Needs need-1, need-2 labels."),
		},
		{
			name:      "check truncation of label list is not excessive",
			labels:    append([]string{}, neededLabels[:2]...),
			milestone: "v1.0",
			inPool:    false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Needs need-a-very-super-duper-extra-not-short-at-all-label-name label."),
		},
		{
			name:      "has forbidden labels",
			labels:    append(append([]string{}, neededLabels...), forbiddenLabels...),
			milestone: "v1.0",
			inPool:    false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Should not have forbidden-1, forbidden-2 labels."),
		},
		{
			name:      "has one forbidden label",
			labels:    append(append([]string{}, neededLabels...), forbiddenLabels[0]),
			milestone: "v1.0",
			inPool:    false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Should not have forbidden-1 label."),
		},
		{
			name:      "only mention one requirement class",
			labels:    append(append([]string{}, neededLabels[1:]...), forbiddenLabels[0]),
			milestone: "v1.0",
			inPool:    false,

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
			name:            "choose query for correct branch",
			baseref:         "bad",
			branchWhitelist: []string{"good"},
			milestone:       "v1.0",
			labels:          neededLabels,
			inPool:          false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Needs 1, 2, 3, 4, 5, 6, 7 labels."),
		},
		{
			name:      "only failed tide context",
			labels:    neededLabels,
			milestone: "v1.0",
			contexts:  []Context{{Context: githubql.String(statusContext), State: githubql.StatusStateError}},
			inPool:    false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, ""),
		},
		{
			name:      "single bad context",
			labels:    neededLabels,
			contexts:  []Context{{Context: githubql.String("job-name"), State: githubql.StatusStateError}},
			milestone: "v1.0",
			inPool:    false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Job job-name has not succeeded."),
		},
		{
			name:      "multiple bad contexts",
			labels:    neededLabels,
			milestone: "v1.0",
			contexts: []Context{
				{Context: githubql.String("job-name"), State: githubql.StatusStateError},
				{Context: githubql.String("other-job-name"), State: githubql.StatusStateError},
			},
			inPool: false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Jobs job-name, other-job-name have not succeeded."),
		},
		{
			name:      "wrong milestone",
			labels:    neededLabels,
			milestone: "v1.1",
			contexts:  []Context{{Context: githubql.String("job-name"), State: githubql.StatusStateSuccess}},
			inPool:    false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Must be in milestone v1.0."),
		},
		{
			name:      "unknown requirement",
			labels:    neededLabels,
			milestone: "v1.0",
			contexts:  []Context{{Context: githubql.String("job-name"), State: githubql.StatusStateSuccess}},
			inPool:    false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, ""),
		},
		{
			name:      "check that min diff query is used",
			labels:    []string{"3", "4", "5", "6", "7"},
			milestone: "v1.0",
			inPool:    false,

			state: github.StatusPending,
			desc:  fmt.Sprintf(statusNotInPool, " Needs 1, 2 labels."),
		},
	}

	for _, tc := range testcases {
		t.Logf("Test Case: %q\n", tc.name)
		secondQuery := config.TideQuery{
			Orgs:      []string{""},
			Labels:    []string{"1", "2", "3", "4", "5", "6", "7"}, // lots of requirements
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
		if len(tc.contexts) > 0 {
			pr.HeadRefOID = githubql.String("head")
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
		if tc.milestone != "" {
			pr.Milestone = &struct {
				Title githubql.String
			}{githubql.String(tc.milestone)}
		}
		var pool map[string]PullRequest
		if tc.inPool {
			pool = map[string]PullRequest{"#0": {}}
		}

		state, desc := expectedStatus(queriesByRepo, &pr, pool, &config.TideContextPolicy{})
		if state != tc.state {
			t.Errorf("Expected status state %q, but got %q.", string(tc.state), string(state))
		}
		if desc != tc.desc {
			t.Errorf("Expected status description %q, but got %q.", tc.desc, desc)
		}
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

		sc := &statusController{ghc: fc, ca: ca, logger: log}
		sc.setStatuses([]PullRequest{pr}, pool)
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
			config:      config.Tide{TargetURL: "tide.com", PRStatusBaseURL: "pr.status.com"},
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
			config:      config.Tide{PRStatusBaseURL: "pr.status.com"},
			expectedURL: "pr.status.com?query=is%3Apr+repo%3Aorg%2Frepo+author%3Aauthor+head%3Ahead",
		},
	}

	for _, tc := range testcases {
		ca := &config.Agent{}
		ca.Set(&config.Config{ProwConfig: config.ProwConfig{Tide: tc.config}})
		log := logrus.WithField("controller", "status-update")
		if actual, expected := targetURL(ca, tc.pr, log), tc.expectedURL; actual != expected {
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
