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
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"
	"unicode"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/logrusutil"
)

const maxConcurrentWorkers = 20

// A label in a repository.

// LabelTarget specifies the intent of the label (PR or issue)
type LabelTarget string

const (
	prTarget    LabelTarget = "prs"
	issueTarget             = "issues"
	bothTarget              = "both"
)

// Label holds declarative data about the label.
type Label struct {
	// Name is the current name of the label
	Name string `json:"name"`
	// Color is rrggbb or color
	Color string `json:"color"`
	// Description is brief text explaining its meaning, who can apply it
	Description string `json:"description"`
	// Target specifies whether it targets PRs, issues or both
	Target LabelTarget `json:"target"`
	// ProwPlugin specifies which prow plugin add/removes this label
	ProwPlugin string `json:"prowPlugin"`
	// IsExternalPlugin specifies if the prow plugin is external or not
	IsExternalPlugin bool `json:"isExternalPlugin"`
	// AddedBy specifies whether human/munger/bot adds the label
	AddedBy string `json:"addedBy"`
	// Previously lists deprecated names for this label
	Previously []Label `json:"previously,omitempty"`
	// DeleteAfter specifies the label is retired and a safe date for deletion
	DeleteAfter *time.Time `json:"deleteAfter,omitempty"`
	parent      *Label     // Current name for previous labels (used internally)
}

// Configuration is a list of Repos defining Required Labels to sync into them
// There is also a Default list of labels applied to every Repo
type Configuration struct {
	Repos   map[string]RepoConfig `json:"repos,omitempty"`
	Default RepoConfig            `json:"default"`
}

// RepoConfig contains only labels for the moment
type RepoConfig struct {
	Labels []Label `json:"labels"`
}

// RepoLabels holds a repo => []github.Label mapping.
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

const (
	defaultTokens = 300
	defaultBurst  = 100
)

// TODO(fejta): rewrite this to use an option struct which we can unit test, like everything else.
var (
	debug           = flag.Bool("debug", false, "Turn on debug to be more verbose")
	confirm         = flag.Bool("confirm", false, "Make mutating API calls to GitHub.")
	endpoint        = flagutil.NewStrings(github.DefaultAPIEndpoint)
	graphqlEndpoint = flag.String("graphql-endpoint", github.DefaultGraphQLEndpoint, "GitHub's GraphQL API endpoint")
	labelsPath      = flag.String("config", "", "Path to labels.yaml")
	onlyRepos       = flag.String("only", "", "Only look at the following comma separated org/repos")
	orgs            = flag.String("orgs", "", "Comma separated list of orgs to sync")
	skipRepos       = flag.String("skip", "", "Comma separated list of org/repos to skip syncing")
	token           = flag.String("token", "", "Path to github oauth secret")
	action          = flag.String("action", "sync", "One of: sync, docs")
	cssTemplate     = flag.String("css-template", "", "Path to template file for label css")
	cssOutput       = flag.String("css-output", "", "Path to output file for css")
	docsTemplate    = flag.String("docs-template", "", "Path to template file for label docs")
	docsOutput      = flag.String("docs-output", "", "Path to output file for docs")
	tokens          = flag.Int("tokens", defaultTokens, "Throttle hourly token consumption (0 to disable)")
	tokenBurst      = flag.Int("token-burst", defaultBurst, "Allow consuming a subset of hourly tokens in a short burst")
)

func init() {
	flag.Var(&endpoint, "endpoint", "GitHub's API endpoint")
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Writes the golang text template at templatePath to outputPath using the given data
func writeTemplate(templatePath string, outputPath string, data interface{}) error {
	// set up template
	funcMap := template.FuncMap{
		"anchor": func(input string) string {
			return strings.Replace(input, ":", " ", -1)
		},
	}
	t, err := template.New(filepath.Base(templatePath)).Funcs(funcMap).ParseFiles(templatePath)
	if err != nil {
		return err
	}

	// ensure output path exists
	if !pathExists(outputPath) {
		_, err = os.Create(outputPath)
		if err != nil {
			return err
		}
	}

	// open file at output path and truncate
	f, err := os.OpenFile(outputPath, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	f.Truncate(0)

	// render template to output path
	err = t.Execute(f, data)
	if err != nil {
		return err
	}

	return nil
}

// validate runs checks to ensure the label inputs are valid
// It ensures that no two label names (including previous names) have the same
// lowercase value, and that the description is not over 100 characters.
func validate(labels []Label, parent string, seen map[string]string) (map[string]string, error) {
	newSeen := copyStringMap(seen)
	for _, l := range labels {
		name := strings.ToLower(l.Name)
		path := parent + "." + name
		if other, present := newSeen[name]; present {
			return newSeen, fmt.Errorf("duplicate label %s at %s and %s", name, path, other)
		}
		newSeen[name] = path
		if newSeen, err := validate(l.Previously, path, newSeen); err != nil {
			return newSeen, err
		}
		if len(l.Description) > 100 { // github limits the description field to 100 chars
			return newSeen, fmt.Errorf("description for %s is too long", name)
		}
	}
	return newSeen, nil
}

func copyStringMap(originalMap map[string]string) map[string]string {
	newMap := make(map[string]string)
	for k, v := range originalMap {
		newMap[k] = v
	}
	return newMap
}

func stringInSortedSlice(a string, list []string) bool {
	i := sort.SearchStrings(list, a)
	if i < len(list) && list[i] == a {
		return true
	}
	return false
}

// Labels returns a sorted list of labels unique by name
func (c Configuration) Labels() []Label {
	var labelarrays [][]Label
	labelarrays = append(labelarrays, c.Default.Labels)
	for _, repo := range c.Repos {
		labelarrays = append(labelarrays, repo.Labels)
	}

	labelmap := make(map[string]Label)
	for _, labels := range labelarrays {
		for _, l := range labels {
			name := strings.ToLower(l.Name)
			if _, ok := labelmap[name]; !ok {
				labelmap[name] = l
			}
		}
	}

	var labels []Label
	for _, label := range labelmap {
		labels = append(labels, label)
	}
	sort.Slice(labels, func(i, j int) bool { return labels[i].Name < labels[j].Name })
	return labels
}

// TODO(spiffxp): needs to validate labels duped across repos are identical
// Ensures the config does not duplicate label names between default and repo
func (c Configuration) validate(orgs string) error {
	// Check default labels
	seen, err := validate(c.Default.Labels, "default", make(map[string]string))
	if err != nil {
		return fmt.Errorf("invalid config: %v", err)
	}

	// Generate list of orgs
	sortedOrgs := strings.Split(orgs, ",")
	sort.Strings(sortedOrgs)
	// Check other repos labels
	for repo, repoconfig := range c.Repos {
		// Will complain if a label is both in default and repo
		if _, err := validate(repoconfig.Labels, repo, seen); err != nil {
			return fmt.Errorf("invalid config: %v", err)
		}
		// If orgs have been specified, warn if repo isn't under orgs
		if len(orgs) != 0 {
			data := strings.Split(repo, "/")
			if len(data) == 2 {
				if !stringInSortedSlice(data[0], sortedOrgs) {
					logrus.WithField("orgs", orgs).WithField("org", data[0]).WithField("repo", repo).Warn("Repo isn't inside orgs")
				}
			}
		}
	}
	return nil
}

// LabelsForTarget returns labels that have a given target
func LabelsForTarget(labels []Label, target LabelTarget) (filteredLabels []Label) {
	for _, label := range labels {
		if target == label.Target {
			filteredLabels = append(filteredLabels, label)
		}
	}
	// We also sort to make nice tables
	sort.Slice(filteredLabels, func(i, j int) bool { return filteredLabels[i].Name < filteredLabels[j].Name })
	return
}

// LoadConfig reads the yaml config at path
func LoadConfig(path string, orgs string) (*Configuration, error) {
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
	if err = c.validate(orgs); err != nil { // Ensure no dups
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

// loadRepos read what (filtered) repos exist under an org
func loadRepos(org string, gc client) ([]string, error) {
	org, isUser := GetOrg(org)
	repos, err := gc.GetRepos(org, isUser)
	if err != nil {
		return nil, err
	}
	var rl []string
	for _, r := range repos {
		// Skip Archived repos as they can't be modified in this way
		if r.Archived {
			continue
		}
		// Skip private security forks as they can't be modified in this way
		if r.Private && github.SecurityForkNameRE.MatchString(r.Name) {
			continue
		}
		rl = append(rl, r.Name)
	}
	return rl, nil
}

// loadLabels returns what labels exist in github
func loadLabels(gc client, org string, repos []string) (*RepoLabels, error) {
	repoChan := make(chan string, len(repos))
	for _, repo := range repos {
		repoChan <- repo
	}
	close(repoChan)

	wg := sync.WaitGroup{}
	wg.Add(maxConcurrentWorkers)
	labels := make(chan RepoLabels, len(repos))
	errChan := make(chan error, len(repos))
	for i := 0; i < maxConcurrentWorkers; i++ {
		go func(repositories <-chan string) {
			defer wg.Done()
			for repository := range repositories {
				logrus.WithField("org", org).WithField("repo", repository).Info("Listing labels for repo")
				repoLabels, err := gc.GetRepoLabels(org, repository)
				if err != nil {
					logrus.WithField("org", org).WithField("repo", repository).Error("Failed listing labels for repo")
					errChan <- err
				}
				labels <- RepoLabels{repository: repoLabels}
			}
		}(repoChan)
	}

	wg.Wait()
	close(labels)
	close(errChan)

	rl := RepoLabels{}
	for data := range labels {
		for repo, repoLabels := range data {
			rl[repo] = repoLabels
		}
	}

	var overallErr error
	if len(errChan) > 0 {
		var listErrs []error
		for listErr := range errChan {
			listErrs = append(listErrs, listErr)
		}
		overallErr = fmt.Errorf("failed to list labels: %v", listErrs)
	}

	return &rl, overallErr
}

// Delete the label
func kill(repo string, label Label) Update {
	logrus.WithField("repo", repo).WithField("label", label.Name).Info("kill")
	return Update{Why: "dead", Current: &label, repo: repo}
}

// Create the label
func create(repo string, label Label) Update {
	logrus.WithField("repo", repo).WithField("label", label.Name).Info("create")
	return Update{Why: "missing", Wanted: &label, repo: repo}
}

// Rename the label (will also update color)
func rename(repo string, previous, wanted Label) Update {
	logrus.WithField("repo", repo).WithField("from", previous.Name).WithField("to", wanted.Name).Info("rename")
	return Update{Why: "rename", Current: &previous, Wanted: &wanted, repo: repo}
}

// Update the label color/description
func change(repo string, label Label) Update {
	logrus.WithField("repo", repo).WithField("label", label.Name).WithField("color", label.Color).Info("change")
	return Update{Why: "change", Current: &label, Wanted: &label, repo: repo}
}

// Migrate labels to another label
func move(repo string, previous, wanted Label) Update {
	logrus.WithField("repo", repo).WithField("from", previous.Name).WithField("to", wanted.Name).Info("migrate")
	return Update{Why: "migrate", Wanted: &wanted, Current: &previous, repo: repo}
}

// classifyLabels will put labels into the required, archaic, dead maps as appropriate.
func classifyLabels(labels []Label, required, archaic, dead map[string]Label, now time.Time, parent *Label) (map[string]Label, map[string]Label, map[string]Label) {
	newRequired := copyLabelMap(required)
	newArchaic := copyLabelMap(archaic)
	newDead := copyLabelMap(dead)
	for i, l := range labels {
		first := parent
		if first == nil {
			first = &labels[i]
		}
		lower := strings.ToLower(l.Name)
		switch {
		case parent == nil && l.DeleteAfter == nil: // Live label
			newRequired[lower] = l
		case l.DeleteAfter != nil && now.After(*l.DeleteAfter):
			newDead[lower] = l
		case parent != nil:
			l.parent = parent
			newArchaic[lower] = l
		}
		newRequired, newArchaic, newDead = classifyLabels(l.Previously, newRequired, newArchaic, newDead, now, first)
	}
	return newRequired, newArchaic, newDead
}

func copyLabelMap(originalMap map[string]Label) map[string]Label {
	newMap := make(map[string]Label)
	for k, v := range originalMap {
		newMap[k] = v
	}
	return newMap
}

func syncLabels(config Configuration, org string, repos RepoLabels) (RepoUpdates, error) {
	// Find required, dead and archaic labels
	defaultRequired, defaultArchaic, defaultDead := classifyLabels(config.Default.Labels, make(map[string]Label), make(map[string]Label), make(map[string]Label), time.Now(), nil)

	var validationErrors []error
	var actions []Update
	// Process all repos
	for repo, repoLabels := range repos {
		var required, archaic, dead map[string]Label
		// Check if we have more labels for repo
		if repoconfig, ok := config.Repos[org+"/"+repo]; ok {
			// Use classifyLabels() to add them to default ones
			required, archaic, dead = classifyLabels(repoconfig.Labels, defaultRequired, defaultArchaic, defaultDead, time.Now(), nil)
		} else {
			// Otherwise just copy the pointers
			required = defaultRequired // Must exist
			archaic = defaultArchaic   // Migrate
			dead = defaultDead         // Delete
		}
		// Convert github.Label to Label
		var labels []Label
		for _, l := range repoLabels {
			labels = append(labels, Label{Name: l.Name, Description: l.Description, Color: l.Color})
		}
		// Check for any duplicate labels
		if _, err := validate(labels, "", make(map[string]string)); err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("invalid labels in %s: %v", repo, err))
			continue
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
			desired := Label{Name: l.parent.Name, Description: l.Description, Color: l.parent.Color}
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
				actions = append(actions, change(repo, l))
			case l.Description != cur.Description:
				actions = append(actions, change(repo, l))
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

	var overallErr error
	if len(validationErrors) > 0 {
		overallErr = fmt.Errorf("label validation failed: %v", validationErrors)
	}
	return u, overallErr
}

type repoUpdate struct {
	repo   string
	update Update
}

// DoUpdates iterates generated update data and adds and/or modifies labels on repositories
// Uses AddLabel GH API to add missing labels
// And UpdateLabel GH API to update color or name (name only when case differs)
func (ru RepoUpdates) DoUpdates(org string, gc client) error {
	var numUpdates int
	for _, updates := range ru {
		numUpdates += len(updates)
	}

	updateChan := make(chan repoUpdate, numUpdates)
	for repo, updates := range ru {
		logrus.WithField("org", org).WithField("repo", repo).Infof("Applying %d changes", len(updates))
		for _, item := range updates {
			updateChan <- repoUpdate{repo: repo, update: item}
		}
	}
	close(updateChan)

	wg := sync.WaitGroup{}
	wg.Add(maxConcurrentWorkers)
	errChan := make(chan error, numUpdates)
	for i := 0; i < maxConcurrentWorkers; i++ {
		go func(updates <-chan repoUpdate) {
			defer wg.Done()
			for item := range updates {
				repo := item.repo
				update := item.update
				logrus.WithField("org", org).WithField("repo", repo).WithField("why", update.Why).Debug("running update")
				switch update.Why {
				case "missing":
					err := gc.AddRepoLabel(org, repo, update.Wanted.Name, update.Wanted.Description, update.Wanted.Color)
					if err != nil {
						errChan <- err
					}
				case "change", "rename":
					err := gc.UpdateRepoLabel(org, repo, update.Current.Name, update.Wanted.Name, update.Wanted.Description, update.Wanted.Color)
					if err != nil {
						errChan <- err
					}
				case "dead":
					err := gc.DeleteRepoLabel(org, repo, update.Current.Name)
					if err != nil {
						errChan <- err
					}
				case "migrate":
					issues, err := gc.FindIssues(fmt.Sprintf("is:open repo:%s/%s label:\"%s\" -label:\"%s\"", org, repo, update.Current.Name, update.Wanted.Name), "", false)
					if err != nil {
						errChan <- err
					}
					if len(issues) == 0 {
						if err = gc.DeleteRepoLabel(org, repo, update.Current.Name); err != nil {
							errChan <- err
						}
					}
					for _, i := range issues {
						if err = gc.AddLabel(org, repo, i.Number, update.Wanted.Name); err != nil {
							errChan <- err
							continue
						}
						if err = gc.RemoveLabel(org, repo, i.Number, update.Current.Name); err != nil {
							errChan <- err
						}
					}
				default:
					errChan <- errors.New("unknown label operation: " + update.Why)
				}
			}
		}(updateChan)
	}

	wg.Wait()
	close(errChan)

	var overallErr error
	if len(errChan) > 0 {
		var updateErrs []error
		for updateErr := range errChan {
			updateErrs = append(updateErrs, updateErr)
		}
		overallErr = fmt.Errorf("failed to list labels: %v", updateErrs)
	}

	return overallErr
}

type client interface {
	AddRepoLabel(org, repo, name, description, color string) error
	UpdateRepoLabel(org, repo, currentName, newName, description, color string) error
	DeleteRepoLabel(org, repo, label string) error
	AddLabel(org, repo string, number int, label string) error
	RemoveLabel(org, repo string, number int, label string) error
	FindIssues(query, order string, ascending bool) ([]github.Issue, error)
	GetRepos(org string, isUser bool) ([]github.Repo, error)
	GetRepoLabels(string, string) ([]github.Label, error)
}

func newClient(tokenPath string, tokens, tokenBurst int, dryRun bool, graphqlEndpoint string, hosts ...string) (client, error) {
	if tokenPath == "" {
		return nil, errors.New("--token unset")
	}

	secretAgent := &secret.Agent{}
	if err := secretAgent.Start([]string{tokenPath}); err != nil {
		logrus.WithError(err).Fatal("Error starting secrets agent.")
	}

	if dryRun {
		return github.NewDryRunClient(secretAgent.GetTokenGenerator(tokenPath), secretAgent.Censor, graphqlEndpoint, hosts...), nil
	}
	c := github.NewClient(secretAgent.GetTokenGenerator(tokenPath), secretAgent.Censor, graphqlEndpoint, hosts...)
	if tokens > 0 && tokenBurst >= tokens {
		return nil, fmt.Errorf("--tokens=%d must exceed --token-burst=%d", tokens, tokenBurst)
	}
	if tokens > 0 {
		c.Throttle(tokens, tokenBurst) // 300 hourly tokens, bursts of 100
	}
	return c, nil
}

// Main function
// Typical run with production configuration should require no parameters
// It expects:
// "labels" file in "/etc/config/labels.yaml"
// github OAuth2 token in "/etc/github/oauth", this token must have write access to all org's repos
// It uses request retrying (in case of run out of GH API points)
// It took about 10 minutes to process all my 8 repos with all wanted "kubernetes" labels (70+)
// Next run takes about 22 seconds to check if all labels are correct on all repos
func main() {
	logrusutil.ComponentInit()

	flag.Parse()
	if *debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	config, err := LoadConfig(*labelsPath, *orgs)
	if err != nil {
		logrus.WithError(err).Fatalf("failed to load --config=%s", *labelsPath)
	}

	if *onlyRepos != "" && *skipRepos != "" {
		logrus.Fatalf("--only and --skip cannot both be set")
	}

	if *onlyRepos != "" && *orgs != "" {
		logrus.Fatalf("--only and --orgs cannot both be set")
	}

	switch {
	case *action == "docs":
		if err := writeDocs(*docsTemplate, *docsOutput, *config); err != nil {
			logrus.WithError(err).Fatalf("failed to write docs using docs-template %s to docs-output %s", *docsTemplate, *docsOutput)
		}
	case *action == "css":
		if err := writeCSS(*cssTemplate, *cssOutput, *config); err != nil {
			logrus.WithError(err).Fatalf("failed to write css file using css-template %s to css-output %s", *cssTemplate, *cssOutput)
		}
	case *action == "sync":
		githubClient, err := newClient(*token, *tokens, *tokenBurst, !*confirm, *graphqlEndpoint, endpoint.Strings()...)
		if err != nil {
			logrus.WithError(err).Fatal("failed to create client")
		}

		// there are three ways to configure which repos to sync:
		//  - a list of org/repo values
		//  - a list of orgs for which we sync all repos
		//  - a list of orgs to sync with a list of org/repo values to skip
		if *onlyRepos != "" {
			reposToSync, parseError := parseCommaDelimitedList(*onlyRepos)
			if parseError != nil {
				logrus.WithError(err).Fatal("invalid value for --only")
			}
			for org := range reposToSync {
				if err = syncOrg(org, githubClient, *config, reposToSync[org]); err != nil {
					logrus.WithError(err).Fatalf("failed to update %s", org)
				}
			}
			return
		}

		skippedRepos := map[string][]string{}
		if *skipRepos != "" {
			reposToSkip, parseError := parseCommaDelimitedList(*skipRepos)
			if parseError != nil {
				logrus.WithError(err).Fatal("invalid value for --skip")
			}
			skippedRepos = reposToSkip
		}

		for _, org := range strings.Split(*orgs, ",") {
			org = strings.TrimSpace(org)
			logger := logrus.WithField("org", org)
			logger.Info("Reading repos")
			repos, err := loadRepos(org, githubClient)
			if err != nil {
				logger.WithError(err).Fatalf("failed to read repos")
			}
			if skipped, exist := skippedRepos[org]; exist {
				repos = sets.NewString(repos...).Difference(sets.NewString(skipped...)).UnsortedList()
			}
			if err = syncOrg(org, githubClient, *config, repos); err != nil {
				logrus.WithError(err).Fatalf("failed to update %s", org)
			}
		}
	default:
		logrus.Fatalf("unrecognized action: %s", *action)
	}
}

// parseCommaDelimitedList parses values in the format:
//   org/repo,org2/repo2,org/repo3
// into a mapping of org to repos, i.e.:
//   org:  repo, repo3
//   org2: repo2
func parseCommaDelimitedList(list string) (map[string][]string, error) {
	mapping := map[string][]string{}
	for _, r := range strings.Split(list, ",") {
		value := strings.TrimSpace(r)
		if strings.Count(value, "/") != 1 {
			return nil, fmt.Errorf("invalid org/repo value %q", value)
		}
		parts := strings.SplitN(value, "/", 2)
		if others, exist := mapping[parts[0]]; !exist {
			mapping[parts[0]] = []string{parts[1]}
		} else {
			mapping[parts[0]] = append(others, parts[1])
		}
	}
	return mapping, nil
}

type labelData struct {
	Description, Link, Labels interface{}
}

func writeDocs(template string, output string, config Configuration) error {
	var desc string
	data := []labelData{}
	desc = "all repos, for both issues and PRs"
	data = append(data, labelData{desc, linkify(desc), LabelsForTarget(config.Default.Labels, bothTarget)})
	desc = "all repos, only for issues"
	data = append(data, labelData{desc, linkify(desc), LabelsForTarget(config.Default.Labels, issueTarget)})
	desc = "all repos, only for PRs"
	data = append(data, labelData{desc, linkify(desc), LabelsForTarget(config.Default.Labels, prTarget)})
	// Let's sort repos
	repos := make([]string, 0)
	for repo := range config.Repos {
		repos = append(repos, repo)
	}
	sort.Strings(repos)
	// And append their labels
	for _, repo := range repos {
		if l := LabelsForTarget(config.Repos[repo].Labels, bothTarget); len(l) > 0 {
			desc = repo + ", for both issues and PRs"
			data = append(data, labelData{desc, linkify(desc), l})
		}
		if l := LabelsForTarget(config.Repos[repo].Labels, issueTarget); len(l) > 0 {
			desc = repo + ", only for issues"
			data = append(data, labelData{desc, linkify(desc), l})
		}
		if l := LabelsForTarget(config.Repos[repo].Labels, prTarget); len(l) > 0 {
			desc = repo + ", only for PRs"
			data = append(data, labelData{desc, linkify(desc), l})
		}
	}
	if err := writeTemplate(*docsTemplate, *docsOutput, data); err != nil {
		return err
	}
	return nil
}

// linkify transforms a string into a markdown anchor link
// I could not find a proper doc, so rules here a mostly empirical
func linkify(text string) string {
	// swap space with dash
	link := strings.Replace(text, " ", "-", -1)
	// discard some special characters
	discard, _ := regexp.Compile("[,/]")
	link = discard.ReplaceAllString(link, "")
	// lowercase
	return strings.ToLower(link)
}

func syncOrg(org string, githubClient client, config Configuration, repos []string) error {
	logger := logrus.WithField("org", org)
	logger.Infof("Found %d repos", len(repos))
	currLabels, err := loadLabels(githubClient, org, repos)
	if err != nil {
		return err
	}

	logger.Infof("Syncing labels for %d repos", len(repos))
	updates, err := syncLabels(config, org, *currLabels)
	if err != nil {
		return err
	}

	y, _ := yaml.Marshal(updates)
	logger.Debug(string(y))

	if !*confirm {
		logger.Infof("Running without --confirm, no mutations made")
		return nil
	}

	if err = updates.DoUpdates(org, githubClient); err != nil {
		return err
	}
	return nil
}

type labelCSSData struct {
	BackgroundColor, Color, Name string
}

// Returns the CSS escaped label name. Escaped method based on
// https://www.w3.org/International/questions/qa-escapes#cssescapes
func cssEscape(s string) (escaped string) {
	var IsAlpha = regexp.MustCompile(`^[a-zA-Z]+$`).MatchString
	for i, c := range s {
		if (i == 0 && unicode.IsDigit(c)) || !(unicode.IsDigit(c) || IsAlpha(string(c))) {
			escaped += fmt.Sprintf("x%0.6x", c)
			continue
		}
		escaped += string(c)
	}
	return
}

// Returns the text color (whether black or white) given the background color.
// Details: https://www.w3.org/TR/WCAG20/#contrastratio
func getTextColor(backgroundColor string) (string, error) {
	d, err := hex.DecodeString(backgroundColor)
	if err != nil || len(d) != 3 {
		return "", errors.New("expect 6-digit color hex of label")
	}

	// Calculate the relative luminance (L) of a color
	// L = 0.2126 * R + 0.7152 * G + 0.0722 * B
	// Formula details at: https://www.w3.org/TR/WCAG20/#relativeluminancedef
	color := [3]float64{}
	for i, v := range d {
		color[i] = float64(v) / 255.0
		if color[i] <= 0.03928 {
			color[i] = color[i] / 12.92
		} else {
			color[i] = math.Pow((color[i]+0.055)/1.055, 2.4)
		}
	}
	L := 0.2126*color[0] + 0.7152*color[1] + 0.0722*color[2]

	if (L+0.05)/(0.0+0.05) > (1.0+0.05)/(L+0.05) {
		return "000000", nil
	}
	return "ffffff", nil
}

func writeCSS(tmplPath string, outPath string, config Configuration) error {
	var labelCSS []labelCSSData
	for _, l := range config.Labels() {
		textColor, err := getTextColor(l.Color)
		if err != nil {
			return err
		}

		labelCSS = append(labelCSS, labelCSSData{
			BackgroundColor: l.Color,
			Color:           textColor,
			Name:            cssEscape(l.Name),
		})
	}

	return writeTemplate(tmplPath, outPath, labelCSS)
}
