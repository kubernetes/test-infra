/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package pulls

import (
	"fmt"
	"strings"

	"k8s.io/contrib/mungegithub/opts"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
)

var mungerMap = map[string]Munger{}

type Munger interface {
	// Take action on a specific pull request includes:
	//   * The org/user pair for the PR
	//   * The PR object
	//   * The issue object for the PR, github stores some things (e.g. labels) in an "issue" object with the same number as the PR
	//   * The commits for the PR
	//   * The events on the PR
	//   * dryrun, if true, the munger should take no action, and only report what it would have done.
	MungePullRequest(client *github.Client, pr *github.PullRequest, issue *github.Issue, commits []github.RepositoryCommit, events []github.IssueEvent, opts opts.MungeOptions)
	Name() string
}

func getMungers(mungers []string) ([]Munger, error) {
	result := make([]Munger, len(mungers))
	for ix := range mungers {
		munger, found := mungerMap[mungers[ix]]
		if !found {
			return nil, fmt.Errorf("Couldn't find a munger named: %s", mungers[ix])
		}
		result[ix] = munger
	}
	return result, nil
}

func RegisterMunger(munger Munger) error {
	if _, found := mungerMap[munger.Name()]; found {
		return fmt.Errorf("A munger with that name (%s) already exists", munger.Name())
	}
	mungerMap[munger.Name()] = munger
	return nil
}

func RegisterMungerOrDie(munger Munger) {
	if err := RegisterMunger(munger); err != nil {
		glog.Fatalf("Failed to register munger: %s", err)
	}
}

func MungePullRequests(client *github.Client, pullMungers string, opts opts.MungeOptions) error {
	mungers, err := getMungers(strings.Split(pullMungers, ","))
	if err != nil {
		return err
	}

	page := 0
	for {
		glog.V(4).Infof("Fetching page %d", page)
		listOpts := &github.PullRequestListOptions{
			Sort:        "desc",
			ListOptions: github.ListOptions{PerPage: 100, Page: page},
		}
		prs, response, err := client.PullRequests.List(opts.Org, opts.Project, listOpts)
		if err != nil {
			return err
		}
		if err := mungePullRequestList(prs, client, mungers, opts); err != nil {
			return err
		}
		if response.LastPage == 0 || response.LastPage == page {
			break
		}
		page = response.NextPage
	}
	return nil
}

func mungePullRequestList(list []github.PullRequest, client *github.Client, mungers []Munger, opts opts.MungeOptions) error {
	for ix := range list {
		pr := &list[ix]
		glog.V(2).Infof("-=-=-=-=%d-=-=-=-=-", *pr.Number)
		if *pr.Number < opts.MinPRNumber {
			glog.V(3).Infof("skipping %d less %d", *pr.Number, opts.MinPRNumber)
			continue
		}
		if p, _, err := client.PullRequests.Get(opts.Org, opts.Project, *pr.Number); err != nil {
			return err
		} else {
			*pr = *p
		}
		commits, _, err := client.PullRequests.ListCommits(opts.Org, opts.Project, *pr.Number, &github.ListOptions{})
		if err != nil {
			return err
		}
		filledCommits := []github.RepositoryCommit{}
		for _, c := range commits {
			commit, _, err := client.Repositories.GetCommit(opts.Org, opts.Project, *c.SHA)
			if err != nil {
				glog.Errorf("Can't load commit %s %s %s", opts.Org, opts.Project, *commit.SHA)
				continue
			}
			filledCommits = append(filledCommits, *commit)
		}
		events, _, err := client.Issues.ListIssueEvents(opts.Org, opts.Project, *pr.Number, &github.ListOptions{})
		if err != nil {
			return err
		}
		issue, _, err := client.Issues.Get(opts.Org, opts.Project, *pr.Number)
		if err != nil {
			return err
		}
		for _, munger := range mungers {
			munger.MungePullRequest(client, pr, issue, filledCommits, events, opts)
		}
	}
	return nil
}

func HasLabel(labels []github.Label, name string) bool {
	for _, label := range labels {
		if label.Name != nil && *label.Name == name {
			return true
		}
	}
	return false
}

func GetLabelsWithPrefix(labels []github.Label, prefix string) []string {
	var ret []string
	for _, label := range labels {
		if label.Name != nil && strings.HasPrefix(*label.Name, prefix) {
			ret = append(ret, *label.Name)
		}
	}
	return ret
}
