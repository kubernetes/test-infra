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

// LGTMToMerged plugin: Count LGTM to Merged latency
type LGTMToMerged struct {
	DB     InfluxDatabase
	last   time.Time
	lgtmed map[int]time.Time
}

// NewLGTMToMergedPlugin initializes the plugin. Requires an
// InfluxDatabase to push the metric
func NewLGTMToMergedPlugin(DB InfluxDatabase) *LGTMToMerged {
	last, err := DB.GetLastMeasurement("lgtm_to_merged")
	if err != nil {
		glog.Fatal("Failed to create LGTMToMerged plugin: ", err)
	}
	return &LGTMToMerged{
		DB:     DB,
		last:   last,
		lgtmed: make(map[int]time.Time),
	}
}

// ReceiveIssue is not used for this metric
func (m *LGTMToMerged) ReceiveIssue(sql.Issue) error {
	return nil
}

// ReceiveComment is not used for this metric
func (m *LGTMToMerged) ReceiveComment(sql.Comment) error {
	return nil
}

func (m *LGTMToMerged) MergeEvent(event sql.IssueEvent) error {
	lgtmDate, ok := m.lgtmed[event.IssueId]
	if !ok {
		// Issue is merged without LGTM, just discard
		return nil
	}

	if !event.EventCreatedAt.After(m.last) {
		return nil
	}

	lgtm_to_merge := event.EventCreatedAt.Sub(lgtmDate)
	return m.DB.Push(
		"lgtm_to_merged",
		nil,
		map[string]interface{}{"value": int(lgtm_to_merge / time.Minute)},
		event.EventCreatedAt,
	)
}

func (m *LGTMToMerged) LGTMEvent(event sql.IssueEvent) error {
	_, ok := m.lgtmed[event.IssueId]
	if !ok {
		m.lgtmed[event.IssueId] = event.EventCreatedAt
	}

	return nil
}

// ReceiveIssueEvent filters "merged" events, and add the measurement
func (m *LGTMToMerged) ReceiveIssueEvent(event sql.IssueEvent) error {
	switch event.Event {
	case "merged":
		return m.MergeEvent(event)
	case "labeled":
		if event.Label != nil && *event.Label == "lgtm" {
			return m.LGTMEvent(event)
		}
		return nil
	default:
		return nil
	}
}
