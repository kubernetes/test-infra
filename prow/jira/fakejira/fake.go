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
	"fmt"

	"github.com/andygrunwald/go-jira"
	jiraclient "k8s.io/test-infra/prow/jira"
)

type FakeClient struct {
	ExistingIssues []jira.Issue
	ExistingLinks  map[string][]jira.RemoteLink
	NewLinks       []jira.RemoteLink
	GetIssueError  error
}

func (f *FakeClient) ListProjects() (*jira.ProjectList, error) {
	return nil, nil
}

func (f *FakeClient) GetIssue(id string) (*jira.Issue, error) {
	if f.GetIssueError != nil {
		return nil, f.GetIssueError
	}
	for _, existingIssue := range f.ExistingIssues {
		if existingIssue.ID == id {
			return &existingIssue, nil
		}
	}
	return nil, jiraclient.NewNotFoundError(fmt.Errorf("No issue %s found", id))
}

func (f *FakeClient) GetRemoteLinks(id string) ([]jira.RemoteLink, error) {
	return f.ExistingLinks[id], nil
}

func (f *FakeClient) AddRemoteLink(id string, link *jira.RemoteLink) error {
	if _, err := f.GetIssue(id); err != nil {
		return err
	}
	f.NewLinks = append(f.NewLinks, *link)
	return nil
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
