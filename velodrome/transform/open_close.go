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

// OpenToClose plugin: Count Open To Close latency
type OpenToClose struct {
	DB   InfluxDatabase
	last time.Time
	open map[int]time.Time
}

// NewOpenToClosePlugin initializes the plugin. Requires an
// InfluxDatabase to push the metric
func NewOpenToClosePlugin(DB InfluxDatabase) *OpenToClose {
	last, err := DB.GetLastMeasurement("open_to_close")
	if err != nil {
		glog.Fatal("Failed to create OpenToClose plugin: ", err)
	}
	return &OpenToClose{
		DB:   DB,
		last: last,
		open: map[int]time.Time{},
	}
}

// ReceiveIssue set-up issues that are actually PRs
func (o *OpenToClose) ReceiveIssue(issue sql.Issue) error {
	if _, ok := o.open[issue.ID]; ok {
		return nil
	}

	if !issue.IsPR {
		return nil
	}

	o.open[issue.ID] = issue.IssueCreatedAt
	return nil
}

// ReceiveComment is not used for this metric
func (*OpenToClose) ReceiveComment(sql.Comment) error {
	return nil
}

// ReceiveIssueEvent computes the time since open (if closed)
func (o *OpenToClose) ReceiveIssueEvent(event sql.IssueEvent) error {
	if event.Event != "closed" {
		return nil
	}
	if !event.EventCreatedAt.After(o.last) {
		return nil
	}

	open, ok := o.open[event.IssueId]
	if !ok {
		// Either never opened, or is issue
		return nil
	}

	open_to_close := event.EventCreatedAt.Sub(open)
	return o.DB.Push(
		"open_to_close",
		nil,
		map[string]interface{}{"value": int(open_to_close / time.Minute)},
		event.EventCreatedAt,
	)
}
