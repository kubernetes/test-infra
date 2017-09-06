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

package config

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

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
