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

import "testing"

func TestNormalizeRef(t *testing.T) {
	tests := []struct {
		branch string
		want   string
	}{
		{"master", "refs/heads/master"},
		{"upstream/master", "refs/remotes/upstream/master"},
		{"a/b/c", "refs/remotes/a/b/c"},
		{"refs/heads/master", "refs/heads/master"},
		{"refs/tags/foo", "refs/tags/foo"},
	}

	for _, tt := range tests {
		if got := normalizeRef(tt.branch); got != tt.want {
			t.Errorf("normalizeRef(%v) = %v, want %v", tt.branch, got, tt.want)
		}
	}
}
