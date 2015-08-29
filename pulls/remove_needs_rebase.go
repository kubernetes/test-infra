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
	"time"

	"k8s.io/contrib/mungegithub/opts"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
)

type RemoveNeedsRebaseMunger struct{}

func init() {
	RegisterMungerOrDie(RemoveNeedsRebaseMunger{})
}

func (RemoveNeedsRebaseMunger) Name() string { return "remove-needs-rebase" }

func (RemoveNeedsRebaseMunger) MungePullRequest(client *github.Client, pr *github.PullRequest, issue *github.Issue, commits []github.RepositoryCommit, events []github.IssueEvent, opts opts.MungeOptions) {
	if !HasLabel(issue.Labels, "needs-rebase") {
		return
	}
	if pr.Mergeable == nil {
		// If mergeable is nil, github is likely re-calculating mergeability, wait a bit and reload the PR.
		time.Sleep(2 * time.Second)
		var err error
		if pr, _, err = client.PullRequests.Get(opts.Org, opts.Project, *pr.Number); err != nil {
			glog.Errorf("Failed to resync project: %d", *pr.Number)
			return
		}
	}
	if pr.Mergeable != nil && *pr.Mergeable {
		if opts.Dryrun {
			glog.Infof("Would have removed needs-rebase for %d", *pr.Number)
		} else {
			client.Issues.RemoveLabelForIssue(opts.Org, opts.Project, *pr.Number, "needs-rebase")
		}
	}
}
