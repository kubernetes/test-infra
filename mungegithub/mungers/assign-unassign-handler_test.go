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

package mungers

import (
	"runtime"
	"testing"

	"k8s.io/contrib/mungegithub/github"
	github_test "k8s.io/contrib/mungegithub/github/testing"
	"k8s.io/kubernetes/pkg/util/sets"

	goGithub "github.com/google/go-github/github"
)

var (
	testUsername = "test-user"
	testUser     = goGithub.User{ID: intPtr(-1), Name: &testUsername}
)

func TestAssignComment(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	tests := []struct {
		testName          string
		comments          []*goGithub.IssueComment
		existingAssignees []*goGithub.User
		newAssignees      sets.String
		removedAssignees  sets.String
	}{
		{
			testName: "Assign cmd should add to existing assignees",
			comments: []*goGithub.IssueComment{
				github_test.IssueComment(1, assignCommand, "user 1", 0),
			},
			existingAssignees: []*goGithub.User{},
			newAssignees:      sets.NewString("user 1"),
			removedAssignees:  sets.NewString(),
		},
		{
			testName: "Assign cmd should not modify existing assignees",
			comments: []*goGithub.IssueComment{
				github_test.IssueComment(1, assignCommand, "user 1", 0),
			},
			existingAssignees: []*goGithub.User{{Login: testUser.Name}},
			newAssignees:      sets.NewString("user 1"),
			removedAssignees:  sets.NewString(),
		},
		{
			testName: "Unassign should remove existing assignee",
			comments: []*goGithub.IssueComment{
				github_test.IssueComment(1, unassignCommand, *testUser.Name, 0),
			},
			existingAssignees: []*goGithub.User{{Login: testUser.Name}},
			newAssignees:      sets.NewString(""),
			removedAssignees:  sets.NewString(*testUser.Name),
		},
		{
			testName: "Reassign by someone else should leave assignees in tact",
			comments: []*goGithub.IssueComment{
				github_test.IssueComment(2, unassignCommand, "NotAUser", 1),
			},
			existingAssignees: []*goGithub.User{{Login: testUser.Name}},
			newAssignees:      sets.NewString(),
			removedAssignees:  sets.NewString(),
		},
	}

	for testNum, test := range tests {
		pr := github.MungeObject{}
		pr.Issue = &goGithub.Issue{}
		pr.Issue.Assignees = test.existingAssignees
		ah := AssignUnassignHandler{}
		assignees, unassignees := ah.getAssigneesAndUnassignees(&pr, test.comments, []*goGithub.CommitFile{}, weightMap{"user 1": 1})
		if assignees.Difference(test.newAssignees).Len() != 0 {
			t.Errorf("For test %v, the expected new assignees did not match the returned new assignees %v %v", testNum, test.newAssignees, assignees)
		}
		if unassignees.Difference(test.removedAssignees).Len() != 0 {
			t.Errorf("Existing Assignees %v", *pr.Issue.Assignees[0])
			t.Errorf("For test %v, the expected removed assignees %v actual removed assignees %v", testNum, test.removedAssignees, unassignees)
		}
	}
}
