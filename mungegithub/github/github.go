/*
Copyright 2015 The Kubernetes Authors.

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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/options"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
	"github.com/gregjones/httpcache"
	"github.com/gregjones/httpcache/diskcache"
	"github.com/peterbourgon/diskv"
	"golang.org/x/oauth2"
)

const (
	// stolen from https://groups.google.com/forum/#!msg/golang-nuts/a9PitPAHSSU/ziQw1-QHw3EJ
	maxInt     = int(^uint(0) >> 1)
	tokenLimit = 250 // How many github api tokens to not use

	authenticatedTokenLimit   = 5000
	unauthenticatedTokenLimit = 60

	headerRateRemaining = "X-RateLimit-Remaining"
	headerRateReset     = "X-RateLimit-Reset"

	maxCommentLen = 65535

	ghApproved         = "APPROVED"
	ghChangesRequested = "CHANGES_REQUESTED"
)

var (
	releaseMilestoneRE = regexp.MustCompile(`^v[\d]+.[\d]+$`)
	priorityLabelRE    = regexp.MustCompile(`priority/[pP]([\d]+)`)
	fixesIssueRE       = regexp.MustCompile(`(?i)(?:close|closes|closed|fix|fixes|fixed|resolve|resolves|resolved)[\s]+#([\d]+)`)
	reviewableFooterRE = regexp.MustCompile(`(?s)<!-- Reviewable:start -->.*<!-- Reviewable:end -->`)
	htmlCommentRE      = regexp.MustCompile(`(?s)<!--[^<>]*?-->\n?`)
	maxTime            = time.Unix(1<<63-62135596801, 999999999) // http://stackoverflow.com/questions/25065055/what-is-the-maximum-time-time-in-go

	// How long we locally cache the combined status of an object. We will not
	// hit the github API more than this often (per mungeObject) no matter how
	// often a caller asks for the status. Ca be much much faster for testing
	combinedStatusLifetime = 5 * time.Second
)

func suggestOauthScopes(resp *github.Response, err error) error {
	if resp != nil && resp.StatusCode == http.StatusForbidden {
		if oauthScopes := resp.Header.Get("X-Accepted-OAuth-Scopes"); len(oauthScopes) > 0 {
			err = fmt.Errorf("%v - are you using at least one of the following oauth scopes?: %s", err, oauthScopes)
		}
	}
	return err
}

func stringPtr(val string) *string { return &val }
func boolPtr(val bool) *bool       { return &val }

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

func (c *callLimitRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if c.delegate == nil {
		c.delegate = http.DefaultTransport
	}
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

	// BotName is the login for the authenticated user
	BotName string

	Org         string
	Project     string
	Url         string
	mergeMethod string

	// Filters used when munging issues
	State  string
	Labels []string

	// token is private so it won't get printed in the logs.
	token      string
	tokenFile  string
	tokenInUse string

	httpCache     httpcache.Cache
	HTTPCacheDir  string
	HTTPCacheSize uint64

	MinPRNumber int
	MaxPRNumber int

	// If true, don't make any mutating API calls
	DryRun bool

	// Base sleep time for retry loops. Defaults to 1 second.
	BaseWaitTime time.Duration

	// When we clear analytics we store the last values here
	lastAnalytics analytics
	analytics     analytics

	// Webhook configuration
	HookHandler *WebHook

	// Last fetch
	since time.Time
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

	AddLabels              analytic
	AddLabelToRepository   analytic
	RemoveLabels           analytic
	ListCollaborators      analytic
	GetIssue               analytic
	CloseIssue             analytic
	CreateIssue            analytic
	ListIssues             analytic
	ListIssueEvents        analytic
	ListCommits            analytic
	ListLabels             analytic
	GetCommit              analytic
	ListFiles              analytic
	GetCombinedStatus      analytic
	SetStatus              analytic
	GetPR                  analytic
	AddAssignee            analytic
	RemoveAssignees        analytic
	ClosePR                analytic
	OpenPR                 analytic
	GetContents            analytic
	ListComments           analytic
	ListReviewComments     analytic
	CreateComment          analytic
	DeleteComment          analytic
	EditComment            analytic
	Merge                  analytic
	GetUser                analytic
	ClearMilestone         analytic
	SetMilestone           analytic
	ListMilestones         analytic
	GetBranch              analytic
	UpdateBranchProtection analytic
	GetBranchProtection    analytic
	ListReviews            analytic
}

func (a analytics) print() {
	glog.Infof("Made %d API calls since the last Reset %f calls/sec", a.apiCount, a.apiPerSec)

	buf := new(bytes.Buffer)
	w := new(tabwriter.Writer)
	w.Init(buf, 0, 0, 1, ' ', tabwriter.AlignRight)
	fmt.Fprintf(w, "AddLabels\t%d\t\n", a.AddLabels.Count)
	fmt.Fprintf(w, "AddLabelToRepository\t%d\t\n", a.AddLabelToRepository.Count)
	fmt.Fprintf(w, "RemoveLabels\t%d\t\n", a.RemoveLabels.Count)
	fmt.Fprintf(w, "ListCollaborators\t%d\t\n", a.ListCollaborators.Count)
	fmt.Fprintf(w, "GetIssue\t%d\t\n", a.GetIssue.Count)
	fmt.Fprintf(w, "CloseIssue\t%d\t\n", a.CloseIssue.Count)
	fmt.Fprintf(w, "CreateIssue\t%d\t\n", a.CreateIssue.Count)
	fmt.Fprintf(w, "ListIssues\t%d\t\n", a.ListIssues.Count)
	fmt.Fprintf(w, "ListIssueEvents\t%d\t\n", a.ListIssueEvents.Count)
	fmt.Fprintf(w, "ListCommits\t%d\t\n", a.ListCommits.Count)
	fmt.Fprintf(w, "ListLabels\t%d\t\n", a.ListLabels.Count)
	fmt.Fprintf(w, "GetCommit\t%d\t\n", a.GetCommit.Count)
	fmt.Fprintf(w, "ListFiles\t%d\t\n", a.ListFiles.Count)
	fmt.Fprintf(w, "GetCombinedStatus\t%d\t\n", a.GetCombinedStatus.Count)
	fmt.Fprintf(w, "SetStatus\t%d\t\n", a.SetStatus.Count)
	fmt.Fprintf(w, "GetPR\t%d\t\n", a.GetPR.Count)
	fmt.Fprintf(w, "AddAssignee\t%d\t\n", a.AddAssignee.Count)
	fmt.Fprintf(w, "ClosePR\t%d\t\n", a.ClosePR.Count)
	fmt.Fprintf(w, "OpenPR\t%d\t\n", a.OpenPR.Count)
	fmt.Fprintf(w, "GetContents\t%d\t\n", a.GetContents.Count)
	fmt.Fprintf(w, "ListReviewComments\t%d\t\n", a.ListReviewComments.Count)
	fmt.Fprintf(w, "ListComments\t%d\t\n", a.ListComments.Count)
	fmt.Fprintf(w, "CreateComment\t%d\t\n", a.CreateComment.Count)
	fmt.Fprintf(w, "DeleteComment\t%d\t\n", a.DeleteComment.Count)
	fmt.Fprintf(w, "Merge\t%d\t\n", a.Merge.Count)
	fmt.Fprintf(w, "GetUser\t%d\t\n", a.GetUser.Count)
	fmt.Fprintf(w, "ClearMilestone\t%d\t\n", a.ClearMilestone.Count)
	fmt.Fprintf(w, "SetMilestone\t%d\t\n", a.SetMilestone.Count)
	fmt.Fprintf(w, "ListMilestones\t%d\t\n", a.ListMilestones.Count)
	fmt.Fprintf(w, "GetBranch\t%d\t\n", a.GetBranch.Count)
	fmt.Fprintf(w, "UpdateBranchProtection\t%d\t\n", a.UpdateBranchProtection.Count)
	fmt.Fprintf(w, "GetBranchProctection\t%d\t\n", a.GetBranchProtection.Count)
	fmt.Fprintf(w, "ListReviews\t%d\t\n", a.ListReviews.Count)
	w.Flush()
	glog.V(2).Infof("\n%v", buf)
}

// MungeObject is the object that mungers deal with. It is a combination of
// different github API objects.
type MungeObject struct {
	config      *Config
	Issue       *github.Issue
	pr          *github.PullRequest
	commits     []*github.RepositoryCommit
	events      []*github.IssueEvent
	comments    []*github.IssueComment
	prComments  []*github.PullRequestComment
	prReviews   []*github.PullRequestReview
	commitFiles []*github.CommitFile

	// we cache the combinedStatus for `combinedStatusLifetime` seconds.
	combinedStatus     *github.CombinedStatus
	combinedStatusTime time.Time

	Annotations map[string]string //annotations are things you can set yourself.
}

// Number is short for *obj.Issue.Number.
func (obj *MungeObject) Number() int {
	return *obj.Issue.Number
}

// Project is getter for obj.config.Project.
func (obj *MungeObject) Project() string {
	return obj.config.Project
}

// Org is getter for obj.config.Org.
func (obj *MungeObject) Org() string {
	return obj.config.Org
}

// Config is a getter for obj.config.
func (obj *MungeObject) Config() *Config {
	return obj.config
}

// IsRobot determines if the user is the robot running the munger
func (obj *MungeObject) IsRobot(user *github.User) bool {
	return *user.Login == obj.config.BotName
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

// NewTestObject should NEVER be used outside of _test.go code. It creates a
// MungeObject with the given fields. Normally these should be filled in lazily
// as needed
func NewTestObject(config *Config, issue *github.Issue, pr *github.PullRequest, commits []*github.RepositoryCommit, events []*github.IssueEvent) *MungeObject {
	return &MungeObject{
		config:      config,
		Issue:       issue,
		pr:          pr,
		commits:     commits,
		events:      events,
		Annotations: map[string]string{},
	}
}

// SetCombinedStatusLifetime will set the lifetime of CombinedStatus responses.
// Even though we would likely use conditional API calls hitting the CombinedStatus API
// every time we want to get a specific value is just too mean to github. This defaults
// to `combinedStatusLifetime` seconds. If you are doing local testing you may want to make
// this (much) shorter
func SetCombinedStatusLifetime(lifetime time.Duration) {
	combinedStatusLifetime = lifetime
}

// RegisterOptions registers options for the github client and returns any that require a restart
// if they are changed.
func (config *Config) RegisterOptions(opts *options.Options) sets.String {
	opts.RegisterString(&config.Org, "organization", "", "The github organization to scan")
	opts.RegisterString(&config.Project, "project", "", "The github project to scan")

	opts.RegisterString(&config.token, "token", "", "The OAuth Token to use for requests.")
	opts.RegisterString(&config.tokenFile, "token-file", "", "The file containing the OAuth token to use for requests.")
	opts.RegisterInt(&config.MinPRNumber, "min-pr-number", 0, "The minimum PR to start with")
	opts.RegisterInt(&config.MaxPRNumber, "max-pr-number", maxInt, "The maximum PR to start with")
	opts.RegisterString(&config.State, "state", "", "State of PRs to process: 'open', 'all', etc")
	opts.RegisterStringSlice(&config.Labels, "labels", []string{}, "CSV list of label which should be set on processed PRs. Unset is all labels.")
	opts.RegisterString(&config.HTTPCacheDir, "http-cache-dir", "", "Path to directory where github data can be cached across restarts, if unset use in memory cache")
	opts.RegisterUint64(&config.HTTPCacheSize, "http-cache-size", 1000, "Maximum size for the HTTP cache (in MB)")
	opts.RegisterString(&config.Url, "url", "", "The GitHub Enterprise server url (default: https://api.github.com/)")
	opts.RegisterString(&config.mergeMethod, "merge-method", "merge", "The merge method to use: merge/squash/rebase")

	return sets.NewString("token", "token-file", "min-pr-number", "max-pr-number", "state", "labels", "http-cache-dir", "http-cache-size", "url")
}

// Token returns the token.
func (config *Config) Token() string {
	return config.tokenInUse
}

// PreExecute will initialize the Config. It MUST be run before the config
// may be used to get information from Github.
func (config *Config) PreExecute() error {
	if len(config.Org) == 0 {
		glog.Fatalf("The '%s' option is required.", "organization")
	}
	if len(config.Project) == 0 {
		glog.Fatalf("The '%s' option is required.", "project")
	}

	token := config.token
	if len(token) == 0 && len(config.tokenFile) != 0 {
		data, err := ioutil.ReadFile(config.tokenFile)
		if err != nil {
			glog.Fatalf("error reading token file: %v", err)
		}
		token = strings.TrimSpace(string(data))
		config.tokenInUse = token
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

	var t *httpcache.Transport
	if config.HTTPCacheDir != "" {
		maxBytes := config.HTTPCacheSize * 1000000 // convert M to B. This is storage so not base 2...
		d := diskv.New(diskv.Options{
			BasePath:     config.HTTPCacheDir,
			CacheSizeMax: maxBytes,
		})
		cache := diskcache.NewWithDiskv(d)
		t = httpcache.NewTransport(cache)
		config.httpCache = cache
	} else {
		cache := httpcache.NewMemoryCache()
		t = httpcache.NewTransport(cache)
		config.httpCache = cache
	}
	t.Transport = transport

	zeroCacheTransport := &zeroCacheRoundTripper{
		delegate: t,
	}

	transport = zeroCacheTransport

	var tokenObj *oauth2.Token
	if len(token) > 0 {
		tokenObj = &oauth2.Token{AccessToken: token}
	}
	if tokenObj != nil {
		ts := oauth2.StaticTokenSource(tokenObj)
		transport = &oauth2.Transport{
			Base:   transport,
			Source: oauth2.ReuseTokenSource(nil, ts),
		}
	}

	client := &http.Client{
		Transport: transport,
	}
	config.client = github.NewClient(client)
	if len(config.Url) > 0 {
		url, err := url.Parse(config.Url)
		if err != nil {
			glog.Fatalf("Unable to parse url: %v: %v", config.Url, err)
		}
		config.client.BaseURL = url
	}
	config.ResetAPICount()

	// passing an empty username returns information
	// about the currently authenticated user
	username := ""
	user, _, err := config.client.Users.Get(context.Background(), username)
	if err != nil {
		return fmt.Errorf("failed to retrieve currently authenticatd user: %v", err)
	} else if user == nil {
		return errors.New("failed to retrieve currently authenticatd user: got nil result")
	} else if user.Login == nil {
		return errors.New("failed to retrieve currently authenticatd user: got nil result for user login")
	}
	config.BotName = *user.Login

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

func (config *Config) serveDebugStats(res http.ResponseWriter, req *http.Request) {
	stats := config.GetDebugStats()
	b, err := json.Marshal(stats)
	if err != nil {
		glog.Errorf("Unable to Marshal Status: %v: %v", stats, err)
		res.Header().Set("Content-type", "text/plain")
		res.WriteHeader(http.StatusInternalServerError)
		return
	}
	res.Header().Set("Content-type", "application/json")
	res.WriteHeader(http.StatusOK)
	res.Write(b)
}

// ServeDebugStats will serve out debug information at the path
func (config *Config) ServeDebugStats(path string) {
	http.HandleFunc(path, config.serveDebugStats)
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

func (config *Config) getPR(num int) (*github.PullRequest, error) {
	pr, response, err := config.client.PullRequests.Get(
		context.Background(),
		config.Org,
		config.Project,
		num,
	)
	config.analytics.GetPR.Call(config, response)
	if err != nil {
		err = suggestOauthScopes(response, err)
		glog.Errorf("Error getting PR# %d: %v", num, err)
		return nil, err
	}
	return pr, nil
}

func (config *Config) getIssue(num int) (*github.Issue, error) {
	issue, resp, err := config.client.Issues.Get(
		context.Background(),
		config.Org,
		config.Project,
		num,
	)
	config.analytics.GetIssue.Call(config, resp)
	if err != nil {
		err = suggestOauthScopes(resp, err)
		glog.Errorf("getIssue: %v", err)
		return nil, err
	}
	return issue, nil
}

func (config *Config) deleteCache(resp *github.Response) {
	cache := config.httpCache
	if cache == nil {
		return
	}
	if resp.Response == nil {
		return
	}
	req := resp.Response.Request
	if req == nil {
		return
	}
	cacheKey := req.URL.String()
	glog.Infof("Deleting cache entry for %q", cacheKey)
	cache.Delete(cacheKey)
}

// protects a branch and sets the required contexts
func (config *Config) setBranchProtection(name string, request *github.ProtectionRequest) error {
	glog.Infof("Setting protections for branch: %s", name)
	config.analytics.UpdateBranchProtection.Call(config, nil)
	if config.DryRun {
		return nil
	}
	_, resp, err := config.client.Repositories.UpdateBranchProtection(
		context.Background(),
		config.Org,
		config.Project,
		name,
		request,
	)
	if err != nil {
		err = suggestOauthScopes(resp, err)
		glog.Errorf("Unable to set branch protections for %s: %v", name, err)
		return err
	}
	return nil
}

// needsBranchProtection returns false if the branch is protected by exactly the set of
// contexts in the argument, otherwise returns true.
func (config *Config) needsBranchProtection(prot *github.Protection, contexts []string) bool {
	if prot == nil {
		if len(contexts) == 0 {
			return false
		}
		glog.Infof("Setting branch protections because Protection is nil or disabled.")
		return true
	}
	if prot.RequiredStatusChecks == nil {
		glog.Infof("Setting branch protections because branch.Protection.RequiredStatusChecks is nil")
		return true
	}
	if prot.RequiredStatusChecks.Contexts == nil {
		glog.Infof("Setting branch protections because Protection.RequiredStatusChecks.Contexts is wrong")
		return true
	}
	if prot.RequiredStatusChecks.Strict {
		glog.Infof("Setting branch protections because Protection.RequiredStatusChecks.Strict is wrong")
		return true
	}
	if prot.EnforceAdmins == nil || prot.EnforceAdmins.Enabled {
		glog.Infof("Setting branch protections because Protection.EnforceAdmins.Enabled is wrong")
		return true
	}
	branchContexts := prot.RequiredStatusChecks.Contexts

	oldSet := sets.NewString(branchContexts...)
	newSet := sets.NewString(contexts...)
	if !oldSet.Equal(newSet) {
		glog.Infof("Updating branch protections old: %v new:%v", oldSet.List(), newSet.List())
		return true
	}
	return false
}

// SetBranchProtection protects a branch and sets the required contexts
func (config *Config) SetBranchProtection(name string, contexts []string) error {
	branch, resp, err := config.client.Repositories.GetBranch(
		context.Background(),
		config.Org,
		config.Project,
		name,
	)
	config.analytics.GetBranch.Call(config, resp)
	if err != nil {
		err = suggestOauthScopes(resp, err)
		glog.Errorf("Failed to get the branch '%s': %v\n", name, err)
		return err
	}
	var prot *github.Protection
	if branch != nil && branch.Protected != nil && *branch.Protected {
		prot, resp, err = config.client.Repositories.GetBranchProtection(
			context.Background(),
			config.Org,
			config.Project,
			name,
		)
		config.analytics.GetBranchProtection.Call(config, resp)
		if err != nil {
			err = suggestOauthScopes(resp, err)
			glog.Errorf("Got error getting branch protection for branch %s: %v", name, err)
			return err
		}
	}

	if !config.needsBranchProtection(prot, contexts) {
		return nil
	}

	request := &github.ProtectionRequest{
		RequiredStatusChecks: &github.RequiredStatusChecks{
			Strict:   false,
			Contexts: contexts,
		},
		RequiredPullRequestReviews: prot.RequiredPullRequestReviews,
		Restrictions:               unchangedRestrictionRequest(prot.Restrictions),
		EnforceAdmins:              false,
	}
	return config.setBranchProtection(name, request)
}

// unchangedRestrictionRequest generates a request that will
// not make any changes to the teams and users that can merge
// into a branch
func unchangedRestrictionRequest(restrictions *github.BranchRestrictions) *github.BranchRestrictionsRequest {
	if restrictions == nil {
		return nil
	}

	request := &github.BranchRestrictionsRequest{
		Users: []string{},
		Teams: []string{},
	}

	if restrictions.Users != nil {
		for _, user := range restrictions.Users {
			request.Users = append(request.Users, *user.Login)
		}
	}
	if restrictions.Teams != nil {
		for _, team := range restrictions.Teams {
			request.Teams = append(request.Teams, *team.Name)
		}
	}
	return request
}

// Refresh will refresh the Issue (and PR if this is a PR)
// (not the commits or events)
func (obj *MungeObject) Refresh() bool {
	num := *obj.Issue.Number
	issue, err := obj.config.getIssue(num)
	if err != nil {
		glog.Errorf("Error in Refresh: %v", err)
		return false
	}
	obj.Issue = issue
	if !obj.IsPR() {
		return true
	}
	pr, err := obj.config.getPR(*obj.Issue.Number)
	if err != nil {
		return false
	}
	obj.pr = pr
	return true
}

// ListMilestones will return all milestones of the given `state`
func (config *Config) ListMilestones(state string) ([]*github.Milestone, bool) {
	listopts := github.MilestoneListOptions{
		State: state,
	}
	milestones, resp, err := config.client.Issues.ListMilestones(
		context.Background(),
		config.Org,
		config.Project,
		&listopts,
	)
	config.analytics.ListMilestones.Call(config, resp)
	if err != nil {
		glog.Errorf("Error getting milestones of state %q: %v", state, suggestOauthScopes(resp, err))
		return milestones, false
	}
	return milestones, true
}

// GetObject will return an object (with only the issue filled in)
func (config *Config) GetObject(num int) (*MungeObject, error) {
	issue, err := config.getIssue(num)
	if err != nil {
		return nil, err
	}
	obj := &MungeObject{
		config:      config,
		Issue:       issue,
		Annotations: map[string]string{},
	}
	return obj, nil
}

// NewIssue will file a new issue and return an object for it.
// If "owners" is not empty, the issue will be assigned to the owners.
func (config *Config) NewIssue(title, body string, labels []string, owners []string) (*MungeObject, error) {
	config.analytics.CreateIssue.Call(config, nil)
	glog.Infof("Creating an issue: %q", title)
	if config.DryRun {
		return nil, fmt.Errorf("can't make issues in dry-run mode")
	}
	if len(owners) == 0 {
		owners = []string{}
	}
	if len(body) > maxCommentLen {
		body = body[:maxCommentLen]
	}

	issue, resp, err := config.client.Issues.Create(
		context.Background(),
		config.Org,
		config.Project,
		&github.IssueRequest{
			Title:     &title,
			Body:      &body,
			Labels:    &labels,
			Assignees: &owners,
		},
	)
	if err != nil {
		err = suggestOauthScopes(resp, err)
		glog.Errorf("createIssue: %v", err)
		return nil, err
	}
	obj := &MungeObject{
		config:      config,
		Issue:       issue,
		Annotations: map[string]string{},
	}
	return obj, nil
}

// GetBranchCommits gets recent commits for the given branch.
func (config *Config) GetBranchCommits(branch string, limit int) ([]*github.RepositoryCommit, error) {
	commits := []*github.RepositoryCommit{}
	page := 0
	for {
		commitsPage, response, err := config.client.Repositories.ListCommits(
			context.Background(),
			config.Org,
			config.Project,
			&github.CommitsListOptions{
				ListOptions: github.ListOptions{PerPage: 100, Page: page},
				SHA:         branch,
			},
		)
		config.analytics.ListCommits.Call(config, response)
		if err != nil {
			err = suggestOauthScopes(response, err)
			glog.Errorf("Error reading commits for branch %s: %v", branch, err)
			return nil, err
		}
		commits = append(commits, commitsPage...)
		if response.LastPage == 0 || response.LastPage <= page || len(commits) > limit {
			break
		}
		page++
	}
	return commits, nil
}

// GetTokenUsage returns the api token usage of the current github user.
func (config *Config) GetTokenUsage() int {
	config.apiLimit.Lock()
	remaining := config.apiLimit.remaining
	config.apiLimit.Unlock()

	if config.tokenInUse != "" {
		return authenticatedTokenLimit - remaining
	}
	return unauthenticatedTokenLimit - remaining
}

// Branch returns the branch the PR is for. Return "" if this is not a PR or
// it does not have the required information.
func (obj *MungeObject) Branch() (string, bool) {
	pr, ok := obj.GetPR()
	if !ok {
		return "", ok
	}
	if pr.Base != nil && pr.Base.Ref != nil {
		return *pr.Base.Ref, ok
	}
	return "", ok
}

// IsForBranch return true if the object is a PR for a branch with the given
// name. It return false if it is not a pr, it isn't against the given branch,
// or we can't tell
func (obj *MungeObject) IsForBranch(branch string) (bool, bool) {
	objBranch, ok := obj.Branch()
	if !ok {
		return false, ok
	}
	if objBranch == branch {
		return true, ok
	}
	return false, ok
}

// LastModifiedTime returns the time the last commit was made
// BUG: this should probably return the last time a git push happened or something like that.
func (obj *MungeObject) LastModifiedTime() (*time.Time, bool) {
	var lastModified *time.Time
	commits, ok := obj.GetCommits()
	if !ok {
		glog.Errorf("Error in LastModifiedTime, unable to get commits")
		return lastModified, ok
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
	return lastModified, true
}

// FirstLabelTime returns the first time the request label was added to an issue.
// If the label was never added you will get a nil time.
func (obj *MungeObject) FirstLabelTime(label string) *time.Time {
	event := obj.labelEvent(label, firstTime)
	if event == nil {
		return nil
	}
	return event.CreatedAt
}

// Return true if 'a' is preferable to 'b'. Handle nil times!
type timePred func(a, b *time.Time) bool

func firstTime(a, b *time.Time) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return !a.After(*b)
}

func lastTime(a, b *time.Time) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return a.After(*b)
}

// labelEvent returns the event where the given label was added to an issue.
// 'pred' is used to select which label event is chosen if there are multiple.
func (obj *MungeObject) labelEvent(label string, pred timePred) *github.IssueEvent {
	var labelTime *time.Time
	var out *github.IssueEvent
	events, ok := obj.GetEvents()
	if !ok {
		return nil
	}
	for _, event := range events {
		if *event.Event == "labeled" && *event.Label.Name == label {
			if pred(event.CreatedAt, labelTime) {
				labelTime = event.CreatedAt
				out = event
			}
		}
	}
	return out
}

// LabelTime returns the last time the request label was added to an issue.
// If the label was never added you will get a nil time.
func (obj *MungeObject) LabelTime(label string) (*time.Time, bool) {
	event := obj.labelEvent(label, lastTime)
	if event == nil {
		glog.Errorf("Error in LabelTime, received nil event value")
		return nil, false
	}
	return event.CreatedAt, true
}

// LabelCreator returns the user who (last) created the given label
func (obj *MungeObject) LabelCreator(label string) (*github.User, bool) {
	event := obj.labelEvent(label, lastTime)
	if event == nil || event.Actor == nil || event.Actor.Login == nil {
		glog.Errorf("Error in LabelCreator, received nil event value")
		return nil, false
	}
	return event.Actor, true
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

// LabelSet returns the name of all of the labels applied to the object as a
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

// AddLabel adds a single `label` to the issue
func (obj *MungeObject) AddLabel(label string) error {
	return obj.AddLabels([]string{label})
}

// AddLabels will add all of the named `labels` to the issue
func (obj *MungeObject) AddLabels(labels []string) error {
	config := obj.config
	prNum := *obj.Issue.Number
	config.analytics.AddLabels.Call(config, nil)
	glog.Infof("Adding labels %v to PR %d", labels, prNum)
	if len(labels) == 0 {
		glog.Info("No labels to add: quitting")
		return nil
	}

	if config.DryRun {
		return nil
	}
	for _, l := range labels {
		label := github.Label{
			Name: &l,
		}
		obj.Issue.Labels = append(obj.Issue.Labels, label)
	}
	_, resp, err := config.client.Issues.AddLabelsToIssue(
		context.Background(),
		obj.Org(),
		obj.Project(),
		prNum,
		labels,
	)
	if err != nil {
		glog.Errorf("Failed to set labels %v for PR %d: %v", labels, prNum, suggestOauthScopes(resp, err))
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
		// We do this crazy delete since users might be iterating over `range obj.Issue.Labels`
		// Make a completely new copy and leave their ranging alone.
		temp := make([]github.Label, len(obj.Issue.Labels)-1)
		copy(temp, obj.Issue.Labels[:which])
		copy(temp[which:], obj.Issue.Labels[which+1:])
		obj.Issue.Labels = temp
	}

	config.analytics.RemoveLabels.Call(config, nil)
	glog.Infof("Removing label %q to PR %d", label, prNum)
	if config.DryRun {
		return nil
	}
	resp, err := config.client.Issues.RemoveLabelForIssue(
		context.Background(),
		obj.Org(),
		obj.Project(),
		prNum,
		label,
	)
	if err != nil {
		glog.Errorf("Failed to remove %v from issue %d: %v", label, prNum, suggestOauthScopes(resp, err))
		return err
	}
	return nil
}

// ModifiedAfterLabeled returns true if the PR was updated after the last time the
// label was applied.
func (obj *MungeObject) ModifiedAfterLabeled(label string) (after bool, ok bool) {
	labelTime, ok := obj.LabelTime(label)
	if !ok || labelTime == nil {
		glog.Errorf("Unable to find label time for: %q on %d", label, obj.Number())
		return false, false
	}
	lastModifiedTime, ok := obj.LastModifiedTime()
	if !ok || lastModifiedTime == nil {
		glog.Errorf("Unable to find last modification time for %d", obj.Number())
		return false, false
	}
	after = lastModifiedTime.After(*labelTime)
	return after, true
}

// GetHeadAndBase returns the head SHA and the base ref, so that you can get
// the base's sha in a second step. Purpose: if head and base SHA are the same
// across two merge attempts, we don't need to rerun tests.
func (obj *MungeObject) GetHeadAndBase() (headSHA, baseRef string, ok bool) {
	pr, ok := obj.GetPR()
	if !ok {
		return "", "", false
	}
	if pr.Head == nil || pr.Head.SHA == nil {
		return "", "", false
	}
	headSHA = *pr.Head.SHA
	if pr.Base == nil || pr.Base.Ref == nil {
		return "", "", false
	}
	baseRef = *pr.Base.Ref
	return headSHA, baseRef, true
}

// GetSHAFromRef returns the current SHA of the given ref (i.e., branch).
func (obj *MungeObject) GetSHAFromRef(ref string) (sha string, ok bool) {
	commit, response, err := obj.config.client.Repositories.GetCommit(
		context.Background(),
		obj.Org(),
		obj.Project(),
		ref,
	)
	obj.config.analytics.GetCommit.Call(obj.config, response)
	if err != nil {
		glog.Errorf("Failed to get commit for %v, %v, %v: %v", obj.Org(), obj.Project(), ref, suggestOauthScopes(response, err))
		return "", false
	}
	if commit.SHA == nil {
		return "", false
	}
	return *commit.SHA, true
}

// ClearMilestone will remove a milestone if present
func (obj *MungeObject) ClearMilestone() bool {
	if obj.Issue.Milestone == nil {
		return true
	}
	obj.config.analytics.ClearMilestone.Call(obj.config, nil)
	obj.Issue.Milestone = nil
	if obj.config.DryRun {
		return true
	}

	// Send the request manually to work around go-github's use of
	// omitempty (precluding the use of null) in the json field
	// definition for milestone.
	//
	// Reference: https://github.com/google/go-github/issues/236
	u := fmt.Sprintf("repos/%v/%v/issues/%d", obj.Org(), obj.Project(), *obj.Issue.Number)
	req, err := obj.config.client.NewRequest("PATCH", u, &struct {
		Milestone interface{} `json:"milestone"`
	}{})
	if err != nil {
		glog.Errorf("Failed to clear milestone on issue %d: %v", *obj.Issue.Number, err)
		return false
	}
	_, err = obj.config.client.Do(context.Background(), req, nil)
	if err != nil {
		glog.Errorf("Failed to clear milestone on issue %d: %v", *obj.Issue.Number, err)
		return false
	}
	return true
}

// SetMilestone will set the milestone to the value specified
func (obj *MungeObject) SetMilestone(title string) bool {
	milestones, ok := obj.config.ListMilestones("all")
	if !ok {
		glog.Errorf("Error in SetMilestone, obj.config.ListMilestones failed")
		return false
	}
	var milestone *github.Milestone
	for _, m := range milestones {
		if m.Title == nil || m.Number == nil {
			glog.Errorf("Found milestone with nil title of number: %v", m)
			continue
		}
		if *m.Title == title {
			milestone = m
			break
		}
	}
	if milestone == nil {
		glog.Errorf("Unable to find milestone with title %q", title)
		return false
	}

	obj.config.analytics.SetMilestone.Call(obj.config, nil)
	obj.Issue.Milestone = milestone
	if obj.config.DryRun {
		return true
	}

	_, resp, err := obj.config.client.Issues.Edit(
		context.Background(),
		obj.Org(),
		obj.Project(),
		*obj.Issue.Number,
		&github.IssueRequest{Milestone: milestone.Number},
	)
	if err != nil {
		glog.Errorf("Failed to set milestone %d on issue %d: %v", *milestone.Number, *obj.Issue.Number, suggestOauthScopes(resp, err))
		return false
	}
	return true
}

// ReleaseMilestone returns the name of the 'release' milestone or an empty string
// if none found. Release milestones are determined by the format "vX.Y"
func (obj *MungeObject) ReleaseMilestone() (string, bool) {
	milestone := obj.Issue.Milestone
	if milestone == nil {
		return "", true
	}
	title := milestone.Title
	if title == nil {
		glog.Errorf("Error in ReleaseMilestone, nil milestone.Title")
		return "", false
	}
	if !releaseMilestoneRE.MatchString(*title) {
		return "", true
	}
	return *title, true
}

// ReleaseMilestoneDue returns the due date for a milestone. It ONLY looks at
// milestones of the form 'vX.Y' where X and Y are integeters. Return the maximum
// possible time if there is no milestone or the milestone doesn't look like a
// release milestone
func (obj *MungeObject) ReleaseMilestoneDue() (time.Time, bool) {
	milestone := obj.Issue.Milestone
	if milestone == nil {
		return maxTime, true
	}
	title := milestone.Title
	if title == nil {
		glog.Errorf("Error in ReleaseMilestoneDue, nil milestone.Title")
		return maxTime, false
	}
	if !releaseMilestoneRE.MatchString(*title) {
		return maxTime, true
	}
	if milestone.DueOn == nil {
		return maxTime, true
	}
	return *milestone.DueOn, true
}

// Priority returns the priority an issue was labeled with.
// The labels must take the form 'priority/[pP][0-9]+'
// or math.MaxInt32 if unset
//
// If a PR has both priority/p0 and priority/p1 it will be considered a p0.
func (obj *MungeObject) Priority() int {
	priority := math.MaxInt32
	priorityLabels := GetLabelsWithPrefix(obj.Issue.Labels, "priority/")
	for _, label := range priorityLabels {
		matches := priorityLabelRE.FindStringSubmatch(label)
		// First match should be the whole label, second match the number itself
		if len(matches) != 2 {
			continue
		}
		prio, err := strconv.Atoi(matches[1])
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

// Collaborators is a set of all logins who can be
// listed as assignees, reviewers or approvers for
// issues and pull requests in this repo
func (config *Config) Collaborators() (sets.String, error) {
	logins := sets.NewString()
	users, err := config.fetchAllCollaborators()
	if err != nil {
		return logins, err
	}
	for _, user := range users {
		if user.Login != nil && *user.Login != "" {
			logins.Insert(strings.ToLower(*user.Login))
		}
	}
	return logins, nil
}

func (config *Config) fetchAllCollaborators() ([]*github.User, error) {
	page := 1
	var result []*github.User
	for {
		glog.V(4).Infof("Fetching page %d of all users", page)
		listOpts := &github.ListOptions{PerPage: 100, Page: page}
		users, response, err := config.client.Repositories.ListCollaborators(
			context.Background(),
			config.Org,
			config.Project,
			listOpts,
		)
		if err != nil {
			return nil, suggestOauthScopes(response, err)
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
func (config *Config) UsersWithAccess() ([]*github.User, []*github.User, error) {
	pushUsers := []*github.User{}
	pullUsers := []*github.User{}

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
	user, response, err := config.client.Users.Get(context.Background(), login)
	config.analytics.GetUser.Call(config, response)
	return user, err
}

// DescribeUser returns the Login string, which may be nil.
func DescribeUser(u *github.User) string {
	if u != nil && u.Login != nil {
		return *u.Login
	}
	return "<nil>"
}

// IsPR returns if the obj is a PR or an Issue.
func (obj *MungeObject) IsPR() bool {
	if obj.Issue.PullRequestLinks == nil {
		return false
	}
	return true
}

// GetEvents returns a list of all events for a given pr.
func (obj *MungeObject) GetEvents() ([]*github.IssueEvent, bool) {
	config := obj.config
	prNum := *obj.Issue.Number
	events := []*github.IssueEvent{}
	page := 1
	// Try to work around not finding events--suspect some cache invalidation bug when the number of pages changes.
	tryNextPageAnyway := false
	var lastResponse *github.Response
	for {
		eventPage, response, err := config.client.Issues.ListIssueEvents(
			context.Background(),
			obj.Org(),
			obj.Project(),
			prNum,
			&github.ListOptions{PerPage: 100, Page: page},
		)
		config.analytics.ListIssueEvents.Call(config, response)
		if err != nil {
			if tryNextPageAnyway {
				// Cached last page was actually truthful -- expected error.
				break
			}
			glog.Errorf("Error getting events for issue %d: %v", *obj.Issue.Number, suggestOauthScopes(response, err))
			return nil, false
		}
		if tryNextPageAnyway {
			if len(eventPage) == 0 {
				break
			}
			glog.Infof("For %v: supposedly there weren't more events, but we asked anyway and found %v more.", prNum, len(eventPage))
			obj.config.deleteCache(lastResponse)
			tryNextPageAnyway = false
		}
		events = append(events, eventPage...)
		if response.LastPage == 0 || response.LastPage <= page {
			if len(events)%100 == 0 {
				tryNextPageAnyway = true
				lastResponse = response
			} else {
				break
			}
		}
		page++
	}
	obj.events = events
	return events, true
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

func (obj *MungeObject) getCombinedStatus() (status *github.CombinedStatus, ok bool) {
	now := time.Now()
	if now.Before(obj.combinedStatusTime.Add(combinedStatusLifetime)) {
		return obj.combinedStatus, true
	}

	config := obj.config
	pr, ok := obj.GetPR()
	if !ok {
		return nil, false
	}
	if pr.Head == nil {
		glog.Errorf("pr.Head is nil in getCombinedStatus for PR# %d", *obj.Issue.Number)
		return nil, false
	}
	// TODO If we have more than 100 statuses we need to deal with paging.
	combinedStatus, response, err := config.client.Repositories.GetCombinedStatus(
		context.Background(),
		obj.Org(),
		obj.Project(),
		*pr.Head.SHA,
		&github.ListOptions{},
	)
	config.analytics.GetCombinedStatus.Call(config, response)
	if err != nil {
		glog.Errorf("Failed to get combined status: %v", suggestOauthScopes(response, err))
		return nil, false
	}
	obj.combinedStatus = combinedStatus
	obj.combinedStatusTime = now
	return combinedStatus, true
}

// SetStatus allowes you to set the Github Status
func (obj *MungeObject) SetStatus(state, url, description, statusContext string) bool {
	config := obj.config
	status := &github.RepoStatus{
		State:       &state,
		Description: &description,
		Context:     &statusContext,
	}
	if len(url) > 0 {
		status.TargetURL = &url
	}
	pr, ok := obj.GetPR()
	if !ok {
		glog.Errorf("Error in SetStatus")
		return false
	}
	ref := *pr.Head.SHA
	glog.Infof("PR %d setting %q Github status to %q", *obj.Issue.Number, statusContext, description)
	config.analytics.SetStatus.Call(config, nil)
	if config.DryRun {
		return true
	}
	_, resp, err := config.client.Repositories.CreateStatus(
		context.Background(),
		obj.Org(),
		obj.Project(),
		ref,
		status,
	)
	if err != nil {
		glog.Errorf("Unable to set status. PR %d Ref: %q: %v", *obj.Issue.Number, ref, suggestOauthScopes(resp, err))
		return false
	}
	return false
}

// GetStatus returns the actual requested status, or nil if not found
func (obj *MungeObject) GetStatus(context string) (*github.RepoStatus, bool) {
	combinedStatus, ok := obj.getCombinedStatus()
	if !ok {
		glog.Errorf("Error in GetStatus, getCombinedStatus returned error")
		return nil, false
	} else if combinedStatus == nil {
		return nil, true
	}
	for _, status := range combinedStatus.Statuses {
		if *status.Context == context {
			return &status, true
		}
	}
	return nil, true
}

// GetStatusState gets the current status of a PR.
//    * If any member of the 'requiredContexts' list is missing, it is 'incomplete'
//    * If any is 'pending', the PR is 'pending'
//    * If any is 'error', the PR is in 'error'
//    * If any is 'failure', the PR is 'failure'
//    * Otherwise the PR is 'success'
func (obj *MungeObject) GetStatusState(requiredContexts []string) (string, bool) {
	combinedStatus, ok := obj.getCombinedStatus()
	if !ok || combinedStatus == nil {
		return "failure", ok
	}
	return computeStatus(combinedStatus, requiredContexts), ok
}

// IsStatusSuccess makes sure that the combined status for all commits in a PR is 'success'
func (obj *MungeObject) IsStatusSuccess(requiredContexts []string) (bool, bool) {
	status, ok := obj.GetStatusState(requiredContexts)
	if ok && status == "success" {
		return true, ok
	}
	return false, ok
}

// GetStatusTime returns when the status was set
func (obj *MungeObject) GetStatusTime(context string) (*time.Time, bool) {
	status, ok := obj.GetStatus(context)
	if status == nil || ok == false {
		return nil, false
	}
	if status.UpdatedAt != nil {
		return status.UpdatedAt, true
	}
	return status.CreatedAt, true
}

// Sleep for the given amount of time and then write to the channel
func timeout(sleepTime time.Duration, c chan bool) {
	time.Sleep(sleepTime)
	c <- true
}

func (obj *MungeObject) doWaitStatus(pending bool, requiredContexts []string, c chan bool) {
	config := obj.config

	sleepTime := 30 * time.Second
	// If the time was explicitly set, use that instead
	if config.BaseWaitTime != 0 {
		sleepTime = 30 * config.BaseWaitTime
	}

	for {
		status, ok := obj.GetStatusState(requiredContexts)
		if !ok {
			time.Sleep(sleepTime)
			continue
		}
		var done bool
		if pending {
			done = (status == "pending")
		} else {
			done = (status != "pending")
		}
		if done {
			c <- true
			return
		}
		if config.DryRun {
			glog.V(4).Infof("PR# %d is not pending, would wait 30 seconds, but --dry-run was set", *obj.Issue.Number)
			c <- true
			return
		}
		if pending {
			glog.V(4).Infof("PR# %d is not pending, waiting for %f seconds", *obj.Issue.Number, sleepTime.Seconds())
		} else {
			glog.V(4).Infof("PR# %d is pending, waiting for %f seconds", *obj.Issue.Number, sleepTime.Seconds())
		}
		time.Sleep(sleepTime)

		// If it has been closed, assume that we want to break from the poll loop early.
		obj.Refresh()
		if obj.Issue != nil && obj.Issue.State != nil && *obj.Issue.State == "closed" {
			c <- true
		}
	}
}

// WaitForPending will wait for a PR to move into Pending.  This is useful
// because the request to test a PR again is asynchronous with the PR actually
// moving into a pending state
// returns true if it completed and false if it timed out
func (obj *MungeObject) WaitForPending(requiredContexts []string, prMaxWaitTime time.Duration) bool {
	timeoutChan := make(chan bool, 1)
	done := make(chan bool, 1)
	// Wait for the github e2e test to start
	go timeout(prMaxWaitTime, timeoutChan)
	go obj.doWaitStatus(true, requiredContexts, done)
	select {
	case <-done:
		return true
	case <-timeoutChan:
		glog.Errorf("PR# %d timed out waiting to go \"pending\"", *obj.Issue.Number)
		return false
	}
}

// WaitForNotPending will check if the github status is "pending" (CI still running)
// if so it will sleep and try again until all required status hooks have complete
// returns true if it completed and false if it timed out
func (obj *MungeObject) WaitForNotPending(requiredContexts []string, prMaxWaitTime time.Duration) bool {
	timeoutChan := make(chan bool, 1)
	done := make(chan bool, 1)
	// Wait for the github e2e test to finish
	go timeout(prMaxWaitTime, timeoutChan)
	go obj.doWaitStatus(false, requiredContexts, done)
	select {
	case <-done:
		return true
	case <-timeoutChan:
		glog.Errorf("PR# %d timed out waiting to go \"not pending\"", *obj.Issue.Number)
		return false
	}
}

// GetCommits returns all of the commits for a given PR
func (obj *MungeObject) GetCommits() ([]*github.RepositoryCommit, bool) {
	if obj.commits != nil {
		return obj.commits, true
	}
	config := obj.config
	commits := []*github.RepositoryCommit{}
	page := 0
	for {
		commitsPage, response, err := config.client.PullRequests.ListCommits(
			context.Background(),
			obj.Org(),
			obj.Project(),
			*obj.Issue.Number,
			&github.ListOptions{PerPage: 100, Page: page},
		)
		config.analytics.ListCommits.Call(config, response)
		if err != nil {
			glog.Errorf("Error commits for PR %d: %v", *obj.Issue.Number, suggestOauthScopes(response, err))
			return nil, false
		}
		commits = append(commits, commitsPage...)
		if response.LastPage == 0 || response.LastPage <= page {
			break
		}
		page++
	}

	filledCommits := []*github.RepositoryCommit{}
	for _, c := range commits {
		if c.SHA == nil {
			glog.Errorf("Invalid Repository Commit: %v", c)
			continue
		}
		commit, response, err := config.client.Repositories.GetCommit(
			context.Background(),
			obj.Org(),
			obj.Project(),
			*c.SHA,
		)
		config.analytics.GetCommit.Call(config, response)
		if err != nil {
			glog.Errorf("Can't load commit %s %s %s: %v", obj.Org(), obj.Project(), *c.SHA, suggestOauthScopes(response, err))
			continue
		}
		filledCommits = append(filledCommits, commit)
	}
	obj.commits = filledCommits
	return filledCommits, true
}

// ListFiles returns all changed files in a pull-request
func (obj *MungeObject) ListFiles() ([]*github.CommitFile, bool) {
	if obj.commitFiles != nil {
		return obj.commitFiles, true
	}

	pr, ok := obj.GetPR()
	if !ok {
		return nil, ok
	}

	prNum := *pr.Number
	allFiles := []*github.CommitFile{}

	listOpts := &github.ListOptions{}

	config := obj.config
	page := 1
	for {
		listOpts.Page = page
		glog.V(8).Infof("Fetching page %d of changed files for issue %d", page, prNum)
		files, response, err := obj.config.client.PullRequests.ListFiles(
			context.Background(),
			obj.Org(),
			obj.Project(),
			prNum,
			listOpts,
		)
		config.analytics.ListFiles.Call(config, response)
		if err != nil {
			glog.Errorf("Unable to ListFiles: %v", suggestOauthScopes(response, err))
			return nil, false
		}
		allFiles = append(allFiles, files...)
		if response.LastPage == 0 || response.LastPage <= page {
			break
		}
		page++
	}
	obj.commitFiles = allFiles
	return allFiles, true
}

// GetPR will return the PR of the object.
func (obj *MungeObject) GetPR() (*github.PullRequest, bool) {
	if obj.pr != nil {
		return obj.pr, true
	}
	if !obj.IsPR() {
		return nil, false
	}
	pr, err := obj.config.getPR(*obj.Issue.Number)
	if err != nil {
		glog.Errorf("Error in GetPR: %v", err)
		return nil, false
	}
	obj.pr = pr
	return pr, true
}

// Returns true if the github usr is in the list of assignees
func userInList(user *github.User, assignees []string) bool {
	if user == nil {
		return false
	}

	for _, assignee := range assignees {
		if *user.Login == assignee {
			return true
		}
	}

	return false
}

// RemoveAssignees removes the passed-in assignees from the github PR's assignees list
func (obj *MungeObject) RemoveAssignees(assignees ...string) error {
	config := obj.config
	prNum := *obj.Issue.Number
	config.analytics.RemoveAssignees.Call(config, nil)
	glog.Infof("Unassigning %v from PR# %d to %v", assignees, prNum)
	if config.DryRun {
		return nil
	}
	_, resp, err := config.client.Issues.RemoveAssignees(
		context.Background(),
		obj.Org(),
		obj.Project(),
		prNum,
		assignees,
	)
	if err != nil {
		err = suggestOauthScopes(resp, err)
		glog.Errorf("Error unassigning %v from PR# %d: %v", assignees, prNum, err)
		return err
	}

	// Remove people from the local object. Replace with an entirely new copy of the list
	newIssueAssignees := []*github.User{}
	for _, user := range obj.Issue.Assignees {
		if userInList(user, assignees) {
			// Skip this user
			continue
		}
		newIssueAssignees = append(newIssueAssignees, user)
	}
	obj.Issue.Assignees = newIssueAssignees

	return nil
}

// AddAssignee will assign `prNum` to the `owner` where the `owner` is asignee's github login
func (obj *MungeObject) AddAssignee(owner string) error {
	config := obj.config
	prNum := *obj.Issue.Number
	config.analytics.AddAssignee.Call(config, nil)
	glog.Infof("Assigning PR# %d to %v", prNum, owner)
	if config.DryRun {
		return nil
	}
	_, resp, err := config.client.Issues.AddAssignees(
		context.Background(),
		obj.Org(),
		obj.Project(),
		prNum,
		[]string{owner},
	)
	if err != nil {
		err = suggestOauthScopes(resp, err)
		glog.Errorf("Error assigning issue #%d to %v: %v", prNum, owner, err)
		return err
	}

	obj.Issue.Assignees = append(obj.Issue.Assignees, &github.User{
		Login: &owner,
	})

	return nil
}

// CloseIssuef will close the given issue with a message
func (obj *MungeObject) CloseIssuef(format string, args ...interface{}) error {
	config := obj.config
	msg := fmt.Sprintf(format, args...)
	if msg != "" {
		if err := obj.WriteComment(msg); err != nil {
			return fmt.Errorf("failed to write comment to %v: %q: %v", *obj.Issue.Number, msg, err)
		}
	}
	closed := "closed"
	state := &github.IssueRequest{State: &closed}
	config.analytics.CloseIssue.Call(config, nil)
	glog.Infof("Closing issue #%d: %v", *obj.Issue.Number, msg)
	if config.DryRun {
		return nil
	}
	_, resp, err := config.client.Issues.Edit(
		context.Background(),
		obj.Org(),
		obj.Project(),
		*obj.Issue.Number,
		state,
	)
	if err != nil {
		err = suggestOauthScopes(resp, err)
		glog.Errorf("Error closing issue #%d: %v: %v", *obj.Issue.Number, msg, err)
		return err
	}
	return nil
}

// ClosePR will close the Given PR
func (obj *MungeObject) ClosePR() bool {
	config := obj.config
	pr, ok := obj.GetPR()
	if !ok {
		return false
	}
	config.analytics.ClosePR.Call(config, nil)
	glog.Infof("Closing PR# %d", *pr.Number)
	if config.DryRun {
		return true
	}
	state := "closed"
	pr.State = &state
	_, resp, err := config.client.PullRequests.Edit(
		context.Background(),
		obj.Org(),
		obj.Project(),
		*pr.Number,
		pr,
	)
	if err != nil {
		glog.Errorf("Failed to close pr %d: %v", *pr.Number, suggestOauthScopes(resp, err))
		return false
	}
	return true
}

// OpenPR will attempt to open the given PR.
// It will attempt to reopen the pr `numTries` before returning an error
// and giving up.
func (obj *MungeObject) OpenPR(numTries int) bool {
	config := obj.config
	pr, ok := obj.GetPR()
	if !ok {
		glog.Errorf("Error in OpenPR")
		return false
	}
	config.analytics.OpenPR.Call(config, nil)
	glog.Infof("Opening PR# %d", *pr.Number)
	if config.DryRun {
		return true
	}
	state := "open"
	pr.State = &state
	// Try pretty hard to re-open, since it's pretty bad if we accidentally leave a PR closed
	for tries := 0; tries < numTries; tries++ {
		_, resp, err := config.client.PullRequests.Edit(
			context.Background(),
			obj.Org(),
			obj.Project(),
			*pr.Number,
			pr,
		)
		if err == nil {
			return true
		}
		glog.Warningf("failed to re-open pr %d: %v", *pr.Number, suggestOauthScopes(resp, err))
		time.Sleep(5 * time.Second)
	}
	if !ok {
		glog.Errorf("failed to re-open pr %d after %d tries, giving up", *pr.Number, numTries)
	}
	return false
}

// GetFileContents will return the contents of the `file` in the repo at `sha`
// as a string
func (obj *MungeObject) GetFileContents(file, sha string) (string, error) {
	config := obj.config
	getOpts := &github.RepositoryContentGetOptions{Ref: sha}
	if len(sha) > 0 {
		getOpts.Ref = sha
	}
	output, _, response, err := config.client.Repositories.GetContents(
		context.Background(),
		obj.Org(),
		obj.Project(),
		file,
		getOpts,
	)
	config.analytics.GetContents.Call(config, response)
	if err != nil {
		err = fmt.Errorf("unable to get %q at commit %q", file, sha)
		err = suggestOauthScopes(response, err)
		// I'm using .V(2) because .generated docs is still not in the repo...
		glog.V(2).Infof("%v", err)
		return "", err
	}
	if output == nil {
		err = fmt.Errorf("got empty contents for %q at commit %q", file, sha)
		glog.Errorf("%v", err)
		return "", err
	}
	content, err := output.GetContent()
	if err != nil {
		glog.Errorf("Unable to decode file contents: %v", err)
		return "", err
	}
	return content, nil
}

// MergeCommit will return the sha of the merge. PRs which have not merged
// (or if we hit an error) will return nil
func (obj *MungeObject) MergeCommit() (*string, bool) {
	events, ok := obj.GetEvents()
	if !ok {
		return nil, false
	}
	for _, event := range events {
		if *event.Event == "merged" {
			return event.CommitID, true
		}
	}
	return nil, true
}

// cleanIssueBody removes irrelevant parts from the issue body,
// including Reviewable footers and extra whitespace.
func cleanIssueBody(issueBody string) string {
	issueBody = reviewableFooterRE.ReplaceAllString(issueBody, "")
	issueBody = htmlCommentRE.ReplaceAllString(issueBody, "")
	return strings.TrimSpace(issueBody)
}

// MergePR will merge the given PR, duh
// "who" is who is doing the merging, like "submit-queue"
func (obj *MungeObject) MergePR(who string) bool {
	config := obj.config
	prNum := *obj.Issue.Number
	config.analytics.Merge.Call(config, nil)
	glog.Infof("Merging PR# %d", prNum)
	if config.DryRun {
		return true
	}
	mergeBody := fmt.Sprintf("Automatic merge from %s.", who)
	obj.WriteComment(mergeBody)

	if obj.Issue.Title != nil {
		mergeBody = fmt.Sprintf("%s\n\n%s", mergeBody, *obj.Issue.Title)
	}

	// Get the text of the issue body
	issueBody := ""
	if obj.Issue.Body != nil {
		issueBody = cleanIssueBody(*obj.Issue.Body)
	}

	// Get the text of the first commit
	firstCommit := ""
	if commits, ok := obj.GetCommits(); !ok {
		return false
	} else if commits[0].Commit.Message != nil {
		firstCommit = *commits[0].Commit.Message
	}

	// Include the contents of the issue body if it is not the exact same text as was
	// included in the first commit.  PRs with a single commit (by default when opened
	// via the web UI) have the same text as the first commit. If there are multiple
	// commits people often put summary info in the body. But sometimes, even with one
	// commit people will edit/update the issue body. So if there is any reason, include
	// the issue body in the merge commit in git.
	if !strings.Contains(firstCommit, issueBody) {
		mergeBody = fmt.Sprintf("%s\n\n%s", mergeBody, issueBody)
	}

	option := &github.PullRequestOptions{
		MergeMethod: config.mergeMethod,
	}

	_, resp, err := config.client.PullRequests.Merge(
		context.Background(),
		obj.Org(),
		obj.Project(),
		prNum,
		mergeBody,
		option,
	)

	// The github API https://developer.github.com/v3/pulls/#merge-a-pull-request-merge-button indicates
	// we will only get the below error if we provided a particular sha to merge PUT. We aren't doing that
	// so our best guess is that the API also provides this error message when it is recalulating
	// "mergeable". So if we get this error, check "IsPRMergeable()" which should sleep just a bit until
	// github is finished calculating. If my guess is correct, that also means we should be able to
	// then merge this PR, so try again.
	if err != nil && strings.Contains(err.Error(), "branch was modified. Review and try the merge again.") {
		if mergeable, _ := obj.IsMergeable(); mergeable {
			_, resp, err = config.client.PullRequests.Merge(
				context.Background(),
				obj.Org(),
				obj.Project(),
				prNum,
				mergeBody,
				nil,
			)
		}
	}
	if err != nil {
		glog.Errorf("Failed to merge PR: %d: %v", prNum, suggestOauthScopes(resp, err))
		return false
	}
	return true
}

// GetPRFixesList returns a list of issue numbers that are referenced in the PR body.
func (obj *MungeObject) GetPRFixesList() []int {
	prBody := ""
	if obj.Issue.Body != nil {
		prBody = *obj.Issue.Body
	}
	matches := fixesIssueRE.FindAllStringSubmatch(prBody, -1)
	if matches == nil {
		return nil
	}

	issueNums := []int{}
	for _, match := range matches {
		if num, err := strconv.Atoi(match[1]); err == nil {
			issueNums = append(issueNums, num)
		}
	}
	return issueNums
}

// ListReviewComments returns all review (diff) comments for the PR in question
func (obj *MungeObject) ListReviewComments() ([]*github.PullRequestComment, bool) {
	if obj.prComments != nil {
		return obj.prComments, true
	}

	pr, ok := obj.GetPR()
	if !ok {
		return nil, ok
	}
	prNum := *pr.Number
	allComments := []*github.PullRequestComment{}

	listOpts := &github.PullRequestListCommentsOptions{ListOptions: github.ListOptions{PerPage: 100}}

	config := obj.config
	page := 1
	// Try to work around not finding comments--suspect some cache invalidation bug when the number of pages changes.
	tryNextPageAnyway := false
	var lastResponse *github.Response
	for {
		listOpts.ListOptions.Page = page
		glog.V(8).Infof("Fetching page %d of comments for PR %d", page, prNum)
		comments, response, err := obj.config.client.PullRequests.ListComments(
			context.Background(),
			obj.Org(),
			obj.Project(),
			prNum,
			listOpts,
		)
		config.analytics.ListReviewComments.Call(config, response)
		if err != nil {
			if tryNextPageAnyway {
				// Cached last page was actually truthful -- expected error.
				break
			}
			glog.Errorf("Failed to fetch page of comments for PR %d: %v", prNum, suggestOauthScopes(response, err))
			return nil, false
		}
		if tryNextPageAnyway {
			if len(comments) == 0 {
				break
			}
			glog.Infof("For %v: supposedly there weren't more review comments, but we asked anyway and found %v more.", prNum, len(comments))
			obj.config.deleteCache(lastResponse)
			tryNextPageAnyway = false
		}
		allComments = append(allComments, comments...)
		if response.LastPage == 0 || response.LastPage <= page {
			if len(allComments)%100 == 0 {
				tryNextPageAnyway = true
				lastResponse = response
			} else {
				break
			}
		}
		page++
	}
	obj.prComments = allComments
	return allComments, true
}

// WithListOpt configures the options to list comments of github issue.
type WithListOpt func(*github.IssueListCommentsOptions) *github.IssueListCommentsOptions

// ListComments returns all comments for the issue/PR in question
func (obj *MungeObject) ListComments() ([]*github.IssueComment, bool) {
	config := obj.config
	issueNum := *obj.Issue.Number
	allComments := []*github.IssueComment{}

	if obj.comments != nil {
		return obj.comments, true
	}

	listOpts := &github.IssueListCommentsOptions{ListOptions: github.ListOptions{PerPage: 100}}

	page := 1
	// Try to work around not finding comments--suspect some cache invalidation bug when the number of pages changes.
	tryNextPageAnyway := false
	var lastResponse *github.Response
	for {
		listOpts.ListOptions.Page = page
		glog.V(8).Infof("Fetching page %d of comments for issue %d", page, issueNum)
		comments, response, err := obj.config.client.Issues.ListComments(
			context.Background(),
			obj.Org(),
			obj.Project(),
			issueNum,
			listOpts,
		)
		config.analytics.ListComments.Call(config, response)
		if err != nil {
			if tryNextPageAnyway {
				// Cached last page was actually truthful -- expected error.
				break
			}
			glog.Errorf("Failed to fetch page of comments for issue %d: %v", issueNum, suggestOauthScopes(response, err))
			return nil, false
		}
		if tryNextPageAnyway {
			if len(comments) == 0 {
				break
			}
			glog.Infof("For %v: supposedly there weren't more comments, but we asked anyway and found %v more.", issueNum, len(comments))
			obj.config.deleteCache(lastResponse)
			tryNextPageAnyway = false
		}
		allComments = append(allComments, comments...)
		if response.LastPage == 0 || response.LastPage <= page {
			if len(comments)%100 == 0 {
				tryNextPageAnyway = true
				lastResponse = response
			} else {
				break
			}
		}
		page++
	}
	obj.comments = allComments
	return allComments, true
}

// WriteComment will send the `msg` as a comment to the specified PR
func (obj *MungeObject) WriteComment(msg string) error {
	config := obj.config
	prNum := obj.Number()
	config.analytics.CreateComment.Call(config, nil)
	comment := msg
	if len(comment) > 512 {
		comment = comment[:512]
	}
	glog.Infof("Commenting in %d: %q", prNum, comment)
	if config.DryRun {
		return nil
	}
	if len(msg) > maxCommentLen {
		glog.Info("Comment in %d was larger than %d and was truncated", prNum, maxCommentLen)
		msg = msg[:maxCommentLen]
	}
	_, resp, err := config.client.Issues.CreateComment(
		context.Background(),
		obj.Org(),
		obj.Project(),
		prNum,
		&github.IssueComment{Body: &msg},
	)
	if err != nil {
		err = suggestOauthScopes(resp, err)
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
	which := -1
	for i, c := range obj.comments {
		if c.ID == nil || *c.ID != *comment.ID {
			continue
		}
		which = i
	}
	if which != -1 {
		// We do this crazy delete since users might be iterating over `range obj.comments`
		// Make a completely new copy and leave their ranging alone.
		temp := make([]*github.IssueComment, len(obj.comments)-1)
		copy(temp, obj.comments[:which])
		copy(temp[which:], obj.comments[which+1:])
		obj.comments = temp
	}
	body := "UNKNOWN"
	if comment.Body != nil {
		body = *comment.Body
	}
	author := "UNKNOWN"
	if comment.User != nil && comment.User.Login != nil {
		author = *comment.User.Login
	}
	glog.Infof("Removing comment %d from Issue %d. Author:%s Body:%q", *comment.ID, prNum, author, body)
	if config.DryRun {
		return nil
	}
	resp, err := config.client.Issues.DeleteComment(
		context.Background(),
		obj.Org(),
		obj.Project(),
		*comment.ID,
	)
	if err != nil {
		err = suggestOauthScopes(resp, err)
		glog.Errorf("Error removing comment: %v", err)
		return err
	}
	return nil
}

// EditComment will change the specified comment's body.
func (obj *MungeObject) EditComment(comment *github.IssueComment, body string) error {
	config := obj.config
	prNum := *obj.Issue.Number
	config.analytics.EditComment.Call(config, nil)
	if comment.ID == nil {
		err := fmt.Errorf("Found a comment with nil id for Issue %d", prNum)
		glog.Errorf("Found a comment with nil id for Issue %d", prNum)
		return err
	}
	author := "UNKNOWN"
	if comment.User != nil && comment.User.Login != nil {
		author = *comment.User.Login
	}
	shortBody := body
	if len(shortBody) > 512 {
		shortBody = shortBody[:512]
	}
	glog.Infof("Editing comment %d from Issue %d. Author:%s New Body:%q", *comment.ID, prNum, author, shortBody)
	if config.DryRun {
		return nil
	}
	if len(body) > maxCommentLen {
		glog.Info("Comment in %d was larger than %d and was truncated", prNum, maxCommentLen)
		body = body[:maxCommentLen]
	}
	patch := github.IssueComment{Body: &body}
	ic, resp, err := config.client.Issues.EditComment(
		context.Background(),
		obj.Org(),
		obj.Project(),
		*comment.ID,
		&patch,
	)
	if err != nil {
		err = suggestOauthScopes(resp, err)
		glog.Errorf("Error editing comment: %v", err)
		return err
	}
	comment.Body = ic.Body
	return nil
}

// IsMergeable will return if the PR is mergeable. It will pause and get the
// PR again if github did not respond the first time. So the hopefully github
// will have a response the second time. If we have no answer twice, we return
// false
func (obj *MungeObject) IsMergeable() (bool, bool) {
	if !obj.IsPR() {
		return false, true
	}
	pr, ok := obj.GetPR()
	if !ok {
		return false, ok
	}
	prNum := obj.Number()
	// Github might not have computed mergeability yet. Try again a few times.
	for try := 1; try <= 5 && pr.Mergeable == nil && (pr.Merged == nil || *pr.Merged == false); try++ {
		glog.V(4).Infof("Waiting for mergeability on %q %d", *pr.Title, prNum)
		// Sleep for 2-32 seconds on successive attempts.
		// Worst case, we'll wait for up to a minute for GitHub
		// to compute it before bailing out.
		baseDelay := time.Second
		if obj.config.BaseWaitTime != 0 { // Allow shorter delays in tests.
			baseDelay = obj.config.BaseWaitTime
		}
		time.Sleep((1 << uint(try)) * baseDelay)
		ok := obj.Refresh()
		if !ok {
			return false, ok
		}
		pr, ok = obj.GetPR()
		if !ok {
			return false, ok
		}
	}
	if pr.Merged != nil && *pr.Merged {
		glog.Errorf("Found that PR #%d is merged while looking up mergeability, Skipping", prNum)
		return false, false
	}
	if pr.Mergeable == nil {
		glog.Errorf("No mergeability information for %q %d, Skipping", *pr.Title, prNum)
		return false, false
	}
	return *pr.Mergeable, true
}

// IsMerged returns if the issue in question was already merged
func (obj *MungeObject) IsMerged() (bool, bool) {
	if !obj.IsPR() {
		glog.Errorf("Issue: %d is not a PR and is thus 'merged' is indeterminate", *obj.Issue.Number)
		return false, false
	}
	pr, ok := obj.GetPR()
	if !ok {
		return false, false
	}
	if pr.Merged != nil {
		return *pr.Merged, true
	}
	glog.Errorf("Unable to determine if PR %d was merged", *obj.Issue.Number)
	return false, false
}

// MergedAt returns the time an issue was merged (for nil if unmerged)
func (obj *MungeObject) MergedAt() (*time.Time, bool) {
	if !obj.IsPR() {
		return nil, false
	}
	pr, ok := obj.GetPR()
	if !ok {
		return nil, false
	}
	return pr.MergedAt, true
}

// ListReviews returns a list of the Pull Request Reviews on a PR.
func (obj *MungeObject) ListReviews() ([]*github.PullRequestReview, bool) {
	if obj.prReviews != nil {
		return obj.prReviews, true
	}
	if !obj.IsPR() {
		return nil, false
	}

	pr, ok := obj.GetPR()
	if !ok {
		return nil, false
	}
	prNum := *pr.Number
	allReviews := []*github.PullRequestReview{}

	listOpts := &github.ListOptions{PerPage: 100}

	config := obj.config
	page := 1
	// Try to work around not finding comments--suspect some cache invalidation bug when the number of pages changes.
	tryNextPageAnyway := false
	var lastResponse *github.Response
	for {
		listOpts.Page = page
		glog.V(8).Infof("Fetching page %d of reviews for pr %d", page, prNum)
		reviews, response, err := obj.config.client.PullRequests.ListReviews(
			context.Background(),
			obj.Org(),
			obj.Project(),
			prNum,
			listOpts,
		)
		config.analytics.ListReviews.Call(config, response)
		if err != nil {
			if tryNextPageAnyway {
				// Cached last page was actually truthful -- expected error.
				break
			}
			glog.Errorf("Failed to fetch page %d of reviews for pr %d: %v", page, prNum, suggestOauthScopes(response, err))
			return nil, false
		}
		if tryNextPageAnyway {
			if len(reviews) == 0 {
				break
			}
			glog.Infof("For %v: supposedly there weren't more reviews, but we asked anyway and found %v more.", prNum, len(reviews))
			obj.config.deleteCache(lastResponse)
			tryNextPageAnyway = false
		}
		allReviews = append(allReviews, reviews...)
		if response.LastPage == 0 || response.LastPage <= page {
			if len(allReviews)%100 == 0 {
				tryNextPageAnyway = true
				lastResponse = response
			} else {
				break
			}
		}
		page++
	}
	obj.prReviews = allReviews
	return allReviews, true
}

func (obj *MungeObject) CollectGHReviewStatus() ([]*github.PullRequestReview, []*github.PullRequestReview, bool) {
	reviews, ok := obj.ListReviews()
	if !ok {
		glog.Warning("Cannot get all reviews")
		return nil, nil, false
	}
	var approvedReviews, changesRequestReviews []*github.PullRequestReview
	latestReviews := make(map[string]*github.PullRequestReview)
	for _, review := range reviews {
		reviewer := review.User.GetLogin()
		if r, exist := latestReviews[reviewer]; !exist || r.GetSubmittedAt().Before(review.GetSubmittedAt()) {
			latestReviews[reviewer] = review
		}
	}

	for _, review := range latestReviews {
		if review.GetState() == ghApproved {
			approvedReviews = append(approvedReviews, review)
		} else if review.GetState() == ghChangesRequested {
			changesRequestReviews = append(changesRequestReviews, review)
		}
	}
	return approvedReviews, changesRequestReviews, true
}

func (config *Config) runMungeFunction(obj *MungeObject, fn MungeFunction) error {
	if obj.Issue.Number == nil {
		glog.Infof("Skipping issue with no number, very strange")
		return nil
	}
	if obj.Issue.User == nil || obj.Issue.User.Login == nil {
		glog.V(2).Infof("Skipping PR %d with no user info %#v.", *obj.Issue.Number, obj.Issue.User)
		return nil
	}
	if *obj.Issue.Number < config.MinPRNumber {
		glog.V(6).Infof("Dropping %d < %d", *obj.Issue.Number, config.MinPRNumber)
		return nil
	}
	if *obj.Issue.Number > config.MaxPRNumber {
		glog.V(6).Infof("Dropping %d > %d", *obj.Issue.Number, config.MaxPRNumber)
		return nil
	}

	// Update pull-requests references if we track them with webhooks
	if config.HookHandler != nil {
		if obj.IsPR() {
			if pr, ok := obj.GetPR(); !ok {
				return nil
			} else if pr.Head != nil && pr.Head.Ref != nil && pr.Head.SHA != nil {
				config.HookHandler.UpdatePullRequest(*obj.Issue.Number, *pr.Head.SHA)
			}
		}
	}

	glog.V(2).Infof("----==== %d ====----", *obj.Issue.Number)
	glog.V(8).Infof("Issue %d labels: %v isPR: %v", *obj.Issue.Number, obj.Issue.Labels, obj.Issue.PullRequestLinks != nil)
	if err := fn(obj); err != nil {
		return err
	}
	return nil
}

// ForEachIssueDo will run for each Issue in the project that matches:
//   * pr.Number >= minPRNumber
//   * pr.Number <= maxPRNumber
func (config *Config) ForEachIssueDo(fn MungeFunction) error {
	page := 1
	count := 0

	extraIssues := sets.NewInt()
	// Add issues modified by a received event
	if config.HookHandler != nil {
		extraIssues.Insert(config.HookHandler.PopIssues()...)
	} else {
		// We're not using the webhooks, make sure we fetch all
		// the issues
		config.since = time.Time{}
	}

	// It's a new day, let's restart from scratch.
	// Use PST timezone to determine when its a new day.
	pst := time.FixedZone("PacificStandardTime", -8*60*60 /* seconds offset from UTC */)
	if time.Now().In(pst).Format("Jan 2 2006") != config.since.In(pst).Format("Jan 2 2006") {
		config.since = time.Time{}
	}

	since := time.Now()
	for {
		glog.V(4).Infof("Fetching page %d of issues", page)
		issues, response, err := config.client.Issues.ListByRepo(
			context.Background(),
			config.Org,
			config.Project,
			&github.IssueListByRepoOptions{
				Sort:        "created",
				State:       config.State,
				Labels:      config.Labels,
				Direction:   "asc",
				ListOptions: github.ListOptions{PerPage: 100, Page: page},
				Since:       config.since,
			},
		)
		config.analytics.ListIssues.Call(config, response)
		if err != nil {
			return suggestOauthScopes(response, err)
		}
		for i := range issues {
			obj := &MungeObject{
				config:      config,
				Issue:       issues[i],
				Annotations: map[string]string{},
			}
			count++
			err := config.runMungeFunction(obj, fn)
			if err != nil {
				return err
			}
			if obj.Issue.UpdatedAt != nil && obj.Issue.UpdatedAt.After(since) {
				since = *obj.Issue.UpdatedAt
			}
			if obj.Issue.Number != nil {
				delete(extraIssues, *obj.Issue.Number)
			}
		}
		if response.LastPage == 0 || response.LastPage <= page {
			break
		}
		page++
	}
	config.since = since

	// Handle additional issues
	for id := range extraIssues {
		obj, err := config.GetObject(id)
		if err != nil {
			return err
		}
		count++
		glog.V(2).Info("Munging extra-issue: ", id)
		err = config.runMungeFunction(obj, fn)
		if err != nil {
			return err
		}
	}

	glog.Infof("Munged %d modified issues. (%d because of status change)", count, len(extraIssues))

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
		issues, response, err := config.client.Issues.ListByRepo(
			context.Background(),
			config.Org,
			config.Project,
			listOpts,
		)
		config.analytics.ListIssues.Call(config, response)
		if err != nil {
			return nil, suggestOauthScopes(response, err)
		}
		for i := range issues {
			issue := issues[i]
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

// GetLabels grabs all labels from a particular repository so you don't have to
// worry about paging.
func (config *Config) GetLabels() ([]*github.Label, error) {
	var listOpts github.ListOptions
	var allLabels []*github.Label
	page := 1
	for {
		glog.V(4).Infof("Fetching page %d of labels", page)
		listOpts = github.ListOptions{PerPage: 100, Page: page}
		labels, response, err := config.client.Issues.ListLabels(
			context.Background(),
			config.Org,
			config.Project,
			&listOpts,
		)
		config.analytics.ListLabels.Call(config, response)
		if err != nil {
			return nil, suggestOauthScopes(response, err)
		}
		for i := range labels {
			allLabels = append(allLabels, labels[i])
		}
		if response.LastPage == 0 || response.LastPage <= page {
			break
		}
		page++
	}
	return allLabels, nil
}

// AddLabel adds a single github label to the repository.
func (config *Config) AddLabel(label *github.Label) error {
	config.analytics.AddLabelToRepository.Call(config, nil)
	glog.Infof("Adding label %v to %v, %v", *label.Name, config.Org, config.Project)
	if config.DryRun {
		return nil
	}
	_, resp, err := config.client.Issues.CreateLabel(
		context.Background(),
		config.Org,
		config.Project,
		label,
	)
	if err != nil {
		return suggestOauthScopes(resp, err)
	}
	return nil
}
