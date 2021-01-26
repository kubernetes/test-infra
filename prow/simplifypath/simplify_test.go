/*
Copyright 2019 The Kubernetes Authors.

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

package simplifypath

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/diff"
)

func TestLiteral(t *testing.T) {
	l := L("fragment", Node{})
	if !l.Matches("fragment") {
		t.Errorf("expected literal to match fragment, but didn't")
	}
	if actual, expected := l.Represent(), "fragment"; actual != expected {
		t.Errorf("expected literal to be represented by %v, but saw: %v", expected, actual)
	}
}

func TestEmptyLiteral(t *testing.T) {
	l := L("", Node{})
	if !l.Matches(strings.Split("/", "/")[0]) {
		t.Errorf("expected empty literal to match root, but didn't")
	}
	if actual, expected := l.Represent(), ""; actual != expected {
		t.Errorf("expected empty literal to be represented by %v, but saw: %v", expected, actual)
	}
}

func TestVariable(t *testing.T) {
	l := V("variable", Node{})
	if !l.Matches("variable") {
		t.Errorf("expected variable to match itself, but didn't")
	}
	if !l.Matches("askdljfhasdjfas") {
		t.Errorf("expected variable to match random string, but didn't")
	}
	if actual, expected := l.Represent(), ":variable"; actual != expected {
		t.Errorf("expected literal to be represented by %v, but saw: %v", expected, actual)
	}
}

func TestSimplify(t *testing.T) {
	s := NewSimplifier(L("", // shadow element mimicing the root
		L(""),
		L("repos",
			V("owner",
				V("repo",
					L("branches", V("branch", L("protection",
						L("restrictions", L("users"), L("teams")),
						L("required_status_checks", L("contexts")),
						L("required_pull_request_reviews"),
						L("required_signatures"),
						L("enforce_admins"))),
					),
				),
			),
		),
	))

	var testCases = []struct {
		name, path, expected string
	}{
		{
			name:     "root",
			path:     "/",
			expected: "/"},
		{
			name:     "repo branches",
			path:     "/repos/testOwner/testRepo/branches",
			expected: "/repos/:owner/:repo/branches"},
		{
			name:     "repo branches by name",
			path:     "/repos/testOwner/testRepo/branches/testBranch",
			expected: "/repos/:owner/:repo/branches/:branch"},
		{
			name:     "repo branches protection by name ",
			path:     "/repos/testOwner/testRepo/branches/testBranch/protection",
			expected: "/repos/:owner/:repo/branches/:branch/protection"},
		{
			name:     "repo branches protection (required status checks) by name ",
			path:     "/repos/testOwner/testRepo/branches/testBranch/protection/required_status_checks",
			expected: "/repos/:owner/:repo/branches/:branch/protection/required_status_checks"},
		{
			name:     "repo branches protection (required status checks, contexts) by name ",
			path:     "/repos/testOwner/testRepo/branches/testBranch/protection/required_status_checks/contexts",
			expected: "/repos/:owner/:repo/branches/:branch/protection/required_status_checks/contexts"},
		{
			name:     "repo branches protection (required pull request reviews) by name ",
			path:     "/repos/testOwner/testRepo/branches/testBranch/protection/required_pull_request_reviews",
			expected: "/repos/:owner/:repo/branches/:branch/protection/required_pull_request_reviews"},
		{
			name:     "repo branches protection (required signatures) by name ",
			path:     "/repos/testOwner/testRepo/branches/testBranch/protection/required_signatures",
			expected: "/repos/:owner/:repo/branches/:branch/protection/required_signatures"},
		{
			name:     "repo branches protection (enforce admins) by name ",
			path:     "/repos/testOwner/testRepo/branches/testBranch/protection/enforce_admins",
			expected: "/repos/:owner/:repo/branches/:branch/protection/enforce_admins"},
		{
			name:     "repo branches protection (restrictions) by name ",
			path:     "/repos/testOwner/testRepo/branches/testBranch/protection/restrictions",
			expected: "/repos/:owner/:repo/branches/:branch/protection/restrictions"},
		{
			name:     "repo branches protection (restrictions for teams) by name ",
			path:     "/repos/testOwner/testRepo/branches/testBranch/protection/restrictions/teams",
			expected: "/repos/:owner/:repo/branches/:branch/protection/restrictions/teams"},
		{
			name:     "repo branches protection (restrictions for users) by name ",
			path:     "/repos/testOwner/testRepo/branches/testBranch/protection/restrictions/users",
			expected: "/repos/:owner/:repo/branches/:branch/protection/restrictions/users"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if actual, expected := s.Simplify(testCase.path), testCase.expected; actual != expected {
				t.Errorf("%s: got incorrect simplification: %v", testCase.name, diff.StringDiff(actual, expected))
			}
		})
	}
}
