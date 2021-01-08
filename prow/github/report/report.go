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
	"fmt"
	"strings"
	"text/template"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const (
	commentTag = "<!-- test report -->"
)

// GitHubClient provides a client interface to report job status updates
// through GitHub comments.
type GitHubClient interface {
	BotUserChecker() (func(candidate string) bool, error)
	CreateStatus(org, repo, ref string, s github.Status) error
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
	CreateComment(org, repo string, number int, comment string) error
	DeleteComment(org, repo string, ID int) error
	EditComment(org, repo string, ID int, comment string) error
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
	return "", fmt.Errorf("Unknown prowjob state: %v", pjState)
}

const (
	maxLen = 140 // https://developer.github.com/v3/repos/deployments/#parameters-2
	elide  = " ... "
)

// truncate converts "really long messages" into "really ... messages".
func truncate(in string) string {
	const (
		half = (maxLen - len(elide)) / 2
	)
	if len(in) <= maxLen {
		return in
	}
	return in[:half] + elide + in[len(in)-half:]
}

// reportStatus should be called on any prowjob status changes
func reportStatus(ghc GitHubClient, pj prowapi.ProwJob) error {
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
		if err := ghc.CreateStatus(refs.Org, refs.Repo, sha, github.Status{
			State:       contextState,
			Description: truncate(pj.Status.Description),
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
func Report(ghc GitHubClient, reportTemplate *template.Template, pj prowapi.ProwJob, validTypes []prowapi.ProwJobType) error {
	if ghc == nil {
		return fmt.Errorf("trying to report pj %s, but found empty github client", pj.ObjectMeta.Name)
	}

	if !ShouldReport(pj, validTypes) {
		return nil
	}

	refs := pj.Spec.Refs
	// we are not reporting for batch jobs, we can consider support that in the future
	if len(refs.Pulls) > 1 {
		return nil
	}

	if err := reportStatus(ghc, pj); err != nil {
		return fmt.Errorf("error setting status: %w", err)
	}

	// Report manually aborted Jenkins jobs and jobs with invalid pod specs alongside
	// test successes/failures.
	if !pj.Complete() {
		return nil
	}

	if len(refs.Pulls) == 0 {
		return nil
	}

	ics, err := ghc.ListIssueComments(refs.Org, refs.Repo, refs.Pulls[0].Number)
	if err != nil {
		return fmt.Errorf("error listing comments: %v", err)
	}
	botNameChecker, err := ghc.BotUserChecker()
	if err != nil {
		return fmt.Errorf("error getting bot name checker: %w", err)
	}
	deletes, entries, updateID := parseIssueComments(pj, botNameChecker, ics)
	for _, delete := range deletes {
		if err := ghc.DeleteComment(refs.Org, refs.Repo, delete); err != nil {
			return fmt.Errorf("error deleting comment: %v", err)
		}
	}
	if len(entries) > 0 {
		comment, err := createComment(reportTemplate, pj, entries)
		if err != nil {
			return fmt.Errorf("generating comment: %v", err)
		}
		if updateID == 0 {
			if err := ghc.CreateComment(refs.Org, refs.Repo, refs.Pulls[0].Number, comment); err != nil {
				return fmt.Errorf("error creating comment: %v", err)
			}
		} else {
			if err := ghc.EditComment(refs.Org, refs.Repo, updateID, comment); err != nil {
				return fmt.Errorf("error updating comment: %v", err)
			}
		}
	}
	return nil
}

// parseIssueComments returns a list of comments to delete, a list of table
// entries, and the ID of the comment to update. If there are no table entries
// then don't make a new comment. Otherwise, if the comment to update is 0,
// create a new comment.
func parseIssueComments(pj prowapi.ProwJob, isBot func(string) bool, ics []github.IssueComment) ([]int, []string, int) {
	var delete []int
	var previousComments []int
	var latestComment int
	var entries []string
	// First accumulate result entries and comment IDs
	for _, ic := range ics {
		if !isBot(ic.User.Login) {
			continue
		}
		// Old report comments started with the context. Delete them.
		// TODO(spxtr): Delete this check a few weeks after this merges.
		if strings.HasPrefix(ic.Body, pj.Spec.Context) {
			delete = append(delete, ic.ID)
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
		if pj.Spec.Context == f1[0] {
			keep = false
		}
		if keep {
			newEntries = append(newEntries, entries[i])
		}
	}
	var createNewComment bool
	if string(pj.Status.State) == github.StatusFailure {
		newEntries = append(newEntries, createEntry(pj))
		createNewComment = true
	}
	delete = append(delete, previousComments...)
	if (createNewComment || len(newEntries) == 0) && latestComment != 0 {
		delete = append(delete, latestComment)
		latestComment = 0
	}
	return delete, newEntries, latestComment
}

func createEntry(pj prowapi.ProwJob) string {
	return strings.Join([]string{
		pj.Spec.Context,
		pj.Spec.Refs.Pulls[0].SHA,
		fmt.Sprintf("[link](%s)", pj.Status.URL),
		fmt.Sprintf("`%s`", pj.Spec.RerunCommand),
	}, " | ")
}

// createComment take a ProwJob and a list of entries generated with
// createEntry and returns a nicely formatted comment. It may fail if template
// execution fails.
func createComment(reportTemplate *template.Template, pj prowapi.ProwJob, entries []string) (string, error) {
	plural := ""
	if len(entries) > 1 {
		plural = "s"
	}
	var b bytes.Buffer
	if reportTemplate != nil {
		if err := reportTemplate.Execute(&b, &pj); err != nil {
			return "", err
		}
	}
	lines := []string{
		fmt.Sprintf("@%s: The following test%s **failed**, say `/retest` to rerun all failed tests:", pj.Spec.Refs.Pulls[0].Author, plural),
		"",
		"Test name | Commit | Details | Rerun command",
		"--- | --- | --- | ---",
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
