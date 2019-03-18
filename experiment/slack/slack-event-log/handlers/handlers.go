/*
Copyright 2019 The Kubernetes Authors.

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

// Package handlers contains handlers for all Slack events.
package handlers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"k8s.io/test-infra/experiment/slack/slack"
)

type handlerFunc func(body []byte) ([]byte, error)

// Handler handles Slack events.
type Handler struct {
	client *slack.Client
}

// New returns a new Handler.
func New(client *slack.Client) *Handler {
	return &Handler{client: client}
}

// HandleWebhook can be passed to http.HandlerFunc and will perform all processing associated with
// Slack webhooks.
func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf(fmt.Sprintf("failed to read body: %v", err))
		http.Error(w, fmt.Sprintf("failed to read body: %v", err), 500)
		return
	}
	log.Printf("%#v", r.Header)
	log.Printf(string(body))
	if err := h.client.VerifySignature(body, r.Header); err != nil {
		log.Printf(fmt.Sprintf("signature verification failed: %v", err))
		http.Error(w, fmt.Sprintf("signature verification failed: %v", err), 403)
		return
	}

	response, err := h.HandleMessage(body)

	if err != nil {
		log.Printf("Handling message failed: %v", err)
	}

	if response != nil {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(response)
	}
}

// HandleMessage handles a Slack webhook that has already been validated.
func (h *Handler) HandleMessage(body []byte) ([]byte, error) {
	t := struct {
		Type string `json:"type"`
	}{}
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, err
	}

	messageMapping := map[string]handlerFunc{
		"url_verification": h.handleURLVerification,
		"event_callback":   h.handleEvent,
	}

	fn, ok := messageMapping[t.Type]
	if !ok {
		return nil, fmt.Errorf("unknown event type %q", t.Type)
	}
	output, err := fn(body)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", t.Type, err)
	}
	return output, nil
}

func (h *Handler) sendMessage(message string, args ...interface{}) {
	s := fmt.Sprintf(message, args...)
	log.Printf("Sending message: %q", s)
	if err := h.client.SendMessage(s); err != nil {
		log.Printf("Sending message failed: %v", err)
	}
}

func (h *Handler) handleEvent(body []byte) ([]byte, error) {
	event := struct {
		Event struct {
			Type string `json:"type"`
		} `json:"event"`
	}{}
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("error unmarshalling event type: %v", err)
	}
	t := event.Event.Type

	eventMapping := map[string]handlerFunc{
		"emoji_changed":      h.handleEmojiChanged,
		"team_join":          h.handleTeamJoin,
		"user_change":        h.handleUserChange,
		"team_rename":        h.handleTeamRename,
		"team_domain_change": h.handleTeamDomainChange,
		"subteam_updated":    h.handleSubteamUpdated,
		"subteam_created":    h.handleSubteamCreated,
		"channel_unarchive":  h.handleChannelUnarchive,
		"channel_rename":     h.handleChannelRename,
		"channel_deleted":    h.handleChannelDeleted,
		"channel_created":    h.handleChannelCreated,
		"channel_archive":    h.handleChannelArchive,
	}

	fn, ok := eventMapping[t]
	if !ok {
		return nil, fmt.Errorf("unknown event type %q", t)
	}
	response, err := fn(body)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", t, err)
	}
	return response, nil
}
