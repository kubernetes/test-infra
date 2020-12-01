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

package github

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/ghproxy/ghcache"
	"k8s.io/test-infra/prow/version"
)

type timeClient interface {
	Sleep(time.Duration)
	Until(time.Time) time.Duration
}

type standardTime struct{}

func (s *standardTime) Sleep(d time.Duration) {
	time.Sleep(d)
}
func (s *standardTime) Until(t time.Time) time.Duration {
	return time.Until(t)
}

// OrganizationClient interface for organisation related API actions
type OrganizationClient interface {
	IsMember(org, user string) (bool, error)
	GetOrg(name string) (*Organization, error)
	EditOrg(name string, config Organization) (*Organization, error)
	ListOrgInvitations(org string) ([]OrgInvitation, error)
	ListOrgMembers(org, role string) ([]TeamMember, error)
	HasPermission(org, repo, user string, roles ...string) (bool, error)
	GetUserPermission(org, repo, user string) (string, error)
	UpdateOrgMembership(org, user string, admin bool) (*OrgMembership, error)
	RemoveOrgMembership(org, user string) error
}

// HookClient interface for hook related API actions
type HookClient interface {
	ListOrgHooks(org string) ([]Hook, error)
	ListRepoHooks(org, repo string) ([]Hook, error)
	EditRepoHook(org, repo string, id int, req HookRequest) error
	EditOrgHook(org string, id int, req HookRequest) error
	CreateOrgHook(org string, req HookRequest) (int, error)
	CreateRepoHook(org, repo string, req HookRequest) (int, error)
	DeleteOrgHook(org string, id int, req HookRequest) error
	DeleteRepoHook(org, repo string, id int, req HookRequest) error
}

// CommentClient interface for comment related API actions
type CommentClient interface {
	CreateComment(org, repo string, number int, comment string) error
	DeleteComment(org, repo string, id int) error
	EditComment(org, repo string, id int, comment string) error
	CreateCommentReaction(org, repo string, id int, reaction string) error
	DeleteStaleComments(org, repo string, number int, comments []IssueComment, isStale func(IssueComment) bool) error
}

// IssueClient interface for issue related API actions
type IssueClient interface {
	CreateIssue(org, repo, title, body string, milestone int, labels, assignees []string) (int, error)
	CreateIssueReaction(org, repo string, id int, reaction string) error
	ListIssueComments(org, repo string, number int) ([]IssueComment, error)
	GetIssueLabels(org, repo string, number int) ([]Label, error)
	ListIssueEvents(org, repo string, num int) ([]ListedIssueEvent, error)
	AssignIssue(org, repo string, number int, logins []string) error
	UnassignIssue(org, repo string, number int, logins []string) error
	CloseIssue(org, repo string, number int) error
	ReopenIssue(org, repo string, number int) error
	FindIssues(query, sort string, asc bool) ([]Issue, error)
	ListOpenIssues(org, repo string) ([]Issue, error)
	GetIssue(org, repo string, number int) (*Issue, error)
	EditIssue(org, repo string, number int, issue *Issue) (*Issue, error)
}

// PullRequestClient interface for pull request related API actions
type PullRequestClient interface {
	GetPullRequests(org, repo string) ([]PullRequest, error)
	GetPullRequest(org, repo string, number int) (*PullRequest, error)
	EditPullRequest(org, repo string, number int, pr *PullRequest) (*PullRequest, error)
	GetPullRequestPatch(org, repo string, number int) ([]byte, error)
	CreatePullRequest(org, repo, title, body, head, base string, canModify bool) (int, error)
	UpdatePullRequest(org, repo string, number int, title, body *string, open *bool, branch *string, canModify *bool) error
	GetPullRequestChanges(org, repo string, number int) ([]PullRequestChange, error)
	ListPullRequestComments(org, repo string, number int) ([]ReviewComment, error)
	ListReviews(org, repo string, number int) ([]Review, error)
	ClosePR(org, repo string, number int) error
	ReopenPR(org, repo string, number int) error
	CreateReview(org, repo string, number int, r DraftReview) error
	RequestReview(org, repo string, number int, logins []string) error
	UnrequestReview(org, repo string, number int, logins []string) error
	Merge(org, repo string, pr int, details MergeDetails) error
	IsMergeable(org, repo string, number int, SHA string) (bool, error)
	ListPRCommits(org, repo string, number int) ([]RepositoryCommit, error)
}

// CommitClient interface for commit related API actions
type CommitClient interface {
	CreateStatus(org, repo, SHA string, s Status) error
	ListStatuses(org, repo, ref string) ([]Status, error)
	GetSingleCommit(org, repo, SHA string) (SingleCommit, error)
	GetCombinedStatus(org, repo, ref string) (*CombinedStatus, error)
	ListCheckRuns(org, repo, ref string) (*CheckRunList, error)
	GetRef(org, repo, ref string) (string, error)
	DeleteRef(org, repo, ref string) error
}

// RepositoryClient interface for repository related API actions
type RepositoryClient interface {
	GetRepo(owner, name string) (FullRepo, error)
	GetRepos(org string, isUser bool) ([]Repo, error)
	GetBranches(org, repo string, onlyProtected bool) ([]Branch, error)
	GetBranchProtection(org, repo, branch string) (*BranchProtection, error)
	RemoveBranchProtection(org, repo, branch string) error
	UpdateBranchProtection(org, repo, branch string, config BranchProtectionRequest) error
	AddRepoLabel(org, repo, label, description, color string) error
	UpdateRepoLabel(org, repo, label, newName, description, color string) error
	DeleteRepoLabel(org, repo, label string) error
	GetRepoLabels(org, repo string) ([]Label, error)
	AddLabel(org, repo string, number int, label string) error
	RemoveLabel(org, repo string, number int, label string) error
	GetFile(org, repo, filepath, commit string) ([]byte, error)
	IsCollaborator(org, repo, user string) (bool, error)
	ListCollaborators(org, repo string) ([]User, error)
	CreateFork(owner, repo string) (string, error)
	ListRepoTeams(org, repo string) ([]Team, error)
	CreateRepo(owner string, isUser bool, repo RepoCreateRequest) (*FullRepo, error)
	UpdateRepo(owner, name string, repo RepoUpdateRequest) (*FullRepo, error)
}

// TeamClient interface for team related API actions
type TeamClient interface {
	CreateTeam(org string, team Team) (*Team, error)
	EditTeam(org string, t Team) (*Team, error)
	DeleteTeam(org string, id int) error
	ListTeams(org string) ([]Team, error)
	UpdateTeamMembership(org string, id int, user string, maintainer bool) (*TeamMembership, error)
	RemoveTeamMembership(org string, id int, user string) error
	ListTeamMembers(org string, id int, role string) ([]TeamMember, error)
	ListTeamRepos(org string, id int) ([]Repo, error)
	UpdateTeamRepo(id int, org, repo string, permission TeamPermission) error
	RemoveTeamRepo(id int, org, repo string) error
	ListTeamInvitations(org string, id int) ([]OrgInvitation, error)
	TeamHasMember(org string, teamID int, memberLogin string) (bool, error)
	GetTeamBySlug(slug string, org string) (*Team, error)
}

// UserClient interface for user related API actions
type UserClient interface {
	BotName() (string, error)
	BotUser() (*User, error)
	Email() (string, error)
}

// ProjectClient interface for project related API actions
type ProjectClient interface {
	GetRepoProjects(owner, repo string) ([]Project, error)
	GetOrgProjects(org string) ([]Project, error)
	GetProjectColumns(org string, projectID int) ([]ProjectColumn, error)
	CreateProjectCard(org string, columnID int, projectCard ProjectCard) (*ProjectCard, error)
	GetColumnProjectCards(org string, columnID int) ([]ProjectCard, error)
	GetColumnProjectCard(org string, columnID int, issueURL string) (*ProjectCard, error)
	MoveProjectCard(org string, projectCardID int, newColumnID int) error
	DeleteProjectCard(org string, projectCardID int) error
}

// MilestoneClient interface for milestone related API actions
type MilestoneClient interface {
	ClearMilestone(org, repo string, num int) error
	SetMilestone(org, repo string, issueNum, milestoneNum int) error
	ListMilestones(org, repo string) ([]Milestone, error)
}

// RerunClient interface for job rerun access check related API actions
type RerunClient interface {
	TeamHasMember(org string, teamID int, memberLogin string) (bool, error)
	GetTeamBySlug(slug string, org string) (*Team, error)
	IsCollaborator(org, repo, user string) (bool, error)
	IsMember(org, user string) (bool, error)
	GetIssueLabels(org, repo string, number int) ([]Label, error)
}

// Client interface for GitHub API
type Client interface {
	PullRequestClient
	RepositoryClient
	CommitClient
	IssueClient
	CommentClient
	OrganizationClient
	TeamClient
	ProjectClient
	MilestoneClient
	UserClient
	HookClient

	Throttle(hourlyTokens, burst int)
	Query(ctx context.Context, q interface{}, vars map[string]interface{}) error

	SetMax404Retries(int)

	WithFields(fields logrus.Fields) Client
	ForPlugin(plugin string) Client
	ForSubcomponent(subcomponent string) Client
}

// client interacts with the github api.
type client struct {
	// If logger is non-nil, log all method calls with it.
	logger *logrus.Entry
	// identifier is used to add more identification to the user-agent header
	identifier string
	*delegate
}

// delegate actually does the work to talk to GitHub
type delegate struct {
	time timeClient

	maxRetries    int
	max404Retries int
	maxSleepTime  time.Duration
	initialDelay  time.Duration

	gqlc     gqlClient
	client   httpClient
	bases    []string
	dry      bool
	fake     bool
	throttle throttler
	getToken func() []byte
	censor   func([]byte) []byte

	mut      sync.Mutex // protects botName and email
	userData *User
}

// ForPlugin clones the client, keeping the underlying delegate the same but adding
// a plugin identifier and log field
func (c *client) ForPlugin(plugin string) Client {
	return c.forKeyValue("plugin", plugin)
}

// ForSubcomponent clones the client, keeping the underlying delegate the same but adding
// an identifier and log field
func (c *client) ForSubcomponent(subcomponent string) Client {
	return c.forKeyValue("subcomponent", subcomponent)
}

func (c *client) forKeyValue(key, value string) Client {
	return &client{
		identifier: value,
		logger:     c.logger.WithField(key, value),
		delegate:   c.delegate,
	}
}

func (c *client) userAgent() string {
	if c.identifier != "" {
		return version.UserAgentWithIdentifier(c.identifier)
	}
	return version.UserAgent()
}

// WithFields clones the client, keeping the underlying delegate the same but adding
// fields to the logging context
func (c *client) WithFields(fields logrus.Fields) Client {
	return &client{
		logger:   c.logger.WithFields(fields),
		delegate: c.delegate,
	}
}

var (
	teamRe = regexp.MustCompile(`^(.*)/(.*)$`)
)

const (
	acceptNone = ""
	// Abort requests that don't return in 5 mins. Longest graphql calls can
	// take up to 2 minutes. This limit should ensure all successful calls return
	// but will prevent an indefinite stall if GitHub never responds.
	maxRequestTime = 5 * time.Minute

	defaultMaxRetries    = 8
	defaultMax404Retries = 2
	defaultMaxSleepTime  = 2 * time.Minute
	defaultInitialDelay  = 2 * time.Second
)

// Force the compiler to check if the TokenSource is implementing correctly.
// Tokensource is needed to dynamically update the token in the GraphQL client.
var _ oauth2.TokenSource = &reloadingTokenSource{}

type reloadingTokenSource struct {
	getToken func() []byte
}

// Interface for how prow interacts with the http client, which we may throttle.
type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Interface for how prow interacts with the graphql client, which we may throttle.
type gqlClient interface {
	Query(ctx context.Context, q interface{}, vars map[string]interface{}) error
}

// throttler sets a ceiling on the rate of GitHub requests.
// Configure with Client.Throttle()
type throttler struct {
	ticker   *time.Ticker
	throttle chan time.Time
	http     httpClient
	graph    gqlClient
	slow     int32 // Helps log once when requests start/stop being throttled
	lock     sync.RWMutex
}

func (t *throttler) Wait() {
	log := logrus.WithFields(logrus.Fields{"client": "github", "throttled": true})
	t.lock.RLock()
	defer t.lock.RUnlock()
	var more bool
	select {
	case _, more = <-t.throttle:
		// If we were throttled and the channel is now somewhat (25%+) full, note this
		if len(t.throttle) > cap(t.throttle)/4 && atomic.CompareAndSwapInt32(&t.slow, 1, 0) {
			log.Debug("Unthrottled")
		}
		if !more {
			log.Debug("Throttle channel closed")
		}
		return
	default: // Do not wait if nothing is available right now
	}
	// If this is the first time we are waiting, note this
	if slow := atomic.SwapInt32(&t.slow, 1); slow == 0 {
		log.Debug("Throttled")
	}
	_, more = <-t.throttle
	if !more {
		log.Debug("Throttle channel closed")
	}
}

func (t *throttler) Refund() {
	t.lock.RLock()
	defer t.lock.RUnlock()
	select {
	case t.throttle <- time.Now():
	default:
	}
}

func (t *throttler) Do(req *http.Request) (*http.Response, error) {
	t.Wait()
	resp, err := t.http.Do(req)
	if err == nil {
		cacheMode := ghcache.CacheResponseMode(resp.Header.Get(ghcache.CacheModeHeader))
		if ghcache.CacheModeIsFree(cacheMode) {
			// This request was fulfilled by ghcache without using an API token.
			// Refund the throttling token we preemptively consumed.
			logrus.WithFields(logrus.Fields{
				"client":     "github",
				"throttled":  true,
				"cache-mode": string(cacheMode),
			}).Debug("Throttler refunding token for free response from ghcache.")
			t.Refund()
		} else {
			logrus.WithFields(logrus.Fields{
				"client":     "github",
				"throttled":  true,
				"cache-mode": string(cacheMode),
				"path":       req.URL.Path,
				"method":     req.Method,
			}).Debug("Used token for request")

		}
	}
	return resp, err
}

func (t *throttler) Query(ctx context.Context, q interface{}, vars map[string]interface{}) error {
	t.Wait()
	t.lock.Lock()
	defer t.lock.Unlock()
	return t.graph.Query(ctx, q, vars)
}

// Throttle client to a rate of at most hourlyTokens requests per hour,
// allowing burst tokens.
func (c *client) Throttle(hourlyTokens, burst int) {
	c.log("Throttle", hourlyTokens, burst)
	c.throttle.lock.Lock()
	defer c.throttle.lock.Unlock()
	previouslyThrottled := c.throttle.ticker != nil
	if hourlyTokens <= 0 || burst <= 0 { // Disable throttle
		if previouslyThrottled { // Unwrap clients if necessary
			c.client = c.throttle.http
			c.gqlc = c.throttle.graph
			c.throttle.ticker.Stop()
			c.throttle.ticker = nil
		}
		return
	}
	rate := time.Hour / time.Duration(hourlyTokens)
	ticker := time.NewTicker(rate)
	throttle := make(chan time.Time, burst)
	for i := 0; i < burst; i++ { // Fill up the channel
		throttle <- time.Now()
	}
	go func() {
		// Refill the channel
		for t := range ticker.C {
			select {
			case throttle <- t:
			default:
			}
		}
	}()
	if !previouslyThrottled { // Wrap clients if we haven't already
		c.throttle.http = c.client
		c.throttle.graph = c.gqlc
		c.client = &c.throttle
		c.gqlc = &c.throttle
	}
	c.throttle.ticker = ticker
	c.throttle.throttle = throttle
}

func (c *client) SetMax404Retries(max int) {
	c.max404Retries = max
}

// NewClientWithFields creates a new fully operational GitHub client. With
// added logging fields.
// 'getToken' is a generator for the GitHub access token to use.
// 'bases' is a variadic slice of endpoints to use in order of preference.
//   An endpoint is used when all preceding endpoints have returned a conn err.
//   This should be used when using the ghproxy GitHub proxy cache to allow
//   this client to bypass the cache if it is temporarily unavailable.
func NewClientWithFields(fields logrus.Fields, getToken func() []byte, censor func([]byte) []byte, graphqlEndpoint string, bases ...string) Client {
	return &client{
		logger: logrus.WithFields(fields).WithField("client", "github"),
		delegate: &delegate{
			time: &standardTime{},
			gqlc: githubql.NewEnterpriseClient(
				graphqlEndpoint,
				&http.Client{
					Timeout: maxRequestTime,
					Transport: &oauth2.Transport{
						Source: newReloadingTokenSource(getToken),
						Base:   newAddHeaderTransport(),
					},
				}),
			client:        &http.Client{Timeout: maxRequestTime},
			bases:         bases,
			getToken:      getToken,
			censor:        censor,
			dry:           false,
			maxRetries:    defaultMaxRetries,
			max404Retries: defaultMax404Retries,
			initialDelay:  defaultInitialDelay,
			maxSleepTime:  defaultMaxSleepTime,
		},
	}
}

func newAddHeaderTransport() http.RoundTripper {
	return &addHeaderTransport{}
}

type addHeaderTransport struct {
}

func (s *addHeaderTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	// We have to add this header to enable the Checks scheme preview:
	// https://docs.github.com/en/enterprise-server@2.22/graphql/overview/schema-previews
	// Any GHE version after 2.22 will enable the Checks types per default
	r.Header.Add("Accept", "application/vnd.github.antiope-preview+json")
	return http.DefaultTransport.RoundTrip(r)
}

// NewClient creates a new fully operational GitHub client.
func NewClient(getToken func() []byte, censor func([]byte) []byte, graphqlEndpoint string, bases ...string) Client {
	return NewClientWithFields(logrus.Fields{}, getToken, censor, graphqlEndpoint, bases...)
}

// NewDryRunClientWithFields creates a new client that will not perform mutating actions
// such as setting statuses or commenting, but it will still query GitHub and
// use up API tokens. Additional fields are added to the logger.
// 'getToken' is a generator the GitHub access token to use.
// 'bases' is a variadic slice of endpoints to use in order of preference.
//   An endpoint is used when all preceding endpoints have returned a conn err.
//   This should be used when using the ghproxy GitHub proxy cache to allow
//   this client to bypass the cache if it is temporarily unavailable.
func NewDryRunClientWithFields(fields logrus.Fields, getToken func() []byte, censor func([]byte) []byte, graphqlEndpoint string, bases ...string) Client {
	return &client{
		logger: logrus.WithFields(fields).WithField("client", "github"),
		delegate: &delegate{
			time: &standardTime{},
			gqlc: githubql.NewEnterpriseClient(
				graphqlEndpoint,
				&http.Client{
					Timeout:   maxRequestTime,
					Transport: &oauth2.Transport{Source: newReloadingTokenSource(getToken)},
				}),
			client:        &http.Client{Timeout: maxRequestTime},
			bases:         bases,
			getToken:      getToken,
			censor:        censor,
			dry:           true,
			maxRetries:    defaultMaxRetries,
			max404Retries: defaultMax404Retries,
			initialDelay:  defaultInitialDelay,
			maxSleepTime:  defaultMaxSleepTime,
		},
	}
}

// NewDryRunClient creates a new client that will not perform mutating actions
// such as setting statuses or commenting, but it will still query GitHub and
// use up API tokens.
// 'getToken' is a generator the GitHub access token to use.
// 'bases' is a variadic slice of endpoints to use in order of preference.
//   An endpoint is used when all preceding endpoints have returned a conn err.
//   This should be used when using the ghproxy GitHub proxy cache to allow
//   this client to bypass the cache if it is temporarily unavailable.
func NewDryRunClient(getToken func() []byte, censor func([]byte) []byte, graphqlEndpoint string, bases ...string) Client {
	return NewDryRunClientWithFields(logrus.Fields{}, getToken, censor, graphqlEndpoint, bases...)
}

// NewFakeClient creates a new client that will not perform any actions at all.
func NewFakeClient() Client {
	return &client{
		logger: logrus.WithField("client", "github"),
		delegate: &delegate{
			time: &standardTime{},
			fake: true,
			dry:  true,
		},
	}
}

func (c *client) log(methodName string, args ...interface{}) (logDuration func()) {
	if c.logger == nil {
		return func() {}
	}
	var as []string
	for _, arg := range args {
		as = append(as, fmt.Sprintf("%v", arg))
	}
	start := time.Now()
	c.logger.Infof("%s(%s)", methodName, strings.Join(as, ", "))
	return func() {
		c.logger.WithField("duration", time.Since(start).String()).Debugf("%s(%s) finished", methodName, strings.Join(as, ", "))
	}
}

type request struct {
	method      string
	path        string
	accept      string
	requestBody interface{}
	exitCodes   []int
}

type requestError struct {
	ClientError error
	ErrorString string
}

func (r requestError) Error() string {
	return r.ErrorString
}

func (r requestError) ErrorMessages() []string {
	clientErr, isClientError := r.ClientError.(ClientError)
	if isClientError {
		errors := []string{}
		for _, subErr := range clientErr.Errors {
			errors = append(errors, subErr.Message)
		}
		return errors
	}
	alternativeClientErr, isAlternativeClientError := r.ClientError.(AlternativeClientError)
	if isAlternativeClientError {
		return alternativeClientErr.Errors
	}
	return []string{}
}

// NewNotFound returns a NotFound error which may be useful for tests
func NewNotFound() error {
	return requestError{
		ClientError: ClientError{
			Errors: []clientErrorSubError{{Message: "status code 404"}},
		},
	}
}

func IsNotFound(err error) bool {
	if err == nil {
		return false
	}

	var requestErr requestError
	if !errors.As(err, &requestErr) {
		return false
	}

	for _, errorMsg := range requestErr.ErrorMessages() {
		if strings.Contains(errorMsg, "status code 404") {
			return true
		}
	}
	return false
}

// Make a request with retries. If ret is not nil, unmarshal the response body
// into it. Returns an error if the exit code is not one of the provided codes.
func (c *client) request(r *request, ret interface{}) (int, error) {
	statusCode, b, err := c.requestRaw(r)
	if err != nil {
		return statusCode, err
	}
	if ret != nil {
		if err := json.Unmarshal(b, ret); err != nil {
			return statusCode, err
		}
	}
	return statusCode, nil
}

// requestRaw makes a request with retries and returns the response body.
// Returns an error if the exit code is not one of the provided codes.
func (c *client) requestRaw(r *request) (int, []byte, error) {
	if c.fake || (c.dry && r.method != http.MethodGet) {
		return r.exitCodes[0], nil, nil
	}
	resp, err := c.requestRetry(r.method, r.path, r.accept, r.requestBody)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	var okCode bool
	for _, code := range r.exitCodes {
		if code == resp.StatusCode {
			okCode = true
			break
		}
	}
	if !okCode {
		clientError := unmarshalClientError(b)
		err = requestError{
			ClientError: clientError,
			ErrorString: fmt.Sprintf("status code %d not one of %v, body: %s", resp.StatusCode, r.exitCodes, string(b)),
		}
	}
	return resp.StatusCode, b, err
}

// Retry on transport failures. Retries on 500s, retries after sleep on
// ratelimit exceeded, and retries 404s a couple times.
// This function closes the response body iff it also returns an error.
func (c *client) requestRetry(method, path, accept string, body interface{}) (*http.Response, error) {
	var hostIndex int
	var resp *http.Response
	var err error
	backoff := c.initialDelay
	for retries := 0; retries < c.maxRetries; retries++ {
		if retries > 0 && resp != nil {
			resp.Body.Close()
		}
		resp, err = c.doRequest(method, c.bases[hostIndex]+path, accept, body)
		if err == nil {
			if resp.StatusCode == 404 && retries < c.max404Retries {
				// Retry 404s a couple times. Sometimes GitHub is inconsistent in
				// the sense that they send us an event such as "PR opened" but an
				// immediate request to GET the PR returns 404. We don't want to
				// retry more than a couple times in this case, because a 404 may
				// be caused by a bad API call and we'll just burn through API
				// tokens.
				c.logger.WithField("backoff", backoff.String()).Debug("Retrying 404")
				c.time.Sleep(backoff)
				backoff *= 2
			} else if resp.StatusCode == 403 {
				if resp.Header.Get("X-RateLimit-Remaining") == "0" {
					// If we are out of API tokens, sleep first. The X-RateLimit-Reset
					// header tells us the time at which we can request again.
					var t int
					if t, err = strconv.Atoi(resp.Header.Get("X-RateLimit-Reset")); err == nil {
						// Sleep an extra second plus how long GitHub wants us to
						// sleep. If it's going to take too long, then break.
						sleepTime := c.time.Until(time.Unix(int64(t), 0)) + time.Second
						if sleepTime < c.maxSleepTime {
							c.logger.WithField("backoff", sleepTime.String()).WithField("path", path).Debug("Retrying after token budget reset")
							c.time.Sleep(sleepTime)
						} else {
							err = fmt.Errorf("sleep time for token reset exceeds max sleep time (%v > %v)", sleepTime, c.maxSleepTime)
							resp.Body.Close()
							break
						}
					} else {
						err = fmt.Errorf("failed to parse rate limit reset unix time %q: %v", resp.Header.Get("X-RateLimit-Reset"), err)
						resp.Body.Close()
						break
					}
				} else if rawTime := resp.Header.Get("Retry-After"); rawTime != "" && rawTime != "0" {
					// If we are getting abuse rate limited, we need to wait or
					// else we risk continuing to make the situation worse
					var t int
					if t, err = strconv.Atoi(rawTime); err == nil {
						// Sleep an extra second plus how long GitHub wants us to
						// sleep. If it's going to take too long, then break.
						sleepTime := time.Duration(t+1) * time.Second
						if sleepTime < c.maxSleepTime {
							c.logger.WithField("backoff", sleepTime.String()).WithField("path", path).Debug("Retrying after abuse ratelimit reset")
							c.time.Sleep(sleepTime)
						} else {
							err = fmt.Errorf("sleep time for abuse rate limit exceeds max sleep time (%v > %v)", sleepTime, c.maxSleepTime)
							resp.Body.Close()
							break
						}
					} else {
						err = fmt.Errorf("failed to parse abuse rate limit wait time %q: %v", rawTime, err)
						resp.Body.Close()
						break
					}
				} else if oauthScopes := resp.Header.Get("X-Accepted-OAuth-Scopes"); len(oauthScopes) > 0 {
					authorizedScopes := resp.Header.Get("X-OAuth-Scopes")
					if authorizedScopes == "" {
						authorizedScopes = "no"
					}
					err = fmt.Errorf("the account is using %s oauth scopes, please make sure you are using at least one of the following oauth scopes: %s", authorizedScopes, oauthScopes)
					resp.Body.Close()
					break
				}
			} else if resp.StatusCode < 500 {
				// Normal, happy case.
				break
			} else {
				// Retry 500 after a break.
				c.logger.WithField("backoff", backoff.String()).Debug("Retrying 5XX")
				c.time.Sleep(backoff)
				backoff *= 2
			}
		} else {
			// Connection problem. Try a different host.
			oldHostIndex := hostIndex
			hostIndex = (hostIndex + 1) % len(c.bases)
			c.logger.WithFields(logrus.Fields{
				"backoff":      backoff.String(),
				"old-endpoint": c.bases[oldHostIndex],
				"new-endpoint": c.bases[hostIndex],
			}).Debug("Retrying request due to connection problem")
			c.time.Sleep(backoff)
			backoff *= 2
		}
	}
	return resp, err
}

func (c *client) doRequest(method, path, accept string, body interface{}) (*http.Response, error) {
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		b = c.censor(b)
		buf = bytes.NewBuffer(b)
	}
	req, err := http.NewRequest(method, path, buf)
	if err != nil {
		return nil, err
	}
	if header := c.authHeader(); len(header) > 0 {
		req.Header.Set("Authorization", header)
	}
	if accept == acceptNone {
		req.Header.Add("Accept", "application/vnd.github.v3+json")
	} else {
		req.Header.Add("Accept", accept)
	}
	if userAgent := c.userAgent(); userAgent != "" {
		req.Header.Add("User-Agent", userAgent)
	}
	// Disable keep-alive so that we don't get flakes when GitHub closes the
	// connection prematurely.
	// https://go-review.googlesource.com/#/c/3210/ fixed it for GET, but not
	// for POST.
	req.Close = true

	c.logger.WithField("curl", toCurl(req)).Trace("Executing http request")
	return c.client.Do(req)
}

// toCurl is a slightly adjusted copy of https://github.com/kubernetes/kubernetes/blob/74053d555d71a14e3853b97e204d7d6415521375/staging/src/k8s.io/client-go/transport/round_trippers.go#L339
func toCurl(r *http.Request) string {
	headers := ""
	for key, values := range r.Header {
		for _, value := range values {
			headers += fmt.Sprintf(` -H %q`, fmt.Sprintf("%s: %s", key, maskAuthorizationHeader(key, value)))
		}
	}

	return fmt.Sprintf("curl -k -v -X%s %s '%s'", r.Method, headers, r.URL.String())
}

var knownAuthTypes = sets.NewString("bearer", "basic", "negotiate")

// maskAuthorizationHeader masks credential content from authorization headers
// See https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Authorization
func maskAuthorizationHeader(key string, value string) string {
	if !strings.EqualFold(key, "Authorization") {
		return value
	}
	if len(value) == 0 {
		return ""
	}
	var authType string
	if i := strings.Index(value, " "); i > 0 {
		authType = value[0:i]
	} else {
		authType = value
	}
	if !knownAuthTypes.Has(strings.ToLower(authType)) {
		return "<masked>"
	}
	if len(value) > len(authType)+1 {
		value = authType + " <masked>"
	} else {
		value = authType
	}
	return value
}

func (c *client) authHeader() string {
	token := c.getToken()
	if len(token) == 0 {
		return ""
	}
	return fmt.Sprintf("Bearer %s", token)
}

// userInfo provides the 'github_user_info' vector that is indexed
// by the user's information.
var userInfo = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "github_user_info",
		Help: "Metadata about a user, tied to their token hash.",
	},
	[]string{"token_hash", "login", "email"},
)

func init() {
	prometheus.MustRegister(userInfo)
}

// Not thread-safe - callers need to hold c.mut.
func (c *client) getUserData() error {
	c.log("User")
	var u User
	_, err := c.request(&request{
		method:    http.MethodGet,
		path:      "/user",
		exitCodes: []int{200},
	}, &u)
	if err != nil {
		return err
	}
	c.userData = &u
	// email needs to be publicly accessible via the profile
	// of the current account. Read below for more info
	// https://developer.github.com/v3/users/#get-a-single-user

	// record information for the user
	authHeaderHash := fmt.Sprintf("%x", sha256.Sum256([]byte(c.authHeader()))) // use %x to make this a utf-8 string for use as a label
	userInfo.With(prometheus.Labels{"token_hash": authHeaderHash, "login": c.userData.Login, "email": c.userData.Email}).Set(1)
	return nil
}

// BotName returns the login of the authenticated identity.
//
// See https://developer.github.com/v3/users/#get-the-authenticated-user
func (c *client) BotName() (string, error) {
	c.mut.Lock()
	defer c.mut.Unlock()
	if c.userData == nil {
		if err := c.getUserData(); err != nil {
			return "", fmt.Errorf("fetching bot name from GitHub: %v", err)
		}
	}
	return c.userData.Login, nil
}

// BotUser returns the user data of the authenticated identity.
//
// See https://developer.github.com/v3/users/#get-the-authenticated-user
func (c *client) BotUser() (*User, error) {
	c.mut.Lock()
	defer c.mut.Unlock()
	if c.userData == nil {
		if err := c.getUserData(); err != nil {
			return nil, fmt.Errorf("fetching bot name from GitHub: %v", err)
		}
	}
	return c.userData, nil
}

// Email returns the user-configured email for the authenticated identity.
//
// See https://developer.github.com/v3/users/#get-the-authenticated-user
func (c *client) Email() (string, error) {
	c.mut.Lock()
	defer c.mut.Unlock()
	if c.userData == nil {
		if err := c.getUserData(); err != nil {
			return "", fmt.Errorf("fetching e-mail from GitHub: %v", err)
		}
	}
	return c.userData.Email, nil
}

// IsMember returns whether or not the user is a member of the org.
//
// See https://developer.github.com/v3/orgs/members/#check-membership
func (c *client) IsMember(org, user string) (bool, error) {
	c.log("IsMember", org, user)
	if org == user {
		// Make it possible to run a couple of plugins on personal repos.
		return true, nil
	}
	code, err := c.request(&request{
		method:    http.MethodGet,
		path:      fmt.Sprintf("/orgs/%s/members/%s", org, user),
		exitCodes: []int{204, 404, 302},
	}, nil)
	if err != nil {
		return false, err
	}
	if code == 204 {
		return true, nil
	} else if code == 404 {
		return false, nil
	} else if code == 302 {
		return false, fmt.Errorf("requester is not %s org member", org)
	}
	// Should be unreachable.
	return false, fmt.Errorf("unexpected status: %d", code)
}

func (c *client) listHooks(org string, repo *string) ([]Hook, error) {
	var ret []Hook
	var path string
	if repo != nil {
		path = fmt.Sprintf("/repos/%s/%s/hooks", org, *repo)
	} else {
		path = fmt.Sprintf("/orgs/%s/hooks", org)
	}
	err := c.readPaginatedResults(
		path,
		acceptNone,
		func() interface{} {
			return &[]Hook{}
		},
		func(obj interface{}) {
			ret = append(ret, *(obj.(*[]Hook))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// ListOrgHooks returns a list of hooks for the org.
// https://developer.github.com/v3/orgs/hooks/#list-hooks
func (c *client) ListOrgHooks(org string) ([]Hook, error) {
	c.log("ListOrgHooks", org)
	return c.listHooks(org, nil)
}

// ListRepoHooks returns a list of hooks for the repo.
// https://developer.github.com/v3/repos/hooks/#list-hooks
func (c *client) ListRepoHooks(org, repo string) ([]Hook, error) {
	c.log("ListRepoHooks", org, repo)
	return c.listHooks(org, &repo)
}

func (c *client) editHook(org string, repo *string, id int, req HookRequest) error {
	if c.dry {
		return nil
	}
	var path string
	if repo != nil {
		path = fmt.Sprintf("/repos/%s/%s/hooks/%d", org, *repo, id)
	} else {
		path = fmt.Sprintf("/orgs/%s/hooks/%d", org, id)
	}

	_, err := c.request(&request{
		method:      http.MethodPatch,
		path:        path,
		exitCodes:   []int{200},
		requestBody: &req,
	}, nil)
	return err
}

// EditRepoHook updates an existing hook with new info (events/url/secret)
// https://developer.github.com/v3/repos/hooks/#edit-a-hook
func (c *client) EditRepoHook(org, repo string, id int, req HookRequest) error {
	c.log("EditRepoHook", org, repo, id)
	return c.editHook(org, &repo, id, req)
}

// EditOrgHook updates an existing hook with new info (events/url/secret)
// https://developer.github.com/v3/orgs/hooks/#edit-a-hook
func (c *client) EditOrgHook(org string, id int, req HookRequest) error {
	c.log("EditOrgHook", org, id)
	return c.editHook(org, nil, id, req)
}

func (c *client) createHook(org string, repo *string, req HookRequest) (int, error) {
	if c.dry {
		return -1, nil
	}
	var path string
	if repo != nil {
		path = fmt.Sprintf("/repos/%s/%s/hooks", org, *repo)
	} else {
		path = fmt.Sprintf("/orgs/%s/hooks", org)
	}
	var ret Hook
	_, err := c.request(&request{
		method:      http.MethodPost,
		path:        path,
		exitCodes:   []int{201},
		requestBody: &req,
	}, &ret)
	if err != nil {
		return 0, err
	}
	return ret.ID, nil
}

// CreateOrgHook creates a new hook for the org
// https://developer.github.com/v3/orgs/hooks/#create-a-hook
func (c *client) CreateOrgHook(org string, req HookRequest) (int, error) {
	c.log("CreateOrgHook", org)
	return c.createHook(org, nil, req)
}

// CreateRepoHook creates a new hook for the repo
// https://developer.github.com/v3/repos/hooks/#create-a-hook
func (c *client) CreateRepoHook(org, repo string, req HookRequest) (int, error) {
	c.log("CreateRepoHook", org, repo)
	return c.createHook(org, &repo, req)
}

func (c *client) deleteHook(path string) error {
	if c.dry {
		return nil
	}

	_, err := c.request(&request{
		method:    http.MethodDelete,
		path:      path,
		exitCodes: []int{204},
	}, nil)
	return err
}

// DeleteRepoHook deletes an existing repo level webhook.
// https://developer.github.com/v3/repos/hooks/#delete-a-hook
func (c *client) DeleteRepoHook(org, repo string, id int, req HookRequest) error {
	c.log("DeleteRepoHook", org, repo, id)
	path := fmt.Sprintf("/repos/%s/%s/hooks/%d", org, repo, id)
	return c.deleteHook(path)
}

// DeleteOrgHook deletes and existing org level webhook.
// https://developer.github.com/v3/orgs/hooks/#edit-a-hook
func (c *client) DeleteOrgHook(org string, id int, req HookRequest) error {
	c.log("DeleteOrgHook", org, id)
	path := fmt.Sprintf("/orgs/%s/hooks/%d", org, id)
	return c.deleteHook(path)
}

// GetOrg returns current metadata for the org
//
// https://developer.github.com/v3/orgs/#get-an-organization
func (c *client) GetOrg(name string) (*Organization, error) {
	c.log("GetOrg", name)
	var retOrg Organization
	_, err := c.request(&request{
		method:    http.MethodGet,
		path:      fmt.Sprintf("/orgs/%s", name),
		exitCodes: []int{200},
	}, &retOrg)
	if err != nil {
		return nil, err
	}
	return &retOrg, nil
}

// EditOrg will update the metadata for this org.
//
// https://developer.github.com/v3/orgs/#edit-an-organization
func (c *client) EditOrg(name string, config Organization) (*Organization, error) {
	c.log("EditOrg", name, config)
	if c.dry {
		return &config, nil
	}
	var retOrg Organization
	_, err := c.request(&request{
		method:      http.MethodPatch,
		path:        fmt.Sprintf("/orgs/%s", name),
		exitCodes:   []int{200},
		requestBody: &config,
	}, &retOrg)
	if err != nil {
		return nil, err
	}
	return &retOrg, nil
}

// ListOrgInvitations lists pending invitations to th org.
//
// https://developer.github.com/v3/orgs/members/#list-pending-organization-invitations
func (c *client) ListOrgInvitations(org string) ([]OrgInvitation, error) {
	c.log("ListOrgInvitations", org)
	if c.fake {
		return nil, nil
	}
	path := fmt.Sprintf("/orgs/%s/invitations", org)
	var ret []OrgInvitation
	err := c.readPaginatedResults(
		path,
		acceptNone,
		func() interface{} {
			return &[]OrgInvitation{}
		},
		func(obj interface{}) {
			ret = append(ret, *(obj.(*[]OrgInvitation))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// ListOrgMembers list all users who are members of an organization. If the authenticated
// user is also a member of this organization then both concealed and public members
// will be returned.
//
// Role options are "all", "admin" and "member"
//
// https://developer.github.com/v3/orgs/members/#members-list
func (c *client) ListOrgMembers(org, role string) ([]TeamMember, error) {
	c.log("ListOrgMembers", org, role)
	if c.fake {
		return nil, nil
	}
	path := fmt.Sprintf("/orgs/%s/members", org)
	var teamMembers []TeamMember
	err := c.readPaginatedResultsWithValues(
		path,
		url.Values{
			"per_page": []string{"100"},
			"role":     []string{role},
		},
		acceptNone,
		func() interface{} {
			return &[]TeamMember{}
		},
		func(obj interface{}) {
			teamMembers = append(teamMembers, *(obj.(*[]TeamMember))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return teamMembers, nil
}

// HasPermission returns true if GetUserPermission() returns any of the roles.
func (c *client) HasPermission(org, repo, user string, roles ...string) (bool, error) {
	perm, err := c.GetUserPermission(org, repo, user)
	if err != nil {
		return false, err
	}
	for _, r := range roles {
		if r == perm {
			return true, nil
		}
	}
	return false, nil
}

// GetUserPermission returns the user's permission level for a repo
//
// https://developer.github.com/v3/repos/collaborators/#review-a-users-permission-level
func (c *client) GetUserPermission(org, repo, user string) (string, error) {
	c.log("GetUserPermission", org, repo, user)

	var perm struct {
		Perm string `json:"permission"`
	}
	_, err := c.request(&request{
		method:    http.MethodGet,
		path:      fmt.Sprintf("/repos/%s/%s/collaborators/%s/permission", org, repo, user),
		exitCodes: []int{200},
	}, &perm)
	if err != nil {
		return "", err
	}
	return perm.Perm, nil
}

// UpdateOrgMembership invites a user to the org and/or updates their permission level.
//
// If the user is not already a member, this will invite them.
// This will also change the role to/from admin, on either the invitation or membership setting.
//
// https://developer.github.com/v3/orgs/members/#add-or-update-organization-membership
func (c *client) UpdateOrgMembership(org, user string, admin bool) (*OrgMembership, error) {
	c.log("UpdateOrgMembership", org, user, admin)
	om := OrgMembership{}
	if admin {
		om.Role = RoleAdmin
	} else {
		om.Role = RoleMember
	}
	if c.dry {
		return &om, nil
	}

	_, err := c.request(&request{
		method:      http.MethodPut,
		path:        fmt.Sprintf("/orgs/%s/memberships/%s", org, user),
		requestBody: &om,
		exitCodes:   []int{200},
	}, &om)
	return &om, err
}

// RemoveOrgMembership removes the user from the org.
//
// https://developer.github.com/v3/orgs/members/#remove-organization-membership
func (c *client) RemoveOrgMembership(org, user string) error {
	c.log("RemoveOrgMembership", org, user)
	_, err := c.request(&request{
		method:    http.MethodDelete,
		path:      fmt.Sprintf("/orgs/%s/memberships/%s", org, user),
		exitCodes: []int{204},
	}, nil)
	return err
}

// CreateComment creates a comment on the issue.
//
// See https://developer.github.com/v3/issues/comments/#create-a-comment
func (c *client) CreateComment(org, repo string, number int, comment string) error {
	c.log("CreateComment", org, repo, number, comment)
	ic := IssueComment{
		Body: comment,
	}
	_, err := c.request(&request{
		method:      http.MethodPost,
		path:        fmt.Sprintf("/repos/%s/%s/issues/%d/comments", org, repo, number),
		requestBody: &ic,
		exitCodes:   []int{201},
	}, nil)
	return err
}

// DeleteComment deletes the comment.
//
// See https://developer.github.com/v3/issues/comments/#delete-a-comment
func (c *client) DeleteComment(org, repo string, id int) error {
	c.log("DeleteComment", org, repo, id)
	_, err := c.request(&request{
		method:    http.MethodDelete,
		path:      fmt.Sprintf("/repos/%s/%s/issues/comments/%d", org, repo, id),
		exitCodes: []int{204},
	}, nil)
	return err
}

// EditComment changes the body of comment id in org/repo.
//
// See https://developer.github.com/v3/issues/comments/#edit-a-comment
func (c *client) EditComment(org, repo string, id int, comment string) error {
	c.log("EditComment", org, repo, id, comment)
	ic := IssueComment{
		Body: comment,
	}
	_, err := c.request(&request{
		method:      http.MethodPatch,
		path:        fmt.Sprintf("/repos/%s/%s/issues/comments/%d", org, repo, id),
		requestBody: &ic,
		exitCodes:   []int{200},
	}, nil)
	return err
}

// CreateCommentReaction responds emotionally to comment id in org/repo.
//
// See https://developer.github.com/v3/reactions/#create-reaction-for-an-issue-comment
func (c *client) CreateCommentReaction(org, repo string, id int, reaction string) error {
	c.log("CreateCommentReaction", org, repo, id, reaction)
	r := Reaction{Content: reaction}
	_, err := c.request(&request{
		method:      http.MethodPost,
		path:        fmt.Sprintf("/repos/%s/%s/issues/comments/%d/reactions", org, repo, id),
		accept:      "application/vnd.github.squirrel-girl-preview",
		exitCodes:   []int{201},
		requestBody: &r,
	}, nil)
	return err
}

// CreateIssue creates a new issue and returns its number if
// the creation is successful, otherwise any error that is encountered.
//
// See https://developer.github.com/v3/issues/#create-an-issue
func (c *client) CreateIssue(org, repo, title, body string, milestone int, labels, assignees []string) (int, error) {
	durationLogger := c.log("CreateIssue", org, repo, title)
	defer durationLogger()

	data := struct {
		Title     string   `json:"title,omitempty"`
		Body      string   `json:"body,omitempty"`
		Milestone int      `json:"milestone,omitempty"`
		Labels    []string `json:"labels,omitempty"`
		Assignees []string `json:"assignees,omitempty"`
	}{
		Title:     title,
		Body:      body,
		Milestone: milestone,
		Labels:    labels,
		Assignees: assignees,
	}
	var resp struct {
		Num int `json:"number"`
	}
	_, err := c.request(&request{
		// allow the description and draft fields
		// https://developer.github.com/changes/2018-02-22-label-description-search-preview/
		// https://developer.github.com/changes/2019-02-14-draft-pull-requests/
		accept:      "application/vnd.github.symmetra-preview+json, application/vnd.github.shadow-cat-preview",
		method:      http.MethodPost,
		path:        fmt.Sprintf("/repos/%s/%s/issues", org, repo),
		requestBody: &data,
		exitCodes:   []int{201},
	}, &resp)
	if err != nil {
		return 0, err
	}
	return resp.Num, nil
}

// CreateIssueReaction responds emotionally to org/repo#id
//
// See https://developer.github.com/v3/reactions/#create-reaction-for-an-issue
func (c *client) CreateIssueReaction(org, repo string, id int, reaction string) error {
	c.log("CreateIssueReaction", org, repo, id, reaction)
	r := Reaction{Content: reaction}
	_, err := c.request(&request{
		method:      http.MethodPost,
		path:        fmt.Sprintf("/repos/%s/%s/issues/%d/reactions", org, repo, id),
		accept:      "application/vnd.github.squirrel-girl-preview",
		requestBody: &r,
		exitCodes:   []int{200, 201},
	}, nil)
	return err
}

// DeleteStaleComments iterates over comments on an issue/PR, deleting those which the 'isStale'
// function identifies as stale. If 'comments' is nil, the comments will be fetched from GitHub.
func (c *client) DeleteStaleComments(org, repo string, number int, comments []IssueComment, isStale func(IssueComment) bool) error {
	var err error
	if comments == nil {
		comments, err = c.ListIssueComments(org, repo, number)
		if err != nil {
			return fmt.Errorf("failed to list comments while deleting stale comments. err: %v", err)
		}
	}
	for _, comment := range comments {
		if isStale(comment) {
			if err := c.DeleteComment(org, repo, comment.ID); err != nil {
				return fmt.Errorf("failed to delete stale comment with ID '%d'", comment.ID)
			}
		}
	}
	return nil
}

// readPaginatedResults iterates over all objects in the paginated result indicated by the given url.
//
// newObj() should return a new slice of the expected type
// accumulate() should accept that populated slice for each page of results.
//
// Returns an error any call to GitHub or object marshalling fails.
func (c *client) readPaginatedResults(path, accept string, newObj func() interface{}, accumulate func(interface{})) error {
	values := url.Values{
		"per_page": []string{"100"},
	}
	return c.readPaginatedResultsWithValues(path, values, accept, newObj, accumulate)
}

// readPaginatedResultsWithValues is an override that allows control over the query string.
func (c *client) readPaginatedResultsWithValues(path string, values url.Values, accept string, newObj func() interface{}, accumulate func(interface{})) error {
	pagedPath := path
	if len(values) > 0 {
		pagedPath += "?" + values.Encode()
	}
	for {
		resp, err := c.requestRetry(http.MethodGet, pagedPath, accept, nil)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return fmt.Errorf("return code not 2XX: %s", resp.Status)
		}

		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		obj := newObj()
		if err := json.Unmarshal(b, obj); err != nil {
			return err
		}

		accumulate(obj)

		link := parseLinks(resp.Header.Get("Link"))["next"]
		if link == "" {
			break
		}

		// Example for github.com:
		// * c.bases[0]: api.github.com
		// * initial call: api.github.com/repos/kubernetes/kubernetes/pulls?per_page=100
		// * next: api.github.com/repositories/22/pulls?per_page=100&page=2
		// * in this case prefix will be empty and we're just calling the path returned by next
		// Example for github enterprise:
		// * c.bases[0]: <ghe-url>/api/v3
		// * initial call: <ghe-url>/api/v3/repos/kubernetes/kubernetes/pulls?per_page=100
		// * next: <ghe-url>/api/v3/repositories/22/pulls?per_page=100&page=2
		// * in this case prefix will be "/api/v3" and we will strip the prefix. If we don't do that,
		//   the next call will go to <ghe-url>/api/v3/api/v3/repositories/22/pulls?per_page=100&page=2
		prefix := strings.TrimSuffix(resp.Request.URL.RequestURI(), pagedPath)

		u, err := url.Parse(link)
		if err != nil {
			return fmt.Errorf("failed to parse 'next' link: %v", err)
		}
		pagedPath = strings.TrimPrefix(u.RequestURI(), prefix)
	}
	return nil
}

// ListIssueComments returns all comments on an issue.
//
// Each page of results consumes one API token.
//
// See https://developer.github.com/v3/issues/comments/#list-comments-on-an-issue
func (c *client) ListIssueComments(org, repo string, number int) ([]IssueComment, error) {
	c.log("ListIssueComments", org, repo, number)
	if c.fake {
		return nil, nil
	}
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", org, repo, number)
	var comments []IssueComment
	err := c.readPaginatedResults(
		path,
		acceptNone,
		func() interface{} {
			return &[]IssueComment{}
		},
		func(obj interface{}) {
			comments = append(comments, *(obj.(*[]IssueComment))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return comments, nil
}

// ListOpenIssues returns all open issues, including pull requests
//
// Each page of results consumes one API token.
//
// See https://developer.github.com/v3/issues/#list-issues-for-a-repository
func (c *client) ListOpenIssues(org, repo string) ([]Issue, error) {
	c.log("ListOpenIssues", org, repo)
	if c.fake {
		return nil, nil
	}
	path := fmt.Sprintf("/repos/%s/%s/issues", org, repo)
	var issues []Issue
	err := c.readPaginatedResults(
		path,
		acceptNone,
		func() interface{} {
			return &[]Issue{}
		},
		func(obj interface{}) {
			issues = append(issues, *(obj.(*[]Issue))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return issues, nil
}

// GetPullRequests get all open pull requests for a repo.
//
// See https://developer.github.com/v3/pulls/#list-pull-requests
func (c *client) GetPullRequests(org, repo string) ([]PullRequest, error) {
	c.log("GetPullRequests", org, repo)
	var prs []PullRequest
	if c.fake {
		return prs, nil
	}
	path := fmt.Sprintf("/repos/%s/%s/pulls", org, repo)
	err := c.readPaginatedResults(
		path,
		// allow the description and draft fields
		// https://developer.github.com/changes/2018-02-22-label-description-search-preview/
		// https://developer.github.com/changes/2019-02-14-draft-pull-requests/
		"application/vnd.github.symmetra-preview+json, application/vnd.github.shadow-cat-preview",
		func() interface{} {
			return &[]PullRequest{}
		},
		func(obj interface{}) {
			prs = append(prs, *(obj.(*[]PullRequest))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return prs, err
}

// GetPullRequest gets a pull request.
//
// See https://developer.github.com/v3/pulls/#get-a-single-pull-request
func (c *client) GetPullRequest(org, repo string, number int) (*PullRequest, error) {
	durationLogger := c.log("GetPullRequest", org, repo, number)
	defer durationLogger()

	var pr PullRequest
	_, err := c.request(&request{
		// allow the description and draft fields
		// https://developer.github.com/changes/2018-02-22-label-description-search-preview/
		// https://developer.github.com/changes/2019-02-14-draft-pull-requests/
		accept:    "application/vnd.github.symmetra-preview+json, application/vnd.github.shadow-cat-preview",
		method:    http.MethodGet,
		path:      fmt.Sprintf("/repos/%s/%s/pulls/%d", org, repo, number),
		exitCodes: []int{200},
	}, &pr)
	return &pr, err
}

// EditPullRequest will update the pull request.
//
// See https://developer.github.com/v3/pulls/#update-a-pull-request
func (c *client) EditPullRequest(org, repo string, number int, pr *PullRequest) (*PullRequest, error) {
	durationLogger := c.log("EditPullRequest", org, repo, number)
	defer durationLogger()

	if c.dry {
		return pr, nil
	}
	edit := struct {
		Title string `json:"title,omitempty"`
		Body  string `json:"body,omitempty"`
		State string `json:"state,omitempty"`
	}{
		Title: pr.Title,
		Body:  pr.Body,
		State: pr.State,
	}
	var ret PullRequest
	_, err := c.request(&request{
		method:      http.MethodPatch,
		path:        fmt.Sprintf("/repos/%s/%s/pulls/%d", org, repo, number),
		exitCodes:   []int{200},
		requestBody: &edit,
	}, &ret)
	if err != nil {
		return nil, err
	}
	return &ret, nil
}

// GetIssue gets an issue.
//
// See https://developer.github.com/v3/issues/#get-a-single-issue
func (c *client) GetIssue(org, repo string, number int) (*Issue, error) {
	durationLogger := c.log("GetIssue", org, repo, number)
	defer durationLogger()

	var i Issue
	_, err := c.request(&request{
		// allow emoji
		// https://developer.github.com/changes/2018-02-22-label-description-search-preview/
		accept:    "application/vnd.github.symmetra-preview+json",
		method:    http.MethodGet,
		path:      fmt.Sprintf("/repos/%s/%s/issues/%d", org, repo, number),
		exitCodes: []int{200},
	}, &i)
	return &i, err
}

// EditIssue will update the issue.
//
// See https://developer.github.com/v3/issues/#edit-an-issue
func (c *client) EditIssue(org, repo string, number int, issue *Issue) (*Issue, error) {
	durationLogger := c.log("EditIssue", org, repo, number)
	defer durationLogger()

	if c.dry {
		return issue, nil
	}
	edit := struct {
		Title string `json:"title,omitempty"`
		Body  string `json:"body,omitempty"`
		State string `json:"state,omitempty"`
	}{
		Title: issue.Title,
		Body:  issue.Body,
		State: issue.State,
	}
	var ret Issue
	_, err := c.request(&request{
		method:      http.MethodPatch,
		path:        fmt.Sprintf("/repos/%s/%s/issues/%d", org, repo, number),
		exitCodes:   []int{200},
		requestBody: &edit,
	}, &ret)
	if err != nil {
		return nil, err
	}
	return &ret, nil
}

// GetPullRequestPatch gets the patch version of a pull request.
//
// See https://developer.github.com/v3/media/#commits-commit-comparison-and-pull-requests
func (c *client) GetPullRequestPatch(org, repo string, number int) ([]byte, error) {
	durationLogger := c.log("GetPullRequestPatch", org, repo, number)
	defer durationLogger()

	_, patch, err := c.requestRaw(&request{
		accept:    "application/vnd.github.VERSION.patch",
		method:    http.MethodGet,
		path:      fmt.Sprintf("/repos/%s/%s/pulls/%d", org, repo, number),
		exitCodes: []int{200},
	})
	return patch, err
}

// CreatePullRequest creates a new pull request and returns its number if
// the creation is successful, otherwise any error that is encountered.
//
// See https://developer.github.com/v3/pulls/#create-a-pull-request
func (c *client) CreatePullRequest(org, repo, title, body, head, base string, canModify bool) (int, error) {
	durationLogger := c.log("CreatePullRequest", org, repo, title)
	defer durationLogger()

	data := struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		Head  string `json:"head"`
		Base  string `json:"base"`
		// MaintainerCanModify allows maintainers of the repo to modify this
		// pull request, eg. push changes to it before merging.
		MaintainerCanModify bool `json:"maintainer_can_modify"`
	}{
		Title: title,
		Body:  body,
		Head:  head,
		Base:  base,

		MaintainerCanModify: canModify,
	}
	var resp struct {
		Num int `json:"number"`
	}
	_, err := c.request(&request{
		// allow the description and draft fields
		// https://developer.github.com/changes/2018-02-22-label-description-search-preview/
		// https://developer.github.com/changes/2019-02-14-draft-pull-requests/
		accept:      "application/vnd.github.symmetra-preview+json, application/vnd.github.shadow-cat-preview",
		method:      http.MethodPost,
		path:        fmt.Sprintf("/repos/%s/%s/pulls", org, repo),
		requestBody: &data,
		exitCodes:   []int{201},
	}, &resp)
	if err != nil {
		return 0, err
	}
	return resp.Num, nil
}

// UpdatePullRequest modifies the title, body, open state
func (c *client) UpdatePullRequest(org, repo string, number int, title, body *string, open *bool, branch *string, canModify *bool) error {
	durationLogger := c.log("UpdatePullRequest", org, repo, title)
	defer durationLogger()

	data := struct {
		State *string `json:"state,omitempty"`
		Title *string `json:"title,omitempty"`
		Body  *string `json:"body,omitempty"`
		Base  *string `json:"base,omitempty"`
		// MaintainerCanModify allows maintainers of the repo to modify this
		// pull request, eg. push changes to it before merging.
		MaintainerCanModify *bool `json:"maintainer_can_modify,omitempty"`
	}{
		Title:               title,
		Body:                body,
		Base:                branch,
		MaintainerCanModify: canModify,
	}
	if open != nil && *open {
		op := "open"
		data.State = &op
	} else if open != nil {
		cl := "closed"
		data.State = &cl
	}
	_, err := c.request(&request{
		// allow the description and draft fields
		// https://developer.github.com/changes/2018-02-22-label-description-search-preview/
		// https://developer.github.com/changes/2019-02-14-draft-pull-requests/
		accept:      "application/vnd.github.symmetra-preview+json, application/vnd.github.shadow-cat-preview",
		method:      http.MethodPatch,
		path:        fmt.Sprintf("/repos/%s/%s/pulls/%d", org, repo, number),
		requestBody: &data,
		exitCodes:   []int{200},
	}, nil)
	return err
}

// GetPullRequestChanges gets a list of files modified in a pull request.
//
// See https://developer.github.com/v3/pulls/#list-pull-requests-files
func (c *client) GetPullRequestChanges(org, repo string, number int) ([]PullRequestChange, error) {
	durationLogger := c.log("GetPullRequestChanges", org, repo, number)
	defer durationLogger()

	if c.fake {
		return []PullRequestChange{}, nil
	}
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/files", org, repo, number)
	var changes []PullRequestChange
	err := c.readPaginatedResults(
		path,
		acceptNone,
		func() interface{} {
			return &[]PullRequestChange{}
		},
		func(obj interface{}) {
			changes = append(changes, *(obj.(*[]PullRequestChange))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return changes, nil
}

// ListPullRequestComments returns all *review* comments on a pull request.
//
// Multiple-pages of comments consumes multiple API tokens.
//
// See https://developer.github.com/v3/pulls/comments/#list-comments-on-a-pull-request
func (c *client) ListPullRequestComments(org, repo string, number int) ([]ReviewComment, error) {
	durationLogger := c.log("ListPullRequestComments", org, repo, number)
	defer durationLogger()

	if c.fake {
		return nil, nil
	}
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/comments", org, repo, number)
	var comments []ReviewComment
	err := c.readPaginatedResults(
		path,
		acceptNone,
		func() interface{} {
			return &[]ReviewComment{}
		},
		func(obj interface{}) {
			comments = append(comments, *(obj.(*[]ReviewComment))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return comments, nil
}

// ListReviews returns all reviews on a pull request.
//
// Multiple-pages of results consumes multiple API tokens.
//
// See https://developer.github.com/v3/pulls/reviews/#list-reviews-on-a-pull-request
func (c *client) ListReviews(org, repo string, number int) ([]Review, error) {
	durationLogger := c.log("ListReviews", org, repo, number)
	defer durationLogger()

	if c.fake {
		return nil, nil
	}
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", org, repo, number)
	var reviews []Review
	err := c.readPaginatedResults(
		path,
		acceptNone,
		func() interface{} {
			return &[]Review{}
		},
		func(obj interface{}) {
			reviews = append(reviews, *(obj.(*[]Review))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return reviews, nil
}

// CreateStatus creates or updates the status of a commit.
//
// See https://docs.github.com/en/free-pro-team@latest/rest/reference/repos#create-a-commit-status
func (c *client) CreateStatus(org, repo, SHA string, s Status) error {
	durationLogger := c.log("CreateStatus", org, repo, SHA, s)
	defer durationLogger()

	_, err := c.request(&request{
		method:      http.MethodPost,
		path:        fmt.Sprintf("/repos/%s/%s/statuses/%s", org, repo, SHA),
		requestBody: &s,
		exitCodes:   []int{201},
	}, nil)
	return err
}

// ListStatuses gets commit statuses for a given ref.
//
// See https://developer.github.com/v3/repos/statuses/#list-statuses-for-a-specific-ref
func (c *client) ListStatuses(org, repo, ref string) ([]Status, error) {
	durationLogger := c.log("ListStatuses", org, repo, ref)
	defer durationLogger()

	path := fmt.Sprintf("/repos/%s/%s/statuses/%s", org, repo, ref)
	var statuses []Status
	err := c.readPaginatedResults(
		path,
		acceptNone,
		func() interface{} {
			return &[]Status{}
		},
		func(obj interface{}) {
			statuses = append(statuses, *(obj.(*[]Status))...)
		},
	)
	return statuses, err
}

// GetRepo returns the repo for the provided owner/name combination.
//
// See https://developer.github.com/v3/repos/#get
func (c *client) GetRepo(owner, name string) (FullRepo, error) {
	durationLogger := c.log("GetRepo", owner, name)
	defer durationLogger()

	var repo FullRepo
	_, err := c.request(&request{
		method:    http.MethodGet,
		path:      fmt.Sprintf("/repos/%s/%s", owner, name),
		exitCodes: []int{200},
	}, &repo)
	return repo, err
}

// CreateRepo creates a new repository
// See https://developer.github.com/v3/repos/#create
func (c *client) CreateRepo(owner string, isUser bool, repo RepoCreateRequest) (*FullRepo, error) {
	durationLogger := c.log("CreateRepo", owner, isUser, repo)
	defer durationLogger()

	if repo.Name == nil || *repo.Name == "" {
		return nil, errors.New("repo.Name must be non-empty")
	}
	if c.fake {
		return nil, nil
	} else if c.dry {
		return repo.ToRepo(), nil
	}

	path := "/user/repos"
	if !isUser {
		path = fmt.Sprintf("/orgs/%s/repos", owner)
	}
	var retRepo FullRepo
	_, err := c.request(&request{
		method:      http.MethodPost,
		path:        path,
		requestBody: &repo,
		exitCodes:   []int{201},
	}, &retRepo)
	return &retRepo, err
}

// UpdateRepo edits an existing repository
// See https://developer.github.com/v3/repos/#edit
func (c *client) UpdateRepo(owner, name string, repo RepoUpdateRequest) (*FullRepo, error) {
	durationLogger := c.log("UpdateRepo", owner, name, repo)
	defer durationLogger()

	if c.fake {
		return nil, nil
	} else if c.dry {
		return repo.ToRepo(), nil
	}

	path := fmt.Sprintf("/repos/%s/%s", owner, name)
	var retRepo FullRepo
	_, err := c.request(&request{
		method:      http.MethodPatch,
		path:        path,
		requestBody: &repo,
		exitCodes:   []int{200},
	}, &retRepo)
	return &retRepo, err
}

// GetRepos returns all repos in an org.
//
// This call uses multiple API tokens when results are paginated.
//
// See https://developer.github.com/v3/repos/#list-organization-repositories
func (c *client) GetRepos(org string, isUser bool) ([]Repo, error) {
	durationLogger := c.log("GetRepos", org, isUser)
	defer durationLogger()

	var (
		repos   []Repo
		nextURL string
	)
	if c.fake {
		return repos, nil
	}
	if isUser {
		nextURL = fmt.Sprintf("/users/%s/repos", org)
	} else {
		nextURL = fmt.Sprintf("/orgs/%s/repos", org)
	}
	err := c.readPaginatedResults(
		nextURL,    // path
		acceptNone, // accept
		func() interface{} { // newObj
			return &[]Repo{}
		},
		func(obj interface{}) { // accumulate
			repos = append(repos, *(obj.(*[]Repo))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return repos, nil
}

// GetSingleCommit returns a single commit.
//
// See https://developer.github.com/v3/repos/#get
func (c *client) GetSingleCommit(org, repo, SHA string) (SingleCommit, error) {
	durationLogger := c.log("GetSingleCommit", org, repo, SHA)
	defer durationLogger()

	var commit SingleCommit
	_, err := c.request(&request{
		method:    http.MethodGet,
		path:      fmt.Sprintf("/repos/%s/%s/commits/%s", org, repo, SHA),
		exitCodes: []int{200},
	}, &commit)
	return commit, err
}

// GetBranches returns all branches in the repo.
//
// If onlyProtected is true it will only return repos with protection enabled,
// and branch.Protected will be true. Otherwise Protected is the default value (false).
//
// This call uses multiple API tokens when results are paginated.
//
// See https://developer.github.com/v3/repos/branches/#list-branches
func (c *client) GetBranches(org, repo string, onlyProtected bool) ([]Branch, error) {
	durationLogger := c.log("GetBranches", org, repo)
	defer durationLogger()

	var branches []Branch
	err := c.readPaginatedResultsWithValues(
		fmt.Sprintf("/repos/%s/%s/branches", org, repo),
		url.Values{
			"protected": []string{strconv.FormatBool(onlyProtected)},
			"per_page":  []string{"100"},
		},
		acceptNone,
		func() interface{} { // newObj
			return &[]Branch{}
		},
		func(obj interface{}) {
			branches = append(branches, *(obj.(*[]Branch))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return branches, nil
}

// GetBranchProtection returns current protection object for the branch
//
// See https://developer.github.com/v3/repos/branches/#get-branch-protection
func (c *client) GetBranchProtection(org, repo, branch string) (*BranchProtection, error) {
	durationLogger := c.log("GetBranchProtection", org, repo, branch)
	defer durationLogger()

	code, body, err := c.requestRaw(&request{
		method: http.MethodGet,
		path:   fmt.Sprintf("/repos/%s/%s/branches/%s/protection", org, repo, branch),
		// GitHub returns 404 for this call if either:
		// - The branch is not protected
		// - The access token used does not have sufficient privileges
		// We therefore need to introspect the response body.
		exitCodes: []int{200, 404},
	})

	switch {
	case err != nil:
		return nil, err
	case code == 200:
		var bp BranchProtection
		if err := json.Unmarshal(body, &bp); err != nil {
			return nil, err
		}
		return &bp, nil
	case code == 404:
		// continue
	default:
		return nil, fmt.Errorf("unexpected status code: %v", code)
	}

	var ge githubError
	if err := json.Unmarshal(body, &ge); err != nil {
		return nil, err
	}

	// If the error was because the branch is not protected, we return a
	// nil pointer to indicate this.
	if ge.Message == "Branch not protected" {
		return nil, nil
	}

	// Otherwise we got some other 404 error.
	return nil, fmt.Errorf("getting branch protection 404: %s", ge.Message)
}

// RemoveBranchProtection unprotects org/repo=branch.
//
// See https://developer.github.com/v3/repos/branches/#remove-branch-protection
func (c *client) RemoveBranchProtection(org, repo, branch string) error {
	durationLogger := c.log("RemoveBranchProtection", org, repo, branch)
	defer durationLogger()

	_, err := c.request(&request{
		method:    http.MethodDelete,
		path:      fmt.Sprintf("/repos/%s/%s/branches/%s/protection", org, repo, branch),
		exitCodes: []int{204},
	}, nil)
	return err
}

// UpdateBranchProtection configures org/repo=branch.
//
// See https://developer.github.com/v3/repos/branches/#update-branch-protection
func (c *client) UpdateBranchProtection(org, repo, branch string, config BranchProtectionRequest) error {
	durationLogger := c.log("UpdateBranchProtection", org, repo, branch, config)
	defer durationLogger()

	_, err := c.request(&request{
		accept:      "application/vnd.github.luke-cage-preview+json", // for required_approving_review_count
		method:      http.MethodPut,
		path:        fmt.Sprintf("/repos/%s/%s/branches/%s/protection", org, repo, branch),
		requestBody: config,
		exitCodes:   []int{200},
	}, nil)
	return err
}

// AddRepoLabel adds a defined label given org/repo
//
// See https://developer.github.com/v3/issues/labels/#create-a-label
func (c *client) AddRepoLabel(org, repo, label, description, color string) error {
	durationLogger := c.log("AddRepoLabel", org, repo, label, description, color)
	defer durationLogger()

	_, err := c.request(&request{
		method:      http.MethodPost,
		path:        fmt.Sprintf("/repos/%s/%s/labels", org, repo),
		accept:      "application/vnd.github.symmetra-preview+json", // allow the description field -- https://developer.github.com/changes/2018-02-22-label-description-search-preview/
		requestBody: Label{Name: label, Description: description, Color: color},
		exitCodes:   []int{201},
	}, nil)
	return err
}

// UpdateRepoLabel updates a org/repo label to new name, description, and color
//
// See https://developer.github.com/v3/issues/labels/#update-a-label
func (c *client) UpdateRepoLabel(org, repo, label, newName, description, color string) error {
	durationLogger := c.log("UpdateRepoLabel", org, repo, label, newName, color)
	defer durationLogger()

	_, err := c.request(&request{
		method:      http.MethodPatch,
		path:        fmt.Sprintf("/repos/%s/%s/labels/%s", org, repo, label),
		accept:      "application/vnd.github.symmetra-preview+json", // allow the description field -- https://developer.github.com/changes/2018-02-22-label-description-search-preview/
		requestBody: Label{Name: newName, Description: description, Color: color},
		exitCodes:   []int{200},
	}, nil)
	return err
}

// DeleteRepoLabel deletes a label in org/repo
//
// See https://developer.github.com/v3/issues/labels/#delete-a-label
func (c *client) DeleteRepoLabel(org, repo, label string) error {
	durationLogger := c.log("DeleteRepoLabel", org, repo, label)
	defer durationLogger()

	_, err := c.request(&request{
		method:      http.MethodDelete,
		accept:      "application/vnd.github.symmetra-preview+json", // allow the description field -- https://developer.github.com/changes/2018-02-22-label-description-search-preview/
		path:        fmt.Sprintf("/repos/%s/%s/labels/%s", org, repo, label),
		requestBody: Label{Name: label},
		exitCodes:   []int{204},
	}, nil)
	return err
}

// GetCombinedStatus returns the latest statuses for a given ref.
//
// See https://developer.github.com/v3/repos/statuses/#get-the-combined-status-for-a-specific-ref
func (c *client) GetCombinedStatus(org, repo, ref string) (*CombinedStatus, error) {
	durationLogger := c.log("GetCombinedStatus", org, repo, ref)
	defer durationLogger()

	var combinedStatus CombinedStatus
	err := c.readPaginatedResults(
		fmt.Sprintf("/repos/%s/%s/commits/%s/status", org, repo, ref),
		"",
		func() interface{} {
			return &CombinedStatus{}
		},
		func(obj interface{}) {
			cs := *(obj.(*CombinedStatus))
			cs.Statuses = append(combinedStatus.Statuses, cs.Statuses...)
			combinedStatus = cs
		},
	)
	return &combinedStatus, err
}

// getLabels is a helper function that retrieves a paginated list of labels from a github URI path.
func (c *client) getLabels(path string) ([]Label, error) {
	var labels []Label
	if c.fake {
		return labels, nil
	}
	err := c.readPaginatedResults(
		path,
		"application/vnd.github.symmetra-preview+json", // allow the description field -- https://developer.github.com/changes/2018-02-22-label-description-search-preview/
		func() interface{} {
			return &[]Label{}
		},
		func(obj interface{}) {
			labels = append(labels, *(obj.(*[]Label))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return labels, nil
}

// GetRepoLabels returns the list of labels accessible to org/repo.
//
// See https://developer.github.com/v3/issues/labels/#list-all-labels-for-this-repository
func (c *client) GetRepoLabels(org, repo string) ([]Label, error) {
	durationLogger := c.log("GetRepoLabels", org, repo)
	defer durationLogger()

	return c.getLabels(fmt.Sprintf("/repos/%s/%s/labels", org, repo))
}

// GetIssueLabels returns the list of labels currently on issue org/repo#number.
//
// See https://developer.github.com/v3/issues/labels/#list-labels-on-an-issue
func (c *client) GetIssueLabels(org, repo string, number int) ([]Label, error) {
	durationLogger := c.log("GetIssueLabels", org, repo, number)
	defer durationLogger()

	return c.getLabels(fmt.Sprintf("/repos/%s/%s/issues/%d/labels", org, repo, number))
}

// AddLabel adds label to org/repo#number, returning an error on a bad response code.
//
// See https://developer.github.com/v3/issues/labels/#add-labels-to-an-issue
func (c *client) AddLabel(org, repo string, number int, label string) error {
	durationLogger := c.log("AddLabel", org, repo, number, label)
	defer durationLogger()

	_, err := c.request(&request{
		method:      http.MethodPost,
		path:        fmt.Sprintf("/repos/%s/%s/issues/%d/labels", org, repo, number),
		requestBody: []string{label},
		exitCodes:   []int{200},
	}, nil)
	return err
}

type githubError struct {
	Message string `json:"message,omitempty"`
}

// RemoveLabel removes label from org/repo#number, returning an error on any failure.
//
// See https://developer.github.com/v3/issues/labels/#remove-a-label-from-an-issue
func (c *client) RemoveLabel(org, repo string, number int, label string) error {
	durationLogger := c.log("RemoveLabel", org, repo, number, label)
	defer durationLogger()

	code, body, err := c.requestRaw(&request{
		method: http.MethodDelete,
		path:   fmt.Sprintf("/repos/%s/%s/issues/%d/labels/%s", org, repo, number, label),
		// GitHub sometimes returns 200 for this call, which is a bug on their end.
		// Do not expect a 404 exit code and handle it separately because we need
		// to introspect the request's response body.
		exitCodes: []int{200, 204},
	})

	switch {
	case code == 200 || code == 204:
		// If our code was 200 or 204, no error info.
		return nil
	case code == 404:
		// continue
	case err != nil:
		return err
	default:
		return fmt.Errorf("unexpected status code: %v", code)
	}

	ge := &githubError{}
	if err := json.Unmarshal(body, ge); err != nil {
		return err
	}

	// If the error was because the label was not found, we don't really
	// care since the label won't exist anyway.
	if ge.Message == "Label does not exist" {
		return nil
	}

	// Otherwise we got some other 404 error.
	return fmt.Errorf("deleting label 404: %s", ge.Message)
}

// MissingUsers is an error specifying the users that could not be unassigned.
type MissingUsers struct {
	Users  []string
	action string
}

func (m MissingUsers) Error() string {
	return fmt.Sprintf("could not %s the following user(s): %s.", m.action, strings.Join(m.Users, ", "))
}

// AssignIssue adds logins to org/repo#number, returning an error if any login is missing after making the call.
//
// See https://developer.github.com/v3/issues/assignees/#add-assignees-to-an-issue
func (c *client) AssignIssue(org, repo string, number int, logins []string) error {
	durationLogger := c.log("AssignIssue", org, repo, number, logins)
	defer durationLogger()

	assigned := make(map[string]bool)
	var i Issue
	_, err := c.request(&request{
		method:      http.MethodPost,
		path:        fmt.Sprintf("/repos/%s/%s/issues/%d/assignees", org, repo, number),
		requestBody: map[string][]string{"assignees": logins},
		exitCodes:   []int{201},
	}, &i)
	if err != nil {
		return err
	}
	for _, assignee := range i.Assignees {
		assigned[NormLogin(assignee.Login)] = true
	}
	missing := MissingUsers{action: "assign"}
	for _, login := range logins {
		if !assigned[NormLogin(login)] {
			missing.Users = append(missing.Users, login)
		}
	}
	if len(missing.Users) > 0 {
		return missing
	}
	return nil
}

// ExtraUsers is an error specifying the users that could not be unassigned.
type ExtraUsers struct {
	Users  []string
	action string
}

func (e ExtraUsers) Error() string {
	return fmt.Sprintf("could not %s the following user(s): %s.", e.action, strings.Join(e.Users, ", "))
}

// UnassignIssue removes logins from org/repo#number, returns an error if any login remains assigned.
//
// See https://developer.github.com/v3/issues/assignees/#remove-assignees-from-an-issue
func (c *client) UnassignIssue(org, repo string, number int, logins []string) error {
	durationLogger := c.log("UnassignIssue", org, repo, number, logins)
	defer durationLogger()

	assigned := make(map[string]bool)
	var i Issue
	_, err := c.request(&request{
		method:      http.MethodDelete,
		path:        fmt.Sprintf("/repos/%s/%s/issues/%d/assignees", org, repo, number),
		requestBody: map[string][]string{"assignees": logins},
		exitCodes:   []int{200},
	}, &i)
	if err != nil {
		return err
	}
	for _, assignee := range i.Assignees {
		assigned[NormLogin(assignee.Login)] = true
	}
	extra := ExtraUsers{action: "unassign"}
	for _, login := range logins {
		if assigned[NormLogin(login)] {
			extra.Users = append(extra.Users, login)
		}
	}
	if len(extra.Users) > 0 {
		return extra
	}
	return nil
}

// CreateReview creates a review using the draft.
//
// https://developer.github.com/v3/pulls/reviews/#create-a-pull-request-review
func (c *client) CreateReview(org, repo string, number int, r DraftReview) error {
	durationLogger := c.log("CreateReview", org, repo, number, r)
	defer durationLogger()

	_, err := c.request(&request{
		method:      http.MethodPost,
		path:        fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", org, repo, number),
		accept:      "application/vnd.github.black-cat-preview+json",
		requestBody: r,
		exitCodes:   []int{200},
	}, nil)
	return err
}

// prepareReviewersBody separates reviewers from team_reviewers and prepares a map
// {
//   "reviewers": [
//     "octocat",
//     "hubot",
//     "other_user"
//   ],
//   "team_reviewers": [
//     "justice-league"
//   ]
// }
//
// https://developer.github.com/v3/pulls/review_requests/#create-a-review-request
func prepareReviewersBody(logins []string, org string) (map[string][]string, error) {
	body := map[string][]string{}
	var errors []error
	for _, login := range logins {
		mat := teamRe.FindStringSubmatch(login)
		if mat == nil {
			if _, exists := body["reviewers"]; !exists {
				body["reviewers"] = []string{}
			}
			body["reviewers"] = append(body["reviewers"], login)
		} else if mat[1] == org {
			if _, exists := body["team_reviewers"]; !exists {
				body["team_reviewers"] = []string{}
			}
			body["team_reviewers"] = append(body["team_reviewers"], mat[2])
		} else {
			errors = append(errors, fmt.Errorf("team %s is not part of %s org", login, org))
		}
	}
	return body, utilerrors.NewAggregate(errors)
}

func (c *client) tryRequestReview(org, repo string, number int, logins []string) (int, error) {
	durationLogger := c.log("RequestReview", org, repo, number, logins)
	defer durationLogger()

	var pr PullRequest
	body, err := prepareReviewersBody(logins, org)
	if err != nil {
		// At least one team not in org,
		// let RequestReview handle retries and alerting for each login.
		return http.StatusUnprocessableEntity, err
	}
	return c.request(&request{
		method:      http.MethodPost,
		path:        fmt.Sprintf("/repos/%s/%s/pulls/%d/requested_reviewers", org, repo, number),
		accept:      "application/vnd.github.symmetra-preview+json",
		requestBody: body,
		exitCodes:   []int{http.StatusCreated /*201*/},
	}, &pr)
}

// RequestReview tries to add the users listed in 'logins' as requested reviewers of the specified PR.
// If any user in the 'logins' slice is not a contributor of the repo, the entire POST will fail
// without adding any reviewers. The GitHub API response does not specify which user(s) were invalid
// so if we fail to request reviews from the members of 'logins' we try to request reviews from
// each member individually. We try first with all users in 'logins' for efficiency in the common case.
//
// See https://developer.github.com/v3/pulls/review_requests/#create-a-review-request
func (c *client) RequestReview(org, repo string, number int, logins []string) error {
	statusCode, err := c.tryRequestReview(org, repo, number, logins)
	if err != nil && statusCode == http.StatusUnprocessableEntity /*422*/ {
		// Failed to set all members of 'logins' as reviewers, try individually.
		missing := MissingUsers{action: "request a PR review from"}
		for _, user := range logins {
			statusCode, err = c.tryRequestReview(org, repo, number, []string{user})
			if err != nil && statusCode == http.StatusUnprocessableEntity /*422*/ {
				// User is not a contributor, or team not in org.
				missing.Users = append(missing.Users, user)
			} else if err != nil {
				return fmt.Errorf("failed to add reviewer to PR. Status code: %d, errmsg: %v", statusCode, err)
			}
		}
		if len(missing.Users) > 0 {
			return missing
		}
		return nil
	}
	return err
}

// UnrequestReview tries to remove the users listed in 'logins' from the requested reviewers of the
// specified PR. The GitHub API treats deletions of review requests differently than creations. Specifically, if
// 'logins' contains a user that isn't a requested reviewer, other users that are valid are still removed.
// Furthermore, the API response lists the set of requested reviewers after the deletion (unlike request creations),
// so we can determine if each deletion was successful.
// The API responds with http status code 200 no matter what the content of 'logins' is.
//
// See https://developer.github.com/v3/pulls/review_requests/#delete-a-review-request
func (c *client) UnrequestReview(org, repo string, number int, logins []string) error {
	durationLogger := c.log("UnrequestReview", org, repo, number, logins)
	defer durationLogger()

	var pr PullRequest
	body, err := prepareReviewersBody(logins, org)
	if len(body) == 0 {
		// No point in doing request for none,
		// if some logins didn't make it to body, extras.Users will catch them.
		return err
	}
	_, err = c.request(&request{
		method:      http.MethodDelete,
		path:        fmt.Sprintf("/repos/%s/%s/pulls/%d/requested_reviewers", org, repo, number),
		accept:      "application/vnd.github.symmetra-preview+json",
		requestBody: body,
		exitCodes:   []int{http.StatusOK /*200*/},
	}, &pr)
	if err != nil {
		return err
	}
	extras := ExtraUsers{action: "remove the PR review request for"}
	for _, user := range pr.RequestedReviewers {
		found := false
		for _, toDelete := range logins {
			if NormLogin(user.Login) == NormLogin(toDelete) {
				found = true
				break
			}
		}
		if found {
			extras.Users = append(extras.Users, user.Login)
		}
	}
	if len(extras.Users) > 0 {
		return extras
	}
	return nil
}

// CloseIssue closes the existing, open issue provided
//
// See https://developer.github.com/v3/issues/#edit-an-issue
func (c *client) CloseIssue(org, repo string, number int) error {
	durationLogger := c.log("CloseIssue", org, repo, number)
	defer durationLogger()

	_, err := c.request(&request{
		method:      http.MethodPatch,
		path:        fmt.Sprintf("/repos/%s/%s/issues/%d", org, repo, number),
		requestBody: map[string]string{"state": "closed"},
		exitCodes:   []int{200},
	}, nil)
	return err
}

// StateCannotBeChanged represents the "custom" GitHub API
// error that occurs when a resource cannot be changed
type StateCannotBeChanged struct {
	Message string
}

func (s StateCannotBeChanged) Error() string {
	return s.Message
}

// StateCannotBeChanged implements error
var _ error = (*StateCannotBeChanged)(nil)

// convert to a StateCannotBeChanged if appropriate or else return the original error
func stateCannotBeChangedOrOriginalError(err error) error {
	requestErr, ok := err.(requestError)
	if ok {
		for _, errorMsg := range requestErr.ErrorMessages() {
			if strings.Contains(errorMsg, stateCannotBeChangedMessagePrefix) {
				return StateCannotBeChanged{
					Message: errorMsg,
				}
			}
		}
	}
	return err
}

// ReopenIssue re-opens the existing, closed issue provided
//
// See https://developer.github.com/v3/issues/#edit-an-issue
func (c *client) ReopenIssue(org, repo string, number int) error {
	durationLogger := c.log("ReopenIssue", org, repo, number)
	defer durationLogger()

	_, err := c.request(&request{
		method:      http.MethodPatch,
		path:        fmt.Sprintf("/repos/%s/%s/issues/%d", org, repo, number),
		requestBody: map[string]string{"state": "open"},
		exitCodes:   []int{200},
	}, nil)
	return stateCannotBeChangedOrOriginalError(err)
}

// ClosePR closes the existing, open PR provided
// TODO: Rename to ClosePullRequest
//
// See https://developer.github.com/v3/pulls/#update-a-pull-request
func (c *client) ClosePR(org, repo string, number int) error {
	durationLogger := c.log("ClosePR", org, repo, number)
	defer durationLogger()

	_, err := c.request(&request{
		method:      http.MethodPatch,
		path:        fmt.Sprintf("/repos/%s/%s/pulls/%d", org, repo, number),
		requestBody: map[string]string{"state": "closed"},
		exitCodes:   []int{200},
	}, nil)
	return err
}

// ReopenPR re-opens the existing, closed PR provided
// TODO: Rename to ReopenPullRequest
//
// See https://developer.github.com/v3/pulls/#update-a-pull-request
func (c *client) ReopenPR(org, repo string, number int) error {
	durationLogger := c.log("ReopenPR", org, repo, number)
	defer durationLogger()

	_, err := c.request(&request{
		method:      http.MethodPatch,
		path:        fmt.Sprintf("/repos/%s/%s/pulls/%d", org, repo, number),
		requestBody: map[string]string{"state": "open"},
		exitCodes:   []int{200},
	}, nil)
	return stateCannotBeChangedOrOriginalError(err)
}

// GetRef returns the SHA of the given ref, such as "heads/master".
//
// See https://developer.github.com/v3/git/refs/#get-a-reference
// The gitbub api does prefix matching and might return multiple results,
// in which case we will return a GetRefTooManyResultsError
func (c *client) GetRef(org, repo, ref string) (string, error) {
	durationLogger := c.log("GetRef", org, repo, ref)
	defer durationLogger()

	res := GetRefResponse{}
	_, err := c.request(&request{
		method:    http.MethodGet,
		path:      fmt.Sprintf("/repos/%s/%s/git/refs/%s", org, repo, ref),
		exitCodes: []int{200},
	}, &res)
	if err != nil {
		return "", nil
	}

	if n := len(res); n > 1 {
		wantRef := "refs/" + ref
		for _, r := range res {
			if r.Ref == wantRef {
				return r.Object.SHA, nil
			}
		}
		return "", GetRefTooManyResultsError{org: org, repo: repo, ref: ref, resultsRefs: res.RefNames()}
	}
	return res[0].Object.SHA, nil
}

type GetRefTooManyResultsError struct {
	org, repo, ref string
	resultsRefs    []string
}

func (GetRefTooManyResultsError) Is(err error) bool {
	_, ok := err.(GetRefTooManyResultsError)
	return ok
}

func (e GetRefTooManyResultsError) Error() string {
	return fmt.Sprintf("query for %s/%s ref %q didn't match one but multiple refs: %v", e.org, e.repo, e.ref, e.resultsRefs)
}

type GetRefResponse []GetRefResult

// We need a custom unmarshaler because the GetRefResult may either be a
// single GetRefResponse or multiple
func (grr *GetRefResponse) UnmarshalJSON(data []byte) error {
	result := &GetRefResult{}
	if err := json.Unmarshal(data, result); err == nil {
		*(grr) = GetRefResponse{*result}
		return nil
	}
	var response []GetRefResult
	if err := json.Unmarshal(data, &response); err != nil {
		return fmt.Errorf("failed to unmarshal response %s: %w", string(data), err)
	}
	*grr = GetRefResponse(response)
	return nil
}

func (grr *GetRefResponse) RefNames() []string {
	var result []string
	for _, item := range *grr {
		result = append(result, item.Ref)
	}
	return result
}

type GetRefResult struct {
	Ref    string `json:"ref,omitempty"`
	NodeID string `json:"node_id,omitempty"`
	URL    string `json:"url,omitempty"`
	Object struct {
		Type string `json:"type,omitempty"`
		SHA  string `json:"sha,omitempty"`
		URL  string `json:"url,omitempty"`
	} `json:"object,omitempty"`
}

// DeleteRef deletes the given ref
//
// See https://developer.github.com/v3/git/refs/#delete-a-reference
func (c *client) DeleteRef(org, repo, ref string) error {
	durationLogger := c.log("DeleteRef", org, repo, ref)
	defer durationLogger()

	_, err := c.request(&request{
		method:    http.MethodDelete,
		path:      fmt.Sprintf("/repos/%s/%s/git/refs/%s", org, repo, ref),
		exitCodes: []int{204},
	}, nil)
	return err
}

// FindIssues uses the GitHub search API to find issues which match a particular query.
//
// Input query the same way you would into the website.
// Order returned results with sort (usually "updated").
// Control whether oldest/newest is first with asc.
//
// See https://help.github.com/articles/searching-issues-and-pull-requests/ for details.
func (c *client) FindIssues(query, sort string, asc bool) ([]Issue, error) {
	durationLogger := c.log("FindIssues", query)
	defer durationLogger()

	path := fmt.Sprintf("/search/issues?q=%s", url.QueryEscape(query))
	if sort != "" {
		path += "&sort=" + url.QueryEscape(sort)
		if asc {
			path += "&order=asc"
		}
	}
	var issSearchResult IssuesSearchResult
	_, err := c.request(&request{
		method:    http.MethodGet,
		path:      path,
		exitCodes: []int{200},
	}, &issSearchResult)
	return issSearchResult.Issues, err
}

// FileNotFound happens when github cannot find the file requested by GetFile().
type FileNotFound struct {
	org, repo, path, commit string
}

func (e *FileNotFound) Error() string {
	return fmt.Sprintf("%s/%s/%s @ %s not found", e.org, e.repo, e.path, e.commit)
}

// GetFile uses GitHub repo contents API to retrieve the content of a file with commit SHA.
// If commit is empty, it will grab content from repo's default branch, usually master.
// TODO(krzyzacy): Support retrieve a directory
//
// See https://developer.github.com/v3/repos/contents/#get-contents
func (c *client) GetFile(org, repo, filepath, commit string) ([]byte, error) {
	durationLogger := c.log("GetFile", org, repo, filepath, commit)
	defer durationLogger()

	url := fmt.Sprintf("/repos/%s/%s/contents/%s", org, repo, filepath)
	if commit != "" {
		url = fmt.Sprintf("%s?ref=%s", url, commit)
	}

	var res Content
	code, err := c.request(&request{
		method:    http.MethodGet,
		path:      url,
		exitCodes: []int{200, 404},
	}, &res)

	if err != nil {
		return nil, err
	}

	if code == 404 {
		return nil, &FileNotFound{
			org:    org,
			repo:   repo,
			path:   filepath,
			commit: commit,
		}
	}

	decoded, err := base64.StdEncoding.DecodeString(res.Content)
	if err != nil {
		return nil, fmt.Errorf("error decoding %s : %v", res.Content, err)
	}

	return decoded, nil
}

// Query runs a GraphQL query using shurcooL/githubql's client.
func (c *client) Query(ctx context.Context, q interface{}, vars map[string]interface{}) error {
	// Don't log query here because Query is typically called multiple times to get all pages.
	// Instead log once per search and include total search cost.
	return c.gqlc.Query(ctx, q, vars)
}

// CreateTeam adds a team with name to the org, returning a struct with the new ID.
//
// See https://developer.github.com/v3/teams/#create-team
func (c *client) CreateTeam(org string, team Team) (*Team, error) {
	durationLogger := c.log("CreateTeam", org, team)
	defer durationLogger()

	if team.Name == "" {
		return nil, errors.New("team.Name must be non-empty")
	}
	if c.fake {
		return nil, nil
	} else if c.dry {
		return &team, nil
	}
	path := fmt.Sprintf("/orgs/%s/teams", org)
	var retTeam Team
	_, err := c.request(&request{
		method: http.MethodPost,
		path:   path,
		// This accept header enables the nested teams preview.
		// https://developer.github.com/changes/2017-08-30-preview-nested-teams/
		accept:      "application/vnd.github.hellcat-preview+json",
		requestBody: &team,
		exitCodes:   []int{201},
	}, &retTeam)
	return &retTeam, err
}

// EditTeam patches team.ID to contain the specified other values.
//
// See https://developer.github.com/v3/teams/#edit-team
func (c *client) EditTeam(org string, t Team) (*Team, error) {
	durationLogger := c.log("EditTeam", t)
	defer durationLogger()

	if t.ID == 0 {
		return nil, errors.New("team.ID must be non-zero")
	}
	if c.dry {
		return &t, nil
	}
	id := t.ID
	t.ID = 0
	// Need to send parent_team_id: null
	team := struct {
		Team
		ParentTeamID *int `json:"parent_team_id"`
	}{
		Team:         t,
		ParentTeamID: t.ParentTeamID,
	}
	var retTeam Team
	path := fmt.Sprintf("/teams/%d", id)
	_, err := c.request(&request{
		method: http.MethodPatch,
		path:   path,
		// This accept header enables the nested teams preview.
		// https://developer.github.com/changes/2017-08-30-preview-nested-teams/
		accept:      "application/vnd.github.hellcat-preview+json",
		requestBody: &team,
		exitCodes:   []int{200, 201},
	}, &retTeam)
	return &retTeam, err
}

// DeleteTeam removes team.ID from GitHub.
//
// See https://developer.github.com/v3/teams/#delete-team
func (c *client) DeleteTeam(org string, id int) error {
	durationLogger := c.log("DeleteTeam", id)
	defer durationLogger()
	path := fmt.Sprintf("/teams/%d", id)
	_, err := c.request(&request{
		method:    http.MethodDelete,
		path:      path,
		exitCodes: []int{204},
	}, nil)
	return err
}

// ListTeams gets a list of teams for the given org
//
// See https://developer.github.com/v3/teams/#list-teams
func (c *client) ListTeams(org string) ([]Team, error) {
	durationLogger := c.log("ListTeams", org)
	defer durationLogger()

	if c.fake {
		return nil, nil
	}
	path := fmt.Sprintf("/orgs/%s/teams", org)
	var teams []Team
	err := c.readPaginatedResults(
		path,
		// This accept header enables the nested teams preview.
		// https://developer.github.com/changes/2017-08-30-preview-nested-teams/
		"application/vnd.github.hellcat-preview+json",
		func() interface{} {
			return &[]Team{}
		},
		func(obj interface{}) {
			teams = append(teams, *(obj.(*[]Team))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return teams, nil
}

// UpdateTeamMembership adds the user to the team and/or updates their role in that team.
//
// If the user is not a member of the org, GitHub will invite them to become an outside collaborator, setting their status to pending.
//
// https://developer.github.com/v3/teams/members/#add-or-update-team-membership
func (c *client) UpdateTeamMembership(org string, id int, user string, maintainer bool) (*TeamMembership, error) {
	durationLogger := c.log("UpdateTeamMembership", id, user, maintainer)
	defer durationLogger()

	if c.fake {
		return nil, nil
	}
	tm := TeamMembership{}
	if maintainer {
		tm.Role = RoleMaintainer
	} else {
		tm.Role = RoleMember
	}

	if c.dry {
		return &tm, nil
	}

	_, err := c.request(&request{
		method:      http.MethodPut,
		path:        fmt.Sprintf("/teams/%d/memberships/%s", id, user),
		requestBody: &tm,
		exitCodes:   []int{200},
	}, &tm)
	return &tm, err
}

// RemoveTeamMembership removes the user from the team (but not the org).
//
// https://developer.github.com/v3/teams/members/#remove-team-member
func (c *client) RemoveTeamMembership(org string, id int, user string) error {
	durationLogger := c.log("RemoveTeamMembership", id, user)
	defer durationLogger()

	if c.fake {
		return nil
	}
	_, err := c.request(&request{
		method:    http.MethodDelete,
		path:      fmt.Sprintf("/teams/%d/memberships/%s", id, user),
		exitCodes: []int{204},
	}, nil)
	return err
}

// ListTeamMembers gets a list of team members for the given team id
//
// Role options are "all", "maintainer" and "member"
//
// https://developer.github.com/v3/teams/members/#list-team-members
func (c *client) ListTeamMembers(org string, id int, role string) ([]TeamMember, error) {
	durationLogger := c.log("ListTeamMembers", id, role)
	defer durationLogger()

	if c.fake {
		return nil, nil
	}
	path := fmt.Sprintf("/teams/%d/members", id)
	var teamMembers []TeamMember
	err := c.readPaginatedResultsWithValues(
		path,
		url.Values{
			"per_page": []string{"100"},
			"role":     []string{role},
		},
		// This accept header enables the nested teams preview.
		// https://developer.github.com/changes/2017-08-30-preview-nested-teams/
		"application/vnd.github.hellcat-preview+json",
		func() interface{} {
			return &[]TeamMember{}
		},
		func(obj interface{}) {
			teamMembers = append(teamMembers, *(obj.(*[]TeamMember))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return teamMembers, nil
}

// ListTeamRepos gets a list of team repos for the given team id
//
// https://developer.github.com/v3/teams/#list-team-repos
func (c *client) ListTeamRepos(org string, id int) ([]Repo, error) {
	durationLogger := c.log("ListTeamRepos", id)
	defer durationLogger()

	if c.fake {
		return nil, nil
	}
	path := fmt.Sprintf("/teams/%d/repos", id)
	var repos []Repo
	err := c.readPaginatedResultsWithValues(
		path,
		url.Values{
			"per_page": []string{"100"},
		},
		// This accept header enables the nested teams preview.
		// https://developer.github.com/changes/2017-08-30-preview-nested-teams/
		"application/vnd.github.hellcat-preview+json",
		func() interface{} {
			return &[]Repo{}
		},
		func(obj interface{}) {
			for _, repo := range *obj.(*[]Repo) {
				// Currently, GitHub API returns false for all permission levels
				// for a repo on which the team has 'Maintain' or 'Triage' role.
				// This check is to avoid listing a repo under the team but
				// showing the permission level as none.
				if LevelFromPermissions(repo.Permissions) != None {
					repos = append(repos, repo)
				}
			}
		},
	)
	if err != nil {
		return nil, err
	}
	return repos, nil
}

// UpdateTeamRepo adds the repo to the team with the provided role.
//
// https://developer.github.com/v3/teams/#add-or-update-team-repository
func (c *client) UpdateTeamRepo(id int, org, repo string, permission TeamPermission) error {
	durationLogger := c.log("UpdateTeamRepo", id, org, repo, permission)
	defer durationLogger()

	if c.fake || c.dry {
		return nil
	}

	data := struct {
		Permission string `json:"permission"`
	}{
		Permission: string(permission),
	}

	_, err := c.request(&request{
		method:      http.MethodPut,
		path:        fmt.Sprintf("/teams/%d/repos/%s/%s", id, org, repo),
		requestBody: &data,
		exitCodes:   []int{204},
	}, nil)
	return err
}

// RemoveTeamRepo removes the team from the repo.
//
// https://developer.github.com/v3/teams/#add-or-update-team-repository
func (c *client) RemoveTeamRepo(id int, org, repo string) error {
	durationLogger := c.log("RemoveTeamRepo", id, org, repo)
	defer durationLogger()

	if c.fake || c.dry {
		return nil
	}

	_, err := c.request(&request{
		method:    http.MethodDelete,
		path:      fmt.Sprintf("/teams/%d/repos/%s/%s", id, org, repo),
		exitCodes: []int{204},
	}, nil)
	return err
}

// ListTeamInvitations gets a list of team members with pending invitations for the
// given team id
//
// https://developer.github.com/v3/teams/members/#list-pending-team-invitations
func (c *client) ListTeamInvitations(org string, id int) ([]OrgInvitation, error) {
	durationLogger := c.log("ListTeamInvites", id)
	defer durationLogger()

	if c.fake {
		return nil, nil
	}
	path := fmt.Sprintf("/teams/%d/invitations", id)
	var ret []OrgInvitation
	err := c.readPaginatedResults(
		path,
		acceptNone,
		func() interface{} {
			return &[]OrgInvitation{}
		},
		func(obj interface{}) {
			ret = append(ret, *(obj.(*[]OrgInvitation))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// MergeDetails contains desired properties of the merge.
//
// See https://developer.github.com/v3/pulls/#merge-a-pull-request-merge-button
type MergeDetails struct {
	// CommitTitle defaults to the automatic message.
	CommitTitle string `json:"commit_title,omitempty"`
	// CommitMessage defaults to the automatic message.
	CommitMessage string `json:"commit_message,omitempty"`
	// The PR HEAD must match this to prevent races.
	SHA string `json:"sha,omitempty"`
	// Can be "merge", "squash", or "rebase". Defaults to merge.
	MergeMethod string `json:"merge_method,omitempty"`
}

// ModifiedHeadError happens when github refuses to merge a PR because the PR changed.
type ModifiedHeadError string

func (e ModifiedHeadError) Error() string { return string(e) }

// UnmergablePRError happens when github refuses to merge a PR for other reasons (merge confclit).
type UnmergablePRError string

func (e UnmergablePRError) Error() string { return string(e) }

// UnmergablePRBaseChangedError happens when github refuses merging a PR because the base changed.
type UnmergablePRBaseChangedError string

func (e UnmergablePRBaseChangedError) Error() string { return string(e) }

// UnauthorizedToPushError happens when client is not allowed to push to github.
type UnauthorizedToPushError string

func (e UnauthorizedToPushError) Error() string { return string(e) }

// MergeCommitsForbiddenError happens when the repo disallows the merge strategy configured for the repo in Tide.
type MergeCommitsForbiddenError string

func (e MergeCommitsForbiddenError) Error() string { return string(e) }

// Merge merges a PR.
//
// See https://developer.github.com/v3/pulls/#merge-a-pull-request-merge-button
func (c *client) Merge(org, repo string, pr int, details MergeDetails) error {
	durationLogger := c.log("Merge", org, repo, pr, details)
	defer durationLogger()

	ge := githubError{}
	ec, err := c.request(&request{
		method:      http.MethodPut,
		path:        fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", org, repo, pr),
		requestBody: &details,
		exitCodes:   []int{200, 405, 409},
	}, &ge)
	if err != nil {
		return err
	}
	if ec == 405 {
		if strings.Contains(ge.Message, "Base branch was modified") {
			return UnmergablePRBaseChangedError(ge.Message)
		}
		if strings.Contains(ge.Message, "You're not authorized to push to this branch") {
			return UnauthorizedToPushError(ge.Message)
		}
		if strings.Contains(ge.Message, "Merge commits are not allowed on this repository") {
			return MergeCommitsForbiddenError(ge.Message)
		}
		return UnmergablePRError(ge.Message)
	} else if ec == 409 {
		return ModifiedHeadError(ge.Message)
	}

	return nil
}

// IsCollaborator returns whether or not the user is a collaborator of the repo.
// From GitHub's API reference:
// For organization-owned repositories, the list of collaborators includes
// outside collaborators, organization members that are direct collaborators,
// organization members with access through team memberships, organization
// members with access through default organization permissions, and
// organization owners.
//
// See https://developer.github.com/v3/repos/collaborators/
func (c *client) IsCollaborator(org, repo, user string) (bool, error) {
	// This call does not support etags and is therefore not cacheable today
	// by ghproxy. If we can detect that we're using ghproxy, however, we can
	// make a more expensive but cache-able call instead. Detecting that we
	// are pointed at a ghproxy instance is not high fidelity, but a best-effort
	// approach here is guaranteed to make a positive impact and no negative one.
	if strings.Contains(c.bases[0], "ghproxy") {
		users, err := c.ListCollaborators(org, repo)
		if err != nil {
			return false, err
		}
		for _, u := range users {
			if NormLogin(u.Login) == NormLogin(user) {
				return true, nil
			}
		}
		return false, nil
	}
	durationLogger := c.log("IsCollaborator", org, user)
	defer durationLogger()

	if org == user {
		// Make it possible to run a couple of plugins on personal repos.
		return true, nil
	}
	code, err := c.request(&request{
		method: http.MethodGet,
		// This accept header enables the nested teams preview.
		// https://developer.github.com/changes/2017-08-30-preview-nested-teams/
		accept:    "application/vnd.github.hellcat-preview+json",
		path:      fmt.Sprintf("/repos/%s/%s/collaborators/%s", org, repo, user),
		exitCodes: []int{204, 404, 302},
	}, nil)
	if err != nil {
		return false, err
	}
	if code == 204 {
		return true, nil
	} else if code == 404 {
		return false, nil
	}
	return false, fmt.Errorf("unexpected status: %d", code)
}

// ListCollaborators gets a list of all users who have access to a repo (and
// can become assignees or requested reviewers).
//
// See 'IsCollaborator' for more details.
// See https://developer.github.com/v3/repos/collaborators/
func (c *client) ListCollaborators(org, repo string) ([]User, error) {
	durationLogger := c.log("ListCollaborators", org, repo)
	defer durationLogger()

	if c.fake {
		return nil, nil
	}
	path := fmt.Sprintf("/repos/%s/%s/collaborators", org, repo)
	var users []User
	err := c.readPaginatedResults(
		path,
		// This accept header enables the nested teams preview.
		// https://developer.github.com/changes/2017-08-30-preview-nested-teams/
		"application/vnd.github.hellcat-preview+json",
		func() interface{} {
			return &[]User{}
		},
		func(obj interface{}) {
			users = append(users, *(obj.(*[]User))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return users, nil
}

// CreateFork creates a fork for the authenticated user. Forking a repository
// happens asynchronously. Therefore, we may have to wait a short period before
// accessing the git objects. If this takes longer than 5 minutes, GitHub
// recommends contacting their support.
//
// See https://developer.github.com/v3/repos/forks/#create-a-fork
func (c *client) CreateFork(owner, repo string) (string, error) {
	durationLogger := c.log("CreateFork", owner, repo)
	defer durationLogger()

	resp := struct {
		Name string `json:"name"`
	}{}

	_, err := c.request(&request{
		method:    http.MethodPost,
		path:      fmt.Sprintf("/repos/%s/%s/forks", owner, repo),
		exitCodes: []int{202},
	}, &resp)

	// there are many reasons why GitHub may end up forking the
	// repo under a different name -- the repo got re-named, the
	// bot account already has a fork with that name, etc
	return resp.Name, err
}

// ListRepoTeams gets a list of all the teams with access to a repository
// See https://developer.github.com/v3/repos/#list-teams
func (c *client) ListRepoTeams(org, repo string) ([]Team, error) {
	durationLogger := c.log("ListRepoTeams", org, repo)
	defer durationLogger()

	if c.fake {
		return nil, nil
	}
	path := fmt.Sprintf("/repos/%s/%s/teams", org, repo)
	var teams []Team
	err := c.readPaginatedResults(
		path,
		acceptNone,
		func() interface{} {
			return &[]Team{}
		},
		func(obj interface{}) {
			teams = append(teams, *(obj.(*[]Team))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return teams, nil
}

// ListIssueEvents gets a list events from GitHub's events API that pertain to the specified issue.
// The events that are returned have a different format than webhook events and certain event types
// are excluded.
//
// See https://developer.github.com/v3/issues/events/
func (c *client) ListIssueEvents(org, repo string, num int) ([]ListedIssueEvent, error) {
	durationLogger := c.log("ListIssueEvents", org, repo, num)
	defer durationLogger()

	if c.fake {
		return nil, nil
	}
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/events", org, repo, num)
	var events []ListedIssueEvent
	err := c.readPaginatedResults(
		path,
		acceptNone,
		func() interface{} {
			return &[]ListedIssueEvent{}
		},
		func(obj interface{}) {
			events = append(events, *(obj.(*[]ListedIssueEvent))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return events, nil
}

// IsMergeable determines if a PR can be merged.
// Mergeability is calculated by a background job on GitHub and is not immediately available when
// new commits are added so the PR must be polled until the background job completes.
func (c *client) IsMergeable(org, repo string, number int, SHA string) (bool, error) {
	backoff := time.Second * 3
	maxTries := 3
	for try := 0; try < maxTries; try++ {
		pr, err := c.GetPullRequest(org, repo, number)
		if err != nil {
			return false, err
		}
		if pr.Head.SHA != SHA {
			return false, fmt.Errorf("pull request head changed while checking mergeability (%s -> %s)", SHA, pr.Head.SHA)
		}
		if pr.Merged {
			return false, errors.New("pull request was merged while checking mergeability")
		}
		if pr.Mergable != nil {
			return *pr.Mergable, nil
		}
		if try+1 < maxTries {
			c.time.Sleep(backoff)
			backoff *= 2
		}
	}
	return false, fmt.Errorf("reached maximum number of retries (%d) checking mergeability", maxTries)
}

// ClearMilestone clears the milestone from the specified issue
//
// See https://developer.github.com/v3/issues/#edit-an-issue
func (c *client) ClearMilestone(org, repo string, num int) error {
	durationLogger := c.log("ClearMilestone", org, repo, num)
	defer durationLogger()

	issue := &struct {
		// Clearing the milestone requires providing a null value, and
		// interface{} will serialize to null.
		Milestone interface{} `json:"milestone"`
	}{}
	_, err := c.request(&request{
		method:      http.MethodPatch,
		path:        fmt.Sprintf("/repos/%v/%v/issues/%d", org, repo, num),
		requestBody: &issue,
		exitCodes:   []int{200},
	}, nil)
	return err
}

// SetMilestone sets the milestone from the specified issue (if it is a valid milestone)
//
// See https://developer.github.com/v3/issues/#edit-an-issue
func (c *client) SetMilestone(org, repo string, issueNum, milestoneNum int) error {
	durationLogger := c.log("SetMilestone", org, repo, issueNum, milestoneNum)
	defer durationLogger()

	issue := &struct {
		Milestone int `json:"milestone"`
	}{Milestone: milestoneNum}

	_, err := c.request(&request{
		method:      http.MethodPatch,
		path:        fmt.Sprintf("/repos/%v/%v/issues/%d", org, repo, issueNum),
		requestBody: &issue,
		exitCodes:   []int{200},
	}, nil)
	return err
}

// ListMilestones list all milestones in a repo
//
// See https://developer.github.com/v3/issues/milestones/#list-milestones-for-a-repository/
func (c *client) ListMilestones(org, repo string) ([]Milestone, error) {
	durationLogger := c.log("ListMilestones", org)
	defer durationLogger()

	if c.fake {
		return nil, nil
	}
	path := fmt.Sprintf("/repos/%s/%s/milestones", org, repo)
	var milestones []Milestone
	err := c.readPaginatedResults(
		path,
		acceptNone,
		func() interface{} {
			return &[]Milestone{}
		},
		func(obj interface{}) {
			milestones = append(milestones, *(obj.(*[]Milestone))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return milestones, nil
}

// ListPRCommits lists the commits in a pull request.
//
// GitHub API docs: https://developer.github.com/v3/pulls/#list-commits-on-a-pull-request
func (c *client) ListPRCommits(org, repo string, number int) ([]RepositoryCommit, error) {
	durationLogger := c.log("ListPRCommits", org, repo, number)
	defer durationLogger()

	if c.fake {
		return nil, nil
	}
	var commits []RepositoryCommit
	err := c.readPaginatedResults(
		fmt.Sprintf("/repos/%v/%v/pulls/%d/commits", org, repo, number),
		acceptNone,
		func() interface{} { // newObj returns a pointer to the type of object to create
			return &[]RepositoryCommit{}
		},
		func(obj interface{}) { // accumulate is the accumulation function for paginated results
			commits = append(commits, *(obj.(*[]RepositoryCommit))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return commits, nil
}

// newReloadingTokenSource creates a reloadingTokenSource.
func newReloadingTokenSource(getToken func() []byte) *reloadingTokenSource {
	return &reloadingTokenSource{
		getToken: getToken,
	}
}

// Token is an implementation for oauth2.TokenSource interface.
func (s *reloadingTokenSource) Token() (*oauth2.Token, error) {
	return &oauth2.Token{
		AccessToken: string(s.getToken()),
	}, nil
}

// GetRepoProjects returns the list of projects in this repo.
//
// See https://developer.github.com/v3/projects/#list-repository-projects
func (c *client) GetRepoProjects(owner, repo string) ([]Project, error) {
	durationLogger := c.log("GetOrgProjects", owner, repo)
	defer durationLogger()

	path := fmt.Sprintf("/repos/%s/%s/projects", owner, repo)
	var projects []Project
	err := c.readPaginatedResults(
		path,
		"application/vnd.github.inertia-preview+json",
		func() interface{} {
			return &[]Project{}
		},
		func(obj interface{}) {
			projects = append(projects, *(obj.(*[]Project))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return projects, nil
}

// GetOrgProjects returns the list of projects in this org.
//
// See https://developer.github.com/v3/projects/#list-organization-projects
func (c *client) GetOrgProjects(org string) ([]Project, error) {
	durationLogger := c.log("GetOrgProjects", org)
	defer durationLogger()

	path := fmt.Sprintf("/orgs/%s/projects", org)
	var projects []Project
	err := c.readPaginatedResults(
		path,
		"application/vnd.github.inertia-preview+json",
		func() interface{} {
			return &[]Project{}
		},
		func(obj interface{}) {
			projects = append(projects, *(obj.(*[]Project))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return projects, nil
}

// GetProjectColumns returns the list of columns in a project.
//
// See https://developer.github.com/v3/projects/columns/#list-project-columns
func (c *client) GetProjectColumns(org string, projectID int) ([]ProjectColumn, error) {
	durationLogger := c.log("GetProjectColumns", projectID)
	defer durationLogger()

	path := fmt.Sprintf("/projects/%d/columns", projectID)
	var projectColumns []ProjectColumn
	err := c.readPaginatedResults(
		path,
		"application/vnd.github.inertia-preview+json",
		func() interface{} {
			return &[]ProjectColumn{}
		},
		func(obj interface{}) {
			projectColumns = append(projectColumns, *(obj.(*[]ProjectColumn))...)
		},
	)
	if err != nil {
		return nil, err
	}
	return projectColumns, nil
}

// CreateProjectCard adds a project card to the specified project column.
//
// See https://developer.github.com/v3/projects/cards/#create-a-project-card
func (c *client) CreateProjectCard(org string, columnID int, projectCard ProjectCard) (*ProjectCard, error) {
	durationLogger := c.log("CreateProjectCard", columnID, projectCard)
	defer durationLogger()

	if (projectCard.ContentType != "Issue") && (projectCard.ContentType != "PullRequest") {
		return nil, errors.New("projectCard.ContentType must be either Issue or PullRequest")
	}
	if c.dry {
		return &projectCard, nil
	}
	path := fmt.Sprintf("/projects/columns/%d/cards", columnID)
	var retProjectCard ProjectCard
	_, err := c.request(&request{
		method:      http.MethodPost,
		path:        path,
		accept:      "application/vnd.github.inertia-preview+json",
		requestBody: &projectCard,
		exitCodes:   []int{200},
	}, &retProjectCard)
	return &retProjectCard, err
}

// GetProjectColumnCards get all project cards in a column. This helps in iterating all
// issues and PRs that are under a column
func (c *client) GetColumnProjectCards(org string, columnID int) ([]ProjectCard, error) {
	durationLogger := c.log("GetColumnProjectCards", columnID)
	defer durationLogger()

	if c.fake {
		return nil, nil
	}
	path := fmt.Sprintf("/projects/columns/%d/cards", columnID)
	var cards []ProjectCard
	err := c.readPaginatedResults(
		path,
		// projects api requies the accept header to be set this way
		"application/vnd.github.inertia-preview+json",
		func() interface{} {
			return &[]ProjectCard{}
		},
		func(obj interface{}) {
			cards = append(cards, *(obj.(*[]ProjectCard))...)
		},
	)
	return cards, err
}

// GetColumnProjectCard of a specific issue or PR for a specific column in a board/project
// This method requires the URL of the issue/pr to compare the issue with the content_url
// field of the card.  See https://developer.github.com/v3/projects/cards/#list-project-cards
func (c *client) GetColumnProjectCard(org string, columnID int, issueURL string) (*ProjectCard, error) {
	cards, err := c.GetColumnProjectCards(org, columnID)
	if err != nil {
		return nil, err
	}

	for _, card := range cards {
		if card.ContentURL == issueURL {
			return &card, nil
		}
	}
	return nil, nil
}

// MoveProjectCard moves a specific project card to a specified column in the same project
//
// See https://developer.github.com/v3/projects/cards/#move-a-project-card
func (c *client) MoveProjectCard(org string, projectCardID int, newColumnID int) error {
	durationLogger := c.log("MoveProjectCard", projectCardID, newColumnID)
	defer durationLogger()

	reqParams := struct {
		Position string `json:"position"`
		ColumnID int    `json:"column_id"`
	}{"top", newColumnID}

	_, err := c.request(&request{
		method:      http.MethodPost,
		path:        fmt.Sprintf("/projects/columns/cards/%d/moves", projectCardID),
		accept:      "application/vnd.github.inertia-preview+json",
		requestBody: reqParams,
		exitCodes:   []int{201},
	}, nil)
	return err
}

// DeleteProjectCard deletes the project card of a specific issue or PR
//
// See https://developer.github.com/v3/projects/cards/#delete-a-project-card
func (c *client) DeleteProjectCard(org string, projectCardID int) error {
	durationLogger := c.log("DeleteProjectCard", projectCardID)
	defer durationLogger()

	_, err := c.request(&request{
		method:    http.MethodDelete,
		accept:    "application/vnd.github.symmetra-preview+json", // allow the description field -- https://developer.github.com/changes/2018-02-22-label-description-search-preview/
		path:      fmt.Sprintf("/projects/columns/cards/:%d", projectCardID),
		exitCodes: []int{204},
	}, nil)
	return err
}

// TeamHasMember checks if a user belongs to a team
func (c *client) TeamHasMember(org string, teamID int, memberLogin string) (bool, error) {
	durationLogger := c.log("TeamHasMember", teamID, memberLogin)
	defer durationLogger()

	projectMaintainers, err := c.ListTeamMembers(org, teamID, RoleAll)
	if err != nil {
		return false, err
	}
	for _, person := range projectMaintainers {
		if NormLogin(person.Login) == NormLogin(memberLogin) {
			return true, nil
		}
	}
	return false, nil
}

// GetTeamBySlug returns information about that team
//
// See https://developer.github.com/v3/teams/#get-team-by-name
func (c *client) GetTeamBySlug(slug string, org string) (*Team, error) {
	durationLogger := c.log("GetTeamBySlug", slug, org)
	defer durationLogger()

	if c.fake {
		return &Team{}, nil
	}
	var team Team
	_, err := c.request(&request{
		method:    http.MethodGet,
		path:      fmt.Sprintf("/orgs/%s/teams/%s", org, slug),
		exitCodes: []int{200},
	}, &team)
	if err != nil {
		return nil, err
	}
	return &team, err
}

// ListCheckRuns lists all checkruns for the given ref
//
// See https://docs.github.com/en/free-pro-team@latest/rest/reference/checks#list-check-runs-for-a-git-reference
func (c *client) ListCheckRuns(org, repo, ref string) (*CheckRunList, error) {
	durationLogger := c.log("ListCheckRuns", org, repo, ref)
	defer durationLogger()

	var checkRunList CheckRunList
	_, err := c.request(&request{
		method:    http.MethodGet,
		path:      fmt.Sprintf("/repos/%s/%s/commits/%s/check-runs", org, repo, ref),
		exitCodes: []int{200},
	}, &checkRunList)
	if err != nil {
		return nil, err
	}
	return &checkRunList, nil
}
