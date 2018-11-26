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

package clone

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/test-infra/prow/kube"
)

func TestPathForRefs(t *testing.T) {
	var testCases = []struct {
		name     string
		refs     kube.Refs
		expected string
	}{
		{
			name: "literal override",
			refs: kube.Refs{
				PathAlias: "alias",
			},
			expected: "base/src/alias",
		},
		{
			name: "default generated",
			refs: kube.Refs{
				Org:  "org",
				Repo: "repo",
			},
			expected: "base/src/github.com/org/repo",
		},
	}

	for _, testCase := range testCases {
		if actual, expected := PathForRefs("base", testCase.refs), testCase.expected; actual != expected {
			t.Errorf("%s: expected path %q, got %q", testCase.name, expected, actual)
		}
	}
}

func TestCommandsForRefs(t *testing.T) {
	var testCases = []struct {
		name                                       string
		refs                                       kube.Refs
		dir, gitUserName, gitUserEmail, cookiePath string
		env                                        []string
		expected                                   []cloneCommand
	}{
		{
			name: "simplest case, minimal refs",
			refs: kube.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
			},
			dir: "/go",
			expected: []cloneCommand{
				{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "minimal refs with git user name",
			refs: kube.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
			},
			gitUserName: "user",
			dir:         "/go",
			expected: []cloneCommand{
				{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"config", "user.name", "user"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "minimal refs with git user email",
			refs: kube.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
			},
			gitUserEmail: "user@go.com",
			dir:          "/go",
			expected: []cloneCommand{
				{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"config", "user.email", "user@go.com"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "minimal refs with http cookie file",
			refs: kube.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
			},
			cookiePath: "/cookie.txt",
			dir:        "/go",
			expected: []cloneCommand{
				{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"config", "http.cookiefile", "/cookie.txt"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "minimal refs with no submodules",
			refs: kube.Refs{
				Org:            "org",
				Repo:           "repo",
				BaseRef:        "master",
				SkipSubmodules: true,
			},
			dir: "/go",
			expected: []cloneCommand{
				{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
			},
		},
		{
			name: "refs with clone URI override",
			refs: kube.Refs{
				Org:      "org",
				Repo:     "repo",
				BaseRef:  "master",
				CloneURI: "internet.com",
			},
			dir: "/go",
			expected: []cloneCommand{
				{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "internet.com", "--tags", "--prune"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "internet.com", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "refs with path alias",
			refs: kube.Refs{
				Org:       "org",
				Repo:      "repo",
				BaseRef:   "master",
				PathAlias: "my/favorite/dir",
			},
			dir: "/go",
			expected: []cloneCommand{
				{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/my/favorite/dir"}},
				{dir: "/go/src/my/favorite/dir", command: "git", args: []string{"init"}},
				{dir: "/go/src/my/favorite/dir", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
				{dir: "/go/src/my/favorite/dir", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
				{dir: "/go/src/my/favorite/dir", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				{dir: "/go/src/my/favorite/dir", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				{dir: "/go/src/my/favorite/dir", command: "git", args: []string{"checkout", "master"}},
				{dir: "/go/src/my/favorite/dir", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "refs with specific base sha",
			refs: kube.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
				BaseSHA: "abcdef",
			},
			dir: "/go",
			expected: []cloneCommand{
				{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "abcdef"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "abcdef"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "refs with simple pr ref",
			refs: kube.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
				Pulls: []kube.Pull{
					{Number: 1},
				},
			},
			dir: "/go",
			expected: []cloneCommand{
				{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "pull/1/head"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"merge", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "refs with pr ref override",
			refs: kube.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
				Pulls: []kube.Pull{
					{Number: 1, Ref: "pull-me"},
				},
			},
			dir: "/go",
			expected: []cloneCommand{
				{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "pull-me"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"merge", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "refs with pr ref with specific sha",
			refs: kube.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
				Pulls: []kube.Pull{
					{Number: 1, SHA: "abcdef"},
				},
			},
			dir: "/go",
			expected: []cloneCommand{
				{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "pull/1/head"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"merge", "abcdef"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "refs with multiple simple pr refs",
			refs: kube.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
				Pulls: []kube.Pull{
					{Number: 1},
					{Number: 2},
				},
			},
			dir: "/go",
			expected: []cloneCommand{
				{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "pull/1/head"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"merge", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "pull/2/head"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"merge", "FETCH_HEAD"}},
				{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
	}

	for _, testCase := range testCases {
		if actual, expected := commandsForRefs(testCase.refs, testCase.dir, testCase.gitUserName, testCase.gitUserEmail, testCase.cookiePath, testCase.env), testCase.expected; !reflect.DeepEqual(actual, expected) {
			t.Errorf("%s: generated incorrect commands: %v", testCase.name, diff.ObjectGoPrintDiff(expected, actual))
		}
	}
}
