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

package cherrypickapproved

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

// PluginName defines this plugin's registered name.
const PluginName = "cherry-pick-approved"

func init() {
	plugins.RegisterReviewEventHandler(PluginName, handlePullRequestReviewEvent, helpProvider)
}

func helpProvider(cfg *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	yamlSnippet, err := plugins.CommentMap.GenYaml(&plugins.Configuration{
		CherryPickApproved: []plugins.CherryPickApproved{
			{BranchRegexp: "^release-*"},
		},
	})
	if err != nil {
		logrus.WithError(err).Warnf("cannot generate comments for %s plugin", PluginName)
	}

	return &pluginhelp.PluginHelp{
		Description: fmt.Sprintf(
			"The %s plugin helps a defined set of maintainers to approve cherry-picks by using GitHub reviews",
			PluginName,
		),
		Config: map[string]string{
			"": fmt.Sprintf(
				"The %s plugin treats PRs against branch names satisfying the `branchregexp` as cherry-pick PRs. "+
					"There needs to be a defined `approvers` list for the plugin to "+
					"be able to distinguish cherry-pick approvers from regular maintainers.",
				PluginName,
			),
		},
		Snippet: yamlSnippet,
	}, nil
}

type handler struct {
	impl
}

func newHandler() *handler {
	return &handler{
		impl: &defaultImpl{},
	}
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate . impl
type impl interface {
	GetCombinedStatus(gc plugins.PluginGitHubClient, org, repo, ref string) (*github.CombinedStatus, error)
	GetIssueLabels(gc plugins.PluginGitHubClient, org, repo string, number int) ([]github.Label, error)
	AddLabel(gc plugins.PluginGitHubClient, org, repo string, number int, label string) error
	RemoveLabel(gc plugins.PluginGitHubClient, org, repo string, number int, label string) error
}

type defaultImpl struct{}

func (*defaultImpl) GetCombinedStatus(gc plugins.PluginGitHubClient, org, repo, ref string) (*github.CombinedStatus, error) {
	return gc.GetCombinedStatus(org, repo, ref)
}

func (*defaultImpl) GetIssueLabels(gc plugins.PluginGitHubClient, org, repo string, number int) ([]github.Label, error) {
	return gc.GetIssueLabels(org, repo, number)
}

func (*defaultImpl) AddLabel(gc plugins.PluginGitHubClient, org, repo string, number int, label string) error {
	return gc.AddLabel(org, repo, number, label)

}

func (*defaultImpl) RemoveLabel(gc plugins.PluginGitHubClient, org, repo string, number int, label string) error {
	return gc.RemoveLabel(org, repo, number, label)
}

func handlePullRequestReviewEvent(pc plugins.Agent, e github.ReviewEvent) error {
	if err := newHandler().handle(pc.Logger, pc.GitHubClient, e, pc.PluginConfig.CherryPickApproved); err != nil {
		pc.Logger.WithError(err).Error("skipping")
		return err
	}
	return nil
}

func (h *handler) handle(log *logrus.Entry, gc plugins.PluginGitHubClient, e github.ReviewEvent, cfgs []plugins.CherryPickApproved) error {
	funcStart := time.Now()

	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	branch := e.PullRequest.Base.Ref
	prNumber := e.PullRequest.Number

	defer func() {
		log.WithField("duration", time.Since(funcStart).String()).
			WithField("org", org).
			WithField("repo", repo).
			WithField("branch", branch).
			WithField("pr", prNumber).
			Debug("Completed handlePullRequestReviewEvent")
	}()

	var (
		approvers []string
		branchRe  *regexp.Regexp
	)

	// Filter configurations
	foundRepoOrg := false
	for _, cfg := range cfgs {
		if cfg.Org == org && cfg.Repo == repo {
			foundRepoOrg = true
			approvers = cfg.Approvers
			branchRe = cfg.BranchRe
		}
	}

	if !foundRepoOrg {
		log.Debugf("Skipping because repo %s/%s is not part of plugin configuration", org, repo)
		return nil
	}

	if len(approvers) == 0 {
		log.Debug("Skipping because no cherry-pick approvers configured")
		return nil
	}

	if branchRe == nil || !branchRe.MatchString(branch) {
		log.Debugf("Skipping because no release branch regex match for branch: %s", branch)
		return nil
	}

	// Only react to reviews that are being submitted (not edited or dismissed).
	if e.Action != github.ReviewActionSubmitted {
		return nil
	}

	// The review webhook returns state as lowercase, while the review API
	// returns state as uppercase. Uppercase the value here so it always
	// matches the constant.
	if github.ReviewState(strings.ToUpper(string(e.Review.State))) != github.ReviewStateApproved {
		return nil
	}

	// Check the PR state to not have failed tests
	combinedStatus, err := h.GetCombinedStatus(gc, org, repo, e.PullRequest.Head.SHA)
	if err != nil {
		return fmt.Errorf("get combined status: %w", err)
	}
	for _, status := range combinedStatus.Statuses {
		state := status.State
		if state == github.StatusError || state == github.StatusFailure {
			log.Infof("Skipping PR %d because tests failed", prNumber)
			return nil
		}
	}

	// Validate the labels
	issueLabels, err := h.GetIssueLabels(gc, org, repo, prNumber)
	if err != nil {
		return fmt.Errorf("get issue labels: %w", err)
	}

	hasCherryPickApprovedLabel := github.HasLabel(labels.CpApproved, issueLabels)
	hasCherryPickUnapprovedLabel := github.HasLabel(labels.CpUnapproved, issueLabels)
	hasLGTMLabel := github.HasLabel(labels.LGTM, issueLabels)
	hasApprovedLabel := github.HasLabel(labels.Approved, issueLabels)
	hasInvalidLabels := github.HasLabels(
		[]string{
			labels.BlockedPaths,
			labels.ClaNo,
			labels.Hold,
			labels.InvalidOwners,
			labels.InvalidBug,
			labels.MergeCommits,
			labels.NeedsOkToTest,
			labels.NeedsRebase,
			labels.ReleaseNoteLabelNeeded,
			labels.WorkInProgress,
		},
		issueLabels,
	)

	isApprover := false
	for _, approver := range approvers {
		if e.Review.User.Login == approver {
			isApprover = true
		}
	}
	if !isApprover {
		log.Infof("Skipping PR %d because user %s is not an approver from configured list: %v", prNumber, e.Review.User.Login, approvers)
		return nil
	}

	if hasLGTMLabel && hasApprovedLabel && !hasInvalidLabels {
		if !hasCherryPickApprovedLabel {
			if err := h.AddLabel(gc, org, repo, prNumber, labels.CpApproved); err != nil {
				log.WithError(err).Errorf("failed to add the label: %s", labels.CpApproved)
			}
		}

		if hasCherryPickUnapprovedLabel {
			if err := h.RemoveLabel(gc, org, repo, prNumber, labels.CpUnapproved); err != nil {
				log.WithError(err).Errorf("failed to remove the label: %s", labels.CpUnapproved)
			}
		}
	}

	return nil
}
