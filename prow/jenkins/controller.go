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

package jenkins

import (
	"bytes"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/npj"
	reportlib "k8s.io/test-infra/prow/report"
)

const (
	testInfra = "https://github.com/kubernetes/test-infra/issues"

	// maxSyncRoutines is the maximum number of goroutines
	// that will be active at any one time for the sync
	maxSyncRoutines = 20
)

type kubeClient interface {
	CreateProwJob(kube.ProwJob) (kube.ProwJob, error)
	ListProwJobs(map[string]string) ([]kube.ProwJob, error)
	ReplaceProwJob(string, kube.ProwJob) (kube.ProwJob, error)
}

type jenkinsClient interface {
	Build(BuildRequest) (*Build, error)
	Enqueued(string) (bool, error)
	Status(job, id string) (*Status, error)
}

type githubClient interface {
	BotName() (string, error)
	CreateStatus(org, repo, ref string, s github.Status) error
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
	CreateComment(org, repo string, number int, comment string) error
	DeleteComment(org, repo string, ID int) error
	EditComment(org, repo string, ID int, comment string) error
}

type configAgent interface {
	Config() *config.Config
}

// Controller manages ProwJobs.
type Controller struct {
	kc  kubeClient
	jc  jenkinsClient
	ghc githubClient
	ca  configAgent

	pendingJobs map[string]int
	lock        sync.RWMutex
}

// getNumPendingJobs retrieves the number of pending
// ProwJobs for a given job identifier
func (c *Controller) getNumPendingJobs(key string) int {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.pendingJobs[key]
}

// incrementNumPendingJobs increments the amount of
// pending ProwJobs for the given job identifier
func (c *Controller) incrementNumPendingJobs(job string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.pendingJobs[job] = c.pendingJobs[job] + 1
}

// setPending sets the currently pending set of jobs.
// This is NOT thread safe and should only be used for
// initializing the controller during testing.
func (c *Controller) setPending(pendingJobs map[string]int) {
	if pendingJobs == nil {
		c.pendingJobs = make(map[string]int)
	} else {
		c.pendingJobs = pendingJobs
	}
}

// NewController creates a new Controller from the provided clients.
func NewController(kc *kube.Client, jc *Client, ghc *github.Client, ca *config.Agent) (*Controller, error) {
	return &Controller{
		kc:          kc,
		jc:          jc,
		ghc:         ghc,
		ca:          ca,
		pendingJobs: make(map[string]int),
		lock:        sync.RWMutex{},
	}, nil
}

// Sync does one sync iteration.
func (c *Controller) Sync() error {
	pjs, err := c.kc.ListProwJobs(nil)
	if err != nil {
		return fmt.Errorf("error listing prow jobs: %v", err)
	}
	var jenkinsJobs []kube.ProwJob
	for _, pj := range pjs {
		if pj.Spec.Agent == kube.JenkinsAgent {
			jenkinsJobs = append(jenkinsJobs, pj)
		}
	}
	pjs = jenkinsJobs

	c.updatePendingJobs(pjs)
	var syncErrs []error
	if err := c.terminateDupes(pjs); err != nil {
		syncErrs = append(syncErrs, err)
	}

	pjCh := make(chan kube.ProwJob, len(pjs))
	for _, pj := range pjs {
		pjCh <- pj
	}
	close(pjCh)

	errCh := make(chan error, len(pjs))
	reportCh := make(chan kube.ProwJob, len(pjs))
	wg := &sync.WaitGroup{}
	wg.Add(maxSyncRoutines)
	for i := 0; i < maxSyncRoutines; i++ {
		go c.syncProwJob(wg, pjCh, errCh, reportCh)
	}
	wg.Wait()
	close(errCh)
	close(reportCh)

	for err := range errCh {
		syncErrs = append(syncErrs, err)
	}

	var reportErrs []error
	for report := range reportCh {
		if err := reportlib.Report(c.ghc, c.ca, report); err != nil {
			reportErrs = append(reportErrs, err)
		}
	}

	if len(syncErrs) == 0 && len(reportErrs) == 0 {
		return nil
	}
	return fmt.Errorf("errors syncing: %v, errors reporting: %v", syncErrs, reportErrs)
}

func (c *Controller) syncProwJob(wg *sync.WaitGroup, jobs <-chan kube.ProwJob, syncErrors chan<- error, reports chan<- kube.ProwJob) {
	defer wg.Done()
	for pj := range jobs {
		if pj.Spec.Agent == kube.JenkinsAgent {
			if err := c.syncJenkinsJob(pj, reports); err != nil {
				syncErrors <- err
			}
		}
	}
}

// terminateDupes aborts presubmits that have a newer version. It modifies pjs
// in-place when it aborts.
func (c *Controller) terminateDupes(pjs []kube.ProwJob) error {
	// "job org/repo#number" -> newest job
	dupes := make(map[string]kube.ProwJob)
	for i, pj := range pjs {
		if pj.Complete() || pj.Spec.Type != kube.PresubmitJob {
			continue
		}
		n := fmt.Sprintf("%s %s/%s#%d", pj.Spec.Job, pj.Spec.Refs.Org, pj.Spec.Refs.Repo, pj.Spec.Refs.Pulls[0].Number)
		prev, ok := dupes[n]
		if !ok {
			dupes[n] = pj
			continue
		}
		toCancel := pj
		if prev.Status.StartTime.Before(pj.Status.StartTime) {
			toCancel = prev
			dupes[n] = pj
		}
		toCancel.Status.CompletionTime = time.Now()
		toCancel.Status.State = kube.AbortedState
		npj, err := c.kc.ReplaceProwJob(toCancel.Metadata.Name, toCancel)
		if err != nil {
			return err
		}
		pjs[i] = npj
	}
	return nil
}

func (c *Controller) syncJenkinsJob(pj kube.ProwJob, reports chan<- kube.ProwJob) error {
	if c.jc == nil {
		return fmt.Errorf("jenkins client nil, not syncing job %s", pj.Metadata.Name)
	}

	var jerr error
	if pj.Complete() {
		return nil
	} else if pj.Status.State == kube.TriggeredState {
		// Do not start more jobs than specified.
		numPending := c.getNumPendingJobs(pj.Spec.Job)
		if pj.Spec.MaxConcurrency > 0 && numPending >= pj.Spec.MaxConcurrency {
			logrus.WithField("job", pj.Spec.Job).Infof("Not starting another instance of %s, already %d running.", pj.Spec.Job, numPending)
			return nil
		}

		// Start the Jenkins job.
		pj.Status.State = kube.PendingState
		c.incrementNumPendingJobs(pj.Spec.Job)
		env := npj.EnvForSpec(pj.Spec)

		br := BuildRequest{
			JobName:     pj.Spec.Job,
			Refs:        pj.Spec.Refs.String(),
			Environment: env,
		}
		if build, err := c.jc.Build(br); err != nil {
			jerr = fmt.Errorf("error starting Jenkins job for prowjob %s: %v", pj.Metadata.Name, err)
			pj.Status.CompletionTime = time.Now()
			pj.Status.State = kube.ErrorState
			pj.Status.URL = testInfra
			pj.Status.Description = "Error starting Jenkins job."
		} else {
			pj.Status.JenkinsQueueURL = build.QueueURL.String()
			pj.Status.JenkinsBuildID = build.ID
			pj.Status.JenkinsEnqueued = true
			pj.Status.Description = "Jenkins job triggered."
		}
		reports <- pj
	} else if pj.Status.JenkinsEnqueued {
		if eq, err := c.jc.Enqueued(pj.Status.JenkinsQueueURL); err != nil {
			jerr = fmt.Errorf("error checking queue status for prowjob %s: %v", pj.Metadata.Name, err)
			pj.Status.JenkinsEnqueued = false
			pj.Status.CompletionTime = time.Now()
			pj.Status.State = kube.ErrorState
			pj.Status.URL = testInfra
			pj.Status.Description = "Error checking queue status."
			reports <- pj
		} else if eq {
			// Still in queue.
			return nil
		} else {
			pj.Status.JenkinsEnqueued = false
		}
	} else if status, err := c.jc.Status(pj.Spec.Job, pj.Status.JenkinsBuildID); err != nil {
		jerr = fmt.Errorf("error checking build status for prowjob %s: %v", pj.Metadata.Name, err)
		pj.Status.CompletionTime = time.Now()
		pj.Status.State = kube.ErrorState
		pj.Status.URL = testInfra
		pj.Status.Description = "Error checking job status."
		reports <- pj
	} else {
		pj.Status.BuildID = strconv.Itoa(status.Number)
		var b bytes.Buffer
		if err := c.ca.Config().Plank.JobURLTemplate.Execute(&b, &pj); err != nil {
			return fmt.Errorf("error executing URL template: %v", err)
		}
		url := b.String()
		if pj.Status.URL != url {
			pj.Status.URL = url
			pj.Status.PodName = fmt.Sprintf("%s-%d", pj.Spec.Job, status.Number)
		} else if status.Building {
			// Build still going.
			return nil
		}
		if !status.Building && status.Success {
			pj.Status.CompletionTime = time.Now()
			pj.Status.State = kube.SuccessState
			pj.Status.Description = "Jenkins job succeeded."
			for _, nj := range pj.Spec.RunAfterSuccess {
				if _, err := c.kc.CreateProwJob(npj.NewProwJob(nj)); err != nil {
					return fmt.Errorf("error starting next prowjob: %v", err)
				}
			}
		} else if !status.Building {
			pj.Status.CompletionTime = time.Now()
			pj.Status.State = kube.FailureState
			pj.Status.Description = "Jenkins job failed."
		}
		reports <- pj
	}
	_, rerr := c.kc.ReplaceProwJob(pj.Metadata.Name, pj)
	if rerr != nil || jerr != nil {
		return fmt.Errorf("jenkins error: %v, error replacing prow job: %v", jerr, rerr)
	}
	return nil
}

func (c *Controller) updatePendingJobs(pjs []kube.ProwJob) {
	for _, pj := range pjs {
		if pj.Status.State == kube.PendingState {
			c.incrementNumPendingJobs(pj.Spec.Job)
		}
	}
}
