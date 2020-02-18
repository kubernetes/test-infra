/*
Copyright 2020 The Kubernetes Authors.

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

package metrics_test

import (
	"reflect"
	"sort"
	"testing"

	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/metrics"
)

func TestNormalizeResourceMetrics(t *testing.T) {
	type update struct {
		rtype string
		state string
		count float64
	}
	testCases := []struct {
		name            string
		metrics         []common.Metric
		states          []string
		expectedUpdates []update
	}{
		{
			name:            "No metrics",
			states:          common.KnownStates,
			expectedUpdates: []update{},
		},
		{
			name: "one metric",
			metrics: []common.Metric{
				{
					Type: "foo-project",
					Current: map[string]int{
						"free":  2,
						"dirty": 5,
					},
				},
			},
			states: []string{"free", "dirty", "busy"},
			expectedUpdates: []update{
				{"foo-project", "busy", 0},
				{"foo-project", "dirty", 5},
				{"foo-project", "free", 2},
			},
		},
		{
			name: "one metric, no currents",
			metrics: []common.Metric{
				{
					Type: "foo-project",
				},
			},
			states: []string{"free", "dirty", "busy"},
			expectedUpdates: []update{
				{"foo-project", "busy", 0},
				{"foo-project", "dirty", 0},
				{"foo-project", "free", 0},
			},
		},
		{
			name: "multiple metrics with extra states",
			metrics: []common.Metric{
				{
					Type: "bar-project",
					Current: map[string]int{
						"free":     6,
						"dirty":    0,
						"chilling": 5,
						"busy":     2,
					},
				},
				{
					Type: "baz-project",
					Current: map[string]int{
						"dirty": 3,
					},
				},
				{
					Type: "foo-project",
					Current: map[string]int{
						"free":   2,
						"leased": 2,
					},
				},
				{
					Type: "not-so-extra",
					Current: map[string]int{
						"extra": 0,
					},
				},
			},
			states: []string{"free", "dirty"},
			expectedUpdates: []update{
				{"bar-project", "dirty", 0},
				{"bar-project", "free", 6},
				{"bar-project", "other", 7}, // chillling + busy
				{"baz-project", "dirty", 3},
				{"baz-project", "free", 0},
				{"foo-project", "dirty", 0},
				{"foo-project", "free", 2},
				{"foo-project", "other", 2}, // leased
				{"not-so-extra", "dirty", 0},
				{"not-so-extra", "free", 0},
				{"not-so-extra", "other", 0}, // extra
			},
		},
	}

	for _, tc := range testCases {
		updates := []update{}
		metrics.NormalizeResourceMetrics(tc.metrics, tc.states, func(rtype, state string, count float64) {
			updates = append(updates, update{rtype, state, count})
		})
		sort.Slice(updates, func(i, j int) bool {
			if updates[i].rtype != updates[j].rtype {
				return updates[i].rtype < updates[j].rtype
			}
			if updates[i].state != updates[j].state {
				return updates[i].state < updates[j].state
			}
			return updates[i].count < updates[j].count
		})
		if !reflect.DeepEqual(tc.expectedUpdates, updates) {
			t.Errorf("%s: expected %v, got %v", tc.name, tc.expectedUpdates, updates)
		}
	}
}
