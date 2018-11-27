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

package hook

import (
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const failedCommentCoerceFmt = "Could not coerce %s event to a GenericCommentEvent. Unknown 'action': %q."

var (
	nonCommentIssueActions = map[github.IssueEventAction]bool{
		github.IssueActionAssigned:     true,
		github.IssueActionUnassigned:   true,
		github.IssueActionLabeled:      true,
		github.IssueActionUnlabeled:    true,
		github.IssueActionMilestoned:   true,
		github.IssueActionDemilestoned: true,
		github.IssueActionClosed:       true,
		github.IssueActionReopened:     true,
	}
	nonCommentPullRequestActions = map[github.PullRequestEventAction]bool{
		github.PullRequestActionAssigned:             true,
		github.PullRequestActionUnassigned:           true,
		github.PullRequestActionReviewRequested:      true,
		github.PullRequestActionReviewRequestRemoved: true,
		github.PullRequestActionLabeled:              true,
		github.PullRequestActionUnlabeled:            true,
		github.PullRequestActionClosed:               true,
		github.PullRequestActionReopened:             true,
		github.PullRequestActionSynchronize:          true,
	}
)

func (s *Server) handleReviewEvent(l *logrus.Entry, re github.ReviewEvent) {
	defer s.wg.Done()
	l = l.WithFields(logrus.Fields{
		github.OrgLogField:  re.Repo.Owner.Login,
		github.RepoLogField: re.Repo.Name,
		github.PrLogField:   re.PullRequest.Number,
		"review":            re.Review.ID,
		"reviewer":          re.Review.User.Login,
		"url":               re.Review.HTMLURL,
	})
	l.Infof("Review %s.", re.Action)
	for p, h := range s.Plugins.ReviewEventHandlers(re.PullRequest.Base.Repo.Owner.Login, re.PullRequest.Base.Repo.Name) {
		s.wg.Add(1)
		go func(p string, h plugins.ReviewEventHandler) {
			defer s.wg.Done()
			agent := plugins.NewAgent(s.ConfigAgent, s.Plugins, s.ClientAgent, l.WithField("plugin", p))
			agent.InitializeCommentPruner(
				re.Repo.Owner.Login,
				re.Repo.Name,
				re.PullRequest.Number,
			)
			if err := h(agent, re); err != nil {
				agent.Logger.WithError(err).Error("Error handling ReviewEvent.")
			}
		}(p, h)
	}
	action := genericCommentAction(string(re.Action))
	if action == "" {
		l.Errorf(failedCommentCoerceFmt, "pull_request_review", string(re.Action))
		return
	}
	s.handleGenericComment(
		l,
		&github.GenericCommentEvent{
			GUID:         re.GUID,
			IsPR:         true,
			Action:       action,
			Body:         re.Review.Body,
			HTMLURL:      re.Review.HTMLURL,
			Number:       re.PullRequest.Number,
			Repo:         re.Repo,
			User:         re.Review.User,
			IssueAuthor:  re.PullRequest.User,
			Assignees:    re.PullRequest.Assignees,
			IssueState:   re.PullRequest.State,
			IssueBody:    re.PullRequest.Body,
			IssueHTMLURL: re.PullRequest.HTMLURL,
		},
	)
}

func (s *Server) handleReviewCommentEvent(l *logrus.Entry, rce github.ReviewCommentEvent) {
	defer s.wg.Done()
	l = l.WithFields(logrus.Fields{
		github.OrgLogField:  rce.Repo.Owner.Login,
		github.RepoLogField: rce.Repo.Name,
		github.PrLogField:   rce.PullRequest.Number,
		"review":            rce.Comment.ReviewID,
		"commenter":         rce.Comment.User.Login,
		"url":               rce.Comment.HTMLURL,
	})
	l.Infof("Review comment %s.", rce.Action)
	for p, h := range s.Plugins.ReviewCommentEventHandlers(rce.PullRequest.Base.Repo.Owner.Login, rce.PullRequest.Base.Repo.Name) {
		s.wg.Add(1)
		go func(p string, h plugins.ReviewCommentEventHandler) {
			defer s.wg.Done()
			agent := plugins.NewAgent(s.ConfigAgent, s.Plugins, s.ClientAgent, l.WithField("plugin", p))
			agent.InitializeCommentPruner(
				rce.Repo.Owner.Login,
				rce.Repo.Name,
				rce.PullRequest.Number,
			)
			if err := h(agent, rce); err != nil {
				agent.Logger.WithError(err).Error("Error handling ReviewCommentEvent.")
			}
		}(p, h)
	}
	action := genericCommentAction(string(rce.Action))
	if action == "" {
		l.Errorf(failedCommentCoerceFmt, "pull_request_review_comment", string(rce.Action))
		return
	}
	s.handleGenericComment(
		l,
		&github.GenericCommentEvent{
			GUID:         rce.GUID,
			IsPR:         true,
			Action:       action,
			Body:         rce.Comment.Body,
			HTMLURL:      rce.Comment.HTMLURL,
			Number:       rce.PullRequest.Number,
			Repo:         rce.Repo,
			User:         rce.Comment.User,
			IssueAuthor:  rce.PullRequest.User,
			Assignees:    rce.PullRequest.Assignees,
			IssueState:   rce.PullRequest.State,
			IssueBody:    rce.PullRequest.Body,
			IssueHTMLURL: rce.PullRequest.HTMLURL,
		},
	)
}

func (s *Server) handlePullRequestEvent(l *logrus.Entry, pr github.PullRequestEvent) {
	defer s.wg.Done()
	l = l.WithFields(logrus.Fields{
		github.OrgLogField:  pr.Repo.Owner.Login,
		github.RepoLogField: pr.Repo.Name,
		github.PrLogField:   pr.Number,
		"author":            pr.PullRequest.User.Login,
		"url":               pr.PullRequest.HTMLURL,
	})
	l.Infof("Pull request %s.", pr.Action)
	for p, h := range s.Plugins.PullRequestHandlers(pr.PullRequest.Base.Repo.Owner.Login, pr.PullRequest.Base.Repo.Name) {
		s.wg.Add(1)
		go func(p string, h plugins.PullRequestHandler) {
			defer s.wg.Done()
			agent := plugins.NewAgent(s.ConfigAgent, s.Plugins, s.ClientAgent, l.WithField("plugin", p))
			agent.InitializeCommentPruner(
				pr.Repo.Owner.Login,
				pr.Repo.Name,
				pr.PullRequest.Number,
			)
			if err := h(agent, pr); err != nil {
				agent.Logger.WithError(err).Error("Error handling PullRequestEvent.")
			}
		}(p, h)
	}
	action := genericCommentAction(string(pr.Action))
	if action == "" {
		if !nonCommentPullRequestActions[pr.Action] {
			l.Errorf(failedCommentCoerceFmt, "pull_request", string(pr.Action))
		}
		return
	}
	s.handleGenericComment(
		l,
		&github.GenericCommentEvent{
			GUID:         pr.GUID,
			IsPR:         true,
			Action:       action,
			Body:         pr.PullRequest.Body,
			HTMLURL:      pr.PullRequest.HTMLURL,
			Number:       pr.PullRequest.Number,
			Repo:         pr.Repo,
			User:         pr.PullRequest.User,
			IssueAuthor:  pr.PullRequest.User,
			Assignees:    pr.PullRequest.Assignees,
			IssueState:   pr.PullRequest.State,
			IssueBody:    pr.PullRequest.Body,
			IssueHTMLURL: pr.PullRequest.HTMLURL,
		},
	)
}

func (s *Server) handlePushEvent(l *logrus.Entry, pe github.PushEvent) {
	defer s.wg.Done()
	l = l.WithFields(logrus.Fields{
		github.OrgLogField:  pe.Repo.Owner.Name,
		github.RepoLogField: pe.Repo.Name,
		"ref":               pe.Ref,
		"head":              pe.After,
	})
	l.Info("Push event.")
	for p, h := range s.Plugins.PushEventHandlers(pe.Repo.Owner.Name, pe.Repo.Name) {
		s.wg.Add(1)
		go func(p string, h plugins.PushEventHandler) {
			defer s.wg.Done()
			agent := plugins.NewAgent(s.ConfigAgent, s.Plugins, s.ClientAgent, l.WithField("plugin", p))
			if err := h(agent, pe); err != nil {
				agent.Logger.WithError(err).Error("Error handling PushEvent.")
			}
		}(p, h)
	}
}

func (s *Server) handleIssueEvent(l *logrus.Entry, i github.IssueEvent) {
	defer s.wg.Done()
	l = l.WithFields(logrus.Fields{
		github.OrgLogField:  i.Repo.Owner.Login,
		github.RepoLogField: i.Repo.Name,
		github.PrLogField:   i.Issue.Number,
		"author":            i.Issue.User.Login,
		"url":               i.Issue.HTMLURL,
	})
	l.Infof("Issue %s.", i.Action)
	for p, h := range s.Plugins.IssueHandlers(i.Repo.Owner.Login, i.Repo.Name) {
		s.wg.Add(1)
		go func(p string, h plugins.IssueHandler) {
			defer s.wg.Done()
			agent := plugins.NewAgent(s.ConfigAgent, s.Plugins, s.ClientAgent, l.WithField("plugin", p))
			agent.InitializeCommentPruner(
				i.Repo.Owner.Login,
				i.Repo.Name,
				i.Issue.Number,
			)
			if err := h(agent, i); err != nil {
				agent.Logger.WithError(err).Error("Error handling IssueEvent.")
			}
		}(p, h)
	}
	action := genericCommentAction(string(i.Action))
	if action == "" {
		if !nonCommentIssueActions[i.Action] {
			l.Errorf(failedCommentCoerceFmt, "pull_request", string(i.Action))
		}
		return
	}
	s.handleGenericComment(
		l,
		&github.GenericCommentEvent{
			GUID:         i.GUID,
			IsPR:         i.Issue.IsPullRequest(),
			Action:       action,
			Body:         i.Issue.Body,
			HTMLURL:      i.Issue.HTMLURL,
			Number:       i.Issue.Number,
			Repo:         i.Repo,
			User:         i.Issue.User,
			IssueAuthor:  i.Issue.User,
			Assignees:    i.Issue.Assignees,
			IssueState:   i.Issue.State,
			IssueBody:    i.Issue.Body,
			IssueHTMLURL: i.Issue.HTMLURL,
		},
	)
}

func (s *Server) handleIssueCommentEvent(l *logrus.Entry, ic github.IssueCommentEvent) {
	defer s.wg.Done()
	l = l.WithFields(logrus.Fields{
		github.OrgLogField:  ic.Repo.Owner.Login,
		github.RepoLogField: ic.Repo.Name,
		github.PrLogField:   ic.Issue.Number,
		"author":            ic.Comment.User.Login,
		"url":               ic.Comment.HTMLURL,
	})
	l.Infof("Issue comment %s.", ic.Action)
	for p, h := range s.Plugins.IssueCommentHandlers(ic.Repo.Owner.Login, ic.Repo.Name) {
		s.wg.Add(1)
		go func(p string, h plugins.IssueCommentHandler) {
			defer s.wg.Done()
			agent := plugins.NewAgent(s.ConfigAgent, s.Plugins, s.ClientAgent, l.WithField("plugin", p))
			agent.InitializeCommentPruner(
				ic.Repo.Owner.Login,
				ic.Repo.Name,
				ic.Issue.Number,
			)
			if err := h(agent, ic); err != nil {
				agent.Logger.WithError(err).Error("Error handling IssueCommentEvent.")
			}
		}(p, h)
	}
	action := genericCommentAction(string(ic.Action))
	if action == "" {
		l.Errorf(failedCommentCoerceFmt, "issue_comment", string(ic.Action))
		return
	}
	s.handleGenericComment(
		l,
		&github.GenericCommentEvent{
			GUID:         ic.GUID,
			IsPR:         ic.Issue.IsPullRequest(),
			Action:       action,
			Body:         ic.Comment.Body,
			HTMLURL:      ic.Comment.HTMLURL,
			Number:       ic.Issue.Number,
			Repo:         ic.Repo,
			User:         ic.Comment.User,
			IssueAuthor:  ic.Issue.User,
			Assignees:    ic.Issue.Assignees,
			IssueState:   ic.Issue.State,
			IssueBody:    ic.Issue.Body,
			IssueHTMLURL: ic.Issue.HTMLURL,
		},
	)
}

func (s *Server) handleStatusEvent(l *logrus.Entry, se github.StatusEvent) {
	defer s.wg.Done()
	l = l.WithFields(logrus.Fields{
		github.OrgLogField:  se.Repo.Owner.Login,
		github.RepoLogField: se.Repo.Name,
		"context":           se.Context,
		"sha":               se.SHA,
		"state":             se.State,
		"id":                se.ID,
	})
	l.Infof("Status description %s.", se.Description)
	for p, h := range s.Plugins.StatusEventHandlers(se.Repo.Owner.Login, se.Repo.Name) {
		s.wg.Add(1)
		go func(p string, h plugins.StatusEventHandler) {
			defer s.wg.Done()
			agent := plugins.NewAgent(s.ConfigAgent, s.Plugins, s.ClientAgent, l.WithField("plugin", p))
			if err := h(agent, se); err != nil {
				agent.Logger.WithError(err).Error("Error handling StatusEvent.")
			}
		}(p, h)
	}
}

// genericCommentAction normalizes the action string to a GenericCommentEventAction or returns ""
// if the action is unrelated to the comment text. (For example a PR 'label' action.)
func genericCommentAction(action string) github.GenericCommentEventAction {
	switch action {
	case "created", "opened", "submitted":
		return github.GenericCommentActionCreated
	case "edited":
		return github.GenericCommentActionEdited
	case "deleted", "dismissed":
		return github.GenericCommentActionDeleted
	}
	// The action is not related to the text body.
	return ""
}

func (s *Server) handleGenericComment(l *logrus.Entry, ce *github.GenericCommentEvent) {
	for p, h := range s.Plugins.GenericCommentHandlers(ce.Repo.Owner.Login, ce.Repo.Name) {
		s.wg.Add(1)
		go func(p string, h plugins.GenericCommentHandler) {
			defer s.wg.Done()
			agent := plugins.NewAgent(s.ConfigAgent, s.Plugins, s.ClientAgent, l.WithField("plugin", p))
			agent.InitializeCommentPruner(
				ce.Repo.Owner.Login,
				ce.Repo.Name,
				ce.Number,
			)
			if err := h(agent, *ce); err != nil {
				agent.Logger.WithError(err).Error("Error handling GenericCommentEvent.")
			}
		}(p, h)
	}
}
