/*
Copyright 2018 The Kubernetes Authors.

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

// This application reads kubernetes OWNERS files within a repo and compares it
// to collaborators for a repo to find those listed in OWNERS files who are not
// collaborators

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	"github.com/ghodss/yaml"
	"k8s.io/test-infra/prow/github"
)

var (
	// The root location of the repo to start walking the filesystem
	repoRoot string

	// The name of the GitHub org to use when querying for collaborators
	githubOrg string

	// The name of the GitHub repo to use when querying for collaborators
	githubRepo string
)

func init() {
	flag.StringVar(&repoRoot, "srcdir", "", "The location of the repo on the local filesystem to start inspecting")
	flag.StringVar(&githubOrg, "org", "", "The name of the GitHub org to use when querying for collaborators")
	flag.StringVar(&githubRepo, "repo", "", "The name of the GitHub repo to use when querying for collaborators")
}

func main() {

	flag.Parse()

	// We require a github token to make API calls due to rate limiting
	if os.Getenv("GITHUB_TOKEN") == "" {
		fmt.Println("Error: Please supply an environment variable named GITHUB_TOKEN with a valid token")
		os.Exit(1)
	}

	// Ensure all the flags were passed in
	if repoRoot == "" {
		fmt.Println("Error: Please supply a local source code location with the -s flag")
		os.Exit(1)
	}
	if githubOrg == "" {
		fmt.Println("Error: Please supply a GitHub org with the -o flag")
		os.Exit(1)
	}
	if githubRepo == "" {
		fmt.Println("Error: Please supply a GitHub repo name with the -r flag")
		os.Exit(1)
	}

	// Stores the usernames found in the OWNERS files
	handles := make(map[string]struct{})

	// Walk the file tree to gather all the GitHub Logins used in all OWNERS files
	err := filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		name := info.Name()

		// Make sure to skip anything in the .git directory. OWNERS files can be
		// found in there but their format will cause read errors.
		if name == ".git" {
			return filepath.SkipDir
		}

		// Add in handling for OWNERS_ALIASES files
		if name == "OWNERS" {
			// found an OWNERS file. Process it.
			y, e2 := readOwners(path)
			if e2 != nil {
				return e2
			}

			var normV string
			for _, v := range y.Approvers {
				normV = github.NormLogin(v)
				if _, ok := handles[normV]; !ok {
					handles[normV] = struct{}{}
				}
			}

			for _, v := range y.Reviewers {
				normV = github.NormLogin(v)
				if _, ok := handles[normV]; !ok {
					handles[normV] = struct{}{}
				}
			}
		}

		return nil
	})

	if err != nil {
		fmt.Println("Error walking the directory tree:", err)
		os.Exit(1)
	}

	// Query the GitHub API for a repo to get the names of all collaborators.
	// Note, the merge functionality in prow only requires a collaborator with
	// read only access to a repo but is also in an OWNERS file.

	// Using the test-infra github client because it can handle pagenation
	// for long lists
	// TODO: Allow the API endpoint to be configurable
	gc := github.NewClient(os.Getenv("GITHUB_TOKEN"), "https://api.github.com")

	users, err := gc.ListCollaborators(githubOrg, githubRepo)
	if err != nil {
		fmt.Println("Error getting collaborators", err)
		os.Exit(1)
	}

	// Look at all the OWNERS and find which are not collaborators
	var found bool
	var nameList []string
	var normLogin string
	for k := range handles {
		found = false
		for _, v := range users {
			normLogin = github.NormLogin(v.Login)
			if normLogin == k {
				found = true
				break
			}
		}

		if !found {
			nameList = append(nameList, k)
		}
	}

	if len(nameList) > 0 {
		fmt.Println("GitHub Logins not found as collaborators:")
		sort.Strings(nameList)
		for _, v := range nameList {
			fmt.Println("*", v)
		}
	} else {
		fmt.Println("All reviews and approvers found are collaborators")
	}
}

// The portions of the owners file we are working with
// TODO: See if there's a way to have a common library for working with
// OWNERS files.
type ownersConfig struct {
	Approvers []string `json:"approvers,omitempty"`
	Reviewers []string `json:"reviewers,omitempty"`
}

func readOwners(p string) (*ownersConfig, error) {
	b, err := ioutil.ReadFile(p)
	if err != nil {
		return nil, err
	}

	o := &ownersConfig{}
	err = yaml.Unmarshal(b, o)
	if err != nil {
		return nil, err
	}
	return o, nil
}
