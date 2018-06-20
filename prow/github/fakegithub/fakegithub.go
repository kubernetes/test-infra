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

	"k8s.io/test-infra/prow/github"
)

const botName = "k8s-ci-robot"

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

	//All Labels That Exist In The Repo
	ExistingLabels []string
	// org/repo#number:label
	LabelsAdded   []string
	LabelsRemoved []string

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

func (f *FakeClient) BotName() (string, error) {
	return botName, nil
}

func (f *FakeClient) IsMember(org, user string) (bool, error) {
	for _, m := range f.OrgMembers[org] {
		if m == user {
			return true, nil
		}
	}
	return false, nil
}

func (f *FakeClient) ListIssueComments(owner, repo string, number int) ([]github.IssueComment, error) {
	return append([]github.IssueComment{}, f.IssueComments[number]...), nil
}

func (f *FakeClient) ListPullRequestComments(owner, repo string, number int) ([]github.ReviewComment, error) {
	return append([]github.ReviewComment{}, f.PullRequestComments[number]...), nil
}

func (f *FakeClient) ListReviews(owner, repo string, number int) ([]github.Review, error) {
	return append([]github.Review{}, f.Reviews[number]...), nil
}

func (f *FakeClient) ListIssueEvents(owner, repo string, number int) ([]github.ListedIssueEvent, error) {
	return append([]github.ListedIssueEvent{}, f.IssueEvents[number]...), nil
}

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

func (f *FakeClient) CreateReview(org, repo string, number int, r github.DraftReview) error {
	f.Reviews[number] = append(f.Reviews[number], github.Review{
		ID:   f.ReviewID,
		User: github.User{Login: botName},
		Body: r.Body,
	})
	f.ReviewID++
	return nil
}

func (f *FakeClient) CreateCommentReaction(org, repo string, ID int, reaction string) error {
	f.CommentReactionsAdded = append(f.CommentReactionsAdded, fmt.Sprintf("%s/%s#%d:%s", org, repo, ID, reaction))
	return nil
}

func (f *FakeClient) CreateIssueReaction(org, repo string, ID int, reaction string) error {
	f.IssueReactionsAdded = append(f.IssueReactionsAdded, fmt.Sprintf("%s/%s#%d:%s", org, repo, ID, reaction))
	return nil
}

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

func (f *FakeClient) GetPullRequest(owner, repo string, number int) (*github.PullRequest, error) {
	return f.PullRequests[number], nil
}

func (f *FakeClient) GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error) {
	return f.PullRequestChanges[number], nil
}

func (f *FakeClient) GetRef(owner, repo, ref string) (string, error) {
	return "abcde", nil
}

func (f *FakeClient) CreateStatus(owner, repo, sha string, s github.Status) error {
	if f.CreatedStatuses == nil {
		f.CreatedStatuses = make(map[string][]github.Status)
	}
	statuses := f.CreatedStatuses[sha]
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
	f.CreatedStatuses[sha] = statuses
	return nil
}

func (f *FakeClient) ListStatuses(org, repo, ref string) ([]github.Status, error) {
	return f.CreatedStatuses[ref], nil
}

func (f *FakeClient) GetCombinedStatus(owner, repo, ref string) (*github.CombinedStatus, error) {
	return f.CombinedStatuses[ref], nil
}

func (f *FakeClient) GetRepoLabels(owner, repo string) ([]github.Label, error) {
	la := []github.Label{}
	for _, l := range f.ExistingLabels {
		la = append(la, github.Label{Name: l})
	}
	return la, nil
}

func (f *FakeClient) GetIssueLabels(owner, repo string, number int) ([]github.Label, error) {
	// Only labels added to an issue are considered. Removals are ignored by this fake.
	re := regexp.MustCompile(fmt.Sprintf(`^%s/%s#%d:(.*)$`, owner, repo, number))
	la := []github.Label{}
	for _, l := range f.LabelsAdded {
		groups := re.FindStringSubmatch(l)
		if groups != nil {
			la = append(la, github.Label{Name: groups[1]})
		}
	}
	return la, nil
}

func (f *FakeClient) AddLabel(owner, repo string, number int, label string) error {
	if f.ExistingLabels == nil {
		f.LabelsAdded = append(f.LabelsAdded, fmt.Sprintf("%s/%s#%d:%s", owner, repo, number, label))
		return nil
	}
	for _, l := range f.ExistingLabels {
		if label == l {
			f.LabelsAdded = append(f.LabelsAdded, fmt.Sprintf("%s/%s#%d:%s", owner, repo, number, label))
			return nil
		}
	}
	return fmt.Errorf("cannot add %v to %s/%s/#%d", label, owner, repo, number)
}

func (f *FakeClient) RemoveLabel(owner, repo string, number int, label string) error {
	f.LabelsRemoved = append(f.LabelsRemoved, fmt.Sprintf("%s/%s#%d:%s", owner, repo, number, label))
	return nil
}

// FindIssues returns f.Issues
func (f *FakeClient) FindIssues(query, sort string, asc bool) ([]github.Issue, error) {
	return f.Issues, nil
}

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

// ListTeamMembers return a fake team with a single "sig-lead" Github teammember
func (f *FakeClient) ListTeamMembers(teamID int, role string) ([]github.TeamMember, error) {
	if role != github.RoleAll {
		return nil, fmt.Errorf("unsupport role %v (only all supported)", role)
	}
	return []github.TeamMember{{Login: "sig-lead"}}, nil
}

func (f *FakeClient) IsCollaborator(org, repo, login string) (bool, error) {
	normed := github.NormLogin(login)
	for _, collab := range f.Collaborators {
		if github.NormLogin(collab) == normed {
			return true, nil
		}
	}
	return false, nil
}

func (f *FakeClient) ListCollaborators(org, repo string) ([]github.User, error) {
	result := make([]github.User, 0, len(f.Collaborators))
	for _, login := range f.Collaborators {
		result = append(result, github.User{Login: login})
	}
	return result, nil
}

func (f *FakeClient) ClearMilestone(org, repo string, issueNum int) error {
	f.Milestone = 0
	return nil
}

func (f *FakeClient) SetMilestone(org, repo string, issueNum, milestoneNum int) error {
	if milestoneNum < 0 {
		return fmt.Errorf("Milestone Numbers Cannot Be Negative")
	}
	f.Milestone = milestoneNum
	return nil
}

func (f *FakeClient) ListMilestones(org, repo string) ([]github.Milestone, error) {
	milestones := []github.Milestone{}
	for k, v := range f.MilestoneMap {
		milestones = append(milestones, github.Milestone{Title: k, Number: v})
	}
	return milestones, nil
}
