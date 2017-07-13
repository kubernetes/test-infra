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

package updateconfig

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"strings"
	"testing"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/kube"
)

type fakeKubeClient struct {
	maps map[string]kube.ConfigMap
}

func (c *fakeKubeClient) ReplaceConfigMap(name string, config kube.ConfigMap) (kube.ConfigMap, error) {
	if config.Metadata.Name != name {
		return kube.ConfigMap{}, fmt.Errorf("name %s does not match configmap name %s", name, config.Metadata.Name)
	}
	c.maps[name] = config
	return c.maps[name], nil
}

func TestUpdateConfig(t *testing.T) {
	basicPR := github.PullRequest{
		Number: 1,
		Base: github.PullRequestBranch{
			Repo: github.Repo{
				Owner: github.User{
					Login: "kubernetes",
				},
				Name: "kubernetes",
			},
		},
		User: github.User{
			Login: "foo",
		},
	}

	testcases := []struct {
		name          string
		prAction      string
		merged        bool
		mergeCommit   string
		changes       []github.PullRequestChange
		configUpdate  bool
		pluginsUpdate bool
	}{
		{
			name:     "Opened PR, no update",
			prAction: "opened",
			merged:   false,
			changes: []github.PullRequestChange{
				{
					Filename:  "prow/config.yaml",
					Additions: 1,
				},
			},
		},
		{
			name:   "Opened PR, not merged, no update",
			merged: false,
			changes: []github.PullRequestChange{
				{
					Filename:  "prow/config.yaml",
					Additions: 1,
				},
			},
		},
		{
			name:     "Closed PR, no prow changes, no update",
			prAction: "closed",
			merged:   false,
			changes: []github.PullRequestChange{
				{
					Filename:  "foo.txt",
					Additions: 1,
				},
			},
		},
		{
			name:     "For whatever reason no merge commit SHA",
			prAction: "closed",
			merged:   true,
			changes: []github.PullRequestChange{
				{
					Filename:  "prow/config.yaml",
					Additions: 1,
				},
			},
		},
		{
			name:        "changed config.yaml, 1 update",
			prAction:    "closed",
			merged:      true,
			mergeCommit: "12345",
			changes: []github.PullRequestChange{
				{
					Filename:  "prow/config.yaml",
					Additions: 1,
				},
			},
			configUpdate: true,
		},
		{
			name:        "changed plugins.yaml, 1 update",
			prAction:    "closed",
			merged:      true,
			mergeCommit: "12345",
			changes: []github.PullRequestChange{
				{
					Filename:  "prow/plugins.yaml",
					Additions: 1,
				},
			},
			pluginsUpdate: true,
		},
		{
			name:        "changed config.yaml and plugins.yaml, 2 update",
			prAction:    "closed",
			merged:      true,
			mergeCommit: "12345",
			changes: []github.PullRequestChange{
				{
					Filename:  "prow/plugins.yaml",
					Additions: 1,
				},
				{
					Filename:  "prow/config.yaml",
					Additions: 1,
				},
			},
			configUpdate:  true,
			pluginsUpdate: true,
		},
	}

	for _, tc := range testcases {
		log := logrus.WithField("plugin", pluginName)
		event := github.PullRequestEvent{
			Action:      tc.prAction,
			Number:      basicPR.Number,
			PullRequest: basicPR,
		}
		event.PullRequest.Merged = tc.merged
		event.PullRequest.MergeSHA = &tc.mergeCommit

		fgc := &fakegithub.FakeClient{
			PullRequests: map[int]*github.PullRequest{
				basicPR.Number: &basicPR,
			},
			PullRequestChanges: map[int][]github.PullRequestChange{
				basicPR.Number: tc.changes,
			},
			IssueComments: map[int][]github.IssueComment{},
			RemoteFiles: map[string]map[string]string{
				"prow/config.yaml": {
					"master": "old-config",
					"12345":  "new-config",
				},
				"prow/plugins.yaml": {
					"master": "old-plugins",
					"12345":  "new-plugins",
				},
			},
		}
		fkc := &fakeKubeClient{
			maps: map[string]kube.ConfigMap{},
		}

		if err := handle(fgc, fkc, log, event); err != nil {
			t.Fatal(err)
		}

		if tc.configUpdate || tc.pluginsUpdate {
			if len(fgc.IssueComments[basicPR.Number]) != 1 {
				t.Fatalf("tc %s : Expect 1 comment, actually got %d", tc.name, len(fgc.IssueComments[basicPR.Number]))
			}

			comment := fgc.IssueComments[basicPR.Number][0].Body
			if tc.configUpdate && !strings.Contains(comment, "I updated Prow config for you!") {
				t.Fatalf("tc %s : Expect comment %s to contain 'I updated Prow config for you!'", comment, fgc.IssueComments[basicPR.Number][0].Body)
			}

			if tc.pluginsUpdate && !strings.Contains(comment, "I updated Prow plugins config for you!") {
				t.Fatalf("tc %s : Expect comment %s to contain 'I updated Prow plugins config for you!'", comment, fgc.IssueComments[basicPR.Number][0].Body)
			}
		}

		if tc.configUpdate {
			if config, ok := fkc.maps["config"]; !ok {
				t.Fatalf("tc %s : Should have updated configmap for 'config'", tc.name)
			} else if config.Data["config"] != "new-config" {
				t.Fatalf("tc %s : Expect get config 'new-config', got '%s'", tc.name, config.Data["config"])
			}
		}

		if tc.pluginsUpdate {
			if plugins, ok := fkc.maps["plugins"]; !ok {
				t.Fatalf("tc %s : Should have updated configmap for 'plugins'", tc.name)
			} else if plugins.Data["plugins"] != "new-plugins" {
				t.Fatalf("tc %s : Expect get config 'new-plugins', got '%s'", tc.name, plugins.Data["plugins"])
			}
		}
	}
}
