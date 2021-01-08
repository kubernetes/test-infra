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

// Package project implements the `/project` command which allows members of the project
// maintainers team to specify a project to be applied to an Issue or PR.
package project

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/plugins"
)

func TestProjectCommand(t *testing.T) {
	projectColumnsMap := map[string][]github.ProjectColumn{
		"0.0.0": {
			github.ProjectColumn{
				Name: "To do",
				ID:   00000,
			},
			github.ProjectColumn{
				Name: "Backlog",
				ID:   00001,
			},
		},
		"0.1.0": {
			github.ProjectColumn{
				Name: "To do",
				ID:   00002,
			},
			github.ProjectColumn{
				Name: "Backlog",
				ID:   00003,
			},
		},
	}
	repoProjects := map[string][]github.Project{
		"kubernetes/*": {
			github.Project{
				Name: "0.0.0",
				ID:   000,
			},
		},
		"kubernetes/kubernetes": {
			github.Project{
				Name: "0.1.0",
				ID:   010,
			},
		},
	}
	// Maps github project name to maps of column IDs to column string
	columnIDMap := map[string]map[int]string{
		"0.0.0": {
			00000: "To do",
			00001: "Backlog",
		},
		"0.1.0": {
			00002: "To do",
			00003: "Backlog",
		},
	}

	projectConfig := plugins.ProjectConfig{
		// The team ID is set to 0 (or 42) in order to match the teams returned by FakeClient's method ListTeamMembers
		Orgs: map[string]plugins.ProjectOrgConfig{
			"kubernetes": {
				MaintainerTeamID: 0,
				ProjectColumnMap: map[string]string{
					"0.0.0": "Backlog",
				},
				Repos: map[string]plugins.ProjectRepoConfig{
					"kubernetes": {
						MaintainerTeamID: 42,
						ProjectColumnMap: map[string]string{
							"0.1.0": "To do",
						},
					},
					"community": {
						MaintainerTeamID: 0,
						ProjectColumnMap: map[string]string{
							"0.1.0": "does not exist column",
						},
					},
				},
			},
		},
	}

	fakeClient := &fakegithub.FakeClient{
		RepoProjects:      repoProjects,
		ProjectColumnsMap: projectColumnsMap,
		ColumnIDMap:       columnIDMap,
		IssueComments:     make(map[int][]github.IssueComment),
	}
	botUser, err := fakeClient.BotUser()
	if err != nil {
		t.Errorf(err.Error())
	}

	type testCase struct {
		name            string
		action          github.GenericCommentEventAction
		noAction        bool
		body            string
		repo            string
		org             string
		commenter       string
		previousProject string
		previousColumn  string
		expectedProject string
		expectedColumn  string
		expectedComment string
	}

	testcases := []testCase{
		{
			name:            "Setting project and column with valid values, but commenter does not belong to the project maintainer team",
			action:          github.GenericCommentActionCreated,
			body:            "/project 0.0.0 To do",
			repo:            "kubernetes",
			org:             "kubernetes",
			commenter:       "random-user",
			previousProject: "",
			previousColumn:  "",
			expectedProject: "",
			expectedColumn:  "",
			expectedComment: "@random-user: " + fmt.Sprintf(notATeamMemberMsg, "kubernetes", "kubernetes", "kubernetes", "kubernetes"),
		},
		{
			name:            "Setting project and column with valid values; project card does not currently exist for this issue/PR in the project",
			action:          github.GenericCommentActionCreated,
			body:            "/project 0.0.0 To do",
			repo:            "kubernetes",
			org:             "kubernetes",
			commenter:       "sig-lead",
			previousProject: "",
			previousColumn:  "",
			expectedProject: "0.0.0",
			expectedColumn:  "To do",
		},
		{
			name:            "Setting project and column with valid values; project card already exist for this issue/PR in the project, but the project card is under a different column",
			action:          github.GenericCommentActionCreated,
			body:            "/project 0.0.0 To do",
			repo:            "kubernetes",
			org:             "kubernetes",
			commenter:       "sig-lead",
			previousProject: "",
			previousColumn:  "",
			expectedProject: "0.0.0",
			expectedColumn:  "To do",
		},
		{
			name:            "Setting project without column value; the project specified exists on the repo level; the default column is set on the project and it exists on the project",
			action:          github.GenericCommentActionCreated,
			body:            "/project 0.1.0",
			repo:            "kubernetes",
			org:             "kubernetes",
			commenter:       "sig-lead",
			previousProject: "0.0.0",
			previousColumn:  "Backlog",
			expectedProject: "0.1.0",
			expectedColumn:  "To do",
		},
		{
			name:            "Setting project without column value; the project specified exists on the org level; the default column is set on the project and it exists on the project",
			action:          github.GenericCommentActionCreated,
			body:            "/project 0.0.0",
			repo:            "kubernetes",
			org:             "kubernetes",
			commenter:       "sig-lead",
			previousProject: "",
			previousColumn:  "",
			expectedProject: "0.0.0",
			expectedColumn:  "Backlog",
		},
		{
			name:            "Setting project without column value; the default column is set on the project but it does not exist on the project",
			action:          github.GenericCommentActionCreated,
			body:            "/project 0.1.0",
			repo:            "community",
			org:             "kubernetes",
			commenter:       "default-sig-lead",
			previousProject: "0.0.0",
			previousColumn:  "Backlog",
			expectedProject: "0.0.0",
			expectedColumn:  "Backlog",
			expectedComment: "@default-sig-lead: " + fmt.Sprintf(invalidColumn, "0.1.0", []string{"To do", "Backlog"}),
		},
		{
			name:            "Setting project with invalid column value; an error will be returned",
			action:          github.GenericCommentActionCreated,
			body:            "/project 0.1.0 Random 2",
			repo:            "kubernetes",
			org:             "kubernetes",
			commenter:       "sig-lead",
			previousProject: "",
			previousColumn:  "",
			expectedProject: "",
			expectedColumn:  "",
			expectedComment: "@sig-lead: " + fmt.Sprintf(invalidColumn, "0.1.0", []string{"To do", "Backlog"}),
		},
		{
			name:            "Clearing project for a issue/PR; the project name provided is valid",
			action:          github.GenericCommentActionCreated,
			body:            "/project clear 0.0.0",
			repo:            "kubernetes",
			org:             "kubernetes",
			commenter:       "sig-lead",
			previousProject: "0.0.0",
			previousColumn:  "Backlog",
			expectedProject: "0.0.0",
			expectedColumn:  "Backlog",
		},
		{
			name:            "Setting project with invalid project name",
			action:          github.GenericCommentActionCreated,
			body:            "/project invalidprojectname",
			repo:            "community",
			org:             "kubernetes",
			commenter:       "default-sig-lead",
			previousProject: "",
			previousColumn:  "",
			expectedProject: "",
			expectedColumn:  "",
			expectedComment: "@default-sig-lead: " + fmt.Sprintf(invalidProject, "`0.0.0`, `0.1.0`"),
		},
		{
			name:            "Clearing project for a issue/PR; the project name provided is invalid",
			action:          github.GenericCommentActionCreated,
			body:            "/project clear invalidprojectname",
			repo:            "kubernetes",
			org:             "kubernetes",
			commenter:       "sig-lead",
			previousProject: "0.1.0",
			previousColumn:  "To do",
			expectedProject: "0.1.0",
			expectedColumn:  "To do",
			expectedComment: "@sig-lead: " + fmt.Sprintf(invalidProject, "`0.0.0`, `0.1.0`"),
		},
		{
			name:            "Clearing project for a issue/PR; the project does not contain the card",
			action:          github.GenericCommentActionCreated,
			body:            "/project clear 0.1.0",
			repo:            "kubernetes",
			org:             "kubernetes",
			commenter:       "sig-lead",
			previousProject: "0.1.0",
			previousColumn:  "To do",
			expectedProject: "0.1.0",
			expectedColumn:  "To do",
			expectedComment: "@sig-lead: " + fmt.Sprintf(failedClearingProjectMsg, "0.1.0", "1"),
		},
		{
			name:            "No action on events that are not new comments",
			action:          github.GenericCommentActionEdited,
			body:            "/project 0.0.0 To do",
			repo:            "kubernetes",
			org:             "kubernetes",
			commenter:       "sig-lead",
			previousProject: "",
			previousColumn:  "",
			expectedProject: "",
			expectedColumn:  "",
			noAction:        true,
		},
		{
			name:            "No action on bot comments",
			action:          github.GenericCommentActionCreated,
			body:            "/project 0.0.0 To do",
			repo:            "kubernetes",
			org:             "kubernetes",
			commenter:       botUser.Login,
			previousProject: "",
			previousColumn:  "",
			expectedProject: "",
			expectedColumn:  "",
			noAction:        true,
		},
		{
			name:            "No action on non-matching comments",
			action:          github.GenericCommentActionCreated,
			body:            "random comment",
			repo:            "kubernetes",
			org:             "kubernetes",
			commenter:       "sig-lead",
			previousProject: "",
			previousColumn:  "",
			expectedProject: "",
			expectedColumn:  "",
			noAction:        true,
		},
	}

	prevCommentCount := 0
	for _, tc := range testcases {
		fakeClient.Project = tc.previousProject
		fakeClient.Column = tc.previousColumn
		fakeClient.ColumnCardsMap = map[int][]github.ProjectCard{}

		e := &github.GenericCommentEvent{
			Action:       tc.action,
			Body:         tc.body,
			Number:       1,
			IssueHTMLURL: "1",
			Repo:         github.Repo{Owner: github.User{Login: tc.org}, Name: tc.repo},
			User:         github.User{Login: tc.commenter},
		}
		if err := handle(fakeClient, logrus.WithField("plugin", pluginName), e, projectConfig); err != nil {
			t.Errorf("(%s): Unexpected error from handle: %v.", tc.name, err)
			continue
		}
		if fakeClient.Project != tc.expectedProject {
			t.Errorf("(%s): Unexpected project %s but got %s", tc.name, tc.expectedProject, fakeClient.Project)
		}
		if fakeClient.Column != tc.expectedColumn {
			t.Errorf("(%s): Unexpected column %s but got %s", tc.name, tc.expectedColumn, fakeClient.Column)
		}
		issueComments := fakeClient.IssueComments[e.Number]
		if tc.expectedComment != "" {
			actualComment := issueComments[len(issueComments)-1].Body
			// Only check for substring because the actual comment contains a lot of extra stuff
			if !strings.Contains(actualComment, tc.expectedComment) {
				t.Errorf("(%s): Unexpected comment\n%s\nbut got\n%s", tc.name, tc.expectedComment, actualComment)
			}
		}
		if tc.noAction {
			if len(issueComments) != prevCommentCount {
				t.Errorf("(%s): No new comment should be created", tc.name)
			}
		}
		prevCommentCount = len(issueComments)
	}
}

func TestParseCommand(t *testing.T) {
	var testcases = []struct {
		hasMatches      bool
		command         string
		proposedProject string
		proposedColumn  string
		shouldClear     bool
	}{
		{
			hasMatches:      true,
			command:         "/project 0.0.0 To do",
			proposedProject: "0.0.0",
			proposedColumn:  "To do",
			shouldClear:     false,
		},
		{
			hasMatches:      true,
			command:         "/project 0.0.0 Backlog",
			proposedProject: "0.0.0",
			proposedColumn:  "Backlog",
			shouldClear:     false,
		},
		{
			hasMatches:      true,
			command:         "/project clear 0.0.0",
			proposedProject: "0.0.0",
			proposedColumn:  "",
			shouldClear:     true,
		},
		{
			hasMatches:      true,
			command:         "/project clear 0.0.0 Backlog",
			proposedProject: "0.0.0",
			proposedColumn:  "Backlog",
			shouldClear:     true,
		},
		{
			hasMatches:      true,
			command:         "/project clear",
			proposedProject: "",
			proposedColumn:  "",
			shouldClear:     false,
		},
		{
			hasMatches:      true,
			command:         "/project 0.0.0",
			proposedProject: "0.0.0",
			proposedColumn:  "",
			shouldClear:     false,
		},
		{
			hasMatches:      true,
			command:         "/project '0.0.0'",
			proposedProject: "0.0.0",
			proposedColumn:  "",
			shouldClear:     false,
		},
		{
			hasMatches:      true,
			command:         "/project \"0.0.0\"",
			proposedProject: "0.0.0",
			proposedColumn:  "",
			shouldClear:     false,
		},
		{
			hasMatches:      true,
			command:         "/project '0.0.0' To do",
			proposedProject: "0.0.0",
			proposedColumn:  "To do",
			shouldClear:     false,
		},
		{
			hasMatches:      true,
			command:         "/project '0.0.0' \"To do\"",
			proposedProject: "0.0.0",
			proposedColumn:  "To do",
			shouldClear:     false,
		},
		{
			hasMatches:      true,
			command:         "/project 'something 0.0.0' \"To do\"",
			proposedProject: "something 0.0.0",
			proposedColumn:  "To do",
			shouldClear:     false,
		},
		{
			hasMatches:      true,
			command:         "/project clear '0.0.0' \"To do\"",
			proposedProject: "0.0.0",
			proposedColumn:  "To do",
			shouldClear:     true,
		},
		{
			hasMatches:      true,
			command:         "/project clear 'something 0.0.0' \"To do\"",
			proposedProject: "something 0.0.0",
			proposedColumn:  "To do",
			shouldClear:     true,
		},
		{
			hasMatches:      false,
			command:         "/project",
			proposedProject: "",
			proposedColumn:  "",
			shouldClear:     false,
		},
		{
			hasMatches: false,
			command:    "random comment",
		},
	}

	for _, test := range testcases {
		matches := projectRegex.FindStringSubmatch(test.command)
		if !test.hasMatches {
			if len(matches) > 0 {
				t.Errorf("For command %s, project command regex should not match", test.command)
			}
			continue
		}
		proposedProject, proposedColumn, shouldClear, _ := processCommand(matches[1])
		if proposedProject != test.proposedProject ||
			proposedColumn != test.proposedColumn ||
			shouldClear != test.shouldClear {
			t.Errorf("\nFor command %s, expected\n  proposedProject = %s\n  proposedColumn = %s\n  shouldClear = %t\nbut got\n  proposedProject = %s\n  proposedColumn = %s\n  shouldClear = %t\n", test.command, test.proposedProject, test.proposedColumn, test.shouldClear, proposedProject, proposedColumn, shouldClear)
		}
	}
}

func TestGetProjectConfigs(t *testing.T) {
	var testcases = []struct {
		org                      string
		repo                     string
		expectedMaintainerTeamID int
	}{
		{
			org:                      "kubernetes",
			repo:                     "kubernetes",
			expectedMaintainerTeamID: 42,
		},
		{
			org:                      "kubernetes",
			repo:                     "community",
			expectedMaintainerTeamID: 11,
		},
		{
			org:                      "kubernetes-sigs",
			repo:                     "kubespray",
			expectedMaintainerTeamID: 10,
		},
		{
			org:                      "kubernetes-sigs",
			repo:                     "kind",
			expectedMaintainerTeamID: 0,
		},
	}
	projectConfig := plugins.ProjectConfig{
		Orgs: map[string]plugins.ProjectOrgConfig{
			"kubernetes": {
				MaintainerTeamID: 11,
				Repos: map[string]plugins.ProjectRepoConfig{
					"kubernetes": {
						MaintainerTeamID: 42,
					},
				},
			},
			"kubeflow": {
				MaintainerTeamID: 20,
			},
			"kubernetes-sigs": {
				Repos: map[string]plugins.ProjectRepoConfig{
					"kubespray": {
						MaintainerTeamID: 10,
					},
					"kind": {},
				},
			},
		},
	}

	for _, tc := range testcases {
		maintainerTeamID := projectConfig.GetMaintainerTeam(tc.org, tc.repo)
		if maintainerTeamID != tc.expectedMaintainerTeamID {
			t.Errorf("\nFor %s/%s, expected maintainer team ID %d but got ID %d", tc.org, tc.repo, tc.expectedMaintainerTeamID, maintainerTeamID)
		}
	}
}
