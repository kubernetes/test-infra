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
	"path"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName = "config-updater"
)

func init() {
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	var configInfo map[string]string
	if len(enabledRepos) == 1 {
		msg := fmt.Sprintf(
			"The main configuration is kept in sync with '%s/%s'.\nThe plugin configuration is kept in sync with '%s/%s'.",
			enabledRepos[0],
			config.ConfigUpdater.ConfigFile,
			enabledRepos[0],
			config.ConfigUpdater.PluginFile,
		)
		configInfo = map[string]string{"": msg}
	}
	return &pluginhelp.PluginHelp{
			Description: "The config-updater plugin automatically redeploys configuration and plugin configuration files when they change. The plugin watches for pull request merges that modify either of the config files and updates the cluster's configmap resources in response.",
			Config:      configInfo,
		},
		nil
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	GetFile(org, repo, filepath, commit string) ([]byte, error)
}

type kubeClient interface {
	ReplaceConfigMap(name string, config kube.ConfigMap) (kube.ConfigMap, error)
}

func handlePullRequest(pc plugins.PluginClient, pre github.PullRequestEvent) error {
	return handle(pc.GitHubClient, pc.KubeClient, pc.Logger, pre, maps(pc))
}

func maps(pc plugins.PluginClient) map[string]plugins.ConfigMapSpec {
	return pc.PluginConfig.ConfigUpdater.Maps
}

func update(gc githubClient, kc kubeClient, org, repo, commit, filename, name, namespace string) error {
	content, err := gc.GetFile(org, repo, filename, commit)
	if err != nil {
		return fmt.Errorf("get file err: %v", err)
	}

	c := string(content)

	cm := kube.ConfigMap{
		ObjectMeta: kube.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			name:                c,
			path.Base(filename): c,
		},
	}

	_, err = kc.ReplaceConfigMap(name, cm)
	if err != nil {
		return fmt.Errorf("replace config map err: %v", err)
	}
	return nil
}

func handle(gc githubClient, kc kubeClient, log *logrus.Entry, pre github.PullRequestEvent, configMaps map[string]plugins.ConfigMapSpec) error {
	// Only consider newly merged PRs
	if pre.Action != github.PullRequestActionClosed {
		return nil
	}

	if len(configMaps) == 0 { // Nothing to update
		return nil
	}

	pr := pre.PullRequest
	if !pr.Merged || pr.MergeSHA == nil {
		return nil
	}

	org := pr.Base.Repo.Owner.Login
	repo := pr.Base.Repo.Name

	// Which files changed in this PR?
	changes, err := gc.GetPullRequestChanges(org, repo, pr.Number)
	if err != nil {
		return err
	}

	// Are any of the changes files ones that define a configmap we want to update?
	var updated []plugins.ConfigMapSpec
	for _, change := range changes {
		cm, ok := configMaps[change.Filename]
		if !ok {
			continue // This file does not define a configmap
		}
		// Yes, update the configmap with the contents of this file
		if err := update(gc, kc, org, repo, *pr.MergeSHA, change.Filename, cm.Name, cm.Namespace); err != nil {
			return err
		}
		updated = append(updated, cm)
	}

	message := func(cm plugins.ConfigMapSpec) string {
		msg := fmt.Sprintf("%s configmap", cm.Name)
		if cm.Namespace != "" {
			msg = fmt.Sprintf("%s on namespace %s", msg, cm.Namespace)
		}
		return msg
	}

	var msg string
	switch n := len(updated); n {
	case 0:
		return nil
	case 1:
		msg = fmt.Sprintf("Updated the %s", message(updated[0]))
	default:
		msg = fmt.Sprintf("Updated the following %d configmaps:\n", n)
		for _, cm := range updated {
			msg += fmt.Sprintf("  * %s\n", message(cm))
		}
	}

	if err := gc.CreateComment(org, repo, pr.Number, plugins.FormatResponseRaw(pr.Body, pr.HTMLURL, pr.User.Login, msg)); err != nil {
		return fmt.Errorf("comment err: %v", err)
	}
	return nil
}
