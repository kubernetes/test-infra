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
	"fmt"
	"regexp"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
)

const botName = "k8s-ci-robot"

// Bot is the exported botName
const Bot = botName

// FakeClient is like client, but fake.
type FakeClient struct {
	Issues              []github.Issue
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
	Commits             map[string]github.SingleCommit

	//All Labels That Exist In The Repo
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

	// Fake remote git storage. File name are keys
	// and values map SHA to content
	RemoteFiles map[string]map[string]string
}

// BotName returns authenticated login.
func (f *FakeClient) BotName() (string, error) {
	return botName, nil
}

// IsMember returns true if user is in org.
func (f *FakeClient) IsMember(org, user string) (bool, error) {
	for _, m := range f.OrgMembers[org] {
		if m == user {
			return true, nil
		}
	}
	return false, nil
}

// ListIssueComments returns comments.
func (f *FakeClient) ListIssueComments(owner, repo string, number int) ([]github.IssueComment, error) {
	return append([]github.IssueComment{}, f.IssueComments[number]...), nil
}

// ListPullRequestComments returns review comments.
func (f *FakeClient) ListPullRequestComments(owner, repo string, number int) ([]github.ReviewComment, error) {
	return append([]github.ReviewComment{}, f.PullRequestComments[number]...), nil
}

// ListReviews returns reviews.
func (f *FakeClient) ListReviews(owner, repo string, number int) ([]github.Review, error) {
	return append([]github.Review{}, f.Reviews[number]...), nil
}

// ListIssueEvents returns issue events
func (f *FakeClient) ListIssueEvents(owner, repo string, number int) ([]github.ListedIssueEvent, error) {
	return append([]github.ListedIssueEvent{}, f.IssueEvents[number]...), nil
}

// CreateComment adds a comment to a PR
func (f *FakeClient) CreateComment(owner, repo string, number int, comment string) error {
	f.IssueCommentsAdded = append(f.IssueCommentsAdded, fmt.Sprintf("%s/%s#%d:%s", owner, repo, number, comment))
	f.IssueComments[number] = append(f.IssueComments[number], github.IssueComment{
		ID:   f.IssueCommentID,
		Body: comment,
		User: github.User{Login: botName},
	})
	f.IssueCommentID++
	return nil
}

// CreateReview adds a review to a PR
func (f *FakeClient) CreateReview(org, repo string, number int, r github.DraftReview) error {
	f.Reviews[number] = append(f.Reviews[number], github.Review{
		ID:   f.ReviewID,
		User: github.User{Login: botName},
		Body: r.Body,
	})
	f.ReviewID++
	return nil
}

// CreateCommentReaction adds emoji to a comment.
func (f *FakeClient) CreateCommentReaction(org, repo string, ID int, reaction string) error {
	f.CommentReactionsAdded = append(f.CommentReactionsAdded, fmt.Sprintf("%s/%s#%d:%s", org, repo, ID, reaction))
	return nil
}

// CreateIssueReaction adds an emoji to an issue.
func (f *FakeClient) CreateIssueReaction(org, repo string, ID int, reaction string) error {
	f.IssueReactionsAdded = append(f.IssueReactionsAdded, fmt.Sprintf("%s/%s#%d:%s", org, repo, ID, reaction))
	return nil
}

// DeleteComment deletes a comment.
func (f *FakeClient) DeleteComment(owner, repo string, ID int) error {
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
	return f.PullRequests[number], nil
}

// GetPullRequestChanges returns the file modifications in a PR.
func (f *FakeClient) GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error) {
	return f.PullRequestChanges[number], nil
}

// GetRef returns the hash of a ref.
func (f *FakeClient) GetRef(owner, repo, ref string) (string, error) {
	return "abcde", nil
}

// GetSingleCommit returns a single commit.
func (f *FakeClient) GetSingleCommit(org, repo, SHA string) (github.SingleCommit, error) {
	return f.Commits[SHA], nil
}

// CreateStatus adds a status context to a commit.
func (f *FakeClient) CreateStatus(owner, repo, SHA string, s github.Status) error {
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
	return nil
}

// ListStatuses returns individual status contexts on a commit.
func (f *FakeClient) ListStatuses(org, repo, ref string) ([]github.Status, error) {
	return f.CreatedStatuses[ref], nil
}

// GetCombinedStatus returns the overall status for a commit.
func (f *FakeClient) GetCombinedStatus(owner, repo, ref string) (*github.CombinedStatus, error) {
	return f.CombinedStatuses[ref], nil
}

// GetRepoLabels gets labels in a repo.
func (f *FakeClient) GetRepoLabels(owner, repo string) ([]github.Label, error) {
	la := []github.Label{}
	for _, l := range f.RepoLabelsExisting {
		la = append(la, github.Label{Name: l})
	}
	return la, nil
}

// GetIssueLabels gets labels on an issue
func (f *FakeClient) GetIssueLabels(owner, repo string, number int) ([]github.Label, error) {
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
			return nil
		}
	}
	return fmt.Errorf("cannot add %v to %s/%s/#%d", label, owner, repo, number)
}

// RemoveLabel removes a label
func (f *FakeClient) RemoveLabel(owner, repo string, number int, label string) error {
	labelString := fmt.Sprintf("%s/%s#%d:%s", owner, repo, number, label)
	if !sets.NewString(f.IssueLabelsRemoved...).Has(labelString) {
		f.IssueLabelsRemoved = append(f.IssueLabelsRemoved, labelString)
		return nil
	}
	return fmt.Errorf("cannot remove %v from %s/%s/#%d", label, owner, repo, number)
}

// FindIssues returns f.Issues
func (f *FakeClient) FindIssues(query, sort string, asc bool) ([]github.Issue, error) {
	return f.Issues, nil
}

// AssignIssue adds assignees.
func (f *FakeClient) AssignIssue(owner, repo string, number int, assignees []string) error {
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

// ListTeamMembers return a fake team with a single "sig-lead" Github teammember
func (f *FakeClient) ListTeamMembers(teamID int, role string) ([]github.TeamMember, error) {
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
	if milestoneNum < 0 {
		return fmt.Errorf("Milestone Numbers Cannot Be Negative")
	}
	f.Milestone = milestoneNum
	return nil
}

// ListMilestones lists milestones.
func (f *FakeClient) ListMilestones(org, repo string) ([]github.Milestone, error) {
	milestones := []github.Milestone{}
	for k, v := range f.MilestoneMap {
		milestones = append(milestones, github.Milestone{Title: k, Number: v})
	}
	return milestones, nil
}
