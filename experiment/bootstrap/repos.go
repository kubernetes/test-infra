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

// utility method used below, checks a PULL_REFS for `:`
func refHasSHAs(ref string) bool {
	return strings.Contains(ref, ":")
}

// GitBasePath returns the base git path associated with the repo,
// this does not include the branch or pull.
// If ssh is true this assumes git@ is the appropriate user.
// TODO(bentheelder): do we want to continue to assume git@ for ssh in the future?
// This works fine for GitHub and matches jenkins/bootstrap.py's `repository(..)`
func (r *Repo) GitBasePath(ssh bool) string {
	// TODO(bentheelder): perhaps this should contain a map of known prefix
	// replacements instead so that supporting other repos is easier?
	path := r.Name
	if strings.HasPrefix(path, "k8s.io/") {
		path = "github.com/kubernetes/" + strings.TrimPrefix(path, "k8s.io/")
	}
	if ssh {
		if !refHasSHAs(path) {
			path = strings.Replace(path, "/", ":", 1)
		}
		return "git@" + path
	}
	return "https://" + path
}

// PullNumbers converts a Pull's list string into a slice of PR number strings
// NOTE: this assumes that if there are pull requests specified that Repo.Pull
// looks something like: `master:hash,prNum:hash,prNum:hash,...`
func (r *Repo) PullNumbers() []string {
	if refHasSHAs(r.Pull) {
		res := []string{}
		parts := strings.Split(r.Pull, ",")
		for _, part := range parts {
			res = append(res, strings.Split(part, ":")[0])
		}
		return res[1:]
	}
	return []string{}
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
// with one or more comma separated "branch:commit".
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
