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

package githubeventserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	pluginhelp_externalplugins "k8s.io/test-infra/prow/pluginhelp/externalplugins"
	pluginhelp_hook "k8s.io/test-infra/prow/pluginhelp/hook"
	"k8s.io/test-infra/prow/plugins"
)

const (
	eventTypeField = "event-type"

	statusEvent                   = "status"
	pushEvent                     = "push"
	pullRequestReviewCommentEvent = "pull_request_review_comment"
	pullRequestReviewEvent        = "pull_request_review"
	pullRequestEvent              = "pull_request"
	issueCommentEvent             = "issue_comment"
	issuesEvent                   = "issues"
)

// GitHubEventServer hold all the information needed for the
// github event server implementation.
type GitHubEventServer struct {
	wg *sync.WaitGroup

	serveMuxHandler *serveMuxHandler
	httpServeMux    *http.ServeMux

	httpServer *http.Server
}

// New creates a new GitHubEventServer from the given arguments.
// It also assigns the serveMuxHandler in the http.ServeMux.
func New(o Options, hmacTokenGenerator func() []byte, logger *logrus.Entry) *GitHubEventServer {
	var wg sync.WaitGroup

	githubEventServer := &GitHubEventServer{
		wg: &wg,
		serveMuxHandler: &serveMuxHandler{
			hmacTokenGenerator: hmacTokenGenerator,
			log:                logger,
			metrics:            o.Metrics,
			wg:                 &wg,
		},
	}

	httpServeMux := http.NewServeMux()
	httpServeMux.Handle(o.endpoint, githubEventServer.serveMuxHandler)

	githubEventServer.httpServeMux = httpServeMux
	githubEventServer.httpServer = &http.Server{Addr: ":" + strconv.Itoa(o.port), Handler: httpServeMux}

	return githubEventServer
}

// GetServeMuxHandler return the github's event server serveMuxHandler.
func (g *GitHubEventServer) GetServeMuxHandler() *serveMuxHandler {
	return g.serveMuxHandler
}

// ListenAndServe runs the http server
func (g *GitHubEventServer) ListenAndServe() error {
	return g.httpServer.ListenAndServe()
}

// Shutdown shutdowns the http server
func (g *GitHubEventServer) Shutdown(ctx context.Context) error {
	return g.httpServer.Shutdown(ctx)
}

// ReviewCommentEventHandler is a type of function that handles GitHub's review comment events
type ReviewCommentEventHandler func(*logrus.Entry, github.ReviewCommentEvent)

// ReviewEventHandler is a type of function that handles GitHub's review events.
type ReviewEventHandler func(*logrus.Entry, github.ReviewEvent)

// PushEventHandler is a type of function that handles GitHub's push events.
type PushEventHandler func(*logrus.Entry, github.PushEvent)

// IssueCommentEventHandler is a type of function that handles GitHub's issue comment events.
type IssueCommentEventHandler func(*logrus.Entry, github.IssueCommentEvent)

// PullRequestHandler is a type of function that handles GitHub's pull request events.
type PullRequestHandler func(*logrus.Entry, github.PullRequestEvent)

// IssueEventHandler is a type of function that handles GitHub's issue events.
type IssueEventHandler func(*logrus.Entry, github.IssueEvent)

// StatusEventHandler is a type of function that handles GitHub's status events.
type StatusEventHandler func(*logrus.Entry, github.StatusEvent)

// RegisterReviewCommentEventHandler registers an ReviewCommentEventHandler function in GitHubEventServerOptions
func (g *GitHubEventServer) RegisterReviewCommentEventHandler(fn ReviewCommentEventHandler) {
	g.serveMuxHandler.reviewCommentEventHandlers = append(g.serveMuxHandler.reviewCommentEventHandlers, fn)
}

// RegisterReviewEventHandler registers an ReviewEventHandler function in GitHubEventServerOptions
func (g *GitHubEventServer) RegisterReviewEventHandler(fn ReviewEventHandler) {
	g.serveMuxHandler.reviewEventHandlers = append(g.serveMuxHandler.reviewEventHandlers, fn)
}

// RegisterPushEventHandler registers an PushEventHandler function in GitHubEventServerOptions
func (g *GitHubEventServer) RegisterPushEventHandler(fn PushEventHandler) {
	g.serveMuxHandler.pushEventHandlers = append(g.serveMuxHandler.pushEventHandlers, fn)
}

// RegisterHandleIssueCommentEvent registers an IssueCommentEventHandler function in GitHubEventServerOptions
func (g *GitHubEventServer) RegisterHandleIssueCommentEvent(fn IssueCommentEventHandler) {
	g.serveMuxHandler.issueCommentEventHandlers = append(g.serveMuxHandler.issueCommentEventHandlers, fn)
}

// RegisterHandlePullRequestEvent registers an PullRequestHandler function in GitHubEventServerOptions
func (g *GitHubEventServer) RegisterHandlePullRequestEvent(fn PullRequestHandler) {
	g.serveMuxHandler.pullRequestHandlers = append(g.serveMuxHandler.pullRequestHandlers, fn)
}

// RegisterIssueEventHandler registers an IssueEventHandler function in GitHubEventServerOptions
func (g *GitHubEventServer) RegisterIssueEventHandler(fn IssueEventHandler) {
	g.serveMuxHandler.issueEventHandlers = append(g.serveMuxHandler.issueEventHandlers, fn)
}

// RegisterStatusEventHandler registers an StatusEventHandler function in GitHubEventServerOptions
func (g *GitHubEventServer) RegisterStatusEventHandler(fn StatusEventHandler) {
	g.serveMuxHandler.statusEventHandlers = append(g.serveMuxHandler.statusEventHandlers, fn)
}

// RegisterExternalPlugins registers the external plugins in GitHubEventServerOptions
func (g *GitHubEventServer) RegisterExternalPlugins(p map[string][]plugins.ExternalPlugin) {
	g.serveMuxHandler.externalPlugins = p
}

// RegisterHelpProvider registers a help provider function in GitHubEventServerOptions http.ServeMux
func (g *GitHubEventServer) RegisterHelpProvider(helpProvider func([]config.OrgRepo) (*pluginhelp.PluginHelp, error), log *logrus.Entry) {
	pluginhelp_externalplugins.ServeExternalPluginHelp(g.httpServeMux, log, helpProvider)
}

// RegisterPluginHelpAgentHandle registers a help agent in with the given endpoint in the GitHubEventServerOptions http.ServeMux
func (g *GitHubEventServer) RegisterPluginHelpAgentHandle(endpoint string, helpAgent *pluginhelp_hook.HelpAgent) {
	g.httpServeMux.Handle(endpoint, helpAgent)
}

// RegisterCustomFuncHandle registers a custom func(w http.ResponseWriter, r *http.Request)
// with the given endpoint in the GitHubEventServerOptions http.ServeMux
func (g *GitHubEventServer) RegisterCustomFuncHandle(endpoint string, fn func(w http.ResponseWriter, r *http.Request)) {
	g.httpServeMux.HandleFunc(endpoint, fn)
}

// GracefulShutdown handles all requests sent before receiving the shutdown signal.
func (g *GitHubEventServer) GracefulShutdown() {
	logrus.Info("Waiting for the remaining requests")
	g.wg.Wait()
}

// serveMuxHandler is a http serveMux handler that implements the ServeHTTP method.
// see https://godoc.org/net/http#ServeMux
type serveMuxHandler struct {
	log *logrus.Entry
	wg  *sync.WaitGroup

	reviewCommentEventHandlers []ReviewCommentEventHandler
	reviewEventHandlers        []ReviewEventHandler
	pullRequestHandlers        []PullRequestHandler
	pushEventHandlers          []PushEventHandler
	issueCommentEventHandlers  []IssueCommentEventHandler
	issueEventHandlers         []IssueEventHandler
	statusEventHandlers        []StatusEventHandler

	externalPlugins map[string][]plugins.ExternalPlugin

	hmacTokenGenerator func() []byte
	metrics            *Metrics

	c http.Client
}

func (s *serveMuxHandler) handleEvent(eventType, eventGUID string, payload []byte, h http.Header) error {
	var org string
	var repo string

	l := logrus.WithFields(logrus.Fields{eventTypeField: eventType, github.EventGUID: eventGUID})

	// We don't want to fail the webhook due to a metrics error.
	if counter, err := s.metrics.WebhookCounter.GetMetricWithLabelValues(eventType); err != nil {
		l.WithError(err).Warn("Failed to get metric for eventType " + eventType)
	} else {
		counter.Inc()
	}

	switch eventType {
	case issuesEvent:
		var i github.IssueEvent
		if err := json.Unmarshal(payload, &i); err != nil {
			return err
		}
		i.GUID = eventGUID
		org = i.Repo.Owner.Login
		repo = i.Repo.Name

		for _, issueEventHandler := range s.issueEventHandlers {
			fn := issueEventHandler
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				fn(l.WithFields(logrus.Fields{
					github.OrgLogField:  i.Repo.Owner.Login,
					github.RepoLogField: i.Repo.Name,
					github.PrLogField:   i.Issue.Number,
					"author":            i.Issue.User.Login,
					"url":               i.Issue.HTMLURL,
				}), i)
			}()
		}

	case issueCommentEvent:
		var ic github.IssueCommentEvent
		if err := json.Unmarshal(payload, &ic); err != nil {
			return err
		}
		ic.GUID = eventGUID
		org = ic.Repo.Owner.Login
		repo = ic.Repo.Name

		for _, issueCommentEventHandler := range s.issueCommentEventHandlers {
			fn := issueCommentEventHandler
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				fn(l.WithFields(logrus.Fields{
					github.OrgLogField:  ic.Repo.Owner.Login,
					github.RepoLogField: ic.Repo.Name,
					github.PrLogField:   ic.Issue.Number,
					"author":            ic.Comment.User.Login,
					"url":               ic.Comment.HTMLURL,
				}), ic)
			}()
		}

	case pullRequestEvent:
		var pr github.PullRequestEvent
		if err := json.Unmarshal(payload, &pr); err != nil {
			return err
		}
		pr.GUID = eventGUID
		org = pr.Repo.Owner.Login
		repo = pr.Repo.Name

		for _, pullRequestHandler := range s.pullRequestHandlers {
			fn := pullRequestHandler
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				fn(l.WithFields(logrus.Fields{
					github.OrgLogField:  pr.Repo.Owner.Login,
					github.RepoLogField: pr.Repo.Name,
					github.PrLogField:   pr.Number,
					"author":            pr.PullRequest.User.Login,
					"url":               pr.PullRequest.HTMLURL,
				}), pr)
			}()
		}

	case pullRequestReviewEvent:
		var re github.ReviewEvent
		if err := json.Unmarshal(payload, &re); err != nil {
			return err
		}
		re.GUID = eventGUID
		org = re.Repo.Owner.Login
		repo = re.Repo.Name

		for _, reviewEventHandler := range s.reviewEventHandlers {
			fn := reviewEventHandler
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				fn(l.WithFields(logrus.Fields{
					github.OrgLogField:  re.Repo.Owner.Login,
					github.RepoLogField: re.Repo.Name,
					github.PrLogField:   re.PullRequest.Number,
					"review":            re.Review.ID,
					"reviewer":          re.Review.User.Login,
					"url":               re.Review.HTMLURL,
				}), re)
			}()
		}

	case pullRequestReviewCommentEvent:
		var rce github.ReviewCommentEvent
		if err := json.Unmarshal(payload, &rce); err != nil {
			return err
		}
		rce.GUID = eventGUID
		org = rce.Repo.Owner.Login
		repo = rce.Repo.Name

		for _, reviewCommentEventHandler := range s.reviewCommentEventHandlers {
			fn := reviewCommentEventHandler
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				fn(l.WithFields(logrus.Fields{
					github.OrgLogField:  rce.Repo.Owner.Login,
					github.RepoLogField: rce.Repo.Name,
					github.PrLogField:   rce.PullRequest.Number,
					"review":            rce.Comment.ReviewID,
					"commenter":         rce.Comment.User.Login,
					"url":               rce.Comment.HTMLURL,
				}), rce)
			}()
		}

	case pushEvent:
		var pe github.PushEvent
		if err := json.Unmarshal(payload, &pe); err != nil {
			return err
		}
		pe.GUID = eventGUID
		org = pe.Repo.Owner.Login
		repo = pe.Repo.Name

		for _, pushEventHandler := range s.pushEventHandlers {
			fn := pushEventHandler
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				fn(l.WithFields(logrus.Fields{
					github.OrgLogField:  pe.Repo.Owner.Name,
					github.RepoLogField: pe.Repo.Name,
					"ref":               pe.Ref,
					"head":              pe.After,
				}), pe)
			}()
		}

	case statusEvent:
		var se github.StatusEvent
		if err := json.Unmarshal(payload, &se); err != nil {
			return err
		}
		se.GUID = eventGUID
		org = se.Repo.Owner.Login
		repo = se.Repo.Name

		for _, statusEventHandler := range s.statusEventHandlers {
			fn := statusEventHandler
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				fn(l.WithFields(logrus.Fields{
					github.OrgLogField:  se.Repo.Owner.Login,
					github.RepoLogField: se.Repo.Name,
					"context":           se.Context,
					"sha":               se.SHA,
					"state":             se.State,
					"id":                se.ID,
				}), se)
			}()
		}

	default:
		l.Debug("Ignoring unhandled event type.")
	}

	// Redirect event to external plugins if necessary
	s.demuxExternal(l, s.getExternalPluginsForEvent(org, repo, eventType), payload, h, s.wg)

	return nil
}

// ServeHTTP validates an incoming webhook and puts it into the event channel.
func (s *serveMuxHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	eventType, eventGUID, payload, ok, resp := github.ValidateWebhook(w, r, s.hmacTokenGenerator)
	if counter, err := s.metrics.ResponseCounter.GetMetricWithLabelValues(strconv.Itoa(resp)); err != nil {
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

	if err := s.handleEvent(eventType, eventGUID, payload, r.Header); err != nil {
		logrus.WithError(err).Error("Error parsing event.")
	}
}

// demuxExternal dispatches the provided payload to the external plugins.
func (s *serveMuxHandler) demuxExternal(l *logrus.Entry, externalPlugins []plugins.ExternalPlugin, payload []byte, h http.Header, wg *sync.WaitGroup) {
	h.Set("User-Agent", "ProwHook")
	for _, p := range externalPlugins {
		wg.Add(1)
		go func(p plugins.ExternalPlugin) {
			defer wg.Done()
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
func (s *serveMuxHandler) dispatch(endpoint string, payload []byte, h http.Header) error {
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

func (s *serveMuxHandler) do(req *http.Request) (*http.Response, error) {
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

func (s *serveMuxHandler) getExternalPluginsForEvent(org, repo, event string) []plugins.ExternalPlugin {
	pluginsByEvent := make(map[string][]plugins.ExternalPlugin)

	var external []plugins.ExternalPlugin

	fullRepo := fmt.Sprintf("%s/%s", org, repo)
	external = append(external, s.externalPlugins[org]...)
	external = append(external, s.externalPlugins[fullRepo]...)

	for _, ep := range external {
		for _, event := range ep.Events {
			pluginsByEvent[event] = append(pluginsByEvent[event], ep)
		}
	}

	if externalPlugins, ok := pluginsByEvent[event]; ok {
		return externalPlugins
	}
	return []plugins.ExternalPlugin{}
}
