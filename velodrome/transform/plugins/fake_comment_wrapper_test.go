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

func TestFakeCommentWrapper(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	plugin := NewMockPlugin(ctrl)
	fakeComment := NewFakeCommentPluginWrapper(plugin)

	plugin.EXPECT().ReceiveIssueEvent(sql.IssueEvent{
		IssueID:        "1",
		Event:          "commented",
		EventCreatedAt: time.Unix(10, 0),
		Actor:          stringPointer("SomeUser"),
	}).Return([]Point{{}})
	plugin.EXPECT().ReceiveComment(sql.Comment{
		IssueID:          "1",
		CommentCreatedAt: time.Unix(10, 0),
		User:             "SomeUser",
	}).Return([]Point{{}})

	got := fakeComment.ReceiveComment(sql.Comment{
		IssueID:          "1",
		CommentCreatedAt: time.Unix(10, 0),
		User:             "SomeUser",
	})
	if len(got) != 2 {
		t.Errorf("ReceiveComment pass-through failed: length(%+v) = %d, want %d", got, len(got), 2)
	}
}
