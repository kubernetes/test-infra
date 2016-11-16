/*
Copyright 2016 The Kubernetes Authors.

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

package main

import (
	"time"

	"github.com/golang/glog"
	"k8s.io/test-infra/velodrome/sql"
)

type Status struct {
	Open        bool
	Lgtm        bool
	Approved    bool
	NeedsRebase bool
	Merged      bool
	Closed      bool
}

// PRStats plugin: Count various stats on PR
type PRStats struct {
	DB       InfluxDatabase
	last     time.Time
	statuses map[int]*Status
}

func (p PRStats) Count() map[string]int {
	count := map[string]int{}

	for _, status := range p.statuses {
		if status.Lgtm {
			count["lgtm"]++
		}
		if status.Approved {
			count["approved"]++
		}
		if status.Open {
			count["open"]++
		}
		if status.Closed {
			count["closed"]++
		}
		if status.Merged {
			count["merged"]++
		}
		if status.NeedsRebase {
			count["needsrebase"]++
		}
	}

	return count
}

// NewPRStatsPlugin initializes the plugin. Requires an
// InfluxDatabase to push the metric
func NewPRStatsPlugin(DB InfluxDatabase) *PRStats {
	last, err := DB.GetLastMeasurement("prstats")
	if err != nil {
		glog.Fatal("Failed to create LGTMToMerged plugin: ", err)
	}
	return &PRStats{
		DB:       DB,
		last:     last,
		statuses: map[int]*Status{},
	}
}

// ReceiveIssue set-up issues that are actually PRs
func (p *PRStats) ReceiveIssue(issue sql.Issue) error {
	if _, ok := p.statuses[issue.ID]; ok {
		return nil
	}

	if !issue.IsPR {
		return nil
	}

	p.statuses[issue.ID] = &Status{}
	return nil
}

// ReceiveComment is not used for this metric
func (*PRStats) ReceiveComment(sql.Comment) error {
	return nil
}

func updateStatus(status *Status, event sql.IssueEvent) bool {
	previousStatus := *status

	// Something happened on this PR, it must be open
	status.Open = true

	switch event.Event {
	case "closed":
		status.Closed = true
	case "reopened":
		status.Closed = false
	case "merged":
		status.Merged = true
	case "labeled":
		break
	case "unlabeled":
		break
	default:
		return false
	}

	if event.Label == nil {
		return false
	}

	// Handle labels now
	switch *event.Label {
	case "lgtm":
		status.Lgtm = event.Event == "labeled"
	case "approved":
		status.Approved = event.Event == "labeled"
	case "needs-rebase":
		status.NeedsRebase = event.Event == "labeled"
	}

	return *status == previousStatus
}

// ReceiveIssueEvent computes the statistics
func (p *PRStats) ReceiveIssueEvent(event sql.IssueEvent) error {
	status, ok := p.statuses[event.IssueId]
	if !ok {
		return nil
	}
	updated := updateStatus(status, event)
	if !updated {
		return nil
	}

	if !event.EventCreatedAt.After(p.last) {
		return nil
	}

	for tag, count := range p.Count() {
		err := p.DB.Push(
			"prstats",
			map[string]string{"stats": tag},
			map[string]interface{}{"value": count},
			event.EventCreatedAt,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
