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

// This is a label_sync tool, details in README.md
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
)

// Label single label
type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// RequiredLabels is a list of Required Labels to sync in all kubernetes repos
type RequiredLabels struct {
	Labels []Label `json:"labels"`
}

// RepoList is a list of Repositories from Org to be checked by this program
type RepoList struct {
	Org   string        `json:"org"`
	Repos []github.Repo `json:"repos"`
}

// RepoLabels is a mapping repository name --> current list of labels
type RepoLabels struct {
	Labels map[string][]github.Label
}

// UpdateItem is a single item to update
// update can mean: update Label in repo or add missing label in repo
type UpdateItem struct {
	Why                         string
	RequiredLabel, CurrentLabel Label
}

// UpdateData Repositories to update: map repo name --> list of Updates
type UpdateData struct {
	ReposToUpdate map[string][]UpdateItem
}

var (
	labelsPath         = flag.String("labels-path", "/etc/config/labels.yaml", "Path to labels.yaml.")
	githubTokenFile    = flag.String("github-token-file", "/etc/github/oauth", "Path to the file containing the GitHub OAuth secret.")
	org                = flag.String("org", "kubernetes", "Organization to fetch repository list from, to fetch user's public repositories, use user:user_name")
	dry                = flag.Bool("dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")
	local              = flag.Bool("local", false, "Run locally for testing purposes only. Does not require secret files.")
	reposData          = flag.String("repos", "", "Repository file saved as YAML to save GitHub API points, can be used to specify repositories to run")
	repoLabelsData     = flag.String("repo-labels", "", "Repository Labels file saved as YAML to save GitHub API points")
	dumpReposData      = flag.String("dump-repos", "", "Save repositories found as YAML (so this file can be used again with -repos without using Gihub API points in -local mode)")
	dumpRepoLabelsData = flag.String("dump-repo-labels", "", "Save repositories labels found as YAML (so this file can be used again with -repo-labels without using Gihub API points in -local mode)")
	debug              = flag.Bool("debug", false, "Turn on debug to be more verbose")
	endPoint           = flag.String("endpoint", "https://api.github.com", "GitHub's API endpoint")
)

// Load labels from labels.yaml file
func (rl *RequiredLabels) Load(path string) (err error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}
	return yaml.Unmarshal(data, rl)
}

// GetOrg returns organization from "org" or "user:name"
// Org can be organization name like "kubernetes"
// But we can also request all user's public repos via user:github_user_name
func GetOrg(org string) (outOrg string, isUser bool) {
	data, isUser, outOrg := strings.Split(org, ":"), false, org
	if len(data) == 2 && data[0] == "user" {
		outOrg = data[1]
		isUser = true
	}
	return
}

// Get reads repository list for given org
// Use provided githubClient (real, dry, fake)
// Uses GitHub: /orgs/:org/repos
func (rl *RepoList) Get(gc *github.Client) (err error) {
	org, isUser := GetOrg(*org)
	repos, err := gc.GetRepos(org, isUser)
	if err != nil {
		return
	}
	rl.Repos = repos
	rl.Org = org
	return
}

// Get reads repository's labels list
// Use provided githubClient (real, dry, fake)
// Uses GitHub: /repos/:org/:repo/labels
func (rl *RepoLabels) Get(gc *github.Client, repos *RepoList) error {
	for _, repo := range repos.Repos {
		labels, err := gc.GetRepoLabels(repos.Org, repo.Name)
		if err != nil {
			logrus.Errorf("Error getting labels for %s/%s", repos.Org, repo.Name)
			return err
		}
		rl.Labels[repo.Name] = labels
	}
	return nil
}

// UnmarshalReposData returns GitHub data recorded to YAML file via -dump-repos
// This data can be saved in repos.yaml
// This is to save GitHub API points and speedup local development
// This file can also be used to overwite repository list to run this program on
func UnmarshalReposData(repos *RepoList) error {
	data, err := ioutil.ReadFile(*reposData)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(data, repos)
	if err != nil {
		return err
	}
	return nil
}

// UnmarshalRepoLabelsData returns GitHub data recorded to YAML file via -dump-repo-labels
// This data can be saved in repo_labels.yaml
// This is to save GitHub API points and speedup local development
func UnmarshalRepoLabelsData(repoLabels *RepoLabels) error {
	data, err := ioutil.ReadFile(*repoLabelsData)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(data, repoLabels)
	if err != nil {
		return err
	}
	return nil
}

// MarshalReposData is used to save repos data into YAML file
// If -dump-repos parameter is used
func MarshalReposData(repos *RepoList) error {
	data, err := yaml.Marshal(repos)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(*dumpReposData, []byte(data), 0644); err != nil {
		return err
	}
	return nil
}

// MarshalReposLabelsData is used to save repo labels data into YAML file
// If -dump-repo-labels parameter is used
func MarshalReposLabelsData(repoLabels *RepoLabels) error {
	data, err := yaml.Marshal(repoLabels)
	if err != nil {
		return err
	}
	if err = ioutil.WriteFile(*dumpRepoLabelsData, []byte(data), 0644); err != nil {
		return err
	}
	return nil
}

// InitWithRepo initialize (possibly empty UpdatesData)
// Create map if not yet created
// Create empty list for given repo if this repo is not in the map
func (ud *UpdateData) InitWithRepo(repo string) {
	if ud.ReposToUpdate == nil {
		ud.ReposToUpdate = make(map[string][]UpdateItem)
	}
	if _, foundRepo := ud.ReposToUpdate[repo]; !foundRepo {
		ud.ReposToUpdate[repo] = []UpdateItem{}
	}
}

// Eq compares two labels - they're equal if their Name and Color match
func (l *Label) Eq(otherLabel *Label) bool {
	return l.Name == otherLabel.Name && l.Color == otherLabel.Color
}

// SyncLabels this is a main workhorse.
// Pre requisites:
// `labels.yaml` file should should have unique labels with a case insensitive comparison! Otherwise it reports FATAL and exits
// Each repository's labels should be unique when comapred witout case sensitive, if not it reports FATAL and exits
// Labels can be non-unique (without case sensitive) in different repos - because that mean each wrong case label is unique per repo so can be updated there.
// It iterates all repos to see if they have required labels
// Possible cases (for each label on each repo compared with required) iteration: all repos, then all required labels
// 1. Exact match - all OK
// 2. Name exact match - wrong color --> add this label to update list
// 3. If required label (downcased) matches some repo's label (downcased) - wrong name --> add this label to update list
// In this case we have actual repo's label (wrong case) and required (with correct case) we will update name (using wrong case as key) and evetually color too
// 4. Not found (no exact name match and no match without case) --> add this label to update list (with mising flag which mean that it needs to be added)
func SyncLabels(required *RequiredLabels, curr *RepoLabels) (updates UpdateData, err error) {
	var (
		currMap      map[string]map[string]Label
		currMapLower map[string]map[string]Label
		reqMap       map[string]Label
		reqMapLower  map[string]Label
	)

	// Create mapping of required labels --> their label data (name & color)
	// Make sure that downcased names are unique (required labels are constant from labels.yaml file)
	reqMap = make(map[string]Label)
	reqMapLower = make(map[string]Label)
	for _, label := range required.Labels {
		lbl := Label{Name: label.Name, Color: label.Color}
		reqMap[label.Name] = lbl
		lowerName := strings.ToLower(label.Name)
		if _, ok := reqMapLower[lowerName]; ok {
			logrus.Errorf("ERROR: label %s is not unique when downcased", label.Name)
			err = errors.New("label " + label.Name + " is not unique when downcased in input config")
			return
		}
		reqMapLower[lowerName] = lbl
	}

	// create map of [repo][label anme] --> label data
	// Also create map with downcased names
	// Warn if downcased label names are not unique
	currMap = make(map[string]map[string]Label)
	currMapLower = make(map[string]map[string]Label)
	for repo, labels := range curr.Labels {
		currMap[repo] = make(map[string]Label)
		currMapLower[repo] = make(map[string]Label)
		for _, label := range labels {
			lbl := Label{Name: label.Name, Color: label.Color}
			currMap[repo][label.Name] = lbl
			lowerName := strings.ToLower(label.Name)
			if _, ok := currMapLower[repo][lowerName]; ok {
				logrus.Infof("ERROR: repo %s, label %s is not unique when downcased", repo, label.Name)
				err = errors.New("repository: " + repo + ", label " + label.Name + " is not unique when downcased in input config")
				return
			}
			currMapLower[repo][lowerName] = lbl
		}
	}

	// Iterate all repos
	for repo, repoLabels := range currMap {
		repoLabelsLower := currMapLower[repo]
		//logrus.Infof("Checking repo: %s, %T", repo, repoLabels)
		// Iterate required labels
		for requiredName, requiredLab := range reqMap {
			//logrus.Infof("Checking for label: %s, %T", requiredName, requiredLab)
			foundLabel, ok := repoLabels[requiredName]
			if ok && foundLabel.Eq(&requiredLab) {
				// Case 1 - exact label found
				//logrus.Infof("Repo: %s, Label: %s - found exact", repo, requiredName)
				continue
			}
			if ok {
				// Case 2: Wrong label color
				//logrus.Infof("Repo: %s, Label: %s - found but needs color update %s --> %s", repo, requiredName, foundLabel.Color, requiredLab.Color)
				updates.InitWithRepo(repo)
				updates.ReposToUpdate[repo] = append(updates.ReposToUpdate[repo], UpdateItem{Why: "invalid_color", RequiredLabel: requiredLab, CurrentLabel: foundLabel})
				continue
			}
			requiredNameLower := strings.ToLower(requiredName)
			foundLabel, ok = repoLabelsLower[requiredNameLower]
			if ok {
				// Case 3 - wrong label name (wrong case) and possibly color too (it needs update anyway)
				//logrus.Infof("Repo: %s, Label: %s - found but needs name update (and maybe color): %v --> %v", repo, requiredName, foundLabel, requiredLab)
				updates.InitWithRepo(repo)
				updates.ReposToUpdate[repo] = append(updates.ReposToUpdate[repo], UpdateItem{Why: "invalid_name", RequiredLabel: requiredLab, CurrentLabel: foundLabel})
				continue
			}
			// Case 4: label not found - add to missing labels
			//logrus.Infof("Repo: %s, Label: %s - not found %v", repo, requiredName, requiredLab)
			updates.InitWithRepo(repo)
			updates.ReposToUpdate[repo] = append(updates.ReposToUpdate[repo], UpdateItem{Why: "missing", RequiredLabel: requiredLab, CurrentLabel: Label{}})
		}
	}
	return
}

// DoUpdates iterates generated update data and adds and/or modifies labels on repositories
// Uses AddLabel GH API to add missing labels
// And UpdateLabel GH API to update color or name (name only when case differs)
func (ud *UpdateData) DoUpdates(gc *github.Client) error {
	org, _ := GetOrg(*org)
	for repo, list := range ud.ReposToUpdate {
		for _, item := range list {
			if *debug {
				fmt.Printf("%s/%s: %s %+v\n", org, repo, item.Why, item.RequiredLabel)
			}
			if item.Why == "missing" {
				err := gc.AddRepoLabel(org, repo, item.RequiredLabel.Name, item.RequiredLabel.Color)
				if err != nil {
					return err
				}
			} else if item.Why == "invalid_color" || item.Why == "invalid_name" {
				err := gc.UpdateRepoLabel(org, repo, item.RequiredLabel.Name, item.RequiredLabel.Color)
				if err != nil {
					return err
				}
			} else {
				return errors.New("unknown label operation: " + item.Why)
			}
		}
	}
	return nil
}

// Main function
// Typical run with production configuration should require no parameters
// It expects:
// "labels" file in "/etc/config/labels.yaml"
// github OAuth2 token in "/etc/github/oauth", this token must have write access to all org's repos
// default org is "kubernetes"
// It uses request retrying (in case of run out of GH API points)
// It took about 10 minutes to process all my 8 repos with all required "kubernetes" labels (70+)
// Next run takes about 22 seconds to check if all labels are correct on all repos
func main() {
	flag.Parse()

	var (
		labels       RequiredLabels
		repos        RepoList
		currLabels   RepoLabels
		githubClient *github.Client
	)
	currLabels.Labels = make(map[string][]github.Label)

	if err := labels.Load(*labelsPath); err != nil {
		logrus.WithError(err).Fatalf("cannot read labels file: %v", *labelsPath)
	}

	if *local {
		githubClient = github.NewFakeClient()

		if *reposData != "" {
			if err := UnmarshalReposData(&repos); err != nil {
				logrus.WithError(err).Fatalf("cannot unmarshal repositories from %s", *reposData)
			}
		}
		if *repoLabelsData != "" {
			if err := UnmarshalRepoLabelsData(&currLabels); err != nil {
				logrus.WithError(err).Fatalf("cannot unmarshal repositories labels from %s", *repoLabelsData)
			}
		}

		if *reposData == "" || *repoLabelsData == "" {
			logrus.Infof("Cannot get repository and labels data in -local mode without generated YAML files.")
			logrus.Infof("Please provide example data with -repos and -repo-labels (eventually generate it from non-local run via -dump-repos and -dump-repo-labels)")
			logrus.Fatalf("No example data provided for -local mode")
		}

	} else {
		oauthSecretRaw, err := ioutil.ReadFile(*githubTokenFile)
		if err != nil {
			logrus.WithError(err).Fatal("could not read oauth secret file.")
		}
		oauthSecret := string(bytes.TrimSpace(oauthSecretRaw))

		if *dry {
			githubClient = github.NewDryRunClient(oauthSecret, *endPoint)
		} else {
			githubClient = github.NewClient(oauthSecret, *endPoint)
		}

		if *reposData != "" {
			if err := UnmarshalReposData(&repos); err != nil {
				logrus.WithError(err).Fatalf("cannot unmarshal repositories from %s", *reposData)
			}
		} else {
			// This is a real GH API call to get repos and labels, please avoid in local dev if possible
			// Just run once with -dump-repos and -dump-repo-labels and then use generated YAML files to save GH API points
			if err := repos.Get(githubClient); err != nil {
				logrus.WithError(err).Fatalf("cannot read %v repositories", *org)
			}
		}

		if *repoLabelsData != "" {
			if err := UnmarshalRepoLabelsData(&currLabels); err != nil {
				logrus.WithError(err).Fatalf("cannot unmarshal repositories labels from %s", *repoLabelsData)
			}
		} else {
			if err := currLabels.Get(githubClient, &repos); err != nil {
				logrus.WithError(err).Fatalf("cannot read %v repositories labels", *org)
			}
		}
	}

	if *dumpReposData != "" {
		if err := MarshalReposData(&repos); err != nil {
			logrus.WithError(err).Fatalf("cannot marshal repositories to %s", *dumpReposData)
		}
	}

	if *dumpRepoLabelsData != "" {
		if err := MarshalReposLabelsData(&currLabels); err != nil {
			logrus.WithError(err).Fatalf("cannot marshal repositories labels data to %s", *dumpRepoLabelsData)
		}
	}

	updates, err := SyncLabels(&labels, &currLabels)
	if err != nil {
		logrus.WithError(err).Fatalf("failed to sync labels")
	}
	y, err := yaml.Marshal(&updates)
	if *debug {
		fmt.Printf(string(y))
	}

	if *dry || *local {
		logrus.Infof("No real update labels in -dry-run or local -mode, exiting")
		return
	}

	err = updates.DoUpdates(githubClient)
	if err != nil {
		logrus.WithError(err).Fatalf("failed to update labels")
	}
}
