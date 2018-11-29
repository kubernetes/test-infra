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

package releasenote

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const PluginName = "release-note"

const (
	ReleaseNoteLabelNeeded    = "do-not-merge/release-note-label-needed"
	releaseNote               = "release-note"
	releaseNoteNone           = "release-note-none"
	releaseNoteActionRequired = "release-note-action-required"

	releaseNoteFormat       = `Adding the "%s" label because no release-note block was detected, please follow our [release note process](https://git.k8s.io/community/contributors/guide/release-notes.md) to remove it.`
	parentReleaseNoteFormat = `All 'parent' PRs of a cherry-pick PR must have one of the %q or %q labels, or this PR must follow the standard/parent release note labeling requirement.`

	actionRequiredNote = "action required"
)

var (
	releaseNoteBody       = fmt.Sprintf(releaseNoteFormat, ReleaseNoteLabelNeeded)
	parentReleaseNoteBody = fmt.Sprintf(parentReleaseNoteFormat, releaseNote, releaseNoteActionRequired)

	noteMatcherRE = regexp.MustCompile(`(?s)(?:Release note\*\*:\s*(?:<!--[^<>]*-->\s*)?` + "```(?:release-note)?|```release-note)(.+?)```")
	cpRe          = regexp.MustCompile(`Cherry pick of #([[:digit:]]+) on release-([[:digit:]]+\.[[:digit:]]+).`)
	noneRe        = regexp.MustCompile(`(?i)^\W*NONE\W*$`)

	allRNLabels = []string{
		releaseNoteNone,
		releaseNoteActionRequired,
		ReleaseNoteLabelNeeded,
		releaseNote,
	}

	releaseNoteRe               = regexp.MustCompile(`(?mi)^/release-note\s*$`)
	releaseNoteNoneRe           = regexp.MustCompile(`(?mi)^/release-note-none\s*$`)
	releaseNoteActionRequiredRe = regexp.MustCompile(`(?mi)^/release-note-action-required\s*$`)
)

func init() {
	plugins.RegisterIssueCommentHandler(PluginName, handleIssueComment, helpProvider)
	plugins.RegisterPullRequestHandler(PluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	pluginHelp := &pluginhelp.PluginHelp{
		Description: `The releasenote plugin implements a release note process that uses a markdown 'releasenote' code block to associate a release note with a pull request. Until the 'releasenote' block in the pull request body is populated the PR will be assigned the '` + ReleaseNoteLabelNeeded + `' label.
<br>There are three valid types of release notes that can replace this label:
<ol><li>PRs with a normal release note in the 'releasenote' block are given the label '` + releaseNote + `'.</li>
<li>PRs that have a release note of 'none' in the block are given the label '` + releaseNoteNone + `' to indicate that the PR does not warrant a release note.</li>
<li>PRs that contain 'action required' in their 'releasenote' block are given the label '` + releaseNoteActionRequired + `' to indicate that the PR introduces potentially breaking changes that necessitate user action before upgrading to the release.</li></ol>
` + "To use the plugin, in the pull request body text:\n\n```releasenote\n<release note content>\n```",
	}
	// NOTE: the other two commands re deprecated, so we're not documenting them
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/release-note-none",
		Description: "Adds the '" + releaseNoteNone + `' label to indicate that the PR does not warrant a release note. This is deprecated and ideally <a href="https://git.k8s.io/community/contributors/guide/release-notes.md">the release note process</a> should be followed in the PR body instead.`,
		WhoCanUse:   "PR Authors and Org Members.",
		Examples:    []string{"/release-note-none"},
	})
	return pluginHelp, nil
}

type githubClient interface {
	IsMember(org, user string) (bool, error)
	CreateComment(owner, repo string, number int, comment string) error
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
	DeleteStaleComments(org, repo string, number int, comments []github.IssueComment, isStale func(github.IssueComment) bool) error
	BotName() (string, error)
}

func handleIssueComment(pc plugins.Agent, ic github.IssueCommentEvent) error {
	return handleComment(pc.GitHubClient, pc.Logger, ic)
}

func handleComment(gc githubClient, log *logrus.Entry, ic github.IssueCommentEvent) error {
	// Only consider PRs and new comments.
	if !ic.Issue.IsPullRequest() || ic.Action != github.IssueCommentActionCreated {
		return nil
	}

	org := ic.Repo.Owner.Login
	repo := ic.Repo.Name
	number := ic.Issue.Number

	// Which label does the comment want us to add?
	var nl string
	switch {
	case releaseNoteRe.MatchString(ic.Comment.Body):
		nl = releaseNote
	case releaseNoteNoneRe.MatchString(ic.Comment.Body):
		nl = releaseNoteNone
	case releaseNoteActionRequiredRe.MatchString(ic.Comment.Body):
		nl = releaseNoteActionRequired
	default:
		return nil
	}

	// Emit deprecation warning for /release-note and /release-note-action-required.
	if nl == releaseNote || nl == releaseNoteActionRequired {
		format := "the `/%s` and `/%s` commands have been deprecated.\nPlease edit the `release-note` block in the PR body text to include the release note. If the release note requires additional action include the string `action required` in the release note. For example:\n````\n```release-note\nSome release note with action required.\n```\n````"
		resp := fmt.Sprintf(format, releaseNote, releaseNoteActionRequired)
		return gc.CreateComment(org, repo, number, plugins.FormatICResponse(ic.Comment, resp))
	}

	// Only allow authors and org members to add labels.
	isMember, err := gc.IsMember(ic.Repo.Owner.Login, ic.Comment.User.Login)
	if err != nil {
		return err
	}

	isAuthor := ic.Issue.IsAuthor(ic.Comment.User.Login)

	if !isMember && !isAuthor {
		format := "you can only set the release note label to %s if you are the PR author or an org member."
		resp := fmt.Sprintf(format, releaseNoteNone)
		return gc.CreateComment(org, repo, number, plugins.FormatICResponse(ic.Comment, resp))
	}

	// Don't allow the /release-note-none command if the release-note block contains a valid release note.
	blockNL := determineReleaseNoteLabel(ic.Issue.Body)
	if blockNL == releaseNote || blockNL == releaseNoteActionRequired {
		format := "you can only set the release note label to %s if the release-note block in the PR body text is empty or \"none\"."
		resp := fmt.Sprintf(format, releaseNoteNone)
		return gc.CreateComment(org, repo, number, plugins.FormatICResponse(ic.Comment, resp))
	}
	if !ic.Issue.HasLabel(releaseNoteNone) {
		if err := gc.AddLabel(org, repo, number, releaseNoteNone); err != nil {
			return err
		}
	}

	labels := sets.String{}
	for _, label := range ic.Issue.Labels {
		labels.Insert(label.Name)
	}
	// Remove all other release-note-* labels if necessary.
	return removeOtherLabels(
		func(l string) error {
			return gc.RemoveLabel(org, repo, number, l)
		},
		releaseNoteNone,
		allRNLabels,
		labels,
	)
}

func removeOtherLabels(remover func(string) error, label string, labelSet []string, currentLabels sets.String) error {
	var errs []error
	for _, elem := range labelSet {
		if elem != label && currentLabels.Has(elem) {
			if err := remover(elem); err != nil {
				errs = append(errs, err)
			}
			currentLabels.Delete(elem)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("encountered %d errors setting labels: %v", len(errs), errs)
	}
	return nil
}

func handlePullRequest(pc plugins.Agent, pr github.PullRequestEvent) error {
	return handlePR(pc.GitHubClient, pc.Logger, &pr)
}

func handlePR(gc githubClient, log *logrus.Entry, pr *github.PullRequestEvent) error {
	// Only consider events that edit the PR body.
	if pr.Action != github.PullRequestActionOpened && pr.Action != github.PullRequestActionEdited {
		return nil
	}
	org := pr.Repo.Owner.Login
	repo := pr.Repo.Name

	prInitLabels, err := gc.GetIssueLabels(org, repo, pr.Number)
	if err != nil {
		return fmt.Errorf("failed to list labels on PR #%d. err: %v", pr.Number, err)
	}
	prLabels := sets.String{}
	for _, label := range prInitLabels {
		prLabels.Insert(label.Name)
	}

	var comments []github.IssueComment
	labelToAdd := determineReleaseNoteLabel(pr.PullRequest.Body)
	if labelToAdd == ReleaseNoteLabelNeeded {
		if !prMustFollowRelNoteProcess(gc, log, pr, prLabels, true) {
			ensureNoRelNoteNeededLabel(gc, log, pr, prLabels)
			return clearStaleComments(gc, log, pr, prLabels, nil)
		}
		comments, err = gc.ListIssueComments(org, repo, pr.Number)
		if err != nil {
			return fmt.Errorf("failed to list comments on %s/%s#%d. err: %v", org, repo, pr.Number, err)
		}
		if containsNoneCommand(comments) {
			labelToAdd = releaseNoteNone
		} else if !prLabels.Has(ReleaseNoteLabelNeeded) {
			comment := plugins.FormatSimpleResponse(pr.PullRequest.User.Login, releaseNoteBody)
			if err := gc.CreateComment(org, repo, pr.Number, comment); err != nil {
				log.WithError(err).Errorf("Failed to comment on %s/%s#%d with comment %q.", org, repo, pr.Number, comment)
			}
		}
	}

	// Add the label if needed
	if !prLabels.Has(labelToAdd) {
		if err = gc.AddLabel(org, repo, pr.Number, labelToAdd); err != nil {
			return err
		}
		prLabels.Insert(labelToAdd)
	}

	err = removeOtherLabels(
		func(l string) error {
			return gc.RemoveLabel(org, repo, pr.Number, l)
		},
		labelToAdd,
		allRNLabels,
		prLabels,
	)
	if err != nil {
		log.Error(err)
	}

	return clearStaleComments(gc, log, pr, prLabels, comments)
}

// clearStaleComments deletes old comments that are no longer applicable.
func clearStaleComments(gc githubClient, log *logrus.Entry, pr *github.PullRequestEvent, prLabels sets.String, comments []github.IssueComment) error {
	// If the PR must follow the process and hasn't yet completed the process, don't remove comments.
	if prMustFollowRelNoteProcess(gc, log, pr, prLabels, false) && !releaseNoteAlreadyAdded(prLabels) {
		return nil
	}
	botName, err := gc.BotName()
	if err != nil {
		return err
	}
	return gc.DeleteStaleComments(
		pr.Repo.Owner.Login,
		pr.Repo.Name,
		pr.Number,
		comments,
		func(c github.IssueComment) bool { // isStale function
			return c.User.Login == botName &&
				(strings.Contains(c.Body, releaseNoteBody) ||
					strings.Contains(c.Body, parentReleaseNoteBody))
		},
	)
}

func containsNoneCommand(comments []github.IssueComment) bool {
	for _, c := range comments {
		if releaseNoteNoneRe.MatchString(c.Body) {
			return true
		}
	}
	return false
}

func ensureNoRelNoteNeededLabel(gc githubClient, log *logrus.Entry, pr *github.PullRequestEvent, prLabels sets.String) {
	org := pr.Repo.Owner.Login
	repo := pr.Repo.Name
	format := "Failed to remove the label %q from %s/%s#%d."
	if prLabels.Has(ReleaseNoteLabelNeeded) {
		if err := gc.RemoveLabel(org, repo, pr.Number, ReleaseNoteLabelNeeded); err != nil {
			log.WithError(err).Errorf(format, ReleaseNoteLabelNeeded, org, repo, pr.Number)
		}
	}
}

// determineReleaseNoteLabel returns the label to be added based on the contents of the 'release-note'
// section of a PR's body text.
func determineReleaseNoteLabel(body string) string {
	composedReleaseNote := strings.ToLower(strings.TrimSpace(getReleaseNote(body)))

	if composedReleaseNote == "" {
		return ReleaseNoteLabelNeeded
	}
	if noneRe.MatchString(composedReleaseNote) {
		return releaseNoteNone
	}
	if strings.Contains(composedReleaseNote, actionRequiredNote) {
		return releaseNoteActionRequired
	}
	return releaseNote
}

// getReleaseNote returns the release note from a PR body
// assumes that the PR body followed the PR template
func getReleaseNote(body string) string {
	potentialMatch := noteMatcherRE.FindStringSubmatch(body)
	if potentialMatch == nil {
		return ""
	}
	return strings.TrimSpace(potentialMatch[1])
}

func releaseNoteAlreadyAdded(prLabels sets.String) bool {
	return prLabels.HasAny(releaseNote, releaseNoteActionRequired, releaseNoteNone)
}

func prMustFollowRelNoteProcess(gc githubClient, log *logrus.Entry, pr *github.PullRequestEvent, prLabels sets.String, comment bool) bool {
	if pr.PullRequest.Base.Ref == "master" {
		return true
	}

	parents := getCherrypickParentPRNums(pr.PullRequest.Body)
	// if it has no parents it needs to follow the release note process
	if len(parents) == 0 {
		return true
	}

	org := pr.Repo.Owner.Login
	repo := pr.Repo.Name

	var notelessParents []string
	for _, parent := range parents {
		// If the parent didn't set a release note, the CP must
		parentLabels, err := gc.GetIssueLabels(org, repo, parent)
		if err != nil {
			log.WithError(err).Errorf("Failed to list labels on PR #%d (parent of #%d).", parent, pr.Number)
			continue
		}
		if !github.HasLabel(releaseNote, parentLabels) &&
			!github.HasLabel(releaseNoteActionRequired, parentLabels) {
			notelessParents = append(notelessParents, "#"+strconv.Itoa(parent))
		}
	}
	if len(notelessParents) == 0 {
		// All of the parents set the releaseNote or releaseNoteActionRequired label,
		// so this cherrypick PR needs to do nothing.
		return false
	}

	if comment && !prLabels.Has(ReleaseNoteLabelNeeded) {
		comment := plugins.FormatResponse(
			pr.PullRequest.User.Login,
			parentReleaseNoteBody,
			fmt.Sprintf("The following parent PRs have neither the %q nor the %q labels: %s.",
				releaseNote,
				releaseNoteActionRequired,
				strings.Join(notelessParents, ", "),
			),
		)
		if err := gc.CreateComment(org, repo, pr.Number, comment); err != nil {
			log.WithError(err).Errorf("Error creating comment on %s/%s#%d with comment %q.", org, repo, pr.Number, comment)
		}
	}
	return true
}

func getCherrypickParentPRNums(body string) []int {
	lines := strings.Split(body, "\n")

	var out []int
	for _, line := range lines {
		matches := cpRe.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}
		parentNum, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}
		out = append(out, parentNum)
	}
	return out
}
