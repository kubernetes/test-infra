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

// Package gerrit implements a client that can handle multiple gerrit instances
// derived from https://github.com/andygrunwald/go-gerrit
package gerrit

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/andygrunwald/go-gerrit"
	"github.com/sirupsen/logrus"
)

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

// gerritInstanceHandler holds all actual gerrit handlers
type gerritInstanceHandler struct {
	instance string
	projects []string

	auth    gerritAuthentication
	account gerritAccount
	change  gerritChange
}

// Client holds a instance:handler map
type Client struct {
	handlers map[string]*gerritInstanceHandler
}

// ChangeInfo is a gerrit.ChangeInfo
type ChangeInfo = gerrit.ChangeInfo

// RevisionInfo is a gerrit.RevisionInfo
type RevisionInfo = gerrit.RevisionInfo

// NewClient returns a new gerrit client
func NewClient(instances map[string][]string) (*Client, error) {
	c := &Client{
		handlers: map[string]*gerritInstanceHandler{},
	}
	for instance := range instances {
		gc, err := gerrit.NewClient(instance, nil)
		if err != nil {
			return nil, err
		}

		c.handlers[instance] = &gerritInstanceHandler{
			instance: instance,
			projects: instances[instance],
			auth:     gc.Authentication,
			account:  gc.Accounts,
			change:   gc.Changes,
		}
	}

	return c, nil
}

func auth(c *Client) {
	logrus.Info("Starting auth loop...")
	for {
		// TODO(fejta): migrate this to the grandmatriarch

		// look for authdaemon under root dir
		if _, err := os.Stat("/git-cookie-authdaemon"); os.IsNotExist(err) {
			panic("cannot find /git-cookie-authdaemon")
		}

		cmd := exec.Command("/git-cookie-authdaemon")
		if err := cmd.Run(); err != nil {
			logrus.WithError(err).Error("Fail to authenticate to gerrit using git-cookie-authdaemon")
		}

		raw, err := ioutil.ReadFile(filepath.Join(os.Getenv("HOME"), ".git-credential-cache/cookie"))
		if err != nil {
			logrus.WithError(err).Error("Fail to read auth cookie")
		}
		fields := strings.Fields(string(raw))
		token := fields[len(fields)-1]

		// update auth token for each instance
		for _, handler := range c.handlers {
			handler.auth.SetCookieAuth("o", token)

			self, _, err := handler.account.GetAccount("self")
			if err != nil {
				logrus.WithError(err).Error("Fail to auth with token")
			}

			logrus.Infof("Authentication to %s successful, Username: %s", handler.instance, self.Name)
		}

		time.Sleep(10 * time.Minute)
	}
}

// Start will authenticate the client with gerrit periodically
// Start must be called before user calls any client functions.
func (c *Client) Start() {
	go auth(c)
}

// QueryChanges queries for all changes from all projects after lastUpdate time
// returns an instance:changes map
func (c *Client) QueryChanges(lastUpdate time.Time, rateLimit int) map[string][]ChangeInfo {
	result := map[string][]ChangeInfo{}
	for _, h := range c.handlers {
		changes := h.queryAllChanges(lastUpdate, rateLimit)
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
func (c *Client) SetReview(instance, id, revision, message string) error {
	h, ok := c.handlers[instance]
	if !ok {
		return fmt.Errorf("not activated gerrit instance: %s", instance)
	}

	if _, _, err := h.change.SetReview(id, revision, &gerrit.ReviewInput{
		Message: message,
	}); err != nil {
		return fmt.Errorf("cannot comment to gerrit: %v", err)
	}

	return nil
}

// private handler implementation details

func (h *gerritInstanceHandler) queryAllChanges(lastUpdate time.Time, rateLimit int) []gerrit.ChangeInfo {
	result := []gerrit.ChangeInfo{}
	for _, project := range h.projects {
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

func (h *gerritInstanceHandler) queryChangesForProject(project string, lastUpdate time.Time, rateLimit int) ([]gerrit.ChangeInfo, error) {
	pending := []gerrit.ChangeInfo{}

	opt := &gerrit.QueryChangeOptions{}
	opt.Query = append(opt.Query, "project:"+project+"+status:open")
	opt.AdditionalFields = []string{"CURRENT_REVISION", "CURRENT_COMMIT"}

	start := 0

	for {
		opt.Limit = rateLimit
		opt.Start = start

		// The change output is sorted by the last update time, most recently updated to oldest updated.
		// Gerrit API docs: https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-changes
		changes, _, err := h.change.QueryChanges(opt)
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
			const layout = "2006-01-02 15:04:05"
			updated, err := time.Parse(layout, change.Updated)
			if err != nil {
				logrus.WithError(err).Errorf("Parse time %v failed", change.Updated)
				continue
			}

			logrus.Infof("Change %s, last updated %s", change.Number, change.Updated)

			// process if updated later than last updated
			// stop if update was stale
			if updated.After(lastUpdate) {
				// we need to make sure the change update is from a new commit change
				rev, ok := change.Revisions[change.CurrentRevision]
				if !ok {
					logrus.WithError(err).Errorf("(should not happen?)cannot find current revision for change %v", change.ID)
					continue
				}

				created, err := time.Parse(layout, rev.Created)
				if err != nil {
					logrus.WithError(err).Errorf("Parse time %v failed", rev.Created)
					continue
				}

				if !created.After(lastUpdate) {
					// stale commit
					continue
				}

				pending = append(pending, change)
			} else {
				return pending, nil
			}
		}
	}
}
