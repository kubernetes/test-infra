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
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

func TestHandlePE(t *testing.T) {
	testCases := []struct {
		name      string
		pe        github.PushEvent
		jobsToRun int
	}{
		{
			name: "branch deleted",
			pe: github.PushEvent{
				Ref: "master",
				Repo: github.Repo{
					FullName: "org/repo",
				},
				Deleted: true,
			},
			jobsToRun: 0,
		},
		{
			name: "no matching files",
			pe: github.PushEvent{
				Ref: "master",
				Commits: []github.Commit{
					{
						Added: []string{"example.txt"},
					},
				},
				Repo: github.Repo{
					FullName: "org/repo",
				},
			},
		},
		{
			name: "one matching file",
			pe: github.PushEvent{
				Ref: "master",
				Commits: []github.Commit{
					{
						Added:    []string{"example.txt"},
						Modified: []string{"hack.sh"},
					},
				},
				Repo: github.Repo{
					FullName: "org/repo",
				},
			},
			jobsToRun: 1,
		},
		{
			name: "no change matcher",
			pe: github.PushEvent{
				Ref: "master",
				Commits: []github.Commit{
					{
						Added: []string{"example.txt"},
					},
				},
				Repo: github.Repo{
					FullName: "org2/repo2",
				},
			},
			jobsToRun: 1,
		},
	}
	for _, tc := range testCases {
		g := &fakegithub.FakeClient{}
		kc := &fkc{}
		c := client{
			GitHubClient: g,
			KubeClient:   kc,
			Config:       &config.Config{},
			Logger:       logrus.WithField("plugin", pluginName),
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
		}
		if err := c.Config.SetPostsubmits(postsubmits); err != nil {
			t.Fatalf("failed to set postsubmits: %v", err)
		}
		err := handlePE(c, tc.pe)
		if err != nil {
			t.Errorf("test %q: handlePE returned unexpected error %v", tc.name, err)
		}
		if len(kc.started) != tc.jobsToRun {
			t.Errorf("test %q: expected %d jobs to run, got %d", tc.name, tc.jobsToRun, len(kc.started))
		}
	}
}
