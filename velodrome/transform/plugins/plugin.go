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

package plugins

import (
	"time"

	"k8s.io/test-infra/velodrome/sql"
)

// Point is what a plugin will return if it wants to insert a new value
// in db.
type Point struct {
	Tags   map[string]string
	Values map[string]interface{}
	Date   time.Time
}

// Plugin is the generic interface for metrics stats and measurement.
// Each metric will be implemented as a Plugin, compute the measurement
// and push it to the InfluxDatabase. nil Point means there is nothing
// to return.
type Plugin interface {
	ReceiveIssue(sql.Issue) []Point
	ReceiveComment(sql.Comment) []Point
	ReceiveIssueEvent(sql.IssueEvent) []Point
}
