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
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/sirupsen/logrus"
	coreapi "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	ktypes "k8s.io/apimachinery/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowv1 "k8s.io/test-infra/prow/client/clientset/versioned/typed/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	reportlib "k8s.io/test-infra/prow/github/report"
	"k8s.io/test-infra/prow/github/reporter"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pod-utils/decorate"
)

// PodStatus constants
const (
	Evicted = "Evicted"
)

type prowJobClient interface {
	Create(*prowapi.ProwJob) (*prowapi.ProwJob, error)
	Update(*prowapi.ProwJob) (*prowapi.ProwJob, error)
	Get(name string, options metav1.GetOptions) (*prowapi.ProwJob, error)
	List(opts metav1.ListOptions) (*prowapi.ProwJobList, error)
	Patch(name string, pt ktypes.PatchType, data []byte, subresources ...string) (result *prowapi.ProwJob, err error)
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

// TODO: Dry this out
type syncFn func(pj prowapi.ProwJob, pm map[string]coreapi.Pod, reports chan<- prowapi.ProwJob) error

// Controller manages ProwJobs.
type Controller struct {
	prowJobClient prowJobClient
	buildClients  map[string]corev1.PodInterface
	ghc           GitHubClient
	log           *logrus.Entry
	config        config.Getter
	totURL        string
	// selector that will be applied on prowjobs and pods.
	selector string

	// If both this and pjLock are acquired, this must be acquired first
	lock sync.RWMutex

	// pendingJobs is a short-lived cache that helps in limiting
	// the maximum concurrency of jobs.
	pendingJobs map[string]int

	// If `lock` is acquired as well, `lock` must be acquired before locking
	// pjLock
	pjLock sync.RWMutex
	// shared across the controller and a goroutine that gathers metrics.
	pjs []prowapi.ProwJob

	// if skip report job results to github
	skipReport bool
}

// NewController creates a new Controller from the provided clients.
func NewController(prowJobClient prowv1.ProwJobInterface, buildClients map[string]corev1.PodInterface, ghc GitHubClient, logger *logrus.Entry, cfg config.Getter, totURL, selector string, skipReport bool) (*Controller, error) {
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}
	return &Controller{
		prowJobClient: prowJobClient,
		buildClients:  buildClients,
		ghc:           ghc,
		log:           logger,
		config:        cfg,
		pendingJobs:   make(map[string]int),
		totURL:        totURL,
		selector:      selector,
		skipReport:    skipReport,
	}, nil
}

// canExecuteConcurrently checks whether the provided ProwJob can
// be executed concurrently.
func (c *Controller) canExecuteConcurrently(pj *prowapi.ProwJob) bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	if max := c.config().Plank.MaxConcurrency; max > 0 {
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

	var olderMatchingPJs int
	c.pjLock.RLock()
	for _, foundPJ := range c.pjs {
		if foundPJ.Status.State != prowapi.TriggeredState {
			continue
		}
		if foundPJ.Spec.Job != pj.Spec.Job {
			continue
		}
		if foundPJ.CreationTimestamp.Before(&pj.CreationTimestamp) {
			olderMatchingPJs++
		}
	}
	c.pjLock.RUnlock()

	if numPending+olderMatchingPJs >= pj.Spec.MaxConcurrency {
		c.log.WithFields(pjutil.ProwJobFields(pj)).
			Debugf("Not starting another instance of %s, already %d running and %d older instances waiting, together they equal or exceed the total limit of %d",
				pj.Spec.Job, numPending, olderMatchingPJs, pj.Spec.MaxConcurrency)
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

// setPreviousReportState sets the github key for PrevReportStates
// to current state. This is a work-around for plank -> crier
// migration to become seamless.
func (c *Controller) setPreviousReportState(pj prowapi.ProwJob) error {
	// fetch latest before replace
	latestPJ, err := c.prowJobClient.Get(pj.ObjectMeta.Name, metav1.GetOptions{})
	c.log.WithFields(pjutil.ProwJobFields(latestPJ)).Debug("Get ProwJob.")
	if err != nil {
		return err
	}

	if latestPJ.Status.PrevReportStates == nil {
		latestPJ.Status.PrevReportStates = map[string]prowapi.ProwJobState{}
	}
	latestPJ.Status.PrevReportStates[reporter.GitHubReporterName] = latestPJ.Status.State
	_, err = c.prowJobClient.Update(latestPJ)
	c.log.WithFields(pjutil.ProwJobFields(latestPJ)).Debug("Update ProwJob.")
	return err
}

// Sync does one sync iteration.
func (c *Controller) Sync() error {
	pjs, err := c.prowJobClient.List(metav1.ListOptions{LabelSelector: c.selector})
	c.log.WithField("selector", c.selector).Debug("List ProwJobs.")
	if err != nil {
		return fmt.Errorf("error listing prow jobs: %v", err)
	}
	selector := fmt.Sprintf("%s=true", kube.CreatedByProw)
	if len(c.selector) > 0 {
		selector = strings.Join([]string{c.selector, selector}, ",")
	}

	pm := map[string]v1.Pod{}
	for alias, client := range c.buildClients {
		pods, err := client.List(metav1.ListOptions{LabelSelector: selector})
		c.log.WithField("selector", selector).Debug("List Pods.")
		if err != nil {
			return fmt.Errorf("error listing pods in cluster %q: %v", alias, err)
		}
		for _, pod := range pods.Items {
			pm[pod.ObjectMeta.Name] = pod
		}
	}
	// TODO: Replace the following filtering with a field selector once CRDs support field selectors.
	// https://github.com/kubernetes/kubernetes/issues/53459
	var k8sJobs []prowapi.ProwJob
	for _, pj := range pjs.Items {
		if pj.Spec.Agent == prowapi.KubernetesAgent {
			k8sJobs = append(k8sJobs, pj)
		}
	}
	// Sort jobs so jobs started earlier get better chance picked up earlier
	sort.Slice(k8sJobs, func(i, j int) bool {
		return k8sJobs[i].CreationTimestamp.Before(&k8sJobs[j].CreationTimestamp)
	})

	var syncErrs []error
	if err := c.terminateDupes(k8sJobs, pm); err != nil {
		syncErrs = append(syncErrs, err)
	}

	// Share what we have for gathering metrics.
	c.pjLock.Lock()
	c.pjs = k8sJobs
	c.pjLock.Unlock()

	pendingCh, triggeredCh := pjutil.PartitionActive(k8sJobs)
	errCh := make(chan error, len(k8sJobs))
	reportCh := make(chan prowapi.ProwJob, len(k8sJobs))

	// Reinstantiate on every resync of the controller instead of trying
	// to keep this in sync with the state of the world.
	c.pendingJobs = make(map[string]int)
	// Sync pending jobs first so we can determine what is the maximum
	// number of new jobs we can trigger when syncing the non-pendings.
	maxSyncRoutines := c.config().Plank.MaxGoroutines
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
		reportTemplate := c.config().Plank.ReportTemplate
		reportTypes := c.config().GitHubReporter.JobTypesToReport
		for report := range reportCh {
			if err := reportlib.Report(c.ghc, reportTemplate, report, reportTypes); err != nil {
				reportErrs = append(reportErrs, err)
				c.log.WithFields(pjutil.ProwJobFields(&report)).WithError(err).Warn("Failed to report ProwJob status")
			}

			// plank is not retrying on errors, so we just set the current state as reported
			if err := c.setPreviousReportState(report); err != nil {
				c.log.WithFields(pjutil.ProwJobFields(&report)).WithError(err).Error("Failed to patch PrevReportStates")
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
func (c *Controller) terminateDupes(pjs []prowapi.ProwJob, pm map[string]coreapi.Pod) error {
	log := c.log.WithField("aborter", "pod")
	return pjutil.TerminateOlderJobs(c.prowJobClient, log, pjs, func(toCancel prowapi.ProwJob) error {
		// Allow aborting presubmit jobs for commits that have been superseded by
		// newer commits in GitHub pull requests.
		if c.config().Plank.AllowCancellations {
			if pod, exists := pm[toCancel.ObjectMeta.Name]; exists {
				c.log.WithField("name", pod.ObjectMeta.Name).Debug("Delete Pod.")
				if client, ok := c.buildClients[toCancel.ClusterAlias()]; !ok {
					return fmt.Errorf("unknown cluster alias %q", toCancel.ClusterAlias())
				} else if err := client.Delete(pod.ObjectMeta.Name, &metav1.DeleteOptions{}); err != nil {
					return fmt.Errorf("deleting pod: %v", err)
				}
			}
		}
		return nil
	})
}

// TODO: Dry this out
func syncProwJobs(
	l *logrus.Entry,
	syncFn syncFn,
	maxSyncRoutines int,
	jobs <-chan prowapi.ProwJob,
	reports chan<- prowapi.ProwJob,
	syncErrors chan<- error,
	pm map[string]coreapi.Pod,
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

func (c *Controller) syncPendingJob(pj prowapi.ProwJob, pm map[string]coreapi.Pod, reports chan<- prowapi.ProwJob) error {
	// Record last known state so we can log state transitions.
	prevState := pj.Status.State
	prevPJ := pj

	pod, podExists := pm[pj.ObjectMeta.Name]
	if !podExists {
		c.incrementNumPendingJobs(pj.Spec.Job)
		// Pod is missing. This can happen in case the previous pod was deleted manually or by
		// a rescheduler. Start a new pod.
		id, pn, err := c.startPod(pj)
		if err != nil {
			if !kerrors.IsInvalid(err) {
				return fmt.Errorf("error starting pod %s: %v", pod.Name, err)
			}
			pj.Status.State = prowapi.ErrorState
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
		case coreapi.PodUnknown:
			c.incrementNumPendingJobs(pj.Spec.Job)
			// Pod is in Unknown state. This can happen if there is a problem with
			// the node. Delete the old pod, we'll start a new one next loop.
			c.log.WithFields(pjutil.ProwJobFields(&pj)).Info("Pod is in unknown state, deleting & restarting pod")
			client, ok := c.buildClients[pj.ClusterAlias()]
			if !ok {
				return fmt.Errorf("unknown pod %s: unknown cluster alias %q", pod.Name, pj.ClusterAlias())
			}

			c.log.WithField("name", pj.ObjectMeta.Name).Debug("Delete Pod.")
			return client.Delete(pj.ObjectMeta.Name, &metav1.DeleteOptions{})

		case coreapi.PodSucceeded:
			// Pod succeeded. Update ProwJob, talk to GitHub, and start next jobs.
			pj.SetComplete()
			pj.Status.State = prowapi.SuccessState
			pj.Status.Description = "Job succeeded."

		case coreapi.PodFailed:
			if pod.Status.Reason == Evicted {
				// Pod was evicted.
				if pj.Spec.ErrorOnEviction {
					// ErrorOnEviction is enabled, complete the PJ and mark it as errored.
					pj.SetComplete()
					pj.Status.State = prowapi.ErrorState
					pj.Status.Description = "Job pod was evicted by the cluster."
					break
				}
				// ErrorOnEviction is disabled. Delete the pod now and recreate it in
				// the next resync.
				c.incrementNumPendingJobs(pj.Spec.Job)
				client, ok := c.buildClients[pj.ClusterAlias()]
				if !ok {
					return fmt.Errorf("evicted pod %s: unknown cluster alias %q", pod.Name, pj.ClusterAlias())
				}
				c.log.WithField("name", pj.ObjectMeta.Name).Debug("Delete Pod.")
				return client.Delete(pj.ObjectMeta.Name, &metav1.DeleteOptions{})
			}
			// Pod failed. Update ProwJob, talk to GitHub.
			pj.SetComplete()
			pj.Status.State = prowapi.FailureState
			pj.Status.Description = "Job failed."

		case coreapi.PodPending:
			maxPodPending := c.config().Plank.PodPendingTimeout.Duration
			if pod.Status.StartTime.IsZero() || time.Since(pod.Status.StartTime.Time) < maxPodPending {
				// Pod is running. Do nothing.
				c.incrementNumPendingJobs(pj.Spec.Job)
				return nil
			}

			// Pod is stuck in pending state longer than maxPodPending
			// abort the job, and talk to GitHub
			pj.SetComplete()
			pj.Status.State = prowapi.ErrorState
			pj.Status.Description = "Pod pending timeout."
			client, ok := c.buildClients[pj.ClusterAlias()]
			if !ok {
				return fmt.Errorf("pending pod %s: unknown cluster alias %q", pod.Name, pj.ClusterAlias())
			}
			if err := client.Delete(pj.ObjectMeta.Name, &metav1.DeleteOptions{}); err != nil {
				return fmt.Errorf("failed to delete pod %s that was in pending timeout: %v", pod.Name, err)
			}
			c.log.WithFields(pjutil.ProwJobFields(&pj)).Info("Deleted stale pending pod.")

		case coreapi.PodRunning:
			maxPodRunning := c.config().Plank.PodRunningTimeout.Duration
			if pod.Status.StartTime.IsZero() || time.Since(pod.Status.StartTime.Time) < maxPodRunning {
				// Pod is still running. Do nothing.
				c.incrementNumPendingJobs(pj.Spec.Job)
				return nil
			}

			// Pod is stuck in running state longer than maxPodRunning
			// abort the job, and talk to GitHub
			pj.SetComplete()
			pj.Status.State = prowapi.AbortedState
			pj.Status.Description = "Pod running timeout."
			client, ok := c.buildClients[pj.ClusterAlias()]
			if !ok {
				return fmt.Errorf("running pod %s: unknown cluster alias %q", pod.Name, pj.ClusterAlias())
			}
			if err := client.Delete(pj.ObjectMeta.Name, &metav1.DeleteOptions{}); err != nil {
				return fmt.Errorf("failed to delete pod %s that was in running timeout: %v", pod.Name, err)
			}
			c.log.WithFields(pjutil.ProwJobFields(&pj)).Info("Deleted stale running pod.")
		default:
			// other states, ignore
			c.incrementNumPendingJobs(pj.Spec.Job)
			return nil
		}
	}

	pj.Status.URL = pjutil.JobURL(c.config().Plank, pj, c.log)

	reports <- pj

	if prevState != pj.Status.State {
		c.log.WithFields(pjutil.ProwJobFields(&pj)).
			WithField("from", prevState).
			WithField("to", pj.Status.State).Info("Transitioning states.")
	}

	return c.patchProwjob(prevPJ, pj)
}

func (c *Controller) syncTriggeredJob(pj prowapi.ProwJob, pm map[string]coreapi.Pod, reports chan<- prowapi.ProwJob) error {
	// Record last known state so we can log state transitions.
	prevState := pj.Status.State
	prevPJ := pj

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
			if !kerrors.IsInvalid(err) {
				return fmt.Errorf("error starting pod: %v", err)
			}
			pj.Status.State = prowapi.ErrorState
			pj.SetComplete()
			pj.Status.Description = "Job cannot be processed."
			logrus.WithField("job", pj.Spec.Job).WithError(err).Warning("Unprocessable pod.")
		}
	} else {
		id = getPodBuildID(&pod)
		pn = pod.ObjectMeta.Name
	}

	if pj.Status.State == prowapi.TriggeredState {
		// BuildID needs to be set before we execute the job url template.
		pj.Status.BuildID = id
		pj.Status.State = prowapi.PendingState
		pj.Status.PodName = pn
		pj.Status.Description = "Job triggered."
		pj.Status.URL = pjutil.JobURL(c.config().Plank, pj, c.log)
	}
	reports <- pj
	if prevState != pj.Status.State {
		c.log.WithFields(pjutil.ProwJobFields(&pj)).
			WithField("from", prevState).
			WithField("to", pj.Status.State).Info("Transitioning states.")
	}
	return c.patchProwjob(prevPJ, pj)
}

// TODO: No need to return the pod name since we already have the
// prowjob in the call site.
func (c *Controller) startPod(pj prowapi.ProwJob) (string, string, error) {
	buildID, err := c.getBuildID(pj.Spec.Job)
	if err != nil {
		return "", "", fmt.Errorf("error getting build ID: %v", err)
	}

	pod, err := decorate.ProwJobToPod(pj, buildID)
	if err != nil {
		return "", "", err
	}

	client, ok := c.buildClients[pj.ClusterAlias()]
	if !ok {
		return "", "", fmt.Errorf("unknown cluster alias %q", pj.ClusterAlias())
	}
	actual, err := client.Create(pod)
	c.log.WithFields(pjutil.ProwJobFields(&pj)).Debug("Create Pod.")
	if err != nil {
		return "", "", err
	}
	return buildID, actual.ObjectMeta.Name, nil
}

func (c *Controller) getBuildID(name string) (string, error) {
	return pjutil.GetBuildID(name, c.totURL)
}

func getPodBuildID(pod *coreapi.Pod) string {
	for _, env := range pod.Spec.Containers[0].Env {
		if env.Name == "BUILD_ID" {
			return env.Value
		}
	}
	logrus.Warningf("BUILD_ID was not found in pod %q: streaming logs from deck will not work", pod.ObjectMeta.Name)
	return ""
}

func (c *Controller) patchProwjob(srcPJ prowapi.ProwJob, destPJ prowapi.ProwJob) error {
	srcPJData, err := json.Marshal(srcPJ)
	if err != nil {
		return fmt.Errorf("marshal source prow job: %v", err)
	}

	destPJData, err := json.Marshal(destPJ)
	if err != nil {
		return fmt.Errorf("marshal dest prow job: %v", err)
	}

	patch, err := jsonpatch.CreateMergePatch(srcPJData, destPJData)
	if err != nil {
		return fmt.Errorf("cannot create JSON patch: %v", err)
	}

	_, err = c.prowJobClient.Patch(srcPJ.Name, ktypes.MergePatchType, patch)
	c.log.WithFields(pjutil.ProwJobFields(&destPJ)).Debug("Patched ProwJob.")
	return err
}
