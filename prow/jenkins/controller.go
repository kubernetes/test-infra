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
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/bwmarrin/snowflake"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/clock"
	prowv1 "k8s.io/test-infra/prow/client/clientset/versioned/typed/prowjobs/v1"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	reportlib "k8s.io/test-infra/prow/github/report"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
)

type prowJobClient interface {
	Create(context.Context, *prowapi.ProwJob, metav1.CreateOptions) (*prowapi.ProwJob, error)
	List(context.Context, metav1.ListOptions) (*prowapi.ProwJobList, error)
	Patch(ctx context.Context, name string, pt ktypes.PatchType, data []byte, o metav1.PatchOptions, subresources ...string) (result *prowapi.ProwJob, err error)
}

type jenkinsClient interface {
	Build(*prowapi.ProwJob, string) error
	ListBuilds(jobs []BuildQueryParams) (map[string]Build, error)
	Abort(job string, build *Build) error
}

type githubClient interface {
	BotUserChecker() (func(candidate string) bool, error)
	CreateStatus(org, repo, ref string, s github.Status) error
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
	CreateComment(org, repo string, number int, comment string) error
	DeleteComment(org, repo string, ID int) error
	EditComment(org, repo string, ID int, comment string) error
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
}

type syncFn func(prowapi.ProwJob, chan<- prowapi.ProwJob, map[string]Build) error

// Controller manages ProwJobs.
type Controller struct {
	prowJobClient prowJobClient
	jc            jenkinsClient
	ghc           githubClient
	log           *logrus.Entry
	cfg           config.Getter
	node          *snowflake.Node
	totURL        string
	// if skip report job results to github
	skipReport bool

	// retryAbortedJobs is used if the behavior of prow should be to retrigger aborted jenkins
	// jobs where the initiation of desired abortion was not caused by prow
	// This setting is specially useful for cases where you don't control when workers terminate,
	// such as when using spot instances.
	retryAbortedJobs bool

	// selector that will be applied on prowjobs.
	selector string

	lock sync.RWMutex
	// pendingJobs is a short-lived cache that helps in limiting
	// the maximum concurrency of jobs.
	pendingJobs map[string]int

	pjLock sync.RWMutex
	// shared across the controller and a goroutine that gathers metrics.
	pjs   []prowapi.ProwJob
	clock clock.Clock
}

// NewController creates a new Controller from the provided clients.
func NewController(prowJobClient prowv1.ProwJobInterface, jc *Client, ghc github.Client, logger *logrus.Entry, cfg config.Getter, totURL, selector string, skipReport bool, retryAbortedJobs bool) (*Controller, error) {
	n, err := snowflake.NewNode(1)
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}
	return &Controller{
		prowJobClient:    prowJobClient,
		jc:               jc,
		ghc:              ghc,
		log:              logger,
		cfg:              cfg,
		selector:         selector,
		node:             n,
		totURL:           totURL,
		skipReport:       skipReport,
		retryAbortedJobs: retryAbortedJobs,
		pendingJobs:      make(map[string]int),
		clock:            clock.RealClock{},
	}, nil
}

func (c *Controller) config() config.Controller {
	operators := c.cfg().JenkinsOperators
	if len(operators) == 1 {
		return operators[0].Controller
	}
	configured := make([]string, 0, len(operators))
	for _, cfg := range operators {
		if cfg.LabelSelectorString == c.selector {
			return cfg.Controller
		}
		configured = append(configured, cfg.LabelSelectorString)
	}
	if len(c.selector) == 0 {
		c.log.Panicf("You need to specify a non-empty --label-selector (existing selectors: %v).", configured)
	} else {
		c.log.Panicf("No config exists for --label-selector=%s.", c.selector)
	}
	return config.Controller{}
}

// canExecuteConcurrently checks whether the provided ProwJob can
// be executed concurrently.
func (c *Controller) canExecuteConcurrently(pj *prowapi.ProwJob) bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	if max := c.config().MaxConcurrency; max > 0 {
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
	pjs, err := c.prowJobClient.List(context.TODO(), metav1.ListOptions{LabelSelector: c.selector})
	if err != nil {
		return fmt.Errorf("error listing prow jobs: %v", err)
	}
	// Share what we have for gathering metrics.
	c.pjLock.Lock()
	c.pjs = pjs.Items
	c.pjLock.Unlock()

	// TODO: Replace the following filtering with a field selector once CRDs support field selectors.
	// https://github.com/kubernetes/kubernetes/issues/53459
	var jenkinsJobs []prowapi.ProwJob
	for _, pj := range pjs.Items {
		if pj.Spec.Agent == prowapi.JenkinsAgent {
			jenkinsJobs = append(jenkinsJobs, pj)
		}
	}
	jbs, err := c.jc.ListBuilds(getJenkinsJobs(jenkinsJobs))
	if err != nil {
		return fmt.Errorf("error listing jenkins builds: %v", err)
	}

	var syncErrs []error
	if err := c.terminateDupes(jenkinsJobs, jbs); err != nil {
		syncErrs = append(syncErrs, err)
	}

	pendingCh, triggeredCh, abortedCh := pjutil.PartitionActive(jenkinsJobs)
	errCh := make(chan error, len(jenkinsJobs))
	reportCh := make(chan prowapi.ProwJob, len(jenkinsJobs))

	// Reinstantiate on every resync of the controller instead of trying
	// to keep this in sync with the state of the world.
	c.pendingJobs = make(map[string]int)
	// Sync pending jobs first so we can determine what is the maximum
	// number of new jobs we can trigger when syncing the non-pendings.
	maxSyncRoutines := c.config().MaxGoroutines
	c.log.Debugf("Handling %d pending prowjobs", len(pendingCh))
	syncProwJobs(c.log, c.syncPendingJob, maxSyncRoutines, pendingCh, reportCh, errCh, jbs)
	c.log.Debugf("Handling %d triggered prowjobs", len(triggeredCh))
	syncProwJobs(c.log, c.syncTriggeredJob, maxSyncRoutines, triggeredCh, reportCh, errCh, jbs)
	c.log.Debugf("Handling %d aborted prowjobs", len(abortedCh))
	syncProwJobs(c.log, c.syncAbortedJob, maxSyncRoutines, abortedCh, reportCh, errCh, jbs)

	close(errCh)
	close(reportCh)

	for err := range errCh {
		syncErrs = append(syncErrs, err)
	}

	var reportErrs []error
	reportTypes := c.cfg().GitHubReporter.JobTypesToReport
	jConfig := c.config()
	for report := range reportCh {
		// Report status if pending, otherwise crier doesn't know when link is available
		// We always want to report the status URL, when state changes from enqueued to running
		if !c.skipReport || report.Status.Description == "Jenkins job running." {
			reportTemplate := jConfig.ReportTemplateForRepo(report.Spec.Refs)
			if err := reportlib.Report(c.ghc, reportTemplate, report, reportTypes); err != nil {
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
	kube.GatherProwJobMetrics(c.log, c.pjs)
}

// getJenkinsJobs returns all the Jenkins jobs for all active
// prowjobs from the provided list. It handles deduplication.
func getJenkinsJobs(pjs []prowapi.ProwJob) []BuildQueryParams {
	jenkinsJobs := []BuildQueryParams{}

	for _, pj := range pjs {
		if pj.Complete() {
			continue
		}

		jenkinsJobs = append(jenkinsJobs, BuildQueryParams{
			JobName:   getJobName(&pj.Spec),
			ProwJobID: pj.Name,
		})
	}

	return jenkinsJobs
}

// terminateDupes aborts presubmits that have a newer version. It modifies pjs
// in-place when it aborts.
func (c *Controller) terminateDupes(pjs []prowapi.ProwJob, jbs map[string]Build) error {
	// "job org/repo#number" -> newest job
	dupes := make(map[string]int)
	for i, pj := range pjs {
		if pj.Complete() || pj.Spec.Type != prowapi.PresubmitJob {
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

		// Abort presubmit jobs for commits that have been superseded by
		// newer commits in GitHub pull requests.
		build, buildExists := jbs[toCancel.ObjectMeta.Name]
		// Avoid cancelling enqueued builds.
		if buildExists && build.IsEnqueued() {
			continue
		}
		// Otherwise, abort it.
		if buildExists {
			if err := c.jc.Abort(getJobName(&toCancel.Spec), &build); err != nil {
				c.log.WithError(err).WithFields(pjutil.ProwJobFields(&toCancel)).Warn("Cannot cancel Jenkins build")
			}
		}

		srcPJ := toCancel.DeepCopy()
		toCancel.SetComplete()
		prevState := toCancel.Status.State
		toCancel.Status.State = prowapi.AbortedState
		c.log.WithFields(pjutil.ProwJobFields(&toCancel)).
			WithField("from", prevState).
			WithField("to", toCancel.Status.State).Info("Transitioning states.")
		npj, err := pjutil.PatchProwjob(context.TODO(), c.prowJobClient, c.log, *srcPJ, toCancel)
		if err != nil {
			return err
		}
		pjs[cancelIndex] = *npj
	}
	return nil
}

func syncProwJobs(
	l *logrus.Entry,
	syncFn syncFn,
	maxSyncRoutines int,
	jobs <-chan prowapi.ProwJob,
	reports chan<- prowapi.ProwJob,
	syncErrors chan<- error,
	jbs map[string]Build,
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
				if err := syncFn(pj, reports, jbs); err != nil {
					syncErrors <- err
				}
			}
		}()
	}
	wg.Wait()
}

func (c *Controller) syncPendingJob(pj prowapi.ProwJob, reports chan<- prowapi.ProwJob, jbs map[string]Build) error {
	// Record last known state so we can patch
	prevPJ := pj.DeepCopy()

	jb, jbExists := jbs[pj.ObjectMeta.Name]
	reTriggered := false
	if !jbExists {
		pj.SetComplete()
		pj.Status.State = prowapi.ErrorState
		pj.Status.URL = c.cfg().StatusErrorLink
		pj.Status.Description = "Error finding Jenkins job."
	} else {
		switch {
		case jb.IsEnqueued():
			// Still in queue.
			c.incrementNumPendingJobs(pj.Spec.Job)
			return nil

		case jb.IsRunning():
			// Build still going.
			c.incrementNumPendingJobs(pj.Spec.Job)
			if pj.Status.Description == "Jenkins job running." {
				return nil
			}
			pj.Status.Description = "Jenkins job running."

		case jb.IsSuccess():
			// Build is complete.
			pj.SetComplete()
			pj.Status.State = prowapi.SuccessState
			pj.Status.Description = "Jenkins job succeeded."

		case jb.IsFailure():
			pj.SetComplete()
			pj.Status.State = prowapi.FailureState
			pj.Status.Description = "Jenkins job failed."

		case jb.IsAborted():
			// If config is set to retry aborted jobs and if aborted state is not coming from prow
			if c.retryAbortedJobs && pj.Status.State != prowapi.AbortedState {
				buildID, err := c.getBuildID(pj.Spec.Job)
				if err != nil {
					return fmt.Errorf("error getting build ID: %v", err)
				}
				if err := c.jc.Build(&pj, buildID); err != nil {
					c.log.WithError(err).WithFields(pjutil.ProwJobFields(&pj)).Warn("Cannot start Jenkins build")
					pj.SetComplete()
					pj.Status.State = prowapi.ErrorState
					pj.Status.URL = c.cfg().StatusErrorLink
					pj.Status.Description = "Error starting Jenkins job."
				} else {
					reTriggered = true
					now := metav1.NewTime(c.clock.Now())
					pj.Status.PendingTime = &now
					pj.Status.State = prowapi.PendingState
					pj.Status.Description = "Jenkins job enqueued."
					pj.Status.URL = ""
					pj.Status.BuildID = buildID
					pj.Status.JenkinsBuildID = ""
				}
			} else {
				pj.SetComplete()
				pj.Status.State = prowapi.AbortedState
				pj.Status.Description = "Jenkins job aborted."
			}
		}

		if !reTriggered {
			// Construct the status URL that will be used in reports.
			pj.Status.PodName = pj.ObjectMeta.Name
			pj.Status.BuildID = jb.BuildID()
			pj.Status.JenkinsBuildID = strconv.Itoa(jb.Number)
			var b bytes.Buffer
			if err := c.config().JobURLTemplate.Execute(&b, &pj); err != nil {
				c.log.WithFields(pjutil.ProwJobFields(&pj)).Errorf("error executing URL template: %v", err)
			} else {
				pj.Status.URL = b.String()
			}
		}
	}
	// Report to GitHub.
	reports <- pj
	if prevPJ.Status.State != pj.Status.State {
		c.log.WithFields(pjutil.ProwJobFields(&pj)).
			WithField("from", prevPJ.Status.State).
			WithField("to", pj.Status.State).Info("Transitioning states.")
	}
	_, err := pjutil.PatchProwjob(context.TODO(), c.prowJobClient, c.log, *prevPJ, pj)
	return err
}

func (c *Controller) syncAbortedJob(pj prowapi.ProwJob, _ chan<- prowapi.ProwJob, jbs map[string]Build) error {
	if pj.Status.State != prowapi.AbortedState || pj.Complete() {
		return nil
	}

	if build, exists := jbs[pj.Name]; exists {
		if err := c.jc.Abort(getJobName(&pj.Spec), &build); err != nil {
			return fmt.Errorf("failed to abort Jenkins build: %v", err)
		}
	}

	originalPJ := pj.DeepCopy()
	pj.SetComplete()
	_, err := pjutil.PatchProwjob(context.TODO(), c.prowJobClient, c.log, *originalPJ, pj)
	return err
}

func (c *Controller) syncTriggeredJob(pj prowapi.ProwJob, reports chan<- prowapi.ProwJob, jbs map[string]Build) error {
	// Record last known state so we can patch
	prevPJ := pj.DeepCopy()

	if _, jbExists := jbs[pj.ObjectMeta.Name]; !jbExists {
		// Do not start more jobs than specified.
		if !c.canExecuteConcurrently(&pj) {
			return nil
		}
		buildID, err := c.getBuildID(pj.Spec.Job)
		if err != nil {
			return fmt.Errorf("error getting build ID: %v", err)
		}
		// Start the Jenkins job.
		if err := c.jc.Build(&pj, buildID); err != nil {
			c.log.WithError(err).WithFields(pjutil.ProwJobFields(&pj)).Warn("Cannot start Jenkins build")
			pj.SetComplete()
			pj.Status.State = prowapi.ErrorState
			pj.Status.URL = c.cfg().StatusErrorLink
			pj.Status.Description = "Error starting Jenkins job."
		} else {
			now := metav1.NewTime(c.clock.Now())
			pj.Status.PendingTime = &now
			pj.Status.State = prowapi.PendingState
			pj.Status.Description = "Jenkins job enqueued."
		}
	} else {
		// If a Jenkins build already exists for this job, advance the ProwJob to Pending and
		// it should be handled by syncPendingJob in the next sync.
		if pj.Status.PendingTime == nil {
			now := metav1.NewTime(c.clock.Now())
			pj.Status.PendingTime = &now
		}
		pj.Status.State = prowapi.PendingState
		pj.Status.Description = "Jenkins job enqueued."
	}
	// Report to GitHub.
	reports <- pj

	if prevPJ.Status.State != pj.Status.State {
		c.log.WithFields(pjutil.ProwJobFields(&pj)).
			WithField("from", prevPJ.Status.State).
			WithField("to", pj.Status.State).Info("Transitioning states.")
	}
	_, err := pjutil.PatchProwjob(context.TODO(), c.prowJobClient, c.log, *prevPJ, pj)
	return err
}

func (c *Controller) getBuildID(name string) (string, error) {
	if c.totURL == "" {
		return c.node.Generate().String(), nil
	}
	return pjutil.GetBuildID(name, c.totURL)
}
