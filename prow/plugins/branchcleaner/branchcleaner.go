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

package branchcleaner

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName = "branchcleaner"
)

func init() {
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	return &pluginhelp.PluginHelp{
		Description: "The branchcleaner plugin automatically deletes source branches for merged PRs between two branches on the same repository. This is helpful to keep repos that don't allow forking clean."}, nil
}

func handlePullRequest(pc plugins.Agent, pre github.PullRequestEvent) error {
	return handle(pc.GitHubClient, pc.Logger, pre)
}

type githubClient interface {
	DeleteRef(owner, repo, ref string) error
}

func handle(gc githubClient, log *logrus.Entry, pre github.PullRequestEvent) error {
	// Only consider closed PRs that got merged
	if pre.Action != github.PullRequestActionClosed || !pre.PullRequest.Merged {
		return nil
	}

	pr := pre.PullRequest

	//Only consider PRs from the same repo
	if pr.Base.Repo.FullName != pr.Head.Repo.FullName {
		return nil
	}

	if err := gc.DeleteRef(pr.Base.Repo.Owner.Login, pr.Base.Repo.Name, fmt.Sprintf("heads/%s", pr.Head.Ref)); err != nil {
		return fmt.Errorf("failed to delete branch %s on repo %s/%s after Pull Request #%d got merged: %v",
			pr.Head.Ref, pr.Base.Repo.Owner.Login, pr.Base.Repo.Name, pre.PullRequest.Number, err)
	}

	return nil
}
