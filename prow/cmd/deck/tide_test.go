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

package main

import (
	"reflect"
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/equality"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/tide"
	"k8s.io/test-infra/prow/tide/history"
)

func TestFilterHidden(t *testing.T) {
	tests := []struct {
		name string

		hiddenRepos []string
		hiddenOnly  bool
		showHidden  bool
		queries     []config.TideQuery
		pools       []tide.Pool
		hist        map[string][]history.Record

		expectedQueries []config.TideQuery
		expectedPools   []tide.Pool
		expectedHist    map[string][]history.Record
	}{
		{
			name: "public frontend",

			hiddenRepos: []string{
				"kubernetes-security",
				"kubernetes/website",
			},
			hiddenOnly: false,
			queries: []config.TideQuery{
				{
					Repos: []string{"kubernetes/test-infra", "kubernetes/kubernetes"},
				},
				{
					Repos: []string{"kubernetes/website", "kubernetes/docs"},
				},
				{
					Repos: []string{"kubernetes/apiserver", "kubernetes-security/apiserver"},
				},
			},
			pools: []tide.Pool{
				{Org: "kubernetes", Repo: "test-infra"},
				{Org: "kubernetes", Repo: "kubernetes"},
				{Org: "kubernetes", Repo: "website"},
				{Org: "kubernetes", Repo: "docs"},
				{Org: "kubernetes", Repo: "apiserver"},
				{Org: "kubernetes-security", Repo: "apiserver"},
			},
			hist: map[string][]history.Record{
				"kubernetes/test-infra:master":         {{Action: "MERGE"}, {Action: "TRIGGER"}},
				"kubernetes/website:master":            {{Action: "MERGE_BATCH"}, {Action: "TRIGGER_BATCH"}},
				"kubernetes-security/apiserver:master": {{Action: "TRIGGER"}, {Action: "MERGE"}},
				"kubernetes/kubernetes:master":         {{Action: "TRIGGER_BATCH"}, {Action: "MERGE_BATCH"}},
			},

			expectedQueries: []config.TideQuery{
				{
					Repos: []string{"kubernetes/test-infra", "kubernetes/kubernetes"},
				},
			},
			expectedPools: []tide.Pool{
				{Org: "kubernetes", Repo: "test-infra"},
				{Org: "kubernetes", Repo: "kubernetes"},
				{Org: "kubernetes", Repo: "docs"},
				{Org: "kubernetes", Repo: "apiserver"},
			},
			expectedHist: map[string][]history.Record{
				"kubernetes/test-infra:master": {{Action: "MERGE"}, {Action: "TRIGGER"}},
				"kubernetes/kubernetes:master": {{Action: "TRIGGER_BATCH"}, {Action: "MERGE_BATCH"}},
			},
		},
		{
			name: "private frontend",

			hiddenRepos: []string{
				"kubernetes-security",
				"kubernetes/website",
			},
			hiddenOnly: true,
			queries: []config.TideQuery{
				{
					Repos: []string{"kubernetes/test-infra", "kubernetes/kubernetes"},
				},
				{
					Repos: []string{"kubernetes/website", "kubernetes/docs"},
				},
				{
					Repos: []string{"kubernetes/apiserver", "kubernetes-security/apiserver"},
				},
			},
			pools: []tide.Pool{
				{Org: "kubernetes", Repo: "test-infra"},
				{Org: "kubernetes", Repo: "kubernetes"},
				{Org: "kubernetes", Repo: "website"},
				{Org: "kubernetes", Repo: "docs"},
				{Org: "kubernetes", Repo: "apiserver"},
				{Org: "kubernetes-security", Repo: "apiserver"},
			},
			hist: map[string][]history.Record{
				"kubernetes/test-infra:master":         {{Action: "MERGE"}, {Action: "TRIGGER"}},
				"kubernetes/website:master":            {{Action: "MERGE_BATCH"}, {Action: "TRIGGER_BATCH"}},
				"kubernetes-security/apiserver:master": {{Action: "TRIGGER"}, {Action: "MERGE"}},
				"kubernetes/kubernetes:master":         {{Action: "TRIGGER_BATCH"}, {Action: "MERGE_BATCH"}},
			},

			expectedQueries: []config.TideQuery{
				{
					Repos: []string{"kubernetes/website", "kubernetes/docs"},
				},
				{
					Repos: []string{"kubernetes/apiserver", "kubernetes-security/apiserver"},
				},
			},
			expectedPools: []tide.Pool{
				{Org: "kubernetes", Repo: "website"},
				{Org: "kubernetes-security", Repo: "apiserver"},
			},
			expectedHist: map[string][]history.Record{
				"kubernetes/website:master":            {{Action: "MERGE_BATCH"}, {Action: "TRIGGER_BATCH"}},
				"kubernetes-security/apiserver:master": {{Action: "TRIGGER"}, {Action: "MERGE"}},
			},
		},
		{
			name: "frontend for everything",

			showHidden: true,
			hiddenRepos: []string{
				"kubernetes-security",
				"kubernetes/website",
			},

			pools: []tide.Pool{
				{Org: "kubernetes", Repo: "test-infra"},
				{Org: "kubernetes", Repo: "kubernetes"},
				{Org: "kubernetes", Repo: "website"},
				{Org: "kubernetes", Repo: "docs"},
				{Org: "kubernetes", Repo: "apiserver"},
				{Org: "kubernetes-security", Repo: "apiserver"},
			},
			expectedPools: []tide.Pool{
				{Org: "kubernetes", Repo: "test-infra"},
				{Org: "kubernetes", Repo: "kubernetes"},
				{Org: "kubernetes", Repo: "website"},
				{Org: "kubernetes", Repo: "docs"},
				{Org: "kubernetes", Repo: "apiserver"},
				{Org: "kubernetes-security", Repo: "apiserver"},
			},

			queries: []config.TideQuery{
				{
					Repos: []string{"kubernetes/test-infra", "kubernetes/kubernetes"},
				},
				{
					Repos: []string{"kubernetes/website", "kubernetes/docs"},
				},
				{
					Repos: []string{"kubernetes/apiserver", "kubernetes-security/apiserver"},
				},
			},
			expectedQueries: []config.TideQuery{
				{
					Repos: []string{"kubernetes/test-infra", "kubernetes/kubernetes"},
				},
				{
					Repos: []string{"kubernetes/website", "kubernetes/docs"},
				},
				{
					Repos: []string{"kubernetes/apiserver", "kubernetes-security/apiserver"},
				},
			},

			hist: map[string][]history.Record{
				"kubernetes/test-infra:master":         {{Action: "MERGE"}, {Action: "TRIGGER"}},
				"kubernetes/website:master":            {{Action: "MERGE_BATCH"}, {Action: "TRIGGER_BATCH"}},
				"kubernetes-security/apiserver:master": {{Action: "TRIGGER"}, {Action: "MERGE"}},
				"kubernetes/kubernetes:master":         {{Action: "TRIGGER_BATCH"}, {Action: "MERGE_BATCH"}},
			},
			expectedHist: map[string][]history.Record{
				"kubernetes/test-infra:master":         {{Action: "MERGE"}, {Action: "TRIGGER"}},
				"kubernetes/website:master":            {{Action: "MERGE_BATCH"}, {Action: "TRIGGER_BATCH"}},
				"kubernetes-security/apiserver:master": {{Action: "TRIGGER"}, {Action: "MERGE"}},
				"kubernetes/kubernetes:master":         {{Action: "TRIGGER_BATCH"}, {Action: "MERGE_BATCH"}},
			},
		},
	}

	for _, test := range tests {
		t.Logf("running scenario %q", test.name)

		ta := &tideAgent{
			hiddenRepos: func() []string {
				return test.hiddenRepos
			},
			hiddenOnly: test.hiddenOnly,
			showHidden: test.showHidden,
			log:        logrus.WithField("agent", "tide"),
		}

		gotQueries := ta.filterHiddenQueries(test.queries)
		gotPools := ta.filterHiddenPools(test.pools)
		gotHist := ta.filterHiddenHistory(test.hist)
		if !equality.Semantic.DeepEqual(gotQueries, test.expectedQueries) {
			t.Errorf("expected queries:\n%v\ngot queries:\n%v\n", test.expectedQueries, gotQueries)
		}
		if !equality.Semantic.DeepEqual(gotPools, test.expectedPools) {
			t.Errorf("expected pools:\n%v\ngot pools:\n%v\n", test.expectedPools, gotPools)
		}
		// equality.Semantic.DeepEqual doesn't like the unexported fields in time.Time.
		// We don't care about that for this test.
		if !reflect.DeepEqual(gotHist, test.expectedHist) {
			t.Errorf("expected history:\n%v\ngot history:\n%v\n", test.expectedHist, gotHist)
		}
	}
}

func TestMatches(t *testing.T) {
	tests := []struct {
		name string

		repo  string
		repos []string

		expected bool
	}{
		{
			name: "repo exists - exact match",

			repo: "kubernetes/test-infra",
			repos: []string{
				"kubernetes/kubernetes",
				"kubernetes/test-infra",
				"kubernetes/community",
			},

			expected: true,
		},
		{
			name: "repo exists - org match",

			repo: "kubernetes/test-infra",
			repos: []string{
				"openshift/test-infra",
				"openshift/origin",
				"kubernetes-security",
				"kubernetes",
			},

			expected: true,
		},
		{
			name: "repo does not exist",

			repo: "kubernetes/website",
			repos: []string{
				"openshift/test-infra",
				"openshift/origin",
				"kubernetes-security",
				"kubernetes/test-infra",
				"kubernetes/kubernetes",
			},

			expected: false,
		},
	}

	for _, test := range tests {
		t.Logf("running scenario %q", test.name)

		if got := matches(test.repo, test.repos); got != test.expected {
			t.Errorf("unexpected result: expected %t, got %t", test.expected, got)
		}
	}
}
