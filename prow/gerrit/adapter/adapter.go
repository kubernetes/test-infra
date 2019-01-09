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
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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
	QueryChanges(lastUpdate time.Time, rateLimit int) map[string][]client.ChangeInfo
	GetBranchRevision(instance, project, branch string) (string, error)
	SetReview(instance, id, revision, message string, labels map[string]string) error
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
func NewController(lastSyncFallback, cookiefilePath string, projects map[string][]string, kc *kube.Client, ca *config.Agent) (*Controller, error) {
	if lastSyncFallback == "" {
		return nil, errors.New("empty lastSyncFallback")
	}

	var lastUpdate time.Time
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

	c, err := client.NewClient(projects)
	if err != nil {
		return nil, err
	}
	c.Start(cookiefilePath)

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
// and creates prowjobs according to specs
func (c *Controller) Sync() error {
	// gerrit timestamp only has second precision
	syncTime := time.Now().Truncate(time.Second)

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

func makeCloneURI(instance, project string) (*url.URL, error) {
	u, err := url.Parse(instance)
	if err != nil {
		return nil, fmt.Errorf("instance %s is not a url: %v", instance, err)
	}
	if u.Host == "" {
		return nil, errors.New("instance does not set host")
	}
	if u.Path != "" {
		return nil, errors.New("instance cannot set path (this is set by project)")
	}
	u.Path = project
	return u, nil
}

// listChangedFiles lists (in lexicographic order) the files changed as part of a Gerrit patchset
func listChangedFiles(changeInfo client.ChangeInfo) []string {
	changed := []string{}
	revision := changeInfo.Revisions[changeInfo.CurrentRevision]
	for file := range revision.Files {
		changed = append(changed, file)
	}
	return changed
}

// ProcessChange creates new presubmit prowjobs base off the gerrit changes
func (c *Controller) ProcessChange(instance string, change client.ChangeInfo) error {
	rev, ok := change.Revisions[change.CurrentRevision]
	if !ok {
		return fmt.Errorf("cannot find current revision for change %v", change.ID)
	}

	logger := logrus.WithField("gerrit change", change.Number)

	cloneURI, err := makeCloneURI(instance, change.Project)
	if err != nil {
		return fmt.Errorf("failed to create clone uri: %v", err)
	}

	baseSHA, err := c.gc.GetBranchRevision(instance, change.Project, change.Branch)
	if err != nil {
		return fmt.Errorf("failed to get SHA from base branch: %v", err)
	}

	triggeredJobs := []string{}

	kr := kube.Refs{
		Org:      cloneURI.Host,  // Something like android.googlesource.com
		Repo:     change.Project, // Something like platform/build
		BaseRef:  change.Branch,
		BaseSHA:  baseSHA,
		CloneURI: cloneURI.String(), // Something like https://android.googlesource.com/platform/build
		Pulls: []kube.Pull{
			{
				Number: change.Number,
				Author: rev.Commit.Author.Name,
				SHA:    change.CurrentRevision,
				Ref:    rev.Ref,
			},
		},
	}

	type jobSpec struct {
		spec   kube.ProwJobSpec
		labels map[string]string
	}

	var jobSpecs []jobSpec

	changedFiles := listChangedFiles(change)

	switch change.Status {
	case client.Merged:
		postsubmits := c.ca.Config().Postsubmits[cloneURI.String()]
		postsubmits = append(postsubmits, c.ca.Config().Postsubmits[cloneURI.Host+"/"+cloneURI.Path]...)
		for _, postsubmit := range postsubmits {
			if postsubmit.RunsAgainstChanges(changedFiles) {
				jobSpecs = append(jobSpecs, jobSpec{
					spec:   pjutil.PostsubmitSpec(postsubmit, kr),
					labels: postsubmit.Labels,
				})
			}
		}
	case client.New:
		presubmits := c.ca.Config().Presubmits[cloneURI.String()]
		presubmits = append(presubmits, c.ca.Config().Presubmits[cloneURI.Host+"/"+cloneURI.Path]...)
		for _, presubmit := range presubmits {
			if presubmit.RunsAgainstChanges(changedFiles) {
				jobSpecs = append(jobSpecs, jobSpec{
					spec:   pjutil.PresubmitSpec(presubmit, kr),
					labels: presubmit.Labels,
				})
			}
		}
	}

	annotations := map[string]string{
		client.GerritID:       change.ID,
		client.GerritInstance: instance,
	}

	for _, jSpec := range jobSpecs {
		labels := make(map[string]string)
		for k, v := range jSpec.labels {
			labels[k] = v
		}
		labels[client.GerritRevision] = change.CurrentRevision

		pj := pjutil.NewProwJobWithAnnotation(jSpec.spec, labels, annotations)
		if _, err := c.kc.CreateProwJob(pj); err != nil {
			logger.WithError(err).Errorf("fail to create prowjob %v", pj)
		} else {
			triggeredJobs = append(triggeredJobs, jSpec.spec.Job)
		}
	}

	if len(triggeredJobs) > 0 {
		// comment back to gerrit
		message := fmt.Sprintf("Triggered %d prow jobs:", len(triggeredJobs))
		for _, job := range triggeredJobs {
			message += fmt.Sprintf("\n  * Name: %s", job)
		}

		if err := c.gc.SetReview(instance, change.ID, change.CurrentRevision, message, nil); err != nil {
			return err
		}
	}

	return nil
}
