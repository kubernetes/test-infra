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

func (h *Handler) handleSubteamUpdated(body []byte) ([]byte, error) {
	moveEvent := struct {
		Event struct {
			Subteam slack.Subteam `json:"subteam"`
		} `json:"event"`
	}{}
	if err := json.Unmarshal(body, &moveEvent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json: %v", err)
	}

	subteam := moveEvent.Event.Subteam

	if !subteam.IsUsergroup {
		return nil, nil
	}

	if subteam.DeleteTime != 0 {
		h.sendMessage("Usergroup %s (%q) was *deleted* by <@%s>", subteam.Handle, slack.EscapeMessage(subteam.Name), subteam.DeletedBy)
		return nil, nil
	}

	// This group (@test-infra-oncall) is "modified" hourly and is usually an uninteresting noop,
	// just filter it all.
	// TODO(Katharine): make this configurable (or maintain enough state to know this is a noop)
	if subteam.ID == "SGLF0GUQH" {
		return nil, nil
	}

	h.sendMessage("Usergroup <!subteam^%s|%s> (%q) was *updated* by <@%s>", subteam.ID, subteam.Handle, slack.EscapeMessage(subteam.Name), subteam.UpdatedBy)
	return nil, nil
}

func (h *Handler) handleSubteamCreated(body []byte) ([]byte, error) {
	moveEvent := struct {
		Event struct {
			Subteam slack.Subteam `json:"subteam"`
		} `json:"event"`
	}{}
	if err := json.Unmarshal(body, &moveEvent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json: %v", err)
	}

	subteam := moveEvent.Event.Subteam

	if !subteam.IsUsergroup {
		return nil, nil
	}

	h.sendMessage("Usergroup <!subteam^%s|%s> (%q) was *created* by <@%s>", subteam.ID, subteam.Handle, slack.EscapeMessage(subteam.Name), subteam.CreatedBy)
	return nil, nil
}
