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
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/andygrunwald/go-jira"
	"github.com/sirupsen/logrus"

	jiraclient "k8s.io/test-infra/prow/jira"
)

type FakeClient struct {
	Issues            []*jira.Issue
	ExistingLinks     map[string][]jira.RemoteLink
	NewLinks          []jira.RemoteLink
	IssueLinks        []*jira.IssueLink
	GetIssueError     error
	Transitions       []jira.Transition
	Users             []*jira.User
	SearchResponses   map[SearchRequest]SearchResponse
	GetIssueResponses map[GetIssueRequest]GetIssueResponse
}

func (f *FakeClient) ListProjects() (*jira.ProjectList, error) {
	return nil, nil
}

func (f *FakeClient) GetIssue(id string) (*jira.Issue, error) {
	if f.GetIssueError != nil {
		return nil, f.GetIssueError
	}
	for _, existingIssue := range f.Issues {
		if existingIssue.ID == id || existingIssue.Key == id {
			return existingIssue, nil
		}
	}
	return nil, jiraclient.NewNotFoundError(fmt.Errorf("No issue %s found", id))
}

type GetIssueRequest struct {
	issueID string
	options *jira.GetQueryOptions
}

type GetIssueResponse struct {
	issue *jira.Issue
	error error
}

func (f *FakeClient) GetIssueWithOptions(id string, options *jira.GetQueryOptions) (*jira.Issue, error) {
	resp, expected := f.GetIssueResponses[GetIssueRequest{issueID: id, options: options}]
	if !expected {
		return nil, fmt.Errorf("the filtering query: %v for the issue with ID: %s is not registered", options, id)
	}
	return resp.issue, resp.error
}

func (f *FakeClient) GetRemoteLinks(id string) ([]jira.RemoteLink, error) {
	return f.ExistingLinks[id], nil
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
	issue.Fields.Comments.Comments = append(issue.Fields.Comments.Comments, comment)
	return comment, nil
}

func (f *FakeClient) CreateIssueLink(link *jira.IssueLink) error {
	outward, err := f.GetIssue(link.OutwardIssue.ID)
	if err != nil {
		return fmt.Errorf("failed to get outward link issue: %v", err)
	}
	outward.Fields.IssueLinks = append(outward.Fields.IssueLinks, link)
	inward, err := f.GetIssue(link.InwardIssue.ID)
	if err != nil {
		return fmt.Errorf("failed to get inward link issue: %v", err)
	}
	inward.Fields.IssueLinks = append(inward.Fields.IssueLinks, link)
	f.IssueLinks = append(f.IssueLinks, link)
	return nil
}

func (f *FakeClient) CloneIssue(issue *jira.Issue) (*jira.Issue, error) {
	// create deep copy of parent so we can modify key and id for child
	data, err := json.Marshal(issue)
	if err != nil {
		return nil, err
	}
	issueCopy := &jira.Issue{}
	err = json.Unmarshal(data, issueCopy)
	if err != nil {
		return nil, err
	}
	// set ID and Key to unused id and key
	f.updateIssueIDAndKey(issueCopy)
	// run generic cloning function
	return jiraclient.CloneIssue(f, issueCopy)
}

func (f *FakeClient) CreateIssue(issue *jira.Issue) (*jira.Issue, error) {
	// check that there are no ID collisions
	for _, currIssue := range f.Issues {
		if currIssue.ID == issue.ID {
			return nil, fmt.Errorf("Issue ID %s already exists", issue.ID)
		}
		if currIssue.Key == issue.Key {
			return nil, fmt.Errorf("Issue key %s already exists", issue.Key)
		}
	}
	f.updateIssueIDAndKey(issue)
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
			f.ExistingLinks[issueID] = append(f.ExistingLinks[issueID][:index], f.ExistingLinks[issueID][index+1:]...)
		}
	}
	return fmt.Errorf("failed to find link id %d in issue %s", linkID, issueID)
}

func (f *FakeClient) DeleteRemoteLinkViaURL(issueID, url string) error {
	return jiraclient.DeleteRemoteLinkViaURL(f, issueID, url)
}

func (f *FakeClient) updateIssueIDAndKey(newIssue *jira.Issue) error {
	// ensure that a key is set
	if newIssue.Key == "" {
		return errors.New("Issue key must be set")
	}
	// ensure key format is correct
	splitKey := strings.Split(newIssue.Key, "-")
	if len(splitKey) != 2 {
		return fmt.Errorf("Invalid issue key: %s", newIssue.Key)
	}

	// find highest issueID and make new issue one higher
	highestID := -1
	for _, issue := range f.Issues {
		// all IDs are ints, but represented as strings...
		intID, _ := strconv.Atoi(issue.ID)
		if intID > highestID {
			highestID = intID
		}
	}
	newIssue.ID = strconv.Itoa(highestID + 1)
	// if there are issues in the same project, make new issue one above those
	highestKeyID := 0
	keyPrefix := fmt.Sprintf("%s-", splitKey[0])
	for _, issue := range f.Issues {
		if strings.HasPrefix(issue.Key, keyPrefix) {
			stringID := strings.TrimPrefix(issue.Key, keyPrefix)
			intID, _ := strconv.Atoi(stringID)
			if intID > highestKeyID {
				highestKeyID = intID
			}
		}
	}
	newIssue.Key = fmt.Sprintf("%s%d", keyPrefix, highestKeyID)
	return nil
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
	return jiraclient.GetIssueSecurityLevel(f, issue)
}

func (f *FakeClient) UpdateIssue(issue *jira.Issue) (*jira.Issue, error) {
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
	var newFields *jira.IssueFields
	if err := json.Unmarshal(updatedIssueBytes, newFields); err != nil {
		return nil, fmt.Errorf("failed converting updated issue to struct: %v", err)
	}
	retrievedIssue.Fields = newFields
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
