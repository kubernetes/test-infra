/*
Copyright 2017 The Kubernetes Authors.

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

// Package config knows how to read and parse config.yaml.
package config

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"time"

	"github.com/ghodss/yaml"
)

// Config is a read-only snapshot of the config.
type Config struct {
	Presubmits  map[string][]Presubmit  `json:"presubmits,omitempty"`
	Postsubmits map[string][]Postsubmit `json:"postsubmits,omitempty"`
	Periodics   map[string][]Periodic   `json:"periodics,omitempty"`
}

// Load loads and parses the config at path.
func Load(path string) (*Config, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %v", path, err)
	}
	nc := &Config{}
	if err := yaml.Unmarshal(b, nc); err != nil {
		return nil, fmt.Errorf("error unmarshaling %s: %v", path, err)
	}
	if err := parseConfig(nc); err != nil {
		return nil, err
	}
	return nc, nil
}

func parseConfig(c *Config) error {
	// Ensure that presubmit regexes are valid.
	for _, v := range c.Presubmits {
		if err := setRegexes(v); err != nil {
			return fmt.Errorf("could not set regex: %v", err)
		}
	}
	// Ensure that the periodic durations are valid.
	for _, js := range c.Periodics {
		for j := range js {
			d, err := time.ParseDuration(js[j].Interval)
			if err != nil {
				return fmt.Errorf("cannot parse duration for %s: %v", js[j].Name, err)
			}
			js[j].interval = d
		}
	}
	return nil
}

func setRegexes(js []Presubmit) error {
	for i, j := range js {
		if re, err := regexp.Compile(j.Trigger); err == nil {
			js[i].re = re
		} else {
			return fmt.Errorf("could not compile trigger regex for %s: %v", j.Name, err)
		}
		if err := setRegexes(j.RunAfterSuccess); err != nil {
			return err
		}
		if j.RunIfChanged != "" {
			if re, err := regexp.Compile(j.RunIfChanged); err != nil {
				return fmt.Errorf("could not compile changes regex for %s: %v", j.Name, err)
			} else {
				js[i].reChanges = re
			}
		}
	}
	return nil
}
