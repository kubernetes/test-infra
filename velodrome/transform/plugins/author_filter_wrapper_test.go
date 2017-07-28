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
	"testing"

	"github.com/golang/mock/gomock"
	"k8s.io/test-infra/velodrome/sql"
)

// We ignore everything coming from specified people.
func TestAuthorFilterIssues(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	plugin := NewMockPlugin(ctrl)
	authorFilter := NewAuthorFilterPluginWrapper(plugin)
	authorFilter.ignoredAuthors = []string{"OneBot", "OtherBot"}

	plugin.EXPECT().ReceiveIssue(sql.Issue{ID: "2", User: "Person"}).Return([]Point{{}})

	if p := authorFilter.ReceiveIssue(sql.Issue{ID: "1", User: "OneBot"}); len(p) != 0 {
		t.Error("OneBot issues should be filtered")
	}
	if p := authorFilter.ReceiveIssue(sql.Issue{ID: "2", User: "Person"}); len(p) != 1 {
		t.Error("Person issues shouldn't be filtered")
	}
	if p := authorFilter.ReceiveIssue(sql.Issue{ID: "3", User: "OtherBot"}); len(p) != 0 {
		t.Error("OtherBot issues should be filtered")
	}
}

// We ignore everything coming from specified people.
func TestAuthorFilterIssueEvents(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	plugin := NewMockPlugin(ctrl)
	authorFilter := NewAuthorFilterPluginWrapper(plugin)
	authorFilter.ignoredAuthors = []string{"OneBot", "OtherBot"}

	expectedActor := "Person"
	plugin.EXPECT().ReceiveIssueEvent(sql.IssueEvent{Actor: &expectedActor}).Return([]Point{{}})
	plugin.EXPECT().ReceiveIssueEvent(sql.IssueEvent{Actor: nil}).Return([]Point{{}})

	actor := "OneBot"
	if p := authorFilter.ReceiveIssueEvent(sql.IssueEvent{Actor: &actor}); len(p) != 0 {
		t.Error("OneBot issue-events should be filtered")
	}
	actor = "Person"
	if p := authorFilter.ReceiveIssueEvent(sql.IssueEvent{Actor: &actor}); len(p) != 1 {
		t.Error("Person issue-events shouldn't be filtered")
	}
	actor = "OtherBot"
	if p := authorFilter.ReceiveIssueEvent(sql.IssueEvent{Actor: &actor}); len(p) != 0 {
		t.Error("OtherBot issue-events should be filtered")
	}
	if p := authorFilter.ReceiveIssueEvent(sql.IssueEvent{Actor: nil}); len(p) != 1 {
		t.Error("nil Actor issue-events shouldn't be filtered")
	}
}

// We ignore everything coming from specified people.
func TestAuthorFilterComments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	plugin := NewMockPlugin(ctrl)
	authorFilter := NewAuthorFilterPluginWrapper(plugin)
	authorFilter.ignoredAuthors = []string{"OneBot", "OtherBot"}

	plugin.EXPECT().ReceiveComment(sql.Comment{User: "Person"}).Return([]Point{{}})

	if p := authorFilter.ReceiveComment(sql.Comment{User: "OneBot"}); len(p) != 0 {
		t.Error("OneBot issues should be filtered")
	}
	if p := authorFilter.ReceiveComment(sql.Comment{User: "Person"}); len(p) != 1 {
		t.Error("Person issues shouldn't be filtered")
	}
	if p := authorFilter.ReceiveComment(sql.Comment{User: "OtherBot"}); len(p) != 0 {
		t.Error("OtherBot issues should be filtered")
	}
}
