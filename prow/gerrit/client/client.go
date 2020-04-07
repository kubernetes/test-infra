/*
Copyright 2018 The Kubernetes Authors.

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

// Package client implements a client that can handle multiple gerrit instances
// derived from https://github.com/andygrunwald/go-gerrit
package client

import (
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/andygrunwald/go-gerrit"
	"github.com/sirupsen/logrus"
)

const (
	// CodeReview is the default gerrit code review label
	CodeReview = "Code-Review"

	// GerritID identifies a gerrit change
	GerritID = "prow.k8s.io/gerrit-id"
	// GerritInstance is the gerrit host url
	GerritInstance = "prow.k8s.io/gerrit-instance"
	// GerritRevision is the SHA of current patchset from a gerrit change
	GerritRevision = "prow.k8s.io/gerrit-revision"
	// GerritReportLabel is the gerrit label prow will cast vote on, fallback to CodeReview label if unset
	GerritReportLabel = "prow.k8s.io/gerrit-report-label"

	// Merged status indicates a Gerrit change has been merged
	Merged = "MERGED"
	// New status indicates a Gerrit change is new (ie pending)
	New = "NEW"
)

// ProjectsFlag is the flag type for gerrit projects when initializing a gerrit client
type ProjectsFlag map[string][]string

func (p ProjectsFlag) String() string {
	var hosts []string
	for host, repos := range p {
		hosts = append(hosts, host+"="+strings.Join(repos, ","))
	}
	return strings.Join(hosts, " ")
}

// Set populates ProjectsFlag upon flag.Parse()
func (p ProjectsFlag) Set(value string) error {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("%s not in the form of host=repo-a,repo-b,etc", value)
	}
	host := parts[0]
	if _, ok := p[host]; ok {
		return fmt.Errorf("duplicate host: %s", host)
	}
	repos := strings.Split(parts[1], ",")
	p[host] = repos
	return nil
}

type gerritAuthentication interface {
	SetCookieAuth(name, value string)
}

type gerritAccount interface {
	GetAccount(name string) (*gerrit.AccountInfo, *gerrit.Response, error)
	SetUsername(accountID string, input *gerrit.UsernameInput) (*string, *gerrit.Response, error)
}

type gerritChange interface {
	QueryChanges(opt *gerrit.QueryChangeOptions) (*[]gerrit.ChangeInfo, *gerrit.Response, error)
	SetReview(changeID, revisionID string, input *gerrit.ReviewInput) (*gerrit.ReviewResult, *gerrit.Response, error)
}

type gerritProjects interface {
	GetBranch(projectName, branchID string) (*gerrit.BranchInfo, *gerrit.Response, error)
}

// gerritInstanceHandler holds all actual gerrit handlers
type gerritInstanceHandler struct {
	instance string
	projects []string

	authService    gerritAuthentication
	accountService gerritAccount
	changeService  gerritChange
	projectService gerritProjects
}

// Client holds a instance:handler map
type Client struct {
	handlers map[string]*gerritInstanceHandler
	// map of instance to gerrit account
	accounts map[string]*gerrit.AccountInfo
}

// ChangeInfo is a gerrit.ChangeInfo
type ChangeInfo = gerrit.ChangeInfo

// RevisionInfo is a gerrit.RevisionInfo
type RevisionInfo = gerrit.RevisionInfo

// FileInfo is a gerrit.FileInfo
type FileInfo = gerrit.FileInfo

// Map from instance name to repos to lastsync time for that repo
type LastSyncState map[string]map[string]time.Time

func (l LastSyncState) DeepCopy() LastSyncState {
	result := LastSyncState{}
	for host, lastSyncs := range l {
		result[host] = map[string]time.Time{}
		for projects, lastSync := range lastSyncs {
			result[host][projects] = lastSync
		}
	}
	return result
}

// NewClient returns a new gerrit client
func NewClient(instances map[string][]string) (*Client, error) {
	c := &Client{
		handlers: map[string]*gerritInstanceHandler{},
		accounts: map[string]*gerrit.AccountInfo{},
	}
	for instance := range instances {
		gc, err := gerrit.NewClient(instance, nil)
		if err != nil {
			return nil, err
		}

		c.handlers[instance] = &gerritInstanceHandler{
			instance:       instance,
			projects:       instances[instance],
			authService:    gc.Authentication,
			accountService: gc.Accounts,
			changeService:  gc.Changes,
			projectService: gc.Projects,
		}
	}

	return c, nil
}

func auth(c *Client, cookiefilePath string) {
	logrus.Info("Starting auth loop...")
	var previousToken string
	wait := time.Minute
	for {
		raw, err := ioutil.ReadFile(cookiefilePath)
		if err != nil {
			logrus.WithError(err).Error("Failed to read auth cookie")
		}
		fields := strings.Fields(string(raw))
		token := fields[len(fields)-1]

		if token == previousToken {
			time.Sleep(wait)
			continue
		}

		logrus.Info("New token, updating handlers...")

		// update auth token for each instance
		for instance, handler := range c.handlers {
			handler.authService.SetCookieAuth("o", token)

			self, _, err := handler.accountService.GetAccount("self")
			if err != nil {
				logrus.WithError(err).Error("Failed to auth with token")
				continue
			}
			logrus.Infof("Authentication to %s successful, Username: %s", handler.instance, self.Name)
			c.accounts[instance] = self
		}
		previousToken = token
		time.Sleep(wait)
	}
}

// Start will authenticate the client with gerrit periodically
// Start must be called before user calls any client functions.
func (c *Client) Start(cookiefilePath string) {
	if cookiefilePath != "" {
		go auth(c, cookiefilePath)
	}
}

// QueryChanges queries for all changes from all projects after lastUpdate time
// returns an instance:changes map
func (c *Client) QueryChanges(lastState LastSyncState, rateLimit int) map[string][]ChangeInfo {
	result := map[string][]ChangeInfo{}
	for _, h := range c.handlers {
		lastStateForInstance := lastState[h.instance]
		changes := h.queryAllChanges(lastStateForInstance, rateLimit)
		if len(changes) > 0 {
			result[h.instance] = []ChangeInfo{}
			for _, change := range changes {
				result[h.instance] = append(result[h.instance], change)
			}
		}
	}
	return result
}

// SetReview writes a review comment base on the change id + revision
func (c *Client) SetReview(instance, id, revision, message string, labels map[string]string) error {
	h, ok := c.handlers[instance]
	if !ok {
		return fmt.Errorf("not activated gerrit instance: %s", instance)
	}

	if _, _, err := h.changeService.SetReview(id, revision, &gerrit.ReviewInput{
		Message: message,
		Labels:  labels,
	}); err != nil {
		return fmt.Errorf("cannot comment to gerrit: %v", err)
	}

	return nil
}

// GetBranchRevision returns SHA of HEAD of a branch
func (c *Client) GetBranchRevision(instance, project, branch string) (string, error) {
	h, ok := c.handlers[instance]
	if !ok {
		return "", fmt.Errorf("not activated gerrit instance: %s", instance)
	}

	res, _, err := h.projectService.GetBranch(project, branch)
	if err != nil {
		return "", err
	}

	return res.Revision, nil
}

// Account returns gerrit account for the given instance
func (c *Client) Account(instance string) *gerrit.AccountInfo {
	return c.accounts[instance]
}

// private handler implementation details

func (h *gerritInstanceHandler) queryAllChanges(lastState map[string]time.Time, rateLimit int) []gerrit.ChangeInfo {
	result := []gerrit.ChangeInfo{}
	timeNow := time.Now()
	for _, project := range h.projects {
		lastUpdate, ok := lastState[project]
		if !ok {
			logrus.WithField("project", project).Warnf("could not find lastTime for project %q, probably something went wrong with initTracker?", project)
			lastUpdate = timeNow
		}
		changes, err := h.queryChangesForProject(project, lastUpdate, rateLimit)
		if err != nil {
			// don't halt on error from one project, log & continue
			logrus.WithError(err).Errorf("fail to query changes for project %s", project)
			continue
		}
		result = append(result, changes...)
	}

	return result
}

func parseStamp(value gerrit.Timestamp) time.Time {
	return value.Time
}

func (h *gerritInstanceHandler) queryChangesForProject(project string, lastUpdate time.Time, rateLimit int) ([]gerrit.ChangeInfo, error) {
	pending := []gerrit.ChangeInfo{}

	opt := &gerrit.QueryChangeOptions{}
	opt.Query = append(opt.Query, "project:"+project)
	opt.AdditionalFields = []string{"CURRENT_REVISION", "CURRENT_COMMIT", "CURRENT_FILES", "MESSAGES"}

	start := 0

	for {
		opt.Limit = rateLimit
		opt.Start = start

		// The change output is sorted by the last update time, most recently updated to oldest updated.
		// Gerrit API docs: https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-changes
		changes, _, err := h.changeService.QueryChanges(opt)
		if err != nil {
			// should not happen? Let next sync loop catch up
			return nil, fmt.Errorf("failed to query gerrit changes: %v", err)
		}

		if changes == nil || len(*changes) == 0 {
			logrus.Infof("no more changes from query, returning...")
			return pending, nil
		}

		logrus.Infof("Find %d changes from query %v", len(*changes), opt.Query)

		start += len(*changes)

		for _, change := range *changes {
			// if we already processed this change, then we stop the current sync loop
			updated := parseStamp(change.Updated)

			logrus.Infof("Change %d, last updated %s, status %s", change.Number, change.Updated, change.Status)

			// process if updated later than last updated
			// stop if update was stale
			if updated.After(lastUpdate) {
				switch change.Status {
				case Merged:
					submitted := parseStamp(*change.Submitted)
					if !submitted.After(lastUpdate) {
						logrus.Infof("Change %d, submitted %s before lastUpdate %s, skipping this patchset", change.Number, submitted, lastUpdate)
						continue
					}
					pending = append(pending, change)
				case New:
					// we need to make sure the change update is from a fresh commit change
					rev, ok := change.Revisions[change.CurrentRevision]
					if !ok {
						logrus.WithError(err).Errorf("(should not happen?)cannot find current revision for change %v", change.ID)
						continue
					}

					created := parseStamp(rev.Created)
					changeMessages := change.Messages
					newMessages := false

					for _, message := range changeMessages {
						if message.RevisionNumber == rev.Number {
							messageTime := parseStamp(message.Date)
							if messageTime.After(lastUpdate) {
								logrus.Infof("Change %d: Found a new message %s at time %v after lastSync at %v", change.Number, message.Message, messageTime, lastUpdate)
								newMessages = true
								break
							}
						}
					}

					if !newMessages && !created.After(lastUpdate) {
						// stale commit
						logrus.Infof("Change %d, latest revision updated %s before lastUpdate %s, skipping this patchset", change.Number, created, lastUpdate)
						continue
					}

					pending = append(pending, change)
				default:
					// change has been abandoned, do nothing
				}
			} else {
				logrus.Infof("Change %d, updated %s before lastUpdate %s, return", change.Number, change.Updated, lastUpdate)
				return pending, nil
			}
		}
	}
}
