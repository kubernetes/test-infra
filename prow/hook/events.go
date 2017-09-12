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

func (s *Server) handleReviewEvent(re github.ReviewEvent) {
	l := logrus.WithFields(logrus.Fields{
		"org":      re.PullRequest.Base.Repo.Owner.Login,
		"repo":     re.PullRequest.Base.Repo.Name,
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
			if err := h(pc, re); err != nil {
				pc.Logger.WithError(err).Error("Error handling ReviewEvent.")
			}
		}(p, h)
	}
}

func (s *Server) handleReviewCommentEvent(rce github.ReviewCommentEvent) {
	l := logrus.WithFields(logrus.Fields{
		"org":       rce.PullRequest.Base.Repo.Owner.Login,
		"repo":      rce.PullRequest.Base.Repo.Name,
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
			if err := h(pc, rce); err != nil {
				pc.Logger.WithError(err).Error("Error handling ReviewCommentEvent.")
			}
		}(p, h)
	}
}

func (s *Server) handlePullRequestEvent(pr github.PullRequestEvent) {
	l := logrus.WithFields(logrus.Fields{
		"org":    pr.PullRequest.Base.Repo.Owner.Login,
		"repo":   pr.PullRequest.Base.Repo.Name,
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
			if err := h(pc, pr); err != nil {
				pc.Logger.WithError(err).Error("Error handling PullRequestEvent.")
			}
		}(p, h)
	}
}

func (s *Server) handlePushEvent(pe github.PushEvent) {
	l := logrus.WithFields(logrus.Fields{
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

func (s *Server) handleIssueEvent(i github.IssueEvent) {
	l := logrus.WithFields(logrus.Fields{
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
			if err := h(pc, i); err != nil {
				pc.Logger.WithError(err).Error("Error handleing IssueEvent.")
			}
		}(p, h)
	}
}

func (s *Server) handleIssueCommentEvent(ic github.IssueCommentEvent) {
	l := logrus.WithFields(logrus.Fields{
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
			if err := h(pc, ic); err != nil {
				pc.Logger.WithError(err).Error("Error handling IssueCommentEvent.")
			}
		}(p, h)
	}
}

func (s *Server) handleStatusEvent(se github.StatusEvent) {
	l := logrus.WithFields(logrus.Fields{
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
