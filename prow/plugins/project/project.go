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
	"regexp"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	pluginName = "project"
)

var (
	projectRegex              = regexp.MustCompile(`(?m)^/project\s(.*?)$`)
	notTeamConfigMsg          = "There is no maintainer team for this repo or org."
	notATeamMemberMsg         = "You must be a member of the [%s/%s](https://github.com/orgs/%s/teams/%s/members) github team to set the project and column."
	invalidProject            = "The provided project is not valid for this organization. Projects in Kubernetes orgs and repositories: [%s]."
	invalidColumn             = "A column is not provided or it's not valid for the project %s. Please provide one of the following columns in the command:\n%v"
	invalidNumArgs            = "Please provide 1 or more arguments. Example usage: /project 0.5.0, /project 0.5.0 To do, /project clear 0.4.0"
	projectTeamMsg            = "The project maintainers team is the github team with ID: %d."
	columnsMsg                = "An issue/PR with unspecified column will be added to one of the following columns: %v."
	successMovingCardMsg      = "You have successfully moved the project card for this issue to column %s (ID %d)."
	successCreatingCardMsg    = "You have successfully created a project card for this issue. It's been added to project %s column %s (ID %D)."
	successClearingProjectMsg = "You have successfully removed this issue/PR from project %s."
	failedClearingProjectMsg  = "The project %q is not valid for the issue/PR %v. Please provide a valid project to which this issue belongs."
	clearKeyword              = "clear"
	projectNameToIDMap        = make(map[string]int)
)

type githubClient interface {
	BotUserChecker() (func(candidate string) bool, error)
	CreateComment(owner, repo string, number int, comment string) error
	ListTeamMembers(org string, id int, role string) ([]github.TeamMember, error)
	GetRepos(org string, isUser bool) ([]github.Repo, error)
	GetRepoProjects(owner, repo string) ([]github.Project, error)
	GetOrgProjects(org string) ([]github.Project, error)
	GetProjectColumns(org string, projectID int) ([]github.ProjectColumn, error)
	CreateProjectCard(org string, columnID int, projectCard github.ProjectCard) (*github.ProjectCard, error)
	GetColumnProjectCard(org string, columnID int, contentURL string) (*github.ProjectCard, error)
	MoveProjectCard(org string, projectCardID int, newColumnID int) error
	DeleteProjectCard(org string, projectCardID int) error
	TeamHasMember(org string, teamID int, memberLogin string) (bool, error)
}

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericComment, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	projectConfig := config.Project
	configInfo := map[string]string{}
	for _, repo := range enabledRepos {
		if maintainerTeamID := projectConfig.GetMaintainerTeam(repo.Org, repo.Repo); maintainerTeamID != -1 {
			configInfo[repo.String()] = fmt.Sprintf(projectTeamMsg, maintainerTeamID)
		} else {
			configInfo[repo.String()] = "There are no maintainer team specified for this repo or its org."
		}

		if columnMap := projectConfig.GetColumnMap(repo.Org, repo.Repo); len(columnMap) != 0 {
			configInfo[repo.String()] = fmt.Sprintf(columnsMsg, columnMap)
		}
	}
	yamlSnippet, err := plugins.CommentMap.GenYaml(&plugins.Configuration{
		Project: plugins.ProjectConfig{
			Orgs: map[string]plugins.ProjectOrgConfig{
				"org": {
					MaintainerTeamID: 123456,
					ProjectColumnMap: map[string]string{
						"project1": "To do",
						"project2": "Backlog",
					},
					Repos: map[string]plugins.ProjectRepoConfig{
						"repo": {
							MaintainerTeamID: 123456,
							ProjectColumnMap: map[string]string{
								"project3": "To do",
								"project4": "Backlog",
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
		Description: "The project plugin allows members of a GitHub team to set the project and column on an issue or pull request.",
		Config:      configInfo,
		Snippet:     yamlSnippet,
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/project <board>, /project <board> <column>, or /project clear <board>",
		Description: "Add an issue or PR to a project board and column",
		Featured:    false,
		WhoCanUse:   "Members of the project maintainer GitHub team can use the '/project' command.",
		Examples:    []string{"/project 0.5.0", "/project 0.5.0 To do", "/project clear 0.4.0"},
	})
	return pluginHelp, nil
}

func handleGenericComment(pc plugins.Agent, e github.GenericCommentEvent) error {
	return handle(pc.GitHubClient, pc.Logger, &e, pc.PluginConfig.Project)
}

func updateProjectNameToIDMap(projects []github.Project) {
	for _, project := range projects {
		projectNameToIDMap[project.Name] = project.ID
	}
}

// processCommand processes the user command regex matches and returns the proposed project name,
// proposed column name, whether the command is to remove issue/PR from project,
// and the error message
func processCommand(match string) (string, string, bool, string) {
	proposedProject := ""
	proposedColumnName := ""

	var shouldClear = false
	content := strings.TrimSpace(match)

	// Take care of clear
	if strings.HasPrefix(content, clearKeyword) {
		shouldClear = true
		content = strings.TrimSpace(strings.Replace(content, clearKeyword, "", 1))
	}

	// Normalize " to ' for easier handle
	content = strings.ReplaceAll(content, "\"", "'")
	var parts []string
	if strings.Contains(content, "'") {
		parts = strings.Split(content, "'")
	} else { // Split by space
		parts = strings.SplitN(content, " ", 2)
	}

	var validParts []string
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			validParts = append(validParts, strings.TrimSpace(part))
		}
	}
	if len(validParts) == 0 || len(validParts) > 2 {
		msg := invalidNumArgs
		return "", "", false, msg
	}

	proposedProject = validParts[0]
	if len(validParts) > 1 {
		proposedColumnName = validParts[1]
	}

	return proposedProject, proposedColumnName, shouldClear, ""
}

func handle(gc githubClient, log *logrus.Entry, e *github.GenericCommentEvent, projectConfig plugins.ProjectConfig) error {
	// Only handle new comments
	if e.Action != github.GenericCommentActionCreated {
		return nil
	}

	// Only handle comments that don't come from the bot
	botUserChecker, err := gc.BotUserChecker()
	if err != nil {
		return err
	}
	if botUserChecker(e.User.Login) {
		return nil
	}

	// Only handle comments that match the regex
	matches := projectRegex.FindStringSubmatch(e.Body)
	if len(matches) == 0 {
		return nil
	}

	org := e.Repo.Owner.Login
	repo := e.Repo.Name
	proposedProject, proposedColumnName, shouldClear, msg := processCommand(matches[1])
	if proposedProject == "" {
		return gc.CreateComment(org, repo, e.Number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, msg))
	}

	maintainerTeamID := projectConfig.GetMaintainerTeam(org, repo)
	if maintainerTeamID == -1 {
		return gc.CreateComment(org, repo, e.Number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, notTeamConfigMsg))
	}
	isAMember, err := gc.TeamHasMember(org, maintainerTeamID, e.User.Login)
	if err != nil {
		return err
	}
	if !isAMember {
		// not in the project maintainers team
		msg = fmt.Sprintf(notATeamMemberMsg, org, repo, org, repo)
		return gc.CreateComment(org, repo, e.Number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, msg))
	}

	var projects []github.Project

	// see if the project in the same repo as the issue/pr
	repoProjects, err := gc.GetRepoProjects(org, repo)
	if err == nil {
		projects = append(projects, repoProjects...)
	}
	updateProjectNameToIDMap(projects)

	var projectID int
	var ok bool
	// Only fetch the other repos in the org if we did not find the project in the same repo as the issue/pr
	if projectID, ok = projectNameToIDMap[proposedProject]; !ok {
		repos, err := gc.GetRepos(org, false)
		if err != nil {
			return err
		}
		// Get all projects for all repos
		for _, repo := range repos {
			repoProjects, err := gc.GetRepoProjects(org, repo.Name)
			if err != nil {
				return err
			}
			projects = append(projects, repoProjects...)
		}
	}
	// Only fetch org projects if we can't find the proposed project / project to clear in the repo projects
	updateProjectNameToIDMap(projects)
	if projectID, ok = projectNameToIDMap[proposedProject]; !ok {
		// Get all projects for this org
		orgProjects, err := gc.GetOrgProjects(org)
		if err != nil {
			return err
		}
		projects = append(projects, orgProjects...)

		// If still can't find proposed project / project to clear in the list of projects, abort and create a comment
		updateProjectNameToIDMap(projects)
		if projectID, ok = projectNameToIDMap[proposedProject]; !ok {
			slice := make([]string, 0, len(projectNameToIDMap))
			for k := range projectNameToIDMap {
				slice = append(slice, fmt.Sprintf("`%s`", k))
			}
			sort.Strings(slice)

			msg = fmt.Sprintf(invalidProject, strings.Join(slice, ", "))
			return gc.CreateComment(org, repo, e.Number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, msg))
		}
	}

	// Get all columns for proposedProject
	projectColumns, err := gc.GetProjectColumns(org, projectID)
	if err != nil {
		return err
	}

	// If proposedColumnName is not found (or not provided), add to one of the default
	// columns. If none of the default columns exists, an error will be shown to the user
	columnFound := false
	proposedColumnID := 0
	for _, c := range projectColumns {
		if c.Name == proposedColumnName {
			columnFound = true
			proposedColumnID = c.ID
			break
		}
	}
	if !columnFound && !shouldClear {
		// If user does not provide a column name, look for the columns
		// specified in the project config and see if any of them exists on the
		// proposed project
		if proposedColumnName == "" {
			defaultColumn, exists := projectConfig.GetColumnMap(org, repo)[proposedProject]
			if !exists {
				// Try to find the proposedProject in the org config in case the
				// project is on the org level
				defaultColumn, exists = projectConfig.GetOrgColumnMap(org)[proposedProject]
			}
			if exists {
				// See if the default column exists in the actual list of project columns
				for _, pc := range projectColumns {
					if pc.Name == defaultColumn {
						proposedColumnID = pc.ID
						proposedColumnName = pc.Name
						columnFound = true
						break
					}
				}
			}
		}
		// In this case, user does not provide the column name in the command,
		// or the provided column name cannot be found, and none of the default
		// columns are available in the proposed project. An error will be
		// shown to the user
		if !columnFound {
			projectColumnNames := []string{}
			for _, c := range projectColumns {
				projectColumnNames = append(projectColumnNames, c.Name)
			}
			msg = fmt.Sprintf(invalidColumn, proposedProject, projectColumnNames)
			return gc.CreateComment(org, repo, e.Number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, msg))
		}
	}

	// Move this issue/PR to the new column if there's already a project card for
	// this issue/PR in this project
	var existingProjectCard *github.ProjectCard
	var foundColumnID int
	for _, colID := range projectColumns {
		// make issue URL in the form of card content URL
		issueURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%v", org, repo, e.Number)
		existingProjectCard, err = gc.GetColumnProjectCard(org, colID.ID, issueURL)
		if err != nil {
			return err
		}

		if existingProjectCard != nil {
			foundColumnID = colID.ID
			break
		}
	}

	// no need to move the card if it is in the same column
	if (existingProjectCard != nil) && (proposedColumnID == foundColumnID) {
		return nil
	}

	// Clear issue/PR from project if command is to clear
	if shouldClear {
		if existingProjectCard != nil {
			if err := gc.DeleteProjectCard(org, existingProjectCard.ID); err != nil {
				return err
			}
			msg = fmt.Sprintf(successClearingProjectMsg, proposedProject)
			return gc.CreateComment(org, repo, e.Number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, msg))
		}
		msg = fmt.Sprintf(failedClearingProjectMsg, proposedProject, e.Number)
		return gc.CreateComment(org, repo, e.Number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, msg))
	}

	// Move this issue/PR to the new column if there's already a project card for this issue/PR in this project
	if existingProjectCard != nil {
		log.Infof("Move card to column proposedColumnID: %v with issue: %v ", proposedColumnID, e.Number)
		if err := gc.MoveProjectCard(org, existingProjectCard.ID, proposedColumnID); err != nil {
			return err
		}
		msg = fmt.Sprintf(successMovingCardMsg, proposedColumnName, proposedColumnID)
		return gc.CreateComment(org, repo, e.Number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, msg))
	}

	projectCard := github.ProjectCard{}
	projectCard.ContentID = e.ID
	if e.IsPR {
		projectCard.ContentType = "PullRequest"
	} else {
		projectCard.ContentType = "Issue"
	}

	if _, err := gc.CreateProjectCard(org, proposedColumnID, projectCard); err != nil {
		return err
	}

	msg = fmt.Sprintf(successCreatingCardMsg, proposedProject, proposedColumnName, proposedColumnID)
	return gc.CreateComment(org, repo, e.Number, plugins.FormatResponseRaw(e.Body, e.HTMLURL, e.User.Login, msg))
}
