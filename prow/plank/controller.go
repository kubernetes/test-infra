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

package plank

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/bwmarrin/snowflake"
	uuid "github.com/satori/go.uuid"

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

	CreatePod(kube.Pod) (kube.Pod, error)
	ListPods(map[string]string) ([]kube.Pod, error)
	DeletePod(string) error
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
	kc     kubeClient
	pkc    kubeClient
	ghc    githubClient
	ca     configAgent
	node   *snowflake.Node
	totURL string

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
func NewController(kc, pkc *kube.Client, ghc *github.Client, ca *config.Agent, totURL string) (*Controller, error) {
	n, err := snowflake.NewNode(1)
	if err != nil {
		return nil, err
	}
	return &Controller{
		kc:          kc,
		pkc:         pkc,
		ghc:         ghc,
		ca:          ca,
		node:        n,
		pendingJobs: make(map[string]int),
		lock:        sync.RWMutex{},
		totURL:      totURL,
	}, nil
}

// Sync does one sync iteration.
func (c *Controller) Sync() error {
	pjs, err := c.kc.ListProwJobs(nil)
	if err != nil {
		return fmt.Errorf("error listing prow jobs: %v", err)
	}
	pods, err := c.pkc.ListPods(nil)
	if err != nil {
		return fmt.Errorf("error listing pods: %v", err)
	}
	pm := map[string]kube.Pod{}
	for _, pod := range pods {
		pm[pod.Metadata.Name] = pod
	}

	var k8sJobs []kube.ProwJob
	for _, pj := range pjs {
		if pj.Spec.Agent == kube.KubernetesAgent {
			k8sJobs = append(k8sJobs, pj)
		}
	}
	pjs = k8sJobs

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
		go c.syncProwJob(wg, pjCh, errCh, reportCh, pm)
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

func (c *Controller) syncProwJob(wg *sync.WaitGroup, jobs <-chan kube.ProwJob, syncErrors chan<- error, reports chan<- kube.ProwJob, pm map[string]kube.Pod) {
	defer wg.Done()
	for pj := range jobs {
		if pj.Spec.Agent == kube.KubernetesAgent {
			if err := c.syncKubernetesJob(pj, pm, reports); err != nil {
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

func (c *Controller) syncKubernetesJob(pj kube.ProwJob, pm map[string]kube.Pod, reports chan<- kube.ProwJob) error {
	if pj.Complete() {
		if pj.Status.PodName == "" {
			// Completed ProwJob, already cleaned up the pod. Nothing to do.
			return nil
		} else if _, ok := pm[pj.Status.PodName]; ok {
			// Delete the old pod.
			if err := c.pkc.DeletePod(pj.Status.PodName); err != nil {
				return fmt.Errorf("error deleting pod %s: %v", pj.Status.PodName, err)
			}
		}
		pj.Status.PodName = ""
	} else if pj.Status.PodName == "" {
		// Do not start more jobs than specified.
		numPending := c.getNumPendingJobs(pj.Spec.Job)
		if pj.Spec.MaxConcurrency > 0 && numPending >= pj.Spec.MaxConcurrency {
			logrus.WithField("job", pj.Spec.Job).Infof("Not starting another instance of %s, already %d running.", pj.Spec.Job, numPending)
			return nil
		}

		// We haven't started the pod yet. Do so.
		pj.Status.State = kube.PendingState
		c.incrementNumPendingJobs(pj.Spec.Job)
		if id, pn, err := c.startPod(pj); err == nil {
			pj.Status.PodName = pn
			pj.Status.BuildID = id
			var b bytes.Buffer
			if err := c.ca.Config().Plank.JobURLTemplate.Execute(&b, &pj); err != nil {
				return fmt.Errorf("error executing URL template: %v", err)
			}
			url := b.String()
			pj.Status.URL = url
		} else {
			return fmt.Errorf("error starting pod: %v", err)
		}
		pj.Status.Description = "Job triggered."
		reports <- pj
	} else if pod, ok := pm[pj.Status.PodName]; !ok {
		// Pod is missing. This shouldn't happen normally, but if someone goes
		// in and manually deletes the pod then we'll hit it. Start a new pod.
		pj.Status.PodName = ""
		pj.Status.State = kube.PendingState
	} else if pod.Status.Phase == kube.PodUnknown {
		// Pod is in Unknown state. This can happen if there is a problem with
		// the node. Delete the old pod, we'll start a new one next loop.
		if err := c.pkc.DeletePod(pj.Status.PodName); err != nil {
			return fmt.Errorf("error deleting pod %s: %v", pj.Status.PodName, err)
		}
		pj.Status.PodName = ""
		pj.Status.State = kube.PendingState
	} else if pod.Status.Phase == kube.PodSucceeded {
		// Pod succeeded. Update ProwJob, talk to GitHub, and start next jobs.
		pj.Status.CompletionTime = time.Now()
		pj.Status.State = kube.SuccessState
		pj.Status.Description = "Job succeeded."
		var b bytes.Buffer
		if err := c.ca.Config().Plank.JobURLTemplate.Execute(&b, &pj); err != nil {
			return fmt.Errorf("error executing URL template: %v", err)
		}
		pj.Status.URL = b.String()
		reports <- pj
		for _, nj := range pj.Spec.RunAfterSuccess {
			if _, err := c.kc.CreateProwJob(npj.NewProwJob(nj)); err != nil {
				return fmt.Errorf("error starting next prowjob: %v", err)
			}
		}
	} else if pod.Status.Phase == kube.PodFailed {
		if pod.Status.Reason == kube.Evicted {
			// Pod was evicted. Restart it.
			pj.Status.PodName = ""
			pj.Status.State = kube.PendingState
		} else {
			// Pod failed. Update ProwJob, talk to GitHub.
			pj.Status.CompletionTime = time.Now()
			pj.Status.State = kube.FailureState
			pj.Status.Description = "Job failed."
			var b bytes.Buffer
			if err := c.ca.Config().Plank.JobURLTemplate.Execute(&b, &pj); err != nil {
				return fmt.Errorf("error executing URL template: %v", err)
			}
			pj.Status.URL = b.String()
			reports <- pj
		}
	} else {
		// Pod is running. Do nothing.
		return nil
	}
	_, err := c.kc.ReplaceProwJob(pj.Metadata.Name, pj)
	return err
}

func (c *Controller) startPod(pj kube.ProwJob) (string, string, error) {
	buildID, err := c.getBuildID(pj.Spec.Job)
	if err != nil {
		return "", "", fmt.Errorf("error getting build ID: %v", err)
	}
	env := npj.EnvForSpec(pj.Spec)
	env["BUILD_NUMBER"] = buildID
	kubeEnv := kubeEnv(env)

	spec := pj.Spec.PodSpec
	spec.RestartPolicy = "Never"
	podName := newPodName()

	// Set environment variables in each container in the pod spec. We don't
	// want to update the spec in place, since that will update the ProwJob
	// spec. Instead, create a copy.
	spec.Containers = []kube.Container{}
	for i := range pj.Spec.PodSpec.Containers {
		spec.Containers = append(spec.Containers, pj.Spec.PodSpec.Containers[i])
		spec.Containers[i].Name = fmt.Sprintf("%s-%d", podName, i)
		spec.Containers[i].Env = append(spec.Containers[i].Env, kubeEnv...)
	}
	p := kube.Pod{
		Metadata: kube.ObjectMeta{
			Name: podName,
			Labels: map[string]string{
				kube.CreatedByProw: "true",
			},
		},
		Spec: spec,
	}
	actual, err := c.pkc.CreatePod(p)
	if err != nil {
		return "", "", fmt.Errorf("error creating pod: %v", err)
	}
	return buildID, actual.Metadata.Name, nil
}

// kubeEnv transforms a mapping of environment variables
// into their serialized form for a PodSpec
func kubeEnv(environment map[string]string) []kube.EnvVar {
	var kubeEnvironment []kube.EnvVar
	for key, value := range environment {
		kubeEnvironment = append(kubeEnvironment, kube.EnvVar{
			Name:  key,
			Value: value,
		})
	}

	return kubeEnvironment
}

func (c *Controller) getBuildID(name string) (string, error) {
	if c.totURL == "" {
		return c.node.Generate().String(), nil
	}
	var err error
	url := c.totURL + "/vend/" + name
	for retries := 0; retries < 60; retries++ {
		if retries > 0 {
			time.Sleep(2 * time.Second)
		}
		var resp *http.Response
		resp, err = http.Get(url)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			continue
		}
		if buf, err := ioutil.ReadAll(resp.Body); err == nil {
			return string(buf), nil
		}
		return "", err
	}
	return "", err
}

func (c *Controller) updatePendingJobs(pjs []kube.ProwJob) {
	for _, pj := range pjs {
		if pj.Status.State == kube.PendingState {
			c.incrementNumPendingJobs(pj.Spec.Job)
		}
	}
}

var newPodName = func() string {
	return uuid.NewV1().String()
}
