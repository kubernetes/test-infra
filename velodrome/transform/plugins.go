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

type InfluxDatabase interface {
	// GetLastMeasurement returns the time of the last measurement pushed to the database
	GetLastMeasurement(string) (*time.Time, error)
	// Push this point to database
	Push(string, map[string]string, map[string]interface{}, time.Time) error
}

type Plugin interface {
	ReceiveIssue(sql.Issue) error
	ReceiveComment(sql.Comment) error
	ReceiveIssueEvent(sql.IssueEvent) error
}

type Plugins []Plugin

func NewPlugins(idb InfluxDatabase) Plugins {
	plugins := Plugins{
	// No plugin yet.
	}

	return plugins
}

func (p Plugins) Dispatch(issues chan sql.Issue, issueEvents chan sql.IssueEvent, comments chan sql.Comment) {
	for {
		select {
		case issue, ok := <-issues:
			if !ok {
				return
			}
			for i := range p {
				if err := p[i].ReceiveIssue(issue); err != nil {
					glog.Fatal("Failed to handle issue: ", err)
				}
			}
		case comment, ok := <-comments:
			if !ok {
				return
			}
			for c := range p {
				if err := p[c].ReceiveComment(comment); err != nil {
					glog.Fatal("Failed to handle comment: ", err)
				}
			}
		case event, ok := <-issueEvents:
			if !ok {
				return
			}
			for i := range p {
				if err := p[i].ReceiveIssueEvent(event); err != nil {
					glog.Fatal("Failed to handle event: ", err)
				}
			}
		}
	}
}
