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
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/plugins"
)

var (
	okToTest = regexp.MustCompile(`(?m)^/ok-to-test\s*$`)
	retest   = regexp.MustCompile(`(?m)^/retest\s*$`)
)

func handleCE(c client, trustedOrg string, e *github.GenericCommentEvent) error {
	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	number := e.Number
	commentAuthor := e.User.Login
	// Only take action when a comment is first created.
	if e.Action != github.GenericCommentActionCreated {
		return nil
	}
	// If it's not an open PR, skip it.
	if !e.IsPR {
		return nil
	}
	if e.IssueState != "open" {
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

	var changedFiles []string
	files := func() ([]string, error) {
		if changedFiles != nil {
			return changedFiles, nil
		}
		changes, err := c.GitHubClient.GetPullRequestChanges(org, repo, number)
		if err != nil {
			return nil, err
		}
		changedFiles = []string{}
		for _, change := range changes {
			changedFiles = append(changedFiles, change.Filename)
		}
		return changedFiles, nil
	}

	// Which jobs does the comment want us to run?
	testAll := okToTest.MatchString(e.Body)
	shouldRetestFailed := retest.MatchString(e.Body)
	requestedJobs, err := c.Config.MatchingPresubmits(e.Repo.FullName, e.Body, testAll, files)
	if err != nil {
		return err
	}
	if !shouldRetestFailed && len(requestedJobs) == 0 {
		return nil
	}

	pr, err := c.GitHubClient.GetPullRequest(org, repo, number)
	if err != nil {
		return err
	}

	if shouldRetestFailed {
		combinedStatus, err := c.GitHubClient.GetCombinedStatus(org, repo, pr.Head.SHA)
		if err != nil {
			return err
		}
		skipContexts := make(map[string]bool) // these succeeded or are running
		runContexts := make(map[string]bool)  // these failed and should be re-run
		for _, status := range combinedStatus.Statuses {
			state := status.State
			if state == github.StatusSuccess || state == github.StatusPending {
				skipContexts[status.Context] = true
			} else if state == github.StatusError || state == github.StatusFailure {
				runContexts[status.Context] = true
			}
		}
		retests, err := c.Config.RetestPresubmits(e.Repo.FullName, skipContexts, runContexts, files)
		if err != nil {
			return err
		}
		for _, job := range retests {
			requestedJobs[job.Name] = job
		}
	}

	var comments []github.IssueComment
	// Skip untrusted users.
	orgMember, err := c.GitHubClient.IsMember(trustedOrg, commentAuthor)
	if err != nil {
		return err
	} else if !orgMember {
		comments, err = c.GitHubClient.ListIssueComments(pr.Base.Repo.Owner.Login, pr.Base.Repo.Name, pr.Number)
		if err != nil {
			return err
		}
		trusted, err := trustedPullRequest(c.GitHubClient, *pr, trustedOrg, comments)
		if err != nil {
			return err
		}
		if !trusted {
			resp := fmt.Sprintf("you can't request testing unless you are a [%s](https://github.com/orgs/%s/people) member.", trustedOrg, trustedOrg)
			c.Logger.Infof("Commenting \"%s\".", resp)
			return c.GitHubClient.CreateComment(org, repo, number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, resp))
		}
	}

	if testAll {
		prHasLabel, err := checkPRHasLabel(c.GitHubClient, *pr, needsOkToTest)
		if err != nil {
			c.Logger.Warn(err)
		}
		if prHasLabel {
			err = clearStaleComments(c.GitHubClient, trustedOrg, *pr, comments)
			if err != nil {
				c.Logger.Warnf("Failed to clear stale comments: %v.", err)
			}
			err = removePRLabelIfExists(c.GitHubClient, *pr, needsOkToTest)
			if err != nil {
				c.Logger.Warnf("Failed to check/remove label: %v.", err)
			}
		}
	}

	ref, err := c.GitHubClient.GetRef(org, repo, "heads/"+pr.Base.Ref)
	if err != nil {
		return err
	}

	var errors []error
	for _, job := range requestedJobs {
		build := true
		if !job.RunsAgainstBranch(pr.Base.Ref) {
			build = false
		} else if !job.AlwaysRun && job.RunIfChanged != "" {
			changes, err := files()
			if err != nil {
				return err
			}
			if !job.RunsAgainstChanges(changes) {
				build = false
			}
		}
		if !build {
			if !job.SkipReport {
				if err := c.GitHubClient.CreateStatus(org, repo, pr.Head.SHA, github.Status{
					State:       github.StatusSuccess,
					Context:     job.Context,
					Description: "Skipped",
				}); err != nil {
					return err
				}
			}
			continue
		}

		c.Logger.Infof("Starting %s build.", job.Name)
		kr := kube.Refs{
			Org:     org,
			Repo:    repo,
			BaseRef: pr.Base.Ref,
			BaseSHA: ref,
			Pulls: []kube.Pull{
				{
					Number: number,
					Author: pr.User.Login,
					SHA:    pr.Head.SHA,
				},
			},
		}
		labels := make(map[string]string)
		for k, v := range job.Labels {
			labels[k] = v
		}
		labels[github.EventGUID] = e.GUID
		if _, err := c.KubeClient.CreateProwJob(pjutil.NewProwJob(pjutil.PresubmitSpec(job, kr), labels)); err != nil {
			errors = append(errors, err)
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("errors starting jobs: %v", errors)
	}
	return nil
}
