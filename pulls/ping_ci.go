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
	github_util "k8s.io/contrib/submit-queue/github"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
)

// PingCIMunger looks for situations CI (Travis | Shippable) has flaked for some
// reason and we want to re-run them.  Achieves this by closing and re-opening the pr
type PingCIMunger struct{}

func init() {
	RegisterMungerOrDie(PingCIMunger{})
}

func (PingCIMunger) Name() string { return "ping-ci" }

func (PingCIMunger) MungePullRequest(client *github.Client, pr *github.PullRequest, issue *github.Issue, commits []github.RepositoryCommit, events []github.IssueEvent, opts opts.MungeOptions) {
	if !HasLabel(issue.Labels, "lgtm") {
		return
	}
	status, err := github_util.GetStatus(client, opts.Org, opts.Project, *pr.Number, []string{"Shippable", "continuous-integration/travis-ci/pr"})
	if err != nil {
		glog.Errorf("unexpected error getting status: %v", err)
		return
	}
	if status == "incomplete" {
		if opts.Dryrun {
			glog.Infof("would have pinged CI for %d", pr.Number)
			return
		}
		glog.V(2).Infof("status is incomplete, closing and re-opening")
		msg := "Continuous integration appears to have missed, closing and re-opening to trigger it"
		if _, _, err := client.Issues.CreateComment(opts.Org, opts.Project, *pr.Number, &github.IssueComment{Body: &msg}); err != nil {
			glog.Errorf("failed to create comment: %v", err)
		}
		state := "closed"
		pr.State = &state
		if _, _, err := client.PullRequests.Edit(opts.Org, opts.Project, *pr.Number, pr); err != nil {
			glog.Errorf("Failed to close pr %d: %v", *pr.Number, err)
			return
		}
		time.Sleep(5 * time.Second)
		state = "open"
		pr.State = &state
		// Try pretty hard to re-open, since it's pretty bad if we accidentally leave a PR closed
		for tries := 0; tries < 10; tries++ {
			if _, _, err := client.PullRequests.Edit(opts.Org, opts.Project, *pr.Number, pr); err == nil {
				break
			} else {
				glog.Errorf("failed to re-open pr %d: %v", *pr.Number, err)
			}
			time.Sleep(5 * time.Second)
		}
	}
}
