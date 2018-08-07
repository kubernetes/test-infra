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

package jira_linker

import (
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/plugins"
)

func TestJiraLink(t *testing.T) {
	const jiraBaseUrl = "http://example.com/jira"

	repoLabelCommentPrefix := "org/repo#5:"

	for _, tc := range []struct {
		name                 string
		title                string
		hasLabels            []string
		labelsAddedOverall   []string
		labelsRemovedOverall []string
		shouldComment        bool
		comment              string
	}{
		{
			name:               "non-jira PR",
			title:              "add sauce to spaghetti",
			hasLabels:          []string{},
			labelsAddedOverall: []string{noJiraLabel},
			shouldComment:      false,
		},
		{
			name:               "basic jira PR",
			title:              "TEST-43 add sauce to spaghetti",
			hasLabels:          []string{},
			labelsAddedOverall: []string{jiraPrefix + "TEST"},
			shouldComment:      true,
			comment:            commentForTicket(jiraLink(jiraBaseUrl, "TEST-43")),
		},
		{
			name:                 "starts with wrong labels",
			title:                "TEST-32 fix every bug",
			hasLabels:            []string{noJiraLabel},
			labelsAddedOverall:   []string{noJiraLabel, jiraPrefix + "TEST"},
			labelsRemovedOverall: []string{noJiraLabel},
			shouldComment:        true,
			comment:              commentForTicket(jiraLink(jiraBaseUrl, "TEST-32")),
		},
		{
			name:               "starts with right labels",
			title:              "TEST-32 fix every bug",
			hasLabels:          []string{jiraPrefix + "TEST"},
			labelsAddedOverall: []string{jiraPrefix + "TEST"},
			shouldComment:      false,
		},
		{
			name:               "two tickets in the title",
			title:              "TEST-32 fix every bug and fixes TEST-303",
			hasLabels:          []string{jiraPrefix + "TEST"},
			labelsAddedOverall: []string{jiraPrefix + "TEST"},
			shouldComment:      false,
		},
	} {
		fc := &fakegithub.FakeClient{
			IssueComments: make(map[int][]github.IssueComment),
		}
		e := &github.PullRequestEvent{
			Action: github.PullRequestActionEdited,
			PullRequest: github.PullRequest{
				Title: tc.title,
			},
			Number: 5,
			Repo:   github.Repo{Owner: github.User{Login: "org"}, Name: "repo"},
		}

		fc.LabelsAdded = []string{}
		for _, label := range tc.hasLabels {
			fc.LabelsAdded = append(fc.LabelsAdded, repoLabelCommentPrefix+label)
		}
		t.Logf("BEFORE\nAdded: %+v,\nRemoved: %+v\nAll: %+v", fc.LabelsAdded, fc.LabelsRemoved, fc.ExistingLabels)

		err := handle(fc, logrus.WithField("plugin", pluginName), plugins.JiraLinker{JiraBaseUrl: jiraBaseUrl}, e)
		if err != nil {
			t.Errorf("For case %s, didn't expect error: %v", tc.name, err)
			continue
		}

		t.Logf("AFTER\nAdded: %+v,\nRemoved: %+v\nAll: %+v", fc.LabelsAdded, fc.LabelsRemoved, fc.ExistingLabels)

		if len(fc.LabelsRemoved) != len(tc.labelsRemovedOverall) {
			t.Errorf("Unexpected labels removed for case %s (got %+v, expected %+v)", tc.name, fc.LabelsRemoved, tc.labelsRemovedOverall)
		} else {
			for i, label := range fc.LabelsRemoved {
				if repoLabelCommentPrefix+tc.labelsRemovedOverall[i] != label {
					t.Errorf("Unexpected labels removed for case %s (got %+v, expected %+v)", tc.name, fc.LabelsRemoved, tc.labelsRemovedOverall)
				}
			}
		}

		if len(fc.LabelsAdded) != len(tc.labelsAddedOverall) {
			t.Errorf("Unexpected labels added for case %s (got %+v, expected %+v)", tc.name, fc.LabelsAdded, tc.labelsAddedOverall)
		} else {
			for i, label := range fc.LabelsAdded {
				if repoLabelCommentPrefix+tc.labelsAddedOverall[i] != label {
					t.Errorf("Unexpected labels added for case %s (got %+v, expected %+v)", tc.name, fc.LabelsAdded, tc.labelsAddedOverall)
				}
			}
		}

		if tc.shouldComment {
			if len(fc.IssueCommentsAdded) == 1 {
				if fc.IssueCommentsAdded[0] != repoLabelCommentPrefix+tc.comment {
					t.Errorf("Unexpected comment added for case %s - expected \"%s\" but got \"%s\"", tc.name, tc.comment, fc.IssueCommentsAdded[0])
				}
			} else {
				t.Errorf("Expected issue comment for case %s but none / too many were added", tc.name)
			}
		} else if len(fc.IssueCommentsAdded) > 0 {
			t.Errorf("Issue comment added unexpectedly for case %s (comments[0]: %s)", tc.name, fc.IssueCommentsAdded[0])
		}
	}
}
