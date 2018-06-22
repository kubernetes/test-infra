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
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/repoowners"
)

const (
	pluginName     = "verify-owners"
	ownersFileName = "OWNERS"
)

var (
	invalidOwnersLabel = "do-not-merge/invalid-owners-file"
)

func init() {
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	return &pluginhelp.PluginHelp{
			Description: fmt.Sprintf("The verify-owners plugin validates OWNERS files if they are modified in a PR. On validation failure it automatically adds the '%s' label to the PR, and a review comment on the incriminating file(s).", invalidOwnersLabel),
		},
		nil
}

type ownersClient interface {
	LoadRepoOwners(org, repo, base string) (repoowners.RepoOwnerInterface, error)
}

type githubClient interface {
	AddLabel(org, repo string, number int, label string) error
	CreateComment(owner, repo string, number int, comment string) error
	CreateReview(org, repo string, number int, r github.DraftReview) error
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	RemoveLabel(owner, repo string, number int, label string) error
}

func handlePullRequest(pc plugins.PluginClient, pre github.PullRequestEvent) error {
	if pre.Action != github.PullRequestActionOpened && pre.Action != github.PullRequestActionReopened && pre.Action != github.PullRequestActionSynchronize {
		return nil
	}
	return handle(pc.GitHubClient, pc.GitClient, pc.Logger, &pre, pc.PluginConfig.Owners.LabelsBlackList)
}

func handle(ghc githubClient, gc *git.Client, log *logrus.Entry, pre *github.PullRequestEvent, labelsBlackList []string) error {
	org := pre.Repo.Owner.Login
	repo := pre.Repo.Name
	wrongOwnersFiles := map[string]error{}

	// Get changes.
	changes, err := ghc.GetPullRequestChanges(org, repo, pre.Number)
	if err != nil {
		return fmt.Errorf("error getting PR changes: %v", err)
	}

	// List modified OWNERS files.
	var modifiedOwnersFiles []string
	for _, change := range changes {
		if filepath.Base(change.Filename) == ownersFileName {
			modifiedOwnersFiles = append(modifiedOwnersFiles, change.Filename)
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

	// Check each OWNERS file.
	for _, f := range modifiedOwnersFiles {
		// Try to load OWNERS file.
		path := filepath.Join(r.Dir, f)
		b, err := ioutil.ReadFile(path)
		if err != nil {
			log.WithError(err).Errorf("Failed to read %s.", path)
			return nil
		}
		var approvers []string
		var labels []string
		simple, err := repoowners.ParseSimpleConfig(b)
		if err != nil || simple.Empty() {
			full, err := repoowners.ParseFullConfig(b)
			if err != nil {
				wrongOwnersFiles[f] = fmt.Errorf("error occurred parsing the OWNERS file in %s: %v", f, err)
				continue
			} else {
				// it's a FullConfig
				for _, config := range full.Filters {
					approvers = append(approvers, config.Approvers...)
					labels = append(labels, config.Labels...)
				}
			}
		} else {
			// it's a SimpleConfig
			approvers = simple.Config.Approvers
			labels = simple.Config.Labels
		}
		// Check labels against blacklist
		if sets.NewString(labels...).HasAny(labelsBlackList...) {
			wrongOwnersFiles[f] = fmt.Errorf("OWNERS file contains blacklisted labels: %s", sets.NewString(simple.Config.Labels...).Intersection(sets.NewString(labelsBlackList...)).List())
			continue
		}
		// Check approvers isn't empty
		if filepath.Dir(f) == "." && len(approvers) == 0 {
			wrongOwnersFiles[f] = errors.New("no approvers in the root directory OWNERS file")
			continue
		}
	}
	// React if we saw something.
	if len(wrongOwnersFiles) > 0 {
		if err := ghc.AddLabel(org, repo, pre.Number, invalidOwnersLabel); err != nil {
			return err
		}
		log.Debugf("Creating a review for %d OWNERS files.", len(wrongOwnersFiles))
		var comments []github.DraftReviewComment
		for errFile, errMsg := range wrongOwnersFiles {
			resp := fmt.Sprintf("Invalid OWNERS file: %v", errMsg)
			comments = append(comments, github.DraftReviewComment{
				Path: errFile,
				Body: resp,
			})
		}
		// Make the review body.
		s := "s"
		if len(wrongOwnersFiles) == 1 {
			s = ""
		}
		response := fmt.Sprintf("%d invalid OWNERS file%s", len(wrongOwnersFiles), s)
		err := ghc.CreateReview(org, repo, pre.Number, github.DraftReview{
			Body:     plugins.FormatResponseRaw(pre.PullRequest.Body, pre.PullRequest.HTMLURL, pre.PullRequest.User.Login, response),
			Action:   github.Comment,
			Comments: comments,
		})
		if err != nil {
			return fmt.Errorf("error creating a review for invalid OWNERS file(s): %v", err)
		}
	} else {
		// Don't bother checking if it has the label...it's a race, and we'll have
		// to handle failure due to not being labeled anyway.
		labelNotFound := true
		if err := ghc.RemoveLabel(org, repo, pre.Number, invalidOwnersLabel); err != nil {
			if _, labelNotFound = err.(*github.LabelNotFound); !labelNotFound {
				return fmt.Errorf("failed removing lgtm label: %v", err)
			}
			// If the error is indeed *github.LabelNotFound, consider it a success.
		}
	}
	return nil
}
