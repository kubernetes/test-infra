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

// Package projectmanager is a plugin to auto add pull requests to project boards based on specified conditions
package projectmanager

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName = "project-manager"
)

var (
	failedToAddProjectCard = "Failed to add project card for the issue/PR"
	issueAlreadyInProject  = "The issue/PR %d already assigned to the project %s"

	handleIssueActions = map[github.IssueEventAction]bool{
		github.IssueActionOpened:    true,
		github.IssueActionReopened:  true,
		github.IssueActionLabeled:   true,
		github.IssueActionUnlabeled: true,
	}
)

/* Sample projectmanager configuration
org/repos:
      org1/repo1:
        projects:
          test_project:
            columns:
              - id: 0
                name: triage
                state: open
                org:  org1
                labels:
                  - area/conformance
                    area/sig-testing
              - name: triage
                state: open
                org:  org1
                labels:
                - area/conformance
                  area/sig-testing
*/
// TODO Handle Label deletion, pr/issue should be removed from the project when label criteria does  not meet
// TODO Pr/issue state change, pr/iisue is on project board only if its state is listed in the configuration
func init() {
	plugins.RegisterIssueHandler(pluginName, handleIssueOrPullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, _ []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	projectConfig := config.ProjectManager
	if len(projectConfig.OrgRepos) == 0 {
		pluginHelp := &pluginhelp.PluginHelp{
			Description: "The project-manager plugin automatically adds Pull Requests to specified GitHub Project Columns, if the label on the PR matches with configured project and the column.",
			Config:      map[string]string{},
		}
		return pluginHelp, nil
	}

	configString := map[string]string{}
	repoDescr := ""
	for orgRepoName, managedOrgRepo := range config.ProjectManager.OrgRepos {
		for projectName, managedProject := range managedOrgRepo.Projects {
			for _, managedColumn := range managedProject.Columns {
				repoDescr = fmt.Sprintf("%s\nIssue/PRs org: %s, with matching labels: %s and state: %s will be added to the project: %s\n", repoDescr, managedColumn.Org, managedColumn.Labels, managedColumn.State, projectName)
			}
		}
		configString[orgRepoName] = repoDescr
	}
	id := 123
	yamlSnippet, err := plugins.CommentMap.GenYaml(&plugins.Configuration{
		ProjectManager: plugins.ProjectManager{
			OrgRepos: map[string]plugins.ManagedOrgRepo{
				"org/repo": {
					Projects: map[string]plugins.ManagedProject{
						"project": {
							Columns: []plugins.ManagedColumn{
								{
									ID:    &id,
									Name:  "To do",
									State: "open",
									Labels: []string{
										"area/conformance",
									},
									Org: "org",
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		logrus.WithError(err).Warnf("cannot generate comments for %s plugin", pluginName)
	}
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The project-manager plugin automatically adds Pull Requests to specified GitHub Project Columns, if the label on the PR matches with configured project and the column.",
		Config:      configString,
		Snippet:     yamlSnippet,
	}
	return pluginHelp, nil
}

// Strict subset of *github.Client methods.
type githubClient interface {
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	GetRepoProjects(owner, repo string) ([]github.Project, error)
	GetOrgProjects(org string) ([]github.Project, error)
	GetProjectColumns(org string, projectID int) ([]github.ProjectColumn, error)
	GetColumnProjectCards(org string, columnID int) ([]github.ProjectCard, error)
	CreateProjectCard(org string, columnID int, projectCard github.ProjectCard) (*github.ProjectCard, error)
}

type eventData struct {
	id     int
	number int
	isPR   bool
	org    string
	repo   string
	state  string
	labels []github.Label
	remove bool
}

type DuplicateCard struct {
	projectName string
	issueURL    string
}

func (m *DuplicateCard) Error() string {
	return fmt.Sprintf(issueAlreadyInProject, m.issueURL, m.projectName)
}

func handleIssueOrPullRequest(pc plugins.Agent, ie github.IssueEvent) error {
	if !handleIssueActions[ie.Action] {
		return nil
	}
	eventData := eventData{
		id:     ie.Issue.ID,
		number: ie.Issue.Number,
		isPR:   ie.Issue.IsPullRequest(),
		org:    ie.Repo.Owner.Login,
		repo:   ie.Repo.Name,
		state:  ie.Issue.State,
		labels: ie.Issue.Labels,
		remove: (ie.Action == github.IssueActionUnlabeled),
	}

	return handle(pc.GitHubClient, pc.PluginConfig.ProjectManager, pc.Logger, eventData)
}

func handle(gc githubClient, projectManager plugins.ProjectManager, log *logrus.Entry, e eventData) error {

	// Get any ManagedProjects that match this PR
	matchedColumnIDs := getMatchingColumnIDs(gc, projectManager.OrgRepos, e, log)

	// For each ManagedColumn that matches this PR, add this PR to that Project Column
	// All the matchedColumnID are valid column ids and the checked to see if the project card
	// we are adding is not already part of the project and thus avoiding duplication.
	for _, matchedColumnID := range matchedColumnIDs {
		err := addIssueToColumn(gc, matchedColumnID, e)
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"matchedColumnID": matchedColumnID,
			}).Error(failedToAddProjectCard)
			return err
		}
	}
	return nil
}

func getMatchingColumnIDs(gc githubClient, orgRepos map[string]plugins.ManagedOrgRepo, e eventData, log *logrus.Entry) []int {
	var matchedColumnIDs []int
	var err error
	// Don't use GetIssueLabels unless it's required and keep track of whether the labels have been fetched to avoid unnecessary API usage.
	if len(e.labels) == 0 {
		e.labels, err = gc.GetIssueLabels(e.org, e.repo, e.number)
		if err != nil {
			log.Infof("Cannot get labels for issue/PR: %d, error: %s", e.number, err)
		}
	}

	issueURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%v", e.org, e.repo, e.number)
	for orgRepoName, managedOrgRepo := range orgRepos {
		for projectName, managedProject := range managedOrgRepo.Projects {
			for _, managedColumn := range managedProject.Columns {
				// Org is not specified or does not match we just ignore processing this column
				if managedColumn.Org == "" || managedColumn.Org != e.org {
					log.Infof("Ignoring column: {%v}, for issue/PR: %d, due to org: %v", managedColumn, e.number, e.org)
					continue
				}
				// If state is not matching we ignore processing this column
				// If state is empty then it defaults to 'open'
				if managedColumn.State != "" && managedColumn.State != e.state {
					log.Infof("Ignoring column: {%v}, for issue/PR: %d, due to state: %v", managedColumn, e.number, e.state)
					continue
				}

				// if labels do not match we continue to the next project
				// if labels are empty on the column, the match should return false
				if !github.HasLabels(managedColumn.Labels, e.labels) {
					log.Infof("Ignoring column: {%v}, for issue/PR: %d, labels due to labels: %v ", managedColumn, e.number, e.labels)
					continue
				}

				columnID := managedColumn.ID
				// Currently this assumes columnID having a value if 0 means it is unset
				// While it's highly unlikely that an actual project would have an ID of 0, given that
				// these IDs are global across GitHub, this doesn't seem like an ideal solution.
				if columnID == nil {
					var err error
					columnID, err = getColumnID(gc, orgRepoName, projectName, managedColumn.Name, issueURL)
					if err != nil {
						if err, ok := err.(*DuplicateCard); ok {
							log.Infof("Card already exists for issue: %s, under project: %s", err.issueURL, err.projectName)
						}
						log.Infof("Cannot add the issue/PR: %d to the project: %s, column: %s, error: %s", e.number, projectName, managedColumn.Name, err)

						break
					}
				}
				matchedColumnIDs = append(matchedColumnIDs, *columnID)
				// if the configuration allows to match multiple columns within the same
				// project, we will only take the first column match from the list
				break
			}
		}
	}
	return matchedColumnIDs
}

// getColumnID returns a column id only if the issue if the project and column name provided are valid
// and the issue is not already in the project
func getColumnID(gc githubClient, orgRepoName, projectName, columnName, issueURL string) (*int, error) {
	var projects []github.Project
	var err error
	orgRepoParts := strings.Split(orgRepoName, "/")
	switch len(orgRepoParts) {
	case 2:
		projects, err = gc.GetRepoProjects(orgRepoParts[0], orgRepoParts[1])
	case 1:
		projects, err = gc.GetOrgProjects(orgRepoParts[0])
	default:
		return nil, fmt.Errorf("could not determine org or org/repo from %s", orgRepoName)
	}

	if err != nil {
		return nil, err
	}

	for _, project := range projects {
		if project.Name == projectName {
			columns, err := gc.GetProjectColumns(orgRepoParts[0], project.ID)
			if err != nil {
				return nil, err
			}

			for _, column := range columns {
				cards, err := gc.GetColumnProjectCards(orgRepoParts[0], column.ID)
				if err != nil {
					return nil, err
				}

				for _, card := range cards {
					if card.ContentURL == issueURL {
						return nil, &DuplicateCard{issueURL: issueURL, projectName: projectName}
					}
				}
			}
			for _, column := range columns {
				if column.Name == columnName {
					return &column.ID, nil
				}
			}
			return nil, fmt.Errorf("could not find column %s in project %s", columnName, projectName)
		}
	}
	return nil, fmt.Errorf("could not find project %s in org/repo %s", projectName, orgRepoName)
}

func addIssueToColumn(gc githubClient, columnID int, e eventData) error {
	// Create project card and add this PR
	projectCard := github.ProjectCard{}
	if e.isPR {
		projectCard.ContentType = "PullRequest"
	} else {
		projectCard.ContentType = "Issue"
	}
	projectCard.ContentID = e.id
	_, err := gc.CreateProjectCard(e.org, columnID, projectCard)
	return err
}
