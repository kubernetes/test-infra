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

package config

import (
	"strings"
	"testing"

	"k8s.io/test-infra/prow/github"
)

func TestTideQuery(t *testing.T) {
	tq := &TideQuery{
		Repos:                  []string{"k/k", "k/t-i"},
		Labels:                 []string{"lgtm", "approved"},
		MissingLabels:          []string{"foo"},
		ReviewApprovedRequired: true,
	}
	q := " " + tq.Query() + " "
	checkTok := func(tok string) {
		if !strings.Contains(q, " "+tok+" ") {
			t.Errorf("Expected query to contain \"%s\", got \"%s\"", tok, q)
		}
	}
	checkTok("is:pr")
	checkTok("state:open")
	checkTok("repo:\"k/k\"")
	checkTok("repo:\"k/t-i\"")
	checkTok("label:\"lgtm\"")
	checkTok("label:\"approved\"")
	checkTok("-label:\"foo\"")
	checkTok("review:approved")
}

func TestMergeMethod(t *testing.T) {
	ti := &Tide{
		MergeType: map[string]github.PullRequestMergeType{
			"kubernetes/kops":             github.MergeRebase,
			"kubernetes/charts":           github.MergeSquash,
			"kubernetes-helm":             github.MergeSquash,
			"kubernetes-helm/chartmuseum": github.MergeMerge,
		},
	}

	var testcases = []struct {
		org      string
		repo     string
		expected github.PullRequestMergeType
	}{
		{
			"kubernetes",
			"kubernetes",
			github.MergeMerge,
		},
		{
			"kubernetes",
			"kops",
			github.MergeRebase,
		},
		{
			"kubernetes",
			"charts",
			github.MergeSquash,
		},
		{
			"kubernetes-helm",
			"monocular",
			github.MergeSquash,
		},
		{
			"kubernetes-helm",
			"chartmuseum",
			github.MergeMerge,
		},
	}

	for _, test := range testcases {
		if ti.MergeMethod(test.org, test.repo) != test.expected {
			t.Errorf("Expected merge method %q but got %q for %s/%s", test.expected, ti.MergeMethod(test.org, test.repo), test.org, test.repo)
		}
	}
}
