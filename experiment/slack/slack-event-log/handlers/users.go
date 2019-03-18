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

package handlers

import (
	"encoding/json"
	"fmt"

	"k8s.io/test-infra/experiment/slack/slack"
)

func (h *Handler) handleTeamJoin(body []byte) ([]byte, error) {
	userEvent := struct {
		Event struct {
			User slack.User `json:"user"`
		} `json:"event"`
	}{}
	if err := json.Unmarshal(body, &userEvent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json: %v", err)
	}

	user := userEvent.Event.User
	displayName := slack.EscapeMessage(user.Profile.DisplayName)
	if displayName == "" {
		displayName = "_none_"
	}
	h.sendMessage(fmt.Sprintf("A *new user joined*: <@%s> (display name: %s, real name: %s)", user.ID, displayName, slack.EscapeMessage(user.Profile.RealName)))
	return nil, nil
}

func (h *Handler) handleUserChange(body []byte) ([]byte, error) {
	userEvent := struct {
		Event struct {
			User slack.User `json:"user"`
		} `json:"event"`
	}{}
	if err := json.Unmarshal(body, &userEvent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json: %v", err)
	}

	user := userEvent.Event.User
	if user.Deleted {
		h.sendMessage("A *user was deactivated*: <@%s> (this is heuristic: they are definitely deactivated now, but may also have been before)", user.ID)
	}
	return nil, nil
}
