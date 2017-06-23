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

package plank

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/plugins"
)

const commentTag = "<!-- test report -->"

var tmpl = template.Must(template.New("report").Parse(`[Full PR test history](https://k8s-gubernator.appspot.com/pr/{{if eq .Spec.Refs.Org "kubernetes"}}{{if eq .Spec.Refs.Repo "kubernetes"}}{{else}}{{.Spec.Refs.Repo}}/{{end}}{{else}}{{.Spec.Refs.Org}}_{{.Spec.Refs.Repo}}/{{end}}{{with index .Spec.Refs.Pulls 0}}{{.Number}}{{end}}). [Your PR dashboard](https://k8s-gubernator.appspot.com/pr/{{with index .Spec.Refs.Pulls 0}}{{.Author}}{{end}}). Please help us cut down on flakes by [linking to](https://github.com/kubernetes/community/blob/master/contributors/devel/flaky-tests.md#filing-issues-for-flaky-tests) an [open issue](https://github.com/{{.Spec.Refs.Org}}/{{.Spec.Refs.Repo}}/issues?q=is:issue+is:open) when you hit one in your PR.`))

func (c *Controller) report(pj kube.ProwJob) error {
	if !pj.Spec.Report {
		return nil
	}
	refs := pj.Spec.Refs
	if len(refs.Pulls) != 1 {
		return fmt.Errorf("prowjob %s has %d pulls, not 1", pj.Metadata.Name, len(refs.Pulls))
	}
	if err := c.ghc.CreateStatus(refs.Org, refs.Repo, refs.Pulls[0].SHA, github.Status{
		State:       string(pj.Status.State),
		Description: pj.Status.Description,
		Context:     pj.Spec.Context,
		TargetURL:   pj.Status.URL,
	}); err != nil {
		return fmt.Errorf("error setting status: %v", err)
	}
	if pj.Status.State != github.StatusSuccess && pj.Status.State != github.StatusFailure {
		return nil
	}
	ics, err := c.ghc.ListIssueComments(refs.Org, refs.Repo, refs.Pulls[0].Number)
	if err != nil {
		return fmt.Errorf("error listing comments: %v", err)
	}
	deletes, entries, updateID := parseIssueComments(pj, c.ghc.BotName(), ics)
	for _, delete := range deletes {
		if err := c.ghc.DeleteComment(refs.Org, refs.Repo, delete); err != nil {
			return fmt.Errorf("error deleting comment: %v", err)
		}
	}
	if len(entries) > 0 {
		comment, err := createComment(pj, entries)
		if err != nil {
			return fmt.Errorf("generating comment: %v", err)
		}
		if updateID == 0 {
			if err := c.ghc.CreateComment(refs.Org, refs.Repo, refs.Pulls[0].Number, comment); err != nil {
				return fmt.Errorf("error creating comment: %v", err)
			}
		} else {
			if err := c.ghc.EditComment(refs.Org, refs.Repo, updateID, comment); err != nil {
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
func parseIssueComments(pj kube.ProwJob, botName string, ics []github.IssueComment) ([]int, []string, int) {
	var delete []int
	var previousComments []int
	var latestComment int
	var entries []string
	// First accumulate result entries and comment IDs
	for _, ic := range ics {
		if ic.User.Login != botName {
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

func createEntry(pj kube.ProwJob) string {
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
func createComment(pj kube.ProwJob, entries []string) (string, error) {
	plural := ""
	if len(entries) > 1 {
		plural = "s"
	}
	var b bytes.Buffer
	if err := tmpl.Execute(&b, &pj); err != nil {
		return "", err
	}
	lines := []string{
		fmt.Sprintf("@%s: The following test%s **failed**, say `/retest` to rerun them all:", pj.Spec.Refs.Pulls[0].Author, plural),
		"",
		"Test name | Commit | Details | Rerun command",
		"--- | --- | --- | ---",
	}
	lines = append(lines, entries...)
	lines = append(lines, []string{
		"",
		b.String(),
		"",
		"<details>",
		"",
		plugins.AboutThisBot,
		"</details>",
		commentTag,
	}...)
	return strings.Join(lines, "\n"), nil
}
