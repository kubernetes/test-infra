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

package slack

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
)

// Config is the information needed to communicate with Slack.
type Config struct {
	SigningSecret string `json:"signingSecret"`
	WebhookURL    string `json:"webhook"`
	AccessToken   string `json:"accessToken"`
}

// LoadConfig loads a Config from a JSON file.
func LoadConfig(path string) (Config, error) {
	config := Config{}
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return config, fmt.Errorf("couldn't open file: %v", err)
	}
	if err := json.Unmarshal(content, &config); err != nil {
		return config, fmt.Errorf("couldn't parse config: %v", err)
	}
	return config, nil
}
