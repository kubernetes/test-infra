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
	"bytes"
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
	SetUsername(accountID string, input *gerrit.UsernameInput) (*string, *gerrit.Response, error)
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

	logrus.Infof("Authentication successful, Username: %s", self.Name)

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
	changes := c.QueryChanges()

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

func (c *Controller) queryProjectChanges(proj string) ([]gerrit.ChangeInfo, error) {
	pending := []gerrit.ChangeInfo{}

	opt := &gerrit.QueryChangeOptions{}
	opt.Query = append(opt.Query, "project:"+proj+"+status:open")
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
			return pending, fmt.Errorf("failed to query gerrit changes: %v", err)
		}

		logrus.Infof("Find %d changes from query %v", len(*changes), opt.Query)

		if len(*changes) == 0 {
			return pending, nil
		}
		start += len(*changes)

		for _, change := range *changes {
			// if we already processed this change, then we stop the current sync loop
			const layout = "2006-01-02 15:04:05"
			updated, err := time.Parse(layout, change.Updated)
			if err != nil {
				logrus.WithError(err).Errorf("Parse time %v failed", change.Updated)
				continue
			}

			// process if updated later than last updated
			// stop if update was stale
			if updated.After(c.lastUpdate) {
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

				if !created.After(c.lastUpdate) {
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

// QueryChanges will query all valid gerrit changes since controller's last sync loop
func (c *Controller) QueryChanges() []gerrit.ChangeInfo {
	// store a map of changeID:change
	pending := []gerrit.ChangeInfo{}

	// can only query against one project at a time :-(
	for _, proj := range c.projects {
		if res, err := c.queryProjectChanges(proj); err != nil {
			logrus.WithError(err).Errorf("fail to query changes for project %s", proj)
		} else {
			pending = append(pending, res...)
		}
	}

	return pending
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

	type triggeredJob struct {
		Name, URL string
	}
	triggeredJobs := []triggeredJob{}

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
					Ref:    rev.Ref,
				},
			},
		}

		// TODO(krzyzacy): Support AlwaysRun and RunIfChanged
		pj := pjutil.NewProwJob(pjutil.PresubmitSpec(spec, kr), map[string]string{})
		logger.WithFields(pjutil.ProwJobFields(&pj)).Infof("Creating a new prowjob for change %s.", change.Number)
		if _, err := c.kc.CreateProwJob(pj); err != nil {
			logger.WithError(err).Errorf("fail to create prowjob %v", pj)
		} else {
			var b bytes.Buffer
			url := ""
			template := c.ca.Config().Plank.JobURLTemplate
			if template != nil {
				if err := template.Execute(&b, &pj); err != nil {
					logger.WithFields(pjutil.ProwJobFields(&pj)).Errorf("error executing URL template: %v", err)
				}
				// TODO(krzyzacy): We doesn't have buildID here yet - do a hack to get a proper URL to the PR
				// Remove this once we have proper report interface.

				// mangle
				// https://gubernator.k8s.io/build/gob-prow/pr-logs/pull/some/repo/8940/pull-test-infra-presubmit//
				// to
				// https://gubernator.k8s.io/builds/gob-prow/pr-logs/pull/some_repo/8940/pull-test-infra-presubmit/
				url = b.String()
				url = strings.Replace(url, "build", "builds", 1)
				// TODO(krzyzacy): gerrit path can be foo.googlesource.com/bar/baz, which means we took bar/baz as the repo
				// we are mangling the path in bootstrap.py, we need to handle this better in podutils
				url = strings.Replace(url, change.Project, strings.Replace(change.Project, "/", "_", -1), 1)
				url = strings.TrimSuffix(url, "//")
			}
			triggeredJobs = append(triggeredJobs, triggeredJob{Name: spec.Name, URL: url})
		}
	}

	if len(triggeredJobs) > 0 {
		// comment back to gerrit
		message := "Triggered presubmit:"
		for _, job := range triggeredJobs {
			if job.URL != "" {
				message += fmt.Sprintf("\n  * Name: %s, URL: %s", job.Name, job.URL)
			} else {
				message += fmt.Sprintf("\n  * Name: %s", job.Name)
			}
		}

		if _, _, err := c.gc.SetReview(change.ID, change.CurrentRevision, &gerrit.ReviewInput{
			Message: message,
		}); err != nil {
			return fmt.Errorf("cannot comment to gerrit: %v", err)
		}
	}

	return nil
}
