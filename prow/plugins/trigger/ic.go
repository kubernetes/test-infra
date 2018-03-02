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

var okToTestRe = regexp.MustCompile(`(?m)^/ok-to-test\s*$`)
var testAllRe = regexp.MustCompile(`(?m)^/test all\s*$`)
var retestRe = regexp.MustCompile(`(?m)^/retest\s*$`)

func handleIC(c client, trustedOrg string, ic github.IssueCommentEvent) error {
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
	okToTest := okToTestRe.MatchString(ic.Comment.Body)
	testAll := okToTest || testAllRe.MatchString(ic.Comment.Body)
	shouldRetestFailed := retestRe.MatchString(ic.Comment.Body)
	requestedJobs := c.Config.MatchingPresubmits(ic.Repo.FullName, ic.Comment.Body, testAll)
	if !shouldRetestFailed && len(requestedJobs) == 0 {
		// Check for the presence of the needs-ok-to-test label and remove it
		// if a trusted member has commented "/ok-to-test".
		if okToTest && ic.Issue.HasLabel(needsOkToTest) {
			orgMember, err := isUserTrusted(c.GitHubClient, commentAuthor, trustedOrg, org)
			if err != nil {
				return err
			}
			if orgMember {
				return c.GitHubClient.RemoveLabel(ic.Repo.Owner.Login, ic.Repo.Name, ic.Issue.Number, needsOkToTest)
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

	var comments []github.IssueComment
	// Skip untrusted users.
	orgMember, err := isUserTrusted(c.GitHubClient, commentAuthor, trustedOrg, org)
	if err != nil {
		return err
	}
	if !orgMember {
		comments, err = c.GitHubClient.ListIssueComments(org, repo, number)
		if err != nil {
			return err
		}
		trusted, err := trustedPullRequest(c.GitHubClient, *pr, trustedOrg, comments)
		if err != nil {
			return err
		}
		if !trusted {
			var more string
			if org != trustedOrg {
				more = fmt.Sprintf("or [%s](https://github.com/orgs/%s/people) ", org, org)
			}
			resp := fmt.Sprintf("you can't request testing unless you are a [%s](https://github.com/orgs/%s/people) %smember.", trustedOrg, trustedOrg, more)
			c.Logger.Infof("Commenting \"%s\".", resp)
			return c.GitHubClient.CreateComment(org, repo, number, plugins.FormatICResponse(ic.Comment, resp))
		}
	}

	if okToTest && ic.Issue.HasLabel(needsOkToTest) {
		if err := c.GitHubClient.RemoveLabel(ic.Repo.Owner.Login, ic.Repo.Name, ic.Issue.Number, needsOkToTest); err != nil {
			c.Logger.WithError(err).Errorf("Failed at removing %s label", needsOkToTest)
		}
		err = clearStaleComments(c.GitHubClient, trustedOrg, *pr, comments)
		if err != nil {
			c.Logger.Warnf("Failed to clear stale comments: %v.", err)
		}
	}

	return runOrSkipRequested(c, pr, requestedJobs, forceRunContexts, ic.Comment.Body, ic.GUID)
}
