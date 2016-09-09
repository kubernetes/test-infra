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

	"k8s.io/test-infra/velodrome/sql"

	"github.com/golang/mock/gomock"
)

func makeSimpleDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func TestMerged(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockIDB := NewMockInfluxDatabase(mockCtrl)

	mockIDB.EXPECT().GetLastMeasurement("merged").Return(makeSimpleDate(2000, time.January, 15), nil)
	mockIDB.EXPECT().Push("merged", nil, map[string]interface{}{"value": 1}, makeSimpleDate(2000, time.January, 20))
	mockIDB.EXPECT().Push("merged", nil, map[string]interface{}{"value": 1}, makeSimpleDate(2000, time.January, 30))

	plugin := NewMergedPlugin(mockIDB)

	plugin.ReceiveIssueEvent(sql.IssueEvent{Event: "something", EventCreatedAt: makeSimpleDate(2000, time.January, 5)})
	plugin.ReceiveIssueEvent(sql.IssueEvent{Event: "merged", EventCreatedAt: makeSimpleDate(2000, time.January, 10)})
	plugin.ReceiveIssueEvent(sql.IssueEvent{Event: "merged", EventCreatedAt: makeSimpleDate(2000, time.January, 15)})
	plugin.ReceiveIssueEvent(sql.IssueEvent{Event: "merged", EventCreatedAt: makeSimpleDate(2000, time.January, 20)})
	plugin.ReceiveIssueEvent(sql.IssueEvent{Event: "something", EventCreatedAt: makeSimpleDate(2000, time.January, 25)})
	plugin.ReceiveIssueEvent(sql.IssueEvent{Event: "merged", EventCreatedAt: makeSimpleDate(2000, time.January, 30)})
}
