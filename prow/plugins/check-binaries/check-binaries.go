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

package checkbinaries

import (
	"fmt"
	"regexp"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	// PluginName defines this plugin's registered name.
	PluginName                    = "check-binaries"
	InvalidBinariesResponseFormat = `The following file(s) %s includes invalid binaries`
)

var (
	commandRe = regexp.MustCompile(`(?mi)^/check-binaries\s*$`)
)

func init() {
	plugins.RegisterPullRequestHandler(PluginName, handlePullRequest, helpProvider)
	plugins.RegisterGenericCommentHandler(PluginName, handleGenericCommentEvent, helpProvider)
}

// optionsForRepo gets the plugins.CheckBinaries struct that is applicable to the indicated repo.
func optionsForRepo(config *plugins.Configuration, org, repo string) *plugins.CheckBinaries {
	fullName := fmt.Sprintf("%s/%s", org, repo)

	// First search for repo config
	for _, c := range config.CheckBinaries {
		if !sets.NewString(c.Repos...).Has(fullName) {
			continue
		}
		return &c
	}

	// If you don't find anything, loop again looking for an org config
	for _, c := range config.CheckBinaries {
		if !sets.NewString(c.Repos...).Has(org) {
			continue
		}
		return &c
	}

	// Return an empty config
	return &plugins.CheckBinaries{}
}

func configForRepo(options *plugins.CheckBinaries) string {
	if options.CheckBinariesRe != "" {
		return options.CheckBinariesRe
	}
	return `debian/source/include-binaries|.*\.so(\.\d+)?`
}

func helpProvider(c *plugins.Configuration, orgRepo []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	config := map[string]string{}
	for _, repo := range orgRepo {
		checkBinariesReStr := configForRepo(optionsForRepo(c, repo.Org, repo.Repo))
		config[repo.String()] = fmt.Sprintf("The check-binaries plugin configured to check PRs binaries with re: %s", checkBinariesReStr)
	}

	_, err := plugins.CommentMap.GenYaml(&plugins.Configuration{
		CheckBinaries: []plugins.CheckBinaries{
			{
				Repos: []string{
					"org",
					"org/repo",
				},
				CheckBinariesRe: `debian/source/include-binaries|.*\.so(\.\d+)?`,
			},
		},
	})
	if err != nil {
		logrus.WithError(err).Warnf("cannot generate comments for %s plugin", PluginName)
	}

	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The check binaries plugin check source whether include binaries.",
		Config:      config,
		//Snippet:     yamlSnippet,
	}

	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/check-binaries",
		Description: labels.InvalidBinaries,
		Featured:    false,
		WhoCanUse:   "Anyone",
		Examples:    []string{"/check-binaries"},
	})
	return pluginHelp, nil
}

type githubClient interface {
	AddLabel(org, repo string, number int, label string) error
	// CreateComment(owner, repo string, number int, comment string) error
	CreateReview(org, repo string, number int, r github.DraftReview) error
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	RemoveLabel(owner, repo string, number int, label string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	// BotUserChecker() (func(candidate string) bool, error)
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

	prInfo := info{
		org:          pre.Repo.Owner.Login,
		repo:         pre.Repo.Name,
		repoFullName: pre.Repo.FullName,
		number:       pre.Number,
	}

	return handle(pc.GitHubClient, pc.Logger, &pre.PullRequest, prInfo, cp, configForRepo(optionsForRepo(pc.PluginConfig, pre.Repo.Owner.Login, pre.Repo.Name)))
}

func handleGenericCommentEvent(pc plugins.Agent, e github.GenericCommentEvent) error {

	cp, err := pc.CommentPruner()
	if err != nil {
		return err
	}

	return handleGenericComment(pc.GitHubClient, pc.Logger, &e, cp, configForRepo(optionsForRepo(pc.PluginConfig, e.Repo.Owner.Login, e.Repo.Name)))
}

func handleGenericComment(ghc githubClient, log *logrus.Entry, ce *github.GenericCommentEvent, cp commentPruner, reStr string) error {
	// Only consider open PRs and new comments.
	if ce.IssueState != "open" || !ce.IsPR || ce.Action != github.GenericCommentActionCreated {
		return nil
	}

	if !commandRe.MatchString(ce.Body) {
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

	return handle(ghc, log, pr, prInfo, cp, reStr)
}

func handle(ghc githubClient, log *logrus.Entry, pr *github.PullRequest, info info, cp commentPruner, reStr string) error {
	org := info.org
	repo := info.repo
	number := info.number
	checkBinariesRe := regexp.MustCompile(reStr)

	// Get changes.
	changes, err := ghc.GetPullRequestChanges(org, repo, number)
	if err != nil {
		return fmt.Errorf("error getting PR changes: %w", err)
	}

	// List invalid binaries files.
	var invalidBinariesFiles []github.PullRequestChange
	for _, change := range changes {
		if checkBinariesRe.MatchString(change.Filename) && change.Status != github.PullRequestFileRemoved {
			invalidBinariesFiles = append(invalidBinariesFiles, change)
		}
	}

	issueLabels, err := ghc.GetIssueLabels(org, repo, number)
	if err != nil {
		return err
	}

	hasInvalidBinariesLabel := github.HasLabel(labels.InvalidBinaries, issueLabels)
	if len(invalidBinariesFiles) == 0 && !hasInvalidBinariesLabel {
		return nil
	}

	if len(invalidBinariesFiles) > 0 {
		if !hasInvalidBinariesLabel {
			if err := ghc.AddLabel(org, repo, number, labels.InvalidBinaries); err != nil {
				return err
			}
		}

		log.Debugf("Creating a review for %d invalid binaries.", len(invalidBinariesFiles))
		var comments []github.DraftReviewComment
		for _, errFile := range invalidBinariesFiles {
			comments = append(comments, github.DraftReviewComment{
				Path:     errFile.Filename,
				Body:     "This file checked with invalid binaries",
				Position: 1,
			})
		}

		// Make the review body.
		response := fmt.Sprintf("%d invalid binaries files", len(invalidBinariesFiles))
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
			return fmt.Errorf("error creating a review for invalid binaries files: %w", err)
		}
	}

	if len(invalidBinariesFiles) == 0 && hasInvalidBinariesLabel {
		// Don't bother checking if it has the label...it's a race, and we'll have
		// to handle failure due to not being labeled anyway.
		if err := ghc.RemoveLabel(org, repo, number, labels.InvalidBinaries); err != nil {
			return fmt.Errorf("failed removing %s label: %w", labels.InvalidBinaries, err)
		}
	}
	return nil
}
