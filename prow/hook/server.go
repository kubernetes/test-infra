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
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/githubeventserver"
	_ "k8s.io/test-infra/prow/hook/plugin-imports"
	"k8s.io/test-infra/prow/plugins"
)

const (
	failedCommentCoerceFmt = "Could not coerce %s event to a GenericCommentEvent. Unknown 'action': %q."
	eventTypeField         = "event-type"
)

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
		github.IssueActionPinned:       true,
		github.IssueActionUnpinned:     true,
		github.IssueActionTransferred:  true,
		github.IssueActionDeleted:      true,
		github.IssueActionLocked:       true,
		github.IssueActionUnlocked:     true,
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
		github.PullRequestActionReadyForReview:       true,
		github.PullRequestActionConvertedToDraft:     true,
		github.PullRequestActionLocked:               true,
		github.PullRequestActionUnlocked:             true,
	}
)

// Server implements http.Handler. It validates incoming GitHub webhooks and
// then dispatches them to the appropriate plugins.
type Server struct {
	ClientAgent *plugins.ClientAgent
	Plugins     *plugins.ConfigAgent
	ConfigAgent *config.Agent
	Metrics     *githubeventserver.Metrics
	RepoEnabled func(org, repo string) bool

	// c is an http client used for dispatching events
	// to external plugin services.
	c http.Client
	// Tracks running handlers for graceful shutdown
	wg sync.WaitGroup
}

func (s *Server) HandleReviewEvent(l *logrus.Entry, re github.ReviewEvent) {
	if !s.RepoEnabled(re.Repo.Owner.Login, re.Repo.Name) {
		return
	}

	l.Infof("Review %s.", re.Action)

	for p, h := range s.Plugins.ReviewEventHandlers(re.PullRequest.Base.Repo.Owner.Login, re.PullRequest.Base.Repo.Name) {
		s.wg.Add(1)
		go func(p string, h plugins.ReviewEventHandler) {
			defer s.wg.Done()
			agent := plugins.NewAgent(s.ConfigAgent, s.Plugins, s.ClientAgent, s.Metrics.Metrics, l, p)
			agent.InitializeCommentPruner(
				re.Repo.Owner.Login,
				re.Repo.Name,
				re.PullRequest.Number,
			)
			start := time.Now()
			labels := prometheus.Labels{"event_type": l.Data[eventTypeField].(string), "action": string(re.Action), "plugin": p}
			if err := h(agent, re); err != nil {
				agent.Logger.WithError(err).Error("Error handling ReviewEvent.")
				s.Metrics.PluginHandleErrors.With(labels).Inc()
			}
			s.Metrics.PluginHandleDuration.With(labels).Observe(time.Since(start).Seconds())
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
			IssueTitle:   re.PullRequest.Title,
			IssueBody:    re.PullRequest.Body,
			IssueHTMLURL: re.PullRequest.HTMLURL,
		},
	)
}

func (s *Server) HandleReviewCommentEvent(l *logrus.Entry, rce github.ReviewCommentEvent) {
	if !s.RepoEnabled(rce.Repo.Owner.Login, rce.Repo.Name) {
		return
	}
	l.Infof("Review comment %s.", rce.Action)

	for p, h := range s.Plugins.ReviewCommentEventHandlers(rce.PullRequest.Base.Repo.Owner.Login, rce.PullRequest.Base.Repo.Name) {
		s.wg.Add(1)
		go func(p string, h plugins.ReviewCommentEventHandler) {
			defer s.wg.Done()
			agent := plugins.NewAgent(s.ConfigAgent, s.Plugins, s.ClientAgent, s.Metrics.Metrics, l, p)
			agent.InitializeCommentPruner(
				rce.Repo.Owner.Login,
				rce.Repo.Name,
				rce.PullRequest.Number,
			)
			start := time.Now()
			labels := prometheus.Labels{"event_type": l.Data[eventTypeField].(string), "action": string(rce.Action), "plugin": p}
			if err := h(agent, rce); err != nil {
				agent.Logger.WithError(err).Error("Error handling ReviewCommentEvent.")
				s.Metrics.PluginHandleErrors.With(labels).Inc()
			}
			s.Metrics.PluginHandleDuration.With(labels).Observe(time.Since(start).Seconds())
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
			CommentID:    intPtr(rce.Comment.ID),
			Action:       action,
			Body:         rce.Comment.Body,
			HTMLURL:      rce.Comment.HTMLURL,
			Number:       rce.PullRequest.Number,
			Repo:         rce.Repo,
			User:         rce.Comment.User,
			IssueAuthor:  rce.PullRequest.User,
			Assignees:    rce.PullRequest.Assignees,
			IssueState:   rce.PullRequest.State,
			IssueTitle:   rce.PullRequest.Title,
			IssueBody:    rce.PullRequest.Body,
			IssueHTMLURL: rce.PullRequest.HTMLURL,
		},
	)
}

func (s *Server) HandlePullRequestEvent(l *logrus.Entry, pr github.PullRequestEvent) {
	if !s.RepoEnabled(pr.Repo.Owner.Login, pr.Repo.Name) {
		return
	}
	l.Infof("Pull request %s.", pr.Action)

	for p, h := range s.Plugins.PullRequestHandlers(pr.PullRequest.Base.Repo.Owner.Login, pr.PullRequest.Base.Repo.Name) {
		s.wg.Add(1)
		go func(p string, h plugins.PullRequestHandler) {
			defer s.wg.Done()
			agent := plugins.NewAgent(s.ConfigAgent, s.Plugins, s.ClientAgent, s.Metrics.Metrics, l, p)
			agent.InitializeCommentPruner(
				pr.Repo.Owner.Login,
				pr.Repo.Name,
				pr.PullRequest.Number,
			)
			start := time.Now()
			labels := prometheus.Labels{"event_type": l.Data[eventTypeField].(string), "action": string(pr.Action), "plugin": p}
			if err := h(agent, pr); err != nil {
				agent.Logger.WithError(err).Error("Error handling PullRequestEvent.")
				s.Metrics.PluginHandleErrors.With(labels).Inc()
			}
			s.Metrics.PluginHandleDuration.With(labels).Observe(time.Since(start).Seconds())
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
			ID:           pr.PullRequest.ID,
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
			IssueTitle:   pr.PullRequest.Title,
			IssueBody:    pr.PullRequest.Body,
			IssueHTMLURL: pr.PullRequest.HTMLURL,
		},
	)
}

func (s *Server) HandlePushEvent(l *logrus.Entry, pe github.PushEvent) {
	if !s.RepoEnabled(pe.Repo.Owner.Login, pe.Repo.Name) {
		return
	}
	l.Info("Push event.")

	for p, h := range s.Plugins.PushEventHandlers(pe.Repo.Owner.Name, pe.Repo.Name) {
		s.wg.Add(1)
		go func(p string, h plugins.PushEventHandler) {
			defer s.wg.Done()
			agent := plugins.NewAgent(s.ConfigAgent, s.Plugins, s.ClientAgent, s.Metrics.Metrics, l, p)
			start := time.Now()
			labels := prometheus.Labels{"event_type": l.Data[eventTypeField].(string), "action": "none", "plugin": p}
			if err := h(agent, pe); err != nil {
				agent.Logger.WithError(err).Error("Error handling PushEvent.")
				s.Metrics.PluginHandleErrors.With(labels).Inc()
			}
			s.Metrics.PluginHandleDuration.With(labels).Observe(time.Since(start).Seconds())
		}(p, h)
	}
}

func (s *Server) HandleIssueEvent(l *logrus.Entry, i github.IssueEvent) {
	if !s.RepoEnabled(i.Repo.Owner.Login, i.Repo.Name) {
		return
	}
	l.Infof("Issue %s.", i.Action)

	for p, h := range s.Plugins.IssueHandlers(i.Repo.Owner.Login, i.Repo.Name) {
		s.wg.Add(1)
		go func(p string, h plugins.IssueHandler) {
			defer s.wg.Done()
			agent := plugins.NewAgent(s.ConfigAgent, s.Plugins, s.ClientAgent, s.Metrics.Metrics, l, p)
			agent.InitializeCommentPruner(
				i.Repo.Owner.Login,
				i.Repo.Name,
				i.Issue.Number,
			)
			start := time.Now()
			labels := prometheus.Labels{"event_type": l.Data[eventTypeField].(string), "action": string(i.Action), "plugin": p}
			if err := h(agent, i); err != nil {
				agent.Logger.WithError(err).Error("Error handling IssueEvent.")
				s.Metrics.PluginHandleErrors.With(labels).Inc()
			}
			s.Metrics.PluginHandleDuration.With(labels).Observe(time.Since(start).Seconds())
		}(p, h)
	}
	action := genericCommentAction(string(i.Action))
	if action == "" {
		if !nonCommentIssueActions[i.Action] {
			l.Errorf(failedCommentCoerceFmt, "issues", string(i.Action))
		}
		return
	}
	s.handleGenericComment(
		l,
		&github.GenericCommentEvent{
			ID:           i.Issue.ID,
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
			IssueTitle:   i.Issue.Title,
			IssueBody:    i.Issue.Body,
			IssueHTMLURL: i.Issue.HTMLURL,
		},
	)
}

func (s *Server) HandleIssueCommentEvent(l *logrus.Entry, ic github.IssueCommentEvent) {
	if !s.RepoEnabled(ic.Repo.Owner.Login, ic.Repo.Name) {
		return
	}
	l.Infof("Issue comment %s.", ic.Action)

	for p, h := range s.Plugins.IssueCommentHandlers(ic.Repo.Owner.Login, ic.Repo.Name) {
		s.wg.Add(1)
		go func(p string, h plugins.IssueCommentHandler) {
			defer s.wg.Done()
			agent := plugins.NewAgent(s.ConfigAgent, s.Plugins, s.ClientAgent, s.Metrics.Metrics, l, p)
			agent.InitializeCommentPruner(
				ic.Repo.Owner.Login,
				ic.Repo.Name,
				ic.Issue.Number,
			)
			start := time.Now()
			labels := prometheus.Labels{"event_type": l.Data[eventTypeField].(string), "action": string(ic.Action), "plugin": p}
			if err := h(agent, ic); err != nil {
				agent.Logger.WithError(err).Error("Error handling IssueCommentEvent.")
				s.Metrics.PluginHandleErrors.With(labels).Inc()
			}
			s.Metrics.PluginHandleDuration.With(labels).Observe(time.Since(start).Seconds())
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
			ID:           ic.Issue.ID,
			CommentID:    intPtr(ic.Comment.ID),
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
			IssueTitle:   ic.Issue.Title,
			IssueBody:    ic.Issue.Body,
			IssueHTMLURL: ic.Issue.HTMLURL,
		},
	)
}

func (s *Server) HandleStatusEvent(l *logrus.Entry, se github.StatusEvent) {
	if !s.RepoEnabled(se.Repo.Owner.Login, se.Repo.Name) {
		return
	}
	l.Infof("Status description %s.", se.Description)

	for p, h := range s.Plugins.StatusEventHandlers(se.Repo.Owner.Login, se.Repo.Name) {
		s.wg.Add(1)
		go func(p string, h plugins.StatusEventHandler) {
			defer s.wg.Done()
			agent := plugins.NewAgent(s.ConfigAgent, s.Plugins, s.ClientAgent, s.Metrics.Metrics, l, p)
			start := time.Now()
			labels := prometheus.Labels{"event_type": l.Data[eventTypeField].(string), "action": "none", "plugin": p}
			if err := h(agent, se); err != nil {
				agent.Logger.WithError(err).Error("Error handling StatusEvent.")
				s.Metrics.PluginHandleErrors.With(labels).Inc()
			}
			s.Metrics.PluginHandleDuration.With(labels).Observe(time.Since(start).Seconds())
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
			agent := plugins.NewAgent(s.ConfigAgent, s.Plugins, s.ClientAgent, s.Metrics.Metrics, l, p)
			agent.InitializeCommentPruner(
				ce.Repo.Owner.Login,
				ce.Repo.Name,
				ce.Number,
			)
			start := time.Now()
			labels := prometheus.Labels{"event_type": l.Data[eventTypeField].(string), "action": string(ce.Action), "plugin": p}
			if err := h(agent, *ce); err != nil {
				agent.Logger.WithError(err).Error("Error handling GenericCommentEvent.")
				s.Metrics.PluginHandleErrors.With(labels).Inc()
			}
			s.Metrics.PluginHandleDuration.With(labels).Observe(time.Since(start).Seconds())
		}(p, h)
	}
}

func intPtr(i int) *int {
	return &i
}
