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

package verifyowners

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/golint"
	"k8s.io/test-infra/prow/plugins/trigger"
	"k8s.io/test-infra/prow/repoowners"
)

const (
	// PluginName defines this plugin's registered name.
	PluginName                    = "verify-owners"
	ownersFileName                = "OWNERS"
	ownersAliasesFileName         = "OWNERS_ALIASES"
	nonCollaboratorResponseFormat = "The following users are mentioned in %s file(s) but are not members of the %s org."
)

func init() {
	plugins.RegisterPullRequestHandler(PluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	pluginHelp := &pluginhelp.PluginHelp{
		Description: fmt.Sprintf("The verify-owners plugin validates %s and %s files and ensures that they always contain collaborators of the org, if they are modified in a PR. On validation failure it automatically adds the '%s' label to the PR, and a review comment on the incriminating file(s).", ownersFileName, ownersAliasesFileName, labels.InvalidOwners),
	}
	if config.Owners.LabelsBlackList != nil {
		pluginHelp.Config = map[string]string{
			"": fmt.Sprintf(`The verify-owners plugin will complain if %s files contain any of the following blacklisted labels: %s.`,
				ownersFileName,
				strings.Join(config.Owners.LabelsBlackList, ", ")),
		}
	}
	return pluginHelp, nil
}

type ownersClient interface {
	ParseSimpleConfig(path string) (repoowners.SimpleConfig, error)
	ParseFullConfig(path string) (repoowners.FullConfig, error)
}

type repoownersClient interface {
	LoadRepoOwners(org, repo, base string) (repoowners.RepoOwner, error)
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
}

type commentPruner interface {
	PruneComments(shouldPrune func(github.IssueComment) bool)
}

func handlePullRequest(pc plugins.Agent, pre github.PullRequestEvent) error {
	if pre.Action != github.PullRequestActionOpened && pre.Action != github.PullRequestActionReopened && pre.Action != github.PullRequestActionSynchronize {
		return nil
	}

	cp, err := pc.CommentPruner()
	if err != nil {
		return err
	}

	var skipTrustedUserCheck bool
	for _, r := range pc.PluginConfig.Owners.SkipCollaborators {
		if r == pre.Repo.Name {
			skipTrustedUserCheck = true
			break
		}
	}
	return handle(pc.GitHubClient, pc.GitClient, pc.OwnersClient, pc.Logger, &pre, pc.PluginConfig.Owners.LabelsBlackList, pc.PluginConfig.TriggerFor(pre.Repo.Owner.Login, pre.Repo.Name), skipTrustedUserCheck, cp)
}

type messageWithLine struct {
	line    int
	message string
}

func handle(ghc githubClient, gc *git.Client, roc repoownersClient, log *logrus.Entry, pre *github.PullRequestEvent, labelsBlackList []string, triggerConfig plugins.Trigger, skipTrustedUserCheck bool, cp commentPruner) error {
	org := pre.Repo.Owner.Login
	repo := pre.Repo.Name
	number := pre.Number
	wrongOwnersFiles := map[string]messageWithLine{}

	// Get changes.
	changes, err := ghc.GetPullRequestChanges(org, repo, pre.Number)
	if err != nil {
		return fmt.Errorf("error getting PR changes: %v", err)
	}

	// List modified OWNERS files.
	var modifiedOwnersFiles []github.PullRequestChange
	for _, change := range changes {
		if filepath.Base(change.Filename) == ownersFileName {
			modifiedOwnersFiles = append(modifiedOwnersFiles, change)
		}
	}

	// Check if the OWNERS_ALIASES file was modified.
	var modifiedOwnerAliasesFile github.PullRequestChange
	var ownerAliasesModified bool
	for _, change := range changes {
		if change.Filename == ownersAliasesFileName {
			modifiedOwnerAliasesFile = change
			ownerAliasesModified = true
			break
		}
	}

	if len(modifiedOwnersFiles) == 0 && !ownerAliasesModified {
		return nil
	}

	// Clone the repo, checkout the PR.
	r, err := gc.Clone(pre.Repo.FullName)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Clean(); err != nil {
			log.WithError(err).Error("Error cleaning up repo.")
		}
	}()
	if err := r.CheckoutPullRequest(pre.Number); err != nil {
		return err
	}
	// If we have a specific SHA, use it.
	if pre.PullRequest.Head.SHA != "" {
		if err := r.Checkout(pre.PullRequest.Head.SHA); err != nil {
			return err
		}
	}

	// If OWNERS_ALIASES file exists, get all aliases.
	// If the file was modified, check for non trusted users in the newly added owners.
	nonTrustedUsers, repoAliases, err := nonTrustedUsersInOwnersAliases(ghc, log, triggerConfig, org, repo, r.Dir, modifiedOwnerAliasesFile.Patch, ownerAliasesModified, skipTrustedUserCheck)
	if err != nil {
		return err
	}

	// Check if OWNERS files have the correct config and if they do,
	// check if all newly added owners are trusted users.
	pr, err := ghc.GetPullRequest(org, repo, number)
	if err != nil {
		return fmt.Errorf("error loading PullRequest: %v", err)
	}
	oc, err := roc.LoadRepoOwners(org, repo, pr.Base.Ref)
	if err != nil {
		return fmt.Errorf("error loading RepoOwners: %v", err)
	}

	for _, c := range modifiedOwnersFiles {
		path := filepath.Join(r.Dir, c.Filename)
		msg, owners := parseOwnersFile(oc, path, c, log, labelsBlackList)
		if msg != nil {
			wrongOwnersFiles[c.Filename] = *msg
			continue
		}

		if !skipTrustedUserCheck {
			nonTrustedUsers, err = nonTrustedUsersInOwners(ghc, log, triggerConfig, org, repo, c.Patch, c.Filename, owners, nonTrustedUsers, repoAliases)
			if err != nil {
				return err
			}
		}
	}

	// React if there are files with incorrect configs or non-trusted users.
	issueLabels, err := ghc.GetIssueLabels(org, repo, number)
	if err != nil {
		return err
	}
	hasInvalidOwnersLabel := github.HasLabel(labels.InvalidOwners, issueLabels)

	if len(wrongOwnersFiles) > 0 {
		s := "s"
		if len(wrongOwnersFiles) == 1 {
			s = ""
		}
		if !hasInvalidOwnersLabel {
			if err := ghc.AddLabel(org, repo, number, labels.InvalidOwners); err != nil {
				return err
			}
		}
		log.Debugf("Creating a review for %d %s file%s.", len(wrongOwnersFiles), ownersFileName, s)
		var comments []github.DraftReviewComment
		for errFile, err := range wrongOwnersFiles {
			comments = append(comments, github.DraftReviewComment{
				Path:     errFile,
				Body:     err.message,
				Position: err.line,
			})
		}
		// Make the review body.
		response := fmt.Sprintf("%d invalid %s file%s", len(wrongOwnersFiles), ownersFileName, s)
		draftReview := github.DraftReview{
			Body:     plugins.FormatResponseRaw(pre.PullRequest.Body, pre.PullRequest.HTMLURL, pre.PullRequest.User.Login, response),
			Action:   github.Comment,
			Comments: comments,
		}
		if pre.PullRequest.Head.SHA != "" {
			draftReview.CommitSHA = pre.PullRequest.Head.SHA
		}
		err := ghc.CreateReview(org, repo, pre.Number, draftReview)
		if err != nil {
			return fmt.Errorf("error creating a review for invalid %s file%s: %v", ownersFileName, s, err)
		}
	}

	if len(nonTrustedUsers) > 0 {
		if !hasInvalidOwnersLabel {
			if err := ghc.AddLabel(org, repo, number, labels.InvalidOwners); err != nil {
				return err
			}
		}

		// prune old comments before adding a new one
		cp.PruneComments(func(comment github.IssueComment) bool {
			return strings.Contains(comment.Body, fmt.Sprintf(nonCollaboratorResponseFormat, ownersFileName, org))
		})
		if err := ghc.CreateComment(org, repo, number, markdownFriendlyComment(org, nonTrustedUsers)); err != nil {
			log.WithError(err).Errorf("Could not create comment for listing non-collaborators in %s files", ownersFileName)
		}
	}

	if len(wrongOwnersFiles) == 0 && len(nonTrustedUsers) == 0 {
		// Don't bother checking if it has the label...it's a race, and we'll have
		// to handle failure due to not being labeled anyway.
		if err := ghc.RemoveLabel(org, repo, pre.Number, labels.InvalidOwners); err != nil {
			return fmt.Errorf("failed removing %s label: %v", labels.InvalidOwners, err)
		}
		cp.PruneComments(func(comment github.IssueComment) bool {
			return strings.Contains(comment.Body, fmt.Sprintf(nonCollaboratorResponseFormat, ownersFileName, org))
		})
	}

	return nil
}

func parseOwnersFile(oc ownersClient, path string, c github.PullRequestChange, log *logrus.Entry, labelsBlackList []string) (*messageWithLine, []string) {
	var reviewers []string
	var approvers []string
	var labels []string

	// by default we bind errors to line 1
	lineNumber := 1
	simple, err := oc.ParseSimpleConfig(path)
	if err == filepath.SkipDir {
		return nil, nil
	}
	if err != nil || simple.Empty() {
		full, err := oc.ParseFullConfig(path)
		if err == filepath.SkipDir {
			return nil, nil
		}
		if err != nil {
			lineNumberRe, _ := regexp.Compile(`line (\d+)`)
			lineNumberMatches := lineNumberRe.FindStringSubmatch(err.Error())
			// try to find a line number for the error
			if len(lineNumberMatches) > 1 {
				// we're sure it will convert as it passed the regexp already
				absoluteLineNumber, _ := strconv.Atoi(lineNumberMatches[1])
				// we need to convert it to a line number relative to the patch
				al, err := golint.AddedLines(c.Patch)
				if err != nil {
					log.WithError(err).Errorf("Failed to compute added lines in %s: %v", c.Filename, err)
				} else if val, ok := al[absoluteLineNumber]; ok {
					lineNumber = val
				}
			}
			return &messageWithLine{
				lineNumber,
				fmt.Sprintf("Cannot parse file: %v.", err),
			}, nil
		}
		// it's a FullConfig
		for _, config := range full.Filters {
			reviewers = append(reviewers, config.Reviewers...)
			approvers = append(approvers, config.Approvers...)
			labels = append(labels, config.Labels...)
		}
	} else {
		// it's a SimpleConfig
		reviewers = simple.Config.Reviewers
		approvers = simple.Config.Approvers
		labels = simple.Config.Labels
	}
	// Check labels against blacklist
	if sets.NewString(labels...).HasAny(labelsBlackList...) {
		return &messageWithLine{
			lineNumber,
			fmt.Sprintf("File contains blacklisted labels: %s.", sets.NewString(labels...).Intersection(sets.NewString(labelsBlackList...)).List()),
		}, nil
	}
	// Check approvers isn't empty
	if filepath.Dir(c.Filename) == "." && len(approvers) == 0 {
		return &messageWithLine{
			lineNumber,
			fmt.Sprintf("No approvers defined in this root directory %s file.", ownersFileName),
		}, nil
	}
	owners := append(reviewers, approvers...)
	return nil, owners
}

func markdownFriendlyComment(org string, nonTrustedUsers map[string][]string) string {
	var commentLines []string
	commentLines = append(commentLines, fmt.Sprintf(nonCollaboratorResponseFormat, ownersFileName, org))

	for user, ownersFiles := range nonTrustedUsers {
		commentLines = append(commentLines, fmt.Sprintf("- %s", user))
		for _, filename := range ownersFiles {
			commentLines = append(commentLines, fmt.Sprintf("  - %s", filename))
		}
	}
	return strings.Join(commentLines, "\n")
}

func nonTrustedUsersInOwnersAliases(ghc githubClient, log *logrus.Entry, triggerConfig plugins.Trigger, org, repo, dir, patch string, ownerAliasesModified, skipTrustedUserCheck bool) (map[string][]string, repoowners.RepoAliases, error) {
	repoAliases := make(repoowners.RepoAliases)
	// nonTrustedUsers is a map of non-trusted users to the list of files they are being added in
	nonTrustedUsers := map[string][]string{}
	var err error

	// If OWNERS_ALIASES exists, get all aliases.
	path := filepath.Join(dir, ownersAliasesFileName)
	if _, err := os.Stat(path); err == nil {
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return nonTrustedUsers, repoAliases, fmt.Errorf("Failed to read %s: %v", path, err)
		}
		repoAliases, err = repoowners.ParseAliasesConfig(b)
		if err != nil {
			return nonTrustedUsers, repoAliases, fmt.Errorf("error parsing aliases config for %s file: %v", ownersAliasesFileName, err)
		}
	}

	// If OWNERS_ALIASES file was modified, check if newly added owners are trusted.
	if ownerAliasesModified && !skipTrustedUserCheck {
		allOwners := repoAliases.ExpandAllAliases().List()
		for _, owner := range allOwners {
			// cap the number of checks to avoid exhausting tokens in case of large OWNERS refactors.
			if len(nonTrustedUsers) > 20 {
				break
			}
			nonTrustedUsers, err = checkIfTrustedUser(ghc, log, triggerConfig, owner, patch, ownersAliasesFileName, org, repo, nonTrustedUsers, repoAliases)
			if err != nil {
				return nonTrustedUsers, repoAliases, err
			}
		}
	}

	return nonTrustedUsers, repoAliases, nil
}

func nonTrustedUsersInOwners(ghc githubClient, log *logrus.Entry, triggerConfig plugins.Trigger, org, repo, patch, fileName string, owners []string, nonTrustedUsers map[string][]string, repoAliases repoowners.RepoAliases) (map[string][]string, error) {
	var err error
	for _, owner := range owners {
		// cap the number of checks to avoid exhausting tokens in case of large OWNERS refactors.
		if len(nonTrustedUsers) > 20 {
			break
		}

		// ignore if owner is an alias
		if _, ok := repoAliases[owner]; ok {
			continue
		}

		nonTrustedUsers, err = checkIfTrustedUser(ghc, log, triggerConfig, owner, patch, fileName, org, repo, nonTrustedUsers, repoAliases)
		if err != nil {
			return nonTrustedUsers, err
		}
	}
	return nonTrustedUsers, nil
}

// checkIfTrustedUser looks for newly addded owners by checking if they are in the patch
// and then checks if the owner is a trusted user.
func checkIfTrustedUser(ghc githubClient, log *logrus.Entry, triggerConfig plugins.Trigger, owner, patch, fileName, org, repo string, nonTrustedUsers map[string][]string, repoAliases repoowners.RepoAliases) (map[string][]string, error) {
	if strings.Contains(patch, owner) {
		isTrustedUser, err := trigger.TrustedUser(ghc, triggerConfig, owner, org, repo)
		if err != nil {
			return nonTrustedUsers, err
		}

		if !isTrustedUser {
			if ownersFiles, ok := nonTrustedUsers[owner]; ok {
				nonTrustedUsers[owner] = append(ownersFiles, fileName)
			} else {
				nonTrustedUsers[owner] = []string{fileName}
			}
		}
	}
	return nonTrustedUsers, nil
}
