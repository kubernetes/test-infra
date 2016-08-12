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

package comment

import "github.com/google/go-github/github"

// FilteredComments is a list of comments
type FilteredComments []*github.IssueComment

// FilterComments will return the list of matching comments
func FilterComments(comments []*github.IssueComment, matcher Matcher) FilteredComments {
	matches := FilteredComments{}

	for _, comment := range comments {
		if matcher.Match(comment) {
			matches = append(matches, comment)
		}
	}

	return matches
}
