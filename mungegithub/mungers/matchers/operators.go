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

package matchers

import "github.com/google/go-github/github"

// True is a matcher that is always true
type trueMatcher struct{}

// True returns a matcher that is always true
func True() Matcher {
	return trueMatcher{}
}

// MatchEvent returns true no matter what
func (t trueMatcher) MatchEvent(event *github.IssueEvent) bool {
	return true
}

// MatchComment returns true no matter what
func (t trueMatcher) MatchComment(comment *github.IssueComment) bool {
	return true
}

// MatchReviewComment returns true no matter what
func (t trueMatcher) MatchReviewComment(review *github.PullRequestComment) bool {
	return true
}

// falseMatcher is a matcher that is always false
type falseMatcher struct{}

// False returns a matcher that is always false
func False() Matcher {
	return falseMatcher{}
}

// MatchEvent returns false no matter what
func (t falseMatcher) MatchEvent(event *github.IssueEvent) bool {
	return false
}

// MatchComment returns false no matter what
func (t falseMatcher) MatchComment(comment *github.IssueComment) bool {
	return false
}

// MatchReviewComment returns false no matter what
func (t falseMatcher) MatchReviewComment(review *github.PullRequestComment) bool {
	return false
}

// andMatcher makes sure that each match in the list matches (true if empty)
type andMatcher []Matcher

// And returns a matcher that verifies that all matchers match
func And(matchers ...Matcher) Matcher {
	and := andMatcher{}
	for _, matcher := range matchers {
		and = append(and, matcher)
	}
	return and
}

// MatchEvent returns true if all the matchers in the list matche
func (a andMatcher) MatchEvent(event *github.IssueEvent) bool {
	for _, matcher := range a {
		if !matcher.MatchEvent(event) {
			return false
		}
	}
	return true
}

// MatchComment returns true if all the matchers in the list matche
func (a andMatcher) MatchComment(comment *github.IssueComment) bool {
	for _, matcher := range a {
		if !matcher.MatchComment(comment) {
			return false
		}
	}
	return true
}

// MatchReviewComment returns true if all the matchers in the list matche
func (a andMatcher) MatchReviewComment(review *github.PullRequestComment) bool {
	for _, matcher := range a {
		if !matcher.MatchReviewComment(review) {
			return false
		}
	}
	return true
}

// orMatcher makes sure that at least one element in the list matches (false if empty)
type orMatcher []Matcher

// Or returns a matcher that verifies that one of the matcher matches
func Or(matchers ...Matcher) Matcher {
	or := orMatcher{}
	for _, matcher := range matchers {
		or = append(or, matcher)
	}
	return or
}

// MatchEvent returns true if one of the matcher in the list matches
func (o orMatcher) MatchEvent(event *github.IssueEvent) bool {
	for _, matcher := range o {
		if matcher.MatchEvent(event) {
			return true
		}
	}
	return false
}

// MatchComment returns true if one of the matcher in the list matches
func (o orMatcher) MatchComment(comment *github.IssueComment) bool {
	for _, matcher := range o {
		if matcher.MatchComment(comment) {
			return true
		}
	}
	return false
}

// MatchReviewComment returns true no matter what
func (o orMatcher) MatchReviewComment(review *github.PullRequestComment) bool {
	for _, matcher := range o {
		if matcher.MatchReviewComment(review) {
			return true
		}
	}
	return false
}

// Not reverses the effect of the matcher
type notMatcher struct {
	Matcher Matcher
}

func Not(matcher Matcher) Matcher {
	return notMatcher{Matcher: matcher}
}

// MatchEvent returns true if the matcher would return false, and vice-versa
func (n notMatcher) MatchEvent(event *github.IssueEvent) bool {
	return !n.Matcher.MatchEvent(event)
}

// MatchComment returns true if the matcher would return false, and vice-versa
func (n notMatcher) MatchComment(comment *github.IssueComment) bool {
	return !n.Matcher.MatchComment(comment)
}

// MatchReviewComment returns true if the matcher would return false, and vice-versa
func (n notMatcher) MatchReviewComment(review *github.PullRequestComment) bool {
	return !n.Matcher.MatchReviewComment(review)
}
