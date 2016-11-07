/*
Copyright 2016 The Kubernetes Authors.

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

package trigger

import (
	"regexp"

	"k8s.io/test-infra/prow/github"
)

var okToTest = regexp.MustCompile(`(?m)^(@k8s-bot )?ok to test\r?$`)

func handleIC(c client, ic github.IssueCommentEvent) error {
	owner := ic.Repo.Owner.Login
	name := ic.Repo.Name
	number := ic.Issue.Number
	author := ic.Comment.User.Login
	// Only take action when a comment is first created.
	if ic.Action != "created" {
		return nil
	}
	// If it's not an open PR, skip it.
	if !ic.Issue.IsPullRequest() {
		return nil
	}
	if ic.Issue.State != "open" {
		return nil
	}
	// Skip bot comments.
	if author == "k8s-bot" || author == "k8s-ci-robot" {
		return nil
	}

	// Which jobs does the comment want us to run?
	requestedJobs := c.JobAgent.MatchingJobs(ic.Repo.FullName, ic.Comment.Body, okToTest)
	if len(requestedJobs) == 0 {
		return nil
	}

	// Skip untrusted users.
	orgMember, err := c.GitHubClient.IsMember(trustedOrg, author)
	if err != nil {
		return err
	} else if !orgMember {
		return nil
	}

	pr, err := c.GitHubClient.GetPullRequest(owner, name, number)
	if err != nil {
		return err
	}

	for _, job := range requestedJobs {
		if err := build(c, job, *pr); err != nil {
			return err
		}
	}
	return nil
}
