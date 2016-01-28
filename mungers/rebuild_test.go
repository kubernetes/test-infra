/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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

package mungers

import (
	"testing"

	githubapi "github.com/google/go-github/github"
)

func TestIsRebuild(t *testing.T) {
	tests := []struct {
		value     string
		isRebuild bool
	}{
		{
			value:     "@k8s-bot test this please",
			isRebuild: true,
		},
		{
			value:     "@k8s-bot unit test this please",
			isRebuild: true,
		},
		{
			value:     "@k8s-bot e2e test this please",
			isRebuild: true,
		},
		{
			value:     "@k8s-bot don't test this please",
			isRebuild: false,
		},
		{
			value:     "@other-bot e2e test this please",
			isRebuild: false,
		},
	}
	for _, test := range tests {
		comment := &githubapi.IssueComment{
			Body: &test.value,
		}
		output := isRebuildComment(comment)
		if output != test.isRebuild {
			t.Errorf("expected: %v, saw: %v for %s", test.isRebuild, output, test.value)
		}
	}
}

func TestRebuildMissingIssue(t *testing.T) {
	tests := []struct {
		value         string
		expectMissing bool
	}{
		{
			value:         "@k8s-bot test this please",
			expectMissing: true,
		},
		{
			value:         "@k8s-bot test this please github issue: #123456",
			expectMissing: false,
		},
		{
			value:         "@k8s-bot test this please github issue: #IGNORE",
			expectMissing: false,
		},
		{
			value:         "@k8s-bot test this please github issue #123456",
			expectMissing: false,
		},
		{
			value:         "@k8s-bot test this please github issue #",
			expectMissing: true,
		},
		{
			value:         "@k8s-bot test this please github issue IGNORE",
			expectMissing: true,
		},
		{
			value:         "@k8s-bot test this please flake IGNORE",
			expectMissing: true,
		},
		{
			value:         "@k8s-bot test this please issue: #IGNORE",
			expectMissing: false,
		},
		{
			value:         "@k8s-bot test this please flake: #IGNORE",
			expectMissing: false,
		},
		{
			value:         "@k8s-bot test this please flake: #12345",
			expectMissing: false,
		},
	}
	for _, test := range tests {
		comment := &githubapi.IssueComment{
			Body: &test.value,
		}
		output := rebuildCommentMissingIssueNumber(comment)
		if output != test.expectMissing {
			t.Errorf("expected: %v, saw: %v for %s", test.expectMissing, output, test.value)
		}
	}
}
