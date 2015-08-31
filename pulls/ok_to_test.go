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

// OkToTestMunger looks for situations where a reviewer has LGTM'd a PR, but it
// isn't ok to test by the k8s-bot, and adds an 'ok to test' comment to the PR.
type OkToTestMunger struct{}

func init() {
	RegisterMungerOrDie(OkToTestMunger{})
	dii
}

func (OkToTestMunger) Name() string { return "ok-to-test" }

func isLGTM(issue *github.Issue) {
	for _, label := range issue.Labels {
		if label.Name != nil && *label.Name == "lgtm" {
			return true
		}
	}
	return false
}

func (OkToTestMunger) MungePullRequest(client *github.Client, pr *github.PullRequest, issue *github.Issue, commits []github.RepositoryCommit, events []github.IssueEvent, opts opts.MungeOptions) {
	if !isLGTM(issue) {
		return
	}
	status, err := github_util.GetStatus(client, opts.Org, opts.Project, *pr.Number, []string{"Jenkins GCE e2e"})
	if err != nil {
		glog.Errorf("unexpected error getting status: %v", err)
		return
	}
	if status == "incomplete" {
		if opts.Dryrun {
			glog.Infof("would have marked %d as ok to test", pr.Number)
			return
		}
		glog.V(2).Infof("status is incomplete, adding ok to test")
		msg := `@k8s-bot ok to test

pr builder appears to be missing, activating due to 'lgtm' label.`
		if _, err := client.Issues.CreateComment(opts.Org, opts.Project, *pr.Number, &github.IssueComment{ Body: &msg }); err != nil {
			glog.Errorf("failed to create comment: %v", err)
		}
	}
}
