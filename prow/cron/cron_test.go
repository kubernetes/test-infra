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

package cron

import (
	"testing"

	cron "gopkg.in/robfig/cron.v2"
	"k8s.io/test-infra/prow/config"
)

func TestSync(t *testing.T) {
	c := New()
	initial := &config.Config{
		JobConfig: config.JobConfig{
			Periodics: []config.Periodic{
				{
					JobBase: config.JobBase{
						Name: "interval",
					},
					Interval: "1m",
				},
				{
					JobBase: config.JobBase{
						Name: "cron",
					},
					Cron: "@every 1m",
				},
			},
		},
	}

	addAndUpdate := &config.Config{
		JobConfig: config.JobConfig{
			Periodics: []config.Periodic{
				{
					JobBase: config.JobBase{
						Name: "interval",
					},
					Interval: "1m",
				},
				{
					JobBase: config.JobBase{
						Name: "cron",
					},
					Cron: "@every 1m",
				},
				{
					JobBase: config.JobBase{
						Name: "cron-2",
					},
					Cron: "@every 1m",
				},
			},
		},
	}

	deleted := &config.Config{
		JobConfig: config.JobConfig{
			Periodics: []config.Periodic{
				{
					JobBase: config.JobBase{
						Name: "interval",
					},
					Interval: "1m",
				},
				{
					JobBase: config.JobBase{
						Name: "cron-2",
					},
					Cron: "@every 1h",
				},
			},
		},
	}

	if err := c.SyncConfig(initial); err != nil {
		t.Fatalf("error first sync config: %v", err)
	}

	if c.HasJob("interval") {
		t.Error("1st sync, should not have job 'interval'")
	}

	var cronID cron.EntryID
	if !c.HasJob("cron") {
		t.Error("1st sync, should have job 'cron'")
	} else {
		cronID = c.jobs["cron"].entryID
	}

	if err := c.SyncConfig(addAndUpdate); err != nil {
		t.Fatalf("error sync cfg: %v", err)
	}

	if !c.HasJob("cron") {
		t.Error("2nd sync, should have job 'cron'")
	} else {
		newCronID := c.jobs["cron"].entryID
		if newCronID != cronID {
			t.Errorf("2nd sync, entryID for 'cron' should not be updated")
		}
	}

	if !c.HasJob("cron-2") {
		t.Error("2nd sync, should have job 'cron-2'")
	} else {
		cronID = c.jobs["cron-2"].entryID
	}

	if err := c.SyncConfig(deleted); err != nil {
		t.Fatalf("error sync cfg: %v", err)
	}

	if c.HasJob("cron") {
		t.Error("3rd sync, should not have job 'cron'")
	}

	if !c.HasJob("cron-2") {
		t.Error("3rd sync, should have job 'cron-2'")
	} else {
		newCronID := c.jobs["cron-2"].entryID
		if newCronID == cronID {
			t.Errorf("3rd sync, entryID for 'cron-2' should be updated")
		}
	}
}

func TestTrigger(t *testing.T) {
	c := New()
	cfg := &config.Config{
		JobConfig: config.JobConfig{
			Periodics: []config.Periodic{
				{
					JobBase: config.JobBase{
						Name: "cron",
					},
					Cron: "* 8 * * *",
				},
				{
					JobBase: config.JobBase{
						Name: "periodic",
					},
					Cron: "@every 1h",
				},
			},
		},
	}

	if err := c.SyncConfig(cfg); err != nil {
		t.Fatalf("error sync config: %v", err)
	}

	periodic := false
	for _, job := range c.QueuedJobs() {
		if job == "cron" {
			t.Errorf("should not have triggered job 'cron'")
		} else if job == "periodic" {
			periodic = true
		}
	}

	if !periodic {
		t.Errorf("should have triggered job 'periodic'")
	}

	// force trigger
	for _, entry := range c.cronAgent.Entries() {
		entry.Job.Run()
	}

	periodic = false
	cron := false
	for _, job := range c.QueuedJobs() {
		if job == "cron" {
			cron = true
		} else if job == "periodic" {
			periodic = true
		}
	}

	if !cron {
		t.Error("should have triggered job 'cron'")
	}
	if !periodic {
		t.Error("should have triggered job 'periodic'")
	}
}
