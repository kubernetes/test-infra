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

package event

import "github.com/google/go-github/github"

// True is a matcher that is always true
type True struct{}

// Match returns true no matter what
func (t True) Match(event *github.IssueEvent) bool {
	return true
}

// False is a matcher that is always false
type False struct{}

// Match returns false no matter what
func (t False) Match(event *github.IssueEvent) bool {
	return false
}

// And makes sure that each match in the list matches (true if empty)
type And []Matcher

// Match returns true if all the matcher in the list matches
func (a And) Match(event *github.IssueEvent) bool {
	for _, matcher := range []Matcher(a) {
		if !matcher.Match(event) {
			return false
		}
	}
	return true
}

// Or makes sure that at least one element in the list matches (false if empty)
type Or []Matcher

// Match returns true if one of the matcher in the list matches
func (o Or) Match(event *github.IssueEvent) bool {
	for _, matcher := range []Matcher(o) {
		if matcher.Match(event) {
			return true
		}
	}
	return false
}

// Not reverses the effect of the matcher
type Not struct {
	Matcher Matcher
}

// Match returns true if the matcher would return false, and vice-versa
func (n Not) Match(event *github.IssueEvent) bool {
	return !n.Matcher.Match(event)
}
