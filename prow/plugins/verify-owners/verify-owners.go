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
	"k8s.io/test-infra/prow/repoowners"
)

const (
	// PluginName defines this plugin's registered name.
	PluginName     = "verify-owners"
	ownersFileName = "OWNERS"
)

func init() {
	plugins.RegisterPullRequestHandler(PluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	pluginHelp := &pluginhelp.PluginHelp{
		Description: fmt.Sprintf("The verify-owners plugin validates %s files if they are modified in a PR. On validation failure it automatically adds the '%s' label to the PR, and a review comment on the incriminating file(s).", ownersFileName, labels.InvalidOwners),
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
	LoadRepoOwners(org, repo, base string) (repoowners.RepoOwner, error)
}

type githubClient interface {
	AddLabel(org, repo string, number int, label string) error
	CreateComment(owner, repo string, number int, comment string) error
	CreateReview(org, repo string, number int, r github.DraftReview) error
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	RemoveLabel(owner, repo string, number int, label string) error
}

func handlePullRequest(pc plugins.Agent, pre github.PullRequestEvent) error {
	if pre.Action != github.PullRequestActionOpened && pre.Action != github.PullRequestActionReopened && pre.Action != github.PullRequestActionSynchronize {
		return nil
	}
	return handle(pc.GitHubClient, pc.GitClient, pc.Logger, &pre, pc.PluginConfig.Owners.LabelsBlackList)
}

type messageWithLine struct {
	line    int
	message string
}

func handle(ghc githubClient, gc *git.Client, log *logrus.Entry, pre *github.PullRequestEvent, labelsBlackList []string) error {
	org := pre.Repo.Owner.Login
	repo := pre.Repo.Name
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
	if len(modifiedOwnersFiles) == 0 {
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

	// Check each OWNERS file.
	for _, c := range modifiedOwnersFiles {
		// Try to load OWNERS file.
		path := filepath.Join(r.Dir, c.Filename)
		b, err := ioutil.ReadFile(path)
		if err != nil {
			log.WithError(err).Warningf("Failed to read %s.", path)
			return nil
		}
		if msg := parseOwnersFile(b, c, log, labelsBlackList); msg != nil {
			wrongOwnersFiles[c.Filename] = *msg
		}
	}
	// React if we saw something.
	if len(wrongOwnersFiles) > 0 {
		s := "s"
		if len(wrongOwnersFiles) == 1 {
			s = ""
		}
		if err := ghc.AddLabel(org, repo, pre.Number, labels.InvalidOwners); err != nil {
			return err
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
	} else {
		// Don't bother checking if it has the label...it's a race, and we'll have
		// to handle failure due to not being labeled anyway.
		if err := ghc.RemoveLabel(org, repo, pre.Number, labels.InvalidOwners); err != nil {
			return fmt.Errorf("failed removing %s label: %v", labels.InvalidOwners, err)
		}
	}
	return nil
}

func parseOwnersFile(b []byte, c github.PullRequestChange, log *logrus.Entry, labelsBlackList []string) *messageWithLine {
	var approvers []string
	var labels []string
	// by default we bind errors to line 1
	lineNumber := 1
	simple, err := repoowners.ParseSimpleConfig(b)
	if err != nil || simple.Empty() {
		full, err := repoowners.ParseFullConfig(b)
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
			}
		}
		// it's a FullConfig
		for _, config := range full.Filters {
			approvers = append(approvers, config.Approvers...)
			labels = append(labels, config.Labels...)
		}
	} else {
		// it's a SimpleConfig
		approvers = simple.Config.Approvers
		labels = simple.Config.Labels
	}
	// Check labels against blacklist
	if sets.NewString(labels...).HasAny(labelsBlackList...) {
		return &messageWithLine{
			lineNumber,
			fmt.Sprintf("File contains blacklisted labels: %s.", sets.NewString(labels...).Intersection(sets.NewString(labelsBlackList...)).List()),
		}
	}
	// Check approvers isn't empty
	if filepath.Dir(c.Filename) == "." && len(approvers) == 0 {
		return &messageWithLine{
			lineNumber,
			fmt.Sprintf("No approvers defined in this root directory %s file.", ownersFileName),
		}
	}
	return nil
}
