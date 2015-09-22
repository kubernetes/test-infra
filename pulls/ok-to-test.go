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
	"k8s.io/contrib/mungegithub/config"
	github_util "k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

// OkToTestMunger looks for situations where a reviewer has LGTM'd a PR, but it
// isn't ok to test by the k8s-bot, and adds an 'ok to test' comment to the PR.
type OkToTestMunger struct{}

func init() {
	RegisterMungerOrDie(OkToTestMunger{})
}

// Name is the name usable in --pr-mungers
func (OkToTestMunger) Name() string { return "ok-to-test" }

// AddFlags will add any request flags to the cobra `cmd`
func (OkToTestMunger) AddFlags(cmd *cobra.Command) {}

// MungePullRequest is the workhorse the will actually make updates to the PR
func (OkToTestMunger) MungePullRequest(config *config.MungeConfig, pr *github.PullRequest, issue *github.Issue, commits []github.RepositoryCommit, events []github.IssueEvent) {
	if !github_util.HasLabel(issue.Labels, "lgtm") {
		return
	}
	status, err := config.GetStatus(pr, []string{"Jenkins GCE e2e"})
	if err != nil {
		glog.Errorf("unexpected error getting status: %v", err)
		return
	}
	if status == "incomplete" {
		glog.V(2).Infof("status is incomplete, adding ok to test")
		msg := `@k8s-bot ok to test

	pr builder appears to be missing, activating due to 'lgtm' label.`
		config.WriteComment(*pr.Number, msg)
	}
}
