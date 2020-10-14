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
	"fmt"
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/config"
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
	ie := github.IssueEvent{
		Action: github.IssueActionOpened,
		Issue: github.Issue{
			ID:     2,
			State:  "open",
			Labels: []github.Label{{Name: "label1"}, {Name: "label2"}},
		},
		Repo: github.Repo{
			Name: "someRepo",
			Owner: github.User{
				Login: "otherOrg",
			},
		},
	}
	ie2 := ie
	ie2.Repo.Owner.Login = "otherOrg2"
	// pe belongs to otherOrg/somerepo and has labels 'label1' and 'label2'
	// this pe should land in testproject under column testColumn2
	pe := github.IssueEvent{
		Action: github.IssueActionOpened,
		Issue: github.Issue{
			ID:          2,
			State:       "open",
			Labels:      []github.Label{{Name: "label1"}, {Name: "label2"}},
			PullRequest: &struct{}{},
		},
		Repo: github.Repo{
			Name: "someRepo",
			Owner: github.User{
				Login: "otherOrg",
			},
		},
	}
	pe2 := pe
	pe2.Repo.Owner.Login = "otherOrg2"
	// all issues/PRs will be automatically be populated by these labels depending on which org they belong
	labels := []string{"otherOrg/someRepo#1:label1", "otherOrg/someRepo#1:label2", "otherOrg/someRepo#2:label1", "otherOrg2/someRepo#1:label1"}
	cases := []struct {
		name                string
		gc                  *fakegithub.FakeClient
		projectManager      plugins.ProjectManager
		pe                  github.IssueEvent
		ie                  github.IssueEvent
		expectedColumnCards map[int][]github.ProjectCard
		expectedError       error
	}{
		{
			name: "add Issue/PR to project column with no columnID",
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
			pe: pe,
			ie: ie,
			expectedColumnCards: map[int][]github.ProjectCard{
				2: {
					{
						ContentID: 2,
					},
				},
			},
		},
		{
			name: "add Issue/PR to project column with only columnID",
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
			pe: pe,
			ie: ie,
			expectedColumnCards: map[int][]github.ProjectCard{
				1: {
					{
						ContentID: 2,
					},
				},
			},
		},
		{
			name: "don't add Issue/PR with incorrect column name",
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
			pe:                  pe,
			ie:                  ie,
			expectedColumnCards: map[int][]github.ProjectCard{},
		},
		{
			name: "don't add Issue/PR if all the labels do not match",
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
			pe:                  pe,
			ie:                  ie,
			expectedColumnCards: map[int][]github.ProjectCard{},
		},
		{
			name: "add Issue/PR using column name in multiple repos",
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
										Org:    "otherOrg2",
										Labels: []string{"label1"},
									},
								},
							},
						},
					},
				},
			},
			pe: pe2,
			ie: ie2,
			expectedColumnCards: map[int][]github.ProjectCard{
				4: {
					{
						ContentID: 2,
					},
				},
				1: {
					{
						ContentID: 2,
					},
				},
			},
		},
		{
			name: "add Issue/PR using column name in multirepo to multiple projects",
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
			pe: pe,
			ie: ie,
			expectedColumnCards: map[int][]github.ProjectCard{
				4: {
					{
						ContentID: 2,
					},
				},
				1: {
					{
						ContentID: 2,
					},
				},
			},
		},
		{
			name: "add Issue/PR to multiple columns in a project, should realize conflict",
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
						ContentID:  2,
						ContentURL: "https://api.github.com/repos/otherOrg/someRepo/issues/1",
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
			pe: pe,
			ie: ie,
			expectedColumnCards: map[int][]github.ProjectCard{
				1: {
					{
						ContentID: 2,
					},
				},
			},
		},
		{
			name: "add Issue/PR using column name into org and repo projects",
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
			pe: pe,
			ie: ie,
			expectedColumnCards: map[int][]github.ProjectCard{
				4: {
					{
						ContentID: 2,
					},
				},
				1: {
					{
						ContentID: 2,
					},
				},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name+"[PullRequests]", func(t *testing.T) {
			if !handleIssueActions[ie.Action] {
				t.Logf("%s: Event with Action %s will not be processed by this plugin", c.name, c.ie.Action)
				return
			}
			eData := eventData{
				id:     c.pe.Issue.ID,
				number: c.pe.Issue.Number,
				isPR:   c.pe.Issue.IsPullRequest(),
				org:    c.pe.Repo.Owner.Login,
				repo:   c.pe.Repo.Name,
				state:  c.pe.Issue.State,
				labels: c.pe.Issue.Labels,
				remove: (c.pe.Action == github.IssueActionUnlabeled),
			}

			err := handle(c.gc, c.projectManager, logrus.NewEntry(logrus.New()), eData)
			if err != nil {
				if c.expectedError == nil || c.expectedError.Error() != err.Error() {
					// if we are not expecting an error or if the error did not match with
					// what we are expecting
					t.Fatalf("%s: handlePR error: %v", c.name, err)
				}
			}
			err = checkCards(c.expectedColumnCards, c.gc.ColumnCardsMap, true)
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
		})
	}

	for _, c := range cases {
		t.Run(c.name+"[Issues]", func(t *testing.T) {
			// reset the cards at the beginning of new test cycle.
			c.gc.ColumnCardsMap = map[int][]github.ProjectCard{}
			if !handleIssueActions[ie.Action] {
				t.Logf("%s: Event with Action %s will not be processed by this plugin", c.name, c.ie.Action)
				return
			}
			eData := eventData{
				id:     c.ie.Issue.ID,
				number: c.ie.Issue.Number,
				isPR:   c.ie.Issue.IsPullRequest(),
				org:    c.ie.Repo.Owner.Login,
				repo:   c.ie.Repo.Name,
				state:  c.ie.Issue.State,
				labels: c.ie.Issue.Labels,
				remove: (c.ie.Action == github.IssueActionUnlabeled),
			}

			err := handle(c.gc, c.projectManager, logrus.NewEntry(logrus.New()), eData)
			if err != nil {
				if c.expectedError == nil || c.expectedError.Error() != err.Error() {
					// if we are not expecting an error or if the error did not match with
					// what we are expecting
					t.Fatalf("%s: handleIssue error: %v", c.name, err)
				}
			}
			err = checkCards(c.expectedColumnCards, c.gc.ColumnCardsMap, false)
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
		})
	}
}

func checkCards(expectedColumnCards, projectColumnCards map[int][]github.ProjectCard, isPR bool) error {
	if len(expectedColumnCards) == 0 {
		return nil
	}

	for columnID, expectedCards := range expectedColumnCards {
		projectCards := projectColumnCards[columnID]

		//make sure all expectedCard are in projectCards
		if len(expectedCards) > len(projectCards) {
			return fmt.Errorf("Not all expected cards can be found for column: %d, \nexpected: %v\n found: %v", columnID, expectedCards, projectCards)
		}
		for _, card := range expectedCards {
			found := false
			for _, pcard := range projectCards {
				if pcard.ContentID == card.ContentID {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("Unable to find project card: %v under column: %d", card, columnID)
			}
		}
	}
	return nil
}

func TestHelpProvider(t *testing.T) {
	var i int = 0
	enabledRepos := []config.OrgRepo{
		{Org: "org1", Repo: "repo"},
		{Org: "org2", Repo: "repo"},
	}
	managedCol1 := plugins.ManagedColumn{ID: &i, Name: "col1", State: "open", Labels: []string{"area/conformance", "area/testing"}, Org: "org1"}
	managedCol2 := plugins.ManagedColumn{ID: &i, Name: "col2", State: "open", Labels: []string{"area/conformance2", "area/testing2"}, Org: "org2"}
	managedProj := plugins.ManagedProject{Columns: []plugins.ManagedColumn{managedCol1, managedCol2}}
	managedOrgRepo := plugins.ManagedOrgRepo{Projects: map[string]plugins.ManagedProject{"project1": managedProj}}
	cases := []struct {
		name           string
		config         *plugins.Configuration
		enabledRepos   []config.OrgRepo
		expectedConfig string
		expectedKey    string
		err            bool
	}{
		{
			name:         "Empty config",
			config:       &plugins.Configuration{},
			enabledRepos: enabledRepos,
		},
		{
			name: "Empty projects in ProjectManager Config",
			config: &plugins.Configuration{
				ProjectManager: plugins.ProjectManager{
					OrgRepos: map[string]plugins.ManagedOrgRepo{},
				},
			},
			enabledRepos:   enabledRepos,
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
			enabledRepos: enabledRepos,
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
