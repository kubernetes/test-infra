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

package trigger

import (
	"testing"

	"github.com/sirupsen/logrus"
	clienttesting "k8s.io/client-go/testing"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/diff"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/client/clientset/versioned/fake"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"

	"k8s.io/test-infra/prow/github/fakegithub"
)

func TestCreateRefs(t *testing.T) {
	pe := github.PushEvent{
		Ref: "refs/heads/master",
		Repo: github.Repo{
			Owner: github.User{
				Name: "kubernetes",
			},
			Name:    "repo",
			HTMLURL: "https://example.com/kubernetes/repo",
		},
		After:   "abcdef",
		Compare: "https://example.com/kubernetes/repo/compare/abcdee...abcdef",
	}
	expected := prowapi.Refs{
		Org:      "kubernetes",
		Repo:     "repo",
		RepoLink: "https://example.com/kubernetes/repo",
		BaseRef:  "master",
		BaseSHA:  "abcdef",
		BaseLink: "https://example.com/kubernetes/repo/compare/abcdee...abcdef",
	}
	if actual := createRefs(pe); !equality.Semantic.DeepEqual(expected, actual) {
		t.Errorf("diff between expected and actual refs:%s", diff.ObjectReflectDiff(expected, actual))
	}
}

func TestHandlePE(t *testing.T) {
	testCases := []struct {
		name      string
		pe        github.PushEvent
		jobsToRun int
	}{
		{
			name: "branch deleted",
			pe: github.PushEvent{
				Ref: "refs/heads/master",
				Repo: github.Repo{
					Owner: github.User{Login: "org"},
					Name:  "repo",
				},
				Deleted: true,
			},
			jobsToRun: 0,
		},
		{
			name: "null after sha",
			pe: github.PushEvent{
				After: "0000000000000000000000000000000000000000",
				Ref:   "refs/heads/master",
				Repo: github.Repo{
					Owner: github.User{Login: "org"},
					Name:  "repo",
				},
			},
			jobsToRun: 0,
		},
		{
			name: "no matching files",
			pe: github.PushEvent{
				Ref: "refs/heads/master",
				Commits: []github.Commit{
					{
						Added: []string{"example.txt"},
					},
				},
				Repo: github.Repo{
					Owner: github.User{Login: "org"},
					Name:  "repo",
				},
			},
		},
		{
			name: "one matching file",
			pe: github.PushEvent{
				Ref: "refs/heads/master",
				Commits: []github.Commit{
					{
						Added:    []string{"example.txt"},
						Modified: []string{"hack.sh"},
					},
				},
				Repo: github.Repo{
					Owner: github.User{Login: "org"},
					Name:  "repo",
				},
			},
			jobsToRun: 1,
		},
		{
			name: "no change matcher",
			pe: github.PushEvent{
				Ref: "refs/heads/master",
				Commits: []github.Commit{
					{
						Added: []string{"example.txt"},
					},
				},
				Repo: github.Repo{
					Owner: github.User{Login: "org2"},
					Name:  "repo2",
				},
			},
			jobsToRun: 1,
		},
		{
			name: "branch name with a slash",
			pe: github.PushEvent{
				Ref: "refs/heads/release/v1.14",
				Commits: []github.Commit{
					{
						Added: []string{"hack.sh"},
					},
				},
				Repo: github.Repo{
					Owner: github.User{Login: "org3"},
					Name:  "repo3",
				},
			},
			jobsToRun: 1,
		},
	}
	for _, tc := range testCases {
		g := fakegithub.NewFakeClient()
		fakeProwJobClient := fake.NewSimpleClientset()
		c := Client{
			GitHubClient:  g,
			ProwJobClient: fakeProwJobClient.ProwV1().ProwJobs("prowjobs"),
			Config:        &config.Config{ProwConfig: config.ProwConfig{ProwJobNamespace: "prowjobs"}},
			Logger:        logrus.WithField("plugin", PluginName),
		}
		postsubmits := map[string][]config.Postsubmit{
			"org/repo": {
				{
					JobBase: config.JobBase{
						Name: "pass-butter",
					},
					RegexpChangeMatcher: config.RegexpChangeMatcher{
						RunIfChanged: "\\.sh$",
					},
				},
			},
			"org2/repo2": {
				{
					JobBase: config.JobBase{
						Name: "pass-salt",
					},
				},
			},
			"org3/repo3": {
				{
					JobBase: config.JobBase{
						Name: "pass-pepper",
					},
					Brancher: config.Brancher{
						Branches: []string{"release/v1.14"},
					},
				},
			},
		}
		if err := c.Config.SetPostsubmits(postsubmits); err != nil {
			t.Fatalf("failed to set postsubmits: %v", err)
		}
		err := handlePE(c, tc.pe)
		if err != nil {
			t.Errorf("test %q: handlePE returned unexpected error %v", tc.name, err)
		}
		var numStarted int
		for _, action := range fakeProwJobClient.Fake.Actions() {
			switch action.(type) {
			case clienttesting.CreateActionImpl:
				numStarted++
			}
		}
		if numStarted != tc.jobsToRun {
			t.Errorf("test %q: expected %d jobs to run, got %d", tc.name, tc.jobsToRun, numStarted)
		}
	}
}
