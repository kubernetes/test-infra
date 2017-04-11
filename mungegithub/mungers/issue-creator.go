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
	"strings"

	githubapi "github.com/google/go-github/github"
	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/mungers/testowner"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

var (
	IssueCreatorName = "issue-creator"
)

// RepoClient is the interface IssueCreator used to interact with github.
// This interface is necessary for testing the IssueCreator with dependency injection.
type RepoClient interface {
	GetUser(login string) (*githubapi.User, error)
	GetLabels() ([]*githubapi.Label, error)
	ListAllIssues(options *githubapi.IssueListByRepoOptions) ([]*githubapi.Issue, error)
	NewIssue(title, body string, labels, owners []string) (*github.MungeObject, error)

	isDryRun() bool
	getOrg() string
	getProject() string

	// RealConfig gets the underlying *github.Config or returns nil if the RepoClient is a testing instance.
	RealConfig() *github.Config
}

// gihubConfig is a type alias of github.Config that implements the RepoClient interface.
type githubConfig github.Config

func (c *githubConfig) RealConfig() *github.Config {
	return (*github.Config)(c)
}

func (c *githubConfig) getOrg() string {
	return c.Org
}

func (c *githubConfig) getProject() string {
	return c.Project
}

func (c *githubConfig) isDryRun() bool {
	return c.DryRun
}

func (c *githubConfig) GetUser(login string) (*githubapi.User, error) {
	return ((*github.Config)(c)).GetUser(login)
}

func (c *githubConfig) GetLabels() ([]*githubapi.Label, error) {
	return ((*github.Config)(c)).GetLabels()
}

func (c *githubConfig) ListAllIssues(options *githubapi.IssueListByRepoOptions) ([]*githubapi.Issue, error) {
	return ((*github.Config)(c)).ListAllIssues(options)
}

func (c *githubConfig) NewIssue(title, body string, labels, owners []string) (*github.MungeObject, error) {
	return ((*github.Config)(c)).NewIssue(title, body, labels, owners)
}

// OwnerMapper finds an owner for a given test name.
type OwnerMapper interface {
	// TestOwner returns a GitHub username for a test, or "" if none are found.
	TestOwner(testName string) string

	// TestSIG returns the name of the Special Interest Group (SIG) which owns the test , or "" if none are found.
	TestSIG(testName string) string
}

// Issue is an interface implemented by structs that can be synced with github issues via the IssueCreator.
type Issue interface {
	// Title yields the initial title text of the github issue.
	Title() string
	// Body yields the body text of the github issue and *must* contain the output of ID().
	// closedIssues is a (potentially empty) slice containing all closed
	// issues authored by this bot that contain ID() in their body.
	// if Body returns an empty string no issue is created.
	Body(closedIssues []*githubapi.Issue) string
	// ID returns a string that uniquely identifies this issue.
	// This ID must appear in the body of the issue.
	// DO NOT CHANGE how this ID is formatted or duplicate issues will be created
	// on github for this issue
	ID() string
	// Labels specifies the set of labels to apply to this issue on github.
	Labels() []string
	// Owners returns the github usernames to assign the issue to or nil/empty for no assignment.
	Owners() []string
	// Priority calculates and returns the priority of this issue
	// The returned bool indicates if the returned priority is valid and can be used
	Priority() (string, bool)
}

// IssueCreator handles syncing identified issues with github issues.
// This includes finding existing github issues, creating new ones, and ensuring that duplicate
// github issues are not created.
type IssueCreator struct {
	// config is the github client that is used to interact with github.
	config RepoClient
	// validLabels is the set of labels that are valid for the current repo (populated from github).
	validLabels []*githubapi.Label
	// authorName is the name of the current bot.
	authorName string
	// allIssues is a local cache of all issues in the repo authored by the currently authenticated user.
	// Issues are keyed by issue number.
	allIssues map[int]*githubapi.Issue

	// ownerPath is the path the the test owners csv file or "" if no assignments or SIG areas should be used.
	ownerPath string
	// maxSIGCount is the maximum number of SIG areas to include on a single github issue.
	maxSIGCount int
	// maxAssignees is the maximum number of user to assign to a single github issue.
	maxAssignees int

	// owners is an OwnerMapper that maps test names to owners and SIG areas.
	owners OwnerMapper
}

func init() {
	RegisterMungerOrDie(&IssueCreator{})
}

// Initialize prepares an IssueCreator for use by other mungers.
// This includes determining the currently authenticated user, fetching all issues created by that user
// from github, fetching the labels that are valid for the repo, and initializing the test owner and sig data.
func (c *IssueCreator) Initialize(config *github.Config, feats *features.Features) error {
	cfg := githubConfig(*config)
	c.config = RepoClient(&cfg)

	var err error
	if c.ownerPath != "" {
		c.owners, err = testowner.NewReloadingOwnerList(c.ownerPath)
		if err != nil {
			return err
		}
	}
	if err = c.loadCache(); err != nil {
		return err
	}
	return nil
}

// loadCache loads the valid labels for the repo, the currently authenticated user, and the issue cache from github.
func (c *IssueCreator) loadCache() error {
	user, err := c.config.GetUser("")
	if err != nil {
		return fmt.Errorf("failed to fetch the User struct for the current authenticated user. errmsg: %v\n", err)
	}
	if user == nil {
		return fmt.Errorf("received a nil User struct pointer when trying to look up the currently authenticated user.")
	}
	if user.Login == nil {
		return fmt.Errorf("the user struct for the currently authenticated user does not specify a login.")
	}
	c.authorName = *user.Login

	// Try to get the list of valid labels for the repo.
	if c.validLabels, err = c.config.GetLabels(); err != nil {
		c.validLabels = nil
		glog.Warningf("Failed to retreive the list of valid labels for repo '%s/%s'. Allowing all labels. errmsg: %v\n", c.config.getOrg(), c.config.getProject(), err)
	}

	// Populate the issue cache (allIssues).
	issues, err := c.config.ListAllIssues(&githubapi.IssueListByRepoOptions{
		State:   "all",
		Creator: c.authorName,
	})
	if err != nil {
		return fmt.Errorf("failed to refresh the list of all issues created by %s in repo '%s/%s'. errmsg: %v\n", c.authorName, c.config.getOrg(), c.config.getProject(), err)
	}
	if len(issues) == 0 {
		glog.Warningf("IssueCreator found no issues in the repo '%s/%s' authored by %s.\n", c.config.getOrg(), c.config.getProject(), c.authorName)
	}
	c.allIssues = make(map[int]*githubapi.Issue)
	for _, i := range issues {
		c.allIssues[*i.Number] = i
	}
	return nil
}

// Name specifies the name of this munger to be used with the --pr-mungers flag.
func (c *IssueCreator) Name() string { return IssueCreatorName }

// RequiredFeatures specifies the features required by the IssueCreator (none needed).
func (c *IssueCreator) RequiredFeatures() []string { return []string{} }

// EachLoop is called at the start of every munge loop. The IssueCreator does not use this.
func (c *IssueCreator) EachLoop() error { return nil }

// AddFlags adds flags needed by the IssueCreator to the combra `cmd`.
func (c *IssueCreator) AddFlags(cmd *cobra.Command, config *github.Config) {
	cmd.Flags().StringVar(&c.ownerPath, "test-owners-csv", "", "file containing a CSV-exported test-owners spreadsheet")
	cmd.Flags().IntVar(&c.maxSIGCount, "maxSIGs", 3, "The maximum number of SIG labels to attach to an issue.")
	cmd.Flags().IntVar(&c.maxAssignees, "maxAssignees", 3, "The maximum number of users to assign to an issue.")
}

// Munge updates the IssueCreator's cache of issues if the munge object provided is an issue authored by the currently authenticated user.
func (c *IssueCreator) Munge(obj *github.MungeObject) {
	if obj.IsPR() {
		return
	}
	if *obj.Issue.User.Login != c.authorName {
		return
	}
	c.allIssues[*obj.Issue.Number] = obj.Issue
}

// filterValidLabels extracts the labels that are valid for the current repo from a set of labels.
// If a candidate label is not valid, an error is logged, but execution continues and the invalid
// label is ignored.
func (c *IssueCreator) filterValidLabels(candidates []string) []string {
	if c.validLabels == nil {
		// Failed to fetch list of valid labels for this repo so trust that labels are valid.
		return candidates
	}
	var validCandidates []string
	for _, l := range candidates {
		found := false
		for _, validLabel := range c.validLabels {
			if l == *validLabel.Name {
				found = true
				break
			}
		}
		if !found {
			glog.Errorf("The label '%s' is invalid for the repo '%s/%s'. Dropping label.", l, c.config.getOrg(), c.config.getProject())
		} else {
			validCandidates = append(validCandidates, l)
		}
	}
	return validCandidates
}

// Sync checks to see if an issue is already on github and tries to create a new issue for it if it is not.
func (c *IssueCreator) Sync(issue Issue) {
	// First look for existing issues with this ID.
	id := issue.ID()
	var closedIssues []*githubapi.Issue
	for _, i := range c.allIssues {
		if strings.Contains(*i.Body, id) {
			switch *i.State {
			case "open":
				//if an open issue is found with the ID then the issue is already synced
				return
			case "closed":
				closedIssues = append(closedIssues, i)
			default:
				glog.Errorf("Unrecognized issue state '%s' for issue #%d. Ignoring this issue.\n", *i.State, *i.Number)
			}
		}
	}
	// No open issues exist for the ID.
	body := issue.Body(closedIssues)
	if body == "" {
		// Issue indicated that it should not be synced.
		return
	}
	if !strings.Contains(body, id) {
		glog.Fatalf("Programmer error: The following body text does not contain id '%s'.\n%s\n", id, body)
	}

	title := issue.Title()
	owners := issue.Owners()
	labels := issue.Labels()
	if prio, ok := issue.Priority(); ok {
		labels = append(labels, "priority/"+prio)
	}
	labels = c.filterValidLabels(labels)

	if c.config.isDryRun() {
		glog.Infof("DryRun--Create Issue:\nTitle:%s\nBody:\n%s\nLabels:%v\nAssigned to:%s\n", title, body, labels, owners)
		return
	}
	obj, err := c.config.NewIssue(title, body, labels, owners)
	if err != nil {
		glog.Errorf("Failed to create a new github issue for issue ID '%s'.\n", id)
		return
	}
	c.allIssues[*obj.Issue.Number] = obj.Issue
}

// TestSIG uses the IssueCreator's OwnerMapper to look up the SIG for a test.
func (c *IssueCreator) TestSIG(testName string) string {
	if c.owners == nil {
		return ""
	}
	return c.owners.TestSIG(testName)
}

// TestOwner uses the IssueCreator's OwnerMapper to look up the user assigned to a test.
func (c *IssueCreator) TestOwner(testName string) string {
	if c.owners == nil {
		return ""
	}
	return c.owners.TestOwner(testName)
}

// TestSIG uses the IssueCreator's OwnerMapper to look up the SIGs for a list of tests.
// The number of SIGs returned is limited by maxSIGCount.
func (c *IssueCreator) TestsSIGs(testNames []string) []string {
	if c.owners == nil {
		return []string{}
	}
	var sigs []string
	for _, test := range testNames {
		sig := c.owners.TestSIG(test)
		if sig == "" {
			continue
		}
		found := false
		for _, oldsig := range sigs {
			if sig == oldsig {
				found = true
				break
			}
		}
		if !found {
			sigs = append(sigs, sig)
			if len(sigs) >= c.maxSIGCount {
				break
			}
		}
	}
	return sigs
}

// TestOwner uses the IssueCreator's OwnerMapper to look up the users assigned to a list of tests.
// The number of users returned is limited by maxAssignees.
func (c *IssueCreator) TestsOwners(testNames []string) []string {
	if c.owners == nil {
		return []string{}
	}
	var users []string
	for _, test := range testNames {
		user := c.owners.TestOwner(test)
		if user == "" {
			continue
		}
		found := false
		for _, olduser := range users {
			if olduser == user {
				found = true
				break
			}
		}
		if !found {
			users = append(users, user)
			if len(users) >= c.maxAssignees {
				break
			}
		}
	}
	return users
}
