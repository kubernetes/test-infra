/*
`Copyright 2016 The Kubernetes Authors.

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

// Merged plugin: Count number of merges
type Merged struct {
	DB   InfluxDatabase
	last time.Time
}

// NewMergedPlugin initializes the merge plugin. Requires an
// InfluxDatabase to push the metric
func NewMergedPlugin(DB InfluxDatabase) *Merged {
	last, err := DB.GetLastMeasurement("merged")
	if err != nil {
		glog.Fatal("Failed to create Merged plugin: ", err)
	}
	return &Merged{
		DB:   DB,
		last: *last,
	}
}

// ReceiveIssue is not used for this metric
func (m *Merged) ReceiveIssue(sql.Issue) error {
	return nil
}

// ReceiveComment is not used for this metric
func (m *Merged) ReceiveComment(sql.Comment) error {
	return nil
}

// ReceiveIssueEvent filters "merged" events, and add the measurement
func (m *Merged) ReceiveIssueEvent(event sql.IssueEvent) error {
	if event.Event != "merged" {
		return nil
	}
	if !event.EventCreatedAt.After(m.last) {
		return nil
	}

	return m.DB.Push("merged", nil, map[string]interface{}{"value": 1}, event.EventCreatedAt)
}
