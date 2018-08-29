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

package dco

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"regexp"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName               = "dco"
	dcoContextName           = "dco"
	dcoContextMessageFailed  = "A commit in PR is missing Signed-off-by"
	dcoContextMessageSuccess = "All commits have Signed-off-by"

	dcoYesLabel        = "dco-signoff: yes"
	dcoNoLabel         = "dco-signoff: no"
	dcoMsgPruneMatch   = "Thanks for your pull request. Before we can look at your pull request, you'll need to add a 'DCO signoff' to your commits."
	dcoNotFoundMessage = `Thanks for your pull request. Before we can look at your pull request, you'll need to add a 'DCO signoff' to your commits.

<details>

%s
</details>
`
)

var (
	checkDCORe = regexp.MustCompile(`(?mi)^/check-dco\s*$`)
	testRe     = regexp.MustCompile(`(?mi)^signed-off-by:`)
)

func init() {
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequestEvent, helpProvider)
	plugins.RegisterGenericCommentHandler(pluginName, handleCommentEvent, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	// The {WhoCanUse, Usage, Examples, Config} fields are omitted because this plugin cannot be
	// manually triggered and is not configurable.
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The dco plugin checks pull request commits for 'DCO sign off' and maintains the '" + dcoContextName + "' status context, as well as the 'dco' label.",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/check-dco",
		Description: "Forces rechecking of the DCO status.",
		Featured:    true,
		WhoCanUse:   "Anyone",
		Examples:    []string{"/check-dco"},
	})
	return pluginHelp, nil
}

type gitHubClient interface {
	BotName() (string, error)
	CreateComment(owner, repo string, number int, comment string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
	ListStatuses(org, repo, ref string) ([]github.Status, error)
	CreateStatus(owner, repo, ref string, status github.Status) error
	ListPRCommits(org, repo string, number int) ([]github.RepositoryCommit, error)
	GetPullRequest(owner, repo string, number int) (*github.PullRequest, error)
}

type commentPruner interface {
	PruneComments(shouldPrune func(github.IssueComment) bool)
}

// checkCommitMessages will perform the actual DCO check by retrieving all
// commits contained within the PR with the given number.
// *All* commits in the pull request *must* match the 'testRe' in order to pass.
func checkCommitMessages(gc gitHubClient, l *logrus.Entry, org, repo string, number int) (bool, error) {
	allCommits, err := gc.ListPRCommits(org, repo, number)
	if err != nil {
		return false, fmt.Errorf("error listing commits for pull request: %v", err)
	}
	l.Infof("Found %d commits in PR", len(allCommits))

	signMissing := false
	for _, commit := range allCommits {
		if !testRe.MatchString(*commit.Commit.Message) {
			signMissing = true
			break
		}
	}

	l.Infof("All commits in PR have DCO signoff: %t", !signMissing)
	return !signMissing, nil
}

// checkExistingStatus will retrieve the current status of the DCO context for
// the provided SHA.
func checkExistingStatus(gc gitHubClient, l *logrus.Entry, org, repo, sha string) (string, error) {
	statuses, err := gc.ListStatuses(org, repo, sha)
	if err != nil {
		return "", fmt.Errorf("error listing pull request statuses: %v", err)
	}

	existingStatus := ""
	for _, status := range statuses {
		if status.Context != dcoContextName {
			continue
		}
		existingStatus = status.State
		break
	}
	l.Infof("Existing DCO status context status is %q", existingStatus)
	return existingStatus, nil
}

// checkExistingLabels will check the provided PR for the dco sign off labels,
// returning bool's indicating whether the 'yes' and the 'no' label are present.
func checkExistingLabels(gc gitHubClient, l *logrus.Entry, org, repo string, number int) (hasYesLabel, hasNoLabel bool, err error) {
	labels, err := gc.GetIssueLabels(org, repo, number)
	if err != nil {
		return false, false, fmt.Errorf("error getting pull request labels: %v", err)
	}

	for _, l := range labels {
		if l.Name == dcoYesLabel {
			hasYesLabel = true
		}
		if l.Name == dcoNoLabel {
			hasNoLabel = true
		}
	}

	return hasYesLabel, hasNoLabel, nil
}

// takeAction will take appropriate action on the pull request according to its
// current state.
func takeAction(gc gitHubClient, cp commentPruner, l *logrus.Entry, org, repo string, pr github.PullRequest, signedOff bool, existingStatus string, hasYesLabel, hasNoLabel bool) error {
	targetURL := fmt.Sprintf("https://github.com/%s/%s/blob/master/CONTRIBUTING.md", org, repo)
	botName, err := gc.BotName()
	if err != nil {
		return fmt.Errorf("failed to get bot name: %v", err)
	}

	// handle the 'all commits signed off' case by adding appropriate labels
	// TODO: clean-up old comments?
	if signedOff {
		if hasNoLabel {
			l.Infof("Removing %q label", dcoNoLabel)
			// remove 'dco-signoff: no' label
			if err := gc.RemoveLabel(org, repo, pr.Number, dcoNoLabel); err != nil {
				return fmt.Errorf("error removing label: %v", err)
			}
		}
		if !hasYesLabel {
			l.Infof("Adding %q label", dcoYesLabel)
			// add 'dco-signoff: yes' label
			if err := gc.AddLabel(org, repo, pr.Number, dcoYesLabel); err != nil {
				return fmt.Errorf("error adding label: %v", err)
			}
		}
		if existingStatus != github.StatusSuccess {
			l.Infof("Setting DCO status context to succeeded")
			if err := gc.CreateStatus(org, repo, pr.Head.SHA, github.Status{
				Context:     dcoContextName,
				State:       github.StatusSuccess,
				TargetURL:   targetURL,
				Description: dcoContextMessageSuccess,
			}); err != nil {
				return fmt.Errorf("error setting pull request status: %v", err)
			}
		}

		cp.PruneComments(shouldPrune(l, botName))
		return nil
	}

	// handle the 'not all commits signed off' case

	// we handle this 'base case' early on to avoid adding more comments when
	// they are not needed.
	if hasNoLabel && !hasYesLabel && existingStatus == github.StatusFailure {
		l.Infof("DCO status context and label already up to date")
		return nil
	}

	if !hasNoLabel {
		l.Infof("Adding %q label", dcoNoLabel)
		// add 'dco-signoff: no' label
		if err := gc.AddLabel(org, repo, pr.Number, dcoNoLabel); err != nil {
			return fmt.Errorf("error adding label: %v", err)
		}
	}
	if hasYesLabel {
		l.Infof("Removing %q label", dcoYesLabel)
		// remove 'dco-signoff: yes' label
		if err := gc.RemoveLabel(org, repo, pr.Number, dcoYesLabel); err != nil {
			return fmt.Errorf("error removing label: %v", err)
		}
	}
	if existingStatus != github.StatusFailure {
		l.Infof("Setting DCO status context to failed")
		if err := gc.CreateStatus(org, repo, pr.Head.SHA, github.Status{
			Context:     dcoContextName,
			State:       github.StatusFailure,
			TargetURL:   targetURL,
			Description: dcoContextMessageFailed,
		}); err != nil {
			return fmt.Errorf("error setting pull request status: %v", err)
		}
	}

	cp.PruneComments(shouldPrune(l, botName))
	l.Infof("Commenting on PR to advise users of DCO check")
	if err := gc.CreateComment(org, repo, pr.Number, fmt.Sprintf(dcoNotFoundMessage, plugins.AboutThisBot)); err != nil {
		l.WithError(err).Warning("Could not create DCO not found comment.")
	}

	return nil
}

// 1. Check commit messages in the pull request for the sign-off string
// 2. Check the existing status context value
// 3. Check the existing PR labels
// 4. If signed off, apply appropriate labels and status context.
// 5. If not signed off, apply appropriate labels and status context and add a comment.
func handle(gc gitHubClient, cp commentPruner, log *logrus.Entry, org, repo string, pr github.PullRequest) error {
	l := log.WithField("pr", pr.Number)

	signedOff, err := checkCommitMessages(gc, l, org, repo, pr.Number)
	if err != nil {
		l.WithError(err).Infof("Error running DCO check against commits in PR")
		return err
	}

	existingStatus, err := checkExistingStatus(gc, l, org, repo, pr.Head.SHA)
	if err != nil {
		l.WithError(err).Infof("Error checking existing PR status")
		return err
	}

	hasYesLabel, hasNoLabel, err := checkExistingLabels(gc, l, org, repo, pr.Number)
	if err != nil {
		l.WithError(err).Infof("Error checking existing PR labels")
		return err
	}

	return takeAction(gc, cp, l, org, repo, pr, signedOff, existingStatus, hasYesLabel, hasNoLabel)
}

// shouldPrune finds comments left by this plugin.
func shouldPrune(log *logrus.Entry, botName string) func(github.IssueComment) bool {
	return func(comment github.IssueComment) bool {
		if comment.User.Login != botName {
			return false
		}
		return strings.Contains(comment.Body, dcoMsgPruneMatch)
	}
}

func handlePullRequestEvent(pc plugins.PluginClient, pe github.PullRequestEvent) error {
	org := pe.Repo.Owner.Login
	repo := pe.Repo.Name

	// we only reprocess on label, unlabel, open, reopen and synchronize events
	// this will reduce our API token usage and save processing of unrelated events
	switch pe.Action {
	case github.PullRequestActionLabeled,
		github.PullRequestActionUnlabeled,
		github.PullRequestActionOpened,
		github.PullRequestActionReopened,
		github.PullRequestActionSynchronize:
	default:
		return nil
	}

	return handle(pc.GitHubClient, pc.CommentPruner, pc.Logger, org, repo, pe.PullRequest)
}

func handleCommentEvent(pc plugins.PluginClient, ce github.GenericCommentEvent) error {
	// Only consider open PRs and new comments.
	if ce.IssueState != "open" || ce.Action != github.GenericCommentActionCreated || !ce.IsPR {
		return nil
	}
	// Only consider "/check-dco" comments.
	if !checkDCORe.MatchString(ce.Body) {
		return nil
	}

	gc := pc.GitHubClient
	org := ce.Repo.Owner.Login
	repo := ce.Repo.Name

	pr, err := gc.GetPullRequest(org, repo, ce.Number)
	if err != nil {
		return fmt.Errorf("error getting pull request for comment: %v", err)
	}

	return handle(gc, pc.CommentPruner, pc.Logger, org, repo, *pr)

}
