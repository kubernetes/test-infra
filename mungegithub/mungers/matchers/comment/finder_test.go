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

package comment

import (
	"reflect"
	"testing"
	"time"
)

func TestFilterComments(t *testing.T) {
	comments := []*Comment{
		makeCommentWithBody("1"),
		makeCommentWithBody("2"),
		makeCommentWithBody("3"),
		makeCommentWithBody("4"),
	}

	emptyList := FilterComments(comments, False{})
	if len(emptyList) != 0 {
		t.Error("False filter shouldn't match any element")
	}

	fullList := FilterComments(comments, True{})
	if !reflect.DeepEqual([]*Comment(fullList), comments) {
		t.Error("True filter should have kept every element")
	}
}

func makeCommentWithCreatedAt(year int, month time.Month, day int) *Comment {
	date := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	return &Comment{
		CreatedAt: &date,
	}
}

func TestLastCommentDefault(t *testing.T) {
	if LastComment(nil, True{}, nil) != nil {
		t.Error("Empty list should return nil default")
	}
	if !reflect.DeepEqual(LastComment(nil, True{}, &time.Time{}), &time.Time{}) {
		t.Error("Empty list should return given default value")
	}
}

func TestLastComment(t *testing.T) {
	comments := []*Comment{
		makeCommentWithCreatedAt(2000, 1, 1),
		makeCommentWithCreatedAt(2000, 1, 2),
		makeCommentWithCreatedAt(2000, 1, 3),
	}
	if !reflect.DeepEqual(*LastComment(comments, True{}, nil), time.Date(2000, 1, 3, 0, 0, 0, 0, time.UTC)) {
		t.Error("Should match the last comment")
	}
}
