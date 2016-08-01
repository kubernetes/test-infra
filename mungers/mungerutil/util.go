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

package mungerutil

import (
	"strings"

	"k8s.io/kubernetes/pkg/util/sets"

	"github.com/google/go-github/github"
)

// UserSet is a set a of users
type UserSet sets.String

// GetUsers returns a UserSet
func GetUsers(users ...*github.User) UserSet {
	allUsers := sets.String{}

	for _, user := range users {
		if user != nil && user.Login != nil {
			allUsers.Insert(*user.Login)
		}
	}

	return UserSet(allUsers)
}

// Has tells you if the users can be found in the set
func (u UserSet) Has(user ...*github.User) bool {
	return len(u.intersection(GetUsers(user...))) != 0
}

// Mention adds @ to user in the list who don't have it yet
func (u UserSet) Mention() UserSet {
	mentionedUsers := sets.NewString()

	for _, user := range u.List() {
		if !strings.HasPrefix(user, "@") {
			mentionedUsers.Insert("@" + user)
		} else {
			mentionedUsers.Insert(user)
		}
	}

	return UserSet(mentionedUsers)
}

// List makes a list from the set
func (u UserSet) List() []string {
	return sets.String(u).List()
}

// Join joins each users into a single string
func (u UserSet) Join() string {
	return strings.Join(u.List(), " ")
}

func (u UserSet) union(o UserSet) UserSet {
	return UserSet(sets.String(u).Union(sets.String(o)))
}

func (u UserSet) intersection(o UserSet) UserSet {
	return UserSet(sets.String(u).Intersection(sets.String(o)))
}

// IssueUsers tracks Users involved in a github Issue
type IssueUsers struct {
	Assignees UserSet
	Author    UserSet // This will usually be one or zero
}

// GetIssueUsers creates a new IssueUsers object from an issue's fields
func GetIssueUsers(issue *github.Issue) *IssueUsers {
	return &IssueUsers{
		Assignees: GetUsers(issue.Assignees...).union(GetUsers(issue.Assignee)),
		Author:    GetUsers(issue.User),
	}
}

// AllUsers return a list of unique users (both assignees and author)
func (u *IssueUsers) AllUsers() UserSet {
	return u.Assignees.union(u.Author)
}
