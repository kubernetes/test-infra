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
	"github.com/kubernetes/test-infra/ciongke/github"
)

type FakeClient struct {
	OrgMembers    []string
	IssueComments map[int][]github.IssueComment
	PullRequests  map[int]*github.PullRequest
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
	return f.IssueComments[number], nil
}

func (f *FakeClient) CreateComment(owner, repo string, number int, comment string) error {
	return nil
}

func (f *FakeClient) GetPullRequest(owner, repo string, number int) (*github.PullRequest, error) {
	return f.PullRequests[number], nil
}

func (f *FakeClient) CreateStatus(owner, repo, ref string, s github.Status) error {
	return nil
}
