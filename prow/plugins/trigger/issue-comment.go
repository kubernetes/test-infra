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
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/plugins"
)

var okToTestRe = regexp.MustCompile(`(?m)^/ok-to-test\s*$`)
var testAllRe = regexp.MustCompile(`(?m)^/test all,?($|\s.*)`)
var retestRe = regexp.MustCompile(`(?m)^/retest\s*$`)

func handleIC(c client, trigger *plugins.Trigger, ic github.IssueCommentEvent) error {
	org := ic.Repo.Owner.Login
	repo := ic.Repo.Name
	number := ic.Issue.Number
	commentAuthor := ic.Comment.User.Login
	// Only take action when a comment is first created.
	if ic.Action != github.IssueCommentActionCreated {
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
	botName, err := c.GitHubClient.BotName()
	if err != nil {
		return err
	}
	if commentAuthor == botName {
		return nil
	}

	// Which jobs does the comment want us to run?
	isOkToTest := okToTestRe.MatchString(ic.Comment.Body)
	testAll := isOkToTest || testAllRe.MatchString(ic.Comment.Body)
	shouldRetestFailed := retestRe.MatchString(ic.Comment.Body)
	requestedJobs := c.Config.MatchingPresubmits(ic.Repo.FullName, ic.Comment.Body, testAll)
	if !shouldRetestFailed && len(requestedJobs) == 0 {
		// Check for the presence of the needs-ok-to-test label and remove it
		// if a trusted member has commented "/ok-to-test".
		if isOkToTest && ic.Issue.HasLabel(labels.NeedsOkToTest) {
			trusted, err := TrustedUser(c.GitHubClient, trigger, commentAuthor, org, repo)
			if err != nil {
				return err
			}
			if trusted {
				if err := c.GitHubClient.AddLabel(org, repo, number, labels.OkToTest); err != nil {
					return err
				}
				return c.GitHubClient.RemoveLabel(org, repo, number, labels.NeedsOkToTest)
			}
		}
		return nil
	}

	pr, err := c.GitHubClient.GetPullRequest(org, repo, number)
	if err != nil {
		return err
	}

	var forceRunContexts map[string]bool
	if shouldRetestFailed {
		combinedStatus, err := c.GitHubClient.GetCombinedStatus(org, repo, pr.Head.SHA)
		if err != nil {
			return err
		}
		skipContexts := make(map[string]bool)    // these succeeded or are running
		forceRunContexts = make(map[string]bool) // these failed and should be re-run
		for _, status := range combinedStatus.Statuses {
			state := status.State
			if state == github.StatusSuccess || state == github.StatusPending {
				skipContexts[status.Context] = true
			} else if state == github.StatusError || state == github.StatusFailure {
				forceRunContexts[status.Context] = true
			}
		}
		retests := c.Config.RetestPresubmits(ic.Repo.FullName, skipContexts, forceRunContexts)
		requestedJobs = append(requestedJobs, retests...)
	}

	// Skip untrusted users.
	trusted, err := TrustedUser(c.GitHubClient, trigger, commentAuthor, org, repo)
	if err != nil {
		return fmt.Errorf("error checking trust of %s: %v", commentAuthor, err)
	}
	if !trusted {
		_, trusted, err := trustedPullRequest(c.GitHubClient, trigger, ic.Issue.User.Login, org, repo, number, ic.Issue.Labels)
		if err != nil {
			return err
		}
		if !trusted {
			resp := fmt.Sprintf("Cannot trigger testing until a trusted user reviews the PR and leaves an `/ok-to-test` message.")
			c.Logger.Infof("Commenting \"%s\".", resp)
			return c.GitHubClient.CreateComment(org, repo, number, plugins.FormatICResponse(ic.Comment, resp))
		}
	}

	if isOkToTest {
		if err := c.GitHubClient.AddLabel(ic.Repo.Owner.Login, ic.Repo.Name, ic.Issue.Number, labels.OkToTest); err != nil {
			return err
		}
	}
	if (ic.Issue.HasLabel(labels.OkToTest) || isOkToTest) && ic.Issue.HasLabel(labels.NeedsOkToTest) {
		if err := c.GitHubClient.RemoveLabel(ic.Repo.Owner.Login, ic.Repo.Name, ic.Issue.Number, labels.NeedsOkToTest); err != nil {
			return err
		}
	}

	return runOrSkipRequested(c, pr, requestedJobs, forceRunContexts, ic.Comment.Body, ic.GUID)
}
