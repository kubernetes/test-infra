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
	"regexp"
	"strings"

	"k8s.io/test-infra/prow/plugins/ownersconfig"

	"k8s.io/test-infra/prow/labels"

	"k8s.io/apimachinery/pkg/util/sets"

	verifyowners "k8s.io/test-infra/prow/plugins/verify-owners"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	// PluginName defines this plugin's registered name.
	PluginName = "reward-owners"
)

var (
	addedUserRe   = regexp.MustCompile(`(?m)^\+\s+-(.+?)$`)
	removedUserRe = regexp.MustCompile(`(?m)^-\s+-(.+?)$`)
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
	IsCollaborator(owner, repo, login string) (bool, error)
	IsMember(org, user string) (bool, error)
	AddLabel(org, repo string, number int, label string) error
	CreateComment(owner, repo string, number int, comment string) error
	CreateReview(org, repo string, number int, r github.DraftReview) error
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	RemoveLabel(owner, repo string, number int, label string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	BotUserChecker() (func(candidate string) bool, error)
}

type info struct {
	org          string
	repo         string
	repoFullName string
	number       int
}

func handlePullRequest(pc plugins.Agent, pre github.PullRequestEvent) error {
	if pre.Action != github.PullRequestActionOpened && pre.Action != github.PullRequestActionReopened && pre.Action != github.PullRequestActionSynchronize {
		return nil
	}

	var skipTrustedUserCheck bool
	for _, r := range pc.PluginConfig.Owners.SkipCollaborators {
		if r == pre.Repo.FullName {
			skipTrustedUserCheck = true
			break
		}
	}

	prInfo := info{
		org:          pre.Repo.Owner.Login,
		repo:         pre.Repo.Name,
		repoFullName: pre.Repo.FullName,
		number:       pre.Number,
	}
	return handle(pc.GitHubClient, pc.GitClient, pc.Logger, &pre.PullRequest, prInfo, pc.PluginConfig.TriggerFor(pre.Repo.Owner.Login, pre.Repo.Name), skipTrustedUserCheck, pc.PluginConfig.OwnersFilenames)
}

func handle(ghc githubClient, gc git.ClientFactory, log *logrus.Entry, pr *github.PullRequest, info info, triggerConfig plugins.Trigger, skipTrustedUserCheck bool, resolver ownersconfig.Resolver) error {
	org := info.org
	repo := info.repo
	number := info.number
	filenames := resolver(org, repo)

	// Get changes.
	changes, err := ghc.GetPullRequestChanges(org, repo, number)
	if err != nil {
		return fmt.Errorf("error getting PR changes: %v", err)
	}

	// List added and removed users from OWNERS and OWNERS_ALIASES files.
	addedUsers := sets.NewString()
	removedUsers := sets.NewString()
	for _, change := range changes {
		if (filepath.Base(change.Filename) == filenames.Owners || filepath.Base(change.Filename) == filenames.OwnersAliases) && change.Status != github.PullRequestFileRemoved {
			for _, s := range addedUserRe.FindAllStringSubmatch(change.Patch, -1) {
				addedUsers.Insert(strings.TrimSpace(s[1]))
			}
			for _, s := range removedUserRe.FindAllStringSubmatch(change.Patch, -1) {
				removedUsers.Insert(strings.TrimSpace(s[1]))
			}
		}
	}

	// Deduct removed from added users.
	realAddedUsers := addedUsers.Difference(removedUsers)
	if realAddedUsers.Len() == 0 {
		return nil
	}

	// Check if the OWNERS_ALIASES file was modified.
	var modifiedOwnerAliasesFile github.PullRequestChange
	var ownerAliasesModified bool
	for _, change := range changes {
		if change.Filename == filenames.OwnersAliases {
			modifiedOwnerAliasesFile = change
			ownerAliasesModified = true
			break
		}
	}

	// Clone the repo, checkout the PR.
	r, err := gc.ClientFor(org, repo)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Clean(); err != nil {
			log.WithError(err).Error("Error cleaning up repo.")
		}
	}()
	if err := r.Config("user.name", "prow"); err != nil {
		return err
	}
	if err := r.Config("user.email", "prow@localhost"); err != nil {
		return err
	}
	if err := r.Config("commit.gpgsign", "false"); err != nil {
		log.WithError(err).Errorf("Cannot set gpgsign=false in gitconfig: %v", err)
	}
	if err := r.MergeAndCheckout(pr.Base.Ref, string(github.MergeMerge), pr.Head.SHA); err != nil {
		return err
	}
	// If OWNERS_ALIASES file exists, get all aliases.
	_, _, repoAliases, err := verifyowners.NonTrustedUsersInOwnersAliases(ghc, log, triggerConfig, org, repo, r.Directory(), modifiedOwnerAliasesFile.Patch, ownerAliasesModified, skipTrustedUserCheck, filenames)
	if err != nil {
		return err
	}

	// Reward all users, unless it's an alias.
	for _, user := range realAddedUsers.List() {
		if _, exists := repoAliases[user]; exists {
			continue
		}
		fmt.Println("let's reward", user)
		if err := ghc.AddLabel(org, repo, number, labels.Welcome); err != nil {
			return err
		}
	}

	return nil
}
