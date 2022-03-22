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

package regexbasedlabelmatch

import (
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
)

type FakeGitHub struct {
	Labels                               sets.String
	IssueLabelsAdded, IssueLabelsRemoved sets.String
	Commented                            bool
}

func NewFakeGitHub(initialLabels ...string) *FakeGitHub {
	return &FakeGitHub{
		Labels:             sets.NewString(initialLabels...),
		IssueLabelsAdded:   sets.NewString(),
		IssueLabelsRemoved: sets.NewString(),
	}
}

func (f *FakeGitHub) AddLabel(org, repo string, number int, label string) error {
	f.Labels.Insert(label)
	f.IssueLabelsAdded.Insert(label)
	return nil
}

func (f *FakeGitHub) RemoveLabel(org, repo string, number int, label string) error {
	f.Labels.Delete(label)
	f.IssueLabelsRemoved.Insert(label)
	return nil
}

func (f *FakeGitHub) CreateComment(org, repo string, number int, content string) error {
	f.Commented = true
	return nil
}

func (f *FakeGitHub) GetIssueLabels(org, repo string, number int) ([]github.Label, error) {
	res := make([]github.Label, 0, len(f.Labels))
	for label := range f.Labels {
		res = append(res, github.Label{Name: label})
	}
	return res, nil
}

func (f *FakeGitHub) GetPullRequest(org, repo string, number int) (*github.PullRequest, error) {
	res := &github.PullRequest{}
	return res, nil
}

type FakePruner struct{}

func (fp *FakePruner) PruneComments(shouldPrune func(github.IssueComment) bool) {}
