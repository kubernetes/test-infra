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
	"testing"
	"time"

	"github.com/golang/mock/gomock"

	"k8s.io/test-infra/velodrome/sql"
)

func TestDispatch(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockPlugin := NewMockPlugin(mockCtrl)

	gomock.InOrder(
		mockPlugin.EXPECT().ReceiveIssue(sql.Issue{ID: 1}),
		mockPlugin.EXPECT().ReceiveIssue(sql.Issue{ID: 2}),
	)
	gomock.InOrder(
		mockPlugin.EXPECT().ReceiveIssueEvent(sql.IssueEvent{ID: 1}),
		mockPlugin.EXPECT().ReceiveIssueEvent(sql.IssueEvent{ID: 2}),
	)
	gomock.InOrder(
		mockPlugin.EXPECT().ReceiveComment(sql.Comment{ID: 1}),
		mockPlugin.EXPECT().ReceiveComment(sql.Comment{ID: 2}),
	)

	issues := make(chan sql.Issue)
	events := make(chan sql.IssueEvent)
	comments := make(chan sql.Comment)

	plugins := Plugins([]Plugin{mockPlugin})
	go plugins.Dispatch(issues, events, comments)

	issues <- sql.Issue{ID: 1}
	issues <- sql.Issue{ID: 2}
	events <- sql.IssueEvent{ID: 1}
	events <- sql.IssueEvent{ID: 2}
	comments <- sql.Comment{ID: 1}
	comments <- sql.Comment{ID: 2}

	time.Sleep(time.Millisecond)

	close(issues)
	close(events)
	close(comments)
}
