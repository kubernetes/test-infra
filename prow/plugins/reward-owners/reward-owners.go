/*
Copyright 2021 The Kubernetes Authors.

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

package rewardowners

import (
	"fmt"
	"path/filepath"
	"strings"

	"k8s.io/test-infra/prow/repoowners"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/plugins/ownersconfig"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	// PluginName defines this plugin's registered name.
	PluginName    = "reward-owners"
	RewardMessage = `
Thanks to %s for serving the community in this new capacity!

Next steps:
1- Reach out in [slack.k8s.io](http://slack.k8s.io/) -> #sig-contribex for your special badge swag!

2- Join [slack.k8s.io](http://slack.k8s.io/) -> #kubernetes-contributors in slack and [dev@kubernetes.io](https://groups.google.com/a/kubernetes.io/g/dev) for all upstream info

3- Review the [community-membership.md](https://github.com/kubernetes/community/blob/master/community-membership.md) doc for your role. If for some reason you can't perform the duties associated, Emeritus is a great way to take a break! [OWNERs](https://github.com/kubernetes/community/blob/master/contributors/guide/owners.md) is another great resource for how this works.

4- Look over our [governance](https://github.com/kubernetes/community/blob/master/governance.md) docs now that you are actively involved in the maintenance of the project.
`
)

func init() {
	plugins.RegisterPullRequestHandler(PluginName, handlePullRequest, helpProvider)
}

func helpProvider(_ *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	pluginHelp := &pluginhelp.PluginHelp{
		Description: fmt.Sprintf("The reward-owners plugin watches in %s and %s files for modifications and welcomes new approvers and reviewers.", ownersconfig.DefaultOwnersFile, ownersconfig.DefaultOwnersAliasesFile),
	}
	return pluginHelp, nil
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
}

type ownersClient interface {
	LoadRepoOwnersSha(org, repo, base, sha string, updateCache bool) (repoowners.RepoOwner, error)
}

type info struct {
	base         github.PullRequestBranch
	head         github.PullRequestBranch
	number       int
	org          string
	repo         string
	repoFullName string
}

func handlePullRequest(pc plugins.Agent, pre github.PullRequestEvent) error {
	// Only consider closed PRs that got merged
	if pre.Action != github.PullRequestActionClosed || !pre.PullRequest.Merged {
		return nil
	}

	prInfo := info{
		base:         pre.PullRequest.Base,
		head:         pre.PullRequest.Head,
		number:       pre.Number,
		org:          pre.Repo.Owner.Login,
		repo:         pre.Repo.Name,
		repoFullName: pre.Repo.FullName,
	}
	return handle(pc.GitHubClient, pc.OwnersClient, pc.Logger, prInfo, pc.PluginConfig.OwnersFilenames)
}

func handle(ghc githubClient, oc ownersClient, log *logrus.Entry, info info, resolver ownersconfig.Resolver) error {

	// Get changes.
	changes, err := ghc.GetPullRequestChanges(info.org, info.repo, info.number)
	if err != nil {
		return fmt.Errorf("error getting PR changes: %w", err)
	}

	// Check if OWNERS or OWNERS_ALIASES have been modified.
	var ownersModified bool
	filenames := resolver(info.org, info.repo)
	for _, change := range changes {
		if (filepath.Base(change.Filename) == filenames.Owners || change.Filename == filenames.OwnersAliases) &&
			change.Status != github.PullRequestFileRemoved {
			ownersModified = true
			break
		}
	}
	if !ownersModified {
		return nil
	}

	log.Debug("Resolving repository owners for base branch...")
	baseRepo, err := oc.LoadRepoOwnersSha(info.org, info.repo, info.base.Ref, info.base.SHA, false)
	if err != nil {
		return err
	}

	log.Debug("Resolving repository owners for head branch...")
	headRepo, err := oc.LoadRepoOwnersSha(info.org, info.repo, info.head.Ref, info.head.SHA, false)
	if err != nil {
		return err
	}

	// Reward only new owners.
	newOwners := headRepo.AllOwners().Difference(baseRepo.AllOwners())
	if newOwners.Len() == 0 {
		log.Debug("No new owner to reward, exiting.")
		return nil
	}

	// Tag users by prepending @ to their names.
	taggedNewOwners := make([]string, newOwners.Len())
	for i, o := range newOwners.List() {
		taggedNewOwners[i] = fmt.Sprintf("@%s", o)
	}

	return ghc.CreateComment(info.org, info.repo, info.number, fmt.Sprintf(RewardMessage, strings.Join(taggedNewOwners, ", ")))
}
