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
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

// Server implements http.Handler. It validates incoming GitHub webhooks and
// then dispatches them to the appropriate plugins.
type Server struct {
	Plugins     *plugins.PluginAgent
	ConfigAgent *config.Agent
	HMACSecret  []byte
	Metrics     *Metrics

	// c is an http client used for dispatching events
	// to external plugin services.
	c http.Client
	// Tracks running handlers for graceful shutdown
	wg sync.WaitGroup
}

// ServeHTTP validates an incoming webhook and puts it into the event channel.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	eventType, eventGUID, payload, ok := ValidateWebhook(w, r, s.HMACSecret)
	if !ok {
		return
	}
	fmt.Fprint(w, "Event received. Have a nice day.")

	if err := s.demuxEvent(eventType, eventGUID, payload, r.Header); err != nil {
		logrus.WithError(err).Error("Error parsing event.")
	}
}

// ValidateWebhook ensures that the provided request conforms to the
// format of a Github webhook and the payload can be validated with
// the provided hmac secret. It returns the event type, the event guid,
// the payload of the request, and whether the webhook is valid or not.
func ValidateWebhook(w http.ResponseWriter, r *http.Request, hmacSecret []byte) (string, string, []byte, bool) {
	defer r.Body.Close()

	// Our health check uses GET, so just kick back a 200.
	if r.Method == http.MethodGet {
		return "", "", nil, false
	}

	// Header checks: It must be a POST with an event type and a signature.
	if r.Method != http.MethodPost {
		resp := "405 Method not allowed"
		logrus.Debug(resp)
		http.Error(w, resp, http.StatusMethodNotAllowed)
		return "", "", nil, false
	}
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		resp := "400 Bad Request: Missing X-GitHub-Event Header"
		logrus.Debug(resp)
		http.Error(w, resp, http.StatusBadRequest)
		return "", "", nil, false
	}
	eventGUID := r.Header.Get("X-GitHub-Delivery")
	if eventGUID == "" {
		resp := "400 Bad Request: Missing X-GitHub-Delivery Header"
		logrus.Debug(resp)
		http.Error(w, resp, http.StatusBadRequest)
		return "", "", nil, false
	}
	sig := r.Header.Get("X-Hub-Signature")
	if sig == "" {
		resp := "403 Forbidden: Missing X-Hub-Signature"
		logrus.Debug(resp)
		http.Error(w, resp, http.StatusForbidden)
		return "", "", nil, false
	}
	contentType := r.Header.Get("content-type")
	if contentType != "application/json" {
		resp := "400 Bad Request: Hook only accepts content-type: application/json - please reconfigure this hook on GitHub"
		logrus.Debug(resp)
		http.Error(w, resp, http.StatusBadRequest)
		return "", "", nil, false
	}

	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		resp := "500 Internal Server Error: Failed to read request body"
		logrus.Debug(resp)
		http.Error(w, resp, http.StatusInternalServerError)
		return "", "", nil, false
	}

	// Validate the payload with our HMAC secret.
	if !github.ValidatePayload(payload, sig, hmacSecret) {
		resp := "403 Forbidden: Invalid X-Hub-Signature"
		logrus.Debug(resp)
		http.Error(w, resp, http.StatusForbidden)
		return "", "", nil, false
	}

	return eventType, eventGUID, payload, true
}

func (s *Server) demuxEvent(eventType, eventGUID string, payload []byte, h http.Header) error {
	l := logrus.WithFields(
		logrus.Fields{
			"event-type":     eventType,
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
		s.wg.Add(1)
		go s.handleIssueEvent(l, i)
	case "issue_comment":
		var ic github.IssueCommentEvent
		if err := json.Unmarshal(payload, &ic); err != nil {
			return err
		}
		ic.GUID = eventGUID
		srcRepo = ic.Repo.FullName
		s.wg.Add(1)
		go s.handleIssueCommentEvent(l, ic)
	case "pull_request":
		var pr github.PullRequestEvent
		if err := json.Unmarshal(payload, &pr); err != nil {
			return err
		}
		pr.GUID = eventGUID
		srcRepo = pr.Repo.FullName
		s.wg.Add(1)
		go s.handlePullRequestEvent(l, pr)
	case "pull_request_review":
		var re github.ReviewEvent
		if err := json.Unmarshal(payload, &re); err != nil {
			return err
		}
		re.GUID = eventGUID
		srcRepo = re.Repo.FullName
		s.wg.Add(1)
		go s.handleReviewEvent(l, re)
	case "pull_request_review_comment":
		var rce github.ReviewCommentEvent
		if err := json.Unmarshal(payload, &rce); err != nil {
			return err
		}
		rce.GUID = eventGUID
		srcRepo = rce.Repo.FullName
		s.wg.Add(1)
		go s.handleReviewCommentEvent(l, rce)
	case "push":
		var pe github.PushEvent
		if err := json.Unmarshal(payload, &pe); err != nil {
			return err
		}
		pe.GUID = eventGUID
		srcRepo = pe.Repo.FullName
		s.wg.Add(1)
		go s.handlePushEvent(l, pe)
	case "status":
		var se github.StatusEvent
		if err := json.Unmarshal(payload, &se); err != nil {
			return err
		}
		se.GUID = eventGUID
		srcRepo = se.Repo.FullName
		s.wg.Add(1)
		go s.handleStatusEvent(l, se)
	}
	// Demux events only to external plugins that require this event.
	if external := s.needDemux(eventType, srcRepo); len(external) > 0 {
		go s.demuxExternal(l, external, payload, h)
	}
	return nil
}

// needDemux returns whether there are any external plugins that need to
// get the present event.
func (s *Server) needDemux(eventType, srcRepo string) []plugins.ExternalPlugin {
	var matching []plugins.ExternalPlugin
	srcOrg := strings.Split(srcRepo, "/")[0]

	for repo, plugins := range s.Plugins.Config().ExternalPlugins {
		// Make sure the repositories match
		var matchesRepo bool
		if repo == srcRepo {
			matchesRepo = true
		}
		// If repo is an org, we need to compare orgs.
		if !matchesRepo && !strings.Contains(repo, "/") && repo == srcOrg {
			matchesRepo = true
		}
		// No need to continue if the repos don't match.
		if !matchesRepo {
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

// Implements a graceful shutdown protool. Handles all requests sent before receiving shutdown signal.
func (s *Server) GracefulShutdown() {
	s.wg.Wait() // Handle remaining requests
	return
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
