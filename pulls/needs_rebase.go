/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package pulls

import (
	github_util "k8s.io/contrib/github"
	"k8s.io/contrib/mungegithub/config"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

// NeedsRebaseMunger will add the "needs-rebase" label to any issue which is
// unable to be automatically merged
type NeedsRebaseMunger struct{}

const needsRebase = "needs-rebase"

func init() {
	RegisterMungerOrDie(NeedsRebaseMunger{})
}

// Name is the name usable in --pr-mungers
func (NeedsRebaseMunger) Name() string { return "needs-rebase" }

// AddFlags will add any request flags to the cobra `cmd`
func (NeedsRebaseMunger) AddFlags(cmd *cobra.Command) {}

// MungePullRequest is the workhorse the will actually make updates to the PR
func (NeedsRebaseMunger) MungePullRequest(config *config.MungeConfig, pr *github.PullRequest, issue *github.Issue, commits []github.RepositoryCommit, events []github.IssueEvent) {
	mergeable, err := config.IsPRMergeable(pr)
	if err != nil {
		glog.V(2).Infof("Skipping %d - problem determining mergeable", *pr.Number)
		return
	}
	if mergeable && github_util.HasLabel(issue.Labels, needsRebase) {
		config.RemoveLabel(*pr.Number, needsRebase)
	}
	if !mergeable && !github_util.HasLabel(issue.Labels, needsRebase) {
		config.AddLabels(*pr.Number, []string{needsRebase})
	}
}
