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

package comment

import (
	"time"
)

// FilteredComments is a list of comments
type FilteredComments []*Comment

// GetLast returns the last comment in a series of comments
func (f FilteredComments) GetLast() *Comment {
	if f.Empty() {
		return nil
	}
	return f[len(f)-1]
}

// Empty Checks to see if the list of comments is empty
func (f FilteredComments) Empty() bool {
	return len(f) == 0
}

// FilterComments will return the list of matching comments
func FilterComments(comments []*Comment, matcher Matcher) FilteredComments {
	matches := FilteredComments{}

	for _, comment := range comments {
		if matcher.Match(comment) {
			matches = append(matches, comment)
		}
	}

	return matches
}

// LastComment returns the creation date of the last comment that matches. Or deflt if there is no such comment.
func LastComment(comments []*Comment, matcher Matcher, deflt *time.Time) *time.Time {
	matches := FilterComments(comments, matcher)
	if matches.Empty() {
		return deflt
	}
	return matches.GetLast().CreatedAt
}
