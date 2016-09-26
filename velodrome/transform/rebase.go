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

// Rebase plugin: Count number of rebases
type Rebase struct {
	DB   InfluxDatabase
	last time.Time
	// Number of rebase for each pull-request
	rebases map[int]int
}

// NewRebasePlugin initializes the rebase plugin. Requires an
// InfluxDatabase to push the metric
func NewRebasePlugin(DB InfluxDatabase) *Rebase {
	last, err := DB.GetLastMeasurement("rebase")
	if err != nil {
		glog.Fatal("Failed to create Rebase plugin: ", err)
	}

	return &Rebase{
		DB:      DB,
		last:    last,
		rebases: make(map[int]int),
	}
}

// ReceiveIssue is not used for this metric
func (m *Rebase) ReceiveIssue(sql.Issue) error {
	return nil
}

// ReceiveComment is not used for this metric
func (m *Rebase) ReceiveComment(sql.Comment) error {
	return nil
}

func (m *Rebase) processMerge(issueID int, date time.Time) error {
	if !date.After(m.last) {
		return nil
	}

	return m.DB.Push(
		"rebase",
		nil,
		map[string]interface{}{"value": m.rebases[issueID]},
		date)
}

// ReceiveIssueEvent filters "rebased" events, and add the measurement
func (m *Rebase) ReceiveIssueEvent(event sql.IssueEvent) error {
	if event.Event == "labeled" && event.Label != nil && *event.Label == "needs-rebase" {
		m.rebases[event.IssueId]++
	} else if event.Event == "merged" {
		return m.processMerge(event.IssueId, event.EventCreatedAt)
	}

	return nil
}
