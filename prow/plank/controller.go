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
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pod-utils/decorate"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	reportlib "k8s.io/test-infra/prow/report"
)

const (
	testInfra = "https://github.com/kubernetes/test-infra/issues"
)

type kubeClient interface {
	CreateProwJob(kube.ProwJob) (kube.ProwJob, error)
	ListProwJobs(string) ([]kube.ProwJob, error)
	ReplaceProwJob(string, kube.ProwJob) (kube.ProwJob, error)

	CreatePod(v1.Pod) (kube.Pod, error)
	ListPods(string) ([]kube.Pod, error)
	DeletePod(string) error
}

// GitHubClient contains the methods used by plank on k8s.io/test-infra/prow/github.Client
// Plank's unit tests implement a fake of this.
type GitHubClient interface {
	BotName() (string, error)
	CreateStatus(org, repo, ref string, s github.Status) error
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
	CreateComment(org, repo string, number int, comment string) error
	DeleteComment(org, repo string, ID int) error
	EditComment(org, repo string, ID int, comment string) error
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
}

type configAgent interface {
	Config() *config.Config
}

// TODO: Dry this out
type syncFn func(pj kube.ProwJob, pm map[string]kube.Pod, reports chan<- kube.ProwJob) error

// Controller manages ProwJobs.
type Controller struct {
	kc     kubeClient
	pkcs   map[string]kubeClient
	ghc    GitHubClient
	log    *logrus.Entry
	ca     configAgent
	totURL string
	// selector that will be applied on prowjobs and pods.
	selector string

	lock sync.RWMutex
	// pendingJobs is a short-lived cache that helps in limiting
	// the maximum concurrency of jobs.
	pendingJobs map[string]int

	pjLock sync.RWMutex
	// shared across the controller and a goroutine that gathers metrics.
	pjs []kube.ProwJob

	// if skip report job results to github
	skipReport bool
}

// NewController creates a new Controller from the provided clients.
func NewController(kc *kube.Client, pkcs map[string]*kube.Client, ghc GitHubClient, logger *logrus.Entry, ca *config.Agent, totURL, selector string, skipReport bool) (*Controller, error) {
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}
	buildClusters := map[string]kubeClient{}
	for alias, client := range pkcs {
		buildClusters[alias] = kubeClient(client)
	}
	return &Controller{
		kc:          kc,
		pkcs:        buildClusters,
		ghc:         ghc,
		log:         logger,
		ca:          ca,
		pendingJobs: make(map[string]int),
		totURL:      totURL,
		selector:    selector,
		skipReport:  skipReport,
	}, nil
}

// canExecuteConcurrently checks whether the provided ProwJob can
// be executed concurrently.
func (c *Controller) canExecuteConcurrently(pj *kube.ProwJob) bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	if max := c.ca.Config().Plank.MaxConcurrency; max > 0 {
		var running int
		for _, num := range c.pendingJobs {
			running += num
		}
		if running >= max {
			c.log.WithFields(pjutil.ProwJobFields(pj)).Debugf("Not starting another job, already %d running.", running)
			return false
		}
	}

	if pj.Spec.MaxConcurrency == 0 {
		c.pendingJobs[pj.Spec.Job]++
		return true
	}

	numPending := c.pendingJobs[pj.Spec.Job]
	if numPending >= pj.Spec.MaxConcurrency {
		c.log.WithFields(pjutil.ProwJobFields(pj)).Debugf("Not starting another instance of %s, already %d running.", pj.Spec.Job, numPending)
		return false
	}
	c.pendingJobs[pj.Spec.Job]++
	return true
}

// incrementNumPendingJobs increments the amount of
// pending ProwJobs for the given job identifier
func (c *Controller) incrementNumPendingJobs(job string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.pendingJobs[job]++
}

// Sync does one sync iteration.
func (c *Controller) Sync() error {
	pjs, err := c.kc.ListProwJobs(c.selector)
	if err != nil {
		return fmt.Errorf("error listing prow jobs: %v", err)
	}
	selector := fmt.Sprintf("%s=true", kube.CreatedByProw)
	if len(c.selector) > 0 {
		selector = strings.Join([]string{c.selector, selector}, ",")
	}

	pm := map[string]kube.Pod{}
	for alias, client := range c.pkcs {
		pods, err := client.ListPods(selector)
		if err != nil {
			return fmt.Errorf("error listing pods in cluster %q: %v", alias, err)
		}
		for _, pod := range pods {
			pm[pod.ObjectMeta.Name] = pod
		}
	}
	// TODO: Replace the following filtering with a field selector once CRDs support field selectors.
	// https://github.com/kubernetes/kubernetes/issues/53459
	var k8sJobs []kube.ProwJob
	for _, pj := range pjs {
		if pj.Spec.Agent == kube.KubernetesAgent {
			k8sJobs = append(k8sJobs, pj)
		}
	}
	pjs = k8sJobs

	var syncErrs []error
	if err := c.terminateDupes(pjs, pm); err != nil {
		syncErrs = append(syncErrs, err)
	}

	// Share what we have for gathering metrics.
	c.pjLock.Lock()
	c.pjs = pjs
	c.pjLock.Unlock()

	pendingCh, triggeredCh := pjutil.PartitionActive(pjs)
	errCh := make(chan error, len(pjs))
	reportCh := make(chan kube.ProwJob, len(pjs))

	// Reinstantiate on every resync of the controller instead of trying
	// to keep this in sync with the state of the world.
	c.pendingJobs = make(map[string]int)
	// Sync pending jobs first so we can determine what is the maximum
	// number of new jobs we can trigger when syncing the non-pendings.
	maxSyncRoutines := c.ca.Config().Plank.MaxGoroutines
	c.log.Debugf("Handling %d pending prowjobs", len(pendingCh))
	syncProwJobs(c.log, c.syncPendingJob, maxSyncRoutines, pendingCh, reportCh, errCh, pm)
	c.log.Debugf("Handling %d triggered prowjobs", len(triggeredCh))
	syncProwJobs(c.log, c.syncTriggeredJob, maxSyncRoutines, triggeredCh, reportCh, errCh, pm)

	close(errCh)
	close(reportCh)

	for err := range errCh {
		syncErrs = append(syncErrs, err)
	}

	var reportErrs []error
	if !c.skipReport {
		reportTemplate := c.ca.Config().Plank.ReportTemplate
		for report := range reportCh {
			if err := reportlib.Report(c.ghc, reportTemplate, report); err != nil {
				reportErrs = append(reportErrs, err)
				c.log.WithFields(pjutil.ProwJobFields(&report)).WithError(err).Warn("Failed to report ProwJob status")
			}
		}
	}

	if len(syncErrs) == 0 && len(reportErrs) == 0 {
		return nil
	}
	return fmt.Errorf("errors syncing: %v, errors reporting: %v", syncErrs, reportErrs)
}

// SyncMetrics records metrics for the cached prowjobs.
func (c *Controller) SyncMetrics() {
	c.pjLock.RLock()
	defer c.pjLock.RUnlock()
	kube.GatherProwJobMetrics(c.pjs)
}

// terminateDupes aborts presubmits that have a newer version. It modifies pjs
// in-place when it aborts.
// TODO: Dry this out - need to ensure we can abstract children cancellation first.
func (c *Controller) terminateDupes(pjs []kube.ProwJob, pm map[string]kube.Pod) error {
	// "job org/repo#number" -> newest job
	dupes := make(map[string]int)
	for i, pj := range pjs {
		if pj.Complete() || pj.Spec.Type != kube.PresubmitJob {
			continue
		}
		n := fmt.Sprintf("%s %s/%s#%d", pj.Spec.Job, pj.Spec.Refs.Org, pj.Spec.Refs.Repo, pj.Spec.Refs.Pulls[0].Number)
		prev, ok := dupes[n]
		if !ok {
			dupes[n] = i
			continue
		}
		cancelIndex := i
		if (&pjs[prev].Status.StartTime).Before(&pj.Status.StartTime) {
			cancelIndex = prev
			dupes[n] = i
		}
		toCancel := pjs[cancelIndex]
		// Allow aborting presubmit jobs for commits that have been superseded by
		// newer commits in Github pull requests.
		if c.ca.Config().Plank.AllowCancellations {
			if pod, exists := pm[toCancel.ObjectMeta.Name]; exists {
				if client, ok := c.pkcs[toCancel.ClusterAlias()]; !ok {
					c.log.WithFields(pjutil.ProwJobFields(&toCancel)).Errorf("Unknown cluster alias %q.", toCancel.ClusterAlias())
				} else if err := client.DeletePod(pod.ObjectMeta.Name); err != nil {
					c.log.WithError(err).WithFields(pjutil.ProwJobFields(&toCancel)).Warn("Cannot delete pod")
				}
			}
		}
		toCancel.SetComplete()
		prevState := toCancel.Status.State
		toCancel.Status.State = kube.AbortedState
		c.log.WithFields(pjutil.ProwJobFields(&toCancel)).
			WithField("from", prevState).
			WithField("to", toCancel.Status.State).Info("Transitioning states.")
		npj, err := c.kc.ReplaceProwJob(toCancel.ObjectMeta.Name, toCancel)
		if err != nil {
			return err
		}
		pjs[cancelIndex] = npj
	}
	return nil
}

// TODO: Dry this out
func syncProwJobs(
	l *logrus.Entry,
	syncFn syncFn,
	maxSyncRoutines int,
	jobs <-chan kube.ProwJob,
	reports chan<- kube.ProwJob,
	syncErrors chan<- error,
	pm map[string]kube.Pod,
) {
	goroutines := maxSyncRoutines
	if goroutines > len(jobs) {
		goroutines = len(jobs)
	}
	wg := &sync.WaitGroup{}
	wg.Add(goroutines)
	l.Debugf("Firing up %d goroutines", goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for pj := range jobs {
				if err := syncFn(pj, pm, reports); err != nil {
					syncErrors <- err
				}
			}
		}()
	}
	wg.Wait()
}

func (c *Controller) syncPendingJob(pj kube.ProwJob, pm map[string]kube.Pod, reports chan<- kube.ProwJob) error {
	// Record last known state so we can log state transitions.
	prevState := pj.Status.State

	pod, podExists := pm[pj.ObjectMeta.Name]
	if !podExists {
		c.incrementNumPendingJobs(pj.Spec.Job)
		// Pod is missing. This can happen in case the previous pod was deleted manually or by
		// a rescheduler. Start a new pod.
		id, pn, err := c.startPod(pj)
		if err != nil {
			_, isUnprocessable := err.(kube.UnprocessableEntityError)
			if !isUnprocessable {
				return fmt.Errorf("error starting pod: %v", err)
			}
			pj.Status.State = kube.ErrorState
			pj.SetComplete()
			pj.Status.Description = "Job cannot be processed."
			c.log.WithFields(pjutil.ProwJobFields(&pj)).WithError(err).Warning("Unprocessable pod.")
		} else {
			pj.Status.BuildID = id
			pj.Status.PodName = pn
			c.log.WithFields(pjutil.ProwJobFields(&pj)).Info("Pod is missing, starting a new pod")
		}
	} else {
		switch pod.Status.Phase {
		case kube.PodUnknown:
			c.incrementNumPendingJobs(pj.Spec.Job)
			// Pod is in Unknown state. This can happen if there is a problem with
			// the node. Delete the old pod, we'll start a new one next loop.
			c.log.WithFields(pjutil.ProwJobFields(&pj)).Info("Pod is in unknown state, deleting & restarting pod")
			client, ok := c.pkcs[pj.ClusterAlias()]
			if !ok {
				return fmt.Errorf("Unknown cluster alias %q.", pj.ClusterAlias())
			}
			return client.DeletePod(pj.ObjectMeta.Name)

		case kube.PodSucceeded:
			// Pod succeeded. Update ProwJob, talk to GitHub, and start next jobs.
			pj.SetComplete()
			pj.Status.State = kube.SuccessState
			pj.Status.Description = "Job succeeded."
			for _, nj := range pj.Spec.RunAfterSuccess {
				child := pjutil.NewProwJob(nj, pj.ObjectMeta.Labels)
				if c.ghc != nil && !c.RunAfterSuccessCanRun(&pj, &child, c.ca, c.ghc) {
					continue
				}
				if _, err := c.kc.CreateProwJob(pjutil.NewProwJob(nj, pj.ObjectMeta.Labels)); err != nil {
					return fmt.Errorf("error starting next prowjob: %v", err)
				}
			}

		case kube.PodFailed:
			if pod.Status.Reason == kube.Evicted {
				// Pod was evicted.
				if pj.Spec.ErrorOnEviction {
					// ErrorOnEviction is enabled, complete the PJ and mark it as errored.
					pj.SetComplete()
					pj.Status.State = kube.ErrorState
					pj.Status.Description = "Job pod was evicted by the cluster."
					break
				}
				// ErrorOnEviction is disabled. Delete the pod now and recreate it in
				// the next resync.
				c.incrementNumPendingJobs(pj.Spec.Job)
				client, ok := c.pkcs[pj.ClusterAlias()]
				if !ok {
					return fmt.Errorf("Unknown cluster alias %q.", pj.ClusterAlias())
				}
				return client.DeletePod(pj.ObjectMeta.Name)
			}
			// Pod failed. Update ProwJob, talk to GitHub.
			pj.SetComplete()
			pj.Status.State = kube.FailureState
			pj.Status.Description = "Job failed."

		case kube.PodPending:
			maxPodPending := c.ca.Config().Plank.PodPendingTimeout
			if pod.Status.StartTime.IsZero() || time.Since(pod.Status.StartTime.Time) < maxPodPending {
				// Pod is running. Do nothing.
				c.incrementNumPendingJobs(pj.Spec.Job)
				return nil
			}

			// Pod is stuck in pending state longer than maxPodPending
			// abort the job, and talk to Github
			pj.SetComplete()
			pj.Status.State = kube.AbortedState
			pj.Status.Description = "Job aborted."

		default:
			// Pod is running. Do nothing.
			c.incrementNumPendingJobs(pj.Spec.Job)
			return nil
		}
	}

	pj.Status.URL = jobURL(c.ca.Config().Plank, pj, c.log)

	reports <- pj

	if prevState != pj.Status.State {
		c.log.WithFields(pjutil.ProwJobFields(&pj)).
			WithField("from", prevState).
			WithField("to", pj.Status.State).Info("Transitioning states.")
	}
	_, err := c.kc.ReplaceProwJob(pj.ObjectMeta.Name, pj)
	return err
}

func (c *Controller) syncTriggeredJob(pj kube.ProwJob, pm map[string]kube.Pod, reports chan<- kube.ProwJob) error {
	// Record last known state so we can log state transitions.
	prevState := pj.Status.State

	var id, pn string
	pod, podExists := pm[pj.ObjectMeta.Name]
	// We may end up in a state where the pod exists but the prowjob is not
	// updated to pending if we successfully create a new pod in a previous
	// sync but the prowjob update fails. Simply ignore creating a new pod
	// and rerun the prowjob update.
	if !podExists {
		// Do not start more jobs than specified.
		if !c.canExecuteConcurrently(&pj) {
			return nil
		}
		// We haven't started the pod yet. Do so.
		var err error
		id, pn, err = c.startPod(pj)
		if err != nil {
			_, isUnprocessable := err.(kube.UnprocessableEntityError)
			if !isUnprocessable {
				return fmt.Errorf("error starting pod: %v", err)
			}
			pj.Status.State = kube.ErrorState
			pj.SetComplete()
			pj.Status.Description = "Job cannot be processed."
			logrus.WithField("job", pj.Spec.Job).WithError(err).Warning("Unprocessable pod.")
		}
	} else {
		id = getPodBuildID(&pod)
		pn = pod.ObjectMeta.Name
	}

	if pj.Status.State == kube.TriggeredState {
		// BuildID needs to be set before we execute the job url template.
		pj.Status.BuildID = id
		pj.Status.State = kube.PendingState
		pj.Status.PodName = pn
		pj.Status.Description = "Job triggered."
		pj.Status.URL = jobURL(c.ca.Config().Plank, pj, c.log)
	}
	reports <- pj
	if prevState != pj.Status.State {
		c.log.WithFields(pjutil.ProwJobFields(&pj)).
			WithField("from", prevState).
			WithField("to", pj.Status.State).Info("Transitioning states.")
	}
	_, err := c.kc.ReplaceProwJob(pj.ObjectMeta.Name, pj)
	return err
}

// TODO: No need to return the pod name since we already have the
// prowjob in the call site.
func (c *Controller) startPod(pj kube.ProwJob) (string, string, error) {
	buildID, err := c.getBuildID(pj.Spec.Job)
	if err != nil {
		return "", "", fmt.Errorf("error getting build ID: %v", err)
	}

	pod, err := decorate.ProwJobToPod(pj, buildID)
	if err != nil {
		return "", "", err
	}

	client, ok := c.pkcs[pj.ClusterAlias()]
	if !ok {
		return "", "", fmt.Errorf("Unknown cluster alias %q.", pj.ClusterAlias())
	}
	actual, err := client.CreatePod(*pod)
	if err != nil {
		return "", "", err
	}
	return buildID, actual.ObjectMeta.Name, nil
}

func (c *Controller) getBuildID(name string) (string, error) {
	return pjutil.GetBuildID(name, c.totURL)
}

func getPodBuildID(pod *kube.Pod) string {
	for _, env := range pod.Spec.Containers[0].Env {
		if env.Name == "BUILD_ID" {
			return env.Value
		}
	}
	logrus.Warningf("BUILD_ID was not found in pod %q: streaming logs from deck will not work", pod.ObjectMeta.Name)
	return ""
}

// RunAfterSuccessCanRun returns whether a child job (specified as run_after_success in the
// prow config) can run once its parent job succeeds. The only case we will not run a child job
// is when it is a presubmit job and has a run_if_changed regular expression specified which does
// not match the changed filenames in the pull request the job was meant to run for.
// TODO: Collapse with Jenkins, impossible to reuse as is due to the interfaces.
func (c *Controller) RunAfterSuccessCanRun(parent, child *kube.ProwJob, ca configAgent, ghc GitHubClient) bool {
	if parent.Spec.Type != kube.PresubmitJob {
		return true
	}

	// TODO: Make sure that parent and child have always the same org/repo.
	org := parent.Spec.Refs.Org
	repo := parent.Spec.Refs.Repo
	prNum := parent.Spec.Refs.Pulls[0].Number

	ps := ca.Config().GetPresubmit(org+"/"+repo, child.Spec.Job)
	if ps == nil {
		// The config has changed ever since we started the parent.
		// Not sure what is more correct here. Run the child for now.
		return true
	}
	if ps.RunIfChanged == "" {
		return true
	}
	changesFull, err := ghc.GetPullRequestChanges(org, repo, prNum)
	if err != nil {
		c.log.WithError(err).WithFields(pjutil.ProwJobFields(parent)).Warnf("Cannot get PR changes for #%d", prNum)
		return true
	}
	// We only care about the filenames here
	var changes []string
	for _, change := range changesFull {
		changes = append(changes, change.Filename)
	}
	return ps.RunsAgainstChanges(changes)
}

func jobURL(plank config.Plank, pj kube.ProwJob, log *logrus.Entry) string {
	if pj.Spec.DecorationConfig != nil && plank.JobURLPrefix != "" {
		spec := downwardapi.NewJobSpec(pj.Spec, pj.Status.BuildID, pj.Name)
		gcsConfig := pj.Spec.DecorationConfig.GCSConfiguration
		_, gcsPath, _ := gcsupload.PathsForJob(gcsConfig, &spec, "")

		prefix, _ := url.Parse(plank.JobURLPrefix)
		prefix.Path = path.Join(prefix.Path, gcsConfig.Bucket, gcsPath)
		return prefix.String()
	}
	var b bytes.Buffer
	if err := plank.JobURLTemplate.Execute(&b, &pj); err != nil {
		log.WithFields(pjutil.ProwJobFields(&pj)).Errorf("error executing URL template: %v", err)
	} else {
		return b.String()
	}
	return ""
}
