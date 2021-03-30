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
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/golint"
	"k8s.io/test-infra/prow/plugins/ownersconfig"
	"k8s.io/test-infra/prow/plugins/trigger"
	"k8s.io/test-infra/prow/repoowners"
)

const (
	// PluginName defines this plugin's registered name.
	PluginName              = "verify-owners"
	untrustedResponseFormat = `The following users are mentioned in %s file(s) but are untrusted for the following reasons. One way to make the user trusted is to add them as [members](%s) of the %s org. You can then trigger verification by writing ` + "`/verify-owners`" + ` in a comment.`
)

type nonTrustedReasons struct {
	// files is a list of files they are being added in
	files []string
	// triggerReason is the reason that trigger's TrustedUser responds with for a failed trust check
	triggerReason string
}

var (
	verifyOwnersRe = regexp.MustCompile(`(?mi)^/verify-owners\s*$`)
)

func init() {
	plugins.RegisterPullRequestHandler(PluginName, handlePullRequest, helpProvider)
	plugins.RegisterGenericCommentHandler(PluginName, handleGenericCommentEvent, helpProvider)
}

func helpProvider(c *plugins.Configuration, orgRepo []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	pluginHelp := &pluginhelp.PluginHelp{
		Description: fmt.Sprintf("The verify-owners plugin validates %s and %s files (by default) and ensures that they always contain collaborators of the org, if they are modified in a PR. On validation failure it automatically adds the '%s' label to the PR, and a review comment on the incriminating file(s). Per-repo configuration for filenames is possible.", ownersconfig.DefaultOwnersFile, ownersconfig.DefaultOwnersAliasesFile, labels.InvalidOwners),
		Config:      map[string]string{},
	}
	defaultFilenames := c.OwnersFilenames("", "")
	descriptionFor := func(filenames ownersconfig.Filenames) string {
		description := fmt.Sprintf("%s and %s files are validated.", filenames.Owners, filenames.OwnersAliases)
		if c.Owners.LabelsDenyList != nil {
			description = fmt.Sprintf(`%s The verify-owners plugin will complain if %s files contain any of the following banned labels: %s.`,
				description,
				filenames.Owners,
				strings.Join(c.Owners.LabelsDenyList, ", "))
		}
		return description
	}
	pluginHelp.Config["default"] = descriptionFor(defaultFilenames)
	for _, item := range orgRepo {
		filenames := c.OwnersFilenames(item.Org, item.Repo)
		if !reflect.DeepEqual(filenames, defaultFilenames) {
			pluginHelp.Config[item.String()] = descriptionFor(filenames)
		}
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/verify-owners",
		Description: labels.InvalidOwners,
		Featured:    false,
		WhoCanUse:   "Anyone",
		Examples:    []string{"/verify-owners"},
	})
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

	cp, err := pc.CommentPruner()
	if err != nil {
		return err
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

	return handle(pc.GitHubClient, pc.GitClient, pc.OwnersClient, pc.Logger, &pre.PullRequest, prInfo, pc.PluginConfig.Owners.LabelsDenyList, pc.PluginConfig.TriggerFor(pre.Repo.Owner.Login, pre.Repo.Name), skipTrustedUserCheck, cp, pc.PluginConfig.OwnersFilenames)
}

func handleGenericCommentEvent(pc plugins.Agent, e github.GenericCommentEvent) error {
	cp, err := pc.CommentPruner()
	if err != nil {
		return err
	}

	var skipTrustedUserCheck bool
	for _, r := range pc.PluginConfig.Owners.SkipCollaborators {
		if r == e.Repo.FullName {
			skipTrustedUserCheck = true
			break
		}
	}

	return handleGenericComment(pc.GitHubClient, pc.GitClient, pc.OwnersClient, pc.Logger, &e, pc.PluginConfig.Owners.LabelsDenyList, pc.PluginConfig.TriggerFor(e.Repo.Owner.Login, e.Repo.Name), skipTrustedUserCheck, cp, pc.PluginConfig.OwnersFilenames)
}

func handleGenericComment(ghc githubClient, gc git.ClientFactory, roc repoownersClient, log *logrus.Entry, ce *github.GenericCommentEvent, bannedLabels []string, triggerConfig plugins.Trigger, skipTrustedUserCheck bool, cp commentPruner, resolver ownersconfig.Resolver) error {
	// Only consider open PRs and new comments.
	if ce.IssueState != "open" || !ce.IsPR || ce.Action != github.GenericCommentActionCreated {
		return nil
	}

	if !verifyOwnersRe.MatchString(ce.Body) {
		return nil
	}

	prInfo := info{
		org:          ce.Repo.Owner.Login,
		repo:         ce.Repo.Name,
		repoFullName: ce.Repo.FullName,
		number:       ce.Number,
	}

	pr, err := ghc.GetPullRequest(ce.Repo.Owner.Login, ce.Repo.Name, ce.Number)
	if err != nil {
		return err
	}

	return handle(ghc, gc, roc, log, pr, prInfo, bannedLabels, triggerConfig, skipTrustedUserCheck, cp, resolver)
}

type messageWithLine struct {
	line    int
	message string
}

func handle(ghc githubClient, gc git.ClientFactory, roc repoownersClient, log *logrus.Entry, pr *github.PullRequest, info info, bannedLabels []string, triggerConfig plugins.Trigger, skipTrustedUserCheck bool, cp commentPruner, resolver ownersconfig.Resolver) error {
	org := info.org
	repo := info.repo
	number := info.number
	filenames := resolver(org, repo)
	wrongOwnersFiles := map[string]messageWithLine{}

	// Get changes.
	changes, err := ghc.GetPullRequestChanges(org, repo, number)
	if err != nil {
		return fmt.Errorf("error getting PR changes: %v", err)
	}

	// List modified OWNERS files.
	var modifiedOwnersFiles []github.PullRequestChange
	for _, change := range changes {
		if filepath.Base(change.Filename) == filenames.Owners && change.Status != github.PullRequestFileRemoved {
			modifiedOwnersFiles = append(modifiedOwnersFiles, change)
		}
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

	issueLabels, err := ghc.GetIssueLabels(org, repo, number)
	if err != nil {
		return err
	}
	hasInvalidOwnersLabel := github.HasLabel(labels.InvalidOwners, issueLabels)

	if len(modifiedOwnersFiles) == 0 && !ownerAliasesModified && !hasInvalidOwnersLabel {
		return nil
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
	// If the file was modified, check for non trusted users in the newly added owners.
	nonTrustedUsers, trustedUsers, repoAliases, err := nonTrustedUsersInOwnersAliases(ghc, log, triggerConfig, org, repo, r.Directory(), modifiedOwnerAliasesFile.Patch, ownerAliasesModified, skipTrustedUserCheck, filenames)
	if err != nil {
		return err
	}

	// Check if OWNERS files have the correct config and if they do,
	// check if all newly added owners are trusted users.
	oc, err := roc.LoadRepoOwners(org, repo, pr.Base.Ref)
	if err != nil {
		return fmt.Errorf("error loading RepoOwners: %v", err)
	}

	for _, c := range modifiedOwnersFiles {
		path := filepath.Join(r.Directory(), c.Filename)
		msg, owners := parseOwnersFile(oc, path, c, log, bannedLabels, filenames)
		if msg != nil {
			wrongOwnersFiles[c.Filename] = *msg
			continue
		}

		if !skipTrustedUserCheck {
			nonTrustedUsers, err = nonTrustedUsersInOwners(ghc, log, triggerConfig, org, repo, c.Patch, c.Filename, owners, nonTrustedUsers, trustedUsers, repoAliases)
			if err != nil {
				return err
			}
		}
	}

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
		log.Debugf("Creating a review for %d %s file%s.", len(wrongOwnersFiles), filenames.Owners, s)
		var comments []github.DraftReviewComment
		for errFile, err := range wrongOwnersFiles {
			comments = append(comments, github.DraftReviewComment{
				Path:     errFile,
				Body:     err.message,
				Position: err.line,
			})
		}
		// Make the review body.
		response := fmt.Sprintf("%d invalid %s file%s", len(wrongOwnersFiles), filenames.Owners, s)
		draftReview := github.DraftReview{
			Body:     plugins.FormatResponseRaw(pr.Body, pr.HTMLURL, pr.User.Login, response),
			Action:   github.Comment,
			Comments: comments,
		}
		if pr.Head.SHA != "" {
			draftReview.CommitSHA = pr.Head.SHA
		}
		err := ghc.CreateReview(org, repo, number, draftReview)
		if err != nil {
			return fmt.Errorf("error creating a review for invalid %s file%s: %v", filenames.Owners, s, err)
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
			return strings.Contains(comment.Body, fmt.Sprintf(untrustedResponseFormat, filenames.Owners, triggerConfig.JoinOrgURL, org))
		})
		if err := ghc.CreateComment(org, repo, number, markdownFriendlyComment(org, triggerConfig.JoinOrgURL, nonTrustedUsers, filenames)); err != nil {
			log.WithError(err).Errorf("Could not create comment for listing non-collaborators in %s files", filenames.Owners)
		}
	}

	if len(wrongOwnersFiles) == 0 && len(nonTrustedUsers) == 0 {
		// Don't bother checking if it has the label...it's a race, and we'll have
		// to handle failure due to not being labeled anyway.
		if err := ghc.RemoveLabel(org, repo, number, labels.InvalidOwners); err != nil {
			return fmt.Errorf("failed removing %s label: %v", labels.InvalidOwners, err)
		}
		cp.PruneComments(func(comment github.IssueComment) bool {
			return strings.Contains(comment.Body, fmt.Sprintf(untrustedResponseFormat, filenames.Owners, triggerConfig.JoinOrgURL, org))
		})
	}

	return nil
}

func parseOwnersFile(oc ownersClient, path string, c github.PullRequestChange, log *logrus.Entry, bannedLabels []string, filenames ownersconfig.Filenames) (*messageWithLine, []string) {
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
	// Check labels against ban list
	if sets.NewString(labels...).HasAny(bannedLabels...) {
		return &messageWithLine{
			lineNumber,
			fmt.Sprintf("File contains banned labels: %s.", sets.NewString(labels...).Intersection(sets.NewString(bannedLabels...)).List()),
		}, nil
	}
	// Check approvers isn't empty
	if filepath.Dir(c.Filename) == "." && len(approvers) == 0 {
		return &messageWithLine{
			lineNumber,
			fmt.Sprintf("No approvers defined in this root directory %s file.", filenames.Owners),
		}, nil
	}
	owners := append(reviewers, approvers...)
	return nil, owners
}

func markdownFriendlyComment(org, joinOrgURL string, nonTrustedUsers map[string]nonTrustedReasons, filenames ownersconfig.Filenames) string {
	var commentLines []string
	commentLines = append(commentLines, fmt.Sprintf(untrustedResponseFormat, filenames.Owners, joinOrgURL, org))

	for user, reasons := range nonTrustedUsers {
		commentLines = append(commentLines, fmt.Sprintf("- %s", user))
		commentLines = append(commentLines, fmt.Sprintf("  - %s", reasons.triggerReason))
		for _, filename := range reasons.files {
			commentLines = append(commentLines, fmt.Sprintf("  - %s", filename))
		}
	}
	return strings.Join(commentLines, "\n")
}

func nonTrustedUsersInOwnersAliases(ghc githubClient, log *logrus.Entry, triggerConfig plugins.Trigger, org, repo, dir, patch string, ownerAliasesModified, skipTrustedUserCheck bool, filenames ownersconfig.Filenames) (map[string]nonTrustedReasons, sets.String, repoowners.RepoAliases, error) {
	repoAliases := make(repoowners.RepoAliases)
	// nonTrustedUsers is a map of non-trusted users to the reasons they were not trusted
	nonTrustedUsers := map[string]nonTrustedReasons{}
	trustedUsers := sets.String{}
	var err error

	// If OWNERS_ALIASES exists, get all aliases.
	path := filepath.Join(dir, filenames.OwnersAliases)
	if _, err := os.Stat(path); err == nil {
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return nonTrustedUsers, trustedUsers, repoAliases, fmt.Errorf("Failed to read %s: %v", path, err)
		}
		repoAliases, err = repoowners.ParseAliasesConfig(b)
		if err != nil {
			return nonTrustedUsers, trustedUsers, repoAliases, fmt.Errorf("error parsing aliases config for %s file: %v", filenames.OwnersAliases, err)
		}
	}

	// If OWNERS_ALIASES file was modified, check if newly added owners are trusted.
	if ownerAliasesModified && !skipTrustedUserCheck {
		allOwners := repoAliases.ExpandAllAliases().List()
		for _, owner := range allOwners {
			nonTrustedUsers, err = checkIfTrustedUser(ghc, log, triggerConfig, owner, patch, filenames.OwnersAliases, org, repo, nonTrustedUsers, trustedUsers, repoAliases)
			if err != nil {
				return nonTrustedUsers, trustedUsers, repoAliases, err
			}
		}
	}

	return nonTrustedUsers, trustedUsers, repoAliases, nil
}

func nonTrustedUsersInOwners(ghc githubClient, log *logrus.Entry, triggerConfig plugins.Trigger, org, repo, patch, fileName string, owners []string, nonTrustedUsers map[string]nonTrustedReasons, trustedUsers sets.String, repoAliases repoowners.RepoAliases) (map[string]nonTrustedReasons, error) {
	var err error
	for _, owner := range owners {
		// ignore if owner is an alias
		if _, ok := repoAliases[owner]; ok {
			continue
		}

		nonTrustedUsers, err = checkIfTrustedUser(ghc, log, triggerConfig, owner, patch, fileName, org, repo, nonTrustedUsers, trustedUsers, repoAliases)
		if err != nil {
			return nonTrustedUsers, err
		}
	}
	return nonTrustedUsers, nil
}

// checkIfTrustedUser looks for newly addded owners by checking if they are in the patch
// and then checks if the owner is a trusted user.
// returns a map from user to reasons for not being trusted
func checkIfTrustedUser(ghc githubClient, log *logrus.Entry, triggerConfig plugins.Trigger, owner, patch, fileName, org, repo string, nonTrustedUsers map[string]nonTrustedReasons, trustedUsers sets.String, repoAliases repoowners.RepoAliases) (map[string]nonTrustedReasons, error) {
	// cap the number of checks to avoid exhausting tokens in case of large OWNERS refactors.
	if len(nonTrustedUsers)+trustedUsers.Len() > 50 {
		return nonTrustedUsers, nil
	}
	// only consider owners in the current patch
	newOwnerRe, _ := regexp.Compile(fmt.Sprintf(`\+\s*-\s*\b%s\b`, owner))
	if !newOwnerRe.MatchString(patch) {
		return nonTrustedUsers, nil
	}

	// if we already flagged the owner for the current file, return early
	if reasons, ok := nonTrustedUsers[owner]; ok {
		for _, file := range reasons.files {
			if file == fileName {
				return nonTrustedUsers, nil
			}
		}
		// have to separate assignment from map update due to map implementation (see "index expressions")
		reasons.files = append(reasons.files, fileName)
		nonTrustedUsers[owner] = reasons
		return nonTrustedUsers, nil
	}

	isAlreadyTrusted := trustedUsers.Has(owner)
	var err error
	var triggerTrustedResponse trigger.TrustedUserResponse
	if !isAlreadyTrusted {
		triggerTrustedResponse, err = trigger.TrustedUser(ghc, triggerConfig.OnlyOrgMembers, triggerConfig.TrustedOrg, owner, org, repo)
		if err != nil {
			return nonTrustedUsers, err
		}
	}

	if !isAlreadyTrusted && triggerTrustedResponse.IsTrusted {
		trustedUsers.Insert(owner)
	} else if !isAlreadyTrusted && !triggerTrustedResponse.IsTrusted {
		if reasons, ok := nonTrustedUsers[owner]; ok {
			reasons.triggerReason = triggerTrustedResponse.Reason
			nonTrustedUsers[owner] = reasons
		} else {
			nonTrustedUsers[owner] = nonTrustedReasons{
				// ensure that files is initialized to avoid nil pointer
				files:         []string{},
				triggerReason: triggerTrustedResponse.Reason,
			}
		}
	}

	return nonTrustedUsers, nil
}
