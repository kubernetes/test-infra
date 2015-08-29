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

package issues

import (
	"fmt"
	"strings"

	"k8s.io/contrib/mungegithub/opts"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
)

var mungerMap = map[string]Munger{}

type Munger interface {
	MungeIssue(client *github.Client, org, project string, issue *github.Issue, dryrun bool)
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

func MungeIssues(client *github.Client, issueMungers string, o opts.MungeOptions) error {
	mungers, err := getMungers(strings.Split(issueMungers, ","))
	if err != nil {
		return err
	}
	page := 0
	for {
		glog.V(4).Infof("Fetching page %d", page)
		listOpts := &github.IssueListByRepoOptions{
			Sort:        "desc",
			ListOptions: github.ListOptions{PerPage: 100, Page: page},
		}
		list, response, err := client.Issues.ListByRepo(o.Org, o.Project, listOpts)
		if err != nil {
			return err
		}
		if err := mungeIssueList(list, client, o.Org, o.Project, mungers, o.MinIssueNumber, o.Dryrun); err != nil {
			return err
		}
		if response.LastPage == 0 || response.LastPage == page {
			break
		}
		page++
	}
	return nil
}

func mungeIssueList(list []github.Issue, client *github.Client, org, project string, mungers []Munger, minIssueNumber int, dryrun bool) error {
	for ix := range list {
		issue := &list[ix]
		if *issue.Number < minIssueNumber {
			continue
		}
		for _, munger := range mungers {
			munger.MungeIssue(client, org, project, issue, dryrun)
		}
	}
	return nil
}
