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

package update_config

import (
	"fmt"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName = "config-updater"
	configFile = "prow/config.yaml"
	pluginFile = "prow/plugins.yaml"
)

func init() {
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest)
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	GetPullRequestChanges(pr github.PullRequest) ([]github.PullRequestChange, error)
	GetFile(org, repo, filepath, commit string) ([]byte, error)
}

type kubeClient interface {
	ReplaceConfigMap(name string, config kube.ConfigMap) (kube.ConfigMap, error)
}

func handlePullRequest(pc plugins.PluginClient, pre github.PullRequestEvent) error {
	return handle(pc.GitHubClient, pc.KubeClient, pc.Logger, pre)
}

func handleConfig(gc githubClient, kc kubeClient, org, repo, commit string) error {
	content, err := gc.GetFile(org, repo, configFile, commit)
	if err != nil {
		return err
	}

	c := kube.ConfigMap{
		Metadata: kube.ObjectMeta{
			Name: "config",
		},
		Data: map[string]string{
			"config": string(content),
		},
	}

	_, err = kc.ReplaceConfigMap("config", c)
	return err
}

func handlePlugin(gc githubClient, kc kubeClient, org, repo, commit string) error {
	content, err := gc.GetFile(org, repo, pluginFile, commit)
	if err != nil {
		return err
	}

	c := kube.ConfigMap{
		Metadata: kube.ObjectMeta{
			Name: "plugins",
		},
		Data: map[string]string{
			"plugins": string(content),
		},
	}

	_, err = kc.ReplaceConfigMap("plugins", c)
	return err
}

func handle(gc githubClient, kc kubeClient, log *logrus.Entry, pre github.PullRequestEvent) error {
	// Only consider newly merged PRs
	if pre.Action != "closed" {
		return nil
	}

	pr := pre.PullRequest
	if !pr.Merged {
		return nil
	}

	// Process change to prow/config.yaml and prow/plugin.yaml
	changes, err := gc.GetPullRequestChanges(pr)
	if err != nil {
		return err
	}

	org := pr.Base.Repo.Owner.Login
	repo := pr.Base.Repo.Name

	var msg string
	for _, change := range changes {
		if change.Filename == configFile {
			if err := handleConfig(gc, kc, org, repo, pr.Head.SHA); err != nil {
				return err
			}

			msg += fmt.Sprintf("I updated Prow config for you!")

		} else if change.Filename == pluginFile {
			if err := handlePlugin(gc, kc, org, repo, pr.Head.SHA); err != nil {
				return err
			}

			if msg != "" {
				msg += "\n--------------------------\n"
			}

			msg += fmt.Sprintf("I updated Prow plugins config for you!")
		}
	}

	if msg != "" {
		if gc.CreateComment(org, repo, pr.Number, plugins.FormatResponseRaw(pr.Body, pr.HTMLURL, pr.User.Login, msg)); err != nil {
			return fmt.Errorf("comment err: %v", err)
		}
	}

	return nil
}
