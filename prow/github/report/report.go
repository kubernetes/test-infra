/*
Copyright 2017 The Kubernetes Authors.

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

// Package report contains helpers for writing comments and updating
// statuses in GitHub.
package report

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/plugins"
)

const (
	commentTag = "<!-- test report -->"
)

// GitHubClient provides a client interface to report job status updates
// through GitHub comments.
type GitHubClient interface {
	BotUserCheckerWithContext(ctx context.Context) (func(candidate string) bool, error)
	CreateStatusWithContext(ctx context.Context, org, repo, ref string, s github.Status) error
	ListIssueCommentsWithContext(ctx context.Context, org, repo string, number int) ([]github.IssueComment, error)
	CreateCommentWithContext(ctx context.Context, org, repo string, number int, comment string) error
	DeleteCommentWithContext(ctx context.Context, org, repo string, ID int) error
	EditCommentWithContext(ctx context.Context, org, repo string, ID int, comment string) error
}

// prowjobStateToGitHubStatus maps prowjob status to github states.
// GitHub states can be one of error, failure, pending, or success.
// https://developer.github.com/v3/repos/statuses/#create-a-status
func prowjobStateToGitHubStatus(pjState prowapi.ProwJobState) (string, error) {
	switch pjState {
	case prowapi.TriggeredState:
		return github.StatusPending, nil
	case prowapi.PendingState:
		return github.StatusPending, nil
	case prowapi.SuccessState:
		return github.StatusSuccess, nil
	case prowapi.ErrorState:
		return github.StatusError, nil
	case prowapi.FailureState:
		return github.StatusFailure, nil
	case prowapi.AbortedState:
		return github.StatusFailure, nil
	}
	return "", fmt.Errorf("Unknown prowjob state: %s", pjState)
}

// reportStatus should be called on any prowjob status changes
func reportStatus(ctx context.Context, ghc GitHubClient, pj prowapi.ProwJob) error {
	refs := pj.Spec.Refs
	if pj.Spec.Report {
		contextState, err := prowjobStateToGitHubStatus(pj.Status.State)
		if err != nil {
			return err
		}
		sha := refs.BaseSHA
		if len(refs.Pulls) > 0 {
			sha = refs.Pulls[0].SHA
		}
		if err := ghc.CreateStatusWithContext(ctx, refs.Org, refs.Repo, sha, github.Status{
			State:       contextState,
			Description: config.ContextDescriptionWithBaseSha(pj.Status.Description, refs.BaseSHA),
			Context:     pj.Spec.Context, // consider truncating this too
			TargetURL:   pj.Status.URL,
		}); err != nil {
			return err
		}
	}
	return nil
}

// TODO(krzyzacy):
// Move this logic into github/reporter, once we unify all reporting logic to crier
func ShouldReport(pj prowapi.ProwJob, validTypes []prowapi.ProwJobType) bool {
	valid := false
	for _, t := range validTypes {
		if pj.Spec.Type == t {
			valid = true
		}
	}

	if !valid {
		return false
	}

	if !pj.Spec.Report {
		return false
	}

	return true
}

// Report is creating/updating/removing reports in GitHub based on the state of
// the provided ProwJob.
func Report(ctx context.Context, ghc GitHubClient, reportTemplate *template.Template, pj prowapi.ProwJob, config config.GitHubReporter) error {
	if err := ReportStatusContext(ctx, ghc, pj, config); err != nil {
		return err
	}
	return ReportComment(ctx, ghc, reportTemplate, []v1.ProwJob{pj}, config, false)
}

// ReportStatusContext reports prowjob status on a PR.
func ReportStatusContext(ctx context.Context, ghc GitHubClient, pj prowapi.ProwJob, config config.GitHubReporter) error {
	if ghc == nil {
		return fmt.Errorf("trying to report pj %s, but found empty github client", pj.ObjectMeta.Name)
	}

	if !ShouldReport(pj, config.JobTypesToReport) {
		return nil
	}

	refs := pj.Spec.Refs
	// we are not reporting for batch jobs, we can consider support that in the future
	if len(refs.Pulls) > 1 {
		return nil
	}

	if err := reportStatus(ctx, ghc, pj); err != nil {
		return fmt.Errorf("error setting status: %w", err)
	}
	return nil
}

// ReportComment takes multiple prowjobs as input. When there are more than one
// prowjob, they are required to have identical refs, aka they are the same repo
// and the same pull request.
func ReportComment(ctx context.Context, ghc GitHubClient, reportTemplate *template.Template, pjs []prowapi.ProwJob, config config.GitHubReporter, mustCreate bool) error {
	if ghc == nil {
		return errors.New("trying to report pj, but found empty github client")
	}

	var validPjs []v1.ProwJob
	for _, pj := range pjs {
		// Report manually aborted Jenkins jobs and jobs with invalid pod specs alongside
		// test successes/failures.
		if ShouldReport(pj, config.JobTypesToReport) && pj.Complete() {
			validPjs = append(validPjs, pj)
		}
	}
	if len(validPjs) == 0 {
		return nil
	}

	// Multiple prow jobs passed in to this function requires that all prowjobs from
	// the input have exactly the same refs. Pick the ref from the first PR for checking
	// whether to report or not.
	refs := validPjs[0].Spec.Refs
	// we are not reporting for batch jobs, we can consider support that in the future
	if refs == nil || len(refs.Pulls) != 1 {
		return nil
	}

	ics, err := ghc.ListIssueCommentsWithContext(ctx, refs.Org, refs.Repo, refs.Pulls[0].Number)
	if err != nil {
		return fmt.Errorf("error listing comments: %w", err)
	}
	botNameChecker, err := ghc.BotUserCheckerWithContext(ctx)
	if err != nil {
		return fmt.Errorf("error getting bot name checker: %w", err)
	}
	deletes, entries, updateID := parseIssueComments(validPjs, botNameChecker, ics)
	for _, delete := range deletes {
		if err := ghc.DeleteCommentWithContext(ctx, refs.Org, refs.Repo, delete); err != nil {
			return fmt.Errorf("error deleting comment: %w", err)
		}
	}
	if len(entries) > 0 || mustCreate {
		comment, err := createComment(reportTemplate, validPjs, entries)
		if err != nil {
			return fmt.Errorf("generating comment: %w", err)
		}
		if updateID == 0 {
			if err := ghc.CreateCommentWithContext(ctx, refs.Org, refs.Repo, refs.Pulls[0].Number, comment); err != nil {
				return fmt.Errorf("error creating comment: %w", err)
			}
		} else {
			if err := ghc.EditCommentWithContext(ctx, refs.Org, refs.Repo, updateID, comment); err != nil {
				return fmt.Errorf("error updating comment: %w", err)
			}
		}
	}
	return nil
}

// parseIssueComments returns a list of comments to delete, a list of table
// entries, and the ID of the comment to update. If there are no table entries
// then don't make a new comment. Otherwise, if the comment to update is 0,
// create a new comment.
func parseIssueComments(pjs []prowapi.ProwJob, isBot func(string) bool, ics []github.IssueComment) ([]int, []string, int) {
	var delete []int
	var previousComments []int
	var latestComment int
	var entries []string
	// First accumulate result entries and comment IDs
	for _, ic := range ics {
		if !isBot(ic.User.Login) {
			continue
		}
		if !strings.Contains(ic.Body, commentTag) {
			continue
		}
		if latestComment != 0 {
			previousComments = append(previousComments, latestComment)
		}
		latestComment = ic.ID
		var tracking bool
		for _, line := range strings.Split(ic.Body, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "---") {
				tracking = true
			} else if len(line) == 0 {
				tracking = false
			} else if tracking {
				entries = append(entries, line)
			}
		}
	}
	var newEntries []string
	// Next decide which entries to keep.
	pjsMap := make(map[string]prowapi.ProwJob)
	for _, pj := range pjs {
		pjsMap[pj.Spec.Context] = pj
	}
	for i := range entries {
		keep := true
		f1 := strings.Split(entries[i], " | ")
		for j := range entries {
			if i == j {
				continue
			}
			f2 := strings.Split(entries[j], " | ")
			// Use the newer results if there are multiple.
			if j > i && f2[0] == f1[0] {
				keep = false
			}
		}
		// Use the current result if there is an old one.
		if _, ok := pjsMap[f1[0]]; ok {
			keep = false
		}
		if keep {
			newEntries = append(newEntries, entries[i])
		}
	}
	var createNewComment bool
	for _, pj := range pjs {
		if string(pj.Status.State) == github.StatusFailure {
			newEntries = append(newEntries, createEntry(pj))
			createNewComment = true
		}
	}
	delete = append(delete, previousComments...)
	if (createNewComment || len(newEntries) == 0) && latestComment != 0 {
		delete = append(delete, latestComment)
		latestComment = 0
	}
	return delete, newEntries, latestComment
}

func createEntry(pj prowapi.ProwJob) string {
	required := "unknown"

	if pj.Spec.Type == prowapi.PresubmitJob {
		if label, exist := pj.Labels[kube.IsOptionalLabel]; exist {
			if optional, err := strconv.ParseBool(label); err == nil {
				required = strconv.FormatBool(!optional)
			}
		}
	}

	return strings.Join([]string{
		pj.Spec.Context,
		pj.Spec.Refs.Pulls[0].SHA,
		fmt.Sprintf("[link](%s)", pj.Status.URL),
		required,
		fmt.Sprintf("`%s`", pj.Spec.RerunCommand),
	}, " | ")
}

// createComment take a ProwJob and a list of entries generated with
// createEntry and returns a nicely formatted comment. It may fail if template
// execution fails.
func createComment(reportTemplate *template.Template, pjs []prowapi.ProwJob, entries []string) (string, error) {
	if len(pjs) == 0 {
		return "", nil
	}
	plural := ""
	if len(entries) > 1 {
		plural = "s"
	}
	var b bytes.Buffer
	// The report template is usually related to the PR not a specific PJ,
	// even though it is using the PJ in the template. This is kind of unfortunate
	// and doesn't really make sense given that we maintain one failure comment
	// on PRs, not one per PJ. So we might be better off using the first PJ
	// and still executing the template even if there are multiple PJs.
	if reportTemplate != nil {
		if err := reportTemplate.Execute(&b, &pjs[0]); err != nil {
			return "", err
		}
	}
	lines := []string{
		fmt.Sprintf("@%s: The following test%s **failed**, say `/retest` to rerun all failed tests or `/retest-required` to rerun all mandatory failed tests:", pjs[0].Spec.Refs.Pulls[0].Author, plural),
		"",
		"Test name | Commit | Details | Required | Rerun command",
		"--- | --- | --- | --- | ---",
	}
	if len(entries) == 0 { // No test failed
		lines = []string{
			fmt.Sprintf("@%s: all tests **passed!**", pjs[0].Spec.Refs.Pulls[0].Author),
			"",
		}
	}
	lines = append(lines, entries...)
	if reportTemplate != nil {
		lines = append(lines, "", b.String())
	}
	lines = append(lines, []string{
		"",
		"<details>",
		"",
		plugins.AboutThisBot,
		"</details>",
		commentTag,
	}...)
	return strings.Join(lines, "\n"), nil
}
