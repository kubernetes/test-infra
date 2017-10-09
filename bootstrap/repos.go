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

package main

import (
	"fmt"
	"regexp"
	"strings"
)

// Repo contains the components of git repo refs used in bootstrap
type Repo struct {
	Name   string
	Branch string
	Pull   string
}

// Repos is a slice of Repo where Repos[0] is the main repo
type Repos []Repo

// Main returns the primary repo in a Repos produced by ParseRepos
func (r Repos) Main() *Repo {
	if len(r) == 0 {
		return nil
	}
	return &r[0]
}

// ParseRepos converts the refs related arguments to []Repo
// each repoArgs is expect to be "name=branch:commit,branch:commit"
// with one or more comma seperated "branch:commit".
// EG: "k8s.io/kubernetes=master:42e2ca8c18c93ba25eb0e5bd02ecba2eaa05e871,52057:b4f639f57ae0a89cdf1b43d1810b617c76f4b1b3"
func ParseRepos(repoArgs []string) (Repos, error) {
	repos := []Repo{}
	re := regexp.MustCompile(`([^=]+)(=([^:,~^\s]+(:[0-9a-fA-F]+)?(,|$))+)?$`)
	for _, repoArg := range repoArgs {
		match := re.FindStringSubmatch(repoArg)
		if len(match) == 0 {
			return nil, fmt.Errorf("could not parse repo: %s, %v", repoArg, repos)
		}
		thisRepo := match[1]
		// default to master
		if match[2] == "" {
			repos = append(repos, Repo{
				Name:   thisRepo,
				Branch: "master",
				Pull:   "",
			})
			continue
		}
		commitsString := match[2][1:]
		commits := strings.Split(commitsString, ",")
		// Checking out a branch, possibly at a specific commit
		if len(commits) == 1 {
			repos = append(repos, Repo{
				Name:   thisRepo,
				Branch: commits[0],
				Pull:   "",
			})
			continue
		}
		// Checking out one or more PRs
		repos = append(repos, Repo{
			Name:   thisRepo,
			Branch: "",
			Pull:   commitsString,
		})
	}
	return repos, nil
}

// TODO(bentheelder): unit test the methods below

func refHasSHAs(ref string) bool {
	return strings.Contains(ref, ":")
}

// PullNumbers converts a reference list string into a list of PR number strings
func PullNumbers(pull string) []string {
	if refHasSHAs(pull) {
		res := []string{}
		parts := strings.Split(pull, ",")
		for _, part := range parts {
			res = append(res, strings.Split(part, ":")[0])
		}
		return res[1:]
	}
	return []string{pull}
}

// Repository returns the url associated with the repo
func Repository(repo string, ssh bool) string {
	// TODO(bentheelder): perhaps this should contain a map of known prefix
	// replacements instead so that supporting other repos is easier?
	if strings.HasPrefix(repo, "k8s.io/") {
		repo = "github.com/kubernetes" + strings.TrimPrefix(repo, "k8s.io/")
	}
	if ssh {
		if !refHasSHAs(repo) {
			repo = strings.Replace(repo, "/", ":", 1)
		}
		return "git@" + repo
	}
	return "https://" + repo
}
