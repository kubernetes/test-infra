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

package sql

import (
	"regexp"
	"testing"
)

func TestHasLabel(t *testing.T) {
	issue := Issue{
		Labels: []Label{
			{Name: "priority/P0"},
			{Name: "priority/P1"},
		},
	}
	if len(issue.FindLabels(regexp.MustCompile(`kind/flake`))) != 0 {
		t.Error("regex shouldn't match any item")
	}
	if len(issue.FindLabels(regexp.MustCompile(`^priority/P0$`))) != 1 {
		t.Error("regex should match specific items")
	}
	if len(issue.FindLabels(regexp.MustCompile(`priority/P`))) != 2 {
		t.Error("regex should match every items")
	}
}
