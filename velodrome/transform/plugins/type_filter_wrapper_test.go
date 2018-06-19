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

	"k8s.io/test-infra/velodrome/sql"

	"github.com/golang/mock/gomock"
)

// We can't ignore everything. It should fail.
func TestTypeFilterWrapperFilterAllFails(t *testing.T) {
	typefilter := NewTypeFilterWrapperPlugin(nil)
	typefilter.issues = true
	typefilter.pullRequests = true

	if err := typefilter.CheckFlags(); err == nil {
		t.Error("TypeFilter should fail to be no-issues and no-pr")
	}
}

// We do ignore everything, expect everything to pass through.
func TestTypeFilterWrapperFilterNothing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	plugin := NewMockPlugin(ctrl)
	typefilter := NewTypeFilterWrapperPlugin(plugin)
	// Filter nothing.

	plugin.EXPECT().ReceiveIssue(sql.Issue{ID: "1", IsPR: false}).Return([]Point{{}})
	plugin.EXPECT().ReceiveIssueEvent(sql.IssueEvent{IssueID: "1"}).Return([]Point{{}})
	plugin.EXPECT().ReceiveComment(sql.Comment{IssueID: "1"}).Return([]Point{{}})
	plugin.EXPECT().ReceiveIssue(sql.Issue{ID: "2", IsPR: true}).Return([]Point{{}})
	plugin.EXPECT().ReceiveIssueEvent(sql.IssueEvent{IssueID: "2"}).Return([]Point{{}})
	plugin.EXPECT().ReceiveComment(sql.Comment{IssueID: "2"}).Return([]Point{{}})

	if p := typefilter.ReceiveIssue(sql.Issue{ID: "1", IsPR: false}); len(p) != 1 {
		t.Error("Nothing should be filtered")
	}
	if p := typefilter.ReceiveIssueEvent(sql.IssueEvent{IssueID: "1"}); len(p) != 1 {
		t.Error("Nothing should be filtered")
	}
	if p := typefilter.ReceiveComment(sql.Comment{IssueID: "1"}); len(p) != 1 {
		t.Error("Nothing should be filtered")
	}

	if p := typefilter.ReceiveIssue(sql.Issue{ID: "2", IsPR: true}); len(p) != 1 {
		t.Error("Nothing should be filtered")
	}
	if p := typefilter.ReceiveIssueEvent(sql.IssueEvent{IssueID: "2"}); len(p) != 1 {
		t.Error("Nothing should be filtered")
	}
	if p := typefilter.ReceiveComment(sql.Comment{IssueID: "2"}); len(p) != 1 {
		t.Error("Nothing should be filtered")
	}
}

// Filter issues, PR should pass through
func TestTypeFilterWrapperFilterIssues(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	plugin := NewMockPlugin(ctrl)
	typefilter := NewTypeFilterWrapperPlugin(plugin)
	typefilter.issues = true

	plugin.EXPECT().ReceiveIssue(sql.Issue{ID: "2", IsPR: true}).Return([]Point{{}})
	plugin.EXPECT().ReceiveIssueEvent(sql.IssueEvent{IssueID: "2"}).Return([]Point{{}})
	plugin.EXPECT().ReceiveComment(sql.Comment{IssueID: "2"}).Return([]Point{{}})

	if p := typefilter.ReceiveIssue(sql.Issue{ID: "1", IsPR: false}); len(p) != 0 {
		t.Error("Issue 1 is an issue, should be filtered but received point for Issue")
	}
	if p := typefilter.ReceiveIssueEvent(sql.IssueEvent{IssueID: "1"}); len(p) != 0 {
		t.Error("Issue 1 is an issue, should be filtered but received point for IssueEvent")
	}
	if p := typefilter.ReceiveComment(sql.Comment{IssueID: "1"}); len(p) != 0 {
		t.Error("Issue 1 is an issue, should be filtered but received point for Comment")
	}

	if p := typefilter.ReceiveIssue(sql.Issue{ID: "2", IsPR: true}); len(p) != 1 {
		t.Error("Issue 2 is a PR, should have received point for Issue")
	}
	if p := typefilter.ReceiveIssueEvent(sql.IssueEvent{IssueID: "2"}); len(p) != 1 {
		t.Error("Issue 2 is a PR, should have received point for IssueEvent")
	}
	if p := typefilter.ReceiveComment(sql.Comment{IssueID: "2"}); len(p) != 1 {
		t.Error("Issue 2 is a PR, should have received point for Comment")
	}
}

// Filter PR, issues should pass through
func TestTypeFilterWrapperFilterPR(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	plugin := NewMockPlugin(ctrl)
	typefilter := NewTypeFilterWrapperPlugin(plugin)
	typefilter.pullRequests = true

	plugin.EXPECT().ReceiveIssue(sql.Issue{ID: "2", IsPR: false}).Return([]Point{{}})
	plugin.EXPECT().ReceiveIssueEvent(sql.IssueEvent{IssueID: "2"}).Return([]Point{{}})
	plugin.EXPECT().ReceiveComment(sql.Comment{IssueID: "2"}).Return([]Point{{}})

	if p := typefilter.ReceiveIssue(sql.Issue{ID: "1", IsPR: true}); len(p) != 0 {
		t.Error("Issue 1 is a PR, should be filtered but received point for Issue")
	}
	if p := typefilter.ReceiveIssueEvent(sql.IssueEvent{IssueID: "1"}); len(p) != 0 {
		t.Error("Issue 1 is a PR, should be filtered but received point for IssueEvent")
	}
	if p := typefilter.ReceiveComment(sql.Comment{IssueID: "1"}); len(p) != 0 {
		t.Error("Issue 1 is a PR, should be filtered but received point for Comment")
	}

	if p := typefilter.ReceiveIssue(sql.Issue{ID: "2", IsPR: false}); len(p) != 1 {
		t.Error("Issue 2 is an issue, should have received point for Issue")
	}
	if p := typefilter.ReceiveIssueEvent(sql.IssueEvent{IssueID: "2"}); len(p) != 1 {
		t.Error("Issue 2 is an issue, should have received point for IssueEvent")
	}
	if p := typefilter.ReceiveComment(sql.Comment{IssueID: "2"}); len(p) != 1 {
		t.Error("Issue 2 is an issue, should have received point for Comment")
	}
}

// Event before Issue? Won't pass through. Shouldn't happen.
func TestTypeFilterWrapperFilterMissingIssue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	plugin := NewMockPlugin(ctrl)
	typefilter := NewTypeFilterWrapperPlugin(plugin)

	if points := typefilter.ReceiveComment(sql.Comment{IssueID: "1"}); points != nil {
		t.Errorf("ReceiveComment() = %v, expected nil", points)
	}

	if points := typefilter.ReceiveIssueEvent(sql.IssueEvent{IssueID: "1"}); points != nil {
		t.Errorf("ReceiveIssueEvent() = %v, expected nil", points)
	}
}
