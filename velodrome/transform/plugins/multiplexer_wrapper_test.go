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

	"k8s.io/test-infra/velodrome/sql"

	"github.com/golang/mock/gomock"
)

func TestMultiplexerWrapper(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	plugin1 := NewMockPlugin(ctrl)
	plugin2 := NewMockPlugin(ctrl)
	plugin3 := NewMockPlugin(ctrl)

	multiplexer := NewMultiplexerPluginWrapper(plugin1, plugin2, plugin3)

	plugin1.EXPECT().ReceiveIssue(sql.Issue{ID: "1"}).Return([]Point{{Values: map[string]interface{}{"Issue": 1}}})
	plugin1.EXPECT().ReceiveComment(sql.Comment{ID: "2"}).Return([]Point{{Values: map[string]interface{}{"Comment": 1}}})
	plugin1.EXPECT().ReceiveIssueEvent(sql.IssueEvent{ID: "3"}).Return([]Point{{Values: map[string]interface{}{"Event": 1}}})
	plugin2.EXPECT().ReceiveIssue(sql.Issue{ID: "1"}).Return([]Point{{Values: map[string]interface{}{"Issue": 2}}})
	plugin2.EXPECT().ReceiveComment(sql.Comment{ID: "2"}).Return([]Point{{Values: map[string]interface{}{"Comment": 2}}})
	plugin2.EXPECT().ReceiveIssueEvent(sql.IssueEvent{ID: "3"}).Return([]Point{{Values: map[string]interface{}{"Event": 2}}})
	plugin3.EXPECT().ReceiveIssue(sql.Issue{ID: "1"}).Return([]Point{{Values: map[string]interface{}{"Issue": 3}}})
	plugin3.EXPECT().ReceiveComment(sql.Comment{ID: "2"}).Return([]Point{{Values: map[string]interface{}{"Comment": 3}}})
	plugin3.EXPECT().ReceiveIssueEvent(sql.IssueEvent{ID: "3"}).Return([]Point{{Values: map[string]interface{}{"Event": 3}}})

	got := multiplexer.ReceiveIssue(sql.Issue{ID: "1"})
	want := []Point{
		{Values: map[string]interface{}{"Issue": 1}},
		{Values: map[string]interface{}{"Issue": 2}},
		{Values: map[string]interface{}{"Issue": 3}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf(`multiplexer.ReceiveIssue(sql.Issue{ID: "1"}) = %+v, want %+v`, got, want)
	}

	got = multiplexer.ReceiveComment(sql.Comment{ID: "2"})
	want = []Point{
		{Values: map[string]interface{}{"Comment": 1}},
		{Values: map[string]interface{}{"Comment": 2}},
		{Values: map[string]interface{}{"Comment": 3}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf(`multiplexer.ReceiveComment(sql.Comment{ID: "2"}) = %+v, want %+v`, got, want)
	}

	got = multiplexer.ReceiveIssueEvent(sql.IssueEvent{ID: "3"})
	want = []Point{
		{Values: map[string]interface{}{"Event": 1}},
		{Values: map[string]interface{}{"Event": 2}},
		{Values: map[string]interface{}{"Event": 3}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf(`multiplexer.ReceiveIssueEvent(sql.IssueEvent{ID: "3"}) = %+v, want %+v`, got, want)
	}
}
