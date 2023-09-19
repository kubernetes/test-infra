/*
Copyright 2020 The Kubernetes Authors.

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

package jira

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	stdio "io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/andygrunwald/go-jira"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/version"
)

// These are all the current valid states for Red Hat bugs in jira
const (
	StatusNew            = "NEW"
	StatusBacklog        = "BACKLOG"
	StatusAssigned       = "ASSIGNED"
	StatusInProgess      = "IN PROGRESS"
	StatusModified       = "MODIFIED"
	StatusPost           = "POST"
	StatusOnDev          = "ON_DEV"
	StatusOnQA           = "ON_QA"
	StatusVerified       = "VERIFIED"
	StatusReleasePending = "RELEASE PENDING"
	StatusClosed         = "CLOSED"
)

type Client interface {
	GetIssue(id string) (*jira.Issue, error)
	// SearchWithContext will search for tickets according to the jql
	// Jira API docs: https://developer.atlassian.com/jiradev/jira-apis/jira-rest-apis/jira-rest-api-tutorials/jira-rest-api-example-query-issues
	SearchWithContext(ctx context.Context, jql string, options *jira.SearchOptions) ([]jira.Issue, *jira.Response, error)
	UpdateIssue(*jira.Issue) (*jira.Issue, error)
	CreateIssue(*jira.Issue) (*jira.Issue, error)
	CreateIssueLink(*jira.IssueLink) error
	// CloneIssue copies an issue struct, clears unsettable fields, creates a new
	// issue using the updated struct, and then links the new issue as a clone to
	// the original.
	CloneIssue(*jira.Issue) (*jira.Issue, error)
	GetTransitions(issueID string) ([]jira.Transition, error)
	DoTransition(issueID, transitionID string) error
	// UpdateStatus updates an issue's status by identifying the ID of the provided
	// statusName and then doing the status transition to update the issue.
	UpdateStatus(issueID, statusName string) error
	// GetIssueSecurityLevel returns the security level of an issue. If no security level
	// is set for the issue, the returned SecurityLevel and error will both be nil and
	// the issue will follow the default project security level.
	GetIssueSecurityLevel(*jira.Issue) (*SecurityLevel, error)
	// GetIssueQaContact get the user details for the QA contact. The QA contact is a custom field in Jira
	GetIssueQaContact(*jira.Issue) (*jira.User, error)
	// GetIssueTargetVersion get the issue Target Release. The target release is a custom field in Jira
	GetIssueTargetVersion(issue *jira.Issue) (*[]*jira.Version, error)
	// FindUser returns all users with a field matching the queryParam (ex: email, display name, etc.)
	FindUser(queryParam string) ([]*jira.User, error)
	GetRemoteLinks(id string) ([]jira.RemoteLink, error)
	AddRemoteLink(id string, link *jira.RemoteLink) (*jira.RemoteLink, error)
	UpdateRemoteLink(id string, link *jira.RemoteLink) error
	DeleteLink(id string) error
	DeleteRemoteLink(issueID string, linkID int) error
	// DeleteRemoteLinkViaURL identifies and removes a remote link from an issue
	// the has the provided URL. The returned bool indicates whether a change
	// was made during the operation as a remote link with the URL not existing
	// is not consider an error for this function.
	DeleteRemoteLinkViaURL(issueID, url string) (bool, error)
	ForPlugin(plugin string) Client
	AddComment(issueID string, comment *jira.Comment) (*jira.Comment, error)
	ListProjects() (*jira.ProjectList, error)
	JiraClient() *jira.Client
	JiraURL() string
	Used() bool
	WithFields(fields logrus.Fields) Client
	GetProjectVersions(project string) ([]*jira.Version, error)
}

type BasicAuthGenerator func() (username, password string)
type BearerAuthGenerator func() (token string)

type Options struct {
	BasicAuth  BasicAuthGenerator
	BearerAuth BearerAuthGenerator
	LogFields  logrus.Fields
}

type Option func(*Options)

func WithBasicAuth(basicAuth BasicAuthGenerator) Option {
	return func(o *Options) {
		o.BasicAuth = basicAuth
	}
}

func WithBearerAuth(token BearerAuthGenerator) Option {
	return func(o *Options) {
		o.BearerAuth = token
	}
}

func WithFields(fields logrus.Fields) Option {
	return func(o *Options) {
		o.LogFields = fields
	}
}

func newJiraClient(endpoint string, o Options, retryingClient *retryablehttp.Client) (*jira.Client, error) {
	retryingClient.HTTPClient.Transport = &metricsTransport{
		upstream:       retryingClient.HTTPClient.Transport,
		pathSimplifier: pathSimplifier().Simplify,
		recorder:       requestResults,
	}
	retryingClient.HTTPClient.Transport = userAgentSettingTransport{
		userAgent: version.UserAgent(),
		upstream:  retryingClient.HTTPClient.Transport,
	}

	if o.BasicAuth != nil {
		retryingClient.HTTPClient.Transport = &basicAuthRoundtripper{
			generator: o.BasicAuth,
			upstream:  retryingClient.HTTPClient.Transport,
		}
	}

	if o.BearerAuth != nil {
		retryingClient.HTTPClient.Transport = &bearerAuthRoundtripper{
			generator: o.BearerAuth,
			upstream:  retryingClient.HTTPClient.Transport,
		}
	}

	return jira.NewClient(retryingClient.StandardClient(), endpoint)
}

func NewClient(endpoint string, opts ...Option) (Client, error) {
	o := Options{}
	for _, opt := range opts {
		opt(&o)
	}

	log := logrus.WithField("client", "jira")
	if len(o.LogFields) > 0 {
		log = log.WithFields(o.LogFields)
	}
	retryingClient := retryablehttp.NewClient()
	usedFlagTransport := &clientUsedTransport{
		m:        sync.Mutex{},
		upstream: retryingClient.HTTPClient.Transport,
	}
	retryingClient.HTTPClient.Transport = usedFlagTransport
	retryingClient.Logger = &retryableHTTPLogrusWrapper{log: log}

	jiraClient, err := newJiraClient(endpoint, o, retryingClient)
	if err != nil {
		return nil, err
	}
	url := jiraClient.GetBaseURL()
	return &client{delegate: &delegate{url: url.String(), options: o}, logger: log, upstream: jiraClient, clientUsed: usedFlagTransport}, err
}

type userAgentSettingTransport struct {
	userAgent string
	upstream  http.RoundTripper
}

func (u userAgentSettingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("User-Agent", u.userAgent)
	return u.upstream.RoundTrip(r)
}

type clientUsedTransport struct {
	used     bool
	m        sync.Mutex
	upstream http.RoundTripper
}

func (c *clientUsedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.m.Lock()
	c.used = true
	c.m.Unlock()
	return c.upstream.RoundTrip(r)
}

func (c *clientUsedTransport) Used() bool {
	c.m.Lock()
	defer c.m.Unlock()
	return c.used
}

type used interface {
	Used() bool
}

type client struct {
	logger     *logrus.Entry
	upstream   *jira.Client
	clientUsed used
	*delegate
}

// delegate actually does the work to talk to Jira
type delegate struct {
	url     string
	options Options
}

func (jc *client) JiraClient() *jira.Client {
	return jc.upstream
}

func (jc *client) GetIssue(id string) (*jira.Issue, error) {
	issue, response, err := jc.upstream.Issue.Get(id, &jira.GetQueryOptions{})
	if err != nil {
		if response != nil && response.StatusCode == http.StatusNotFound {
			return nil, NotFoundError{err}
		}
		return nil, HandleJiraError(response, err)
	}

	return issue, nil
}

func (jc *client) ListProjects() (*jira.ProjectList, error) {
	projects, response, err := jc.upstream.Project.GetList()
	if err != nil {
		return nil, HandleJiraError(response, err)
	}
	return projects, nil
}

func IsNotFound(err error) bool {
	return errors.Is(err, NotFoundError{})
}

func NewNotFoundError(err error) error {
	return NotFoundError{err}
}

type NotFoundError struct {
	error
}

func (NotFoundError) Is(target error) bool {
	_, match := target.(NotFoundError)
	return match
}

func (jc *client) GetRemoteLinks(id string) ([]jira.RemoteLink, error) {
	result, resp, err := jc.upstream.Issue.GetRemoteLinks(id)
	if err != nil {
		return nil, HandleJiraError(resp, err)
	}
	return *result, nil
}

func (jc *client) AddRemoteLink(id string, link *jira.RemoteLink) (*jira.RemoteLink, error) {
	result, resp, err := jc.upstream.Issue.AddRemoteLink(id, link)
	if err != nil {
		return nil, fmt.Errorf("failed to add link: %w", HandleJiraError(resp, err))
	}
	return result, nil
}

func (jc *client) DeleteLink(linkID string) error {
	resp, err := jc.upstream.Issue.DeleteLink(linkID)
	if err != nil {
		return HandleJiraError(resp, err)
	}
	return nil
}

func (jc *client) UpdateRemoteLink(id string, link *jira.RemoteLink) error {
	internalLinkId := fmt.Sprint(link.ID)
	req, err := jc.upstream.NewRequest("PUT", "rest/api/2/issue/"+id+"/remotelink/"+internalLinkId, link)
	if err != nil {
		return fmt.Errorf("failed to construct request: %w", err)
	}
	resp, err := jc.upstream.Do(req, nil)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("failed to update link: %w", HandleJiraError(resp, err))
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to update link: expected status code %d but got %d instead", http.StatusNoContent, resp.StatusCode)
	}
	return nil
}

func (jc *client) DeleteRemoteLink(issueID string, linkID int) error {
	apiEndpoint := fmt.Sprintf("/rest/api/2/issue/%s/remotelink/%d", issueID, linkID)
	req, err := jc.upstream.NewRequest("DELETE", apiEndpoint, nil)
	if err != nil {
		return err
	}

	// the response should be empty if it is not an error
	resp, err := jc.upstream.Do(req, nil)
	if resp != nil {
		defer resp.Body.Close()
	}
	// Status code 204 is a success for this function. On success, there will be an error message of `EOF`,
	// so in addition to the nil check for the error, we must check the status code.
	if resp.StatusCode != 204 && err != nil {
		return HandleJiraError(resp, err)
	}
	return nil
}

// DeleteRemoteLinkViaURL identifies and removes a remote link from an issue
// the has the provided URL. The returned bool indicates whether a change
// was made during the operation as a remote link with the URL not existing
// is not consider an error for this function.
func DeleteRemoteLinkViaURL(jc Client, issueID, url string) (bool, error) {
	links, err := jc.GetRemoteLinks(issueID)
	if err != nil {
		return false, err
	}
	for _, link := range links {
		if link.Object.URL == url {
			return true, jc.DeleteRemoteLink(issueID, link.ID)
		}
	}
	return false, fmt.Errorf("could not find remote link on issue with URL `%s`", url)
}

func (jc *client) DeleteRemoteLinkViaURL(issueID, url string) (bool, error) {
	return DeleteRemoteLinkViaURL(jc, issueID, url)
}

func (jc *client) FindUser(queryParam string) ([]*jira.User, error) {
	// JIRA's own documentation here is incorrect; it specifies that either 'accountID',
	// 'query', or 'property' must be used. However, JIRA throws an error unless 'username'
	// is used. This does a search as if it were supposed to be the query param, so we can use it like that
	queryString := "username='" + queryParam + "'"
	queryString = url.PathEscape(queryString)

	apiEndpoint := fmt.Sprintf("/rest/api/2/user/search?%s", queryString)
	req, err := jc.upstream.NewRequest("GET", apiEndpoint, nil)
	if err != nil {
		return nil, err
	}

	users := []*jira.User{}
	resp, err := jc.upstream.Do(req, &users)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, HandleJiraError(resp, err)
	}
	return users, nil
}

// UpdateStatus updates an issue's status by identifying the ID of the provided
// statusName and then doing the status transition using the provided client to update the issue.
func UpdateStatus(jc Client, issueID, statusName string) error {
	transitions, err := jc.GetTransitions(issueID)
	if err != nil {
		return err
	}
	transitionID := ""
	var nameList []string
	for _, transition := range transitions {
		// JIRA shows all statuses as caps in the UI, but internally has different case; use EqualFold to ignore case
		if strings.EqualFold(transition.Name, statusName) {
			transitionID = transition.ID
			break
		}
		nameList = append(nameList, transition.Name)
	}
	if transitionID == "" {
		return fmt.Errorf("No transition status with name `%s` could be found. Please select from the following list: %v", statusName, nameList)
	}
	return jc.DoTransition(issueID, transitionID)
}

func (jc *client) UpdateStatus(issueID, statusName string) error {
	return UpdateStatus(jc, issueID, statusName)
}

func (jc *client) GetTransitions(issueID string) ([]jira.Transition, error) {
	transitions, resp, err := jc.upstream.Issue.GetTransitions(issueID)
	if err != nil {
		return nil, HandleJiraError(resp, err)
	}
	return transitions, nil
}

func (jc *client) DoTransition(issueID, transitionID string) error {
	resp, err := jc.upstream.Issue.DoTransition(issueID, transitionID)
	if err != nil {
		return HandleJiraError(resp, err)
	}
	return nil
}

func (jc *client) UpdateIssue(issue *jira.Issue) (*jira.Issue, error) {
	result, resp, err := jc.upstream.Issue.Update(issue)
	if err != nil {
		return nil, HandleJiraError(resp, err)
	}
	return result, nil
}

func (jc *client) AddComment(issueID string, comment *jira.Comment) (*jira.Comment, error) {
	result, resp, err := jc.upstream.Issue.AddComment(issueID, comment)
	if err != nil {
		return nil, HandleJiraError(resp, err)
	}
	return result, nil
}

func (jc *client) CreateIssue(issue *jira.Issue) (*jira.Issue, error) {
	result, resp, err := jc.upstream.Issue.Create(issue)
	if err != nil {
		return nil, HandleJiraError(resp, err)
	}
	return result, nil
}

func (jc *client) CreateIssueLink(link *jira.IssueLink) error {
	resp, err := jc.upstream.Issue.AddLink(link)
	if err != nil {
		return HandleJiraError(resp, err)
	}
	return nil
}

// CloneIssue copies an issue struct, clears unsettable fields, creates a new
// issue using the updated struct, and then links the new issue as a clone to
// the original.
func CloneIssue(jc Client, parent *jira.Issue) (*jira.Issue, error) {
	// create deep copy of parent "Fields" field
	data, err := json.Marshal(parent.Fields)
	if err != nil {
		return nil, err
	}
	childIssueFields := &jira.IssueFields{}
	err = json.Unmarshal(data, childIssueFields)
	if err != nil {
		return nil, err
	}
	childIssue := &jira.Issue{
		Fields: childIssueFields,
	}
	// update description
	childIssue.Fields.Description = fmt.Sprintf("This is a clone of issue %s. The following is the description of the original issue: \n---\n%s", parent.Key, parent.Fields.Description)

	// attempt to create the new issue
	createdIssue, err := jc.CreateIssue(childIssue)
	if err != nil {
		// some fields cannot be set on creation; unset them
		if JiraErrorStatusCode(err) != 400 {
			return nil, err
		}
		var newErr error
		childIssue, newErr = unsetProblematicFields(childIssue, JiraErrorBody(err))
		if newErr != nil {
			// in this situation, it makes more sense to just return the original error; any error from unsetProblematicFields will be
			// a json marshalling error, indicating an error different from the standard non-settable fields error. The error from
			// unsetProblematicFields is not useful in these cases
			return nil, err
		}
		createdIssue, err = jc.CreateIssue(childIssue)
		if err != nil {
			return nil, err
		}
	}

	// create clone links
	link := &jira.IssueLink{
		OutwardIssue: &jira.Issue{ID: parent.ID},
		InwardIssue:  &jira.Issue{ID: createdIssue.ID},
		Type: jira.IssueLinkType{
			Name:    "Cloners",
			Inward:  "is cloned by",
			Outward: "clones",
		},
	}
	if err := jc.CreateIssueLink(link); err != nil {
		return nil, err
	}
	// Get updated issue, which would have issue links
	if clonedIssue, err := jc.GetIssue(createdIssue.ID); err != nil {
		// still return the originally created child issue here in case of failure to get updated issue
		return createdIssue, fmt.Errorf("Could not get issue after creating issue links: %w", err)
	} else {
		return clonedIssue, nil
	}
}

func (jc *client) CloneIssue(parent *jira.Issue) (*jira.Issue, error) {
	return CloneIssue(jc, parent)
}

func unsetProblematicFields(issue *jira.Issue, responseBody string) (*jira.Issue, error) {
	// handle unsettable "unknown" fields
	processedResponse := CreateIssueError{}
	if newErr := json.Unmarshal([]byte(responseBody), &processedResponse); newErr != nil {
		return nil, fmt.Errorf("Error processing jira error: %w", newErr)
	}
	// turn issue into map to simplify unsetting process
	marshalledIssue, err := json.Marshal(issue)
	if err != nil {
		return nil, err
	}
	issueMap := make(map[string]interface{})
	if err := json.Unmarshal(marshalledIssue, &issueMap); err != nil {
		return nil, err
	}
	fieldsMap := issueMap["fields"].(map[string]interface{})
	for field := range processedResponse.Errors {
		delete(fieldsMap, field)
	}
	// Remove null value "customfields_" because they cause the server to return: 500 Internal Server Error
	for field, value := range fieldsMap {
		if strings.HasPrefix(field, "customfield_") && value == nil {
			delete(fieldsMap, field)
		}
	}
	issueMap["fields"] = fieldsMap
	// turn back into jira.Issue type
	marshalledFixedIssue, err := json.Marshal(issueMap)
	if err != nil {
		return nil, err
	}
	newIssue := jira.Issue{}
	if err := json.Unmarshal(marshalledFixedIssue, &newIssue); err != nil {
		return nil, err
	}
	return &newIssue, nil
}

type CreateIssueError struct {
	ErrorMessages []string          `json:"errorMessages"`
	Errors        map[string]string `json:"errors"`
}

type SecurityLevel struct {
	Self        string `json:"self"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Used determines whether the client has been used
func (jc *client) Used() bool {
	return jc.clientUsed.Used()
}

type bearerAuthRoundtripper struct {
	generator BearerAuthGenerator
	upstream  http.RoundTripper
}

// WithFields clones the client, keeping the underlying delegate the same but adding
// fields to the logging context
func (jc *client) WithFields(fields logrus.Fields) Client {
	return &client{
		clientUsed: jc.clientUsed,
		upstream:   jc.upstream,
		logger:     jc.logger.WithFields(fields),
		delegate:   jc.delegate,
	}
}

// ForPlugin clones the client, keeping the underlying delegate the same but adding
// a plugin identifier and log field
func (jc *client) ForPlugin(plugin string) Client {
	pluginLogger := jc.logger.WithField("plugin", plugin)
	retryingClient := retryablehttp.NewClient()
	usedFlagTransport := &clientUsedTransport{
		m:        sync.Mutex{},
		upstream: retryingClient.HTTPClient.Transport,
	}
	retryingClient.HTTPClient.Transport = usedFlagTransport
	retryingClient.Logger = &retryableHTTPLogrusWrapper{log: pluginLogger}
	// ignore error as url.String() was passed to the delegate
	jiraClient, err := newJiraClient(jc.url, jc.options, retryingClient)
	if err != nil {
		pluginLogger.WithError(err).Error("invalid Jira URL")
		jiraClient = jc.upstream
	}
	return &client{
		logger:     pluginLogger,
		clientUsed: usedFlagTransport,
		upstream:   jiraClient,
		delegate:   jc.delegate,
	}
}

func (jc *client) JiraURL() string {
	return jc.url
}

func (bart *bearerAuthRoundtripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := new(http.Request)
	*req2 = *req
	req2.URL = new(url.URL)
	*req2.URL = *req.URL
	token := bart.generator()
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	logrus.WithField("curl", toCurl(req2)).Trace("Executing http request")
	return bart.upstream.RoundTrip(req2)
}

type basicAuthRoundtripper struct {
	generator BasicAuthGenerator
	upstream  http.RoundTripper
}

func (bart *basicAuthRoundtripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := new(http.Request)
	*req2 = *req
	req2.URL = new(url.URL)
	*req2.URL = *req.URL
	user, pass := bart.generator()
	req2.SetBasicAuth(user, pass)
	logrus.WithField("curl", toCurl(req2)).Trace("Executing http request")
	return bart.upstream.RoundTrip(req2)
}

var knownAuthTypes = sets.New[string]("bearer", "basic", "negotiate")

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

type JiraError struct {
	StatusCode    int
	Body          string
	OriginalError error
}

func (e JiraError) Error() string {
	return fmt.Sprintf("%s: %s", e.OriginalError, e.Body)
}

// JiraErrorStatusCode will identify if an error is a JiraError and return the
// stored status code if it is; if it is not, `-1` will be returned
func JiraErrorStatusCode(err error) int {
	if jiraErr := (&JiraError{}); errors.As(err, &jiraErr) {
		return jiraErr.StatusCode
	}
	jiraErr, ok := err.(*JiraError)
	if !ok {
		return -1
	}
	return jiraErr.StatusCode
}

// JiraErrorBody will identify if an error is a JiraError and return the stored
// response body if it is; if it is not, an empty string will be returned
func JiraErrorBody(err error) string {
	if jiraErr := (&JiraError{}); errors.As(err, &jiraErr) {
		return jiraErr.Body
	}
	jiraErr, ok := err.(*JiraError)
	if !ok {
		return ""
	}
	return jiraErr.Body
}

// HandleJiraError collapses cryptic Jira errors to include response
// bodies if it's detected that the original error holds no
// useful context in the first place
func HandleJiraError(response *jira.Response, err error) error {
	if err != nil && strings.Contains(err.Error(), "Please analyze the request body for more details.") {
		if response != nil && response.Response != nil {
			body, readError := stdio.ReadAll(response.Body)
			if readError != nil && readError.Error() != "http: read on closed response body" {
				logrus.WithError(readError).Warn("Failed to read Jira response body.")
			}
			return &JiraError{
				StatusCode:    response.StatusCode,
				Body:          string(body),
				OriginalError: err,
			}
		}
	}
	return err
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

type retryableHTTPLogrusWrapper struct {
	log *logrus.Entry
}

// fieldsForContext translates a list of context fields to a
// logrus format; any items that don't conform to our expectations
// are omitted
func (l *retryableHTTPLogrusWrapper) fieldsForContext(context ...interface{}) logrus.Fields {
	fields := logrus.Fields{}
	for i := 0; i < len(context)-1; i += 2 {
		key, ok := context[i].(string)
		if !ok {
			continue
		}
		fields[key] = context[i+1]
	}
	return fields
}

func (l *retryableHTTPLogrusWrapper) Error(msg string, context ...interface{}) {
	l.log.WithFields(l.fieldsForContext(context...)).Error(msg)
}

func (l *retryableHTTPLogrusWrapper) Info(msg string, context ...interface{}) {
	l.log.WithFields(l.fieldsForContext(context...)).Info(msg)
}

func (l *retryableHTTPLogrusWrapper) Debug(msg string, context ...interface{}) {
	l.log.WithFields(l.fieldsForContext(context...)).Debug(msg)
}

func (l *retryableHTTPLogrusWrapper) Warn(msg string, context ...interface{}) {
	l.log.WithFields(l.fieldsForContext(context...)).Warn(msg)
}

func (jc *client) SearchWithContext(ctx context.Context, jql string, options *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
	issues, response, err := jc.upstream.Issue.SearchWithContext(ctx, jql, options)
	if err != nil {
		if response != nil && response.StatusCode == http.StatusNotFound {
			return nil, response, NotFoundError{err}
		}
		return nil, response, HandleJiraError(response, err)
	}
	return issues, response, nil
}

func GetUnknownField(field string, issue *jira.Issue, fn func() interface{}) error {
	obj := fn()
	unknownField, ok := issue.Fields.Unknowns[field]
	if !ok {
		return nil
	}
	bytes, err := json.Marshal(unknownField)
	if err != nil {
		return fmt.Errorf("failed to process the custom field %s. Error : %v", field, err)
	}
	if err := json.Unmarshal(bytes, obj); err != nil {
		return fmt.Errorf("failed to unmarshall the json to struct for %s. Error: %v", field, err)
	}
	return err

}

// GetIssueSecurityLevel returns the security level of an issue. If no security level
// is set for the issue, the returned SecurityLevel and error will both be nil and
// the issue will follow the default project security level.
func GetIssueSecurityLevel(issue *jira.Issue) (*SecurityLevel, error) {
	// TODO: Add field to the upstream go-jira package; if a security level exists, it is returned
	// as part of the issue fields
	// See https://github.com/andygrunwald/go-jira/issues/456
	var obj *SecurityLevel
	err := GetUnknownField("security", issue, func() interface{} {
		obj = &SecurityLevel{}
		return obj
	})
	return obj, err
}

func (jc *client) GetIssueSecurityLevel(issue *jira.Issue) (*SecurityLevel, error) {
	return GetIssueSecurityLevel(issue)
}

func GetIssueQaContact(issue *jira.Issue) (*jira.User, error) {
	var obj *jira.User
	err := GetUnknownField("customfield_12316243", issue, func() interface{} {
		obj = &jira.User{}
		return obj
	})
	return obj, err
}

func (jc *client) GetIssueQaContact(issue *jira.Issue) (*jira.User, error) {
	return GetIssueQaContact(issue)
}

func GetIssueTargetVersion(issue *jira.Issue) (*[]*jira.Version, error) {
	var obj *[]*jira.Version
	err := GetUnknownField("customfield_12319940", issue, func() interface{} {
		obj = &[]*jira.Version{{}}
		return obj
	})
	return obj, err
}

func (jc *client) GetIssueTargetVersion(issue *jira.Issue) (*[]*jira.Version, error) {
	return GetIssueTargetVersion(issue)
}

// GetProjectVersions returns the list of all the Versions defined in a Project
func (jc *client) GetProjectVersions(project string) ([]*jira.Version, error) {
	req, err := jc.upstream.NewRequest("GET", "rest/api/2/project/"+project+"/versions", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to construct request: %w", err)
	}
	versions := []*jira.Version{}
	resp, err := jc.upstream.Do(req, &versions)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, HandleJiraError(resp, err)
	}
	return versions, nil
}
