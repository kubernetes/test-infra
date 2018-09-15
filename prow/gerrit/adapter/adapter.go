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

// Package adapter implements a controller that interacts with gerrit instances
package adapter

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/gerrit/client"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
)

type kubeClient interface {
	CreateProwJob(kube.ProwJob) (kube.ProwJob, error)
}

type gerritClient interface {
	QueryChanges(lastUpdate time.Time, rateLimit int) map[string][]gerrit.ChangeInfo
	SetReview(instance, id, revision, message string) error
}

type configAgent interface {
	Config() *config.Config
}

// Controller manages gerrit changes.
type Controller struct {
	ca configAgent
	kc kubeClient
	gc gerritClient

	lastSyncFallback string

	lastUpdate time.Time
}

// NewController returns a new gerrit controller client
func NewController(lastSyncFallback string, projects map[string][]string, kc *kube.Client, ca *config.Agent) (*Controller, error) {
	var lastUpdate time.Time
	if lastSyncFallback != "" {
		if buf, err := ioutil.ReadFile(lastSyncFallback); err == nil {
			unix, err := strconv.ParseInt(string(buf), 10, 64)
			if err != nil {
				return nil, err
			}
			lastUpdate = time.Unix(unix, 0)
		} else if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read lastSyncFallback: %v", err)
		} else {
			logrus.Warnf("lastSyncFallback not found: %s", lastSyncFallback)
			lastUpdate = time.Now()
		}
	} else {
		lastUpdate = time.Now()
	}

	c, err := gerrit.NewClient(projects)
	if err != nil {
		return nil, err
	}
	c.Start()

	return &Controller{
		kc:               kc,
		ca:               ca,
		gc:               c,
		lastUpdate:       lastUpdate,
		lastSyncFallback: lastSyncFallback,
	}, nil
}

func copyFile(srcPath, destPath string) error {
	// fallback to copying the file instead
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	dst, err := os.OpenFile(destPath, os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	if err != nil {
		return err
	}
	dst.Sync()
	dst.Close()
	src.Close()
	return nil
}

// SaveLastSync saves last sync time in Unix to a volume
func (c *Controller) SaveLastSync(lastSync time.Time) error {
	if c.lastSyncFallback == "" {
		return nil
	}

	lastSyncUnix := strconv.FormatInt(lastSync.Unix(), 10)
	logrus.Infof("Writing last sync: %s", lastSyncUnix)

	tempFile, err := ioutil.TempFile(filepath.Dir(c.lastSyncFallback), "temp")
	if err != nil {
		return err
	}
	defer os.Remove(tempFile.Name())

	err = ioutil.WriteFile(tempFile.Name(), []byte(lastSyncUnix), 0644)
	if err != nil {
		return err
	}

	err = os.Rename(tempFile.Name(), c.lastSyncFallback)
	if err != nil {
		logrus.WithError(err).Info("Rename failed, fallback to copyfile")
		return copyFile(tempFile.Name(), c.lastSyncFallback)
	}
	return nil
}

// Sync looks for newly made gerrit changes
// and creates prowjobs according to presubmit specs
func (c *Controller) Sync() error {
	syncTime := time.Now()

	for instance, changes := range c.gc.QueryChanges(c.lastUpdate, c.ca.Config().Gerrit.RateLimit) {
		for _, change := range changes {
			if err := c.ProcessChange(instance, change); err != nil {
				logrus.WithError(err).Errorf("Failed process change %v", change.CurrentRevision)
			}
		}

		logrus.Infof("Processed %d changes for instance %s", len(changes), instance)
	}

	c.lastUpdate = syncTime
	if err := c.SaveLastSync(syncTime); err != nil {
		logrus.WithError(err).Errorf("last sync %v, cannot save to path %v", syncTime, c.lastSyncFallback)
	}

	return nil
}

// ProcessChange creates new presubmit prowjobs base off the gerrit changes
func (c *Controller) ProcessChange(instance string, change gerrit.ChangeInfo) error {
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

	for _, spec := range c.ca.Config().Presubmits[instance+"/"+change.Project] {
		kr := kube.Refs{
			Org:      instance,
			Repo:     change.Project, // Something like https;//android.googlesource.com
			BaseRef:  change.Branch,  // Something like platform/build
			BaseSHA:  parentSHA,
			CloneURI: filepath.Join(change.Project, change.Branch), // Something like https://android.googlesource.com/platform/build
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

		if err := c.gc.SetReview(instance, change.ID, change.CurrentRevision, message); err != nil {
			return err
		}
	}

	return nil
}
