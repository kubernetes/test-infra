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
)

func (h *Handler) handleTeamRename(body []byte) ([]byte, error) {
	renameEvent := struct {
		Event struct {
			Name string `json:"name"`
		} `json:"event"`
	}{}
	if err := json.Unmarshal(body, &renameEvent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json: %v", err)
	}
	h.sendMessage("The *Slack team was renamed* to %q", renameEvent.Event.Name)
	return nil, nil
}

func (h *Handler) handleTeamDomainChange(body []byte) ([]byte, error) {
	moveEvent := struct {
		Event struct {
			URL string `json:"url"`
		} `json:"event"`
	}{}
	if err := json.Unmarshal(body, &moveEvent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json: %v", err)
	}
	h.sendMessage("The *Slack team moved* to %s", moveEvent.Event.URL)
	return nil, nil
}
