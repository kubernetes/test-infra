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
	"strconv"
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

type fakeGithubClient struct {
	orgRepoIssueLabels map[string][]github.Label
	repoProjects       map[string][]github.Project
	orgProjects        map[string][]github.Project
	projectColumns     map[int][]github.ProjectColumn
	columnCards        map[int][]github.ProjectCard
}

func (gc *fakeGithubClient) GetIssueLabels(org, repo string, number int) ([]github.Label, error) {
	return gc.orgRepoIssueLabels[org+"/"+repo+"/"+strconv.Itoa(number)], nil
}

func (gc *fakeGithubClient) GetRepoProjects(owner, repo string) ([]github.Project, error) {
	return gc.repoProjects[owner+"/"+repo], nil
}

func (gc *fakeGithubClient) GetOrgProjects(org string) ([]github.Project, error) {
	return gc.repoProjects[org], nil
}
func (gc *fakeGithubClient) GetProjectColumns(projectID int) ([]github.ProjectColumn, error) {
	return gc.projectColumns[projectID], nil
}

func (gc *fakeGithubClient) CreateProjectCard(columnID int, projectCard github.ProjectCard) (*github.ProjectCard, error) {
	gc.columnCards[columnID] = append(gc.columnCards[columnID], projectCard)
	return &projectCard, nil
}

func TestHandlePR(t *testing.T) {
	cases := []struct {
		name                string
		gc                  *fakeGithubClient
		projectManager      plugins.ProjectManager
		pe                  github.PullRequestEvent
		expectedColumnCards map[int][]github.ProjectCard
	}{
		{
			name: "add pull request to project column with no columnID",
			gc: &fakeGithubClient{
				orgRepoIssueLabels: map[string][]github.Label{
					"otherOrg/someRepo/1": {
						{
							Name: "label1",
						},
						{
							Name: "label2",
						},
					},
				},
				repoProjects: map[string][]github.Project{
					"testOrg/testRepo": {
						{
							Name: "testProject",
							ID:   1,
						},
					},
				},
				orgProjects: map[string][]github.Project{},
				projectColumns: map[int][]github.ProjectColumn{
					1: {
						{
							Name: "testColumn",
							ID:   1,
						},
					},
				},
				columnCards: map[int][]github.ProjectCard{},
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
			name: "add pull request to project column with only columnID",
			gc: &fakeGithubClient{
				orgRepoIssueLabels: map[string][]github.Label{
					"otherOrg/someRepo/1": {
						{
							Name: "label1",
						},
						{
							Name: "label2",
						},
					},
				},
				// Note that repoProjects and projectColumns are empty so the columnID cannot be looked up using repo, project and column names
				// This means if the project_manager plugin is not using the columnID specified in the config this test will fail
				repoProjects:   map[string][]github.Project{},
				orgProjects:    map[string][]github.Project{},
				projectColumns: map[int][]github.ProjectColumn{},
				columnCards:    map[int][]github.ProjectCard{},
			},
			projectManager: plugins.ProjectManager{
				OrgRepos: map[string]plugins.ManagedOrgRepo{
					"testOrg/testRepo": {
						Projects: map[string]plugins.ManagedProject{
							"testProject": {
								Columns: []plugins.ManagedColumn{
									{
										ID:     1,
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
			name: "don't add pull request with incorrect labels",
			gc: &fakeGithubClient{
				orgRepoIssueLabels: map[string][]github.Label{
					"otherOrg/someRepo/1": {
						{
							Name: "label1",
						},
					},
				},
				repoProjects: map[string][]github.Project{
					"testOrg/testRepo": {
						{
							Name: "testProject",
							ID:   1,
						},
					},
				},
				orgProjects: map[string][]github.Project{},
				projectColumns: map[int][]github.ProjectColumn{
					1: {
						{
							Name: "testColumn",
							ID:   1,
						},
					},
				},
				columnCards: map[int][]github.ProjectCard{},
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
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := handlePR(c.gc, c.projectManager, logrus.NewEntry(logrus.New()), c.pe)
			if err != nil {
				t.Fatalf("handlePR error: %v", err)
			}
			for columnID, projectCards := range c.gc.columnCards {
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
