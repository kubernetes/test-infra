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

// a wrapper package of cron.Cron, which manages schedule cron jobs for horologium
package cron

import (
	"fmt"
	"sync"

	"github.com/robfig/cron"

	"k8s.io/test-infra/prow/config"
)

// Cron is a wrapper for cron.Cron
type Cron struct {
	cronAgent *cron.Cron
	jobs      map[string]cron.EntryID
	trigger   map[string]bool
	lock      sync.Mutex
}

// NewClient makes a new Cron object
func NewClient() *Cron {
	return &Cron{
		cronAgent: cron.New(),
		jobs:      map[string]cron.EntryID{},
		trigger:   map[string]bool{},
	}
}

// Start kicks off current cronAgent scheduler
func (c *Cron) Start() {
	c.cronAgent.Start()
}

// Stop pauses current cronAgent scheduler
func (c *Cron) Stop() {
	c.cronAgent.Stop()
}

// DumpQueuedJobs returns job need to be triggered
func (c *Cron) QueuedJobs() map[string]bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	res := map[string]bool{}
	for k, v := range c.trigger {
		res[k] = v
	}
	c.trigger = map[string]bool{}
	return res
}

// SyncConfig syncs current cronAgent with current prow config
// which add/delete jobs accordingly.
func (c *Cron) SyncConfig(cfg *config.Config) error {
	for _, p := range cfg.Periodics {
		if err := c.addPeriodic(p); err != nil {
			return err
		}
	}

	return nil
}

func (c *Cron) addPeriodic(p config.Periodic) error {
	if p.Cron == "" {
		return nil
	}

	if _, ok := c.jobs[p.Name]; ok {
		return nil
	}

	if err := c.addJob(p.Name, p.Cron); err != nil {
		return err
	}

	for _, ras := range p.RunAfterSuccess {
		if err := c.addPeriodic(ras); err != nil {
			return err
		}
	}

	return nil
}

// addJob adds a cron entry for a job to cronAgent
func (c *Cron) addJob(name, cron string) error {
	if id, err := c.cronAgent.AddFunc(cron, func() {
		c.lock.Lock()
		defer c.lock.Unlock()
		// We want to ignore second trigger if first trigger is not consumed yet.
		c.trigger[name] = true
	}); err != nil {
		return fmt.Errorf("cronAgent fails to add job %s with cron %s: %v", name, cron, err)
	} else {
		c.jobs[name] = id
	}

	return nil
}

// removeJob removes the job from cronAgent
func (c *Cron) removeJob(name string) error {
	id, ok := c.jobs[name]
	if !ok {
		return fmt.Errorf("job %s has not been added to cronAgent yet", name)
	}
	c.cronAgent.Remove(id)
	return nil
}
