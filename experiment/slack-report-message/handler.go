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
	"log"
	"net/http"

	"k8s.io/test-infra/experiment/slack-event-log/slack"
)

type handler struct {
	slack *slack.Slack
}

func (h *handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		log.Printf("Failed to parse incoming content: %v", err)
		return
	}
	content := r.Form.Get("payload")
	if content == "" {
		log.Printf("Payload was blank.")
		return
	}
	interaction := slackInteraction{}
	if err := json.Unmarshal([]byte(content), &interaction); err != nil {
		log.Printf("Failed to unmarshal payload: %v", err)
		return
	}
}

type slackInteraction struct {
	Token       string `json:"token"`
	CallbackID  string `json:"callback_id"`
	Type        string `json:"type"`
	TriggerID   string `json:"trigger_id"`
	ResponseURL string `json:"response_url"`
	Team        struct {
		ID     string `json:"id"`
		Domain string `json:"string"`
	} `json:"team"`
	Channel struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"channel"`
	User struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"user"`
	Message struct {
		Type      string `json:"type"`
		User      string `json:"user"`
		Timestamp string `json:"ts"`
		Text      string `json:"text"`
	}
}
