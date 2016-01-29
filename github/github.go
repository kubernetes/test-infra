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
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"k8s.io/kubernetes/pkg/util/sets"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
	"github.com/gregjones/httpcache"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
)

const (
	// stolen from https://groups.google.com/forum/#!msg/golang-nuts/a9PitPAHSSU/ziQw1-QHw3EJ
	maxInt          = int(^uint(0) >> 1)
	tokenLimit      = 500 // How many github api tokens to not use
	asyncTokenLimit = 400 // How many github api tokens to not use for 'asyc' calls

	headerRateRemaining = "X-RateLimit-Remaining"
	headerRateReset     = "X-RateLimit-Reset"
)

type callLimitRoundTripper struct {
	sync.Mutex
	delegate  http.RoundTripper
	remaining int
	resetTime time.Time
}

func (c *callLimitRoundTripper) getTokenExcept(remaining int) {
	c.Lock()
	if c.remaining > remaining {
		c.remaining--
		c.Unlock()
		return
	}
	resetTime := c.resetTime
	c.Unlock()
	sleepTime := resetTime.Sub(time.Now()) + (1 * time.Minute)
	if sleepTime > 0 {
		glog.Errorf("*****************")
		glog.Errorf("Ran out of github API tokens. Sleeping for %v minutes", sleepTime.Minutes())
		glog.Errorf("*****************")
	}
	// negative duration is fine, it means we are past the github api reset and we won't sleep
	time.Sleep(sleepTime)
}

func (c *callLimitRoundTripper) getToken() {
	c.getTokenExcept(tokenLimit)
}

func (c *callLimitRoundTripper) getAsyncToken() {
	c.getTokenExcept(asyncTokenLimit)
}

func (c *callLimitRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if c.delegate == nil {
		c.delegate = http.DefaultTransport
	}
	// TODO Be smart about which should use getToken and which should use getAsyncToken()
	c.getToken()
	resp, err := c.delegate.RoundTrip(req)
	c.Lock()
	defer c.Unlock()
	if resp != nil {
		if remaining := resp.Header.Get(headerRateRemaining); remaining != "" {
			c.remaining, _ = strconv.Atoi(remaining)
		}
		if reset := resp.Header.Get(headerRateReset); reset != "" {
			if v, _ := strconv.ParseInt(reset, 10, 64); v != 0 {
				c.resetTime = time.Unix(v, 0)
			}
		}
	}
	return resp, err
}

// By default github responds to PR requests with:
//    Cache-Control:[private, max-age=60, s-maxage=60]
// Which means the httpcache would not consider anything stale for 60 seconds.
// However, when we re-check 'PR.mergeable' we need to skip the cache.
// I considered checking the req.URL.Path and only setting max-age=0 when
// getting a PR or getting the CombinedStatus, as these are the times we need
// a super fresh copy. But since all of the other calls are only going to be made
// once per poll loop the 60 second github freshness doesn't matter. So I can't
// think of a reason not to just keep this simple and always set max-age=0 on
// every request.
type zeroCacheRoundTripper struct {
	delegate http.RoundTripper
}

func (r *zeroCacheRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Cache-Control", "max-age=0")
	delegate := r.delegate
	if delegate == nil {
		delegate = http.DefaultTransport
	}
	return delegate.RoundTrip(req)
}

// Config is how we are configured to talk to github and provides access
// methods for doing so.
type Config struct {
	client   *github.Client
	apiLimit *callLimitRoundTripper
	Org      string
	Project  string

	Token     string
	TokenFile string

	MinPRNumber int
	MaxPRNumber int

	// If true, don't make any mutating API calls
	DryRun bool

	// Defaults to 30 seconds.
	PendingWaitTime *time.Duration

	useMemoryCache bool

	// When we clear analytics we store the last values here
	lastAnalytics analytics
	analytics     analytics
}

type analytic struct {
	Count       int
	CachedCount int
}

func (a *analytic) Call(config *Config, response *github.Response) {
	if response != nil && response.Response.Header.Get(httpcache.XFromCache) != "" {
		config.analytics.cachedAPICount++
		a.CachedCount++
	}
	config.analytics.apiCount++
	a.Count++
}

type analytics struct {
	lastAPIReset       time.Time
	nextAnalyticUpdate time.Time // when we expect the next update
	apiCount           int       // number of times we called a github API
	cachedAPICount     int       // how many api calls were answered by the local cache
	apiPerSec          float64

	AddLabels         analytic
	RemoveLabels      analytic
	ListCollaborators analytic
	GetIssue          analytic
	ListIssues        analytic
	ListIssueEvents   analytic
	ListCommits       analytic
	GetCommit         analytic
	GetCombinedStatus analytic
	SetStatus         analytic
	GetPR             analytic
	AssignPR          analytic
	ClosePR           analytic
	OpenPR            analytic
	GetContents       analytic
	ListComments      analytic
	CreateComment     analytic
	DeleteComment     analytic
	Merge             analytic
	GetUser           analytic
}

func (a analytics) print() {
	glog.Infof("Made %d API calls since the last Reset %f calls/sec", a.apiCount, a.apiPerSec)

	buf := new(bytes.Buffer)
	w := new(tabwriter.Writer)
	w.Init(buf, 0, 0, 1, ' ', tabwriter.AlignRight)
	fmt.Fprintf(w, "AddLabels\t%d\t\n", a.AddLabels.Count)
	fmt.Fprintf(w, "RemoveLabels\t%d\t\n", a.RemoveLabels.Count)
	fmt.Fprintf(w, "ListCollaborators\t%d\t\n", a.ListCollaborators.Count)
	fmt.Fprintf(w, "GetIssue\t%d\t\n", a.GetIssue.Count)
	fmt.Fprintf(w, "ListIssues\t%d\t\n", a.ListIssues.Count)
	fmt.Fprintf(w, "ListIssueEvents\t%d\t\n", a.ListIssueEvents.Count)
	fmt.Fprintf(w, "ListCommits\t%d\t\n", a.ListCommits.Count)
	fmt.Fprintf(w, "GetCommit\t%d\t\n", a.GetCommit.Count)
	fmt.Fprintf(w, "GetCombinedStatus\t%d\t\n", a.GetCombinedStatus.Count)
	fmt.Fprintf(w, "SetStatus\t%d\t\n", a.SetStatus.Count)
	fmt.Fprintf(w, "GetPR\t%d\t\n", a.GetPR.Count)
	fmt.Fprintf(w, "AssignPR\t%d\t\n", a.AssignPR.Count)
	fmt.Fprintf(w, "ClosePR\t%d\t\n", a.ClosePR.Count)
	fmt.Fprintf(w, "OpenPR\t%d\t\n", a.OpenPR.Count)
	fmt.Fprintf(w, "GetContents\t%d\t\n", a.GetContents.Count)
	fmt.Fprintf(w, "ListComments\t%d\t\n", a.ListComments.Count)
	fmt.Fprintf(w, "CreateComment\t%d\t\n", a.CreateComment.Count)
	fmt.Fprintf(w, "DeleteComment\t%d\t\n", a.DeleteComment.Count)
	fmt.Fprintf(w, "Merge\t%d\t\n", a.Merge.Count)
	fmt.Fprintf(w, "GetUser\t%d\t\n", a.GetUser.Count)
	w.Flush()
	glog.V(2).Infof("\n%v", buf)
}

// MungeObject is the object that mungers deal with. It is a combination of
// different github API objects.
type MungeObject struct {
	config  *Config
	Issue   *github.Issue
	pr      *github.PullRequest
	commits []github.RepositoryCommit
	events  []github.IssueEvent
}

// DebugStats is a structure that tells information about how we have interacted
// with github
type DebugStats struct {
	Analytics      analytics
	APIPerSec      float64
	APICount       int
	CachedAPICount int
	NextLoopTime   time.Time
	LimitRemaining int
	LimitResetTime time.Time
}

// TestObject should NEVER be used outside of _test.go code. It creates a
// MungeObject with the given fields. Normally these should be filled in lazily
// as needed
func TestObject(config *Config, issue *github.Issue, pr *github.PullRequest, commits []github.RepositoryCommit, events []github.IssueEvent) *MungeObject {
	return &MungeObject{
		config:  config,
		Issue:   issue,
		pr:      pr,
		commits: commits,
		events:  events,
	}
}

// AddRootFlags will add all of the flags needed for the github config to the cobra command
func (config *Config) AddRootFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&config.Token, "token", "", "The OAuth Token to use for requests.")
	cmd.PersistentFlags().StringVar(&config.TokenFile, "token-file", "", "The file containing the OAuth Token to use for requests.")
	cmd.PersistentFlags().IntVar(&config.MinPRNumber, "min-pr-number", 0, "The minimum PR to start with")
	cmd.PersistentFlags().IntVar(&config.MaxPRNumber, "max-pr-number", maxInt, "The maximum PR to start with")
	cmd.PersistentFlags().BoolVar(&config.DryRun, "dry-run", false, "If true, don't actually merge anything")
	cmd.PersistentFlags().BoolVar(&config.useMemoryCache, "use-http-cache", true, "If true, use a client side HTTP cache for API requests.")
	cmd.PersistentFlags().StringVar(&config.Org, "organization", "kubernetes", "The github organization to scan")
	cmd.PersistentFlags().StringVar(&config.Project, "project", "kubernetes", "The github project to scan")
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
		token = strings.TrimSpace(string(data))
	}

	// We need to get our Transport/RoundTripper in order based on arguments
	//    oauth2 Transport // if we have an auth token
	//    zeroCacheRoundTripper // if we are using the cache want faster timeouts
	//    webCacheRoundTripper // if we are using the cache
	//    callLimitRoundTripper ** always
	//    [http.DefaultTransport] ** always implicit

	var transport http.RoundTripper

	callLimitTransport := &callLimitRoundTripper{
		remaining: tokenLimit + 500, // put in 500 so we at least have a couple to check our real limits
		resetTime: time.Now().Add(1 * time.Minute),
	}
	config.apiLimit = callLimitTransport
	transport = callLimitTransport

	if config.useMemoryCache {
		t := httpcache.NewMemoryCacheTransport()
		t.Transport = transport

		zeroCacheTransport := &zeroCacheRoundTripper{
			delegate: t,
		}

		transport = zeroCacheTransport
	}

	if len(token) > 0 {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		transport = &oauth2.Transport{
			Base:   transport,
			Source: oauth2.ReuseTokenSource(nil, ts),
		}
	}

	client := &http.Client{
		Transport: transport,
	}
	config.client = github.NewClient(client)
	config.ResetAPICount()
	return nil
}

// GetDebugStats returns information about the bot iself. Things like how many
// API calls has it made, how many of each type, etc.
func (config *Config) GetDebugStats() DebugStats {
	d := DebugStats{
		Analytics:      config.lastAnalytics,
		APIPerSec:      config.lastAnalytics.apiPerSec,
		APICount:       config.lastAnalytics.apiCount,
		CachedAPICount: config.lastAnalytics.cachedAPICount,
		NextLoopTime:   config.lastAnalytics.nextAnalyticUpdate,
	}
	config.apiLimit.Lock()
	defer config.apiLimit.Unlock()
	d.LimitRemaining = config.apiLimit.remaining
	d.LimitResetTime = config.apiLimit.resetTime
	return d
}

// NextExpectedUpdate will set the debug information concerning when the
// mungers are likely to run again.
func (config *Config) NextExpectedUpdate(t time.Time) {
	config.analytics.nextAnalyticUpdate = t
}

// ResetAPICount will both reset the counters of how many api calls have been
// made but will also print the information from the last run.
func (config *Config) ResetAPICount() {
	since := time.Since(config.analytics.lastAPIReset)
	config.analytics.apiPerSec = float64(config.analytics.apiCount) / since.Seconds()
	config.lastAnalytics = config.analytics
	config.analytics.print()

	config.analytics = analytics{}
	config.analytics.lastAPIReset = time.Now()
}

// SetClient should ONLY be used by testing. Normal commands should use PreExecute()
func (config *Config) SetClient(client *github.Client) {
	config.client = client
}

// GetObject will return an object (with only the issue filled in)
func (config *Config) GetObject(num int) (*MungeObject, error) {
	issue, resp, err := config.client.Issues.Get(config.Org, config.Project, num)
	config.analytics.GetIssue.Call(config, resp)
	if err != nil {
		glog.Errorf("GetObject: %v", err)
		return nil, err
	}
	obj := &MungeObject{
		config: config,
		Issue:  issue,
	}
	return obj, nil
}

// LastModifiedTime returns the time the last commit was made
// BUG: this should probably return the last time a git push happened or something like that.
func (obj *MungeObject) LastModifiedTime() *time.Time {
	var lastModified *time.Time
	commits, err := obj.GetCommits()
	if err != nil {
		return lastModified
	}
	for _, commit := range commits {
		if commit.Commit == nil || commit.Commit.Committer == nil || commit.Commit.Committer.Date == nil {
			glog.Errorf("PR %d: Found invalid RepositoryCommit: %v", *obj.Issue.Number, commit)
			continue
		}
		if lastModified == nil || commit.Commit.Committer.Date.After(*lastModified) {
			lastModified = commit.Commit.Committer.Date
		}
	}
	return lastModified
}

// labelEvent returns the most recent event where the given label was added to an issue
func (obj *MungeObject) labelEvent(label string) *github.IssueEvent {
	var labelTime *time.Time
	var out github.IssueEvent
	events, err := obj.GetEvents()
	if err != nil {
		return &out
	}
	for _, event := range events {
		if *event.Event == "labeled" && *event.Label.Name == label {
			if labelTime == nil || event.CreatedAt.After(*labelTime) {
				labelTime = event.CreatedAt
				out = event
			}
		}
	}
	return &out
}

// LabelTime returns the last time the request label was added to an issue.
// If the label was never added you will get the 0 time.
func (obj *MungeObject) LabelTime(label string) *time.Time {
	event := obj.labelEvent(label)
	if event == nil {
		return nil
	}
	return event.CreatedAt
}

// LabelCreator returns the login name of the user who (last) created the given label
func (obj *MungeObject) LabelCreator(label string) string {
	event := obj.labelEvent(label)
	if event == nil {
		return ""
	}
	return *event.Actor.Login
}

// HasLabel returns if the label `name` is in the array of `labels`
func (obj *MungeObject) HasLabel(name string) bool {
	labels := obj.Issue.Labels
	for i := range labels {
		label := &labels[i]
		if label.Name != nil && *label.Name == name {
			return true
		}
	}
	return false
}

// HasLabels returns if all of the label `names` are in the array of `labels`
func (obj *MungeObject) HasLabels(names []string) bool {
	for i := range names {
		if !obj.HasLabel(names[i]) {
			return false
		}
	}
	return true
}

// LabelSet returns the name of all of he labels applied to the object as a
// kubernetes string set.
func (obj *MungeObject) LabelSet() sets.String {
	out := sets.NewString()
	for _, label := range obj.Issue.Labels {
		out.Insert(*label.Name)
	}
	return out
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
func (obj *MungeObject) AddLabels(labels []string) error {
	config := obj.config
	prNum := *obj.Issue.Number
	config.analytics.AddLabels.Call(config, nil)
	glog.Infof("Adding labels %v to PR %d", labels, prNum)
	if config.DryRun {
		return nil
	}
	for _, l := range labels {
		label := github.Label{
			Name: &l,
		}
		obj.Issue.Labels = append(obj.Issue.Labels, label)
	}
	if _, _, err := config.client.Issues.AddLabelsToIssue(config.Org, config.Project, prNum, labels); err != nil {
		glog.Errorf("Failed to set labels %v for %d: %v", labels, prNum, err)
		return err
	}
	return nil
}

// RemoveLabel will remove the `label` from the PR
func (obj *MungeObject) RemoveLabel(label string) error {
	config := obj.config
	prNum := *obj.Issue.Number

	which := -1
	for i, l := range obj.Issue.Labels {
		if l.Name != nil && *l.Name == label {
			which = i
			break
		}
	}
	if which != -1 {
		obj.Issue.Labels = append(obj.Issue.Labels[:which], obj.Issue.Labels[which+1:]...)
	}

	config.analytics.RemoveLabels.Call(config, nil)
	glog.Infof("Removing label %q to PR %d", label, prNum)
	if config.DryRun {
		return nil
	}
	if _, err := config.client.Issues.RemoveLabelForIssue(config.Org, config.Project, prNum, label); err != nil {
		glog.Errorf("Failed to remove %v from issue %d: %v", label, prNum, err)
		return err
	}
	return nil
}

// Priority returns the priority an issue was labeled with.
// The labels must take the form 'priority/P?[0-9]+'
// or math.MaxInt32 if unset
func (obj *MungeObject) Priority() int {
	priority := math.MaxInt32
	priorityLabels := GetLabelsWithPrefix(obj.Issue.Labels, "priority/")
	for _, label := range priorityLabels {
		label = strings.TrimPrefix(label, "priority/")
		label = strings.TrimPrefix(label, "P")
		prio, err := strconv.Atoi(label)
		if err != nil {
			continue
		}
		if prio < priority {
			priority = prio
		}
	}
	return priority
}

// MungeFunction is the type that must be implemented and passed to ForEachIssueDo
type MungeFunction func(*MungeObject) error

func (config *Config) fetchAllCollaborators() ([]github.User, error) {
	page := 1
	var result []github.User
	for {
		glog.V(4).Infof("Fetching page %d of all users", page)
		listOpts := &github.ListOptions{PerPage: 100, Page: page}
		users, response, err := config.client.Repositories.ListCollaborators(config.Org, config.Project, listOpts)
		if err != nil {
			return nil, err
		}
		config.analytics.ListCollaborators.Call(config, response)
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
func (config *Config) UsersWithAccess() ([]github.User, []github.User, error) {
	pushUsers := []github.User{}
	pullUsers := []github.User{}

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
		perms := *user.Permissions
		if perms["push"] {
			pushUsers = append(pushUsers, user)
		} else if perms["pull"] {
			pullUsers = append(pullUsers, user)
		}
	}
	return pushUsers, pullUsers, nil
}

// GetUser will return information about the github user with the given login name
func (config *Config) GetUser(login string) (*github.User, error) {
	user, response, err := config.client.Users.Get(login)
	config.analytics.GetUser.Call(config, response)
	return user, err
}

// IsPR returns if the obj is a PR or an Issue.
func (obj *MungeObject) IsPR() bool {
	if obj.Issue.PullRequestLinks == nil {
		return false
	}
	return true
}

// GetEvents returns a list of all events for a given pr.
func (obj *MungeObject) GetEvents() ([]github.IssueEvent, error) {
	config := obj.config
	prNum := *obj.Issue.Number
	events := []github.IssueEvent{}
	page := 1
	for {
		eventPage, response, err := config.client.Issues.ListIssueEvents(config.Org, config.Project, prNum, &github.ListOptions{PerPage: 100, Page: page})
		config.analytics.ListIssueEvents.Call(config, response)
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
	obj.events = events
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
		glog.V(8).Infof("Failed to find %v in CombinedStatus for %s", missing.List(), *combinedStatus.SHA)
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

func (obj *MungeObject) getCombinedStatus() (status *github.CombinedStatus) {
	config := obj.config
	pr, err := obj.GetPR()
	if err != nil {
		return nil
	}
	if pr.Head == nil {
		glog.Errorf("pr.Head is nil in getCombinedStatus for PR# %d", *obj.Issue.Number)
		return nil
	}
	// TODO If we have more than 100 statuses we need to deal with paging.
	combinedStatus, response, err := config.client.Repositories.GetCombinedStatus(config.Org, config.Project, *pr.Head.SHA, &github.ListOptions{})
	config.analytics.GetCombinedStatus.Call(config, response)
	if err != nil {
		glog.Errorf("Failed to get combined status: %v", err)
		return nil
	}
	return combinedStatus
}

// SetStatus allowes you to set the Github Status
func (obj *MungeObject) SetStatus(state, url, description, context string) error {
	config := obj.config
	status := &github.RepoStatus{
		State:       &state,
		TargetURL:   &url,
		Description: &description,
		Context:     &context,
	}
	pr, err := obj.GetPR()
	if err != nil {
		return err
	}
	ref := *pr.Head.SHA
	glog.Infof("PR %d setting %q Github status to %q", *obj.Issue.Number, context, description)
	config.analytics.SetStatus.Call(config, nil)
	if config.DryRun {
		return nil
	}
	_, _, err = config.client.Repositories.CreateStatus(config.Org, config.Project, ref, status)
	if err != nil {
		glog.Errorf("Unable to set status. PR %d Ref: %q: %v", *obj.Issue.Number, ref, err)
	}
	return err
}

// GetStatus returns the actual requested status, or nil if not found
func (obj *MungeObject) GetStatus(context string) *github.RepoStatus {
	combinedStatus := obj.getCombinedStatus()
	if combinedStatus == nil {
		return nil
	}
	for _, status := range combinedStatus.Statuses {
		if *status.Context == context {
			return &status
		}
	}
	return nil
}

// GetStatusState gets the current status of a PR.
//    * If any member of the 'requiredContexts' list is missing, it is 'incomplete'
//    * If any is 'pending', the PR is 'pending'
//    * If any is 'error', the PR is in 'error'
//    * If any is 'failure', the PR is 'failure'
//    * Otherwise the PR is 'success'
func (obj *MungeObject) GetStatusState(requiredContexts []string) string {
	combinedStatus := obj.getCombinedStatus()
	if combinedStatus == nil {
		return "failure"
	}
	return computeStatus(combinedStatus, requiredContexts)
}

// IsStatusSuccess makes sure that the combined status for all commits in a PR is 'success'
func (obj *MungeObject) IsStatusSuccess(requiredContexts []string) bool {
	status := obj.GetStatusState(requiredContexts)
	if status == "success" {
		return true
	}
	return false
}

// GetStatusTime returns when the status was set
func (obj *MungeObject) GetStatusTime(context string) *time.Time {
	status := obj.GetStatus(context)
	if status == nil {
		return nil
	}
	if status.UpdatedAt != nil {
		return status.UpdatedAt
	}
	return status.CreatedAt
}

// Sleep for the given amount of time and then write to the channel
func timeout(sleepTime time.Duration, c chan bool) {
	time.Sleep(sleepTime)
	c <- true
}

func (obj *MungeObject) doWaitStatus(pending bool, requiredContexts []string, c chan error) {
	config := obj.config
	for {
		status := obj.GetStatusState(requiredContexts)
		var done bool
		if pending {
			done = (status == "pending")
		} else {
			done = (status != "pending")
		}
		if done {
			c <- nil
			return
		}
		if config.DryRun {
			glog.V(4).Infof("PR# %d is not pending, would wait 30 seconds, but --dry-run was set", *obj.Issue.Number)
			c <- nil
			return
		}
		sleepTime := 30 * time.Second
		// If the time was explicitly set, use that instead
		if config.PendingWaitTime != nil {
			sleepTime = *config.PendingWaitTime
		}
		if pending {
			glog.V(4).Infof("PR# %d is not pending, waiting for %f seconds", *obj.Issue.Number, sleepTime.Seconds())
		} else {
			glog.V(4).Infof("PR# %d is pending, waiting for %f seconds", *obj.Issue.Number, sleepTime.Seconds())
		}
		time.Sleep(sleepTime)
	}
}

// WaitForPending will wait for a PR to move into Pending.  This is useful
// because the request to test a PR again is asynchronous with the PR actually
// moving into a pending state
func (obj *MungeObject) WaitForPending(requiredContexts []string) error {
	timeoutChan := make(chan bool, 1)
	done := make(chan error, 1)
	// Wait 45 minutes for the github e2e test to start
	go timeout(45*time.Minute, timeoutChan)
	go obj.doWaitStatus(true, requiredContexts, done)
	select {
	case err := <-done:
		return err
	case <-timeoutChan:
		return fmt.Errorf("PR# %d timed out waiting to go \"pending\"", *obj.Issue.Number)
	}
}

// WaitForNotPending will check if the github status is "pending" (CI still running)
// if so it will sleep and try again until all required status hooks have complete
func (obj *MungeObject) WaitForNotPending(requiredContexts []string) error {
	timeoutChan := make(chan bool, 1)
	done := make(chan error, 1)
	// Wait and hour for the github e2e test to finish
	go timeout(60*time.Minute, timeoutChan)
	go obj.doWaitStatus(false, requiredContexts, done)
	select {
	case err := <-done:
		return err
	case <-timeoutChan:
		return fmt.Errorf("PR# %d timed out waiting to go \"not pending\"", *obj.Issue.Number)
	}
}

// GetCommits returns all of the commits for a given PR
func (obj *MungeObject) GetCommits() ([]github.RepositoryCommit, error) {
	if obj.commits != nil {
		return obj.commits, nil
	}
	config := obj.config
	commits := []github.RepositoryCommit{}
	page := 0
	for {
		commitsPage, response, err := config.client.PullRequests.ListCommits(config.Org, config.Project, *obj.Issue.Number, &github.ListOptions{PerPage: 100, Page: page})
		config.analytics.ListCommits.Call(config, response)
		if err != nil {
			glog.Errorf("Error commits for PR %d: %v", *obj.Issue.Number, err)
			return nil, err
		}
		commits = append(commits, commitsPage...)
		if response.LastPage == 0 || response.LastPage <= page {
			break
		}
		page++
	}

	filledCommits := []github.RepositoryCommit{}
	for _, c := range commits {
		if c.SHA == nil {
			glog.Errorf("Invalid Repository Commit: %v", c)
			continue
		}
		commit, response, err := config.client.Repositories.GetCommit(config.Org, config.Project, *c.SHA)
		config.analytics.GetCommit.Call(config, response)
		if err != nil {
			glog.Errorf("Can't load commit %s %s %s: %v", config.Org, config.Project, *c.SHA, err)
			continue
		}
		filledCommits = append(filledCommits, *commit)
	}
	obj.commits = filledCommits
	return filledCommits, nil
}

// RefreshPR will get the PR again, in case anything changed since last time
func (obj *MungeObject) RefreshPR() (*github.PullRequest, error) {
	config := obj.config
	issueNum := *obj.Issue.Number
	pr, response, err := config.client.PullRequests.Get(config.Org, config.Project, issueNum)
	config.analytics.GetPR.Call(config, response)
	if err != nil {
		glog.Errorf("Error getting PR# %d: %v", issueNum, err)
		return nil, err
	}
	obj.pr = pr
	return pr, nil
}

// GetPR will update the PR in the object.
func (obj *MungeObject) GetPR() (*github.PullRequest, error) {
	if obj.pr != nil {
		return obj.pr, nil
	}
	if !obj.IsPR() {
		return nil, fmt.Errorf("Issue: %d is not a PR", *obj.Issue.Number)
	}
	return obj.RefreshPR()
}

// AssignPR will assign `prNum` to the `owner` where the `owner` is asignee's github login
func (obj *MungeObject) AssignPR(owner string) error {
	config := obj.config
	prNum := *obj.Issue.Number
	assignee := &github.IssueRequest{Assignee: &owner}
	config.analytics.AssignPR.Call(config, nil)
	glog.Infof("Assigning PR# %d  to %v", prNum, owner)
	if config.DryRun {
		return nil
	}
	if _, _, err := config.client.Issues.Edit(config.Org, config.Project, prNum, assignee); err != nil {
		glog.Errorf("Error assigning issue# %d to %v: %v", prNum, owner, err)
		return err
	}
	return nil
}

// ClosePR will close the Given PR
func (obj *MungeObject) ClosePR() error {
	config := obj.config
	pr, err := obj.GetPR()
	if err != nil {
		return err
	}
	config.analytics.ClosePR.Call(config, nil)
	glog.Infof("Closing PR# %d", *pr.Number)
	if config.DryRun {
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
func (obj *MungeObject) OpenPR(numTries int) error {
	config := obj.config
	pr, err := obj.GetPR()
	if err != nil {
		return err
	}
	config.analytics.OpenPR.Call(config, nil)
	glog.Infof("Opening PR# %d", *pr.Number)
	if config.DryRun {
		return nil
	}
	state := "open"
	pr.State = &state
	// Try pretty hard to re-open, since it's pretty bad if we accidentally leave a PR closed
	for tries := 0; tries < numTries; tries++ {
		if _, _, err = config.client.PullRequests.Edit(config.Org, config.Project, *pr.Number, pr); err == nil {
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
func (obj *MungeObject) GetFileContents(file, sha string) (string, error) {
	config := obj.config
	getOpts := &github.RepositoryContentGetOptions{Ref: sha}
	if len(sha) > 0 {
		getOpts.Ref = sha
	}
	output, _, response, err := config.client.Repositories.GetContents(config.Org, config.Project, file, getOpts)
	config.analytics.GetContents.Call(config, response)
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
func (obj *MungeObject) MergePR(who string) error {
	config := obj.config
	prNum := *obj.Issue.Number
	config.analytics.Merge.Call(config, nil)
	glog.Infof("Merging PR# %d", prNum)
	if config.DryRun {
		return nil
	}
	mergeBody := "Automatic merge from " + who
	obj.WriteComment(mergeBody)

	_, _, err := config.client.PullRequests.Merge(config.Org, config.Project, prNum, "Auto commit by PR queue bot")

	// The github API https://developer.github.com/v3/pulls/#merge-a-pull-request-merge-button indicates
	// we will only get the bellow error if we provided a particular sha to merge PUT. We aren't doing that
	// so our best guess is that the API also provides this error message when it is recalulating
	// "mergeable". So if we get this error, check "IsPRMergeable()" which should sleep just a bit until
	// github is finished calculating. If my guess is correct, that also means we should be able to
	// then merge this PR, so try again.
	if err != nil && strings.Contains(err.Error(), "branch was modified. Review and try the merge again.") {
		if mergeable, _ := obj.IsMergeable(); mergeable {
			_, _, err = config.client.PullRequests.Merge(config.Org, config.Project, prNum, "Auto commit by PR queue bot")
		}
	}
	if err != nil {
		glog.Errorf("Failed to merge PR: %d: %v", prNum, err)
		return err
	}
	return nil
}

// ListComments returns all comments for the issue/PR in question
func (obj *MungeObject) ListComments(number int) ([]github.IssueComment, error) {
	config := obj.config
	issueNum := *obj.Issue.Number
	allComments := []github.IssueComment{}

	listOpts := &github.IssueListCommentsOptions{}

	page := 1
	for {
		listOpts.ListOptions.Page = page
		glog.V(8).Infof("Fetching page %d of comments for issue %d", page, issueNum)
		comments, response, err := obj.config.client.Issues.ListComments(config.Org, config.Project, issueNum, listOpts)
		config.analytics.ListComments.Call(config, response)
		if err != nil {
			return nil, err
		}
		allComments = append(allComments, comments...)
		if response.LastPage == 0 || response.LastPage <= page {
			break
		}
		page++
	}
	return allComments, nil
}

// WriteComment will send the `msg` as a comment to the specified PR
func (obj *MungeObject) WriteComment(msg string) error {
	config := obj.config
	prNum := *obj.Issue.Number
	config.analytics.CreateComment.Call(config, nil)
	glog.Infof("Commenting %q in %d", msg, prNum)
	if config.DryRun {
		return nil
	}
	if _, _, err := config.client.Issues.CreateComment(config.Org, config.Project, prNum, &github.IssueComment{Body: &msg}); err != nil {
		glog.Errorf("%v", err)
		return err
	}
	return nil
}

// DeleteComment will remove the specified comment
func (obj *MungeObject) DeleteComment(comment *github.IssueComment) error {
	config := obj.config
	prNum := *obj.Issue.Number
	config.analytics.DeleteComment.Call(config, nil)
	if comment.ID == nil {
		err := fmt.Errorf("Found a comment with nil id for Issue %d", prNum)
		glog.Errorf("Found a comment with nil id for Issue %d", prNum)
		return err
	}
	glog.Infof("Removing comment %d from Issue %d", *comment.ID, prNum)
	if config.DryRun {
		return nil
	}
	if _, err := config.client.Issues.DeleteComment(config.Org, config.Project, *comment.ID); err != nil {
		glog.Errorf("Error removing comment: %v", err)
		return err
	}
	return nil
}

// IsMergeable will return if the PR is mergeable. It will pause and get the
// PR again if github did not respond the first time. So the hopefully github
// will have a response the second time. If we have no answer twice, we return
// false
func (obj *MungeObject) IsMergeable() (bool, error) {
	if !obj.IsPR() {
		return false, nil
	}
	pr, err := obj.GetPR()
	if err != nil {
		return false, err
	}
	prNum := *pr.Number
	if pr.Mergeable == nil {
		glog.V(4).Infof("Waiting for mergeability on %q %d", *pr.Title, *pr.Number)
		// TODO: determine what a good empirical setting for this is.
		time.Sleep(2 * time.Second)
		pr, err = obj.RefreshPR()
		if err != nil {
			glog.Errorf("Unable to get PR# %d: %v", prNum, err)
			return false, err
		}
	}
	if pr.Mergeable == nil {
		err := fmt.Errorf("no mergeability information for %q %d, Skipping", *pr.Title, *pr.Number)
		glog.Errorf("%v", err)
		return false, err
	}
	return *pr.Mergeable, nil
}

// IsMerged returns if the issue in question was already merged
func (obj *MungeObject) IsMerged() (bool, error) {
	if !obj.IsPR() {
		return false, fmt.Errorf("Issue: %d is not a PR and is thus 'merged' is indeterminate", *obj.Issue.Number)
	}
	pr, err := obj.GetPR()
	if err != nil {
		return false, err
	}
	if pr.Merged != nil {
		return *pr.Merged, nil
	}
	return false, fmt.Errorf("Unable to determine if PR %d was merged", *obj.Issue.Number)
}

// ForEachIssueDo will run for each Issue in the project that matches:
//   * pr.Number >= minPRNumber
//   * pr.Number <= maxPRNumber
func (config *Config) ForEachIssueDo(fn MungeFunction) error {
	page := 1
	for {
		glog.V(4).Infof("Fetching page %d of issues", page)
		listOpts := &github.IssueListByRepoOptions{
			Sort:        "created",
			State:       "open",
			Direction:   "asc",
			ListOptions: github.ListOptions{PerPage: 100, Page: page},
		}
		issues, response, err := config.client.Issues.ListByRepo(config.Org, config.Project, listOpts)
		config.analytics.ListIssues.Call(config, response)
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
			glog.V(2).Infof("----==== %d ====----", *issue.Number)
			glog.V(8).Infof("Issue %d labels: %v isPR: %v", *issue.Number, issue.Labels, issue.PullRequestLinks != nil)
			obj := MungeObject{
				config: config,
				Issue:  issue,
			}
			if err := fn(&obj); err != nil {
				continue
			}
		}
		if response.LastPage == 0 || response.LastPage <= page {
			break
		}
		page++
	}
	return nil
}

// ListAllIssues grabs all issues matching the options, so you don't have to
// worry about paging. Enforces some constraints, like min/max PR number and
// having a valid user.
func (config *Config) ListAllIssues(listOpts *github.IssueListByRepoOptions) ([]*github.Issue, error) {
	allIssues := []*github.Issue{}
	page := 1
	for {
		glog.V(4).Infof("Fetching page %d of issues", page)
		listOpts.ListOptions = github.ListOptions{PerPage: 100, Page: page}
		issues, response, err := config.client.Issues.ListByRepo(config.Org, config.Project, listOpts)
		config.analytics.ListIssues.Call(config, response)
		if err != nil {
			return nil, err
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
			allIssues = append(allIssues, issue)
		}
		if response.LastPage == 0 || response.LastPage <= page {
			break
		}
		page++
	}
	return allIssues, nil
}
