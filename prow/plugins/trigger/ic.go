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
	"fmt"
	"regexp"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

var okToTest = regexp.MustCompile(`(?m)^(@k8s-bot )?ok to test\r?$`)

func handleIC(c client, ic github.IssueCommentEvent) error {
	org := ic.Repo.Owner.Login
	repo := ic.Repo.Name
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
	if author == c.GitHubClient.BotName() {
		return nil
	}

	// Which jobs does the comment want us to run?
	requestedJobs := c.Config.MatchingPresubmits(ic.Repo.FullName, ic.Comment.Body, okToTest)
	if len(requestedJobs) == 0 {
		return nil
	}

	pr, err := c.GitHubClient.GetPullRequest(org, repo, number)
	if err != nil {
		return err
	}

	// Skip untrusted users.
	orgMember, err := c.GitHubClient.IsMember(trustedOrg, author)
	if err != nil {
		return err
	} else if !orgMember {
		trusted, err := trustedPullRequest(c.GitHubClient, *pr)
		if err != nil {
			return err
		}
		if !trusted {
			resp := fmt.Sprintf("you can't request testing unless you are a [%s](https://github.com/orgs/%s/people) member", trustedOrg, trustedOrg)
			c.Logger.Infof("Commenting \"%s\".", resp)
			return c.GitHubClient.CreateComment(org, repo, number, plugins.FormatICResponse(ic.Comment, resp))
		}
	}

	ref, err := c.GitHubClient.GetRef(org, repo, "heads/"+pr.Base.Ref)
	if err != nil {
		return err
	}

	var errors []error
	for _, job := range requestedJobs {
		if !job.RunsAgainstBranch(pr.Base.Ref) {
			if err := c.GitHubClient.CreateStatus(org, repo, pr.Head.SHA, github.Status{
				State:       github.StatusSuccess,
				Context:     job.Context,
				Description: "Skipped",
			}); err != nil {
				return err
			}
			continue
		}
		c.Logger.Infof("Starting %s build.", job.Name)
		if err := lineStartPRJob(c.KubeClient, job.Name, job.Context, *pr, ref); err != nil {
			errors = append(errors, err)
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("errors starting jobs: %v", errors)
	}
	return nil
}
