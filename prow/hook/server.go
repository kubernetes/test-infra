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
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
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

// Server implements http.Handler. It validates incoming GitHub webhooks and
// then dispatches them to the appropriate plugins.
type Server struct {
	ClientAgent    *plugins.ClientAgent
	Plugins        *plugins.ConfigAgent
	ConfigAgent    *config.Agent
	TokenGenerator func() []byte
	Metrics        *githubeventserver.Metrics
	RepoEnabled    func(org, repo string) bool

	// c is an http client used for dispatching events
	// to external plugin services.
	c http.Client
	// Tracks running handlers for graceful shutdown
	wg sync.WaitGroup
}

// ServeHTTP validates an incoming webhook and puts it into the event channel.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	eventType, eventGUID, payload, ok, resp := github.ValidateWebhook(w, r, s.TokenGenerator)
	if counter, err := s.Metrics.ResponseCounter.GetMetricWithLabelValues(strconv.Itoa(resp)); err != nil {
		logrus.WithFields(logrus.Fields{
			"status-code": resp,
		}).WithError(err).Error("Failed to get metric for reporting webhook status code")
	} else {
		counter.Inc()
	}

	if !ok {
		return
	}
	fmt.Fprint(w, "Event received. Have a nice day.")

	if err := s.demuxEvent(eventType, eventGUID, payload, r.Header); err != nil {
		logrus.WithError(err).Error("Error parsing event.")
	}
}

func (s *Server) demuxEvent(eventType, eventGUID string, payload []byte, h http.Header) error {
	l := logrus.WithFields(
		logrus.Fields{
			eventTypeField:   eventType,
			github.EventGUID: eventGUID,
		},
	)
	// We don't want to fail the webhook due to a metrics error.
	if counter, err := s.Metrics.WebhookCounter.GetMetricWithLabelValues(eventType); err != nil {
		l.WithError(err).Warn("Failed to get metric for eventType " + eventType)
	} else {
		counter.Inc()
	}
	var srcRepo string
	switch eventType {
	case "issues":
		var i github.IssueEvent
		if err := json.Unmarshal(payload, &i); err != nil {
			return err
		}
		i.GUID = eventGUID
		srcRepo = i.Repo.FullName
		if s.RepoEnabled(i.Repo.Owner.Login, i.Repo.Name) {
			s.wg.Add(1)
			go s.handleIssueEvent(l, i)
		}
	case "issue_comment":
		var ic github.IssueCommentEvent
		if err := json.Unmarshal(payload, &ic); err != nil {
			return err
		}
		ic.GUID = eventGUID
		srcRepo = ic.Repo.FullName
		if s.RepoEnabled(ic.Repo.Owner.Login, ic.Repo.Name) {
			s.wg.Add(1)
			go s.handleIssueCommentEvent(l, ic)
		}
	case "pull_request":
		var pr github.PullRequestEvent
		if err := json.Unmarshal(payload, &pr); err != nil {
			return err
		}
		pr.GUID = eventGUID
		srcRepo = pr.Repo.FullName
		if s.RepoEnabled(pr.Repo.Owner.Login, pr.Repo.Name) {
			s.wg.Add(1)
			go s.handlePullRequestEvent(l, pr)
		}
	case "pull_request_review":
		var re github.ReviewEvent
		if err := json.Unmarshal(payload, &re); err != nil {
			return err
		}
		re.GUID = eventGUID
		srcRepo = re.Repo.FullName
		if s.RepoEnabled(re.Repo.Owner.Login, re.Repo.Name) {
			s.wg.Add(1)
			go s.handleReviewEvent(l, re)
		}
	case "pull_request_review_comment":
		var rce github.ReviewCommentEvent
		if err := json.Unmarshal(payload, &rce); err != nil {
			return err
		}
		rce.GUID = eventGUID
		srcRepo = rce.Repo.FullName
		if s.RepoEnabled(rce.Repo.Owner.Login, rce.Repo.Name) {
			s.wg.Add(1)
			go s.handleReviewCommentEvent(l, rce)
		}
	case "push":
		var pe github.PushEvent
		if err := json.Unmarshal(payload, &pe); err != nil {
			return err
		}
		pe.GUID = eventGUID
		srcRepo = pe.Repo.FullName
		if s.RepoEnabled(pe.Repo.Owner.Login, pe.Repo.Name) {
			s.wg.Add(1)
			go s.handlePushEvent(l, pe)
		}
	case "status":
		var se github.StatusEvent
		if err := json.Unmarshal(payload, &se); err != nil {
			return err
		}
		se.GUID = eventGUID
		srcRepo = se.Repo.FullName
		if s.RepoEnabled(se.Repo.Owner.Login, se.Repo.Name) {
			s.wg.Add(1)
			go s.handleStatusEvent(l, se)
		}
	default:
		l.Debug("Ignoring unhandled event type. (Might still be handled by external plugins.)")
	}
	// Demux events only to external plugins that require this event.
	if external := s.needDemux(eventType, srcRepo); len(external) > 0 {
		go s.demuxExternal(l, external, payload, h)
	}
	return nil
}

// needDemux returns whether there are any external plugins that need to
// get the present event.
func (s *Server) needDemux(eventType, orgRepo string) []plugins.ExternalPlugin {
	var matching []plugins.ExternalPlugin
	split := strings.Split(orgRepo, "/")
	srcOrg := split[0]
	var srcRepo string
	if len(split) > 1 {
		srcRepo = split[1]
	}
	if !s.RepoEnabled(srcOrg, srcRepo) {
		return nil
	}

	for repo, plugins := range s.Plugins.Config().ExternalPlugins {
		// Make sure the repositories match
		if repo != orgRepo && repo != srcOrg {
			continue
		}

		// Make sure the events match
		for _, p := range plugins {
			if len(p.Events) == 0 {
				matching = append(matching, p)
			} else {
				for _, et := range p.Events {
					if et != eventType {
						continue
					}
					matching = append(matching, p)
					break
				}
			}
		}
	}
	return matching
}

// demuxExternal dispatches the provided payload to the external plugins.
func (s *Server) demuxExternal(l *logrus.Entry, externalPlugins []plugins.ExternalPlugin, payload []byte, h http.Header) {
	h.Set("User-Agent", "ProwHook")
	for _, p := range externalPlugins {
		s.wg.Add(1)
		go func(p plugins.ExternalPlugin) {
			defer s.wg.Done()
			if err := s.dispatch(p.Endpoint, payload, h); err != nil {
				l.WithError(err).WithField("external-plugin", p.Name).Error("Error dispatching event to external plugin.")
			} else {
				l.WithField("external-plugin", p.Name).Info("Dispatched event to external plugin")
			}
		}(p)
	}
}

// dispatch creates a new request using the provided payload and headers
// and dispatches the request to the provided endpoint.
func (s *Server) dispatch(endpoint string, payload []byte, h http.Header) error {
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	req.Header = h
	resp, err := s.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rb, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("response has status %q and body %q", resp.Status, string(rb))
	}
	return nil
}

// GracefulShutdown implements a graceful shutdown protocol. It handles all requests sent before
// receiving the shutdown signal.
func (s *Server) GracefulShutdown() {
	s.wg.Wait() // Handle remaining requests
}

func (s *Server) do(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error
	backoff := 100 * time.Millisecond
	maxRetries := 5

	for retries := 0; retries < maxRetries; retries++ {
		resp, err = s.c.Do(req)
		if err == nil {
			break
		}
		time.Sleep(backoff)
		backoff *= 2
	}
	return resp, err
}

const failedCommentCoerceFmt = "Could not coerce %s event to a GenericCommentEvent. Unknown 'action': %q."

const eventTypeField = "event-type"

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
