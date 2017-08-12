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

package help

import (
	"regexp"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const pluginName = "help"

var (
	helpLabel    = "help-wanted"
	helpRe       = regexp.MustCompile(`(?mi)^/help\s*$`)
	helpRemoveRe = regexp.MustCompile(`(?mi)^/remove-help\s*$`)
)

type assignEvent struct {
	action    string
	body      string
	login     string
	org       string
	repo      string
	url       string
	number    int
	issue     github.Issue
	assignees []github.User
	hasLabel  func(label string) (bool, error)
}

func init() {
	plugins.RegisterIssueCommentHandler(pluginName, handleIssueComment)
	plugins.RegisterIssueHandler(pluginName, handleIssue)
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	AddLabel(owner, repo string, number int, label string) error
	RemoveLabel(owner, repo string, number int, label string) error
}

func handleIssueComment(pc plugins.PluginClient, ic github.IssueCommentEvent) error {
	// Only consider open issues.
	if ic.Issue.IsPullRequest() || ic.Issue.State != "open" {
		return nil
	}

	ae := assignEvent{
		action:    ic.Action,
		body:      ic.Comment.Body,
		login:     ic.Comment.User.Login,
		org:       ic.Repo.Owner.Login,
		repo:      ic.Repo.Name,
		url:       ic.Comment.HTMLURL,
		number:    ic.Issue.Number,
		issue:     ic.Issue,
		assignees: ic.Issue.Assignees,
		hasLabel:  func(label string) (bool, error) { return ic.Issue.HasLabel(label), nil },
	}
	return handle(pc.GitHubClient, pc.Logger, ae)
}

func handleIssue(pc plugins.PluginClient, i github.IssueEvent) error {
	// Only consider open issues.
	if i.Issue.IsPullRequest() || i.Issue.State != "open" {
		return nil
	}

	ae := assignEvent{
		action:    i.Action,
		body:      i.Issue.Body,
		login:     i.Issue.User.Login,
		org:       i.Repo.Owner.Login,
		repo:      i.Repo.Name,
		url:       i.Issue.HTMLURL,
		number:    i.Issue.Number,
		issue:     i.Issue,
		assignees: i.Issue.Assignees,
		hasLabel:  func(label string) (bool, error) { return i.Issue.HasLabel(label), nil },
	}
	return handle(pc.GitHubClient, pc.Logger, ae)
}

func handle(gc githubClient, log *logrus.Entry, ae assignEvent) error {
	// Determine if the issue has the help label
	hasHelp, err := ae.hasLabel(helpLabel)
	if err != nil {
		log.WithError(err).Errorf("Unable to determine if request has %s label.", helpLabel)
	}

	// Determine if the issue has been assigned
	isAssigned := false
	if len(ae.assignees) != 0 {
		isAssigned = true
	}

	// If PR has help label and also an assignee, remove label
	if hasHelp && isAssigned {
		if err := gc.RemoveLabel(ae.org, ae.repo, ae.number, helpLabel); err != nil {
			log.WithError(err).Errorf("Github failed to remove the following label: %s", helpLabel)
		}

		return nil
	}

	// If PR has help label and we're asking it to be removed, remove label
	if hasHelp && helpRemoveRe.MatchString(ae.body) {
		if err := gc.RemoveLabel(ae.org, ae.repo, ae.number, helpLabel); err != nil {
			log.WithError(err).Errorf("Github failed to remove the following label: %s", helpLabel)
		}

		return nil
	}

	// If PR does not have an assignee, does not have the label,
	// and we're asking it to be added, add the label
	if !hasHelp && !isAssigned && helpRe.MatchString(ae.body) {
		msg := `
	This request has been marked as needing help from a contributor.

	Please ensure the request meets the requirements listed [here](https://git.k8s.io/community/contributors/devel/help-wanted.md).

	If this request no longer meets these requirements, the label can be removed
	by commenting with the "/remove-help" command.
	`
		if err := gc.CreateComment(ae.org, ae.repo, ae.number, plugins.FormatResponseRaw(ae.body, ae.url, ae.login, msg)); err != nil {
			log.WithError(err).Errorf("Could not create comment \"%s\".", msg)
		}
		if err := gc.AddLabel(ae.org, ae.repo, ae.number, helpLabel); err != nil {
			log.WithError(err).Errorf("Github failed to add the following label: %s", helpLabel)
		}

		return nil
	}

	// If the issue is already assigned, comment that we can't do anything.
	if isAssigned && helpRe.MatchString(ae.body) {
		msg := "Cannot add `help-wanted` label as the issue is already assigned."
		if err := gc.CreateComment(ae.org, ae.repo, ae.number, plugins.FormatResponseRaw(ae.body, ae.url, ae.login, msg)); err != nil {
			log.WithError(err).Errorf("Could not create comment \"%s\".", msg)
		}

		return nil
	}

	return nil
}
