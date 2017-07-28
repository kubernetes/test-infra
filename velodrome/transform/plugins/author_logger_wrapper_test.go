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
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"k8s.io/test-infra/velodrome/sql"
)

func TestAuthorLoggerEnabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	plugin := NewMockPlugin(ctrl)
	authorLogger := NewAuthorLoggerPluginWrapper(plugin)
	authorLogger.enabled = true

	actor := "Person"
	plugin.EXPECT().ReceiveIssue(sql.Issue{User: "Person"}).Return([]Point{{}})
	plugin.EXPECT().ReceiveIssueEvent(sql.IssueEvent{Actor: &actor}).Return([]Point{{}})
	plugin.EXPECT().ReceiveComment(sql.Comment{User: "Person"}).Return([]Point{{}})

	got := authorLogger.ReceiveIssue(sql.Issue{User: "Person"})
	want := []Point{{Values: map[string]interface{}{"author": "Person"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Failure to log author: got %+v, want %+v", got, want)
	}

	got = authorLogger.ReceiveIssueEvent(sql.IssueEvent{Actor: &actor})
	want = []Point{{Values: map[string]interface{}{"author": "Person"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Failure to log author: got %+v, want %+v", got, want)
	}

	got = authorLogger.ReceiveComment(sql.Comment{User: "Person"})
	want = []Point{{Values: map[string]interface{}{"author": "Person"}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Failure to log author: got %+v, want %+v", got, want)
	}
}

func TestAuthorLoggerDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	plugin := NewMockPlugin(ctrl)
	authorLogger := NewAuthorLoggerPluginWrapper(plugin)
	authorLogger.enabled = false

	actor := "Person"
	plugin.EXPECT().ReceiveIssue(sql.Issue{User: "Person"}).Return([]Point{{}})
	plugin.EXPECT().ReceiveIssueEvent(sql.IssueEvent{Actor: &actor}).Return([]Point{{}})
	plugin.EXPECT().ReceiveComment(sql.Comment{User: "Person"}).Return([]Point{{}})

	got := authorLogger.ReceiveIssue(sql.Issue{User: "Person"})
	want := []Point{{}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Failure to pass-through: got %+v, want %+v", got, want)
	}

	got = authorLogger.ReceiveIssueEvent(sql.IssueEvent{Actor: &actor})
	want = []Point{{}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Failure to pass-through: got %+v, want %+v", got, want)
	}

	got = authorLogger.ReceiveComment(sql.Comment{User: "Person"})
	want = []Point{{}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Failure to pass-through: got %+v, want %+v", got, want)
	}
}
