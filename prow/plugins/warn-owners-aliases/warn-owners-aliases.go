/*
Copyright 2023 The Kubernetes Authors.

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

package warnownersaliases

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/ownersconfig"
	"k8s.io/test-infra/prow/repoowners"
)

const (
	// PluginName defines this plugin's registered name.
	PluginName = "warn-owners-aliases"
)

func init() {
	plugins.RegisterPullRequestHandler(PluginName, handlePullRequest, helpProvider)
}

func helpProvider(_ *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	pluginHelp := &pluginhelp.PluginHelp{
		Description: fmt.Sprintf("The warn-owners-aliases plugin watches %s files for modifications and comments with OWNERS files that contain the modified alias.", ownersconfig.DefaultOwnersAliasesFile),
	}
	return pluginHelp, nil
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
}

type ownersClient interface {
	LoadRepoOwnersSha(org, repo, base, sha string, updateCache bool) (repoowners.RepoOwnerWithAliases, error)
}

type info struct {
	base         github.PullRequestBranch
	head         github.PullRequestBranch
	number       int
	org          string
	repo         string
	repoFullName string
	user         string
}

func handlePullRequest(pc plugins.Agent, pre github.PullRequestEvent) error {
	// Only consider newly opened PRs
	if pre.Action != github.PullRequestActionOpened {
		return nil
	}

	prInfo := info{
		base:         pre.PullRequest.Base,
		head:         pre.PullRequest.Head,
		number:       pre.Number,
		org:          pre.Repo.Owner.Login,
		repo:         pre.Repo.Name,
		repoFullName: pre.Repo.FullName,
		user:         pre.PullRequest.User.Login,
	}

	return handle(pc.GitHubClient, pc.OwnersClient, pc.Logger, prInfo, pc.PluginConfig.OwnersFilenames)
}

func handle(ghc githubClient, oc ownersClient, log *logrus.Entry, info info, resolver ownersconfig.Resolver) error {
	// Get changes.
	changes, err := ghc.GetPullRequestChanges(info.org, info.repo, info.number)
	if err != nil {
		return fmt.Errorf("error getting PR changes: %w", err)
	}

	// Check if OWNERS_ALIASES has been modified.
	var ownersAliasesModified bool
	filenames := resolver(info.org, info.repo)
	for _, change := range changes {
		if change.Filename == filenames.OwnersAliases &&
			change.Status != github.PullRequestFileRemoved {
			ownersAliasesModified = true
			break
		}
	}
	if !ownersAliasesModified {
		return nil
	}

	log.Debug("Resolving repository owners for base branch...")
	baseRepo, err := oc.LoadRepoOwnersSha(info.org, info.repo, info.base.Ref, info.base.SHA, false)
	if err != nil {
		return err
	}
	// This shouldn't happen, but if the repo had no owners
	// aliases defined, don't do anything and return.
	if len(baseRepo.OwnersAliases()) == 0 {
		return nil
	}

	log.Debug("Resolving repository owners for head branch...")
	headRepo, err := oc.LoadRepoOwnersSha(info.org, info.repo, info.head.Ref, info.head.SHA, false)
	if err != nil {
		return err
	}
	// If for some reason all owners aliases were deleted as part of this PR,
	// don't do anything and return.
	if len(headRepo.OwnersAliases()) == 0 {
		return nil
	}

	aliasesAddedTo := findOwnersAliasesAddedTo(baseRepo.OwnersAliases(), headRepo.OwnersAliases())
	// if no relevant changes, don't do anything.
	if len(aliasesAddedTo) == 0 {
		return nil
	}

	return ghc.CreateComment(info.org, info.repo, info.number, constructComment(info.org, info.repo, info.user, aliasesAddedTo))
}

// findOwnersAliasesAddedTo finds the existing aliases that were added to
// by comparing the lengths of the owners under each alias.
func findOwnersAliasesAddedTo(base, head repoowners.RepoAliases) []string {
	result := []string{}
	for alias := range head {
		owners, ok := base[alias]
		if !ok { // newly added alias.
			continue
		}
		// If someone was added to this already existing alias
		// as part of this PR, return this alias.
		if head[alias].Difference(owners).Len() > 0 {
			result = append(result, alias)
		}
	}

	return result
}

func constructComment(org, repo, user string, aliases []string) string {
	const message = `@%s **warning**: this PR adds users to one or more owners aliases.
  
The changed aliases are present in one or more OWNERS files, please ensure that the added  
users meet the requirements of a [reviewer/approver](https://github.com/kubernetes/community/blob/master/community-membership.md) for each of the OWNERS files this alias is present in.  
This helps us regulate unintentional grant of reviewer/approver privileges.
  
The list of aliases changes and potential corresponding OWNERS files that contain this alias in this repo is as follows:
%s`

	changeList := ""
	for _, alias := range aliases {
		changeList += fmt.Sprintf("- %s: [OWNERS files containing alias](%s)\n", alias, getHoundLink(org, repo, alias))
	}

	return fmt.Sprintf(message, user, changeList)
}

func getHoundLink(org, repo, alias string) string {
	return fmt.Sprintf("https://cs.k8s.io/?q=%s&i=nope&files=OWNERS$&excludeFiles=vendor&repos=%s/%s", alias, org, repo)
}
