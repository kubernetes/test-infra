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
	"k8s.io/contrib/mungegithub/opts"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
)

type NeedsRebaseMunger struct{}

const needsRebase = "needs-rebase"

func init() {
	RegisterMungerOrDie(NeedsRebaseMunger{})
}

func (NeedsRebaseMunger) Name() string { return "needs-rebase" }

func (NeedsRebaseMunger) MungePullRequest(client *github.Client, pr *github.PullRequest, issue *github.Issue, commits []github.RepositoryCommit, events []github.IssueEvent, opts opts.MungeOptions) {
	if pr.Mergeable == nil {
		glog.Infof("Skipping %d since mergeable is nil", *pr.Number)
		return
	}
	if *pr.Mergeable && HasLabel(issue.Labels, needsRebase) {
		if opts.Dryrun {
			glog.Infof("Would have removed needs-rebase for %d", *pr.Number)
		} else {
			client.Issues.RemoveLabelForIssue(opts.Org, opts.Project, *pr.Number, needsRebase)
		}
	}
	if !*pr.Mergeable && !HasLabel(issue.Labels, needsRebase) {
		if opts.Dryrun {
			glog.Infof("Would have added needs-rebase for %d", *pr.Number)
		} else {
			client.Issues.AddLabelsToIssue(opts.Org, opts.Project, *pr.Number, []string{needsRebase})
		}
	}
}
