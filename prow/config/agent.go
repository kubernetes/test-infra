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
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Delta represents the before and after states of a Config change detected by the Agent.
type Delta struct {
	Before, After Config
}

// DeltaChan is a channel to receive config delta events when config changes.
type DeltaChan = chan<- Delta

// Agent watches a path and automatically loads the config stored
// therein.
type Agent struct {
	mut           sync.RWMutex // do not export Lock, etc methods
	c             *Config
	subscriptions []DeltaChan
}

// Start will begin polling the config file at the path. If the first load
// fails, Start will return the error and abort. Future load failures will log
// the failure message but continue attempting to load.
func (ca *Agent) Start(prowConfig, jobConfig string) error {
	c, err := Load(prowConfig, jobConfig)
	if err != nil {
		return err
	}
	ca.Set(c)
	go func() {
		var lastModTime time.Time
		// Rarely, if two changes happen in the same second, mtime will
		// be the same for the second change, and an mtime-based check would
		// fail. Reload periodically just in case.
		skips := 0
		for range time.Tick(1 * time.Second) {
			if skips < 600 {
				// Check if the file changed to see if it needs to be re-read.
				// os.Stat follows symbolic links, which is how ConfigMaps work.
				prowStat, err := os.Stat(prowConfig)
				if err != nil {
					logrus.WithField("prowConfig", prowConfig).WithError(err).Error("Error loading prow config.")
					continue
				}

				recentModTime := prowStat.ModTime()

				// TODO(krzyzacy): allow empty jobConfig till fully migrate config to subdirs
				if jobConfig != "" {
					jobConfigStat, err := os.Stat(jobConfig)
					if err != nil {
						logrus.WithField("jobConfig", jobConfig).WithError(err).Error("Error loading job configs.")
						continue
					}

					if jobConfigStat.ModTime().After(recentModTime) {
						recentModTime = jobConfigStat.ModTime()
					}
				}

				if !recentModTime.After(lastModTime) {
					skips++
					continue // file hasn't been modified
				}
				lastModTime = recentModTime
			}
			if c, err := Load(prowConfig, jobConfig); err != nil {
				logrus.WithField("prowConfig", prowConfig).
					WithField("jobConfig", jobConfig).
					WithError(err).Error("Error loading config.")
			} else {
				skips = 0
				ca.Set(c)
			}
		}
	}()
	return nil
}

// Subscribe registers the channel for messages on config reload.
// The caller can expect a copy of the previous and current config
// to be sent down the subscribed channel when a new configuration
// is loaded.
func (ca *Agent) Subscribe(subscription DeltaChan) {
	ca.mut.Lock()
	defer ca.mut.Unlock()
	ca.subscriptions = append(ca.subscriptions, subscription)
}

// Getter returns the current Config in a thread-safe manner.
type Getter func() *Config

// Config returns the latest config. Do not modify the config.
func (ca *Agent) Config() *Config {
	ca.mut.RLock()
	defer ca.mut.RUnlock()
	return ca.c
}

// Set sets the config. Useful for testing.
func (ca *Agent) Set(c *Config) {
	ca.mut.Lock()
	defer ca.mut.Unlock()
	var oldConfig Config
	if ca.c != nil {
		oldConfig = *ca.c
	}
	delta := Delta{oldConfig, *c}
	ca.c = c
	for _, subscription := range ca.subscriptions {
		go func(sub DeltaChan) { // wait a minute to send each event
			end := time.NewTimer(time.Minute)
			select {
			case sub <- delta:
			case <-end.C:
			}
			if !end.Stop() { // prevent new events
				<-end.C // drain the pending event
			}
		}(subscription)
	}
}
