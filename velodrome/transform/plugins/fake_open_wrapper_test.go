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
	"time"

	"k8s.io/test-infra/velodrome/sql"

	"github.com/golang/mock/gomock"
)

func stringPointer(s string) *string {
	return &s
}

func TestFakeOpenWrapper(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	plugin := NewMockPlugin(ctrl)
	fakeOpen := NewFakeOpenPluginWrapper(plugin)

	gomock.InOrder(
		plugin.EXPECT().ReceiveIssue(sql.Issue{
			ID:             "1",
			User:           "User1",
			IssueCreatedAt: time.Unix(0, 20),
		}).Return([]Point{{}}),
		plugin.EXPECT().ReceiveIssue(sql.Issue{
			ID:             "2",
			User:           "User2",
			IssueCreatedAt: time.Unix(0, 30),
		}).Return([]Point{{}}),
		plugin.EXPECT().ReceiveIssue(sql.Issue{
			ID:             "3",
			User:           "User3",
			IssueCreatedAt: time.Unix(0, 50),
		}).Return([]Point{{}}),
		plugin.EXPECT().ReceiveIssueEvent(sql.IssueEvent{
			ID:             "1",
			Actor:          stringPointer("Actor1"),
			Event:          "event1",
			EventCreatedAt: time.Unix(0, 10),
		}).Return([]Point{{}}),
		plugin.EXPECT().ReceiveIssueEvent(sql.IssueEvent{
			IssueID:        "1",
			Actor:          stringPointer("User1"),
			Event:          "opened",
			EventCreatedAt: time.Unix(0, 20),
		}).Return([]Point{{}}),
		plugin.EXPECT().ReceiveIssueEvent(sql.IssueEvent{
			IssueID:        "2",
			Actor:          stringPointer("User2"),
			Event:          "opened",
			EventCreatedAt: time.Unix(0, 30),
		}).Return([]Point{{}}),
		plugin.EXPECT().ReceiveIssueEvent(sql.IssueEvent{
			ID:             "2",
			Actor:          stringPointer("Actor2"),
			Event:          "event2",
			EventCreatedAt: time.Unix(0, 40),
		}).Return([]Point{{}}),
		plugin.EXPECT().ReceiveIssueEvent(sql.IssueEvent{
			IssueID:        "3",
			Actor:          stringPointer("User3"),
			Event:          "opened",
			EventCreatedAt: time.Unix(0, 50),
		}).Return([]Point{{}}),
		plugin.EXPECT().ReceiveIssueEvent(sql.IssueEvent{
			ID:             "3",
			Actor:          stringPointer("Actor3"),
			Event:          "event3",
			EventCreatedAt: time.Unix(0, 50),
		}).Return([]Point{{}}),
	)

	got := fakeOpen.ReceiveIssue(sql.Issue{
		ID:             "1",
		User:           "User1",
		IssueCreatedAt: time.Unix(0, 20),
	})
	if len(got) != 1 {
		t.Errorf("ReceiveIssue 1 pass-through failed: length(%+v) = %d, want %d", got, len(got), 1)
	}
	got = fakeOpen.ReceiveIssue(sql.Issue{
		ID:             "2",
		User:           "User2",
		IssueCreatedAt: time.Unix(0, 30),
	})
	if len(got) != 1 {
		t.Errorf("ReceiveIssue 2 pass-through failed: length(%+v) = %d, want %d", got, len(got), 1)
	}
	got = fakeOpen.ReceiveIssue(sql.Issue{
		ID:             "3",
		User:           "User3",
		IssueCreatedAt: time.Unix(0, 50),
	})
	if len(got) != 1 {
		t.Errorf("ReceiveIssue 3 pass-through failed: length(%+v) = %d, want %d", got, len(got), 1)
	}
	got = fakeOpen.ReceiveIssueEvent(sql.IssueEvent{
		ID:             "1",
		Actor:          stringPointer("Actor1"),
		Event:          "event1",
		EventCreatedAt: time.Unix(0, 10),
	})
	if len(got) != 1 {
		t.Errorf("ReceiveIssueEvent 1 pass-through failed: length(%+v) = %d, want %d", got, len(got), 1)
	}
	got = fakeOpen.ReceiveIssueEvent(sql.IssueEvent{
		ID:             "2",
		Actor:          stringPointer("Actor2"),
		Event:          "event2",
		EventCreatedAt: time.Unix(0, 40),
	})
	// This receives points for each opened event we inserted before IssueEvent 2
	if len(got) != 3 {
		t.Errorf("ReceiveIssueEvent 2 pass-through failed: length(%+v) = %d, want %d", got, len(got), 3)
	}
	got = fakeOpen.ReceiveIssueEvent(sql.IssueEvent{
		ID:             "3",
		Actor:          stringPointer("Actor3"),
		Event:          "event3",
		EventCreatedAt: time.Unix(0, 50),
	})
	if len(got) != 2 {
		t.Errorf("ReceiveIssueEvent 3 pass-through failed: length(%+v) = %d, want %d", got, len(got), 2)
	}
}
