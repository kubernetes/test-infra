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
	"strings"
)

func (h *Handler) handleEmojiChanged(body []byte) ([]byte, error) {
	emojiEvent := struct {
		Event struct {
			Subtype string   `json:"subtype"`
			Name    string   `json:"name,omitempty"`
			Names   []string `json:"names,omitempty"`
			Value   string   `json:"value,omitempty"`
		} `json:"event"`
	}{}
	if err := json.Unmarshal(body, &emojiEvent); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json: %v", err)
	}
	emoji := emojiEvent.Event
	if emoji.Subtype == "add" {
		if strings.HasPrefix(emoji.Value, "alias:") {
			h.sendMessage("A *new emoji alias was added*: `:%s:`. It's an alias for `:%s:`. :%s:", emoji.Name, strings.TrimPrefix(emoji.Value, "alias:"), emoji.Name)
		} else {
			h.sendMessage("A *new emoji was added*: `:%s:` :%s:", emoji.Name, emoji.Name)
		}
	} else if emoji.Subtype == "remove" {
		if len(emoji.Names) == 1 {
			h.sendMessage("An *emoji was deleted*: `:%s:`", emoji.Names[0])
		} else {
			var aliases []string
			for _, a := range emoji.Names {
				aliases = append(aliases, "`:"+a+":`")
			}
			h.sendMessage("An *emoji was deleted*. It had several names: %s", strings.Join(aliases, ", "))
		}
	}
	return nil, nil
}
