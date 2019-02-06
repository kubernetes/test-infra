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

	"github.com/bwmarrin/snowflake"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	reportlib "k8s.io/test-infra/prow/report"
)

const (
	testInfra = "https://github.com/kubernetes/test-infra/issues"
)

type kubeClient interface {
	CreateProwJob(kube.ProwJob) (kube.ProwJob, error)
	ListProwJobs(string) ([]kube.ProwJob, error)
	ReplaceProwJob(string, kube.ProwJob) (kube.ProwJob, error)
}

type jenkinsClient interface {
	Build(*kube.ProwJob, string) error
	ListBuilds(jobs []string) (map[string]Build, error)
	Abort(job string, build *Build) error
}

type githubClient interface {
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

type syncFn func(kube.ProwJob, chan<- kube.ProwJob, map[string]Build) error

// Controller manages ProwJobs.
type Controller struct {
	kc     kubeClient
	jc     jenkinsClient
	ghc    githubClient
	log    *logrus.Entry
	cfg    config.Getter
	node   *snowflake.Node
	totURL string
	// selector that will be applied on prowjobs.
	selector string

	lock sync.RWMutex
	// pendingJobs is a short-lived cache that helps in limiting
	// the maximum concurrency of jobs.
	pendingJobs map[string]int

	pjLock sync.RWMutex
	// shared across the controller and a goroutine that gathers metrics.
	pjs []kube.ProwJob
}

// NewController creates a new Controller from the provided clients.
func NewController(kc *kube.Client, jc *Client, ghc *github.Client, logger *logrus.Entry, cfg config.Getter, totURL, selector string) (*Controller, error) {
	n, err := snowflake.NewNode(1)
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}
	return &Controller{
		kc:          kc,
		jc:          jc,
		ghc:         ghc,
		log:         logger,
		cfg:         cfg,
		selector:    selector,
		node:        n,
		totURL:      totURL,
		pendingJobs: make(map[string]int),
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
func (c *Controller) canExecuteConcurrently(pj *kube.ProwJob) bool {
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
	pjs, err := c.kc.ListProwJobs(c.selector)
	if err != nil {
		return fmt.Errorf("error listing prow jobs: %v", err)
	}
	// Share what we have for gathering metrics.
	c.pjLock.Lock()
	c.pjs = pjs
	c.pjLock.Unlock()

	// TODO: Replace the following filtering with a field selector once CRDs support field selectors.
	// https://github.com/kubernetes/kubernetes/issues/53459
	var jenkinsJobs []kube.ProwJob
	for _, pj := range pjs {
		if pj.Spec.Agent == kube.JenkinsAgent {
			jenkinsJobs = append(jenkinsJobs, pj)
		}
	}
	pjs = jenkinsJobs
	jbs, err := c.jc.ListBuilds(getJenkinsJobs(pjs))
	if err != nil {
		return fmt.Errorf("error listing jenkins builds: %v", err)
	}

	var syncErrs []error
	if err := c.terminateDupes(pjs, jbs); err != nil {
		syncErrs = append(syncErrs, err)
	}

	pendingCh, triggeredCh := pjutil.PartitionActive(pjs)
	errCh := make(chan error, len(pjs))
	reportCh := make(chan kube.ProwJob, len(pjs))

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

	close(errCh)
	close(reportCh)

	for err := range errCh {
		syncErrs = append(syncErrs, err)
	}

	var reportErrs []error
	reportTemplate := c.config().ReportTemplate
	for report := range reportCh {
		if err := reportlib.Report(c.ghc, reportTemplate, report); err != nil {
			reportErrs = append(reportErrs, err)
			c.log.WithFields(pjutil.ProwJobFields(&report)).WithError(err).Warn("Failed to report ProwJob status")
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

// getJenkinsJobs returns all the Jenkins jobs for all active
// prowjobs from the provided list. It handles deduplication.
func getJenkinsJobs(pjs []kube.ProwJob) []string {
	jenkinsJobs := make(map[string]struct{})
	for _, pj := range pjs {
		if pj.Complete() {
			continue
		}
		jenkinsJobs[pj.Spec.Job] = struct{}{}
	}
	var jobs []string
	for job := range jenkinsJobs {
		jobs = append(jobs, job)
	}
	return jobs
}

// terminateDupes aborts presubmits that have a newer version. It modifies pjs
// in-place when it aborts.
func (c *Controller) terminateDupes(pjs []kube.ProwJob, jbs map[string]Build) error {
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
		if c.config().AllowCancellations {
			build, buildExists := jbs[toCancel.ObjectMeta.Name]
			// Avoid cancelling enqueued builds.
			if buildExists && build.IsEnqueued() {
				continue
			}
			// Otherwise, abort it.
			if buildExists {
				if err := c.jc.Abort(toCancel.Spec.Job, &build); err != nil {
					c.log.WithError(err).WithFields(pjutil.ProwJobFields(&toCancel)).Warn("Cannot cancel Jenkins build")
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

func syncProwJobs(
	l *logrus.Entry,
	syncFn syncFn,
	maxSyncRoutines int,
	jobs <-chan kube.ProwJob,
	reports chan<- kube.ProwJob,
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

func (c *Controller) syncPendingJob(pj kube.ProwJob, reports chan<- kube.ProwJob, jbs map[string]Build) error {
	// Record last known state so we can log state transitions.
	prevState := pj.Status.State

	jb, jbExists := jbs[pj.ObjectMeta.Name]
	if !jbExists {
		pj.SetComplete()
		pj.Status.State = kube.ErrorState
		pj.Status.URL = testInfra
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
			pj.Status.State = kube.SuccessState
			pj.Status.Description = "Jenkins job succeeded."
			for _, nj := range pj.Spec.RunAfterSuccess {
				child := pjutil.NewProwJob(nj, pj.ObjectMeta.Labels)
				if !c.RunAfterSuccessCanRun(&pj, &child) {
					continue
				}
				if _, err := c.kc.CreateProwJob(pjutil.NewProwJob(nj, pj.ObjectMeta.Labels)); err != nil {
					return fmt.Errorf("error starting next prowjob: %v", err)
				}
			}

		case jb.IsFailure():
			pj.SetComplete()
			pj.Status.State = kube.FailureState
			pj.Status.Description = "Jenkins job failed."

		case jb.IsAborted():
			pj.SetComplete()
			pj.Status.State = kube.AbortedState
			pj.Status.Description = "Jenkins job aborted."
		}
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
	// Report to Github.
	reports <- pj
	if prevState != pj.Status.State {
		c.log.WithFields(pjutil.ProwJobFields(&pj)).
			WithField("from", prevState).
			WithField("to", pj.Status.State).Info("Transitioning states.")
	}
	_, err := c.kc.ReplaceProwJob(pj.ObjectMeta.Name, pj)
	return err
}

func (c *Controller) syncTriggeredJob(pj kube.ProwJob, reports chan<- kube.ProwJob, jbs map[string]Build) error {
	// Record last known state so we can log state transitions.
	prevState := pj.Status.State

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
			pj.Status.State = kube.ErrorState
			pj.Status.URL = testInfra
			pj.Status.Description = "Error starting Jenkins job."
		} else {
			pj.Status.State = kube.PendingState
			pj.Status.Description = "Jenkins job enqueued."
		}
	} else {
		// If a Jenkins build already exists for this job, advance the ProwJob to Pending and
		// it should be handled by syncPendingJob in the next sync.
		pj.Status.State = kube.PendingState
		pj.Status.Description = "Jenkins job enqueued."
	}
	// Report to Github.
	reports <- pj

	if prevState != pj.Status.State {
		c.log.WithFields(pjutil.ProwJobFields(&pj)).
			WithField("from", prevState).
			WithField("to", pj.Status.State).Info("Transitioning states.")
	}
	_, err := c.kc.ReplaceProwJob(pj.ObjectMeta.Name, pj)
	return err
}

func (c *Controller) getBuildID(name string) (string, error) {
	if c.totURL == "" {
		return c.node.Generate().String(), nil
	}
	return pjutil.GetBuildID(name, c.totURL)
}

// RunAfterSuccessCanRun returns whether a child job (specified as run_after_success in the
// prow config) can run once its parent job succeeds. The only case we will not run a child job
// is when it is a presubmit job and has a run_if_changed regular expression specified which does
// not match the changed filenames in the pull request the job was meant to run for.
// TODO: Collapse with plank, impossible to reuse as is due to the interfaces.
func (c *Controller) RunAfterSuccessCanRun(parent, child *kube.ProwJob) bool {
	if parent.Spec.Type != kube.PresubmitJob {
		return true
	}

	// TODO: Make sure that parent and child have always the same org/repo.
	org := parent.Spec.Refs.Org
	repo := parent.Spec.Refs.Repo
	prNum := parent.Spec.Refs.Pulls[0].Number

	ps := c.cfg().GetPresubmit(org+"/"+repo, child.Spec.Job)
	if ps == nil {
		// The config has changed ever since we started the parent.
		// Not sure what is more correct here. Run the child for now.
		return true
	}
	if ps.RunIfChanged == "" {
		return true
	}
	changesFull, err := c.ghc.GetPullRequestChanges(org, repo, prNum)
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
