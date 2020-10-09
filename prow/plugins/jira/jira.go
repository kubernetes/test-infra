/*
Copyright 2020 The Kubernetes Authors.

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

package jira

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/andygrunwald/go-jira"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	jiraclient "k8s.io/test-infra/prow/jira"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	PluginName = "jira"
)

var (
	issueNameRegex = regexp.MustCompile(`\b[a-zA-Z]+-[0-9]+\b`)
)

func init() {
	plugins.RegisterGenericCommentHandler(PluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	// The Config field is omitted because this plugin is not configurable.
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The Jira plugin links Pull Requests and Issues to Jira issues",
	}
	return pluginHelp, nil
}

type githubClient interface {
	EditComment(org, repo string, id int, comment string) error
	GetIssue(org, repo string, number int) (*github.Issue, error)
	EditIssue(org, repo string, number int, issue *github.Issue) (*github.Issue, error)
}

func handleGenericComment(pc plugins.Agent, e github.GenericCommentEvent) error {
	return handle(pc.JiraClient, pc.GitHubClient, pc.Logger, &e)
}

func handle(jc jiraclient.Client, ghc githubClient, log *logrus.Entry, e *github.GenericCommentEvent) error {
	// Nothing to do on deletion
	if e.Action == github.GenericCommentActionDeleted {
		return nil
	}

	issueCandidateNames := issueNameRegex.FindAllString(e.Body, -1)
	issueCandidateNames = append(issueCandidateNames, issueNameRegex.FindAllString(e.IssueTitle, -1)...)
	if len(issueCandidateNames) == 0 {
		return nil
	}

	referencedIssues := sets.String{}
	for _, match := range issueCandidateNames {
		if referencedIssues.Has(match) {
			continue
		}
		_, err := jc.GetIssue(match)
		if err != nil {
			if !jiraclient.IsNotFound(err) {
				log.WithError(err).WithField("Issue", match).Error("Failed to get issue")
			}
			continue
		}
		referencedIssues.Insert(match)
	}

	wg := &sync.WaitGroup{}
	for _, issue := range referencedIssues.List() {
		wg.Add(1)
		go func(issue string) {
			defer wg.Done()
			if err := upsertGitHubLinkToIssue(log, issue, jc, e); err != nil {
				log.WithField("Issue", issue).WithError(err).Error("Failed to ensure GitHub link on Jira issue")
			}
		}(issue)
	}

	if err := updateComment(e, referencedIssues.UnsortedList(), jc.JiraURL(), ghc); err != nil {
		log.WithError(err).Error("Failed to insert links into body")
	}
	wg.Wait()

	return nil
}

func updateComment(e *github.GenericCommentEvent, validIssues []string, jiraBaseURL string, ghc githubClient) error {
	withLinks := insertLinksIntoComment(e.Body, validIssues, jiraBaseURL)
	if withLinks == e.Body {
		return nil
	}
	if e.CommentID != nil {
		return ghc.EditComment(e.Repo.Owner.Login, e.Repo.Name, *e.CommentID, withLinks)
	}

	issue, err := ghc.GetIssue(e.Repo.Owner.Login, e.Repo.Name, e.Number)
	if err != nil {
		return fmt.Errorf("failed to get issue %s/%s#%d: %w", e.Repo.Owner.Login, e.Repo.Name, e.Number, err)
	}

	// Check for the diff on the issues body in case the even't didn't have a commentID but did not originate
	// in issue creation, e.G. PRReviewEvent
	if withLinks := insertLinksIntoComment(issue.Body, validIssues, jiraBaseURL); withLinks != issue.Body {
		issue.Body = withLinks
		_, err := ghc.EditIssue(e.Repo.Owner.Login, e.Repo.Name, e.Number, issue)
		return err
	}

	return nil
}

func insertLinksIntoComment(body string, issueNames []string, jiraBaseURL string) string {
	for _, issue := range issueNames {
		replacement := fmt.Sprintf("[%s](%s/browse/%s)", issue, jiraBaseURL, issue)
		body = replaceStringIfHasntSquareBracketOrSlashPrefix(body, issue, replacement)
	}
	return body
}

// replaceStringIfHasntSquareBracketOrSlashPrefix replaces a string if it is not prefixed by
// a `[` which we use as heuristic for "Already replaced" or a `/` which we use as heuristic
// for "Part of a link in a previous replacement".
// It golang would support backreferences in regex replacements, this would have been a lot
// simpler.
func replaceStringIfHasntSquareBracketOrSlashPrefix(text, old, new string) string {
	if old == "" {
		return text
	}

	var result string

	// Golangs stdlib has no strings.IndexAll, only funcs to get the first
	// or last index for a substring. Definitions/condition/assignments are not
	// in the header of the loop because that makes it completely unreadable.
	var allOldIdx []int
	var startingIdx int
	for {
		idx := strings.Index(text[startingIdx:], old)
		if idx == -1 {
			break
		}
		idx = startingIdx + idx
		// Since we always look for a non-empty string, we know that idx++
		// can not be out of bounds
		allOldIdx = append(allOldIdx, idx)
		startingIdx = idx + 1
	}

	startingIdx = 0
	for _, idx := range allOldIdx {
		result += text[startingIdx:idx]
		if idx == 0 || (text[idx-1] != '[' && text[idx-1] != '/') {
			result += new
		} else {
			result += old
		}
		startingIdx = idx + len(old)
	}
	result += text[startingIdx:]

	return result
}

func upsertGitHubLinkToIssue(log *logrus.Entry, issueID string, jc jiraclient.Client, e *github.GenericCommentEvent) error {
	links, err := jc.GetRemoteLinks(issueID)
	if err != nil {
		return fmt.Errorf("failed to get remote links: %w", err)
	}

	url := e.HTMLURL
	if idx := strings.Index(url, "#"); idx != -1 {
		url = url[:idx]
	}
	for _, link := range links {
		if link.Object.URL == url {
			return nil
		}
	}

	link := &jira.RemoteLink{
		Object: &jira.RemoteLinkObject{
			URL:   url,
			Title: fmt.Sprintf("%s#%d: %s", e.Repo.FullName, e.Number, e.IssueTitle),
			Icon: &jira.RemoteLinkIcon{
				Url16x16: "https://github.com/favicon.ico",
				Title:    "GitHub",
			},
		},
	}

	if err := jc.AddRemoteLink(issueID, link); err != nil {
		return fmt.Errorf("failed to add remote link: %w", err)
	}
	log.Info("Created jira link")

	return nil
}
