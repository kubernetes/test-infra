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

	"k8s.io/test-infra/prow/commentpruner"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

func (s *Server) handleReviewEvent(l *logrus.Entry, re github.ReviewEvent) {
	l = l.WithFields(logrus.Fields{
		"org":      re.Repo.Owner.Login,
		"repo":     re.Repo.Name,
		"pr":       re.PullRequest.Number,
		"review":   re.Review.ID,
		"reviewer": re.Review.User.Login,
		"url":      re.Review.HTMLURL,
	})
	l.Infof("Review %s.", re.Action)
	for p, h := range s.Plugins.ReviewEventHandlers(re.PullRequest.Base.Repo.Owner.Login, re.PullRequest.Base.Repo.Name) {
		go func(p string, h plugins.ReviewEventHandler) {
			pc := s.Plugins.PluginClient
			pc.Logger = l.WithField("plugin", p)
			pc.Config = s.ConfigAgent.Config()
			pc.PluginConfig = s.Plugins.Config()
			pc.CommentPruner = commentpruner.NewEventClient(
				pc.GitHubClient,
				l.WithField("client", "commentpruner"),
				re.Repo.Owner.Login,
				re.Repo.Name,
				re.PullRequest.Number,
			)
			if err := h(pc, re); err != nil {
				pc.Logger.WithError(err).Error("Error handling ReviewEvent.")
			}
		}(p, h)
	}
	action := genericCommentAction(string(re.Action))
	if action == "" {
		return
	}
	s.handleGenericComment(
		l,
		&github.GenericCommentEvent{
			IsPR:        true,
			Action:      action,
			Body:        re.Review.Body,
			HTMLURL:     re.Review.HTMLURL,
			Number:      re.PullRequest.Number,
			Repo:        re.Repo,
			User:        re.Review.User,
			IssueAuthor: re.PullRequest.User,
			Assignees:   re.PullRequest.Assignees,
			IssueState:  re.PullRequest.State,
		},
	)
}

func (s *Server) handleReviewCommentEvent(l *logrus.Entry, rce github.ReviewCommentEvent) {
	l = l.WithFields(logrus.Fields{
		"org":       rce.Repo.Owner.Login,
		"repo":      rce.Repo.Name,
		"pr":        rce.PullRequest.Number,
		"review":    rce.Comment.ReviewID,
		"commenter": rce.Comment.User.Login,
		"url":       rce.Comment.HTMLURL,
	})
	l.Infof("Review comment %s.", rce.Action)
	for p, h := range s.Plugins.ReviewCommentEventHandlers(rce.PullRequest.Base.Repo.Owner.Login, rce.PullRequest.Base.Repo.Name) {
		go func(p string, h plugins.ReviewCommentEventHandler) {
			pc := s.Plugins.PluginClient
			pc.Logger = l.WithField("plugin", p)
			pc.Config = s.ConfigAgent.Config()
			pc.PluginConfig = s.Plugins.Config()
			pc.CommentPruner = commentpruner.NewEventClient(
				pc.GitHubClient,
				l.WithField("client", "commentpruner"),
				rce.Repo.Owner.Login,
				rce.Repo.Name,
				rce.PullRequest.Number,
			)
			if err := h(pc, rce); err != nil {
				pc.Logger.WithError(err).Error("Error handling ReviewCommentEvent.")
			}
		}(p, h)
	}
	action := genericCommentAction(string(rce.Action))
	if action == "" {
		return
	}
	s.handleGenericComment(
		l,
		&github.GenericCommentEvent{
			IsPR:        true,
			Action:      action,
			Body:        rce.Comment.Body,
			HTMLURL:     rce.Comment.HTMLURL,
			Number:      rce.PullRequest.Number,
			Repo:        rce.Repo,
			User:        rce.Comment.User,
			IssueAuthor: rce.PullRequest.User,
			Assignees:   rce.PullRequest.Assignees,
			IssueState:  rce.PullRequest.State,
		},
	)
}

func (s *Server) handlePullRequestEvent(l *logrus.Entry, pr github.PullRequestEvent) {
	l = l.WithFields(logrus.Fields{
		"org":    pr.Repo.Owner.Login,
		"repo":   pr.Repo.Name,
		"pr":     pr.Number,
		"author": pr.PullRequest.User.Login,
		"url":    pr.PullRequest.HTMLURL,
	})
	l.Infof("Pull request %s.", pr.Action)
	for p, h := range s.Plugins.PullRequestHandlers(pr.PullRequest.Base.Repo.Owner.Login, pr.PullRequest.Base.Repo.Name) {
		go func(p string, h plugins.PullRequestHandler) {
			pc := s.Plugins.PluginClient
			pc.Logger = l.WithField("plugin", p)
			pc.Config = s.ConfigAgent.Config()
			pc.PluginConfig = s.Plugins.Config()
			pc.CommentPruner = commentpruner.NewEventClient(
				pc.GitHubClient,
				l.WithField("client", "commentpruner"),
				pr.Repo.Owner.Login,
				pr.Repo.Name,
				pr.PullRequest.Number,
			)
			if err := h(pc, pr); err != nil {
				pc.Logger.WithError(err).Error("Error handling PullRequestEvent.")
			}
		}(p, h)
	}
	action := genericCommentAction(string(pr.Action))
	if action == "" {
		return
	}
	s.handleGenericComment(
		l,
		&github.GenericCommentEvent{
			IsPR:        true,
			Action:      action,
			Body:        pr.PullRequest.Body,
			HTMLURL:     pr.PullRequest.HTMLURL,
			Number:      pr.PullRequest.Number,
			Repo:        pr.Repo,
			User:        pr.PullRequest.User,
			IssueAuthor: pr.PullRequest.User,
			Assignees:   pr.PullRequest.Assignees,
			IssueState:  pr.PullRequest.State,
		},
	)
}

func (s *Server) handlePushEvent(l *logrus.Entry, pe github.PushEvent) {
	l = l.WithFields(logrus.Fields{
		"org":  pe.Repo.Owner.Name,
		"repo": pe.Repo.Name,
		"ref":  pe.Ref,
		"head": pe.After,
	})
	l.Info("Push event.")
	for p, h := range s.Plugins.PushEventHandlers(pe.Repo.Owner.Name, pe.Repo.Name) {
		go func(p string, h plugins.PushEventHandler) {
			pc := s.Plugins.PluginClient
			pc.Logger = l.WithField("plugin", p)
			pc.Config = s.ConfigAgent.Config()
			pc.PluginConfig = s.Plugins.Config()
			if err := h(pc, pe); err != nil {
				pc.Logger.WithError(err).Error("Error handling PushEvent.")
			}
		}(p, h)
	}
}

func (s *Server) handleIssueEvent(l *logrus.Entry, i github.IssueEvent) {
	l = l.WithFields(logrus.Fields{
		"org":    i.Repo.Owner.Login,
		"repo":   i.Repo.Name,
		"pr":     i.Issue.Number,
		"author": i.Issue.User.Login,
		"url":    i.Issue.HTMLURL,
	})
	l.Infof("Issue %s.", i.Action)
	for p, h := range s.Plugins.IssueHandlers(i.Repo.Owner.Login, i.Repo.Name) {
		go func(p string, h plugins.IssueHandler) {
			pc := s.Plugins.PluginClient
			pc.Logger = l.WithField("plugin", p)
			pc.Config = s.ConfigAgent.Config()
			pc.PluginConfig = s.Plugins.Config()
			pc.CommentPruner = commentpruner.NewEventClient(
				pc.GitHubClient,
				l.WithField("client", "commentpruner"),
				i.Repo.Owner.Login,
				i.Repo.Name,
				i.Issue.Number,
			)
			if err := h(pc, i); err != nil {
				pc.Logger.WithError(err).Error("Error handling IssueEvent.")
			}
		}(p, h)
	}
	action := genericCommentAction(string(i.Action))
	if action == "" {
		return
	}
	s.handleGenericComment(
		l,
		&github.GenericCommentEvent{
			IsPR:        i.Issue.IsPullRequest(),
			Action:      action,
			Body:        i.Issue.Body,
			HTMLURL:     i.Issue.HTMLURL,
			Number:      i.Issue.Number,
			Repo:        i.Repo,
			User:        i.Issue.User,
			IssueAuthor: i.Issue.User,
			Assignees:   i.Issue.Assignees,
			IssueState:  i.Issue.State,
		},
	)
}

func (s *Server) handleIssueCommentEvent(l *logrus.Entry, ic github.IssueCommentEvent) {
	l = l.WithFields(logrus.Fields{
		"org":    ic.Repo.Owner.Login,
		"repo":   ic.Repo.Name,
		"pr":     ic.Issue.Number,
		"author": ic.Comment.User.Login,
		"url":    ic.Comment.HTMLURL,
	})
	l.Infof("Issue comment %s.", ic.Action)
	for p, h := range s.Plugins.IssueCommentHandlers(ic.Repo.Owner.Login, ic.Repo.Name) {
		go func(p string, h plugins.IssueCommentHandler) {
			pc := s.Plugins.PluginClient
			pc.Logger = l.WithField("plugin", p)
			pc.Config = s.ConfigAgent.Config()
			pc.PluginConfig = s.Plugins.Config()
			pc.CommentPruner = commentpruner.NewEventClient(
				pc.GitHubClient,
				l.WithField("client", "commentpruner"),
				ic.Repo.Owner.Login,
				ic.Repo.Name,
				ic.Issue.Number,
			)
			if err := h(pc, ic); err != nil {
				pc.Logger.WithError(err).Error("Error handling IssueCommentEvent.")
			}
		}(p, h)
	}
	action := genericCommentAction(string(ic.Action))
	if action == "" {
		return
	}
	s.handleGenericComment(
		l,
		&github.GenericCommentEvent{
			IsPR:        ic.Issue.IsPullRequest(),
			Action:      action,
			Body:        ic.Comment.Body,
			HTMLURL:     ic.Comment.HTMLURL,
			Number:      ic.Issue.Number,
			Repo:        ic.Repo,
			User:        ic.Comment.User,
			IssueAuthor: ic.Issue.User,
			Assignees:   ic.Issue.Assignees,
			IssueState:  ic.Issue.State,
		},
	)
}

func (s *Server) handleStatusEvent(l *logrus.Entry, se github.StatusEvent) {
	l = l.WithFields(logrus.Fields{
		"org":     se.Repo.Owner.Login,
		"repo":    se.Repo.Name,
		"context": se.Context,
		"sha":     se.SHA,
		"state":   se.State,
		"id":      se.ID,
	})
	l.Infof("Status description %s.", se.Description)
	for p, h := range s.Plugins.StatusEventHandlers(se.Repo.Owner.Login, se.Repo.Name) {
		go func(p string, h plugins.StatusEventHandler) {
			pc := s.Plugins.PluginClient
			pc.Logger = l.WithField("plugin", p)
			pc.Config = s.ConfigAgent.Config()
			pc.PluginConfig = s.Plugins.Config()
			if err := h(pc, se); err != nil {
				pc.Logger.WithError(err).Error("Error handling StatusEvent.")
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
		go func(p string, h plugins.GenericCommentHandler) {
			pc := s.Plugins.PluginClient
			pc.Logger = l.WithField("plugin", p)
			pc.Config = s.ConfigAgent.Config()
			pc.PluginConfig = s.Plugins.Config()
			pc.CommentPruner = commentpruner.NewEventClient(
				pc.GitHubClient,
				l.WithField("client", "commentpruner"),
				ce.Repo.Owner.Login,
				ce.Repo.Name,
				ce.Number,
			)
			if err := h(pc, *ce); err != nil {
				pc.Logger.WithError(err).Error("Error handling GenericCommentEvent.")
			}
		}(p, h)
	}
}
