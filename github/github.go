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

package github

import (
	"bytes"
	goflag "flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"text/tabwriter"
	"time"

	"k8s.io/kubernetes/pkg/util"
	"k8s.io/kubernetes/pkg/util/sets"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
	"github.com/gregjones/httpcache"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
)

const (
	// stolen from https://groups.google.com/forum/#!msg/golang-nuts/a9PitPAHSSU/ziQw1-QHw3EJ
	maxInt = int(^uint(0) >> 1)
)

type rateLimitRoundTripper struct {
	delegate http.RoundTripper
	throttle util.RateLimiter
}

func (r *rateLimitRoundTripper) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	r.throttle.Accept()
	return r.delegate.RoundTrip(req)
}

// Config is how we are configured to talk to github and provides access
// methods for doing so.
type Config struct {
	client  *github.Client
	Org     string
	Project string

	RateLimit      float32
	RateLimitBurst int

	Token     string
	TokenFile string

	MinPRNumber int
	MaxPRNumber int

	// If true, don't make any mutating API calls
	DryRun bool

	useMemoryCache bool

	analytics analytics
}

type analytic int

func (a *analytic) Call(config *Config) {
	config.analytics.apiCount++
	*a = *a + 1
}

type analytics struct {
	lastAPIReset time.Time
	apiCount     int // number of times we called a github API

	AddLabels         analytic
	RemoveLabels      analytic
	ListCollaborators analytic
	ListIssues        analytic
	ListIssueEvents   analytic
	ListCommits       analytic
	GetCommit         analytic
	GetCombinedStatus analytic
	GetPR             analytic
	AssignPR          analytic
	ClosePR           analytic
	OpenPR            analytic
	GetContents       analytic
	CreateComment     analytic
	Merge             analytic
}

func (a analytics) Print() {
	since := time.Since(a.lastAPIReset)
	callsPerSec := float64(a.apiCount) / since.Seconds()
	glog.Infof("Made %d API calls since the last Reset %f calls/sec", a.apiCount, callsPerSec)

	buf := new(bytes.Buffer)
	w := new(tabwriter.Writer)
	w.Init(buf, 0, 0, 1, ' ', tabwriter.AlignRight)
	fmt.Fprintf(w, "AddLabels\t%d\t\n", a.AddLabels)
	fmt.Fprintf(w, "RemoveLabels\t%d\t\n", a.RemoveLabels)
	fmt.Fprintf(w, "ListCollaborators\t%d\t\n", a.ListCollaborators)
	fmt.Fprintf(w, "ListIssues\t%d\t\n", a.ListIssues)
	fmt.Fprintf(w, "ListIssueEvents\t%d\t\n", a.ListIssueEvents)
	fmt.Fprintf(w, "ListCommits\t%d\t\n", a.ListCommits)
	fmt.Fprintf(w, "GetCommit\t%d\t\n", a.GetCommit)
	fmt.Fprintf(w, "GetCombinedStatus\t%d\t\n", a.GetCombinedStatus)
	fmt.Fprintf(w, "GetPR\t%d\t\n", a.GetPR)
	fmt.Fprintf(w, "AssignPR\t%d\t\n", a.AssignPR)
	fmt.Fprintf(w, "ClosePR\t%d\t\n", a.ClosePR)
	fmt.Fprintf(w, "OpenPR\t%d\t\n", a.OpenPR)
	fmt.Fprintf(w, "GetContents\t%d\t\n", a.GetContents)
	fmt.Fprintf(w, "CreateComment\t%d\t\n", a.CreateComment)
	fmt.Fprintf(w, "Merge\t%d\t\n", a.Merge)
	w.Flush()
	glog.V(2).Infof("\n%v", buf)
}

// AddRootFlags will add all of the flags needed for the github config to the cobra command
func (config *Config) AddRootFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&config.Token, "token", "", "The OAuth Token to use for requests.")
	cmd.PersistentFlags().StringVar(&config.TokenFile, "token-file", "", "The file containing the OAuth Token to use for requests.")
	cmd.PersistentFlags().IntVar(&config.MinPRNumber, "min-pr-number", 0, "The minimum PR to start with")
	cmd.PersistentFlags().IntVar(&config.MaxPRNumber, "max-pr-number", maxInt, "The maximum PR to start with")
	cmd.PersistentFlags().BoolVar(&config.DryRun, "dry-run", false, "If true, don't actually merge anything")
	cmd.PersistentFlags().BoolVar(&config.useMemoryCache, "use-http-cache", false, "If true, use a client side HTTP cache for API requests.")
	cmd.PersistentFlags().StringVar(&config.Org, "organization", "kubernetes", "The github organization to scan")
	cmd.PersistentFlags().StringVar(&config.Project, "project", "kubernetes", "The github project to scan")
	// Global limit is 5000 Q/Hour, try to only use 4000 to make room for other apps
	cmd.PersistentFlags().Float32Var(&config.RateLimit, "rate-limit", 4000, "Requests per hour we should allow")
	cmd.PersistentFlags().IntVar(&config.RateLimitBurst, "rate-limit-burst", 2000, "Requests we allow to burst over the rate limit")
	cmd.PersistentFlags().AddGoFlagSet(goflag.CommandLine)
}

// PreExecute will initialize the Config. It MUST be run before the config
// may be used to get information from Github
func (config *Config) PreExecute() error {
	if len(config.Org) == 0 {
		glog.Fatalf("--organization is required.")
	}
	if len(config.Project) == 0 {
		glog.Fatalf("--project is required.")
	}

	token := config.Token
	if len(token) == 0 && len(config.TokenFile) != 0 {
		data, err := ioutil.ReadFile(config.TokenFile)
		if err != nil {
			glog.Fatalf("error reading token file: %v", err)
		}
		token = string(data)
	}

	transport := http.DefaultTransport
	if config.useMemoryCache {
		transport = httpcache.NewMemoryCacheTransport()
	}

	// convert from queries per hour to queries per second
	config.RateLimit = config.RateLimit / 3600
	// ignore the configured rate limit if you don't have a token.
	// only get 60 requests per hour!
	if len(token) == 0 {
		glog.Warningf("Ignoring --rate-limit because no token data available")
		config.RateLimit = 0.01
		config.RateLimitBurst = 10
	}
	rateLimitTransport := &rateLimitRoundTripper{
		delegate: transport,
		throttle: util.NewTokenBucketRateLimiter(config.RateLimit, config.RateLimitBurst),
	}

	client := &http.Client{
		Transport: rateLimitTransport,
	}
	if len(token) > 0 {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		client = &http.Client{
			Transport: &oauth2.Transport{
				Base:   rateLimitTransport,
				Source: oauth2.ReuseTokenSource(nil, ts),
			},
		}
	}
	config.client = github.NewClient(client)
	config.analytics.lastAPIReset = time.Now()
	return nil
}

// ResetAPICount will both reset the counters of how many api calls have been
// made but will also print the information from the last run.
func (config *Config) ResetAPICount() {
	config.analytics.Print()
	config.analytics = analytics{}
	config.analytics.lastAPIReset = time.Now()
}

// SetClient should ONLY be used by testing. Normal commands should use PreExecute()
func (config *Config) SetClient(client *github.Client) {
	config.client = client
}

// HasLabel returns if the label `name` is in the array of `labels`
func HasLabel(labels []github.Label, name string) bool {
	for i := range labels {
		label := &labels[i]
		if label.Name != nil && *label.Name == name {
			return true
		}
	}
	return false
}

// HasLabels returns if all of the label `names` are in the array of `labels`
func HasLabels(labels []github.Label, names []string) bool {
	for i := range names {
		if !HasLabel(labels, names[i]) {
			return false
		}
	}
	return true
}

// GetLabelsWithPrefix will return a slice of all label names in `labels` which
// start with given prefix.
func GetLabelsWithPrefix(labels []github.Label, prefix string) []string {
	var ret []string
	for _, label := range labels {
		if label.Name != nil && strings.HasPrefix(*label.Name, prefix) {
			ret = append(ret, *label.Name)
		}
	}
	return ret
}

// AddLabels will add all of the named `labels` to the PR
func (config *Config) AddLabels(prNum int, labels []string) error {
	config.analytics.AddLabels.Call(config)
	if config.DryRun {
		glog.Infof("Would have added labels %v to PR %d but --dry-run is set", labels, prNum)
		return nil
	}
	if _, _, err := config.client.Issues.AddLabelsToIssue(config.Org, config.Project, prNum, labels); err != nil {
		glog.Errorf("Failed to set labels %v for %d: %v", labels, prNum, err)
		return err
	}
	return nil
}

// RemoveLabel will remove the `label` from the PR
func (config *Config) RemoveLabel(prNum int, label string) error {
	config.analytics.RemoveLabels.Call(config)
	if config.DryRun {
		glog.Infof("Would have removed label %q to PR %d but --dry-run is set", label, prNum)
		return nil
	}
	if _, err := config.client.Issues.RemoveLabelForIssue(config.Org, config.Project, prNum, label); err != nil {
		glog.Errorf("Failed to remove %d from issue %d: %v", label, prNum, err)
		return err
	}
	return nil
}

// PRFunction is the type that must be implemented and passed to ForEachPRDo
type PRFunction func(*github.PullRequest, *github.Issue) error

// IssueFunction is the type that must be implemented and passed to ForEachIssueDo
type IssueFunction func(*github.Issue) error

// LastModifiedTime returns the time the last commit was made
// BUG: this should probably return the last time a git push happened or something like that.
func (config *Config) LastModifiedTime(prNum int) (*time.Time, error) {
	config.analytics.ListCommits.Call(config)
	list, _, err := config.client.PullRequests.ListCommits(config.Org, config.Project, prNum, &github.ListOptions{})
	if err != nil {
		return nil, err
	}
	var lastModified *time.Time
	for ix := range list {
		item := list[ix]
		if lastModified == nil || item.Commit.Committer.Date.After(*lastModified) {
			lastModified = item.Commit.Committer.Date
		}
	}
	return lastModified, nil
}

func (config *Config) fetchAllCollaborators() ([]github.User, error) {
	page := 1
	var result []github.User
	for {
		glog.V(4).Infof("Fetching page %d of all users", page)
		config.analytics.ListCollaborators.Call(config)
		listOpts := &github.ListOptions{PerPage: 100, Page: page}
		users, response, err := config.client.Repositories.ListCollaborators(config.Org, config.Project, listOpts)
		if err != nil {
			return nil, err
		}
		result = append(result, users...)
		if response.LastPage == 0 || response.LastPage <= page {
			break
		}
		page++
	}
	return result, nil
}

// UsersWithAccess returns two sets of users. The first set are users with push
// access. The second set is the specific set of user with pull access. If the
// repo is public all users will have pull access, but some with have it
// explicitly
func (config *Config) UsersWithAccess() (pushUsers sets.String, pullUsers sets.String, err error) {
	pushUsers = sets.String{}
	pullUsers = sets.String{}

	users, err := config.fetchAllCollaborators()
	if err != nil {
		glog.Errorf("%v", err)
		return nil, nil, err
	}

	for _, user := range users {
		if user.Permissions == nil || user.Login == nil {
			err := fmt.Errorf("found a user with nil Permissions or Login")
			glog.Errorf("%v", err)
			return nil, nil, err
		}
		login := *user.Login
		perms := *user.Permissions
		if perms["push"] {
			pushUsers.Insert(login)
		} else if perms["pull"] {
			pullUsers.Insert(login)
		}
	}
	return pushUsers, pullUsers, nil
}

// GetAllEventsForPR returns a list of all events for a given pr.
func (config *Config) GetAllEventsForPR(prNum int) ([]github.IssueEvent, error) {
	events := []github.IssueEvent{}
	page := 1
	for {
		config.analytics.ListIssueEvents.Call(config)
		eventPage, response, err := config.client.Issues.ListIssueEvents(config.Org, config.Project, prNum, &github.ListOptions{Page: page})
		if err != nil {
			glog.Errorf("Error getting events for issue: %v", err)
			return nil, err
		}
		events = append(events, eventPage...)
		if response.LastPage == 0 || response.LastPage <= page {
			break
		}
		page++
	}
	return events, nil
}

func computeStatus(combinedStatus *github.CombinedStatus, requiredContexts []string) string {
	states := sets.String{}
	providers := sets.String{}

	if len(requiredContexts) == 0 {
		return *combinedStatus.State
	}

	requires := sets.NewString(requiredContexts...)
	for _, status := range combinedStatus.Statuses {
		if !requires.Has(*status.Context) {
			continue
		}
		states.Insert(*status.State)
		providers.Insert(*status.Context)
	}

	missing := requires.Difference(providers)
	if missing.Len() != 0 {
		glog.V(8).Infof("Failed to find %v in CombinedStatus for %s", missing.List(), combinedStatus.SHA)
		return "incomplete"
	}
	switch {
	case states.Has("pending"):
		return "pending"
	case states.Has("error"):
		return "error"
	case states.Has("failure"):
		return "failure"
	default:
		return "success"
	}
}

// GetStatus gets the current status of a PR.
//    * If any member of the 'requiredContexts' list is missing, it is 'incomplete'
//    * If any is 'pending', the PR is 'pending'
//    * If any is 'error', the PR is in 'error'
//    * If any is 'failure', the PR is 'failure'
//    * Otherwise the PR is 'success'
func (config *Config) GetStatus(pr *github.PullRequest, requiredContexts []string) (string, error) {
	if pr.Head == nil {
		glog.Errorf("pr.Head is nil in GetStatus for PR# %d", *pr.Number)
		return "failure", nil
	}
	combinedStatus, _, err := config.client.Repositories.GetCombinedStatus(config.Org, config.Project, *pr.Head.SHA, &github.ListOptions{})
	config.analytics.GetCombinedStatus.Call(config)
	if err != nil {
		return "failure", err
	}
	return computeStatus(combinedStatus, requiredContexts), nil
}

// IsStatusSuccess makes sure that the combined status for all commits in a PR is 'success'
func (config *Config) IsStatusSuccess(pr *github.PullRequest, requiredContexts []string) bool {
	status, err := config.GetStatus(pr, requiredContexts)
	if err != nil {
		return false
	}
	if status == "success" {
		return true
	}
	return false
}

// WaitForPending will wait for a PR to move into Pending.  This is useful
// because the request to test a PR again is asynchronous with the PR actually
// moving into a pending state
// TODO: add a timeout
func (config *Config) WaitForPending(pr *github.PullRequest) error {
	for {
		status, err := config.GetStatus(pr, []string{})
		if err != nil {
			return err
		}
		if status == "pending" {
			return nil
		}
		glog.V(4).Info("PR is not pending, waiting for 30 seconds")
		time.Sleep(30 * time.Second)
	}
}

// WaitForNotPending will check if the github status is "pending" (CI still running)
// if so it will sleep and try again until all status hooks have complete
func (config *Config) WaitForNotPending(pr *github.PullRequest) error {
	for {
		status, err := config.GetStatus(pr, []string{})
		if err != nil {
			return err
		}
		if status != "pending" {
			return nil
		}
		glog.V(4).Info("PR is pending, waiting for 30 seconds")
		time.Sleep(30 * time.Second)
	}
}

func (config *Config) getCommits(prNum int) ([]github.RepositoryCommit, error) {
	config.analytics.ListCommits.Call(config)
	//TODO: this should handle paging, I believe....
	commits, _, err := config.client.PullRequests.ListCommits(config.Org, config.Project, prNum, &github.ListOptions{})
	if err != nil {
		return nil, err
	}
	return commits, nil
}

// GetFilledCommits returns all of the commits for a given PR
func (config *Config) GetFilledCommits(prNum int) ([]github.RepositoryCommit, error) {
	commits, err := config.getCommits(prNum)
	if err != nil {
		return nil, err
	}
	filledCommits := []github.RepositoryCommit{}
	for _, c := range commits {
		config.analytics.GetCommit.Call(config)
		commit, _, err := config.client.Repositories.GetCommit(config.Org, config.Project, *c.SHA)
		if err != nil {
			glog.Errorf("Can't load commit %s %s %s", config.Org, config.Project, *commit.SHA)
			continue
		}
		filledCommits = append(filledCommits, *commit)
	}
	return filledCommits, nil
}

// GetPR will return a pull request based on the provided number.
// This may be useful if some of the information in the provided
// PR was not filled out when it was retrieved.
func (config *Config) GetPR(prNum int) (*github.PullRequest, error) {
	config.analytics.GetPR.Call(config)
	pr, _, err := config.client.PullRequests.Get(config.Org, config.Project, prNum)
	if err != nil {
		glog.Errorf("Error getting PR# %d: %v", prNum, err)
		return nil, err
	}
	return pr, nil
}

// AssignPR will assign `prNum` to the `owner` where the `owner` is asignee's github login
func (config *Config) AssignPR(prNum int, owner string) error {
	config.analytics.AssignPR.Call(config)
	assignee := &github.IssueRequest{Assignee: &owner}
	if config.DryRun {
		glog.Infof("Would have assigned PR# %d  to %v but --dry-run was set", prNum, owner)
		return nil
	}
	if _, _, err := config.client.Issues.Edit(config.Org, config.Project, prNum, assignee); err != nil {
		glog.Errorf("Error assigning issue# %d to %v: %v", prNum, owner, err)
		return err
	}
	return nil
}

// ClosePR will close the Given PR
func (config *Config) ClosePR(pr *github.PullRequest) error {
	config.analytics.ClosePR.Call(config)
	if config.DryRun {
		glog.Infof("Would have closed PR# %d but --dry-run was set", *pr.Number)
		return nil
	}
	state := "closed"
	pr.State = &state
	if _, _, err := config.client.PullRequests.Edit(config.Org, config.Project, *pr.Number, pr); err != nil {
		glog.Errorf("Failed to close pr %d: %v", *pr.Number, err)
		return err
	}
	return nil
}

// OpenPR will attempt to open the given PR.
// It will attempt to reopen the pr `numTries` before returning an error
// and giving up.
func (config *Config) OpenPR(pr *github.PullRequest, numTries int) error {
	config.analytics.OpenPR.Call(config)
	if config.DryRun {
		glog.Infof("Would have openned PR# %d but --dry-run was set", *pr.Number)
		return nil
	}
	var err error
	state := "open"
	pr.State = &state
	// Try pretty hard to re-open, since it's pretty bad if we accidentally leave a PR closed
	for tries := 0; tries < numTries; tries++ {
		if _, _, err = config.client.PullRequests.Edit(config.Org, config.Project, *pr.Number, pr); err != nil {
			return nil
		}
		glog.Warningf("failed to re-open pr %d: %v", *pr.Number, err)
		time.Sleep(5 * time.Second)
	}
	if err != nil {
		glog.Errorf("failed to re-open pr %d after %d tries, giving up: %v", *pr.Number, numTries, err)
	}
	return err
}

// GetFileContents will return the contents of the `file` in the repo at `sha`
// as a string
func (config *Config) GetFileContents(file, sha string) (string, error) {
	config.analytics.GetContents.Call(config)
	getOpts := &github.RepositoryContentGetOptions{Ref: sha}
	if len(sha) > 0 {
		getOpts.Ref = sha
	}
	output, _, _, err := config.client.Repositories.GetContents(config.Org, config.Project, file, getOpts)
	if err != nil {
		err = fmt.Errorf("unable to get %q at commit %q", file, sha)
		// I'm using .V(2) because .generated docs is still not in the repo...
		glog.V(2).Infof("%v", err)
		return "", err
	}
	if output == nil {
		err = fmt.Errorf("got empty contents for %q at commit %q", file, sha)
		glog.Errorf("%v", err)
		return "", err
	}
	b, err := output.Decode()
	if err != nil {
		glog.Errorf("Unable to decode file contents: %v", err)
		return "", err
	}
	return string(b), nil
}

// MergePR will merge the given PR, duh
// "who" is who is doing the merging, like "submit-queue"
func (config *Config) MergePR(pr *github.PullRequest, who string) error {
	prNum := *pr.Number
	config.analytics.Merge.Call(config)
	if config.DryRun {
		glog.Infof("Would have merged %d but --dry-run is set", prNum)
		return nil
	}
	glog.Infof("Merging PR# %d", prNum)
	mergeBody := "Automatic merge from " + who
	config.WriteComment(prNum, mergeBody)
	_, _, err := config.client.PullRequests.Merge(config.Org, config.Project, prNum, "Auto commit by PR queue bot")

	// The github API https://developer.github.com/v3/pulls/#merge-a-pull-request-merge-button indicates
	// we will only get the bellow error if we provided a particular sha to merge PUT. We aren't doing that
	// so our best guess is that the API also provides this error message when it is recalulating
	// "mergeable". So if we get this error, check "IsPRMergeable()" which should sleep just a bit until
	// github is finished calculating. If my guess is correct, that also means we should be able to
	// then merge this PR, so try again.
	if err != nil && strings.Contains(err.Error(), "branch was modified. Review and try the merge again.") {
		if mergeable, _ := config.IsPRMergeable(pr); mergeable {
			_, _, err = config.client.PullRequests.Merge(config.Org, config.Project, prNum, "Auto commit by PR queue bot")
		}
	}
	if err != nil {
		glog.Errorf("Failed to merge PR: %d: %v", prNum, err)
		return err
	}
	return nil
}

// WriteComment will send the `msg` as a comment to the specified PR
func (config *Config) WriteComment(prNum int, msg string) error {
	config.analytics.CreateComment.Call(config)
	if config.DryRun {
		glog.Infof("Would have commented %q in %d but --dry-run is set", msg, prNum)
		return nil
	}
	glog.Infof("Adding comment: %q to PR %d", msg, prNum)
	if _, _, err := config.client.Issues.CreateComment(config.Org, config.Project, prNum, &github.IssueComment{Body: &msg}); err != nil {
		glog.Errorf("%v", err)
		return err
	}
	return nil
}

// IsPRMergeable will return if the PR is mergeable. It will pause and get the
// PR again if github did not respond the first time. So the hopefully github
// will have a response the second time. If we have no answer twice, we return
// false
func (config *Config) IsPRMergeable(pr *github.PullRequest) (bool, error) {
	if pr.Mergeable == nil {
		var err error
		glog.Infof("Waiting for mergeability on %q %d", *pr.Title, *pr.Number)
		// TODO: determine what a good empirical setting for this is.
		time.Sleep(2 * time.Second)
		pr, err = config.GetPR(*pr.Number)
		if err != nil {
			glog.Errorf("Unable to get PR# %d: %v", *pr.Number, err)
			return false, err
		}
	}
	if pr.Mergeable == nil {
		err := fmt.Errorf("no mergeability information for %q %d, Skipping", *pr.Title, *pr.Number)
		glog.Errorf("%v", err)
		return false, err
	}
	if !*pr.Mergeable {
		return false, nil
	}
	return true, nil

}

// For each Issue in the project that matches:
//   * pr.Number >= minPRNumber
//   * pr.Number <= maxPRNumber
//   * all labels are on the PR
// Run the specified function
func (config *Config) forEachIssueDo(labels []string, fn IssueFunction) error {
	page := 1
	for {
		glog.V(4).Infof("Fetching page %d of issues", page)
		config.analytics.ListIssues.Call(config)
		listOpts := &github.IssueListByRepoOptions{
			Sort:        "created",
			Labels:      labels,
			State:       "open",
			ListOptions: github.ListOptions{PerPage: 20, Page: page},
		}
		issues, response, err := config.client.Issues.ListByRepo(config.Org, config.Project, listOpts)
		if err != nil {
			return err
		}
		for i := range issues {
			issue := &issues[i]
			if issue.Number == nil {
				glog.Infof("Skipping issue with no number, very strange")
				continue
			}
			if issue.User == nil || issue.User.Login == nil {
				glog.V(2).Infof("Skipping PR %d with no user info %#v.", *issue.Number, issue.User)
				continue
			}
			if *issue.Number < config.MinPRNumber {
				glog.V(6).Infof("Dropping %d < %d", *issue.Number, config.MinPRNumber)
				continue
			}
			if *issue.Number > config.MaxPRNumber {
				glog.V(6).Infof("Dropping %d > %d", *issue.Number, config.MaxPRNumber)
				continue
			}
			glog.V(8).Infof("Issue %d labels: %v isPR: %v", *issue.Number, issue.Labels, issue.PullRequestLinks == nil)
			glog.V(8).Infof("%v", issue.Labels)
			if err := fn(issue); err != nil {
				return err
			}
		}
		if response.LastPage == 0 || response.LastPage <= page {
			break
		}
		page++
	}
	return nil
}

// ForEachIssueDo will call the provided IssueFunction once for each issue
// which has the labels provided in `labels`
func (config *Config) ForEachIssueDo(labels []string, fn IssueFunction) error {
	handleIssue := func(issue *github.Issue) error {
		if issue.PullRequestLinks != nil {
			return nil
		}
		glog.V(2).Infof("----==== %d ====----", *issue.Number)

		return fn(issue)
	}
	return config.forEachIssueDo(labels, handleIssue)
}

// ForEachPRDo will call the provided PRFunction once for each issue
// which has the labels provided in `labels`
func (config *Config) ForEachPRDo(labels []string, fn PRFunction) error {
	handlePR := func(issue *github.Issue) error {
		if issue.PullRequestLinks == nil {
			return nil
		}
		pr, err := config.GetPR(*issue.Number)
		if err != nil {
			return err
		}
		if pr.Merged != nil && *pr.Merged {
			glog.V(3).Infof("PR %d was merged, may want to reduce the PerPage so this happens less often", *issue.Number)
			return nil
		}
		glog.V(2).Infof("----==== %d ====----", *issue.Number)

		if pr.Mergeable == nil {
			glog.V(2).Infof("Waiting for mergeability on %q %d", *pr.Title, *pr.Number)
			time.Sleep(2 * time.Second)
			pr, err = config.GetPR(*pr.Number)
			if err != nil {
				return err
			}
			if pr.Mergeable == nil {
				glog.Infof("No mergeability for PR %d after pause. Maybe increase pause time?", *pr.Number)
			}
		}
		return fn(pr, issue)
	}
	return config.forEachIssueDo(labels, handlePR)
}
