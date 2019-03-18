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

func (h *Handler) handleURLVerification(body []byte) ([]byte, error) {
	request := struct {
		Challenge string `json:"challenge"`
	}{}
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, fmt.Errorf("error parsing request: %v", err)
	}
	response := map[string]string{"challenge": request.Challenge}
	return json.Marshal(response)
}
