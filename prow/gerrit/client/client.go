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
	"sort"
	"strings"
	"sync"
	"time"

	gerrit "github.com/andygrunwald/go-gerrit"
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
	// GerritPatchset is the numeric ID of the current patchset
	GerritPatchset = "prow.k8s.io/gerrit-patchset"
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
	ListChangeComments(changeID string) (*map[string][]gerrit.CommentInfo, *gerrit.Response, error)
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

	log logrus.FieldLogger
}

// Client holds a instance:handler map
type Client struct {
	handlers map[string]*gerritInstanceHandler
	// map of instance to gerrit account
	accounts map[string]*gerrit.AccountInfo

	authentication func() (string, error)
	lock           sync.RWMutex
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
			log:            logrus.WithField("host", instance),
		}
	}

	return c, nil
}

func (c *Client) authenticateOnce(previousToken string) string {
	c.lock.RLock()
	auth := c.authentication
	c.lock.RUnlock()

	current, err := auth()
	if err != nil {
		logrus.WithError(err).Error("Failed to read gerrit auth token")
	}

	if current == previousToken {
		return current
	}

	c.lock.Lock()
	defer c.lock.Unlock()
	logrus.Info("New gerrit token, updating handler authentication...")

	// update auth token for each instance
	for instance, handler := range c.handlers {
		log := handler.log
		handler.authService.SetCookieAuth("o", current)

		self, _, err := handler.accountService.GetAccount("self")
		if err != nil {
			log.WithError(err).Error("GetAccount() failed with new authentication")
			continue
		}
		log.WithField("name", self.Name).Info("Authentication successful")
		c.accounts[instance] = self
	}
	return current
}

// Authenticate client calls using the specified file.
// Periodically re-reads the file to check for an updated value.
// cookiefilePath takes precedence over tokenPath if both are set.
func (c *Client) Authenticate(cookiefilePath, tokenPath string) {
	var was, auth func() (string, error)
	switch {
	case cookiefilePath != "":
		if tokenPath != "" {
			logrus.WithFields(logrus.Fields{
				"cookiefile": cookiefilePath,
				"token":      tokenPath,
			}).Warn("Ignoring token path in favor of cookiefile")
		}
		auth = func() (string, error) {
			// TODO(fejta): listen for changes
			raw, err := ioutil.ReadFile(cookiefilePath)
			if err != nil {
				return "", fmt.Errorf("read cookie: %w", err)
			}
			fields := strings.Fields(string(raw))
			token := fields[len(fields)-1]
			return token, nil
		}
	case tokenPath != "":
		auth = func() (string, error) {
			raw, err := ioutil.ReadFile(tokenPath)
			if err != nil {
				return "", fmt.Errorf("read token: %w", err)
			}
			return strings.TrimSpace(string(raw)), nil
		}
	default:
		logrus.Info("Using anonymous authentication to gerrit")
		return
	}
	c.lock.Lock()
	was, c.authentication = c.authentication, auth
	c.lock.Unlock()
	logrus.Info("Authenticating gerrit requests...")
	previousToken := c.authenticateOnce("") // Ensure requests immediately authenticated
	if was == nil {
		go func() {
			for {
				previousToken = c.authenticateOnce(previousToken)
				time.Sleep(time.Minute)
			}
		}()
	}
}

// QueryChanges queries for all changes from all projects after lastUpdate time
// returns an instance:changes map
func (c *Client) QueryChanges(lastState LastSyncState, rateLimit int) map[string][]ChangeInfo {
	result := map[string][]ChangeInfo{}
	for _, h := range c.handlers {
		lastStateForInstance := lastState[h.instance]
		changes := h.queryAllChanges(lastStateForInstance, rateLimit)
		if len(changes) == 0 {
			continue
		}

		result[h.instance] = append(result[h.instance], changes...)
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
		log := h.log.WithField("repo", project)
		lastUpdate, ok := lastState[project]
		if !ok {
			lastUpdate = timeNow
			log.WithField("now", timeNow).Warn("lastState not found, defaultint to now")
		}
		changes, err := h.queryChangesForProject(log, project, lastUpdate, rateLimit)
		if err != nil {
			// don't halt on error from one project, log & continue
			log.WithError(err).WithFields(logrus.Fields{
				"lastUpdate": lastUpdate,
				"rateLimit":  rateLimit,
			}).Error("Failed to query changes")
			continue
		}
		result = append(result, changes...)
	}

	return result
}

func parseStamp(value gerrit.Timestamp) time.Time {
	return value.Time
}

func (h *gerritInstanceHandler) injectPatchsetMessages(change *gerrit.ChangeInfo) error {
	out, _, err := h.changeService.ListChangeComments(change.ID)
	if err != nil {
		return err
	}
	outer := *out
	comments, ok := outer["/PATCHSET_LEVEL"]
	if !ok {
		return nil
	}
	var changed bool
	for _, c := range comments {
		change.Messages = append(change.Messages, gerrit.ChangeMessageInfo{
			Author:         c.Author,
			Date:           *c.Updated,
			Message:        c.Message,
			RevisionNumber: c.PatchSet,
		})
		changed = true
	}
	if changed {
		sort.SliceStable(change.Messages, func(i, j int) bool {
			return change.Messages[i].Date.Before(change.Messages[j].Date.Time)
		})
	}
	return nil
}

func (h *gerritInstanceHandler) queryChangesForProject(log logrus.FieldLogger, project string, lastUpdate time.Time, rateLimit int) ([]gerrit.ChangeInfo, error) {
	var pending []gerrit.ChangeInfo

	var opt gerrit.QueryChangeOptions
	opt.Query = append(opt.Query, "project:"+project)
	opt.AdditionalFields = []string{"CURRENT_REVISION", "CURRENT_COMMIT", "CURRENT_FILES", "MESSAGES"}

	var start int

	for {
		opt.Limit = rateLimit
		opt.Start = start

		// The change output is sorted by the last update time, most recently updated to oldest updated.
		// Gerrit API docs: https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-changes
		changes, _, err := h.changeService.QueryChanges(&opt)
		if err != nil {
			// should not happen? Let next sync loop catch up
			return nil, err
		}

		if changes == nil || len(*changes) == 0 {
			log.Info("No more changes")
			return pending, nil
		}

		log.WithField("query", opt.Query).Infof("Found %d changes", len(*changes))

		start += len(*changes)

		for _, change := range *changes {
			// if we already processed this change, then we stop the current sync loop
			updated := parseStamp(change.Updated)

			log := log.WithFields(logrus.Fields{
				"change":     change.Number,
				"updated":    change.Updated,
				"status":     change.Status,
				"lastUpdate": lastUpdate,
			})

			// stop when we find a change last updated before lastUpdate
			if !updated.After(lastUpdate) {
				log.Info("No more recently updated changes")
				return pending, nil
			}

			// process recently updated change
			switch change.Status {
			case Merged:
				submitted := parseStamp(*change.Submitted)
				log := log.WithField("submitted", submitted)
				if !submitted.After(lastUpdate) {
					log.Info("Skipping previously merged change")
					continue
				}
				log.Info("Found merged change")
				pending = append(pending, change)
			case New:
				// we need to make sure the change update is from a fresh commit change
				rev, ok := change.Revisions[change.CurrentRevision]
				if !ok {
					log.WithError(err).WithField("revision", change.CurrentRevision).Error("Revision not found")
					continue
				}

				created := parseStamp(rev.Created)
				log := log.WithField("created", created)
				if err := h.injectPatchsetMessages(&change); err != nil {
					log.WithError(err).Error("Failed to inject patchset messages")
				}
				changeMessages := change.Messages
				var newMessages bool

				for _, message := range changeMessages {
					if message.RevisionNumber == rev.Number {
						messageTime := parseStamp(message.Date)
						if messageTime.After(lastUpdate) {
							log.WithFields(logrus.Fields{
								"message":     message.Message,
								"messageDate": messageTime,
							}).Info("New messages")
							newMessages = true
							break
						}
					}
				}

				if !newMessages && !created.After(lastUpdate) {
					// stale commit
					log.Info("Skipping existing change")
					continue
				}
				if !newMessages {
					log.Info("Found updated change")
				}
				pending = append(pending, change)
			default:
				// change has been abandoned, do nothing
				log.Info("Ignored change")
			}
		}
	}
}
