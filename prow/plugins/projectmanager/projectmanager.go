/*
Copyright 2018 The Kubernetes Authors.

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

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName = "project-manager"
)

// TODO Create a new handler for issues, look in hook/server.go
func init() {
	plugins.RegisterPullRequestHandler(pluginName, handlePullRequest, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "The project-manager plugin automatically adds Pull Requests to specified GitHub Project Columns if they match given criteria.",
		Config: func(config *plugins.Configuration) map[string]string {
			configMap := make(map[string]string)
			configString := "org/repos: {"
			for orgRepoName, managedOrgRepo := range config.ProjectManager.OrgRepos {
				configString := fmt.Sprintf("%s %s: { projects: {", configString, orgRepoName)
				for projectName, managedProject := range managedOrgRepo.Projects {
					configString := fmt.Sprintf("%s %s: { columns: [", configString, projectName)
					for _, managedColumn := range managedProject.Columns {
						configString := fmt.Sprintf("%s {id: \"%d\", name: \"%s\", state: \"%s\", org: \"%s\" labels: [", configString, managedColumn.ID, managedColumn.Name, managedColumn.State, managedColumn.Org)
						for i, label := range managedColumn.Labels {
							configString = fmt.Sprintf("%s \"%s\"", configString, label)
							if i+1 < len(managedColumn.Labels) {
								configString = fmt.Sprintf("%s,", configString)
							}
						}
						configString = fmt.Sprintf("%s ] }", configString)
					}
					configString = fmt.Sprintf("%s ] }", configString)
				}
				configString = fmt.Sprintf("%s }", configString)
			}
			configString = fmt.Sprintf("%s }", configString)
			configMap[""] = configString
			return configMap
		}(config),
	}
	return pluginHelp, nil
}

func handlePullRequest(pc plugins.Agent, pe github.PullRequestEvent) error {
	return handlePR(pc.GitHubClient, pc.PluginConfig.ProjectManager, pc.Logger, pe)
}

// Strict subset of *github.Client methods.
type githubClient interface {
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	GetRepoProjects(owner, repo string) ([]github.Project, error)
	GetOrgProjects(org string) ([]github.Project, error)
	GetProjectColumns(projectID int) ([]github.ProjectColumn, error)
	CreateProjectCard(columnID int, projectCard github.ProjectCard) (*github.ProjectCard, error)
}

func handlePR(gc githubClient, projectManager plugins.ProjectManager, log *logrus.Entry, pe github.PullRequestEvent) error {
	// Only respond to label add or issue/PR open events
	if pe.Action != github.PullRequestActionOpened &&
		pe.Action != github.PullRequestActionReopened &&
		pe.Action != github.PullRequestActionLabeled {
		return nil
	}
	// Get any ManagedProjects that match this PR
	matchedColumnIDs, err := getMatchingColumnIDs(gc, projectManager, pe)
	if err != nil {
		return err
	}
	// For each ManagedColumn that matches this PR, add this PR to that Project Column
	for _, matchedColumnID := range matchedColumnIDs {
		err = addPRToColumn(gc, matchedColumnID, pe)
		log.WithError(err).Println("Failed to add PR to project")
	}
	return nil
}

func getMatchingColumnIDs(gc githubClient, projectManager plugins.ProjectManager, pe github.PullRequestEvent) ([]int, error) {
	var matchedColumnIDs []int
	// Don't use GetIssueLabels unless it's required and keep track of whether the labels have been fetched to avoid unnecessary API usage.
	labelsFetched := false
	var labels []github.Label
	for orgRepoName, managedOrgRepo := range projectManager.OrgRepos {
		for projectName, managedProject := range managedOrgRepo.Projects {
			for _, managedColumn := range managedProject.Columns {
				if managedColumn.Org != "" && managedColumn.Org != pe.Repo.Owner.Login {
					continue
				}
				if managedColumn.State != "" && managedColumn.State != pe.PullRequest.State {
					continue
				}
				if len(managedColumn.Labels) != 0 {
					if !labelsFetched {
						// If labels are not yet fetched then get them as they are now required
						// GetIssueLabels works for PRs as they are considered issues in the API
						var err error
						labels, err = gc.GetIssueLabels(pe.Repo.Owner.Login, pe.Repo.Name, pe.Number)
						if err != nil {
							return nil, err
						}
						labelsFetched = true
					}
					if !hasLabels(managedColumn.Labels, labels) {
						continue
					}
				}
				columnID := managedColumn.ID
				// Currently this assumes columnID having a value if 0 means it is unset
				// While it's highly unlikely that an actual project would have an ID of 0, given that
				// these IDs are global across GitHub, this doesn't seem like an ideal solution.
				if columnID == 0 {
					var err error
					columnID, err = getColumnID(orgRepoName, projectName, managedColumn.Name, gc)
					if err != nil {
						return nil, err
					}
				}
				matchedColumnIDs = append(matchedColumnIDs, columnID)
			}
		}
	}
	return matchedColumnIDs, nil
}

// hasLabels checks if all labels are in the github.label set "issueLabels"
func hasLabels(labels []string, issueLabels []github.Label) bool {
	for _, label := range labels {
		if !github.HasLabel(label, issueLabels) {
			return false
		}
	}
	return true
}

func getColumnID(orgRepoName, projectName, columnName string, gc githubClient) (int, error) {
	var projects []github.Project
	var err error
	orgRepoParts := strings.Split(orgRepoName, "/")
	switch len(orgRepoParts) {
	case 2:
		projects, err = gc.GetRepoProjects(orgRepoParts[0], orgRepoParts[1])
	case 1:
		projects, err = gc.GetOrgProjects(orgRepoParts[0])
	default:
		return 0, fmt.Errorf("could not determine org or org/repo from %s", orgRepoName)
	}
	if err != nil {
		return 0, err
	}
	for _, project := range projects {
		if project.Name == projectName {
			columns, err := gc.GetProjectColumns(project.ID)
			if err != nil {
				return 0, nil
			}
			for _, column := range columns {
				if column.Name == columnName {
					return column.ID, nil
				}
			}
			return 0, fmt.Errorf("could not find column %s in project %s", columnName, projectName)
		}
	}
	return 0, fmt.Errorf("could not find project %s in org/repo %s", projectName, orgRepoName)
}

func addPRToColumn(gc githubClient, columnID int, pe github.PullRequestEvent) error {
	// Create project card and add this PR
	projectCard := github.ProjectCard{}
	projectCard.ContentType = "PullRequest"
	projectCard.ContentID = pe.PullRequest.ID
	_, err := gc.CreateProjectCard(columnID, projectCard)
	if err != nil {
		return err
	}
	return nil
}
