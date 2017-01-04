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

package main

import (
	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
)

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
		pc := s.Plugins.PluginClient
		pc.Logger = l.WithField("plugin", p)
		if err := h(pc, pr); err != nil {
			pc.Logger.WithError(err).Error("Error handling PullRequestEvent.")
		}
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
		pc := s.Plugins.PluginClient
		pc.Logger = l.WithField("plugin", p)
		if err := h(pc, pe); err != nil {
			pc.Logger.WithError(err).Error("Error handling PushEvent.")
		}
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
		pc := s.Plugins.PluginClient
		pc.Logger = l.WithField("plugin", p)
		if err := h(pc, ic); err != nil {
			pc.Logger.WithError(err).Error("Error handling IssueCommentEvent.")
		}
	}
}

func (s *Server) handleStatusEvent(se github.StatusEvent) {
	l := logrus.WithFields(logrus.Fields{
		"org":     se.Repo.Owner.Login,
		"repo":    se.Repo.Name,
		"context": se.Context,
		"sha":     se.Context,
		"state":   se.State,
		"id":      se.ID,
	})
	l.Infof("Status description %s.", se.Description)
	for p, h := range s.Plugins.StatusEventHandlers(se.Repo.Owner.Login, se.Repo.Name) {
		pc := s.Plugins.PluginClient
		pc.Logger = l.WithField("plugin", p)
		if err := h(pc, se); err != nil {
			pc.Logger.WithError(err).Error("Error handling StatusEvent.")
		}
	}
}
