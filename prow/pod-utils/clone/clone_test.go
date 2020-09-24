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
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/diff"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func TestPathForRefs(t *testing.T) {
	var testCases = []struct {
		name     string
		refs     prowapi.Refs
		expected string
	}{
		{
			name: "literal override",
			refs: prowapi.Refs{
				PathAlias: "alias",
			},
			expected: "base/src/alias",
		},
		{
			name: "default generated",
			refs: prowapi.Refs{
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
	fakeTimestamp := 100200300
	var testCases = []struct {
		name                                       string
		refs                                       prowapi.Refs
		dir, gitUserName, gitUserEmail, cookiePath string
		env                                        []string
		expectedBase                               []runnable
		expectedPull                               []runnable
		oauthToken                                 string
	}{
		{
			name: "simplest case, minimal refs",
			refs: prowapi.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
			},
			dir: "/go",
			expectedBase: []runnable{
				cloneCommand{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
					fetchRetries,
				},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
			},
			expectedPull: []runnable{
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "simple case, root dir",
			refs: prowapi.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
			},
			dir: "/",
			expectedBase: []runnable{
				cloneCommand{dir: "/", command: "mkdir", args: []string{"-p", "/src/github.com/org/repo"}},
				cloneCommand{dir: "/src/github.com/org/repo", command: "git", args: []string{"init"}},
				retryCommand{
					cloneCommand{dir: "/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
					fetchRetries,
				},
				retryCommand{
					cloneCommand{dir: "/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
					fetchRetries,
				},
				cloneCommand{dir: "/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				cloneCommand{dir: "/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				cloneCommand{dir: "/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
			},
			expectedPull: []runnable{
				cloneCommand{dir: "/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "minimal refs with git user name",
			refs: prowapi.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
			},
			gitUserName: "user",
			dir:         "/go",
			expectedBase: []runnable{
				cloneCommand{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"config", "user.name", "user"}},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
					fetchRetries,
				},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
			},
			expectedPull: []runnable{
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "minimal refs with git user email",
			refs: prowapi.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
			},
			gitUserEmail: "user@go.com",
			dir:          "/go",
			expectedBase: []runnable{
				cloneCommand{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"config", "user.email", "user@go.com"}},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
					fetchRetries,
				},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
			},
			expectedPull: []runnable{
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "minimal refs with http cookie file (skip submodules)",
			refs: prowapi.Refs{
				Org:            "org",
				Repo:           "repo",
				BaseRef:        "master",
				SkipSubmodules: true,
			},
			cookiePath: "/cookie.txt",
			dir:        "/go",
			expectedBase: []runnable{
				cloneCommand{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"config", "http.cookiefile", "/cookie.txt"}},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
					fetchRetries,
				},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
			},
		},
		{
			name: "minimal refs with http cookie file",
			refs: prowapi.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
			},
			cookiePath: "/cookie.txt",
			dir:        "/go",
			expectedBase: []runnable{
				cloneCommand{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
					fetchRetries,
				},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
			},
			expectedPull: []runnable{
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "minimal refs with no submodules",
			refs: prowapi.Refs{
				Org:            "org",
				Repo:           "repo",
				BaseRef:        "master",
				SkipSubmodules: true,
			},
			dir: "/go",
			expectedBase: []runnable{
				cloneCommand{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
					fetchRetries,
				},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
			},
			expectedPull: nil,
		},
		{
			name:       "minimal refs with oauth token",
			oauthToken: "12345678",
			refs: prowapi.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
			},
			dir: "/go",
			expectedBase: []runnable{
				cloneCommand{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://12345678:x-oauth-basic@github.com/org/repo.git", "--tags", "--prune"}},
					fetchRetries,
				},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://12345678:x-oauth-basic@github.com/org/repo.git", "master"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
			},
			expectedPull: []runnable{
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			}},
		{
			name: "refs with clone URI override",
			refs: prowapi.Refs{
				Org:      "org",
				Repo:     "repo",
				BaseRef:  "master",
				CloneURI: "internet.com",
			},
			dir: "/go",
			expectedBase: []runnable{
				cloneCommand{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "internet.com", "--tags", "--prune"}},
					fetchRetries,
				},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "internet.com", "master"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
			},
			expectedPull: []runnable{
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name:       "refs with clone URI override and oauth token specified",
			oauthToken: "12345678",
			refs: prowapi.Refs{
				Org:      "org",
				Repo:     "repo",
				BaseRef:  "master",
				CloneURI: "https://internet.com",
			},
			dir: "/go",
			expectedBase: []runnable{
				cloneCommand{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://12345678:x-oauth-basic@internet.com", "--tags", "--prune"}},
					fetchRetries,
				},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://12345678:x-oauth-basic@internet.com", "master"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
			},
			expectedPull: []runnable{
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "refs with path alias",
			refs: prowapi.Refs{
				Org:       "org",
				Repo:      "repo",
				BaseRef:   "master",
				PathAlias: "my/favorite/dir",
			},
			dir: "/go",
			expectedBase: []runnable{
				cloneCommand{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/my/favorite/dir"}},
				cloneCommand{dir: "/go/src/my/favorite/dir", command: "git", args: []string{"init"}},
				retryCommand{
					cloneCommand{dir: "/go/src/my/favorite/dir", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
					fetchRetries,
				},
				retryCommand{
					cloneCommand{dir: "/go/src/my/favorite/dir", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/my/favorite/dir", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/my/favorite/dir", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/my/favorite/dir", command: "git", args: []string{"checkout", "master"}},
			},
			expectedPull: []runnable{
				cloneCommand{dir: "/go/src/my/favorite/dir", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "refs with specific base sha",
			refs: prowapi.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
				BaseSHA: "abcdef",
			},
			dir: "/go",
			expectedBase: []runnable{
				cloneCommand{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
					fetchRetries,
				},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "abcdef"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "abcdef"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
			},
			expectedPull: []runnable{
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "refs with simple pr ref",
			refs: prowapi.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
				Pulls: []prowapi.Pull{
					{Number: 1},
				},
			},
			dir: "/go",
			expectedBase: []runnable{
				cloneCommand{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
					fetchRetries,
				},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
			},
			expectedPull: []runnable{
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "pull/1/head"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"merge", "--no-ff", "FETCH_HEAD"}, env: gitTimestampEnvs(fakeTimestamp + 1)},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "refs with pr ref override",
			refs: prowapi.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
				Pulls: []prowapi.Pull{
					{Number: 1, Ref: "pull-me"},
				},
			},
			dir: "/go",
			expectedBase: []runnable{
				cloneCommand{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
					fetchRetries,
				},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
			},
			expectedPull: []runnable{
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "pull-me"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"merge", "--no-ff", "FETCH_HEAD"}, env: gitTimestampEnvs(fakeTimestamp + 1)},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "refs with pr ref with specific sha",
			refs: prowapi.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
				Pulls: []prowapi.Pull{
					{Number: 1, SHA: "abcdef"},
				},
			},
			dir: "/go",
			expectedBase: []runnable{
				cloneCommand{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
					fetchRetries,
				},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
			},
			expectedPull: []runnable{
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "pull/1/head"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"merge", "--no-ff", "abcdef"}, env: gitTimestampEnvs(fakeTimestamp + 1)},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
		{
			name: "refs with multiple simple pr refs",
			refs: prowapi.Refs{
				Org:     "org",
				Repo:    "repo",
				BaseRef: "master",
				Pulls: []prowapi.Pull{
					{Number: 1},
					{Number: 2},
				},
			},
			dir: "/go",
			expectedBase: []runnable{
				cloneCommand{dir: "/", command: "mkdir", args: []string{"-p", "/go/src/github.com/org/repo"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"init"}},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "--tags", "--prune"}},
					fetchRetries,
				},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "master"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"branch", "--force", "master", "FETCH_HEAD"}},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"checkout", "master"}},
			},
			expectedPull: []runnable{
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "pull/1/head"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"merge", "--no-ff", "FETCH_HEAD"}, env: gitTimestampEnvs(fakeTimestamp + 1)},
				retryCommand{
					cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"fetch", "https://github.com/org/repo.git", "pull/2/head"}},
					fetchRetries,
				},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"merge", "--no-ff", "FETCH_HEAD"}, env: gitTimestampEnvs(fakeTimestamp + 2)},
				cloneCommand{dir: "/go/src/github.com/org/repo", command: "git", args: []string{"submodule", "update", "--init", "--recursive"}},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			g := gitCtxForRefs(testCase.refs, testCase.dir, testCase.env, testCase.oauthToken)
			actualBase := g.commandsForBaseRef(testCase.refs, testCase.gitUserName, testCase.gitUserEmail, testCase.cookiePath)
			if !reflect.DeepEqual(actualBase, testCase.expectedBase) {
				t.Errorf("generated incorrect commands:\nGot: %#v\nExpected:%#v", actualBase, testCase.expectedBase)
			}

			actualPull := g.commandsForPullRefs(testCase.refs, fakeTimestamp)
			if !reflect.DeepEqual(actualPull, testCase.expectedPull) {
				t.Errorf("generated incorrect commands: %v", diff.ObjectGoPrintDiff(testCase.expectedPull, actualPull))
			}
		})
	}
}

func TestGitHeadTimestamp(t *testing.T) {
	fakeTimestamp := 987654321
	fakeGitDir, err := makeFakeGitRepo(fakeTimestamp)
	if err != nil {
		t.Errorf("error creating fake git dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(fakeGitDir); err != nil {
			t.Errorf("error cleaning up fake git dir: %v", err)
		}
	}()

	var testCases = []struct {
		name        string
		dir         string
		noPath      bool
		expected    int
		expectError bool
	}{
		{
			name:        "root - no git",
			dir:         "/",
			expected:    0,
			expectError: true,
		},
		{
			name:        "fake git repo",
			dir:         fakeGitDir,
			expected:    fakeTimestamp,
			expectError: false,
		},
		{
			name:        "fake git repo but no git binary",
			dir:         fakeGitDir,
			noPath:      true,
			expected:    0,
			expectError: true,
		},
	}
	origCwd, err := os.Getwd()
	if err != nil {
		t.Errorf("failed getting cwd: %v", err)
	}
	origPath := os.Getenv("PATH")
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := os.Chdir(testCase.dir); err != nil {
				t.Errorf("%s: failed to chdir to %s: %v", testCase.name, testCase.dir, err)
			}
			if testCase.noPath {
				if err := os.Unsetenv("PATH"); err != nil {
					t.Errorf("%s: failed to unset PATH: %v", testCase.name, err)
				}
			}
			g := gitCtx{
				cloneDir: testCase.dir,
			}
			timestamp, err := g.gitHeadTimestamp()
			if timestamp != testCase.expected {
				t.Errorf("%s: timestamp %d does not match expected timestamp %d", testCase.name, timestamp, testCase.expected)
			}
			if (err == nil && testCase.expectError) || (err != nil && !testCase.expectError) {
				t.Errorf("%s: expect error is %v but received error %v", testCase.name, testCase.expectError, err)
			}
			if err := os.Chdir(origCwd); err != nil {
				t.Errorf("%s: failed to chdir to original cwd %s: %v", testCase.name, origCwd, err)
			}
			if testCase.noPath {
				if err := os.Setenv("PATH", origPath); err != nil {
					t.Errorf("%s: failed to set PATH to original: %v", testCase.name, err)
				}
			}

		})
	}
}

// makeFakeGitRepo creates a fake git repo with a constant digest and timestamp.
func makeFakeGitRepo(fakeTimestamp int) (string, error) {
	fakeGitDir, err := ioutil.TempDir("", "fakegit")
	if err != nil {
		return "", err
	}
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.test"},
		{"git", "config", "user.name", "test test"},
		{"touch", "a_file"},
		{"git", "add", "a_file"},
		{"git", "commit", "-m", "adding a_file"},
	}
	for _, cmd := range cmds {
		c := exec.Command(cmd[0], cmd[1:]...)
		c.Dir = fakeGitDir
		c.Env = append(os.Environ(), gitTimestampEnvs(fakeTimestamp)...)
		if err := c.Run(); err != nil {
			return fakeGitDir, err
		}
	}
	return fakeGitDir, nil
}

func TestCensorToken(t *testing.T) {
	testCases := []struct {
		id       string
		token    string
		msg      string
		expected string
	}{
		{
			id:       "no token",
			msg:      "git fetch https://github.com/kubernetes/test-infra.git",
			expected: "git fetch https://github.com/kubernetes/test-infra.git",
		},
		{
			id:       "with token",
			token:    "123456789",
			msg:      "git fetch 123456789:x-oauth-basic@https://github.com/kubernetes/test-infra.git",
			expected: "git fetch CENSORED:x-oauth-basic@https://github.com/kubernetes/test-infra.git",
		},
		{
			id:    "git output with token",
			token: "123456789",
			msg: `
Cloning into 'test-infa'...
remote: Invalid username or password.
fatal: Authentication failed for 'https://123456789@github.com/kubernetes/test-infa/'
`,
			expected: `
Cloning into 'test-infa'...
remote: Invalid username or password.
fatal: Authentication failed for 'https://CENSORED@github.com/kubernetes/test-infa/'
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			censoredMsg := censorToken(tc.msg, tc.token)
			if !reflect.DeepEqual(censoredMsg, tc.expected) {
				t.Fatalf("expected: %s got %s", tc.expected, censoredMsg)
			}
		})
	}
}

// fakeRunner will pass run() if called when calls == 1,
// decrementing calls each time.
type fakeRunner struct {
	calls int
}

func (fr *fakeRunner) run() (string, string, error) {
	fr.calls--
	if fr.calls == 0 {
		return "command", "output", nil
	}
	return "command", "output", fmt.Errorf("calls: %d", fr.calls)
}

func TestGitFetch(t *testing.T) {
	const short = time.Nanosecond
	command := func(calls int, retries ...time.Duration) retryCommand {
		return retryCommand{
			runnable: &fakeRunner{calls},
			retries:  retries,
		}
	}
	cases := []struct {
		name string
		retryCommand
		err bool
	}{
		{
			name:         "works without retires",
			retryCommand: command(1),
		},
		{
			name:         "errors if first call fails without retries",
			retryCommand: command(0),
			err:          true,
		},
		{
			name:         "works with retries (without retrying)",
			retryCommand: command(1, short),
		},
		{
			name:         "works with retries (retrying)",
			retryCommand: command(2, short),
		},
		{
			name:         "errors without retries if first call fails",
			retryCommand: command(2),
			err:          true,
		},
		{
			name:         "errors with retries when all retries are consumed",
			retryCommand: command(3, short),
			err:          true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := tc.run()
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Error("failed to received expected error")
			}
		})
	}
}
