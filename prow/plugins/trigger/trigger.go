/*
Copyright 2016 The Kubernetes Authors.

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
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName = "trigger"
	lgtmLabel  = "lgtm"
)

func init() {
	plugins.RegisterIssueCommentHandler(pluginName, handleIssueComment, nil)
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest, nil)
	plugins.RegisterPushEventHandler(pluginName, handlePush, nil)
}

type githubClient interface {
	AddLabel(org, repo string, number int, label string) error
	BotName() (string, error)
	IsMember(org, user string) (bool, error)
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetRef(org, repo, ref string) (string, error)
	CreateComment(owner, repo string, number int, comment string) error
	ListIssueComments(owner, repo string, issue int) ([]github.IssueComment, error)
	CreateStatus(owner, repo, ref string, status github.Status) error
	GetCombinedStatus(org, repo, ref string) (*github.CombinedStatus, error)
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	RemoveLabel(org, repo string, number int, label string) error
	DeleteStaleComments(org, repo string, number int, comments []github.IssueComment, isStale func(github.IssueComment) bool) error
}

type kubeClient interface {
	CreateProwJob(kube.ProwJob) (kube.ProwJob, error)
}

type client struct {
	GitHubClient githubClient
	KubeClient   kubeClient
	Config       *config.Config
	Logger       *logrus.Entry
}

func getClient(pc plugins.PluginClient) client {
	return client{
		GitHubClient: pc.GitHubClient,
		Config:       pc.Config,
		KubeClient:   pc.KubeClient,
		Logger:       pc.Logger,
	}
}

func handlePullRequest(pc plugins.PluginClient, pr github.PullRequestEvent) error {
	org, repo := pr.PullRequest.Base.Repo.Owner.Login, pr.PullRequest.Base.Repo.Name
	config := pc.PluginConfig.TriggerFor(org, repo)
	var trustedOrg string
	if config == nil || config.TrustedOrg == "" {
		trustedOrg = org
	} else {
		trustedOrg = config.TrustedOrg
	}
	return handlePR(getClient(pc), trustedOrg, pr)
}

func handleIssueComment(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	org, repo := ic.Repo.Owner.Login, ic.Repo.Name
	config := pc.PluginConfig.TriggerFor(org, repo)
	var trustedOrg string
	if config == nil || config.TrustedOrg == "" {
		trustedOrg = org
	} else {
		trustedOrg = config.TrustedOrg
	}
	return handleIC(getClient(pc), trustedOrg, ic)
}

func handlePush(pc plugins.PluginClient, pe github.PushEvent) error {
	return handlePE(getClient(pc), pe)
}
