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
	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/jobs"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName = "trigger"
	lgtmLabel  = "lgtm"
	trustedOrg = "kubernetes"
)

func init() {
	plugins.RegisterIssueCommentHandler(pluginName, handleIssueComment)
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest)
}

type githubClient interface {
	IsMember(org, user string) (bool, error)
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	CreateComment(owner, repo string, number int, comment string) error
	ListIssueComments(owner, repo string, issue int) ([]github.IssueComment, error)
}

type kubeClient interface {
	CreateJob(j kube.Job) (kube.Job, error)
	ListJobs(labels map[string]string) ([]kube.Job, error)
	GetJob(name string) (kube.Job, error)
	PatchJob(name string, job kube.Job) (kube.Job, error)
	PatchJobStatus(name string, job kube.Job) (kube.Job, error)
}

type client struct {
	GitHubClient githubClient
	JobAgent     *jobs.JobAgent
	KubeClient   kubeClient
	Logger       *logrus.Entry
}

func getClient(pc plugins.PluginClient) client {
	return client{
		GitHubClient: pc.GitHubClient,
		JobAgent:     pc.JobAgent,
		KubeClient:   pc.KubeClient,
		Logger:       pc.Logger,
	}
}

func handlePullRequest(pc plugins.PluginClient, pr github.PullRequestEvent) error {
	return handlePR(getClient(pc), pr)
}

func handleIssueComment(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	return handleIC(getClient(pc), ic)
}
