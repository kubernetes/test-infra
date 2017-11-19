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
	"time"

	"github.com/ghodss/yaml"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
)

// A label in a repository.
type Label struct {
	Name        string     `json:"name"`                  // Current name of the label
	Color       string     `json:"color"`                 // rrggbb or color
	Previously  []Label    `json:"previously,omitempty"`  // Previous names for this label
	DeleteAfter *time.Time `json:"deleteAfter,omitempty"` // Retired labels deleted on this date
	parent      *Label     // Current name for previous labels (used internally)
}

// Configuration is a list of Required Labels to sync in all kubernetes repos
type Configuration struct {
	Labels []Label `json:"labels"`
}

type RepoList []github.Repo
type RepoLabels map[string][]github.Label

// Update a label in a repo
type Update struct {
	repo    string
	Why     string
	Wanted  *Label `json:"wanted,omitempty"`
	Current *Label `json:"current,omitempty"`
}

// RepoUpdates Repositories to update: map repo name --> list of Updates
type RepoUpdates map[string][]Update

var (
	labelsPath = flag.String("config", "", "Path to labels.yaml")
	token      = flag.String("token", "", "Path to github oauth secret")
	orgs       = flag.String("orgs", "", "Comma separated list of orgs to sync")
	skipRepos  = flag.String("skip", "", "Comma separated list of org/repos to skip syncing")
	onlyRepos  = flag.String("only", "", "Only look at the following comma separated org/repos")
	dry        = flag.Bool("dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")
	debug      = flag.Bool("debug", false, "Turn on debug to be more verbose")
	endpoint   = flag.String("endpoint", "https://api.github.com", "GitHub's API endpoint")
)

// Ensures that no two label names (including previous names) have the same lowercase value.
func validate(labels []Label, parent string, seen map[string]string) error {
	for _, l := range labels {
		name := strings.ToLower(l.Name)
		path := parent + "." + name
		if other, present := seen[name]; present {
			return fmt.Errorf("duplicate label %s at %s and %s", name, path, other)
		}
		seen[name] = path
		if err := validate(l.Previously, path, seen); err != nil {
			return err
		}
	}
	return nil
}

// Ensures the config does not duplicate label names
func (c Configuration) validate() error {
	seen := make(map[string]string)
	if err := validate(c.Labels, "", seen); err != nil {
		return fmt.Errorf("invalid config: %v", err)
	}
	return nil
}

// Load yaml config at path
func LoadConfig(path string) (*Configuration, error) {
	if path == "" {
		return nil, errors.New("empty path")
	}
	var c Configuration
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err = yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if err = c.validate(); err != nil { // Ensure no dups
		return nil, err
	}
	return &c, nil
}

// GetOrg returns organization from "org" or "user:name"
// Org can be organization name like "kubernetes"
// But we can also request all user's public repos via user:github_user_name
func GetOrg(org string) (string, bool) {
	data := strings.Split(org, ":")
	if len(data) == 2 && data[0] == "user" {
		return data[1], true
	}
	return org, false
}

// Get reads repository list for given org
// Use provided githubClient (real, dry, fake)
// Uses GitHub: /orgs/:org/repos
func LoadRepos(org string, gc *github.Client, filt filter) (RepoList, error) {
	org, isUser := GetOrg(org)
	repos, err := gc.GetRepos(org, isUser)
	if err != nil {
		return nil, err
	}
	var rl RepoList
	for _, r := range repos {
		if !filt(org, r.Name) {
			continue
		}
		rl = append(rl, r)
	}
	return rl, nil
}

// Get reads repository's labels list
// Use provided githubClient (real, dry, fake)
// Uses GitHub: /repos/:org/:repo/labels
func LoadLabels(gc *github.Client, org string, repos RepoList) (*RepoLabels, error) {
	rl := RepoLabels{}
	for _, repo := range repos {
		logrus.Infof("Get labels in %s/%s", org, repo.Name)
		labels, err := gc.GetRepoLabels(org, repo.Name)
		if err != nil {
			logrus.Errorf("Error getting labels for %s/%s", org, repo.Name)
			return nil, err
		}
		rl[repo.Name] = labels
	}
	return &rl, nil
}

// Delete the label
func kill(repo string, label Label) Update {
	logrus.Infof("kill %s", label.Name)
	return Update{Why: "dead", Current: &label, repo: repo}
}

// Create the label
func create(repo string, label Label) Update {
	logrus.Infof("create %s", label.Name)
	return Update{Why: "missing", Wanted: &label, repo: repo}
}

// Rename the label (will also update color)
func rename(repo string, previous, wanted Label) Update {
	logrus.Infof("rename %s to %s", previous.Name, wanted.Name)
	return Update{Why: "rename", Current: &previous, Wanted: &wanted, repo: repo}
}

// Update the label color
func recolor(repo string, label Label) Update {
	logrus.Infof("recolor %s to %s", label.Name, label.Color)
	return Update{Why: "recolor", Current: &label, Wanted: &label, repo: repo}
}

// Migrate labels to another label
func move(repo string, previous, wanted Label) Update {
	logrus.Infof("move %s to %s", previous.Name, wanted.Name)
	return Update{Why: "migrate", Wanted: &wanted, Current: &previous, repo: repo}
}

func ClassifyLabels(labels []Label, required, archaic, dead map[string]Label, now time.Time, parent *Label) {
	for i, l := range labels {
		first := parent
		if first == nil {
			first = &labels[i]
		}
		lower := strings.ToLower(l.Name)
		switch {
		case parent == nil && l.DeleteAfter == nil: // Live label
			required[lower] = l
		case l.DeleteAfter != nil && now.After(*l.DeleteAfter):
			dead[lower] = l
		case parent != nil:
			l.parent = parent
			archaic[lower] = l
		}
		ClassifyLabels(l.Previously, required, archaic, dead, now, first)
	}
}

func SyncLabels(config Configuration, repos RepoLabels) (RepoUpdates, error) {
	// Ensure the config is valid
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %v", err)
	}

	// Find required, dead and archaic labels
	required := make(map[string]Label) // Must exist
	archaic := make(map[string]Label)  // Migrate
	dead := make(map[string]Label)     // Delete
	ClassifyLabels(config.Labels, required, archaic, dead, time.Now(), nil)

	var actions []Update
	// Process all repos
	for repo, repoLabels := range repos {
		// Convert github.Label to Label
		var labels []Label
		for _, l := range repoLabels {
			labels = append(labels, Label{Name: l.Name, Color: l.Color})
		}
		// Check for any duplicate labels
		if err := validate(labels, "", make(map[string]string)); err != nil {
			return nil, fmt.Errorf("invalid labels in %s: %v", repo, err)
		}
		// Create lowercase map of current labels, checking for dead labels to delete.
		current := make(map[string]Label)
		for _, l := range labels {
			lower := strings.ToLower(l.Name)
			// Should we delete this dead label?
			if _, found := dead[lower]; found {
				actions = append(actions, kill(repo, l))
			}
			current[lower] = l
		}

		var moveActions []Update // Separate list to do last
		// Look for labels to migrate
		for name, l := range archaic {
			// Does the archaic label exist?
			cur, found := current[name]
			if !found { // No
				continue
			}
			// What do we want to migrate it to?
			desired := Label{Name: l.parent.Name, Color: l.parent.Color}
			desiredName := strings.ToLower(l.parent.Name)
			// Does the new label exist?
			_, found = current[desiredName]
			if found { // Yes, migrate all these labels
				moveActions = append(moveActions, move(repo, cur, desired))
			} else { // No, rename the existing label
				actions = append(actions, rename(repo, cur, desired))
				current[desiredName] = desired
			}
		}

		// Look for missing labels
		for name, l := range required {
			cur, found := current[name]
			switch {
			case !found:
				actions = append(actions, create(repo, l))
			case l.Name != cur.Name:
				actions = append(actions, rename(repo, cur, l))
			case l.Color != cur.Color:
				actions = append(actions, recolor(repo, l))
			}
		}

		for _, a := range moveActions {
			actions = append(actions, a)
		}
	}

	u := RepoUpdates{}
	for _, a := range actions {
		u[a.repo] = append(u[a.repo], a)
	}
	return u, nil
}

// DoUpdates iterates generated update data and adds and/or modifies labels on repositories
// Uses AddLabel GH API to add missing labels
// And UpdateLabel GH API to update color or name (name only when case differs)
func (ru RepoUpdates) DoUpdates(org string, gc *github.Client) error {
	for repo, updates := range ru {
		logrus.Infof("Applying %d changes to %s/%s", len(updates), org, repo)
		for _, item := range updates {
			if *debug {
				fmt.Printf("%s/%s: %s %+v\n", org, repo, item.Why, item.Wanted)
			}
			switch item.Why {
			case "missing":
				err := gc.AddRepoLabel(org, repo, item.Wanted.Name, item.Wanted.Color)
				if err != nil {
					return err
				}
			case "recolor", "rename":
				err := gc.UpdateRepoLabel(org, repo, item.Current.Name, item.Wanted.Name, item.Wanted.Color)
				if err != nil {
					return err
				}
			case "dead":
				err := gc.DeleteRepoLabel(org, repo, item.Current.Name)
				if err != nil {
					return err
				}
			case "migrate":
				issues, err := gc.FindIssues(fmt.Sprintf("repo:%s/%s label:\"%s\" -label:\"%s\"", org, repo, item.Current.Name, item.Wanted.Name), "", false)
				if err != nil {
					return err
				}
				if len(issues) == 0 {
					if err = gc.DeleteRepoLabel(org, repo, item.Current.Name); err != nil {
						return err
					}
				}
				for _, i := range issues {
					if err = gc.AddLabel(org, repo, i.Number, item.Wanted.Name); err != nil {
						return err
					}
					if err = gc.RemoveLabel(org, repo, i.Number, item.Wanted.Name); err != nil {
						return err
					}
				}
			default:
				return errors.New("unknown label operation: " + item.Why)
			}
		}
	}
	return nil
}

func newClient(tokenPath, host string, dryRun bool) (*github.Client, error) {
	if tokenPath == "" {
		return nil, errors.New("--token unset")
	}
	b, err := ioutil.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read --token=%s: %v", tokenPath, err)
	}
	oauthSecret := string(bytes.TrimSpace(b))

	if dryRun {
		return github.NewDryRunClient(oauthSecret, host), nil
	}
	return github.NewClient(oauthSecret, host), nil
}

// Main function
// Typical run with production configuration should require no parameters
// It expects:
// "labels" file in "/etc/config/labels.yaml"
// github OAuth2 token in "/etc/github/oauth", this token must have write access to all org's repos
// default org is "kubernetes"
// It uses request retrying (in case of run out of GH API points)
// It took about 10 minutes to process all my 8 repos with all wanted "kubernetes" labels (70+)
// Next run takes about 22 seconds to check if all labels are correct on all repos
func main() {
	flag.Parse()

	config, err := LoadConfig(*labelsPath)
	if err != nil {
		logrus.WithError(err).Fatalf("failed to load --config=%s", *labelsPath)
	}

	githubClient, err := newClient(*token, *endpoint, *dry)
	if err != nil {
		logrus.WithError(err).Fatal("failed to create client")
	}

	var filt filter
	switch {
	case *onlyRepos != "":
		if *skipRepos != "" {
			logrus.Fatalf("--only and --skip cannot both be set")
		}
		only := make(map[string]bool)
		for _, r := range strings.Split(*onlyRepos, ",") {
			only[strings.TrimSpace(r)] = true
		}
		filt = func(org, repo string) bool {
			_, ok := only[org+"/"+repo]
			return ok
		}
	case *skipRepos != "":
		skip := make(map[string]bool)
		for _, r := range strings.Split(*skipRepos, ",") {
			skip[strings.TrimSpace(r)] = true
		}
		filt = func(org, repo string) bool {
			_, ok := skip[org+"/"+repo]
			return !ok
		}
	default:
		filt = func(o, r string) bool {
			return true
		}
	}

	for _, org := range strings.Split(*orgs, ",") {
		org = strings.TrimSpace(org)
		if err = SyncOrg(org, githubClient, *config, filt); err != nil {
			logrus.WithError(err).Fatalf("failed to update %s", org)
		}
	}
}

type filter func(string, string) bool

func SyncOrg(org string, githubClient *github.Client, config Configuration, filt filter) error {
	logrus.Infof("Reading repos in %s", org)
	repos, err := LoadRepos(org, githubClient, filt)
	if err != nil {
		return err
	}

	logrus.Infof("Reading labels in %d repos in %s", len(repos), org)
	currLabels, err := LoadLabels(githubClient, org, repos)
	if err != nil {
		return err
	}

	logrus.Infof("Syncing labels for %d repos in %s", len(repos), org)
	updates, err := SyncLabels(config, *currLabels)
	if err != nil {
		return err
	}
	if *debug {
		y, _ := yaml.Marshal(updates)
		fmt.Println(string(y))
	}

	if *dry {
		logrus.Infof("No real update labels in --dry-run")
		return nil
	}

	if err = updates.DoUpdates(org, githubClient); err != nil {
		return err
	}
	return nil
}
