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

func (h *Handler) handleChannelUnarchive(body []byte) ([]byte, error) {
	unarchiveEvent := struct {
		Event struct {
			Channel string `json:"channel"`
			User    string `json:"user"`
		} `json:"event"`
	}{}
	if err := json.Unmarshal(body, &unarchiveEvent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json: %v", err)
	}

	h.sendMessage("Channel <#%s> was *unarchived* by <@%s>", unarchiveEvent.Event.Channel, unarchiveEvent.Event.User)
	return nil, nil
}

func (h *Handler) handleChannelRename(body []byte) ([]byte, error) {
	renameEvent := struct {
		Event struct {
			Channel struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Created int    `json:"created"`
			} `json:"channel"`
		} `json:"event"`
	}{}
	if err := json.Unmarshal(body, &renameEvent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json: %v", err)
	}

	channel := renameEvent.Event.Channel

	h.sendMessage("Channel <#%s> was *renamed* to %q", channel.ID, slack.EscapeMessage(channel.Name))
	return nil, nil
}

func (h *Handler) handleChannelDeleted(body []byte) ([]byte, error) {
	unarchiveEvent := struct {
		Event struct {
			Channel string `json:"channel"`
			User    string `json:"user"`
		} `json:"event"`
	}{}
	if err := json.Unmarshal(body, &unarchiveEvent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json: %v", err)
	}

	h.sendMessage("Channel <#%s> was *deleted*", unarchiveEvent.Event.Channel)
	return nil, nil
}

func (h *Handler) handleChannelCreated(body []byte) ([]byte, error) {
	createEvent := struct {
		Event struct {
			Channel struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Created int    `json:"created"`
				Creator string `json:"creator"`
			} `json:"channel"`
		} `json:"event"`
	}{}
	if err := json.Unmarshal(body, &createEvent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json: %v", err)
	}

	channel := createEvent.Event.Channel

	h.sendMessage("Channel <#%s|%s> was *created* by <@%s>", channel.ID, channel.Name, channel.Creator)
	return nil, nil
}

func (h *Handler) handleChannelArchive(body []byte) ([]byte, error) {
	archiveEvent := struct {
		Event struct {
			Channel string `json:"channel"`
			User    string `json:"user"`
		} `json:"event"`
	}{}
	if err := json.Unmarshal(body, &archiveEvent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json: %v", err)
	}

	h.sendMessage("Channel <#%s> was *archived* by <@%s>", archiveEvent.Event.Channel, archiveEvent.Event.User)
	return nil, nil
}
