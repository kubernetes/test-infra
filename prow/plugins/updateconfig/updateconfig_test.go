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
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/plugins"
)

const defaultNamespace = "default"

type fakeKubeClient struct {
	maps map[string]kube.ConfigMap
}

func (c *fakeKubeClient) ReplaceConfigMap(name string, config kube.ConfigMap) (kube.ConfigMap, error) {
	if config.ObjectMeta.Name != name {
		return kube.ConfigMap{}, fmt.Errorf("name %s does not match configmap name %s", name, config.ObjectMeta.Name)
	}
	if config.Namespace == "" {
		config.Namespace = defaultNamespace
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
		prAction      github.PullRequestEventAction
		merged        bool
		mergeCommit   string
		changes       []github.PullRequestChange
		configUpdates []string
	}{
		{
			name:     "Opened PR, no update",
			prAction: github.PullRequestActionOpened,
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
			prAction: github.PullRequestActionClosed,
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
			prAction: github.PullRequestActionClosed,
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
			prAction:    github.PullRequestActionClosed,
			merged:      true,
			mergeCommit: "12345",
			changes: []github.PullRequestChange{
				{
					Filename:  "prow/config.yaml",
					Additions: 1,
				},
			},
			configUpdates: []string{"config"},
		},
		{
			name:        "changed plugins.yaml, 1 update",
			prAction:    github.PullRequestActionClosed,
			merged:      true,
			mergeCommit: "12345",
			changes: []github.PullRequestChange{
				{
					Filename:  "prow/plugins.yaml",
					Additions: 1,
				},
			},
			configUpdates: []string{"plugins"},
		},
		{
			name:        "changed resources.yaml, 1 update",
			prAction:    github.PullRequestActionClosed,
			merged:      true,
			mergeCommit: "12345",
			changes: []github.PullRequestChange{
				{
					Filename:  "boskos/resources.yaml",
					Additions: 1,
				},
			},
			configUpdates: []string{"boskos-config"},
		},
		{
			name:        "changed config.yaml and plugins.yaml, 2 update",
			prAction:    github.PullRequestActionClosed,
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
				{
					Filename:  "boskos/resources.yaml",
					Additions: 1,
				},
			},
			configUpdates: []string{"config", "plugins", "boskos-config"},
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
				"boskos/resources.yaml": {
					"master": "old-boskos-config",
					"12345":  "new-boskos-config",
				},
			},
		}
		fkc := &fakeKubeClient{
			maps: map[string]kube.ConfigMap{},
		}

		m := map[string]plugins.ConfigMapSpec{
			"prow/config.yaml": {
				Name: "config",
			},
			"prow/plugins.yaml": {
				Name: "plugins",
			},
			"boskos/resources.yaml": {
				Name:      "boskos-config",
				Namespace: "boskos",
			},
		}

		configNamespaces := map[string]string{
			"config":        defaultNamespace,
			"plugins":       defaultNamespace,
			"boskos-config": "boskos",
		}

		if err := handle(fgc, fkc, log, event, m); err != nil {
			t.Fatal(err)
		}

		if tc.configUpdates != nil {
			if len(fgc.IssueComments[basicPR.Number]) != 1 {
				t.Fatalf("tc %s : Expect 1 comment, actually got %d", tc.name, len(fgc.IssueComments[basicPR.Number]))
			}

			comment := fgc.IssueComments[basicPR.Number][0].Body
			if !strings.Contains(comment, "Updated the") {
				t.Errorf("%s: missing Updated the from %s", tc.name, comment)
			}
			for _, configName := range tc.configUpdates {
				if !strings.Contains(comment, configName) {
					t.Errorf("%s: missing %s from %s", tc.name, configName, comment)
				}
			}
		}

		for _, configName := range tc.configUpdates {
			newConfigContent := fmt.Sprintf("new-%s", configName)
			if config, ok := fkc.maps[configName]; !ok {
				t.Fatalf("tc %s : Should have updated configmap for '%s'", tc.name, configName)
			} else if config.Data[configName] != newConfigContent {
				t.Fatalf(
					"tc %s : Expect get %s '%s', got '%s'",
					tc.name,
					configName,
					newConfigContent,
					config.Data[configName])
			} else if config.Namespace != configNamespaces[configName] {
				t.Fatalf(
					"tc %s : Namespace should be set to %s, found '%s'",
					tc.name,
					configNamespaces[configName],
					config.Data["config"])
			}

		}
	}
}
