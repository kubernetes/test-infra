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
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/diff"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/plugins"
)

const defaultNamespace = "default"

type fakeKubeClient struct {
	maps map[string]kube.ConfigMap
}

func (c *fakeKubeClient) GetConfigMap(name, namespace string) (kube.ConfigMap, error) {
	return c.maps[name], nil
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
			name:        "changed plugins.yaml, 1 update with custom key",
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
			name:        "changed resources.yaml, 1 update with custom namespace",
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
			name:        "changed config.yaml, plugins.yaml and resources.yaml, 3 update",
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
				Key:  "test-key",
			},
			"boskos/resources.yaml": {
				Name:      "boskos-config",
				Namespace: "boskos",
			},
		}

		updatedConfigMaps := map[string]kube.ConfigMap{
			"config": {
				ObjectMeta: kube.ObjectMeta{
					Name:      "config",
					Namespace: defaultNamespace,
				},
				Data: map[string]string{
					"config.yaml": "new-config",
				},
			},
			"plugins": {
				ObjectMeta: kube.ObjectMeta{
					Name:      "plugins",
					Namespace: defaultNamespace,
				},
				Data: map[string]string{
					"test-key": "new-plugins",
				},
			},
			"boskos-config": {
				ObjectMeta: kube.ObjectMeta{
					Name:      "boskos-config",
					Namespace: "boskos",
				},
				Data: map[string]string{
					"resources.yaml": "new-boskos-config",
				},
			},
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
			if config, ok := fkc.maps[configName]; !ok {
				t.Errorf("tc %s : Should have updated configmap for '%s'", tc.name, configName)
			} else if expected, actual := updatedConfigMaps[configName], config; !equality.Semantic.DeepEqual(expected, actual) {
				t.Errorf("%s: incorrect ConfigMap state after update: %v", tc.name, diff.ObjectReflectDiff(expected, actual))
			}

		}
	}
}

func TestUpdateConfigMultipleKeys(t *testing.T) {
	log := logrus.WithField("plugin", pluginName)
	mergeSHA := "12345"
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
		Merged:   true,
		MergeSHA: &mergeSHA,
	}
	event := github.PullRequestEvent{
		Action:      github.PullRequestActionClosed,
		Number:      basicPR.Number,
		PullRequest: basicPR,
	}

	fgc := &fakegithub.FakeClient{
		PullRequests: map[int]*github.PullRequest{
			basicPR.Number: &basicPR,
		},
		PullRequestChanges: map[int][]github.PullRequestChange{
			basicPR.Number: {
				{
					Filename:  "custom/file.yaml",
					Additions: 1,
				},
				{
					Filename:  "custom/other.yaml",
					Additions: 1,
				},
				{
					Filename:  "custom/config.yaml",
					Additions: 1,
				},
			},
		},
		IssueComments: map[int][]github.IssueComment{},
		RemoteFiles: map[string]map[string]string{
			"custom/file.yaml": {
				"master": "old-file",
				"12345":  "new-file",
			},
			"custom/other.yaml": {
				"master": "old-other",
				"12345":  "new-other",
			},
			"custom/config.yaml": {
				"master": "old-config",
				"12345":  "new-config",
			},
		},
	}
	fkc := &fakeKubeClient{
		maps: map[string]kube.ConfigMap{
			"custom": {
				ObjectMeta: kube.ObjectMeta{
					Name:      "custom",
					Namespace: defaultNamespace,
				},
				Data: map[string]string{
					"unchanged": "old-unchanged",
					"file.yaml": "old-file",
				},
			},
		},
	}

	m := map[string]plugins.ConfigMapSpec{
		"custom/file.yaml": {
			Name: "custom",
		},
		"custom/other.yaml": {
			Name: "custom",
		},
		"custom/config.yaml": {
			Name: "custom",
			Key:  "config",
		},
	}

	updatedConfigMap := kube.ConfigMap{
		ObjectMeta: kube.ObjectMeta{
			Name:      "custom",
			Namespace: defaultNamespace,
		},
		Data: map[string]string{
			"unchanged":  "old-unchanged",
			"file.yaml":  "new-file",
			"other.yaml": "new-other",
			"config":     "new-config",
		},
	}

	if err := handle(fgc, fkc, log, event, m); err != nil {
		t.Fatal(err)
	}

	if config, ok := fkc.maps["custom"]; !ok {
		t.Error("Should have updated configmap for 'custom'")
	} else if expected, actual := updatedConfigMap, config; !equality.Semantic.DeepEqual(expected, actual) {
		t.Errorf("incorrect ConfigMap state after update: %v", diff.ObjectReflectDiff(expected, actual))
	}
}
