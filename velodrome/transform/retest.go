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

package main

import (
	"regexp"
	"time"

	"github.com/golang/glog"

	"k8s.io/test-infra/velodrome/sql"
)

var (
	// Matches "@k8s-bot foo bar test this" with the capture group "foo bar".
	testThisRe = regexp.MustCompile(`@k8s-bot (\w*(?: \w+)*) ?test this`)

	robotNames = map[string]bool{
		"k8s-merge-robot": true,
		"k8s-ci-robot":    true,
	}
)

type Retest struct {
	DB   InfluxDatabase
	last time.Time
	// Number of requests for retest per PR.
	retests map[string]int
}

// NewRetestPlugin initializes the plugin.
func NewRetestPlugin(DB InfluxDatabase) *Retest {
	last, err := DB.GetLastMeasurement("retest")
	if err != nil {
		glog.Fatal("Failed to create Retest plugin: ", err)
	}
	return &Retest{
		DB:      DB,
		last:    last,
		retests: make(map[string]int),
	}
}

// ReceiveIssue is not used for this metric.
func (*Retest) ReceiveIssue(sql.Issue) error {
	return nil
}

// ReceiveComment counts occurences of "@k8s-bot test this"-style comments.
func (r *Retest) ReceiveComment(comment sql.Comment) error {
	if robotNames[comment.User] {
		return nil
	}
	if matches := testThisRe.MatchString(comment.Body); !matches {
		return nil
	}
	r.retests[comment.IssueID]++
	return nil
}

// ReceiveIssueEvent adds to the DB when a PR is merged.
func (r *Retest) ReceiveIssueEvent(event sql.IssueEvent) error {
	if !event.EventCreatedAt.After(r.last) {
		return nil
	}
	if event.Event != "merged" {
		return nil
	}
	return r.DB.Push(
		"retest",
		nil,
		map[string]interface{}{"value": r.retests[event.IssueId]},
		event.EventCreatedAt)
}
