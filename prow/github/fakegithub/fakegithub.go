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

type FakeClient struct {
	Issues             []github.Issue
	OrgMembers         []string
	IssueComments      map[int][]github.IssueComment
	IssueCommentID     int
	PullRequests       map[int]*github.PullRequest
	PullRequestChanges map[int][]github.PullRequestChange
	CombinedStatuses   map[string]*github.CombinedStatus

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

	// Fake remote git storage. File name are keys
	// and values map SHA to content
	RemoteFiles map[string]map[string]string
}

func (f *FakeClient) BotName() (string, error) {
	return "k8s-ci-robot", nil
}

func (f *FakeClient) IsMember(org, user string) (bool, error) {
	for _, m := range f.OrgMembers {
		if m == user {
			return true, nil
		}
	}
	return false, nil
}

func (f *FakeClient) ListIssueComments(owner, repo string, number int) ([]github.IssueComment, error) {
	return append([]github.IssueComment{}, f.IssueComments[number]...), nil
}

func (f *FakeClient) CreateComment(owner, repo string, number int, comment string) error {
	f.IssueCommentsAdded = append(f.IssueCommentsAdded, fmt.Sprintf("%s/%s#%d:%s", owner, repo, number, comment))
	f.IssueComments[number] = append(f.IssueComments[number], github.IssueComment{
		ID:   f.IssueCommentID,
		Body: comment,
	})
	f.IssueCommentID++
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

func (f *FakeClient) GetPullRequest(owner, repo string, number int) (*github.PullRequest, error) {
	return f.PullRequests[number], nil
}

func (f *FakeClient) GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error) {
	return f.PullRequestChanges[number], nil
}

func (f *FakeClient) GetRef(owner, repo, ref string) (string, error) {
	return "abcde", nil
}

func (f *FakeClient) CreateStatus(owner, repo, ref string, s github.Status) error {
	return nil
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
	re := regexp.MustCompile(fmt.Sprintf(`%s/%s#%d:([\w-]*)`, owner, repo, number))
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
