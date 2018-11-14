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

func handleGenericComment(c client, trigger *plugins.Trigger, gc github.GenericCommentEvent) error {
	org := gc.Repo.Owner.Login
	repo := gc.Repo.Name
	number := gc.Number
	commentAuthor := gc.User.Login
	// Only take action when a comment is first created,
	// when it belongs to a PR,
	// and the PR is open.
	if gc.Action != github.GenericCommentActionCreated || !gc.IsPR || gc.IssueState != "open" {
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
	allowOkToTest := trigger == nil || !trigger.IgnoreOkToTest
	isOkToTest := okToTestRe.MatchString(gc.Body) && allowOkToTest
	testAll := isOkToTest || testAllRe.MatchString(gc.Body)
	shouldRetestFailed := retestRe.MatchString(gc.Body)
	requestedJobs := c.Config.MatchingPresubmits(gc.Repo.FullName, gc.Body, testAll)
	if !shouldRetestFailed && len(requestedJobs) == 0 {
		// If a trusted member has commented "/ok-to-test",
		// eventually add ok-to-test and remove needs-ok-to-test.
		l, err := c.GitHubClient.GetIssueLabels(org, repo, number)
		if err != nil {
			return err
		}
		if isOkToTest && !github.HasLabel(labels.OkToTest, l) {
			trusted, err := TrustedUser(c.GitHubClient, trigger, commentAuthor, org, repo)
			if err != nil {
				return err
			}
			if trusted {
				if err := c.GitHubClient.AddLabel(org, repo, number, labels.OkToTest); err != nil {
					return err
				}
				if github.HasLabel(labels.NeedsOkToTest, l) {
					if err := c.GitHubClient.RemoveLabel(org, repo, number, labels.NeedsOkToTest); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}

	pr, err := c.GitHubClient.GetPullRequest(org, repo, number)
	if err != nil {
		return err
	}

	// Skip untrusted users comments.
	trusted, err := TrustedUser(c.GitHubClient, trigger, commentAuthor, org, repo)
	if err != nil {
		return fmt.Errorf("error checking trust of %s: %v", commentAuthor, err)
	}
	var l []github.Label
	if !trusted {
		// Skip untrusted PRs.
		l, trusted, err = trustedPullRequest(c.GitHubClient, trigger, gc.IssueAuthor.Login, org, repo, number, nil)
		if err != nil {
			return err
		}
		if !trusted {
			resp := fmt.Sprintf("Cannot trigger testing until a trusted user reviews the PR and leaves an `/ok-to-test` message.")
			c.Logger.Infof("Commenting \"%s\".", resp)
			return c.GitHubClient.CreateComment(org, repo, number, plugins.FormatResponseRaw(gc.Body, gc.HTMLURL, gc.User.Login, resp))
		}
	}

	// At this point we can trust the PR, so we eventually update labels.
	// Ensure we have labels before test, because trustedPullRequest() won't be called
	// when commentAuthor is trusted.
	if l == nil {
		l, err = c.GitHubClient.GetIssueLabels(org, repo, number)
		if err != nil {
			return err
		}
	}
	if isOkToTest && !github.HasLabel(labels.OkToTest, l) {
		if err := c.GitHubClient.AddLabel(org, repo, number, labels.OkToTest); err != nil {
			return err
		}
	}
	if (isOkToTest || github.HasLabel(labels.OkToTest, l)) && github.HasLabel(labels.NeedsOkToTest, l) {
		if err := c.GitHubClient.RemoveLabel(org, repo, number, labels.NeedsOkToTest); err != nil {
			return err
		}
	}

	// Do we have to run some tests?
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
		retests := c.Config.RetestPresubmits(gc.Repo.FullName, skipContexts, forceRunContexts)
		requestedJobs = append(requestedJobs, retests...)
	}

	return runOrSkipRequested(c, pr, requestedJobs, forceRunContexts, gc.Body, gc.GUID)
}
