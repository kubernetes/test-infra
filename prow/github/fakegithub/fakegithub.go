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

package fakegithub

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/github"
)

const botName = "k8s-ci-robot"

const (
	// Bot is the exported botName
	Bot = botName
	// TestRef is the ref returned when calling GetRef
	TestRef = "abcde"
)

// FakeClient is like client, but fake.
type FakeClient struct {
	Issues              map[int]*github.Issue
	IssueID             int
	OrgMembers          map[string][]string
	Collaborators       []string
	IssueComments       map[int][]github.IssueComment
	IssueCommentID      int
	PullRequests        map[int]*github.PullRequest
	PullRequestChanges  map[int][]github.PullRequestChange
	PullRequestComments map[int][]github.ReviewComment
	ReviewID            int
	Reviews             map[int][]github.Review
	CombinedStatuses    map[string]*github.CombinedStatus
	CreatedStatuses     map[string][]github.Status
	IssueEvents         map[int][]github.ListedIssueEvent
	Commits             map[string]github.RepositoryCommit

	// All Labels That Exist In The Repo
	RepoLabelsExisting []string
	// org/repo#number:label
	IssueLabelsAdded    []string
	IssueLabelsExisting []string
	IssueLabelsRemoved  []string

	// org/repo#number:body
	IssueCommentsAdded []string
	// org/repo#issuecommentid
	IssueCommentsDeleted []string

	// org/repo#issuecommentid:reaction
	IssueReactionsAdded   []string
	CommentReactionsAdded []string

	// org/repo#number:assignee
	AssigneesAdded []string

	// org/repo#number:milestone (represents the milestone for a specific issue)
	Milestone    int
	MilestoneMap map[string]int

	// list of commits for each PR
	// org/repo#number:[]commit
	CommitMap map[string][]github.RepositoryCommit

	// Fake remote git storage. File name are keys
	// and values map SHA to content
	RemoteFiles map[string]map[string]string

	// Fake remote git storage. Directory name are keys
	// and values map SHA to directory content
	RemoteDirectories map[string]map[string][]github.DirectoryContent

	// A list of refs that got deleted via DeleteRef
	RefsDeleted []struct{ Org, Repo, Ref string }

	// A map of repo names to projects
	RepoProjects map[string][]github.Project

	// A map of project name to columns
	ProjectColumnsMap map[string][]github.ProjectColumn

	// Maps column ID to the list of project cards
	ColumnCardsMap map[int][]github.ProjectCard

	// Maps project name to maps of column ID to columnName
	ColumnIDMap map[string]map[int]string

	// The project and column names for an issue or PR
	Project            string
	Column             string
	OrgRepoIssueLabels map[string][]github.Label
	OrgProjects        map[string][]github.Project

	// Maps org name to the list of hooks
	OrgHooks map[string][]github.Hook
	// Maps repo name to the list of hooks
	RepoHooks map[string][]github.Hook

	// Error will be returned if set. Currently only implemented for CreateStatus
	Error error

	// lock to be thread safe
	lock sync.RWMutex
}

func (f *FakeClient) BotUser() (*github.UserData, error) {
	return &github.UserData{Login: botName}, nil
}

func (f *FakeClient) BotUserChecker() (func(candidate string) bool, error) {
	return func(candidate string) bool {
		candidate = strings.TrimSuffix(candidate, "[bot]")
		return candidate == botName
	}, nil
}

func NewFakeClient() *FakeClient {
	return &FakeClient{
		Issues:              make(map[int]*github.Issue),
		OrgMembers:          make(map[string][]string),
		IssueComments:       make(map[int][]github.IssueComment),
		PullRequests:        make(map[int]*github.PullRequest),
		PullRequestChanges:  make(map[int][]github.PullRequestChange),
		PullRequestComments: make(map[int][]github.ReviewComment),
		Reviews:             make(map[int][]github.Review),
		CombinedStatuses:    make(map[string]*github.CombinedStatus),
		CreatedStatuses:     make(map[string][]github.Status),
		IssueEvents:         make(map[int][]github.ListedIssueEvent),
		Commits:             make(map[string]github.RepositoryCommit),

		MilestoneMap: make(map[string]int),
		CommitMap:    make(map[string][]github.RepositoryCommit),
		RemoteFiles:  make(map[string]map[string]string),

		RepoProjects:       make(map[string][]github.Project),
		ProjectColumnsMap:  make(map[string][]github.ProjectColumn),
		ColumnCardsMap:     make(map[int][]github.ProjectCard),
		ColumnIDMap:        make(map[string]map[int]string),
		OrgRepoIssueLabels: make(map[string][]github.Label),
		OrgProjects:        make(map[string][]github.Project),
		OrgHooks:           make(map[string][]github.Hook),
		RepoHooks:          make(map[string][]github.Hook),
	}
}

// IsMember returns true if user is in org.
func (f *FakeClient) IsMember(org, user string) (bool, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	for _, m := range f.OrgMembers[org] {
		if m == user {
			return true, nil
		}
	}
	return false, nil
}

// ListOpenIssues returns f.issues
// To mock a mix of issues and pull requests, see github.Issue.PullRequest
func (f *FakeClient) ListOpenIssues(org, repo string) ([]github.Issue, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	var issues []github.Issue
	for _, issue := range f.Issues {
		issues = append(issues, *issue)
	}
	return issues, nil
}

// ListIssueComments returns comments.
func (f *FakeClient) ListIssueComments(owner, repo string, number int) ([]github.IssueComment, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	return append([]github.IssueComment{}, f.IssueComments[number]...), nil
}

// ListPullRequestComments returns review comments.
func (f *FakeClient) ListPullRequestComments(owner, repo string, number int) ([]github.ReviewComment, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	return append([]github.ReviewComment{}, f.PullRequestComments[number]...), nil
}

// ListReviews returns reviews.
func (f *FakeClient) ListReviews(owner, repo string, number int) ([]github.Review, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	return append([]github.Review{}, f.Reviews[number]...), nil
}

// ListIssueEvents returns issue events
func (f *FakeClient) ListIssueEvents(owner, repo string, number int) ([]github.ListedIssueEvent, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	return append([]github.ListedIssueEvent{}, f.IssueEvents[number]...), nil
}

// CreateComment adds a comment to a PR
func (f *FakeClient) CreateComment(owner, repo string, number int, comment string) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.IssueCommentID++
	f.IssueCommentsAdded = append(f.IssueCommentsAdded, fmt.Sprintf("%s/%s#%d:%s", owner, repo, number, comment))
	f.IssueComments[number] = append(f.IssueComments[number], github.IssueComment{
		ID:   f.IssueCommentID,
		Body: comment,
		User: github.User{Login: botName},
	})
	return nil
}

// EditComment edits a comment. Its a stub that does nothing.
func (f *FakeClient) EditComment(org, repo string, ID int, comment string) error {
	return nil
}

// CreateReview adds a review to a PR
func (f *FakeClient) CreateReview(org, repo string, number int, r github.DraftReview) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.ReviewID++
	f.Reviews[number] = append(f.Reviews[number], github.Review{
		ID:   f.ReviewID,
		User: github.User{Login: botName},
		Body: r.Body,
	})
	return nil
}

// CreateCommentReaction adds emoji to a comment.
func (f *FakeClient) CreateCommentReaction(org, repo string, ID int, reaction string) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.CommentReactionsAdded = append(f.CommentReactionsAdded, fmt.Sprintf("%s/%s#%d:%s", org, repo, ID, reaction))
	return nil
}

// CreateIssueReaction adds an emoji to an issue.
func (f *FakeClient) CreateIssueReaction(org, repo string, ID int, reaction string) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.IssueReactionsAdded = append(f.IssueReactionsAdded, fmt.Sprintf("%s/%s#%d:%s", org, repo, ID, reaction))
	return nil
}

// DeleteComment deletes a comment.
func (f *FakeClient) DeleteComment(owner, repo string, ID int) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.IssueCommentsDeleted = append(f.IssueCommentsDeleted, fmt.Sprintf("%s/%s#%d", owner, repo, ID))
	for num, ics := range f.IssueComments {
		for i, ic := range ics {
			if ic.ID == ID {
				f.IssueComments[num] = append(ics[:i], ics[i+1:]...)
				return nil
			}
		}
	}
	return fmt.Errorf("could not find issue comment %d", ID)
}

// DeleteStaleComments deletes comments flagged by isStale.
func (f *FakeClient) DeleteStaleComments(org, repo string, number int, comments []github.IssueComment, isStale func(github.IssueComment) bool) error {
	if comments == nil {
		comments, _ = f.ListIssueComments(org, repo, number)
	}
	for _, comment := range comments {
		if isStale(comment) {
			if err := f.DeleteComment(org, repo, comment.ID); err != nil {
				return fmt.Errorf("failed to delete stale comment with ID '%d'", comment.ID)
			}
		}
	}
	return nil
}

// GetPullRequest returns details about the PR.
func (f *FakeClient) GetPullRequest(owner, repo string, number int) (*github.PullRequest, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	val, exists := f.PullRequests[number]
	if !exists {
		return nil, fmt.Errorf("pull request number %d does not exist", number)
	}
	return val, nil
}

// EditPullRequest edits the pull request.
func (f *FakeClient) EditPullRequest(org, repo string, number int, issue *github.PullRequest) (*github.PullRequest, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if _, exists := f.PullRequests[number]; !exists {
		return nil, fmt.Errorf("issue number %d does not exist", number)
	}
	f.PullRequests[number] = issue
	return issue, nil
}

// GetIssue returns the issue.
func (f *FakeClient) GetIssue(owner, repo string, number int) (*github.Issue, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	val, exists := f.Issues[number]
	if !exists {
		return nil, fmt.Errorf("issue number %d does not exist", number)
	}
	return val, nil
}

// EditIssue edits the issue.
func (f *FakeClient) EditIssue(org, repo string, number int, issue *github.Issue) (*github.Issue, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if _, exists := f.Issues[number]; !exists {
		return nil, fmt.Errorf("issue number %d does not exist", number)
	}
	f.Issues[number] = issue
	return issue, nil
}

// CreateIssue creates the issue.
func (f *FakeClient) CreateIssue(org, repo, title, body string, milestone int, labels, assignees []string) (int, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.IssueID++
	if f.Issues == nil {
		f.Issues = make(map[int]*github.Issue)
	}
	var ls []github.Label
	for _, l := range labels {
		ls = append(ls, github.Label{Name: l})
	}
	var as []github.User
	for _, a := range assignees {
		as = append(as, github.User{Name: a})
	}
	new := &github.Issue{
		ID:        f.IssueID,
		Title:     title,
		Body:      body,
		Milestone: github.Milestone{Number: milestone},
		Labels:    ls,
		Assignees: as,
	}
	f.Issues[f.IssueID] = new
	f.IssueComments[f.IssueID] = make([]github.IssueComment, 0)
	return new.ID, nil
}

// GetPullRequestChanges returns the file modifications in a PR.
func (f *FakeClient) GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	return f.PullRequestChanges[number], nil
}

// GetRef returns the hash of a ref.
func (f *FakeClient) GetRef(owner, repo, ref string) (string, error) {
	return TestRef, nil
}

// DeleteRef returns an error indicating if deletion of the given ref was successful
func (f *FakeClient) DeleteRef(owner, repo, ref string) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.RefsDeleted = append(f.RefsDeleted, struct{ Org, Repo, Ref string }{Org: owner, Repo: repo, Ref: ref})
	return nil
}

// GetSingleCommit returns a single commit.
func (f *FakeClient) GetSingleCommit(org, repo, SHA string) (github.RepositoryCommit, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	return f.Commits[SHA], nil
}

// CreateStatus adds a status context to a commit.
func (f *FakeClient) CreateStatus(owner, repo, SHA string, s github.Status) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	if f.Error != nil {
		return f.Error
	}
	if f.CreatedStatuses == nil {
		f.CreatedStatuses = make(map[string][]github.Status)
	}
	statuses := f.CreatedStatuses[SHA]
	var updated bool
	for i := range statuses {
		if statuses[i].Context == s.Context {
			statuses[i] = s
			updated = true
		}
	}
	if !updated {
		statuses = append(statuses, s)
	}
	f.CreatedStatuses[SHA] = statuses
	f.CombinedStatuses[SHA] = &github.CombinedStatus{
		SHA:      SHA,
		Statuses: statuses,
	}
	return nil
}

// ListStatuses returns individual status contexts on a commit.
func (f *FakeClient) ListStatuses(org, repo, ref string) ([]github.Status, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	return f.CreatedStatuses[ref], nil
}

// GetCombinedStatus returns the overall status for a commit.
func (f *FakeClient) GetCombinedStatus(owner, repo, ref string) (*github.CombinedStatus, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	return f.CombinedStatuses[ref], nil
}

// GetRepoLabels gets labels in a repo.
func (f *FakeClient) GetRepoLabels(owner, repo string) ([]github.Label, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	la := []github.Label{}
	for _, l := range f.RepoLabelsExisting {
		la = append(la, github.Label{Name: l})
	}
	return la, nil
}

// GetIssueLabels gets labels on an issue
func (f *FakeClient) GetIssueLabels(owner, repo string, number int) ([]github.Label, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	re := regexp.MustCompile(fmt.Sprintf(`^%s/%s#%d:(.*)$`, owner, repo, number))
	la := []github.Label{}
	allLabels := sets.NewString(f.IssueLabelsExisting...)
	allLabels.Insert(f.IssueLabelsAdded...)
	allLabels.Delete(f.IssueLabelsRemoved...)
	for _, l := range allLabels.List() {
		groups := re.FindStringSubmatch(l)
		if groups != nil {
			la = append(la, github.Label{Name: groups[1]})
		}
	}
	return la, nil
}

// AddLabel adds a label
func (f *FakeClient) AddLabel(owner, repo string, number int, label string) error {
	return f.AddLabels(owner, repo, number, label)
}

// AddLabels adds a list of labels
func (f *FakeClient) AddLabels(owner, repo string, number int, labels ...string) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	for _, label := range labels {
		labelString := fmt.Sprintf("%s/%s#%d:%s", owner, repo, number, label)
		if sets.NewString(f.IssueLabelsAdded...).Has(labelString) {
			return fmt.Errorf("cannot add %v to %s/%s/#%d", label, owner, repo, number)
		}
		if f.RepoLabelsExisting == nil {
			f.IssueLabelsAdded = append(f.IssueLabelsAdded, labelString)
			return nil
		}
		for _, l := range f.RepoLabelsExisting {
			if label == l {
				f.IssueLabelsAdded = append(f.IssueLabelsAdded, labelString)
				continue
			}

			var repoLabelExists bool
			for _, l := range f.RepoLabelsExisting {
				if label == l {
					f.IssueLabelsAdded = append(f.IssueLabelsAdded, labelString)
					repoLabelExists = true
					break
				}
			}
			if !repoLabelExists {
				return fmt.Errorf("cannot add %v to %s/%s/#%d", label, owner, repo, number)
			}
		}
	}
	return nil
}

// RemoveLabel removes a label
func (f *FakeClient) RemoveLabel(owner, repo string, number int, label string) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	labelString := fmt.Sprintf("%s/%s#%d:%s", owner, repo, number, label)
	if !sets.NewString(f.IssueLabelsRemoved...).Has(labelString) {
		f.IssueLabelsRemoved = append(f.IssueLabelsRemoved, labelString)
		return nil
	}
	return fmt.Errorf("cannot remove %v from %s/%s/#%d", label, owner, repo, number)
}

// FindIssues returns f.Issues
func (f *FakeClient) FindIssues(query, sort string, asc bool) ([]github.Issue, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	var issues []github.Issue
	for _, issue := range f.Issues {
		issues = append(issues, *issue)
	}
	for _, pr := range f.PullRequests {
		issues = append(issues, github.Issue{
			User:   pr.User,
			Number: pr.Number,
		})
	}
	return issues, nil
}

// AssignIssue adds assignees.
func (f *FakeClient) AssignIssue(owner, repo string, number int, assignees []string) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	var m github.MissingUsers
	for _, a := range assignees {
		if a == "not-in-the-org" {
			m.Users = append(m.Users, a)
			continue
		}
		f.AssigneesAdded = append(f.AssigneesAdded, fmt.Sprintf("%s/%s#%d:%s", owner, repo, number, a))
	}
	if m.Users == nil {
		return nil
	}
	return m
}

// GetFile returns the bytes of the file.
func (f *FakeClient) GetFile(org, repo, file, commit string) ([]byte, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	contents, ok := f.RemoteFiles[file]
	if !ok {
		return nil, fmt.Errorf("could not find file %s", file)
	}
	if commit == "" {
		if master, ok := contents["master"]; ok {
			return []byte(master), nil
		}

		return nil, fmt.Errorf("could not find file %s in master", file)
	}

	if content, ok := contents[commit]; ok {
		return []byte(content), nil
	}

	return nil, fmt.Errorf("could not find file %s with ref %s", file, commit)
}

// ListTeams return a list of fake teams that correspond to the fake team members returned by ListTeamMembers
func (f *FakeClient) ListTeams(org string) ([]github.Team, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	return []github.Team{
		{
			ID:   0,
			Name: "Admins",
		},
		{
			ID:   42,
			Name: "Leads",
		},
	}, nil
}

// ListTeamMembers return a fake team with a single "sig-lead" GitHub teammember
func (f *FakeClient) ListTeamMembers(org string, teamID int, role string) ([]github.TeamMember, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	if role != github.RoleAll {
		return nil, fmt.Errorf("unsupported role %v (only all supported)", role)
	}
	teams := map[int][]github.TeamMember{
		0:  {{Login: "default-sig-lead"}},
		42: {{Login: "sig-lead"}},
	}
	members, ok := teams[teamID]
	if !ok {
		return []github.TeamMember{}, nil
	}
	return members, nil
}

// IsCollaborator returns true if the user is a collaborator of the repo.
func (f *FakeClient) IsCollaborator(org, repo, login string) (bool, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	normed := github.NormLogin(login)
	for _, collab := range f.Collaborators {
		if github.NormLogin(collab) == normed {
			return true, nil
		}
	}
	return false, nil
}

// ListCollaborators lists the collaborators.
func (f *FakeClient) ListCollaborators(org, repo string) ([]github.User, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	result := make([]github.User, 0, len(f.Collaborators))
	for _, login := range f.Collaborators {
		result = append(result, github.User{Login: login})
	}
	return result, nil
}

// ClearMilestone removes the milestone
func (f *FakeClient) ClearMilestone(org, repo string, issueNum int) error {
	f.Milestone = 0
	return nil
}

// SetMilestone sets the milestone.
func (f *FakeClient) SetMilestone(org, repo string, issueNum, milestoneNum int) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	if milestoneNum < 0 {
		return fmt.Errorf("Milestone Numbers Cannot Be Negative")
	}
	f.Milestone = milestoneNum
	return nil
}

// ListMilestones lists milestones.
func (f *FakeClient) ListMilestones(org, repo string) ([]github.Milestone, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	milestones := []github.Milestone{}
	for k, v := range f.MilestoneMap {
		milestones = append(milestones, github.Milestone{Title: k, Number: v})
	}
	return milestones, nil
}

// ListPRCommits lists commits for a given PR.
func (f *FakeClient) ListPRCommits(org, repo string, prNumber int) ([]github.RepositoryCommit, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	k := fmt.Sprintf("%s/%s#%d", org, repo, prNumber)
	return f.CommitMap[k], nil
}

// GetRepoProjects returns the list of projects under a repo.
func (f *FakeClient) GetRepoProjects(owner, repo string) ([]github.Project, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.RepoProjects[fmt.Sprintf("%s/%s", owner, repo)], nil
}

// GetOrgProjects returns the list of projects under an org
func (f *FakeClient) GetOrgProjects(org string) ([]github.Project, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	return f.RepoProjects[fmt.Sprintf("%s/*", org)], nil
}

// GetProjectColumns returns the list of columns for a given project.
func (f *FakeClient) GetProjectColumns(org string, projectID int) ([]github.ProjectColumn, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	// Get project name
	for _, projects := range f.RepoProjects {
		for _, project := range projects {
			if projectID == project.ID {
				return f.ProjectColumnsMap[project.Name], nil
			}
		}
	}
	return nil, fmt.Errorf("Cannot find project ID")
}

// CreateProjectCard creates a project card under a given column.
func (f *FakeClient) CreateProjectCard(org string, columnID int, projectCard github.ProjectCard) (*github.ProjectCard, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	for project, columnIDMap := range f.ColumnIDMap {
		if _, exists := columnIDMap[columnID]; exists {
			for id := range columnIDMap {
				// Make sure that we behave same as github API
				// Create project will generate an error when the card already exist in the project
				card, err := f.GetColumnProjectCard(org, id, projectCard.ContentURL)
				if err == nil && card != nil {
					return nil, fmt.Errorf("Card already exist in the project: %s, column %d, cannot add to column  %d", project, id, columnID)
				}
			}
		}
		columnName, exists := columnIDMap[columnID]
		if exists {
			f.ColumnCardsMap[columnID] = append(
				f.ColumnCardsMap[columnID],
				projectCard,
			)
			f.Column = columnName
			f.Project = project
			return &projectCard, nil
		}
	}
	return nil, fmt.Errorf("Provided column %d does not exist, ColumnIDMap is %v", columnID, f.ColumnIDMap)
}

// DeleteProjectCard deletes the project card of a specific issue or PR
func (f *FakeClient) DeleteProjectCard(org string, projectCardID int) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	if f.ColumnCardsMap == nil {
		return fmt.Errorf("Project card doesn't exist")
	}
	f.Project = ""
	f.Column = ""
	newCards := []github.ProjectCard{}
	oldColumnID := -1
	for column, cards := range f.ColumnCardsMap {
		removalIndex := -1
		for i, existingCard := range cards {
			if existingCard.ContentID == projectCardID {
				oldColumnID = column
				removalIndex = i
				break
			}
		}
		if removalIndex != -1 {
			newCards = cards
			newCards[removalIndex] = newCards[len(newCards)-1]
			newCards = newCards[:len(newCards)-1]
			break
		}
	}
	// Update the old column's list of project cards
	if oldColumnID != -1 {
		f.ColumnCardsMap[oldColumnID] = newCards
	}
	return nil
}

// GetColumnProjectCards fetches project cards  under given column
func (f *FakeClient) GetColumnProjectCards(org string, columnID int) ([]github.ProjectCard, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	if f.ColumnCardsMap == nil {
		f.ColumnCardsMap = make(map[int][]github.ProjectCard)
	}
	return f.ColumnCardsMap[columnID], nil
}

// GetColumnProjectCard fetches project card if the content_url in the card matched the issue/pr
func (f *FakeClient) GetColumnProjectCard(org string, columnID int, contentURL string) (*github.ProjectCard, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	cards, err := f.GetColumnProjectCards(org, columnID)
	if err != nil {
		return nil, err
	}

	for _, existingCard := range cards {
		if existingCard.ContentURL == contentURL {
			return &existingCard, nil
		}
	}
	return nil, nil
}

func (f *FakeClient) GetRepos(org string, isUser bool) ([]github.Repo, error) {
	return []github.Repo{
		{
			Owner: github.User{
				Login: "kubernetes",
			},
			Name: "kubernetes",
		},
		{
			Owner: github.User{
				Login: "kubernetes",
			},
			Name: "community",
		},
	}, nil
}

func (f *FakeClient) GetRepo(owner, name string) (github.FullRepo, error) {
	return github.FullRepo{
		Repo: github.Repo{
			Owner:         github.User{Login: owner},
			Name:          name,
			HasIssues:     true,
			HasWiki:       true,
			DefaultBranch: "master",
			Description:   fmt.Sprintf("Test Repo: %s", name),
		},
	}, nil
}

// MoveProjectCard moves a specific project card to a specified column in the same project
func (f *FakeClient) MoveProjectCard(org string, projectCardID int, newColumnID int) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	// Remove project card from old column
	newCards := []github.ProjectCard{}
	oldColumnID := -1
	projectCard := github.ProjectCard{}
	for column, cards := range f.ColumnCardsMap {
		removalIndex := -1
		for i, existingCard := range cards {
			if existingCard.ContentID == projectCardID {
				oldColumnID = column
				removalIndex = i
				projectCard = existingCard
				break
			}
		}
		if removalIndex != -1 {
			newCards = cards
			newCards[removalIndex] = newCards[len(newCards)-1]
			newCards = newCards[:len(newCards)-1]
		}
	}
	if oldColumnID != -1 {
		// Update the old column's list of project cards
		f.ColumnCardsMap[oldColumnID] = newCards
	}

	for project, columnIDMap := range f.ColumnIDMap {
		if columnName, exists := columnIDMap[newColumnID]; exists {
			// Add project card to new column
			f.ColumnCardsMap[newColumnID] = append(
				f.ColumnCardsMap[newColumnID],
				projectCard,
			)
			f.Column = columnName
			f.Project = project
			break
		}
	}

	return nil
}

// TeamHasMember checks if a user belongs to a team
func (f *FakeClient) TeamHasMember(org string, teamID int, memberLogin string) (bool, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	teamMembers, _ := f.ListTeamMembers(org, teamID, github.RoleAll)
	for _, member := range teamMembers {
		if member.Login == memberLogin {
			return true, nil
		}
	}
	return false, nil
}

func (f *FakeClient) GetTeamBySlug(slug string, org string) (*github.Team, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	teams, _ := f.ListTeams(org)
	for _, team := range teams {
		if team.Name == slug {
			return &team, nil
		}
	}
	return &github.Team{}, nil
}

func (f *FakeClient) CreatePullRequest(org, repo, title, body, head, base string, canModify bool) (int, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if f.PullRequests == nil {
		f.PullRequests = map[int]*github.PullRequest{}
	}
	if f.Issues == nil {
		f.Issues = map[int]*github.Issue{}
	}
	for i := 0; i < 999; i++ {
		if f.PullRequests[i] != nil || f.Issues[i] != nil {
			continue
		}
		f.PullRequests[i] = &github.PullRequest{
			Number: i,
			Base: github.PullRequestBranch{
				Ref:  base,
				Repo: github.Repo{Owner: github.User{Login: org}, Name: repo},
			},
		}
		f.Issues[i] = &github.Issue{Number: i}
		return i, nil
	}

	return 0, errors.New("FakeClient supports only 999 PullRequests")
}

func (f *FakeClient) UpdatePullRequest(org, repo string, number int, title, body *string, open *bool, branch *string, canModify *bool) error {
	f.lock.Lock()
	defer f.lock.Unlock()
	pr, found := f.PullRequests[number]
	if !found {
		return fmt.Errorf("no pr with number %d found", number)
	}
	if title != nil {
		pr.Title = *title
	}
	if body != nil {
		pr.Body = *body
	}
	return nil
}

// Query simply exists to allow the fake client to match the interface for packages that need it.
// It does not modify the passed interface at all.
func (f *FakeClient) Query(ctx context.Context, q interface{}, vars map[string]interface{}) error {
	return nil
}

// GetDirectory returns the contents of the file.
func (f *FakeClient) GetDirectory(org, repo, dir, commit string) ([]github.DirectoryContent, error) {
	contents, ok := f.RemoteDirectories[dir]
	if !ok {
		return nil, fmt.Errorf("could not find dir %s", dir)
	}
	if commit == "" {
		if master, ok := contents["master"]; ok {
			return master, nil
		}

		return nil, fmt.Errorf("could not find dir %s in master", dir)
	}

	if content, ok := contents[commit]; ok {
		return content, nil
	}

	return nil, fmt.Errorf("could not find dir %s with ref %s", dir, commit)
}
