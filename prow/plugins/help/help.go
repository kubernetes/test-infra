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
	helpLabel = "help-wanted"
	helpRe    = regexp.MustCompile(`(?mi)^/help\s*$`)
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

	// Determine if we're being asked for help
	wantHelp := false
	if helpRe.MatchString(ae.body) {
		wantHelp = true
	}

	// If PR has help label and also an assignee, remove label
	if hasHelp && isAssigned {
		if err := gc.RemoveLabel(ae.org, ae.repo, ae.number, helpLabel); err != nil {
			log.WithError(err).Errorf("Github failed to remove the following label: %s", helpLabel)
		} else {
			log.Infof("Removing %s label.", helpLabel)
		}
	}

	// If PR does not have an assignee, does not have the label,
	// and we get a help request, add the label
	if !hasHelp && !isAssigned && wantHelp {
		if err := gc.AddLabel(ae.org, ae.repo, ae.number, helpLabel); err != nil {
			log.WithError(err).Errorf("Github failed to add the following label: %s", helpLabel)
		} else {
			log.Infof("Adding %s label.", helpLabel)
		}
	}

	// If the issue is already assigned, comment that we can't do anything.
	if isAssigned && wantHelp {
		msg := "The issue is already assigned."
		if err := gc.CreateComment(ae.org, ae.repo, ae.number, plugins.FormatResponseRaw(ae.body, ae.url, ae.login, msg)); err != nil {
			log.WithError(err).Errorf("Could not create comment \"%s\".", msg)
		} else {
			log.Infof("Commenting with \"%s\".", msg)
		}
	}

	return nil
}
