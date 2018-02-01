/*
Copyright 2018 The Kubernetes Authors.

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

package jenkins

import (
	"fmt"
	"io/ioutil"
	"sync"
	"time"

	"github.com/ghodss/yaml"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
)

// JenkinsOperator is config for the jenkins-operator controller.
type JenkinsOperator struct {
	config.Controller `json:",inline"`
}

// Config is a read-only snapshot of the config.
type Config struct {
	config.Config   `json:",inline"`
	JenkinsOperator JenkinsOperator `json:"jenkins_operator,omitempty"`
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
	if err := config.ParseConfig(&nc.Config); err != nil {
		return nil, err
	}
	if err := parseConfig(nc); err != nil {
		return nil, err
	}
	return nc, nil
}

func parseConfig(c *Config) error {
	if err := config.ValidateController(&c.JenkinsOperator.Controller); err != nil {
		return fmt.Errorf("validating jenkins-operator config: %v", err)
	}
	return nil
}

// Agent watches a path and automatically loads the config stored
// therein.
type Agent struct {
	sync.Mutex
	c *Config
}

// Start will begin polling the config file at the path. If the first load
// fails, Start with return the error and abort. Future load failures will log
// the failure message but continue attempting to load.
func (ca *Agent) Start(path string) error {
	c, err := Load(path)
	if err != nil {
		return err
	}
	ca.c = c
	go func() {
		for range time.Tick(1 * time.Minute) {
			if c, err := Load(path); err != nil {
				logrus.WithField("path", path).WithError(err).Error("Error loading config.")
			} else {
				ca.Lock()
				ca.c = c
				ca.Unlock()
			}
		}
	}()
	return nil
}

// Config returns the latest config. Do not modify the config.
func (ca *Agent) Config() *Config {
	ca.Lock()
	defer ca.Unlock()
	return ca.c
}
