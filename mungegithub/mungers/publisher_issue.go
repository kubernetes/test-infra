/*
Copyright 2017 The Kubernetes Authors.

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

package mungers

import (
	"fmt"

	"github.com/golang/glog"

	githubapi "github.com/google/go-github/github"
	"k8s.io/test-infra/mungegithub/github"
)

const Title = "Repo publisher failure"

const owners = "@kubernetes-repo-publishing-cops"

// publisherFailure includes the information to create an issue
type publisherFailure struct {
	err error
	log string
}

func (p *publisherFailure) title() string {
	return Title
}

func (p *publisherFailure) body() string {
	body := p.err.Error()
	body += fmt.Sprintf("\n")
	body += fmt.Sprintf("cc %s", owners)
	body += fmt.Sprintf("\n")
	body += p.log
	return body
}

// publisherIssueTracker tracks latest open issue regarding publisher failure,
// can update the existing issue or create a new one.
type publisherIssueTracker struct {
	config             *github.Config
	lastestOpenIssue   *githubapi.Issue
	lastestIssueNumber int
}

func NewPublisherIssueTracker(config *github.Config) *publisherIssueTracker {
	return &publisherIssueTracker{config: config}
}

func (p *publisherIssueTracker) Init() error {
	issues, err := p.config.ListAllIssues(&githubapi.IssueListByRepoOptions{
		State: "open",
	})
	if err != nil {
		return err
	}
	for _, issue := range issues {
		if issue.Title != nil && *issue.Title == Title &&
			issue.Number != nil && *issue.Number > p.lastestIssueNumber {
			p.lastestIssueNumber = *issue.Number
			p.lastestOpenIssue = issue
		}
	}
	return nil
}

func (p *publisherIssueTracker) FileIssue(failure publisherFailure) {
	if p.lastestOpenIssue != nil {
		obj, err := p.config.GetObject(*p.lastestOpenIssue.Number)
		if err != nil {
			glog.Errorf("failed to update issue %v: %v", obj, err)
			return
		}
		err = obj.WriteComment(failure.body())
		if err != nil {
			glog.Errorf("failed to update issue %v: %v", obj, err)
			return
		}
		glog.Infof("updated issue %v", obj)
		return
	}

	obj, err := p.config.NewIssue(
		failure.title(),
		failure.body(),
		[]string{},
		"",
	)
	if err != nil {
		glog.Errorf("failed to create issue %v: %v", obj, err)
		return
	}
	glog.Infof("Created issue %v:\n%v", *obj.Issue.Number, failure.body())
}
