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
	"encoding/json"
	"fmt"
	"github.com/Sirupsen/logrus"
	"io/ioutil"
	"net/http"

	"k8s.io/test-infra/prow/github"
)

// Server implements http.Handler. It validates incoming GitHub webhooks and
// then places them into the appropriate event channels.
type Server struct {
	PullRequestEvents  chan<- github.PullRequestEvent
	IssueCommentEvents chan<- github.IssueCommentEvent
	StatusEvents       chan<- github.StatusEvent

	HMACSecret []byte
}

// ServeHTTP validates an incoming webhook and puts it into the event channel.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	// Header checks: It must be a POST with an event type and a signature.
	if r.Method != http.MethodPost {
		http.Error(w, "405 Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		http.Error(w, "400 Bad Request: Missing X-GitHub-Event Header", http.StatusBadRequest)
		return
	}
	sig := r.Header.Get("X-Hub-Signature")
	if sig == "" {
		http.Error(w, "403 Forbidden: Missing X-Hub-Signature", http.StatusForbidden)
		return
	}

	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "500 Internal Server Error: Failed to read request body", http.StatusInternalServerError)
		return
	}

	// Validate the payload with our HMAC secret.
	if !github.ValidatePayload(payload, sig, s.HMACSecret) {
		http.Error(w, "403 Forbidden: Invalid X-Hub-Signature", http.StatusForbidden)
		return
	}
	fmt.Fprint(w, "Event received. Have a nice day.")

	if err := s.demuxEvent(eventType, payload); err != nil {
		logrus.WithError(err).Error("Error parsing event.")
	}
}

func (s *Server) demuxEvent(eventType string, payload []byte) error {
	switch eventType {
	case "pull_request":
		var pr github.PullRequestEvent
		if err := json.Unmarshal(payload, &pr); err != nil {
			return err
		}
		go func() {
			s.PullRequestEvents <- pr
		}()
	case "issue_comment":
		var ic github.IssueCommentEvent
		if err := json.Unmarshal(payload, &ic); err != nil {
			return err
		}
		go func() {
			s.IssueCommentEvents <- ic
		}()
	case "status":
		var se github.StatusEvent
		if err := json.Unmarshal(payload, &se); err != nil {
			return err
		}
		go func() {
			s.StatusEvents <- se
		}()
	}

	return nil
}
