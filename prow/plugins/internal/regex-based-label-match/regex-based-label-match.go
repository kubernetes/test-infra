/*
Copyright 2022 The Kubernetes Authors.

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

package regexbasedlabelmatch

import (
	"fmt"
	"time"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

type GithubClient interface {
	AddLabel(org, repo string, number int, label string) error
	RemoveLabel(org, repo string, number int, label string) error
	CreateComment(org, repo string, number int, content string) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
}

type CommentPruner interface {
	PruneComments(shouldPrune func(github.IssueComment) bool)
}

type Event struct {
	Org    string
	Repo   string
	Number int
	Author string
	// The PR's base branch. If empty this is
	// an Issue, not a PR.
	Branch string
	// The label that was added or removed. If
	// empty this is an open or reopen event.
	Label string
	// The labels currently on the issue. For PRs
	// this is not contained in the webhook payload
	// and may be omitted.
	CurrentLabels []github.Label
	IsPR          bool
}

// ShouldConsiderConfig checks if a config should be considered or not.
// `branch` should be empty for Issues and non-empty for PRs.
// `label` should be omitted in the case of 'open' and 'reopen' actions.
func ShouldConsiderConfig(org, repo, branch, label string, cfg plugins.RegexBasedLabelMatch) bool {
	// Check if the config applies to this issue type.
	if (branch == "" && !cfg.Issues) || (branch != "" && !cfg.PRs) {
		return false
	}
	// Check if the config applies to this 'org[/repo][/branch]'.
	if org != cfg.Org ||
		(cfg.Repo != "" && cfg.Repo != repo) ||
		(cfg.Branch != "" && branch != "" && cfg.Branch != branch) {
		return false
	}
	// If we are reacting to a label event, see if it is relevant.
	if label != "" && !cfg.Re.MatchString(label) {
		return false
	}

	return true
}

// LabelPreChecks checks if an event is a label event or not, if it is not, then we
// must be reacting to an issue or a PR, for which LabelPreChecks will sleep for a
// certain amount of time to allow other parts of the automation to apply labels and
// avoid thrashing.
func LabelPreChecks(e *Event, ghc GithubClient, configs []plugins.RegexBasedLabelMatch) error {
	if e.Label == "" /* not a label event */ {
		// If we are reacting to a PR or Issue being created or reopened, we should wait a
		// few seconds to allow other automation to apply labels in order to minimize thrashing.
		// We use the max grace period from applicable configs.
		gracePeriod := time.Duration(0)
		for _, cfg := range configs {
			if cfg.GracePeriodDuration > gracePeriod {
				gracePeriod = cfg.GracePeriodDuration
			}
		}
		time.Sleep(gracePeriod)
		// If currentLabels was populated it is now stale.
		e.CurrentLabels = nil
	}
	if e.CurrentLabels == nil {
		var err error
		e.CurrentLabels, err = ghc.GetIssueLabels(e.Org, e.Repo, e.Number)
		if err != nil {
			return fmt.Errorf("error getting the issue or pr's labels: %w", err)
		}
	}

	return nil
}
