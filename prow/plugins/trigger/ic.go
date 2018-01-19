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

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
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

	baseRef, err := c.GitHubClient.GetRef(org, repo, "heads/"+pr.Base.Ref)
	if err != nil {
		return err
	}

	var changedFiles []string
	// shouldRun indicates if a job should actually run.
	shouldRun := func(j config.Presubmit) (bool, error) {
		if !j.RunsAgainstBranch(pr.Base.Ref) {
			return false, nil
		}
		if forceRunContexts[j.Context] || j.TriggerMatches(ic.Comment.Body) || j.RunIfChanged == "" {
			return true, nil
		}
		// Fetch the changed files from github at most once.
		if changedFiles == nil {
			changes, err := c.GitHubClient.GetPullRequestChanges(org, repo, number)
			if err != nil {
				return false, fmt.Errorf("error getting pull request changes: %v", err)
			}
			changedFiles = []string{}
			for _, change := range changes {
				changedFiles = append(changedFiles, change.Filename)
			}
		}
		return j.RunsAgainstChanges(changedFiles), nil
	}

	// For each job determine if any sharded version of the job runs.
	// This in turn determines which jobs to run and which contexts to mark as "Skipped".
	var toRunJobs []config.Presubmit
	toRun := sets.NewString()
	toSkip := sets.NewString()
	for _, job := range requestedJobs {
		runs, err := shouldRun(job)
		if err != nil {
			return err
		}
		if runs {
			toRunJobs = append(toRunJobs, job)
			toRun.Insert(job.Context)
		} else if !job.SkipReport {
			toSkip.Insert(job.Context)
		}
	}
	// 'Skip' any context that is required, but doesn't have a job shard run for it.
	for _, context := range toSkip.Difference(toRun).List() {
		if err := c.GitHubClient.CreateStatus(org, repo, pr.Head.SHA, github.Status{
			State:       github.StatusSuccess,
			Context:     context,
			Description: "Skipped",
		}); err != nil {
			return err
		}
	}

	var errors []error
	for _, job := range toRunJobs {
		c.Logger.Infof("Starting %s build.", job.Name)
		kr := kube.Refs{
			Org:     org,
			Repo:    repo,
			BaseRef: pr.Base.Ref,
			BaseSHA: baseRef,
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
		labels[github.EventGUID] = ic.GUID
		pj := pjutil.NewProwJob(pjutil.PresubmitSpec(job, kr), labels)
		c.Logger.WithFields(pjutil.ProwJobFields(&pj)).Info("Creating a new prowjob.")
		if _, err := c.KubeClient.CreateProwJob(pj); err != nil {
			errors = append(errors, err)
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("errors starting jobs: %v", errors)
	}
	return nil
}
