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
	inRepoConfigFailed  = "Unable to get inRepoConfig. This could be due to a merge conflict (please resolve them), an inRepoConfig parsing error (incorrect formatting) in the .prow directory or .prow.yaml file, or a flake. For possible flakes, try again with /test all"
)

var gerritMetrics = struct {
	processingResults           *prometheus.CounterVec
	inrepoconfigResults         *prometheus.CounterVec
	triggerLatency              *prometheus.HistogramVec
	triggerHelpLatency          *prometheus.HistogramVec
	changeProcessDuration       *prometheus.HistogramVec
	processSingleChangeDuration *prometheus.HistogramVec
	changeSyncDuration          *prometheus.HistogramVec
	gerritRepoQueryDuration     *prometheus.HistogramVec
	pickupChangeLatency         *prometheus.HistogramVec
	jobCreationDuration         *prometheus.HistogramVec
}{
	processingResults: prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gerrit_processing_results",
		Help: "Count of change processing by instance, repo, and result (ERROR or SUCCESS).",
	}, []string{
		"org",
		"repo",
		"result",
	}),
	inrepoconfigResults: prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gerrit_inrepoconfig_results",
		Help: "Count of retrieving inrepoconfigs by instance, repo, and result (ERROR or SUCCESS).",
	}, []string{
		"org",
		"repo",
		"result",
	}),
	triggerLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gerrit_trigger_latency",
		Help:    "Histogram of seconds between triggering event and ProwJob creation time.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30, 45, 60, 90, 120, 180, 300, 450, 600, 750, 900, 1050, 1200},
	}, []string{
		"org",
		// We would normally omit 'repo' to avoid excessive cardinality due to the number of buckets, but we need the data.
		// Hopefully this isn't excessive enough to cause metric scraping issues.
		"repo",
	}),
	triggerHelpLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gerrit_trigger_help_latency",
		Help:    "Histogram of seconds between triggering event (help) and ProwJob creation time.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30, 60, 120, 180, 300, 450, 600, 750, 900, 1050, 1200},
	}, []string{
		"org",
	}),
	processSingleChangeDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gerrit_process_single_change_duration",
		Help:    "Histogram of seconds spent processing a single gerrit change, by instance and repo.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30, 45, 60, 90, 120, 180, 300, 450, 600},
	}, []string{
		"org",
		"repo",
	}),
	changeProcessDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gerrit_instance_process_duration",
		Help:    "Histogram of seconds spent processing changes, by instance and repo. This measures the portion of a sync after we've queried for changes.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30, 45, 60, 90, 120, 180, 300, 450, 600, 750, 900, 1050, 1200},
	}, []string{
		"org", "repo",
	}),
	changeSyncDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gerrit_instance_change_sync_duration",
		Help:    "Histogram of seconds spent syncing changes from a single gerrit instance or repo. Includes gerrit_repo_query_duration and gerrit_instance_process_duration.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30, 45, 60, 90, 120, 180, 300, 450, 600, 750, 900, 1050, 1200},
	}, []string{"org", "repo"}),
	gerritRepoQueryDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gerrit_repo_query_duration",
		Help:    "Histogram of seconds spent querying a repo's changes. Includes time spent for rate limiting ourselves.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30, 45, 60, 90, 120, 240},
	}, []string{"org", "repo", "result"}),
	pickupChangeLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gerrit_pickup_change_latency",
		Help:    "Histogram of seconds a query result had to wait after it was retrieved from the Gerrit API but before it was picked up for processing by a worker thread.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30, 45, 60, 90, 120, 240},
	}, []string{"org", "repo"}),
	jobCreationDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gerrit_job_creation_duration",
		Help:    "Histogram of seconds spent creating a ProwJob object in the K8s API server of the Prow service cluster, by instance and repo.",
		Buckets: []float64{0.1, 0.2, 0.5, 0.75, 1, 2, 5, 7.5, 10, 15, 20},
	}, []string{
		"org",
		"repo",
	}),
}

func init() {
	prometheus.MustRegister(gerritMetrics.processingResults)
	prometheus.MustRegister(gerritMetrics.inrepoconfigResults)
	prometheus.MustRegister(gerritMetrics.triggerLatency)
	prometheus.MustRegister(gerritMetrics.triggerHelpLatency)
	prometheus.MustRegister(gerritMetrics.processSingleChangeDuration)
	prometheus.MustRegister(gerritMetrics.changeProcessDuration)
	prometheus.MustRegister(gerritMetrics.changeSyncDuration)
	prometheus.MustRegister(gerritMetrics.gerritRepoQueryDuration)
	prometheus.MustRegister(gerritMetrics.pickupChangeLatency)
	prometheus.MustRegister(gerritMetrics.jobCreationDuration)
}

type prowJobClient interface {
	Create(context.Context, *prowapi.ProwJob, metav1.CreateOptions) (*prowapi.ProwJob, error)
}

type gerritClient interface {
	ApplyGlobalConfig(orgRepoConfigGetter func() *config.GerritOrgRepoConfigs, lastSyncTracker *client.SyncTime, cookiefilePath, tokenPathOverride string, additionalFunc func())
	Authenticate(cookiefilePath, tokenPath string)
	QueryChangesForProject(instance, project string, lastUpdate time.Time, rateLimit int, additionalFilters ...string) ([]gerrit.ChangeInfo, error)
	GetBranchRevision(instance, project, branch string) (string, error)
	SetReview(instance, id, revision, message string, labels map[string]string) error
	Account(instance string) (*gerrit.AccountInfo, error)
	HasRelatedChanges(instance, id, revision string) (bool, error)
}

// Controller manages gerrit changes.
type Controller struct {
	config                      config.Getter
	prowJobClient               prowJobClient
	gc                          gerritClient
	tracker                     LastSyncTracker
	projectsOptOutHelp          map[string]sets.Set[string]
	lock                        sync.RWMutex
	cookieFilePath              string
	configAgent                 *config.Agent
	inRepoConfigGetter          config.InRepoConfigGetter
	inRepoConfigFailuresTracker map[string]bool
	projectsWithWorker          map[string]bool
	latestMux                   sync.Mutex
	workerPoolSize              int
}

type LastSyncTracker interface {
	Current() client.LastSyncState
	Update(client.LastSyncState) error
}

// NewController returns a new gerrit controller client
func NewController(ctx context.Context, prowJobClient prowv1.ProwJobInterface, op io.Opener,
	ca *config.Agent, cookiefilePath, tokenPathOverride, lastSyncFallback string, workerPoolSize int, maxQPS, maxBurst int, ircg config.InRepoConfigGetter) *Controller {

	cfg := ca.Config
	projectsOptOutHelpMap := map[string]sets.Set[string]{}
	if cfg().Gerrit.OrgReposConfig != nil {
		projectsOptOutHelpMap = cfg().Gerrit.OrgReposConfig.OptOutHelpRepos()
	}
	lastSyncTracker := client.NewSyncTime(lastSyncFallback, op, ctx)

	if err := lastSyncTracker.Init(cfg().Gerrit.OrgReposConfig.AllRepos()); err != nil {
		logrus.WithError(err).Fatal("Error initializing lastSyncFallback.")
	}
	gerritClient, err := client.NewClient(nil, maxQPS, maxBurst)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating gerrit client.")
	}
	c := &Controller{
		prowJobClient:               prowJobClient,
		config:                      cfg,
		gc:                          gerritClient,
		tracker:                     lastSyncTracker,
		projectsOptOutHelp:          projectsOptOutHelpMap,
		cookieFilePath:              cookiefilePath,
		configAgent:                 ca,
		inRepoConfigGetter:          ircg,
		inRepoConfigFailuresTracker: map[string]bool{},
		projectsWithWorker:          make(map[string]bool),
		workerPoolSize:              workerPoolSize,
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
	created    time.Time
}

func (c *Controller) processChange(latest client.LastSyncState, changeChan <-chan Change, log *logrus.Entry, wg *sync.WaitGroup, lastProjectSyncTime time.Time) {
	for changeStruct := range changeChan {
		change := changeStruct.changeInfo
		instance := changeStruct.instance
		gerritMetrics.pickupChangeLatency.WithLabelValues(instance, change.Project).Observe(float64(time.Since(changeStruct.created).Seconds()))

		log := log.WithFields(logrus.Fields{
			"branch":   change.Branch,
			"change":   change.Number,
			"repo":     change.Project,
			"revision": change.CurrentRevision,
		})

		now := time.Now()

		result := client.ResultSuccess
		if c.shouldTriggerJobs(change, lastProjectSyncTime) {
			if err := c.triggerJobs(log, instance, change); err != nil {
				result = client.ResultError
				log.WithError(err).Info("Failed to trigger jobs based on change")
			}
		} else {
			log.Info("Skipped triggering jobs for this change.")
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

		gerritMetrics.processSingleChangeDuration.WithLabelValues(instance, change.Project).Observe(float64(time.Since(now).Seconds()))
	}
}

func (c *Controller) processSingleProject(instance, project string) {
	// Assumes the passed in instance was already normalized with https:// prefix.
	log := logrus.WithFields(logrus.Fields{"host": instance, "repo": project})
	tracker := c.tracker.Current()
	syncTime := time.Now()
	if projects, ok := tracker[instance]; ok {
		if t, ok := projects[project]; ok {
			syncTime = t
		}
	}
	latest := tracker.DeepCopy()

	now := time.Now()
	defer func() {
		gerritMetrics.changeSyncDuration.WithLabelValues(instance, project).Observe(float64(time.Since(now).Seconds()))
	}()

	timeQueryChangesForProject := time.Now()

	// Ignore the error. It is already logged.
	changes, err := c.gc.QueryChangesForProject(instance, project, syncTime, c.config().Gerrit.RateLimit)
	queryResult := func() string {
		if err == nil {
			return client.ResultSuccess
		}
		return client.ResultError
	}()
	log = log.WithFields(logrus.Fields{
		"lastUpdate":    syncTime.String(),
		"queryStart":    timeQueryChangesForProject.String(),
		"queryDuration": time.Since(timeQueryChangesForProject).String(),
		"changeCount":   len(changes),
		"result":        queryResult,
	})
	gerritMetrics.gerritRepoQueryDuration.WithLabelValues(instance, project, queryResult).Observe((float64(time.Since(timeQueryChangesForProject).Seconds())))
	checkAndLogQuery(log, changes)

	if len(changes) == 0 {
		return
	}

	timeProcessChangesForProject := time.Now()
	var wg sync.WaitGroup
	wg.Add(len(changes))
	changeChan := make(chan Change)

	poolSize := c.workerPoolSize
	if poolSize > len(changes) {
		poolSize = len(changes)
	}
	for i := 0; i < poolSize; i++ {
		go c.processChange(latest, changeChan, log, &wg, syncTime)
	}
	// We need to call time.Now() outside this loop since <- will block
	// while there are no more available worker threads possibly causing
	// time.Now() to be called later than intended.
	timeChangesCreated := time.Now()
	for _, change := range changes {
		changeChan <- Change{changeInfo: change, instance: instance, created: timeChangesCreated}
	}
	wg.Wait()
	gerritMetrics.changeProcessDuration.WithLabelValues(instance, project).Observe((float64(time.Since(timeProcessChangesForProject).Seconds())))
	close(changeChan)
	c.tracker.Update(latest)
}

func checkAndLogQuery(log *logrus.Entry, changes []gerrit.ChangeInfo) {
	seen := sets.NewInt()
	for _, change := range changes {
		if seen.Has(change.Number) {
			log.WithField("change", change.Number).Error("Gerrit API bug! Received multiple updates for a change from a single query.")
		}
		seen.Insert(change.Number)
	}
	log.Infof("Query returned changes: %v", seen.List())
}

// Sync looks for newly made gerrit changes
// and creates prowjobs according to specs
func (c *Controller) Sync() {
	// Identify projects without worker threads
	id := func(instance, project string) string { return fmt.Sprintf("%s/%s", instance, project) }
	needsWorker := map[string][]string{}
	needsWorkerCount := map[string]int{}
	for instance, projects := range c.config().Gerrit.OrgReposConfig.AllRepos() {
		for project := range projects {
			if _, ok := c.projectsWithWorker[id(instance, project)]; ok {
				// The worker thread is already up for this project, nothing needs
				// to be done.
				continue
			}
			needsWorker[instance] = append(needsWorker[instance], project)
			needsWorkerCount[instance]++
		}
	}
	// First time seeing these projects, spin up worker threads for them.
	staggerPosition := 0
	for instance, projects := range needsWorker {
		staggerIncement := c.config().Gerrit.TickInterval.Duration / time.Duration(needsWorkerCount[instance])
		for _, project := range projects {
			c.projectsWithWorker[id(instance, project)] = true
			logrus.WithFields(logrus.Fields{"instance": instance, "repo": project}).Info("Starting worker for project.")
			go func(instance, project string, staggerPosition int) {
				// Stagger new worker threads across the loop period to reduce load on the Gerrit API and Git server.
				napTime := staggerIncement * time.Duration(staggerPosition)
				time.Sleep(napTime)

				// Now start the repo worker thread.
				previousRun := time.Now()
				for {
					timeDiff := time.Until(previousRun.Add(c.config().Gerrit.TickInterval.Duration))
					if timeDiff > 0 {
						time.Sleep(timeDiff)
					}
					previousRun = time.Now()
					c.processSingleProject(instance, project)
				}
			}(instance, project, staggerPosition)
			staggerPosition++
		}
	}
}

// CreateRefs creates refs for a presubmit job from given changes.
//
// Passed in instance must contain https:// prefix.
func CreateRefs(instance, project, branch, baseSHA string, changes ...client.ChangeInfo) (prowapi.Refs, error) {
	var refs prowapi.Refs
	cloneURI := source.CloneURIFromOrgRepo(instance, project)

	// Something like https://android.googlesource.com
	codeHost := source.EnsureCodeURL(instance)

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
func failedJobs(account int, revision int, messages ...gerrit.ChangeMessageInfo) sets.Set[string] {
	failures := sets.Set[string]{}
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
		// Only report back to Gerrit if we have not reported previously.
		// If any new `/test` commands are given and fail for the same reason we won't post another error message
		// which can be confusing to users. This behavior is to prevent us from reporting the failure again
		// on unrelated comments (including the error message itself!), but we don't need this behavior if
		// we don't process irrelevant comments which is the case if AllowedPresubmitTriggerRe is specified.
		skipIrrelevantComments := c.config().Gerrit.AllowedPresubmitTriggerReRawString != ""
		if _, alreadyReported := c.inRepoConfigFailuresTracker[key]; !alreadyReported || skipIrrelevantComments {
			msg := fmt.Sprintf("%s: %v", inRepoConfigFailed, err)
			if setReviewWerr := c.gc.SetReview(instance, change.ID, change.CurrentRevision, msg, nil); setReviewWerr != nil {
				return fmt.Errorf("failed to get inRepoConfig and failed to set Review to notify user: %v and %v", err, setReviewWerr)
			}
			// The boolean value here is meaningless as we use the tracker as a
			// set data structure, not as a hashmap where values actually
			// matter. We just use a bool for simplicity.
			c.inRepoConfigFailuresTracker[key] = true
		}

		// We do not want to return that there was an error processing change. If we are unable to get inRepoConfig we do not process. This is expected behavior.
		return nil
	}

	// If we are passing now, remove any record of previous failures in our
	// tracker to allow future failures to send an error message back to Gerrit
	// (through this same function).
	delete(c.inRepoConfigFailuresTracker, key)
	return nil
}

// shouldTriggerJobs returns true if we should trigger jobs for the given
// change.
func (c *Controller) shouldTriggerJobs(change client.ChangeInfo, lastProjectSyncTime time.Time) bool {
	// do not skip postsubmit jobs
	if change.Status == client.Merged {
		return true
	}
	revision := change.Revisions[change.CurrentRevision]
	if revision.Created.After(lastProjectSyncTime) {
		return true
	}

	for _, message := range currentMessages(change, lastProjectSyncTime) {
		if c.messageContainsJobTriggeringCommand(message) {
			return true
		}
		if indicatesChangeFromDraftToActiveState(message.Message) {
			return true
		}
	}

	return false
}

func (c *Controller) messageContainsJobTriggeringCommand(message gerrit.ChangeMessageInfo) bool {
	return pjutil.RetestRe.MatchString(message.Message) ||
		pjutil.TestAllRe.MatchString(message.Message) ||
		c.configAgent.Config().Gerrit.IsAllowedPresubmitTrigger(message.Message)
}

// triggerJobs creates new presubmit/postsubmit prowjobs base off the gerrit changes
func (c *Controller) triggerJobs(logger logrus.FieldLogger, instance string, change client.ChangeInfo) error {
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
	baseSHAGetter := func() (string, error) { return baseSHA, nil }
	var hasRelatedChanges *bool
	// This headSHAGetter will return the empty string instead of the head SHA in cases where we can be certain that change does not
	// modify inrepoconfig. This allows multiple changes to share a ProwYAML cache entry so long as they don't touch inrepo config themselves.
	headSHAGetter := func() (string, error) {
		changes, err := client.ChangedFilesProvider(&change)()
		if err != nil {
			// This is a best effort optimization, log the error, but just use CurrentRevision in this case.
			logger.WithError(err).Info("Failed to get changed files for the purpose of prowYAML cache optimization. Skipping optimization.")
			return change.CurrentRevision, nil
		}
		if config.ContainsInRepoConfigPath(changes) {
			return change.CurrentRevision, nil
		}
		if hasRelatedChanges == nil {
			if res, err := c.gc.HasRelatedChanges(instance, change.ChangeID, change.CurrentRevision); err != nil {
				logger.WithError(err).Info("Failed to get related changes for the purpose of prowYAML cache optimization. Skipping optimization.")
				return change.CurrentRevision, nil
			} else {
				hasRelatedChanges = &res
			}
		}
		if *hasRelatedChanges {
			// If the change is part of a chain the commit may include files not identified by the API.
			// So we can't easily check if the change includes inrepo config file changes.
			return change.CurrentRevision, nil
		}
		// If we know the change doesn't touch the inrepo config itself, we don't need to check out the head commits.
		// This is particularly useful because it lets multiple changes share a ProwYAML cache entry so long as they don't touch inrepo config themselves.
		return "", nil
	}

	switch change.Status {
	case client.Merged:
		var postsubmits []config.Postsubmit
		// Gerrit server might be unavailable intermittently, retry inrepoconfig
		// processing for increased reliability.
		for attempt := 0; attempt < inRepoConfigRetries; attempt++ {
			postsubmits, err = c.inRepoConfigGetter.GetPostsubmits(cloneURI, change.Branch, baseSHAGetter, headSHAGetter)
			// Break if there was no error, or if there was a merge conflict
			if err == nil {
				gerritMetrics.inrepoconfigResults.WithLabelValues(instance, change.Project, client.ResultSuccess).Inc()
				break
			}
			if strings.Contains(err.Error(), "Merge conflict in") {
				break
			}
		}
		// Postsubmit jobs are triggered only once. Still try to fall back on
		// static jobs if failed to retrieve inrepoconfig jobs.
		if err != nil {
			gerritMetrics.inrepoconfigResults.WithLabelValues(instance, change.Project, client.ResultError).Inc()

			// Reports error back to Gerrit. handleInRepoConfigError is
			// responsible for not sending the same message again and again on
			// the same commit.
			if postErr := c.handleInRepoConfigError(err, instance, change); postErr != nil {
				logger.WithError(postErr).Error("Failed reporting inrepoconfig processing error back to Gerrit.")
			}
			// Static postsubmit jobs are included as part of output from
			// inRepoConfigCache.GetPostsubmits, fallback to static only
			// when inrepoconfig failed.
			postsubmits = append(postsubmits, c.config().GetPostsubmitsStatic(cloneURI)...)
		}

		for _, postsubmit := range postsubmits {
			if shouldRun, err := postsubmit.ShouldRun(change.Branch, client.ChangedFilesProvider(&change)); err != nil {
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
			presubmits, err = c.inRepoConfigGetter.GetPresubmits(cloneURI, change.Branch, baseSHAGetter, headSHAGetter)
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
				gerritMetrics.triggerHelpLatency.WithLabelValues(instance).Observe(float64(time.Since(msg.Date.Time).Seconds()))
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
		timeBeforeCreate := time.Now()
		if _, err := c.prowJobClient.Create(context.TODO(), &pj, metav1.CreateOptions{}); err != nil {
			logger.WithError(err).Errorf("Failed to create ProwJob")
			continue
		}
		gerritMetrics.jobCreationDuration.WithLabelValues(instance, change.Project).Observe((float64(time.Since(timeBeforeCreate).Seconds())))
		logger.Infof("Triggered new job")
		if eventTime, ok := triggerTimes[pj.Spec.Job]; ok {
			gerritMetrics.triggerLatency.WithLabelValues(instance, change.Project).Observe(float64(time.Since(eventTime).Seconds()))
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
func isProjectOptOutHelp(projectsOptOutHelp map[string]sets.Set[string], instance, project string) bool {
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
