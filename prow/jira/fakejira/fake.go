/*
Copyright 2022 The Kubernetes Authors.

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

package fakejira

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/andygrunwald/go-jira"
	"github.com/sirupsen/logrus"

	jiraclient "k8s.io/test-infra/prow/jira"
)

type FakeClient struct {
	Issues           []*jira.Issue
	ExistingLinks    map[string][]jira.RemoteLink
	NewLinks         []jira.RemoteLink
	RemovedLinks     []jira.RemoteLink
	IssueLinks       []*jira.IssueLink
	GetIssueError    map[string]error
	CreateIssueError map[string]error
	UpdateIssueError map[string]error
	Transitions      []jira.Transition
	Users            []*jira.User
	SearchResponses  map[SearchRequest]SearchResponse
}

func (f *FakeClient) ListProjects() (*jira.ProjectList, error) {
	return nil, nil
}

func (f *FakeClient) GetIssue(id string) (*jira.Issue, error) {
	if f.GetIssueError != nil {
		if err, ok := f.GetIssueError[id]; ok {
			return nil, err
		}
	}
	for _, existingIssue := range f.Issues {
		if existingIssue.ID == id || existingIssue.Key == id {
			return existingIssue, nil
		}
	}
	return nil, jiraclient.NewNotFoundError(fmt.Errorf("No issue %s found", id))
}

func (f *FakeClient) GetRemoteLinks(id string) ([]jira.RemoteLink, error) {
	issue, err := f.GetIssue(id)
	if err != nil {
		return nil, fmt.Errorf("Failed to get issue when chekcing from remote links: %+v", err)
	}
	return append(f.ExistingLinks[issue.ID], f.ExistingLinks[issue.Key]...), nil
}

func (f *FakeClient) AddRemoteLink(id string, link *jira.RemoteLink) (*jira.RemoteLink, error) {
	if _, err := f.GetIssue(id); err != nil {
		return nil, err
	}
	f.NewLinks = append(f.NewLinks, *link)
	return link, nil
}

func (f *FakeClient) JiraClient() *jira.Client {
	panic("not implemented")
}

const FakeJiraUrl = "https://my-jira.com"

func (f *FakeClient) JiraURL() string {
	return FakeJiraUrl
}

func (f *FakeClient) UpdateRemoteLink(id string, link *jira.RemoteLink) error {
	if _, err := f.GetIssue(id); err != nil {
		return err
	}
	if _, found := f.ExistingLinks[id]; !found {
		return jiraclient.NewNotFoundError(fmt.Errorf("Link for issue %s not found", id))
	}
	f.NewLinks = append(f.NewLinks, *link)
	return nil
}

func (f *FakeClient) Used() bool {
	return true
}

func (f *FakeClient) WithFields(fields logrus.Fields) jiraclient.Client {
	return f
}

func (f *FakeClient) ForPlugin(string) jiraclient.Client {
	return f
}

func (f *FakeClient) AddComment(issueID string, comment *jira.Comment) (*jira.Comment, error) {
	issue, err := f.GetIssue(issueID)
	if err != nil {
		return nil, fmt.Errorf("Issue %s not found: %v", issueID, err)
	}
	// make sure the fields exist
	if issue.Fields == nil {
		issue.Fields = &jira.IssueFields{}
	}
	if issue.Fields.Comments == nil {
		issue.Fields.Comments = &jira.Comments{}
	}
	issue.Fields.Comments.Comments = append(issue.Fields.Comments.Comments, comment)
	return comment, nil
}

func (f *FakeClient) CreateIssueLink(link *jira.IssueLink) error {
	outward, err := f.GetIssue(link.OutwardIssue.ID)
	if err != nil {
		return fmt.Errorf("failed to get outward link issue: %v", err)
	}
	// when part of an issue struct, the issue link type does not include the
	// short definition of the issue it is in
	linkForOutward := *link
	linkForOutward.OutwardIssue = nil
	outward.Fields.IssueLinks = append(outward.Fields.IssueLinks, &linkForOutward)
	inward, err := f.GetIssue(link.InwardIssue.ID)
	if err != nil {
		return fmt.Errorf("failed to get inward link issue: %v", err)
	}
	linkForInward := *link
	linkForInward.InwardIssue = nil
	inward.Fields.IssueLinks = append(inward.Fields.IssueLinks, &linkForInward)
	f.IssueLinks = append(f.IssueLinks, link)
	return nil
}

func (f *FakeClient) CloneIssue(issue *jira.Issue) (*jira.Issue, error) {
	return jiraclient.CloneIssue(f, issue)
}

func (f *FakeClient) CreateIssue(issue *jira.Issue) (*jira.Issue, error) {
	if f.CreateIssueError != nil {
		if err, ok := f.CreateIssueError[issue.Key]; ok {
			return nil, err
		}
	}
	if issue.Fields == nil {
		issue.Fields = &jira.IssueFields{}
	}
	// find highest issueID and make new issue one higher
	highestID := 0
	// find highest ID for issues in the same project to make new key one higher
	highestKeyID := 0
	keyPrefix := issue.Fields.Project.Name + "-"
	for _, issue := range f.Issues {
		// all IDs are ints, but represented as strings...
		intID, _ := strconv.Atoi(issue.ID)
		if intID > highestID {
			highestID = intID
		}
		if strings.HasPrefix(issue.Key, keyPrefix) {
			stringID := strings.TrimPrefix(issue.Key, keyPrefix)
			intID, _ := strconv.Atoi(stringID)
			if intID > highestKeyID {
				highestKeyID = intID
			}
		}
	}
	issue.ID = strconv.Itoa(highestID + 1)
	issue.Key = fmt.Sprintf("%s%d", keyPrefix, highestKeyID+1)
	f.Issues = append(f.Issues, issue)
	return issue, nil
}

func (f *FakeClient) DeleteLink(id string) error {
	// find link
	var link *jira.IssueLink
	var linkIndex int
	for index, currLink := range f.IssueLinks {
		if currLink.ID == id {
			link = currLink
			linkIndex = index
			break
		}
	}
	if link == nil {
		return fmt.Errorf("no issue link with id %s found", id)
	}
	outward, err := f.GetIssue(link.OutwardIssue.ID)
	if err != nil {
		return fmt.Errorf("failed to get outward link issue: %v", err)
	}
	outwardIssueIndex := -1
	for index, currLink := range outward.Fields.IssueLinks {
		if currLink.ID == link.ID {
			outwardIssueIndex = index
			break
		}
	}
	// should we error if link doesn't exist in one of the linked issues?
	if outwardIssueIndex != -1 {
		outward.Fields.IssueLinks = append(outward.Fields.IssueLinks[:outwardIssueIndex], outward.Fields.IssueLinks[outwardIssueIndex+1:]...)
	}
	inward, err := f.GetIssue(link.InwardIssue.ID)
	if err != nil {
		return fmt.Errorf("failed to get inward link issue: %v", err)
	}
	inwardIssueIndex := -1
	for index, currLink := range inward.Fields.IssueLinks {
		if currLink.ID == link.ID {
			inwardIssueIndex = index
			break
		}
	}
	// should we error if link doesn't exist in one of the linked issues?
	if inwardIssueIndex != -1 {
		inward.Fields.IssueLinks = append(inward.Fields.IssueLinks[:inwardIssueIndex], inward.Fields.IssueLinks[inwardIssueIndex+1:]...)
	}
	f.IssueLinks = append(f.IssueLinks[:linkIndex], f.IssueLinks[linkIndex+1:]...)
	return nil
}

// TODO: improve handling of remote links in fake client; having a separate NewLinks struct
// that contains links not in existingLinks may limit some aspects of testing
func (f *FakeClient) DeleteRemoteLink(issueID string, linkID int) error {
	for index, remoteLink := range f.ExistingLinks[issueID] {
		if remoteLink.ID == linkID {
			f.RemovedLinks = append(f.RemovedLinks, remoteLink)
			if len(f.ExistingLinks[issueID]) == index+1 {
				f.ExistingLinks[issueID] = f.ExistingLinks[issueID][:index]
			} else {
				f.ExistingLinks[issueID] = append(f.ExistingLinks[issueID][:index], f.ExistingLinks[issueID][index+1:]...)
			}
			return nil
		}
	}
	return fmt.Errorf("failed to find link id %d in issue %s", linkID, issueID)
}

func (f *FakeClient) DeleteRemoteLinkViaURL(issueID, url string) (bool, error) {
	return jiraclient.DeleteRemoteLinkViaURL(f, issueID, url)
}

func (f *FakeClient) GetTransitions(issueID string) ([]jira.Transition, error) {
	return f.Transitions, nil
}

func (f *FakeClient) DoTransition(issueID, transitionID string) error {
	issue, err := f.GetIssue(issueID)
	if err != nil {
		return fmt.Errorf("could not find issue: %v", err)
	}
	var correctTransition *jira.Transition
	for index, transition := range f.Transitions {
		if transition.ID == transitionID {
			correctTransition = &f.Transitions[index]
			break
		}
	}
	if correctTransition == nil {
		return fmt.Errorf("could not find transition with ID %s", transitionID)
	}
	issue.Fields.Status = &correctTransition.To
	return nil
}

func (f *FakeClient) FindUser(property string) ([]*jira.User, error) {
	var foundUsers []*jira.User
	for _, user := range f.Users {
		// multiple different fields can be matched with this query
		if strings.Contains(user.AccountID, property) ||
			strings.Contains(user.DisplayName, property) ||
			strings.Contains(user.EmailAddress, property) ||
			strings.Contains(user.Name, property) {
			foundUsers = append(foundUsers, user)
		}
	}
	if len(foundUsers) == 0 {
		return nil, fmt.Errorf("Not users found with property %s", property)
	}
	return foundUsers, nil
}

func (f *FakeClient) GetIssueSecurityLevel(issue *jira.Issue) (*jiraclient.SecurityLevel, error) {
	return jiraclient.GetIssueSecurityLevel(issue)
}

func (f *FakeClient) GetIssueQaContact(issue *jira.Issue) (*jira.User, error) {
	return jiraclient.GetIssueQaContact(issue)
}

func (f *FakeClient) GetIssueTargetVersion(issue *jira.Issue) (*[]*jira.Version, error) {
	return jiraclient.GetIssueTargetVersion(issue)
}

func (f *FakeClient) UpdateIssue(issue *jira.Issue) (*jira.Issue, error) {
	if f.UpdateIssueError != nil {
		if err, ok := f.UpdateIssueError[issue.ID]; ok {
			return nil, err
		}
	}
	retrievedIssue, err := f.GetIssue(issue.ID)
	if err != nil {
		return nil, fmt.Errorf("unable to find issue to update: %v", err)
	}
	// convert `fields` field of both retrieved and provided issue to interfaces and update the non-nil
	// fields from the provided issue to the retrieved one
	var issueFields, retrievedFields map[string]interface{}
	issueBytes, err := json.Marshal(issue.Fields)
	if err != nil {
		return nil, fmt.Errorf("error converting provided issue to json: %v", err)
	}
	if err := json.Unmarshal(issueBytes, &issueFields); err != nil {
		return nil, fmt.Errorf("failed converting provided issue to map: %v", err)
	}
	retrievedIssueBytes, err := json.Marshal(retrievedIssue.Fields)
	if err != nil {
		return nil, fmt.Errorf("error converting original issue to json: %v", err)
	}
	if err := json.Unmarshal(retrievedIssueBytes, &retrievedFields); err != nil {
		return nil, fmt.Errorf("failed converting original issue to map: %v", err)
	}
	for key, value := range issueFields {
		retrievedFields[key] = value
	}
	updatedIssueBytes, err := json.Marshal(retrievedFields)
	if err != nil {
		return nil, fmt.Errorf("error converting updated issue to json: %v", err)
	}
	var newFields jira.IssueFields
	if err := json.Unmarshal(updatedIssueBytes, &newFields); err != nil {
		return nil, fmt.Errorf("failed converting updated issue to struct: %v", err)
	}
	retrievedIssue.Fields = &newFields
	return retrievedIssue, nil
}

func (f *FakeClient) UpdateStatus(issueID, statusName string) error {
	return jiraclient.UpdateStatus(f, issueID, statusName)
}

type SearchRequest struct {
	query   string
	options *jira.SearchOptions
}

type SearchResponse struct {
	issues   []jira.Issue
	response *jira.Response
	error    error
}

func (f *FakeClient) SearchWithContext(ctx context.Context, jql string, options *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
	resp, expected := f.SearchResponses[SearchRequest{query: jql, options: options}]
	if !expected {
		return nil, nil, fmt.Errorf("the query: %s is not registered", jql)
	}
	return resp.issues, resp.response, resp.error
}
