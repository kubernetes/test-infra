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

package main

import (
	"testing"
)

func Test_isReady(t *testing.T) {
	grid := []struct {
		node     *node
		expected bool
	}{
		{
			node: &node{
				Status: nodeStatus{
					Conditions: []nodeCondition{},
				},
			},
			expected: false,
		},
		{
			node: &node{
				Status: nodeStatus{
					Conditions: []nodeCondition{
						{Type: "Ready", Status: "True"},
					},
				},
			},
			expected: true,
		},
		{
			node: &node{
				Status: nodeStatus{
					Conditions: []nodeCondition{
						{Type: "Ready", Status: "False"},
					},
				},
			},
			expected: false,
		},
		{
			node: &node{
				Status: nodeStatus{
					Conditions: []nodeCondition{
						{Type: "Ready", Status: "Unknown"},
					},
				},
			},
			expected: false,
		},
		{
			node: &node{
				Status: nodeStatus{
					Conditions: []nodeCondition{
						{Type: "Ready", Status: "not-a-known-value"},
					},
				},
			},
			expected: false,
		},
		{
			node: &node{
				Status: nodeStatus{
					Conditions: []nodeCondition{
						{Type: "Random", Status: "True"},
					},
				},
			},
			expected: false,
		},
		{
			node: &node{
				Status: nodeStatus{
					Conditions: []nodeCondition{
						{Type: "Random", Status: "Unknown"},
					},
				},
			},
			expected: false,
		},
		{
			node: &node{
				Status: nodeStatus{
					Conditions: []nodeCondition{
						{Type: "Random", Status: "True"},
						{Type: "Ready", Status: "True"},
					},
				},
			},
			expected: true,
		},
		{
			node: &node{
				Status: nodeStatus{
					Conditions: []nodeCondition{},
				},
			},
			expected: false,
		},
		{
			node: &node{
				Status: nodeStatus{},
			},
			expected: false,
		},
		{
			node:     &node{},
			expected: false,
		},
	}

	for _, g := range grid {
		actual := isReady(g.node)

		if actual != g.expected {
			t.Errorf("unexpected isReady.  actual=%v, expected=%v", actual, g.expected)
			continue
		}
	}
}

func Test_countReadyNodes(t *testing.T) {
	readyNode := node{
		Status: nodeStatus{
			Conditions: []nodeCondition{
				{Type: "Ready", Status: "True"},
			},
		},
	}
	notReadyNode := node{
		Status: nodeStatus{
			Conditions: []nodeCondition{
				{Type: "Ready", Status: "False"},
			},
		},
	}

	grid := []struct {
		nodes    []node
		expected int
	}{
		{
			nodes:    []node{readyNode, readyNode, readyNode},
			expected: 3,
		},
		{
			nodes:    []node{notReadyNode, notReadyNode, notReadyNode},
			expected: 0,
		},
		{
			nodes:    []node{},
			expected: 0,
		},
		{
			nodes:    nil,
			expected: 0,
		},
		{
			nodes:    []node{readyNode, notReadyNode, readyNode},
			expected: 2,
		},
		{
			nodes:    []node{notReadyNode, readyNode},
			expected: 1,
		},
	}

	for _, g := range grid {
		actual := countReadyNodes(&nodeList{Items: g.nodes})
		if actual != g.expected {
			t.Errorf("unexpected countReadyNodes.  actual=%v, expected=%v", actual, g.expected)
			continue
		}
	}
}
