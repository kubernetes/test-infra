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
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andygrunwald/go-gerrit"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowv1 "k8s.io/test-infra/prow/client/clientset/versioned/typed/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	reporter "k8s.io/test-infra/prow/crier/reporters/gerrit"
	"k8s.io/test-infra/prow/gerrit/client"
	"k8s.io/test-infra/prow/gerrit/source"
	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
)

const (
	inRepoConfigRetries = 2
	inRepoConfigFailed  = "Unable to get inRepoConfig. This could be due to a merge conflict or a flake. If a merge conflict, please rebase and fix conflicts. Otherwise try again with /test all"
)

var gerritMetrics = struct {
	processingResults     *prometheus.CounterVec
	triggerLatency        *prometheus.HistogramVec
	changeProcessDuration *prometheus.HistogramVec
}{
	processingResults: prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gerrit_processing_results",
		Help: "Count of change processing by instance, repo, and result.",
	}, []string{
		"org",
		"repo",
		"result",
	}),
	triggerLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gerrit_trigger_latency",
		Help:    "Histogram of seconds between triggering event and ProwJob creation time.",
		Buckets: []float64{5, 10, 20, 30, 60, 120, 180, 300, 600, 1200, 3600},
	}, []string{
		"org",
		// Omit repo to avoid excessive cardinality due to the number of buckets.
	}),
	changeProcessDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gerrit_instance_process_duration",
		Help:    "Histogram of seconds spent processing a single gerrit instance.",
		Buckets: []float64{5, 10, 20, 30, 60, 120, 180, 300, 600, 1200, 3600},
	}, []string{
		"org",
	}),
}

func init() {
	prometheus.MustRegister(gerritMetrics.processingResults)
	prometheus.MustRegister(gerritMetrics.triggerLatency)
	prometheus.MustRegister(gerritMetrics.changeProcessDuration)
}

type prowJobClient interface {
	Create(context.Context, *prowapi.ProwJob, metav1.CreateOptions) (*prowapi.ProwJob, error)
}

type gerritClient interface {
	ApplyGlobalConfig(orgRepoConfigGetter func() *config.GerritOrgRepoConfigs, lastSyncTracker *client.SyncTime, cookiefilePath, tokenPathOverride string, additionalFunc func())
	Authenticate(cookiefilePath, tokenPath string)
	QueryChanges(lastState client.LastSyncState, rateLimit int) map[string][]client.ChangeInfo
	QueryChangesForInstance(instance string, lastState client.LastSyncState, rateLimit int) []client.ChangeInfo
	GetBranchRevision(instance, project, branch string) (string, error)
	SetReview(instance, id, revision, message string, labels map[string]string) error
	Account(instance string) (*gerrit.AccountInfo, error)
}

// Controller manages gerrit changes.
type Controller struct {
	config                   config.Getter
	prowJobClient            prowJobClient
	gc                       gerritClient
	tracker                  LastSyncTracker
	projectsOptOutHelp       map[string]sets.String
	lock                     sync.RWMutex
	cookieFilePath           string
	configAgent              *config.Agent
	inRepoConfigCacheHandler *config.InRepoConfigCacheHandler
	inRepoConfigFailures     map[string]bool
	instancesWithWorker      map[string]bool
	repoCacheMapMux          sync.Mutex
	latestMux                sync.Mutex
	workerPoolSize           int
}

type LastSyncTracker interface {
	Current() client.LastSyncState
	Update(client.LastSyncState) error
}

// NewController returns a new gerrit controller client
func NewController(ctx context.Context, prowJobClient prowv1.ProwJobInterface, op io.Opener,
	ca *config.Agent, cookiefilePath, tokenPathOverride, lastSyncFallback string, workerPoolSize int, inRepoConfigCacheHandler *config.InRepoConfigCacheHandler) *Controller {

	cfg := ca.Config
	projectsOptOutHelpMap := map[string]sets.String{}
	if cfg().Gerrit.OrgReposConfig != nil {
		projectsOptOutHelpMap = cfg().Gerrit.OrgReposConfig.OptOutHelpRepos()
	}
	lastSyncTracker := client.NewSyncTime(lastSyncFallback, op, ctx)

	if err := lastSyncTracker.Init(cfg().Gerrit.OrgReposConfig.AllRepos()); err != nil {
		logrus.WithError(err).Fatal("Error initializing lastSyncFallback.")
	}
	gerritClient, err := client.NewClient(nil)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating gerrit client.")
	}
	c := &Controller{
		prowJobClient:            prowJobClient,
		config:                   cfg,
		gc:                       gerritClient,
		tracker:                  lastSyncTracker,
		projectsOptOutHelp:       projectsOptOutHelpMap,
		cookieFilePath:           cookiefilePath,
		configAgent:              ca,
		inRepoConfigCacheHandler: inRepoConfigCacheHandler,
		inRepoConfigFailures:     map[string]bool{},
		instancesWithWorker:      make(map[string]bool),
		workerPoolSize:           workerPoolSize,
	}

	// applyGlobalConfig reads gerrit configurations from global gerrit config,
	// it will completely override previously configured gerrit hosts and projects.
	// it will also by the way authenticate gerrit
	orgRepoConfigGetter := func() *config.GerritOrgRepoConfigs {
		return cfg().Gerrit.OrgReposConfig
	}
	c.gc.ApplyGlobalConfig(orgRepoConfigGetter, lastSyncTracker, cookiefilePath, tokenPathOverride, func() {
		orgReposConfig := orgRepoConfigGetter()
		if orgReposConfig == nil {
			return
		}
		c.lock.Lock()
		// Updates a map, lock to make sure it's thread safe.
		c.projectsOptOutHelp = orgReposConfig.OptOutHelpRepos()
		c.lock.Unlock()
	})

	// Authenticate creates a goroutine for rotating token secrets when called the first
	// time, afterwards it only authenticate once.
	// applyGlobalConfig calls authenticate only when global gerrit config presents,
	// call it here is required for cases where gerrit repos are defined as command
	// line arg(which is going to be deprecated).
	c.gc.Authenticate(cookiefilePath, tokenPathOverride)

	return c
}

type Change struct {
	changeInfo gerrit.ChangeInfo
	instance   string
}

func (c *Controller) syncChange(latest client.LastSyncState, changeChan <-chan Change, log *logrus.Entry, wg *sync.WaitGroup) {
	for changeStruct := range changeChan {
		change := changeStruct.changeInfo
		instance := changeStruct.instance

		log := log.WithFields(logrus.Fields{
			"branch":   change.Branch,
			"change":   change.Number,
			"repo":     change.Project,
			"revision": change.CurrentRevision,
		})

		result := client.ResultSuccess
		if err := c.processChange(log, instance, change); err != nil {
			result = client.ResultError
			log.WithError(err).Info("Failed to process change")
		}
		gerritMetrics.processingResults.WithLabelValues(instance, change.Project, result).Inc()

		c.latestMux.Lock()
		lastTime, ok := latest[instance][change.Project]
		if !ok || lastTime.Before(change.Updated.Time) {
			lastTime = change.Updated.Time
			latest[instance][change.Project] = lastTime
		}
		c.latestMux.Unlock()
		wg.Done()
	}
}

// Sync looks for newly made gerrit changes
// and creates prowjobs according to specs
func (c *Controller) Sync() {
	processSingleInstance := func(instance string) {
		// Assumes the passed in instance was already normalized with https:// prefix.
		log := logrus.WithField("host", instance)
		syncTime := c.tracker.Current()
		latest := syncTime.DeepCopy()

		now := time.Now()
		defer func() {
			gerritMetrics.changeProcessDuration.WithLabelValues(instance).Observe(float64(time.Since(now).Seconds()))
		}()

		changes := c.gc.QueryChangesForInstance(instance, syncTime, c.config().Gerrit.RateLimit)
		if len(changes) == 0 {
			return
		}
		log.WithFields(logrus.Fields{"instance": instance, "changes": len(changes)}).Info("Finished querying instance for changes")

		var wg sync.WaitGroup
		wg.Add(len(changes))

		changeChan := make(chan Change)
		for i := 0; i < c.workerPoolSize; i++ {
			go c.syncChange(latest, changeChan, log, &wg)
		}
		for _, change := range changes {
			changeChan <- Change{changeInfo: change, instance: instance}
		}
		wg.Wait()
		close(changeChan)
		c.tracker.Update(latest)
	}

	for instance := range c.config().Gerrit.OrgReposConfig.AllRepos() {
		if _, ok := c.instancesWithWorker[instance]; ok {
			// The work thread of already up for this instance, nothing needs
			// to be done.
			continue
		}
		c.instancesWithWorker[instance] = true

		// First time see this instance, spin up a worker thread for it
		logrus.WithField("instance", instance).Info("Start worker for instance.")
		go func(instance string) {
			previousRun := time.Now()
			for {
				timeDiff := time.Until(previousRun.Add(c.config().Gerrit.TickInterval.Duration))
				if timeDiff > 0 {
					time.Sleep(timeDiff)
				}
				previousRun = time.Now()
				processSingleInstance(instance)
			}
		}(instance)
	}
}

// CreateRefs creates refs for a presubmit job from given changes.
//
// Passed in instance must contain https:// prefix.
func CreateRefs(instance, project, branch, baseSHA string, changes ...client.ChangeInfo) (prowapi.Refs, error) {
	var refs prowapi.Refs
	cloneURI := source.CloneURIFromOrgRepo(instance, project)

	var codeHost string // Something like https://android.googlesource.com
	parts := strings.SplitN(instance, ".", 2)
	codeHost = strings.TrimSuffix(parts[0], "-review")
	if len(parts) > 1 {
		codeHost += "." + parts[1]
	}
	refs = prowapi.Refs{
		Org:      instance, // Something like android-review.googlesource.com
		Repo:     project,  // Something like platform/build
		BaseRef:  branch,
		BaseSHA:  baseSHA,
		CloneURI: cloneURI, // Something like https://android-review.googlesource.com/platform/build
		RepoLink: fmt.Sprintf("%s/%s", codeHost, project),
		BaseLink: fmt.Sprintf("%s/%s/+/%s", codeHost, project, baseSHA),
	}
	for _, change := range changes {
		rev, ok := change.Revisions[change.CurrentRevision]
		if !ok {
			return prowapi.Refs{}, fmt.Errorf("cannot find current revision for change %v", change.ID)
		}
		refs.Pulls = append(refs.Pulls, prowapi.Pull{
			Number:     change.Number,
			Author:     rev.Commit.Author.Name,
			SHA:        change.CurrentRevision,
			Ref:        rev.Ref,
			Link:       fmt.Sprintf("%s/c/%s/+/%d", instance, change.Project, change.Number),
			CommitLink: fmt.Sprintf("%s/%s/+/%s", codeHost, change.Project, change.CurrentRevision),
			AuthorLink: fmt.Sprintf("%s/q/%s", instance, rev.Commit.Author.Email),
		})
	}
	return refs, nil
}

func LabelsAndAnnotations(instance string, jobLabels, jobAnnotations map[string]string, changes ...client.ChangeInfo) (labels, annotations map[string]string) {
	labels, annotations = make(map[string]string), make(map[string]string)
	for k, v := range jobLabels {
		labels[k] = v
	}
	for k, v := range jobAnnotations {
		annotations[k] = v
	}
	annotations[kube.GerritInstance] = instance

	// Labels required for Crier reporting back to Gerrit, batch jobs are not
	// expected to report so only add when there is a single change.
	if len(changes) == 1 {
		change := changes[0]
		labels[kube.GerritRevision] = change.CurrentRevision
		labels[kube.GerritPatchset] = strconv.Itoa(change.Revisions[change.CurrentRevision].Number)
		if _, ok := labels[kube.GerritReportLabel]; !ok {
			logrus.Debug("Job uses default value of 'Code-Review' for 'prow.k8s.io/gerrit-report-label' label. This default will removed in March 2022.")
			labels[kube.GerritReportLabel] = client.CodeReview
		}

		annotations[kube.GerritID] = change.ID
	}

	return
}

// failedJobs find jobs currently reported as failing (used for retesting).
//
// Failing means the job is complete and not passing.
// Scans messages for prow reports, which lists jobs and whether they passed.
// Job is included in the set if the latest report has it failing.
func failedJobs(account int, revision int, messages ...gerrit.ChangeMessageInfo) sets.String {
	failures := sets.String{}
	times := map[string]time.Time{}
	for _, message := range messages {
		if message.Author.AccountID != account { // Ignore reports from other accounts
			continue
		}
		if message.RevisionNumber != revision { // Ignore reports for old commits
			continue
		}
		// TODO(fejta): parse triggered job reports and remove from failure set.
		// (alternatively refactor this whole process rely less on fragile string parsing)
		report := reporter.ParseReport(message.Message)
		if report == nil {
			continue
		}
		for _, job := range report.Jobs {
			name := job.Name
			if latest, present := times[name]; present && message.Date.Before(latest) {
				continue
			}
			times[name] = message.Date.Time
			if job.State == prowapi.FailureState || job.State == prowapi.ErrorState || job.State == prowapi.AbortedState {
				failures.Insert(name)
			} else {
				failures.Delete(name)
			}
		}
	}
	return failures
}

func (c *Controller) handleInRepoConfigError(err error, instance string, change gerrit.ChangeInfo) error {
	key := fmt.Sprintf("%s%s%s", instance, change.ID, change.CurrentRevision)
	if err != nil {
		// If we have not already recorded this failure send an error essage
		if failed, ok := c.inRepoConfigFailures[key]; !ok || !failed {
			if setReviewWerr := c.gc.SetReview(instance, change.ID, change.CurrentRevision, inRepoConfigFailed, nil); setReviewWerr != nil {
				return fmt.Errorf("failed to get inRepoConfig and failed to set Review to notify user: %v and %v", err, setReviewWerr)
			}
			c.inRepoConfigFailures[key] = true
		}

		// We do not want to return that there was an error processing change. If we are unable to get inRepoConfig we do not process. This is expected behavior.
		return nil
	}

	// If failed in the past but passes now, allow future failures to send message
	if _, ok := c.inRepoConfigFailures[key]; ok {
		c.inRepoConfigFailures[key] = false
	}
	return nil
}

// processChange creates new presubmit/postsubmit prowjobs base off the gerrit changes
func (c *Controller) processChange(logger logrus.FieldLogger, instance string, change client.ChangeInfo) error {
	cloneURI := source.CloneURIFromOrgRepo(instance, change.Project)
	baseSHA, err := c.gc.GetBranchRevision(instance, change.Project, change.Branch)
	if err != nil {
		return fmt.Errorf("GetBranchRevision: %w", err)
	}

	type triggeredJob struct {
		name   string
		report bool
	}
	var triggeredJobs []triggeredJob
	triggerTimes := map[string]time.Time{}

	refs, err := CreateRefs(instance, change.Project, change.Branch, baseSHA, change)
	if err != nil {
		return fmt.Errorf("createRefs from %s at %s: %w", cloneURI, baseSHA, err)
	}

	type jobSpec struct {
		spec        prowapi.ProwJobSpec
		labels      map[string]string
		annotations map[string]string
	}

	var jobSpecs []jobSpec

	changedFiles := client.ChangedFilesProvider(&change)

	switch change.Status {
	case client.Merged:
		var postsubmits []config.Postsubmit
		// Gerrit server might be unavailable intermittently, retry inrepoconfig
		// processing for increased reliability.
		for attempt := 0; attempt < inRepoConfigRetries; attempt++ {
			postsubmits, err = c.inRepoConfigCacheHandler.GetPostsubmits(cloneURI, func() (string, error) { return baseSHA, nil }, func() (string, error) { return change.CurrentRevision, nil })
			// Break if there was no error, or if there was a merge conflict
			if err == nil || strings.Contains(err.Error(), "Merge conflict in") {
				break
			}
		}
		// Postsubmit jobs are triggered only once. Still try to fall back on
		// static jobs if failed to retrieve inrepoconfig jobs.
		if err != nil {
			// Reports error back to Gerrit. handleInRepoConfigError is
			// responsible for not sending the same message again and again on
			// the same commit.
			if postErr := c.handleInRepoConfigError(err, instance, change); postErr != nil {
				logger.WithError(postErr).Error("Failed reporting inrepoconfig processing error back to Gerrit.")
			}
			// Static postsubmit jobs are included as part of output from
			// inRepoConfigCacheHandler.GetPostsubmits, fallback to static only
			// when inrepoconfig failed.
			postsubmits = append(postsubmits, c.config().GetPostsubmitsStatic(cloneURI)...)
		}

		for _, postsubmit := range postsubmits {
			if shouldRun, err := postsubmit.ShouldRun(change.Branch, changedFiles); err != nil {
				return fmt.Errorf("failed to determine if postsubmit %q should run: %w", postsubmit.Name, err)
			} else if shouldRun {
				if change.Submitted != nil {
					triggerTimes[postsubmit.Name] = change.Submitted.Time
				}
				jobSpecs = append(jobSpecs, jobSpec{
					spec:        pjutil.PostsubmitSpec(postsubmit, refs),
					labels:      postsubmit.Labels,
					annotations: postsubmit.Annotations,
				})
			}
		}
	case client.New:
		var presubmits []config.Presubmit
		// Gerrit server might be unavailable intermittently, retry inrepoconfig
		// processing for increased reliability.
		for attempt := 0; attempt < inRepoConfigRetries; attempt++ {
			presubmits, err = c.inRepoConfigCacheHandler.GetPresubmits(cloneURI, func() (string, error) { return baseSHA, nil }, func() (string, error) { return change.CurrentRevision, nil })
			if err == nil {
				break
			}
		}
		if err != nil {
			// Reports error back to Gerrit. handleInRepoConfigError is
			// responsible for not sending the same message again and again on
			// the same commit.
			if postErr := c.handleInRepoConfigError(err, instance, change); postErr != nil {
				logger.WithError(postErr).Error("Failed reporting inrepoconfig processing error back to Gerrit.")
			}
			// There is no need to keep going when failed to get inrepoconfig
			// jobs.
			// Imagining the scenario that:
			// - Commit #abc triggered static job job-A, inrepoconfig jobs job-B
			// and job-C
			// - Both job-B and job-C failed
			// - Commit #def was pushed. Inrepoconfig failed, falling back to
			// trigger static job job-A.
			// - job-A passed.
			// - Prow would make decision on the result of job-A and ignore the
			// rest. (Yes this is a Prow bug, which should not be a problem when
			// each prowjob is reported to an individual Gerrit Check).
			// So long story short: kicking off partial prowjobs is worse than
			// kicking off nothing.
			return err
		}

		account, err := c.gc.Account(instance)
		if err != nil {
			// This would happen if authenticateOnce hasn't done register this instance yet
			return fmt.Errorf("account not found for %q: %w", instance, err)
		}

		lastUpdate, ok := c.tracker.Current()[instance][change.Project]
		if !ok {
			lastUpdate = time.Now()
			logger.WithField("lastUpdate", lastUpdate).Warnf("lastUpdate not found, falling back to now")
		}

		revision := change.Revisions[change.CurrentRevision]
		failedJobs := failedJobs(account.AccountID, revision.Number, change.Messages...)
		failed, all := presubmitContexts(failedJobs, presubmits, logger)
		messages := currentMessages(change, lastUpdate)
		logger.WithField("failed", len(failed)).Debug("Failed jobs parsed from previous comments.")
		filters := []pjutil.Filter{
			messageFilter(messages, failed, all, triggerTimes, logger),
		}
		// Automatically trigger the Prow jobs if the revision is new and the
		// change is not in WorkInProgress.
		if revision.Created.Time.After(lastUpdate) && !change.WorkInProgress {
			filters = append(filters, &timeAnnotationFilter{
				Filter:       pjutil.NewTestAllFilter(),
				eventTime:    revision.Created.Time,
				triggerTimes: triggerTimes,
			})
		}
		toTrigger, err := pjutil.FilterPresubmits(pjutil.NewAggregateFilter(filters), client.ChangedFilesProvider(&change), change.Branch, presubmits, logger)
		if err != nil {
			return fmt.Errorf("filter presubmits: %w", err)
		}
		// At this point triggerTimes should be properly populated as a side effect of FilterPresubmits.

		// Reply with help information to run the presubmit Prow jobs if requested.
		for _, msg := range messages {
			needsHelp, note := pjutil.ShouldRespondWithHelp(msg.Message, len(toTrigger))
			// Lock for projectOptOutHelp, which is a map.
			c.lock.RLock()
			optedOut := isProjectOptOutHelp(c.projectsOptOutHelp, instance, change.Project)
			c.lock.RUnlock()
			if needsHelp && !optedOut {
				runWithTestAllNames, optionalJobsCommands, requiredJobsCommands, err := pjutil.AvailablePresubmits(client.ChangedFilesProvider(&change), change.Branch, presubmits, logger.WithField("help", true))
				if err != nil {
					return err
				}
				message := pjutil.HelpMessage(instance, change.Project, change.Branch, note, runWithTestAllNames, optionalJobsCommands, requiredJobsCommands)
				if err := c.gc.SetReview(instance, change.ID, change.CurrentRevision, message, nil); err != nil {
					return err
				}
				gerritMetrics.triggerLatency.WithLabelValues(instance).Observe(float64(time.Since(msg.Date.Time).Seconds()))
				// Only respond to the first message that requests help information.
				break
			}
		}

		for _, presubmit := range toTrigger {
			jobSpecs = append(jobSpecs, jobSpec{
				spec:        pjutil.PresubmitSpec(presubmit, refs),
				labels:      presubmit.Labels,
				annotations: presubmit.Annotations,
			})
		}
	}

	for _, jSpec := range jobSpecs {
		labels, annotations := LabelsAndAnnotations(instance, jSpec.labels, jSpec.annotations, change)

		pj := pjutil.NewProwJob(jSpec.spec, labels, annotations)
		logger := logger.WithField("prowjob", pj.Name)
		if _, err := c.prowJobClient.Create(context.TODO(), &pj, metav1.CreateOptions{}); err != nil {
			logger.WithError(err).Errorf("Failed to create ProwJob")
			continue
		}
		logger.Infof("Triggered new job")
		if eventTime, ok := triggerTimes[pj.Spec.Job]; ok {
			gerritMetrics.triggerLatency.WithLabelValues(instance).Observe(float64(time.Since(eventTime).Seconds()))
		}
		triggeredJobs = append(triggeredJobs, triggeredJob{
			name:   jSpec.spec.Job,
			report: jSpec.spec.Report,
		})
	}

	if len(triggeredJobs) == 0 {
		return nil
	}

	// comment back to gerrit if Report is set for any of the jobs
	var reportingJobs int
	var jobList string
	for _, job := range triggeredJobs {
		if job.report {
			jobList += fmt.Sprintf("\n  * Name: %s", job.name)
			reportingJobs++
		}
	}

	if reportingJobs > 0 {
		message := fmt.Sprintf("Triggered %d prow jobs (%d suppressed reporting): ", len(triggeredJobs), len(triggeredJobs)-reportingJobs)
		// If we have a Deck URL, link to all results for the CL, otherwise list the triggered jobs.
		link, err := deckLinkForPR(c.config().Gerrit.DeckURL, refs, change.Status)
		if err != nil {
			logger.WithError(err).Error("Failed to generate link to job results on Deck.")
		}
		if link != "" && err == nil {
			message = message + link
		} else {
			message = message + jobList
		}
		if err := c.gc.SetReview(instance, change.ID, change.CurrentRevision, message, nil); err != nil {
			return err
		}
	}

	return nil
}

// isProjectOptOutHelp returns if the project is opt-out from getting help
// information about how to run presubmit tests on their changes.
func isProjectOptOutHelp(projectsOptOutHelp map[string]sets.String, instance, project string) bool {
	ps, ok := projectsOptOutHelp[instance]
	if !ok {
		return false
	}
	return ps.Has(project)
}

func deckLinkForPR(deckURL string, refs prowapi.Refs, changeStatus string) (string, error) {
	if deckURL == "" || changeStatus == client.Merged {
		return "", nil
	}

	parsed, err := url.Parse(deckURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse gerrit.deck_url (impossible: this should have been caught at load time): %w", err)
	}
	query := parsed.Query()
	query.Set("repo", fmt.Sprintf("%s/%s", refs.Org, refs.Repo))
	if len(refs.Pulls) != 1 {
		return "", fmt.Errorf("impossible: triggered jobs for a Gerrit change, but refs.pulls was empty")
	}
	query.Set("pull", strconv.Itoa(refs.Pulls[0].Number))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
