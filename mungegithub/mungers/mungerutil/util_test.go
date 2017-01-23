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

package mungerutil

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/google/go-github/github"
)

func makeUser(name string) *github.User {
	return &github.User{
		Login: &name,
	}
}

func TestGetUsers(t *testing.T) {
	if len(GetUsers(nil)) != 0 {
		t.Error("Nil user doesn't make empty Users")
	}
	if len(GetUsers(&github.User{})) != 0 {
		t.Error("Nil user login doesn't make empty Users")
	}
	if len(GetUsers(makeUser("John"), makeUser("John"))) != 1 {
		t.Error("Duplicate users are not removed")
	}
}

func TestUserSetHas(t *testing.T) {
	if GetUsers().Has(makeUser("John")) {
		t.Error("Empty list found someone ...")
	}
	if GetUsers(makeUser("John")).Has(makeUser("Jane")) {
		t.Error("List has found the wrong person")
	}
	if !GetUsers(makeUser("John")).Has(makeUser("John")) {
		t.Error("Failed to find user from the list")
	}
}

func TestUserSetMention(t *testing.T) {
	if !reflect.DeepEqual(
		GetUsers(makeUser("John"), makeUser("Jane")).Mention(),
		GetUsers(makeUser("@John"), makeUser("@Jane"))) {
		t.Error("Failed to mention users")
	}

	if !reflect.DeepEqual(
		GetUsers(makeUser("@John")).Mention().List(),
		GetUsers(makeUser("@John")).List()) {
		fmt.Println(GetUsers(makeUser("@John")).List())
		t.Error("Failed to re-mention users")
	}
}

func TestUserSetJoin(t *testing.T) {
	if GetUsers().Join() != "" {
		t.Error("Empty UserSet doesn't return empty string")
	}
	if GetUsers(makeUser("John")).Join() != "John" {
		t.Error("Single user should be unmodified")
	}
	if GetUsers(makeUser("John"), makeUser("Jane")).Join() != "Jane John" {
		t.Error("UserSet join doesn't join properly")
	}
}

func TestGetIssueUsers(t *testing.T) {
	users := GetIssueUsers(&github.Issue{
		Assignees: []*github.User{makeUser("Jane")},
		Assignee:  makeUser("John"),
		User:      makeUser("Bob"),
	})

	expectedAssignees := []string{"Jane", "John"}
	if !reflect.DeepEqual(users.Assignees.List(), expectedAssignees) {
		t.Errorf("Assignees (%s) doesn't match expected: %s", users.Assignees.List(), expectedAssignees)
	}

	expectedAuthor := []string{"Bob"}
	if !reflect.DeepEqual(users.Author.List(), expectedAuthor) {
		t.Errorf("Author (%s) doesn't match expected: %s", users.Author.List(), expectedAuthor)
	}
}

func TestIssueUsersAll(t *testing.T) {
	users := GetIssueUsers(&github.Issue{
		Assignees: []*github.User{makeUser("Jane")},
		Assignee:  makeUser("John"),
		User:      makeUser("Bob"),
	})

	expected := []string{"Bob", "Jane", "John"}
	if !reflect.DeepEqual(users.AllUsers().List(), expected) {
		t.Errorf("AllUsers (%s) doesn't match expected list: %s", users.AllUsers().List(), expected)
	}
}
