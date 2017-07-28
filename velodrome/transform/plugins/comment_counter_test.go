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
	"time"

	"k8s.io/test-infra/velodrome/sql"
)

func commentPoint(t time.Time) Point {
	return Point{
		Values: map[string]interface{}{
			"comment": 1,
		},
		Date: t,
	}
}

func TestCommentCounter(t *testing.T) {
	tests := []struct {
		pattern  string
		comments []sql.Comment
		expected []Point
	}{
		{
			pattern: "",
			comments: []sql.Comment{
				{
					Body:             "Something",
					CommentCreatedAt: time.Unix(10, 0),
				},
				{
					Body:             "Anything",
					CommentCreatedAt: time.Unix(20, 0),
				},
				{
					Body:             "It doesn't matter",
					CommentCreatedAt: time.Unix(30, 0),
				},
			},
			expected: []Point{
				commentPoint(time.Unix(10, 0)),
				commentPoint(time.Unix(20, 0)),
				commentPoint(time.Unix(30, 0)),
			},
		},
		{
			pattern: `(?m)/lgtm\s*$`,
			comments: []sql.Comment{
				{
					Body:             "/lgtm cancel",
					CommentCreatedAt: time.Unix(10, 0),
				},
				{
					Body:             "/lgtm",
					CommentCreatedAt: time.Unix(20, 0),
				},
				{
					Body:             "/lgtm    \nOr not.",
					CommentCreatedAt: time.Unix(30, 0),
				},
			},
			expected: []Point{
				commentPoint(time.Unix(20, 0)),
				commentPoint(time.Unix(30, 0)),
			},
		},
	}

	for _, test := range tests {
		plugin := CommentCounterPlugin{pattern: []string{test.pattern}}
		if err := plugin.CheckFlags(); err != nil {
			t.Fatalf("Failed to initial comment counter (%s): %s", test.pattern, err)
		}
		got := []Point{}
		for _, comment := range test.comments {
			got = append(got, plugin.ReceiveComment(comment)...)
		}
		want := test.expected
		if !reflect.DeepEqual(got, want) {
			t.Errorf(`CommentCounterPlugin{pattern: "%s".ReceiveComment = %+v, want %+v`,
				test.pattern, got, want)
		}
	}
}
