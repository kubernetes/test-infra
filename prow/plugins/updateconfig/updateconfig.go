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

	"github.com/mattn/go-zglob"
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
	GetConfigMap(name, namespace string) (kube.ConfigMap, error)
	ReplaceConfigMap(name string, config kube.ConfigMap) (kube.ConfigMap, error)
}

func handlePullRequest(pc plugins.PluginClient, pre github.PullRequestEvent) error {
	return handle(pc.GitHubClient, pc.KubeClient, pc.Logger, pre, maps(pc))
}

func maps(pc plugins.PluginClient) map[string]plugins.ConfigMapSpec {
	return pc.PluginConfig.ConfigUpdater.Maps
}

func update(gc githubClient, kc kubeClient, org, repo, commit, name, namespace string, updates map[string]string) error {
	currentContent, err := kc.GetConfigMap(name, namespace)
	if _, isNotFound := err.(kube.NotFoundError); err != nil && !isNotFound {
		return fmt.Errorf("failed to fetch current state of configmap: %v", err)
	}

	data := map[string]string{}
	if currentContent.Data != nil {
		data = currentContent.Data
	}

	for key, filename := range updates {
		if filename == "" {
			delete(data, key)
			continue
		}

		content, err := gc.GetFile(org, repo, filename, commit)
		if err != nil {
			return fmt.Errorf("get file err: %v", err)
		}
		data[key] = string(content)
	}

	cm := kube.ConfigMap{
		ObjectMeta: kube.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
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

	if !pr.Merged || pr.MergeSHA == nil || pr.Base.Repo.DefaultBranch != pr.Base.Ref {
		return nil
	}

	org := pr.Base.Repo.Owner.Login
	repo := pr.Base.Repo.Name

	// Which files changed in this PR?
	changes, err := gc.GetPullRequestChanges(org, repo, pr.Number)
	if err != nil {
		return err
	}

	message := func(name, namespace string, data map[string]string) string {
		identifier := fmt.Sprintf("`%s` configmap", name)
		if namespace != "" {
			identifier = fmt.Sprintf("%s in namespace `%s`", identifier, namespace)
		}
		msg := fmt.Sprintf("%s using the following files:", identifier)
		for key, file := range data {
			msg = fmt.Sprintf("%s\n    - key `%s` using file `%s`", msg, key, file)
		}
		return msg
	}

	// Are any of the changes files ones that define a configmap we want to update?
	var updated []string
	type configMapID struct {
		name, namespace string
	}
	toUpdate := map[configMapID]map[string]string{}
	for _, change := range changes {
		var cm plugins.ConfigMapSpec
		found := false

		for key, configMap := range configMaps {
			found, err = zglob.Match(key, change.Filename)
			if err != nil {
				// Should not happen, log err and continue
				log.WithError(err).Info("key matching error")
				continue
			}

			if found {
				cm = configMap
				break
			}
		}

		if !found {
			continue // This file does not define a configmap
		}

		// Yes, update the configmap with the contents of this file
		key := cm.Key
		if key == "" {
			key = path.Base(change.Filename)
		}
		id := configMapID{name: cm.Name, namespace: cm.Namespace}
		if _, ok := toUpdate[id]; !ok {
			toUpdate[id] = map[string]string{}
		}
		if change.Status == "removed" {
			toUpdate[id][key] = ""
		} else {
			toUpdate[id][key] = change.Filename
		}
	}

	for cm, data := range toUpdate {
		if err := update(gc, kc, org, repo, *pr.MergeSHA, cm.name, cm.namespace, data); err != nil {
			return err
		}
		updated = append(updated, message(cm.name, cm.namespace, data))
	}

	var msg string
	switch n := len(updated); n {
	case 0:
		return nil
	case 1:
		msg = fmt.Sprintf("Updated the %s", updated[0])
	default:
		msg = fmt.Sprintf("Updated the following %d configmaps:\n", n)
		for _, updateMsg := range updated {
			msg += fmt.Sprintf("  * %s\n", updateMsg)
		}
	}

	if err := gc.CreateComment(org, repo, pr.Number, plugins.FormatResponseRaw(pr.Body, pr.HTMLURL, pr.User.Login, msg)); err != nil {
		return fmt.Errorf("comment err: %v", err)
	}
	return nil
}
