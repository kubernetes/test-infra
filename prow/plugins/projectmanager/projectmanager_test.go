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

package projectmanager

import (
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/plugins"
)

func TestHandlePR(t *testing.T) {
	columnID := new(int)
	*columnID = 1
	columnIDMap := map[string]map[int]string{
		"testProject": {
			00001: "testColumn",
			00002: "testColumn2",
		},
		"testProject2": {
			00003: "testColumn",
			00004: "testColumn2",
		},
	}
	// all issues/PRs will be automatically be populated by these labels depending on which org they belong
	labels := []string{"otherOrg/someRepo#1:label1", "otherOrg/someRepo#1:label2", "otherOrg/someRepo#2:label1", "otherOrg2/someRepo#1:label1"}
	cases := []struct {
		name                string
		gc                  *fakegithub.FakeClient
		projectManager      plugins.ProjectManager
		pe                  github.PullRequestEvent
		expectedColumnCards map[int][]github.ProjectCard
		expectedError       error
	}{
		{
			name: "add pull request to project column with no columnID",
			gc: &fakegithub.FakeClient{
				IssueLabelsAdded:   labels,
				IssueLabelsRemoved: []string{},
				RepoProjects: map[string][]github.Project{
					"testOrg/testRepo": {
						{
							Name: "testProject",
							ID:   1,
						},
					},
				},
				OrgProjects: map[string][]github.Project{},
				ProjectColumnsMap: map[string][]github.ProjectColumn{
					"testProject": {
						{
							Name: "testColumn2",
							ID:   2,
						},
						{
							Name: "testColumn",
							ID:   1,
						},
					},
				},
				ColumnCardsMap: map[int][]github.ProjectCard{},
				ColumnIDMap:    columnIDMap,
			},
			projectManager: plugins.ProjectManager{
				OrgRepos: map[string]plugins.ManagedOrgRepo{
					"testOrg/testRepo": {
						Projects: map[string]plugins.ManagedProject{
							"testProject": {
								Columns: []plugins.ManagedColumn{
									{
										Name:   "testColumn2",
										State:  "open",
										Org:    "otherOrg",
										Labels: []string{"label1", "label2"},
									},
								},
							},
						},
					},
				},
			},
			// pe belongs to otherOrg/somerepo and has labels 'label1' and 'label2'
			// this pe should land in testproject under column testColumn2
			pe: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				Number: 1,
				PullRequest: github.PullRequest{
					ID:    2,
					State: "open",
				},
				Repo: github.Repo{
					Name: "someRepo",
					Owner: github.User{
						Login: "otherOrg",
					},
				},
			},
			expectedColumnCards: map[int][]github.ProjectCard{
				2: {
					{
						ContentID:   2,
						ContentType: "PullRequest",
					},
				},
			},
		},
		{
			name: "add pull request to project column with only columnID",
			gc: &fakegithub.FakeClient{
				IssueLabelsAdded:   labels,
				IssueLabelsRemoved: []string{},
				// Note that repoProjects and ProjectColumns are empty so the columnID cannot be looked up using repo, project and column names
				// This means if the project_manager plugin is not using the columnID specified in the config this test will fail
				RepoProjects: map[string][]github.Project{
					"testOrg/testRepo": {
						{
							Name: "testProject",
							ID:   1,
						},
					},
				},
				OrgProjects: map[string][]github.Project{},
				ProjectColumnsMap: map[string][]github.ProjectColumn{
					"testProject": {
						{
							Name: "testColumn2",
							ID:   2,
						},
						{
							Name: "testColumn",
							ID:   1,
						},
					},
				},
				ColumnCardsMap: map[int][]github.ProjectCard{},
				ColumnIDMap:    columnIDMap,
			},
			projectManager: plugins.ProjectManager{
				OrgRepos: map[string]plugins.ManagedOrgRepo{
					"testOrg/testRepo": {
						Projects: map[string]plugins.ManagedProject{
							"testProject": {
								Columns: []plugins.ManagedColumn{
									{
										ID:     columnID,
										State:  "open",
										Org:    "otherOrg",
										Labels: []string{"label1", "label2"},
									},
								},
							},
						},
					},
				},
			},
			// pe belongs to otherOrg/somerepo and has labels 'label1' and 'label2'
			// this pe should land in testproject under column id 1
			pe: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				Number: 1,
				PullRequest: github.PullRequest{
					ID:    2,
					State: "open",
				},
				Repo: github.Repo{
					Name: "someRepo",
					Owner: github.User{
						Login: "otherOrg",
					},
				},
			},
			expectedColumnCards: map[int][]github.ProjectCard{
				1: {
					{
						ContentID:   2,
						ContentType: "PullRequest",
					},
				},
			},
		},
		{
			name: "don't add pull request with incorrect column name",
			gc: &fakegithub.FakeClient{
				IssueLabelsAdded:   labels,
				IssueLabelsRemoved: []string{},
				RepoProjects: map[string][]github.Project{
					"testOrg/testRepo": {
						{
							Name: "testProject",
							ID:   1,
						},
					},
				},
				OrgProjects: map[string][]github.Project{},
				ProjectColumnsMap: map[string][]github.ProjectColumn{
					"testProject": {
						{
							Name: "testColumn2",
							ID:   2,
						},
					},
					"testProject2": {
						{
							Name: "testColumn2",
							ID:   4,
						},
						{
							Name: "testColumn",
							ID:   3,
						},
					},
				},
				ColumnCardsMap: map[int][]github.ProjectCard{},
			},
			projectManager: plugins.ProjectManager{
				OrgRepos: map[string]plugins.ManagedOrgRepo{
					"testOrg/testRepo": {
						Projects: map[string]plugins.ManagedProject{
							"testProject": {
								Columns: []plugins.ManagedColumn{
									{
										Name:   "testColumn",
										State:  "open",
										Org:    "otherOrg",
										Labels: []string{"label1", "label2"},
									},
								},
							},
						},
					},
				},
			},
			// pe belongs to otherOrg/somerepo and has labels 'label1' and 'label2'
			// this pe cannot be added to a non-existent column 'testColumn'
			pe: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				Number: 1,
				PullRequest: github.PullRequest{
					ID:    2,
					State: "open",
				},
				Repo: github.Repo{
					Name: "someRepo",
					Owner: github.User{
						Login: "otherOrg",
					},
				},
			},
			expectedColumnCards: map[int][]github.ProjectCard{},
		},
		{
			name: "don't add pull request if all the labels do not match",
			gc: &fakegithub.FakeClient{
				IssueLabelsAdded:   labels,
				IssueLabelsRemoved: []string{},
				RepoProjects: map[string][]github.Project{
					"testOrg/testRepo": {
						{
							Name: "testProject",
							ID:   1,
						},
					},
				},
				OrgProjects: map[string][]github.Project{},
				ProjectColumnsMap: map[string][]github.ProjectColumn{
					"testProject": {
						{
							Name: "testColumn2",
							ID:   2,
						},
					},
					"testProject2": {
						{
							Name: "testColumn2",
							ID:   4,
						},
						{
							Name: "testColumn",
							ID:   3,
						},
					},
				},
				ColumnCardsMap: map[int][]github.ProjectCard{},
			},
			projectManager: plugins.ProjectManager{
				OrgRepos: map[string]plugins.ManagedOrgRepo{
					"testOrg/testRepo": {
						Projects: map[string]plugins.ManagedProject{
							"testProject": {
								Columns: []plugins.ManagedColumn{
									{
										Name:   "testColumn",
										State:  "open",
										Org:    "otherOrg",
										Labels: []string{"label1", "label2", "label3"},
									},
								},
							},
						},
					},
				},
			},
			// pe belongs to otherOrg/somerepo and has labels 'label1' and 'label2'
			// this pe cannot be added to a non-existent column 'testColumn'
			pe: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				Number: 1,
				PullRequest: github.PullRequest{
					ID:    2,
					State: "open",
				},
				Repo: github.Repo{
					Name: "someRepo",
					Owner: github.User{
						Login: "otherOrg",
					},
				},
			},
			expectedColumnCards: map[int][]github.ProjectCard{},
		},
		{
			name: "add pull request using column name in multiple repos",
			gc: &fakegithub.FakeClient{
				IssueLabelsAdded:   labels,
				IssueLabelsRemoved: []string{},
				RepoProjects: map[string][]github.Project{
					"testOrg/testRepo": {
						{
							Name: "testProject",
							ID:   1,
						},
					},
					"testOrg/testRepo2": {
						{
							Name: "testProject2",
							ID:   2,
						},
					},
				},
				OrgProjects: map[string][]github.Project{},
				ProjectColumnsMap: map[string][]github.ProjectColumn{
					"testProject": {
						{
							Name: "testColumn2",
							ID:   2,
						},
						{
							Name: "testColumn",
							ID:   1,
						},
					},
					"testProject2": {
						{
							Name: "testColumn2",
							ID:   4,
						},
						{
							Name: "testColumn",
							ID:   3,
						},
					},
				},
				ColumnCardsMap: map[int][]github.ProjectCard{},
				ColumnIDMap:    columnIDMap,
			},
			projectManager: plugins.ProjectManager{
				OrgRepos: map[string]plugins.ManagedOrgRepo{
					"testOrg/testRepo": {
						Projects: map[string]plugins.ManagedProject{
							"testProject": {
								Columns: []plugins.ManagedColumn{
									{
										Name:   "testColumn",
										State:  "open",
										Org:    "otherOrg2",
										Labels: []string{"label1", "label2"},
									},
								},
							},
						},
					},
					"testOrg/testRepo2": {
						Projects: map[string]plugins.ManagedProject{
							"testProject2": {
								Columns: []plugins.ManagedColumn{
									{
										Name:   "testColumn2",
										State:  "open",
										Org:    "otherOrg2",
										Labels: []string{"label1", "label2"},
									},
								},
							},
						},
					},
				},
			},
			// pe belongs to otherOrg2/someRepo and hence has labels 'label1'
			// this pe should be added to testProject.testColumn and testProject2.testColumn2
			pe: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				Number: 1,
				PullRequest: github.PullRequest{
					ID:    2,
					State: "open",
				},
				Repo: github.Repo{
					Name: "someRepo",
					Owner: github.User{
						Login: "otherOrg2",
					},
				},
			},
			expectedColumnCards: map[int][]github.ProjectCard{
				4: {
					{
						ContentID:   2,
						ContentType: "PullRequest",
					},
				},
				1: {
					{
						ContentID:   2,
						ContentType: "PullRequest",
					},
				},
			},
		},
		{
			name: "add pull request using column name in multirepo to multiple projects",
			gc: &fakegithub.FakeClient{
				IssueLabelsAdded:   labels,
				IssueLabelsRemoved: []string{},
				RepoProjects: map[string][]github.Project{
					"testOrg/testRepo": {
						{
							Name: "testProject",
							ID:   1,
						},
					},
					"testOrg/testRepo2": {
						{
							Name: "testProject2",
							ID:   2,
						},
					},
				},
				OrgProjects: map[string][]github.Project{},
				ProjectColumnsMap: map[string][]github.ProjectColumn{
					"testProject": {
						{
							Name: "testColumn2",
							ID:   2,
						},
						{
							Name: "testColumn",
							ID:   1,
						},
					},
					"testProject2": {
						{
							Name: "testColumn2",
							ID:   4,
						},
						{
							Name: "testColumn",
							ID:   3,
						},
					},
				},
				ColumnCardsMap: map[int][]github.ProjectCard{},
				ColumnIDMap:    columnIDMap,
			},
			projectManager: plugins.ProjectManager{
				OrgRepos: map[string]plugins.ManagedOrgRepo{
					"testOrg/testRepo": {
						Projects: map[string]plugins.ManagedProject{
							"testProject": {
								Columns: []plugins.ManagedColumn{
									{
										Name:   "testColumn",
										State:  "open",
										Org:    "otherOrg",
										Labels: []string{"label1"},
									},
								},
							},
						},
					},
					"testOrg/testRepo2": {
						Projects: map[string]plugins.ManagedProject{
							"testProject2": {
								Columns: []plugins.ManagedColumn{
									{
										Name:   "testColumn2",
										State:  "open",
										Org:    "otherOrg",
										Labels: []string{"label1", "label2"},
									},
								},
							},
						},
					},
				},
			},
			// pe belongs to otherOrg/someRepo and hence has labels 'label1' and 'label2'
			// this pe should be added to testProject.testColumn in testRepo and testProject2.testColumn2 in testRepo2
			pe: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				Number: 1,
				PullRequest: github.PullRequest{
					ID:    2,
					State: "open",
				},
				Repo: github.Repo{
					Name: "someRepo",
					Owner: github.User{
						Login: "otherOrg",
					},
				},
			},
			expectedColumnCards: map[int][]github.ProjectCard{
				4: {
					{
						ContentID:   2,
						ContentType: "PullRequest",
					},
				},
				1: {
					{
						ContentID:   2,
						ContentType: "PullRequest",
					},
				},
			},
		},
		{
			name: "add pull request to multiple columns in a project, should realize conflict",
			gc: &fakegithub.FakeClient{
				IssueLabelsAdded:   labels,
				IssueLabelsRemoved: []string{},
				RepoProjects: map[string][]github.Project{
					"testOrg/testRepo": {
						{
							Name: "testProject",
							ID:   1,
						},
					},
				},
				OrgProjects: map[string][]github.Project{},
				ProjectColumnsMap: map[string][]github.ProjectColumn{
					"testProject": {
						{
							Name: "testColumn2",
							ID:   2,
						},
						{
							Name: "testColumn",
							ID:   1,
						},
					},
				},
				ColumnCardsMap: map[int][]github.ProjectCard{1: {
					{
						ContentID:   2,
						ContentURL:  "https://api.github.com/repos/otherOrg/someRepo/issues/1",
						ContentType: "PullRequest",
					},
				}},
				ColumnIDMap: columnIDMap,
			},
			projectManager: plugins.ProjectManager{
				OrgRepos: map[string]plugins.ManagedOrgRepo{
					"testOrg/testRepo": {
						Projects: map[string]plugins.ManagedProject{
							"testProject": {
								Columns: []plugins.ManagedColumn{
									{
										Name:   "testColumn",
										State:  "open",
										Org:    "otherOrg",
										Labels: []string{"label1", "label2"},
									},
									{
										Name:   "testColumn2",
										State:  "open",
										Org:    "otherOrg",
										Labels: []string{"label1", "label2"},
									},
								},
							},
						},
					},
				},
			},
			// pe belongs to otherOrg/someRepo and hence has labels 'label1' and 'label2'
			// this pe should be added to testProject.testColumn and then match occurs to
			// testProject.testColumn2 which will be ignored as the card is already in the project
			pe: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				Number: 1,
				PullRequest: github.PullRequest{
					ID:    2,
					State: "open",
				},
				Repo: github.Repo{
					Name: "someRepo",
					Owner: github.User{
						Login: "otherOrg",
					},
				},
			},
			expectedColumnCards: map[int][]github.ProjectCard{
				1: {
					{
						ContentID:   2,
						ContentType: "PullRequest",
					},
				},
			},
		},
		{
			name: "add pull request using column name into org and repo projects",
			gc: &fakegithub.FakeClient{
				IssueLabelsAdded:   labels,
				IssueLabelsRemoved: []string{},
				RepoProjects: map[string][]github.Project{
					"testOrg/*": {
						{
							Name: "testProject",
							ID:   1,
						},
					},
					"testOrg/testRepo2": {
						{
							Name: "testProject2",
							ID:   2,
						},
					},
				},
				OrgProjects: map[string][]github.Project{},
				ProjectColumnsMap: map[string][]github.ProjectColumn{
					"testProject": {
						{
							Name: "testColumn2",
							ID:   2,
						},
						{
							Name: "testColumn",
							ID:   1,
						},
					},
					"testProject2": {
						{
							Name: "testColumn2",
							ID:   4,
						},
						{
							Name: "testColumn",
							ID:   3,
						},
					},
				},
				ColumnCardsMap: map[int][]github.ProjectCard{},
				ColumnIDMap:    columnIDMap,
			},
			projectManager: plugins.ProjectManager{
				OrgRepos: map[string]plugins.ManagedOrgRepo{
					"testOrg": {
						Projects: map[string]plugins.ManagedProject{
							"testProject": {
								Columns: []plugins.ManagedColumn{
									{
										Name:   "testColumn",
										State:  "open",
										Org:    "otherOrg",
										Labels: []string{"label1"},
									},
								},
							},
						},
					},
					"testOrg/testRepo2": {
						Projects: map[string]plugins.ManagedProject{
							"testProject2": {
								Columns: []plugins.ManagedColumn{
									{
										Name:   "testColumn2",
										State:  "open",
										Org:    "otherOrg",
										Labels: []string{"label1", "label2"},
									},
								},
							},
						},
					},
				},
			},
			// pe belongs to otherOrg/someRepo and hence has labels 'label1' and 'label2'
			// this pe should be added to testProject.testColumn in the org testOrg  and testProject2.testColumn2 in testRepo2
			pe: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				Number: 1,
				PullRequest: github.PullRequest{
					ID:    2,
					State: "open",
				},
				Repo: github.Repo{
					Name: "someRepo",
					Owner: github.User{
						Login: "otherOrg",
					},
				},
			},
			expectedColumnCards: map[int][]github.ProjectCard{
				4: {
					{
						ContentID:   2,
						ContentType: "PullRequest",
					},
				},
				1: {
					{
						ContentID:   2,
						ContentType: "PullRequest",
					},
				},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := handlePR(c.gc, c.projectManager, logrus.NewEntry(logrus.New()), c.pe)
			if err != nil {
				if c.expectedError == nil || c.expectedError.Error() != err.Error() {
					// if we are not expecting an error or if the error did not match with
					// what we are expecting
					t.Fatalf("handlePR error: %v", err)
				}
			}
			if c.expectedColumnCards == nil || len(c.expectedColumnCards) == 0 {
				return
			}

			for columnID, projectCards := range c.gc.ColumnCardsMap {
				expectedProjectCards := c.expectedColumnCards[columnID]
				if len(projectCards) != len(expectedProjectCards) {
					t.Fatalf("handlePR error, number of projectCards did not match number of expectedProjectCards for columnID %d, projectCards: %v, expectedProjectCards: %v", columnID, projectCards, expectedProjectCards)
				}
				for projectCardIndex, projectCard := range projectCards {
					expectedProjectCard := expectedProjectCards[projectCardIndex]
					if projectCard.ContentID != expectedProjectCard.ContentID ||
						projectCard.ContentType != expectedProjectCard.ContentType {
						t.Fatalf("handlePR error, projectCard did not match expectedProjectCard for index: %d, projectCard: %v, expectedProjectCard %v", projectCardIndex, projectCard, expectedProjectCard)
					}
				}
			}
		})
	}
}

func TestHelpProvider(t *testing.T) {
	var i int = 0
	managedCol1 := plugins.ManagedColumn{ID: &i, Name: "col1", State: "open", Labels: []string{"area/conformance", "area/testing"}, Org: "org1"}
	managedCol2 := plugins.ManagedColumn{ID: &i, Name: "col2", State: "open", Labels: []string{"area/conformance2", "area/testing2"}, Org: "org2"}
	managedProj := plugins.ManagedProject{Columns: []plugins.ManagedColumn{managedCol1, managedCol2}}
	managedOrgRepo := plugins.ManagedOrgRepo{Projects: map[string]plugins.ManagedProject{"project1": managedProj}}
	cases := []struct {
		name           string
		config         *plugins.Configuration
		enabledRepos   []string
		expectedConfig string
		expectedKey    string
		err            bool
	}{
		{
			name:         "Empty config",
			config:       &plugins.Configuration{},
			enabledRepos: []string{"org1", "org2/repo"},
		},
		{
			name:         "Overlapping org and org/repo",
			config:       &plugins.Configuration{},
			enabledRepos: []string{"org2", "org2/repo"},
		},
		{
			name:         "Invalid enabledRepos",
			config:       &plugins.Configuration{},
			enabledRepos: []string{"org1", "org2/repo/extra"},
			err:          true,
		},
		{
			name: "Empty projects in ProjectManager Config",
			config: &plugins.Configuration{
				ProjectManager: plugins.ProjectManager{
					OrgRepos: map[string]plugins.ManagedOrgRepo{},
				},
			},
			enabledRepos:   []string{"org1", "org2/repo"},
			expectedConfig: "",
			expectedKey:    "Config",
		},
		{
			name: "Properly formed ProjectManager Config",
			config: &plugins.Configuration{
				ProjectManager: plugins.ProjectManager{
					OrgRepos: map[string]plugins.ManagedOrgRepo{"org1": managedOrgRepo},
				},
			},
			enabledRepos: []string{"org1", "org2/repo"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ph, err := helpProvider(c.config, c.enabledRepos)
			if err != nil && !c.err {
				t.Fatalf("helpProvider error: %v", err)
			}
			if ph.Config[c.expectedKey] != c.expectedConfig {
				t.Fatalf("Error running the test %s, \nexpected: %s, \nreceived: %s", c.name, c.expectedConfig, ph.Config[c.expectedKey])
			}
		})
	}
}
