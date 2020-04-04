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

// Package cron provides a wrapper of robfig/cron, which manages schedule cron jobs for horologium
package cron

import (
	"fmt"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	cron "gopkg.in/robfig/cron.v2" // using v2 api, doc at https://godoc.org/gopkg.in/robfig/cron.v2
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/config"
)

// jobStatus is a cache layer for tracking existing cron jobs
type jobStatus struct {
	// entryID is a unique-identifier for each cron entry generated from cronAgent
	entryID cron.EntryID
	// triggered marks if a job has been triggered for the next cron.QueuedJobs() call
	triggered bool
	// cronStr is a cache for job's cron status
	// cron entry will be regenerated if cron string changes from the periodic job
	cronStr string
}

// Cron is a wrapper for cron.Cron
type Cron struct {
	cronAgent *cron.Cron
	jobs      map[string]*jobStatus
	logger    *logrus.Entry
	lock      sync.Mutex
}

// New makes a new Cron object
func New() *Cron {
	return &Cron{
		cronAgent: cron.New(),
		jobs:      map[string]*jobStatus{},
		logger:    logrus.WithField("client", "cron"),
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

// QueuedJobs returns a list of jobs that need to be triggered
// and reset trigger in jobStatus
func (c *Cron) QueuedJobs() []string {
	c.lock.Lock()
	defer c.lock.Unlock()

	res := []string{}
	for k, v := range c.jobs {
		if v.triggered {
			res = append(res, k)
		}
		c.jobs[k].triggered = false
	}
	return res
}

// SyncConfig syncs current cronAgent with current prow config
// which add/delete jobs accordingly.
func (c *Cron) SyncConfig(cfg *config.Config) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	for _, p := range cfg.Periodics {
		if err := c.addPeriodic(p); err != nil {
			return err
		}
	}

	periodicNames := sets.NewString()
	for _, p := range cfg.AllPeriodics() {
		periodicNames.Insert(p.Name)
	}

	existing := sets.NewString()
	for k := range c.jobs {
		existing.Insert(k)
	}

	var removalErrors []error
	for _, job := range existing.Difference(periodicNames).List() {
		if err := c.removeJob(job); err != nil {
			removalErrors = append(removalErrors, err)
		}
	}

	return utilerrors.NewAggregate(removalErrors)
}

// HasJob returns if a job has been scheduled in cronAgent or not
func (c *Cron) HasJob(name string) bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	_, ok := c.jobs[name]
	return ok
}

func (c *Cron) addPeriodic(p config.Periodic) error {
	if p.Cron == "" {
		return nil
	}

	if job, ok := c.jobs[p.Name]; ok {
		if job.cronStr == p.Cron {
			return nil
		}
		// job updated, remove old entry
		if err := c.removeJob(p.Name); err != nil {
			return err
		}
	}

	if err := c.addJob(p.Name, p.Cron); err != nil {
		return err
	}

	return nil
}

// addJob adds a cron entry for a job to cronAgent
func (c *Cron) addJob(name, cron string) error {
	id, err := c.cronAgent.AddFunc("TZ=UTC "+cron, func() {
		c.lock.Lock()
		defer c.lock.Unlock()

		c.jobs[name].triggered = true
		c.logger.Infof("Triggering cron job %s.", name)
	})

	if err != nil {
		return fmt.Errorf("cronAgent fails to add job %s with cron %s: %v", name, cron, err)
	}

	c.jobs[name] = &jobStatus{
		entryID: id,
		cronStr: cron,
		// try to kick of a periodic trigger right away
		triggered: strings.HasPrefix(cron, "@every"),
	}

	c.logger.Infof("Added new cron job %s with trigger %s.", name, cron)
	return nil
}

// removeJob removes the job from cronAgent
func (c *Cron) removeJob(name string) error {
	job, ok := c.jobs[name]
	if !ok {
		return fmt.Errorf("job %s has not been added to cronAgent yet", name)
	}
	c.cronAgent.Remove(job.entryID)
	delete(c.jobs, name)
	c.logger.Infof("Removed previous cron job %s.", name)
	return nil
}
