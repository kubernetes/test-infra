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

// Package gerrit implements a gerrit-fetcher using https://github.com/andygrunwald/go-gerrit
package gerrit

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/andygrunwald/go-gerrit"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
)

type kubeClient interface {
	CreateProwJob(kube.ProwJob) (kube.ProwJob, error)
}

type gerritAuthentication interface {
	SetCookieAuth(name, value string)
}

type gerritAccount interface {
	GetAccount(name string) (*gerrit.AccountInfo, *gerrit.Response, error)
	SetAccountName(accountID string, input *gerrit.AccountNameInput) (*string, *gerrit.Response, error)
}

type gerritChange interface {
	QueryChanges(opt *gerrit.QueryChangeOptions) (*[]gerrit.ChangeInfo, *gerrit.Response, error)
	SetReview(changeID, revisionID string, input *gerrit.ReviewInput) (*gerrit.ReviewResult, *gerrit.Response, error)
}

type configAgent interface {
	Config() *config.Config
}

// Controller manages gerrit changes.
type Controller struct {
	ca configAgent

	// go-gerrit change endpoint client
	auth     gerritAuthentication
	account  gerritAccount
	gc       gerritChange
	instance string
	storage  string
	projects []string

	kc kubeClient

	lastUpdate time.Time
}

// NewController returns a new gerrit controller client
func NewController(instance, storage string, projects []string, kc *kube.Client, ca *config.Agent) (*Controller, error) {
	lastUpdate := time.Now()
	if storage != "" {
		buf, err := ioutil.ReadFile(storage)
		if err == nil {
			unix, err := strconv.ParseInt(string(buf), 10, 64)
			if err != nil {
				return nil, err
			} else {
				lastUpdate = time.Unix(unix, 0)
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		// fallback to time.Now() if file does not exist yet
	}

	c, err := gerrit.NewClient(instance, nil)
	if err != nil {
		return nil, err
	}

	return &Controller{
		instance:   instance,
		projects:   projects,
		kc:         kc,
		ca:         ca,
		auth:       c.Authentication,
		account:    c.Accounts,
		gc:         c.Changes,
		lastUpdate: lastUpdate,
		storage:    storage,
	}, nil
}

// Auth authenticates to gerrit server
// Token will expire, so we need to regenerate it once so often
func (c *Controller) Auth() error {
	cmd := exec.Command("python", "./git-cookie-authdaemon")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Fail to authenticate to gerrit using git-cookie-authdaemon : %v", err)
	}

	raw, err := ioutil.ReadFile(filepath.Join(os.Getenv("HOME"), ".git-credential-cache/cookie"))
	if err != nil {
		return err
	}
	fields := strings.Fields(string(raw))
	token := fields[len(fields)-1]

	c.auth.SetCookieAuth("o", token)

	self, _, err := c.account.GetAccount("self")
	if err != nil {
		logrus.WithError(err).Errorf("Fail to auth with token: %s", token)
		return err
	}

	// set account name
	// TODO(krzyzacy): this call needs specific permission from gerrit
	account := strconv.Itoa(self.AccountID)
	name, _, err := c.account.SetAccountName(account, &gerrit.AccountNameInput{Name: "gerrit-prow-robot"})
	if err != nil {
		logrus.WithError(err).Errorf("Fail to set account name for: %s", account)
	}

	logrus.Infof("Authentication successful, Username: %s", *name)
	return nil
}

// SaveLastSync saves last sync time in Unix to a volume
func (c *Controller) SaveLastSync(lastSync time.Time) error {
	if c.storage == "" {
		return nil
	}

	lastSyncUnix := strconv.FormatInt(lastSync.Unix(), 10)
	logrus.Infof("Writing last sync: %s", lastSyncUnix)

	err := ioutil.WriteFile(c.storage+".tmp", []byte(lastSyncUnix), 0644)
	if err != nil {
		return err
	}
	return os.Rename(c.storage+".tmp", c.storage)
}

// Sync looks for newly made gerrit changes
// and creates prowjobs according to presubmit specs
func (c *Controller) Sync() error {
	syncTime := time.Now()
	changes, err := c.QueryChanges()
	if err != nil {
		return fmt.Errorf("failed query changes : %v", err)
	}

	for _, change := range changes {
		if err := c.ProcessChange(change); err != nil {
			logrus.WithError(err).Errorf("Failed process change %v", change.CurrentRevision)
		}
	}

	c.lastUpdate = syncTime
	if err := c.SaveLastSync(syncTime); err != nil {
		logrus.WithError(err).Errorf("last sync %v, cannot save to path %v", syncTime, c.storage)
	}
	logrus.Infof("Processed %d changes", len(changes))
	return nil
}

// QueryChanges will query all gerrit changes since controller's last sync loop
func (c *Controller) QueryChanges() (map[string]gerrit.ChangeInfo, error) {
	// store a map of changeID:change
	pending := map[string]gerrit.ChangeInfo{}

	// can only query against one project at a time :-(
	for _, proj := range c.projects {
		opt := &gerrit.QueryChangeOptions{}
		opt.Query = append(opt.Query, "project:"+proj+"+status:open")
		//opt.Query = append(opt.Query, )
		opt.AdditionalFields = []string{"CURRENT_REVISION", "CURRENT_COMMIT"}

		start := 0

		for {
			opt.Limit = c.ca.Config().Gerrit.RateLimit
			opt.Start = start

			// The change output is sorted by the last update time, most recently updated to oldest updated.
			// Gerrit API docs: https://gerrit-review.googlesource.com/Documentation/rest-api-changes.html#list-changes
			changes, _, err := c.gc.QueryChanges(opt)
			if err != nil {
				// should not happen? Let next sync loop catch up
				logrus.WithError(err).Errorf("failed to query gerrit changes: %v", err)
				break
			}

			logrus.Infof("Find %d changes from query %v", len(*changes), opt.Query)

			if len(*changes) == 0 {
				break
			}
			start += len(*changes)

			for _, change := range *changes {
				// if we already processed this change, then we stop the current sync loop
				const layout = "2006-01-02 15:04:05"
				updated, err := time.Parse(layout, change.Updated)
				if err != nil {
					logrus.WithError(err).Error("Parse time %v failed", change.Updated)
					continue
				}

				// process if updated later than last updated
				// stop if already parsed
				if updated.After(c.lastUpdate) {
					// here we use changeID as the key, since multiple revisions can occur for the same change
					// and since we sorted by recent timestamp, first change will be the most recent revision
					if _, ok := pending[change.ID]; !ok {
						pending[change.ID] = change
					}
				} else {
					break
				}
			}
		}
	}

	return pending, nil
}

// ProcessChange creates new presubmit prowjobs base off the gerrit changes
func (c *Controller) ProcessChange(change gerrit.ChangeInfo) error {
	rev, ok := change.Revisions[change.CurrentRevision]
	if !ok {
		return fmt.Errorf("cannot find current revision for change %v", change.ID)
	}

	parentSHA := ""
	if len(rev.Commit.Parents) > 0 {
		parentSHA = rev.Commit.Parents[0].Commit
	}

	logger := logrus.WithField("gerrit change", change.Number)
	triggered := []string{}

	for _, spec := range c.ca.Config().Presubmits[c.instance+"/"+change.Project] {
		kr := kube.Refs{
			Org:     c.instance,
			Repo:    change.Project,
			BaseRef: change.Branch,
			BaseSHA: parentSHA,
			Pulls: []kube.Pull{
				{
					Number: change.Number,
					Author: rev.Commit.Author.Name,
					SHA:    change.CurrentRevision,
				},
			},
		}

		// TODO(krzyzacy): Support AlwaysRun and RunIfChanged
		pj := pjutil.NewProwJob(pjutil.PresubmitSpec(spec, kr), map[string]string{})
		logger.WithFields(pjutil.ProwJobFields(&pj)).Infof("Creating a new prowjob for change %s.", change.Number)
		if _, err := c.kc.CreateProwJob(pj); err != nil {
			logger.WithError(err).Errorf("fail to create prowjob %v", pj)
		} else {
			triggered = append(triggered, spec.Name)
		}
	}

	if len(triggered) > 0 {
		// comment back to gerrit
		if _, _, err := c.gc.SetReview(change.ID, change.CurrentRevision, &gerrit.ReviewInput{
			Message: fmt.Sprintf("Triggered presubmit: %v", triggered),
		}); err != nil {
			return fmt.Errorf("cannot comment to gerrit: %v", err)
		}
	}

	return nil
}
