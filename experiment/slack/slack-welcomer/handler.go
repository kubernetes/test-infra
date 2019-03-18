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

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"k8s.io/test-infra/experiment/slack/slack"
	"log"
	"net/http"
)

type handler struct {
	client      *slack.Client
	messagePath string
}

func logError(rw http.ResponseWriter, format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...)
	log.Println(s)
	http.Error(rw, s, 500)
}

type handlerFunc func(body []byte) ([]byte, error)

// ServeHTTP handles Slack webhook requests.
func (h *handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logError(rw, "Failed to read incoming request body: %v", err)
		return
	}
	if err := h.client.VerifySignature(body, r.Header); err != nil {
		logError(rw, "Failed validation: %v", err)
		return
	}
	response, err := h.handleMessage(body)
	if err != nil {
		logError(rw, "Failed to handle message: %v", err)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	_, _ = rw.Write(response)
}

func (h *handler) handleMessage(body []byte) ([]byte, error) {
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

func (h *handler) handleURLVerification(body []byte) ([]byte, error) {
	request := struct {
		Challenge string `json:"challenge"`
	}{}
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, fmt.Errorf("error parsing request: %v", err)
	}
	response := map[string]string{"challenge": request.Challenge}
	return json.Marshal(response)
}

func (h *handler) handleEvent(body []byte) ([]byte, error) {
	event := struct {
		Event struct {
			Type string     `json:"type"`
			User slack.User `json:"user"`
		} `json:"event"`
	}{}
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %v", err)
	}

	// We should only be getting team_join events, but be sure to filter out anything else.
	// We don't consider this an error, because Slack might get upset if we did.
	if event.Event.Type != "team_join" {
		return []byte{}, nil
	}

	if err := h.sendWelcome(event.Event.User.ID); err != nil {
		return nil, fmt.Errorf("failed to send welcome: %v", err)
	}
	return []byte{}, nil
}

func (h *handler) sendWelcome(uid string) error {
	welcome, err := h.getWelcome()
	if err != nil {
		return fmt.Errorf("couldn't get welcome: %v", err)
	}

	// Slack requires that we first open an "IM channel" that we can then use to actually send messages.
	response := struct {
		Channel struct {
			ID string `json:"id"`
		} `json:"channel"`
	}{}
	if err := h.client.CallMethod("im.open", map[string]string{"user": uid}, &response); err != nil {
		return fmt.Errorf("couldn't open IM channel: %v", err)
	}
	channel := response.Channel.ID

	message := struct {
		Channel   string `json:"channel"`
		Text      string `json:"text"`
		AsUser    bool   `json:"as_user"`
		LinkNames bool   `json:"link_names"`
	}{
		Channel:   channel,
		Text:      welcome,
		AsUser:    true, // Send messages as the bot user, rather than as the app (a very subtle distinction)
		LinkNames: true, // Parse @names and #names in the welcome message but still allow other fancy formatting.
	}
	if err := h.client.CallMethod("chat.postMessage", message, nil); err != nil {
		return fmt.Errorf("failed to send message: %v", err)
	}
	return nil
}

func (h *handler) getWelcome() (string, error) {
	message, err := ioutil.ReadFile(h.messagePath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %v", h.messagePath, err)
	}
	return string(message), nil
}
