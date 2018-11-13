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

package creator

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/google/go-github/github"
	"k8s.io/test-infra/pkg/ghclient"
	"k8s.io/test-infra/robots/issue-creator/testowner"

	"github.com/golang/glog"
)

// RepoClient is the interface IssueCreator used to interact with github.
// This interface is necessary for testing the IssueCreator with dependency injection.
type RepoClient interface {
	GetUser(login string) (*github.User, error)
	GetRepoLabels(org, repo string) ([]*github.Label, error)
	GetIssues(org, repo string, options *github.IssueListByRepoOptions) ([]*github.Issue, error)
	CreateIssue(org, repo, title, body string, labels, owners []string) (*github.Issue, error)
	GetCollaborators(org, repo string) ([]*github.User, error)
}

// gihubClient is an wrapper of ghclient.Client that implements the RepoClient interface.
// This is used for dependency injection testing.
type githubClient struct {
	*ghclient.Client
}

func (c githubClient) GetUser(login string) (*github.User, error) {
	return c.Client.GetUser(login)
}

func (c githubClient) GetRepoLabels(org, repo string) ([]*github.Label, error) {
	return c.Client.GetRepoLabels(org, repo)
}

func (c githubClient) GetIssues(org, repo string, options *github.IssueListByRepoOptions) ([]*github.Issue, error) {
	return c.Client.GetIssues(org, repo, options)
}

func (c githubClient) CreateIssue(org, repo, title, body string, labels, owners []string) (*github.Issue, error) {
	return c.Client.CreateIssue(org, repo, title, body, labels, owners)
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
	Body(closedIssues []*github.Issue) string
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

// IssueSource represents a source of auto-filed issues, such as triage-filer or flakyjob-reporter.
type IssueSource interface {
	Issues(*IssueCreator) ([]Issue, error)
	RegisterFlags()
}

// IssueCreator handles syncing identified issues with github issues.
// This includes finding existing github issues, creating new ones, and ensuring that duplicate
// github issues are not created.
type IssueCreator struct {
	// client is the github client that is used to interact with github.
	client RepoClient
	// validLabels is the set of labels that are valid for the current repo (populated from github).
	validLabels []string
	// Collaborators is the set of Users that are valid assignees for the current repo (populated from GH).
	Collaborators []string
	// authorName is the name of the current bot.
	authorName string
	// allIssues is a local cache of all issues in the repo authored by the currently authenticated user.
	// Issues are keyed by issue number.
	allIssues map[int]*github.Issue

	// ownerPath is the path the test owners csv file or "" if no assignments or SIG areas should be used.
	ownerPath string
	// maxSIGCount is the maximum number of SIG areas to include on a single github issue.
	MaxSIGCount int
	// maxAssignees is the maximum number of user to assign to a single github issue.
	MaxAssignees int
	// tokenFIle is the file containing the github authentication token to use.
	tokenFile string
	// dryRun is true iff no modifying or 'write' operations should be made to github.
	dryRun bool
	// project is the name of the github repo.
	project string
	// org is the github organization that owns the repo.
	org string

	// Owners is an OwnerMapper that maps test names to owners and SIG areas.
	Owners OwnerMapper
}

var sources = map[string]IssueSource{}

// RegisterSourceOrDie registers a source of auto-filed issues.
func RegisterSourceOrDie(name string, src IssueSource) {
	if _, ok := sources[name]; ok {
		glog.Fatalf("Cannot register an IssueSource with name %q, already exists!", name)
	}
	sources[name] = src
	glog.Infof("Registered issue source '%s'.", name)
}

func (c *IssueCreator) initialize() error {
	if c.org == "" {
		return errors.New("'--org' is a required flag")
	}
	if c.project == "" {
		return errors.New("'--project' is a required flag")
	}
	if c.tokenFile == "" {
		return errors.New("'--token-file' is a required flag")
	}
	b, err := ioutil.ReadFile(c.tokenFile)
	if err != nil {
		return fmt.Errorf("failed to read token file '%s': %v", c.tokenFile, err)
	}
	token := strings.TrimSpace(string(b))

	c.client = RepoClient(githubClient{ghclient.NewClient(token, c.dryRun)})

	if c.ownerPath == "" {
		c.Owners = nil
	} else {
		var err error
		if c.Owners, err = testowner.NewReloadingOwnerList(c.ownerPath); err != nil {
			return err
		}
	}

	return c.loadCache()
}

// CreateAndSync is the main workhorse function of IssueCreator. It initializes the IssueCreator,
// asks each source for its issues to sync, and syncs the issues.
func (c *IssueCreator) CreateAndSync() {
	var err error
	if err = c.initialize(); err != nil {
		glog.Fatalf("Error initializing IssueCreator: %v.", err)
	}
	glog.Info("IssueCreator initialization complete.")

	for srcName, src := range sources {
		glog.Infof("Generating issues from source: %s.", srcName)
		var issues []Issue
		if issues, err = src.Issues(c); err != nil {
			glog.Errorf("Error generating issues. Source: %s Msg: %v.", srcName, err)
			continue
		}

		// Note: We assume that no issues made by this bot with ID's matching issues generated by
		// sources will be created while this code is creating issues. If this is a possibility then
		// this loop should be updated to fetch recently changed issues from github after every issue
		// sync that results in an issue being created.
		glog.Infof("Syncing issues from source: %s.", srcName)
		created := 0
		for _, issue := range issues {
			if c.sync(issue) {
				created++
			}
		}
		glog.Infof(
			"Created issues for %d of the %d issues synced from source: %s.",
			created,
			len(issues),
			srcName,
		)
	}
}

// loadCache loads the valid labels for the repo, the currently authenticated user, and the issue cache from github.
func (c *IssueCreator) loadCache() error {
	user, err := c.client.GetUser("")
	if err != nil {
		return fmt.Errorf("failed to fetch the User struct for the current authenticated user. errmsg: %v", err)
	}
	if user == nil {
		return fmt.Errorf("received a nil User struct pointer when trying to look up the currently authenticated user")
	}
	if user.Login == nil {
		return fmt.Errorf("the user struct for the currently authenticated user does not specify a login")
	}
	c.authorName = *user.Login

	// Try to get the list of valid labels for the repo.
	if validLabels, err := c.client.GetRepoLabels(c.org, c.project); err != nil {
		c.validLabels = nil
		glog.Errorf("Failed to retrieve the list of valid labels for repo '%s/%s'. Allowing all labels. errmsg: %v\n", c.org, c.project, err)
	} else {
		c.validLabels = make([]string, 0, len(validLabels))
		for _, label := range validLabels {
			if label.Name != nil && *label.Name != "" {
				c.validLabels = append(c.validLabels, *label.Name)
			}
		}
	}
	// Try to get the valid collaborators for the repo.
	if collaborators, err := c.client.GetCollaborators(c.org, c.project); err != nil {
		c.Collaborators = nil
		glog.Errorf("Failed to retrieve the list of valid collaborators for repo '%s/%s'. Allowing all assignees. errmsg: %v\n", c.org, c.project, err)
	} else {
		c.Collaborators = make([]string, 0, len(collaborators))
		for _, user := range collaborators {
			if user.Login != nil && *user.Login != "" {
				c.Collaborators = append(c.Collaborators, strings.ToLower(*user.Login))
			}
		}
	}

	// Populate the issue cache (allIssues).
	issues, err := c.client.GetIssues(
		c.org,
		c.project,
		&github.IssueListByRepoOptions{
			State:   "all",
			Creator: c.authorName,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to refresh the list of all issues created by %s in repo '%s/%s'. errmsg: %v", c.authorName, c.org, c.project, err)
	}
	if len(issues) == 0 {
		glog.Warningf("IssueCreator found no issues in the repo '%s/%s' authored by '%s'.\n", c.org, c.project, c.authorName)
	}
	c.allIssues = make(map[int]*github.Issue)
	for _, i := range issues {
		c.allIssues[*i.Number] = i
	}
	return nil
}

// RegisterFlags registers options for this munger; returns any that require a restart when changed.
func (c *IssueCreator) RegisterFlags() {
	flag.StringVar(&c.ownerPath, "test-owners-csv", "", "file containing a CSV-exported test-owners spreadsheet")
	flag.IntVar(&c.MaxSIGCount, "maxSIGs", 3, "The maximum number of SIG labels to attach to an issue.")
	flag.IntVar(&c.MaxAssignees, "maxAssignees", 3, "The maximum number of users to assign to an issue.")

	flag.StringVar(&c.tokenFile, "token-file", "", "The file containing the github authentication token to use.")
	flag.StringVar(&c.project, "project", "", "The name of the github repo to create issues in.")
	flag.StringVar(&c.org, "org", "", "The name of the organization that owns the repo to create issues in.")
	flag.BoolVar(&c.dryRun, "dry-run", true, "True iff only 'read' operations should be made on github.")

	for _, src := range sources {
		src.RegisterFlags()
	}
}

// setIntersect removes any elements from the first list that are not in the second, returning the
// new set and the removed elements.
func setIntersect(a, b []string) (filtered, removed []string) {
	for _, elemA := range a {
		found := false
		for _, elemB := range b {
			if elemA == elemB {
				found = true
				break
			}
		}
		if found {
			filtered = append(filtered, elemA)
		} else {
			removed = append(removed, elemA)
		}
	}
	return
}

// sync checks to see if an issue is already on github and tries to create a new issue for it if it is not.
// True is returned iff a new issue is created.
func (c *IssueCreator) sync(issue Issue) bool {
	// First look for existing issues with this ID.
	id := issue.ID()
	var closedIssues []*github.Issue
	for _, i := range c.allIssues {
		if strings.Contains(*i.Body, id) {
			switch *i.State {
			case "open":
				//if an open issue is found with the ID then the issue is already synced
				return false
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
		glog.Infof("Issue aborted sync by providing \"\" (empty) body. ID: %s.", id)
		return false
	}
	if !strings.Contains(body, id) {
		glog.Fatalf("Programmer error: The following body text does not contain id '%s'.\n%s\n", id, body)
	}

	title := issue.Title()
	owners := issue.Owners()
	if c.Collaborators != nil {
		var removedOwners []string
		owners, removedOwners = setIntersect(owners, c.Collaborators)
		if len(removedOwners) > 0 {
			glog.Errorf("Filtered the following invalid assignees from issue %q: %q.", title, removedOwners)
		}
	}

	labels := issue.Labels()
	if prio, ok := issue.Priority(); ok {
		labels = append(labels, "priority/"+prio)
	}
	if c.validLabels != nil {
		var removedLabels []string
		labels, removedLabels = setIntersect(labels, c.validLabels)
		if len(removedLabels) > 0 {
			glog.Errorf("Filtered the following invalid labels from issue %q: %q.", title, removedLabels)
		}
	}

	glog.Infof("Create Issue: %q Assigned to: %q\n", title, owners)
	if c.dryRun {
		return true
	}

	created, err := c.client.CreateIssue(c.org, c.project, title, body, labels, owners)
	if err != nil {
		glog.Errorf("Failed to create a new github issue for issue ID '%s'.\n", id)
		return false
	}
	c.allIssues[*created.Number] = created
	return true
}

// TestSIG uses the IssueCreator's OwnerMapper to look up the SIG for a test.
func (c *IssueCreator) TestSIG(testName string) string {
	if c.Owners == nil {
		return ""
	}
	return c.Owners.TestSIG(testName)
}

// TestOwner uses the IssueCreator's OwnerMapper to look up the user assigned to a test.
func (c *IssueCreator) TestOwner(testName string) string {
	if c.Owners == nil {
		return ""
	}
	owner := c.Owners.TestOwner(testName)
	if !c.isAssignable(owner) {
		return ""
	}
	return owner
}

// TestsSIGs uses the IssueCreator's OwnerMapper to look up the SIGs for a list of tests.
// The number of SIGs returned is limited by MaxSIGCount.
// The return value is a map from sigs to the tests from testNames that each sig owns.
func (c *IssueCreator) TestsSIGs(testNames []string) map[string][]string {
	if c.Owners == nil {
		return nil
	}
	sigs := make(map[string][]string)
	for _, test := range testNames {
		sig := c.Owners.TestSIG(test)
		if sig == "" {
			continue
		}

		if len(sigs) >= c.MaxSIGCount {
			if tests, ok := sigs[sig]; ok {
				sigs[sig] = append(tests, test)
			}
		} else {
			sigs[sig] = append(sigs[sig], test)
		}
	}
	return sigs
}

// TestsOwners uses the IssueCreator's OwnerMapper to look up the users assigned to a list of tests.
// The number of users returned is limited by MaxAssignees.
// The return value is a map from users to the test names from testNames that each user owns.
func (c *IssueCreator) TestsOwners(testNames []string) map[string][]string {
	if c.Owners == nil {
		return nil
	}
	users := make(map[string][]string)
	for _, test := range testNames {
		user := c.TestOwner(test)
		if user == "" {
			continue
		}

		if len(users) >= c.MaxAssignees {
			if tests, ok := users[user]; ok {
				users[user] = append(tests, test)
			}
		} else {
			users[user] = append(users[user], test)
		}
	}
	return users
}

// ExplainTestAssignments returns a string explaining how tests caused the individual/sig assignments.
func (c *IssueCreator) ExplainTestAssignments(testNames []string) string {
	assignees := c.TestsOwners(testNames)
	sigs := c.TestsSIGs(testNames)
	var buf bytes.Buffer
	if len(assignees) > 0 || len(sigs) > 0 {
		fmt.Fprint(&buf, "\n<details><summary>Rationale for assignments:</summary>\n")
		fmt.Fprint(&buf, "\n| Assignee or SIG area | Owns test(s) |\n| --- | --- |\n")
		for assignee, tests := range assignees {
			if len(tests) > 3 {
				tests = tests[0:3]
			}
			fmt.Fprintf(&buf, "| %s | %s |\n", assignee, strings.Join(tests, "; "))
		}
		for sig, tests := range sigs {
			if len(tests) > 3 {
				tests = tests[0:3]
			}
			fmt.Fprintf(&buf, "| sig/%s | %s |\n", sig, strings.Join(tests, "; "))
		}
		fmt.Fprint(&buf, "\n</details><br>\n")
	}
	return buf.String()
}

func (c *IssueCreator) isAssignable(login string) bool {
	if c.Collaborators == nil {
		return true
	}

	login = strings.ToLower(login)
	for _, user := range c.Collaborators {
		if user == login {
			return true
		}
	}
	return false
}
