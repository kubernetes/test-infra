/*
Copyright 2019 The Kubernetes Authors.

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

// Package milestoneapplier implements the plugin to automatically apply
// the configured milestone after a PR is merged.
package milestoneapplier

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/milestone"
)

const pluginName = "milestoneapplier"

type githubClient interface {
	SetMilestone(org, repo string, issueNum, milestoneNum int) error
	ListMilestones(org, repo string) ([]github.Milestone, error)
}

func init() {
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	configInfo := map[string]string{}
	for _, repo := range enabledRepos {
		var branchesToMilestone []string
		for branch, milestone := range config.MilestoneApplier[repo.String()] {
			branchesToMilestone = append(branchesToMilestone, fmt.Sprintf("- `%s`: `%s`", branch, milestone))
		}
		configInfo[repo.String()] = fmt.Sprintf("The configured branches and milestones for this repo are:\n%s", strings.Join(branchesToMilestone, "\n"))
	}

	// The {WhoCanUse, Usage, Examples} fields are omitted because this plugin is not triggered with commands.
	yamlSnippet, err := plugins.CommentMap.GenYaml(&plugins.Configuration{
		MilestoneApplier: map[string]plugins.BranchToMilestone{
			"kubernetes/kubernetes": {
				"release-1.19": "v1.19",
				"release-1.18": "v1.18",
			},
		},
	})
	if err != nil {
		logrus.WithError(err).Warnf("cannot generate comments for %s plugin", pluginName)
	}
	return &pluginhelp.PluginHelp{
		Description: "The milestoneapplier plugin automatically applies the configured milestone for the base branch after a PR is merged. If a PR targets a non-default branch, it also adds the milestone when the PR is opened.",
		Config:      configInfo,
		Snippet:     yamlSnippet,
	}, nil
}

func handlePullRequest(pc plugins.Agent, pre github.PullRequestEvent) error {
	org := pre.PullRequest.Base.Repo.Owner.Login
	repo := pre.PullRequest.Base.Repo.Name
	baseBranch := pre.PullRequest.Base.Ref

	// if there are no branch to milestone mappings for this repo, return early
	branchToMilestone, ok := pc.PluginConfig.MilestoneApplier[fmt.Sprintf("%s/%s", org, repo)]
	if !ok {
		return nil
	}
	// if the repo does not define milestones for this branch, return early
	milestone, ok := branchToMilestone[baseBranch]
	if !ok {
		return nil
	}

	return handle(pc.GitHubClient, pc.Logger, milestone, pre)
}

func handle(gc githubClient, log *logrus.Entry, configuredMilestone string, pre github.PullRequestEvent) error {
	pr := pre.PullRequest

	// if the current milestone is equal to the configured milestone, return early
	if pr.Milestone != nil && pr.Milestone.Title == configuredMilestone {
		return nil
	}

	// if a PR targets a non-default branch, apply milestone when opened and on merge
	// if a PR targets the default branch, apply the milestone only on merge
	merged := pre.Action == github.PullRequestActionClosed && pr.Merged
	if pr.Base.Repo.DefaultBranch != pr.Base.Ref {
		if !merged && pre.Action != github.PullRequestActionOpened {
			return nil
		}
	} else if !merged {
		return nil
	}

	number := pre.Number
	org := pr.Base.Repo.Owner.Login
	repo := pr.Base.Repo.Name

	milestones, err := gc.ListMilestones(org, repo)
	if err != nil {
		log.WithError(err).Errorf("Error listing the milestones in the %s/%s repo", org, repo)
		return err
	}

	milestoneMap := milestone.BuildMilestoneMap(milestones)
	configuredMilestoneNumber, ok := milestoneMap[configuredMilestone]
	if !ok {
		return fmt.Errorf("The configured milestone %s for %s branch does not exist in the %s/%s repo", configuredMilestone, pr.Base.Ref, org, repo)
	}

	if err := gc.SetMilestone(org, repo, number, configuredMilestoneNumber); err != nil {
		log.WithError(err).Errorf("Error adding the milestone %s to %s/%s#%d.", configuredMilestone, org, repo, number)
		return err
	}

	return nil
}
