/*
Copyright 2019 The Kubernetes Authors.

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

package gitattributes

import (
	"testing"
)

func TestParsePattern(t *testing.T) {
	var cases = []struct {
		name        string
		pattern     string
		expectError bool
	}{
		{
			name:        "file",
			pattern:     `abc.json`,
			expectError: false,
		},
		{
			name:        "negative file",
			pattern:     `!abc.json`,
			expectError: true,
		},
		{
			name:        "directory",
			pattern:     `abc/`,
			expectError: true,
		},
		{
			name:        "directory recursive",
			pattern:     `abc/**`,
			expectError: false,
		},
		{
			name:        "glob",
			pattern:     `a/*/c`,
			expectError: false,
		},
		{
			name:        "needs trim",
			pattern:     `abc.json `,
			expectError: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parsePattern(c.pattern); err != nil && !c.expectError {
				t.Fatalf("load error: %v", err)
			}
		})
	}
}

func TestMatch(t *testing.T) {
	var cases = []struct {
		name        string
		pattern     string
		path        string
		shouldMatch bool
	}{
		{
			name:        "file",
			pattern:     `abc.json`,
			path:        "abc.json",
			shouldMatch: true,
		},
		{
			name:        "file in dir",
			pattern:     `abc.json`,
			path:        "a/abc.json",
			shouldMatch: true,
		},
		{
			name:        "file fail",
			pattern:     `abc.js`,
			path:        "abc.json",
			shouldMatch: false,
		},
		{
			name:        "glob",
			pattern:     `*.json`,
			path:        "abc.json",
			shouldMatch: true,
		},
		{
			name:        "glob in dir",
			pattern:     `a/*.json`,
			path:        "a/abc.json",
			shouldMatch: true,
		},
		{
			name:        "glob dir",
			pattern:     `*/abc.json`,
			path:        "a/abc.json",
			shouldMatch: true,
		},
		{
			name:        "glob dir fail",
			pattern:     `*/abc.json`,
			path:        "a/b/abc.json",
			shouldMatch: false,
		},
		{
			name:        "recursive file",
			pattern:     `**/abc.json`,
			path:        "a/b/abc.json",
			shouldMatch: true,
		},
		{
			name:        "recursive file fail",
			pattern:     `**/abc.js`,
			path:        "a/b/abc.json",
			shouldMatch: false,
		},
		{
			name:        "recursive dir",
			pattern:     `a/**`,
			path:        "a/b/abc.json",
			shouldMatch: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, _ := parsePattern(c.pattern)
			if p.Match(c.path) != c.shouldMatch {
				t.Fatalf("mismatch")
			}
		})
	}
}
