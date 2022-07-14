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
	"errors"
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
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/io"
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
	QueryChanges(lastState client.LastSyncState, rateLimit int) map[string][]client.ChangeInfo
	QueryChangesForInstance(instance string, lastState client.LastSyncState, rateLimit int) []client.ChangeInfo
	GetBranchRevision(instance, project, branch string) (string, error)
	SetReview(instance, id, revision, message string, labels map[string]string) error
	Account(instance string) (*gerrit.AccountInfo, error)
}

// Controller manages gerrit changes.
type Controller struct {
	config               config.Getter
	prowJobClient        prowJobClient
	gc                   gerritClient
	tracker              LastSyncTracker
	projectsOptOutHelp   map[string]sets.String
	lock                 sync.RWMutex
	cookieFilePath       string
	cacheSize            int
	configAgent          *config.Agent
	repoCacheMap         map[string]*config.InRepoConfigCache
	inRepoConfigFailures map[string]bool
	instancesWithWorker  map[string]bool
	repoCacheMapMux      sync.Mutex
	latestMux            sync.Mutex
	workerPoolSize       int
}

type LastSyncTracker interface {
	Current() client.LastSyncState
	Update(client.LastSyncState) error
}

// NewController returns a new gerrit controller client
func NewController(ctx context.Context, prowJobClient prowv1.ProwJobInterface, op io.Opener,
	ca *config.Agent, projects, projectsOptOutHelp map[string][]string, cookiefilePath, tokenPathOverride, lastSyncFallback string, cacheSize, workerPoolSize int) *Controller {

	cfg := ca.Config
	projectsOptOutHelpMap := map[string]sets.String{}
	if cfg().Gerrit.OrgReposConfig != nil {
		projectsOptOutHelpMap = cfg().Gerrit.OrgReposConfig.OptOutHelpRepos()
	} else {
		for i, p := range projectsOptOutHelp {
			projectsOptOutHelpMap[i] = sets.NewString(p...)
		}
	}
	lastSyncTracker := &syncTime{
		path:   lastSyncFallback,
		ctx:    ctx,
		opener: op,
	}
	if err := lastSyncTracker.init(projects); err != nil {
		logrus.WithError(err).Fatal("Error initializing lastSyncFallback.")
	}
	gerritClient, err := client.NewClient(projects)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating gerrit client.")
	}
	c := &Controller{
		prowJobClient:        prowJobClient,
		config:               cfg,
		gc:                   gerritClient,
		tracker:              lastSyncTracker,
		projectsOptOutHelp:   projectsOptOutHelpMap,
		cookieFilePath:       cookiefilePath,
		cacheSize:            cacheSize,
		configAgent:          ca,
		repoCacheMap:         map[string]*config.InRepoConfigCache{},
		inRepoConfigFailures: map[string]bool{},
		instancesWithWorker:  make(map[string]bool),
		workerPoolSize:       workerPoolSize,
	}

	// applyGlobalConfig reads gerrit configurations from global gerrit config,
	// it will completely override previously configured gerrit hosts and projects.
	// it will also by the way authenticate gerrit
	c.applyGlobalConfig(cfg, gerritClient, lastSyncTracker, cookiefilePath, tokenPathOverride)

	// Authenticate creates a goroutine for rotating token secrets when called the first
	// time, afterwards it only authenticate once.
	// applyGlobalConfig calls authenticate only when global gerrit config presents,
	// call it here is required for cases where gerrit repos are defined as command
	// line arg(which is going to be deprecated).
	gerritClient.Authenticate(cookiefilePath, tokenPathOverride)

	return c
}

func (c *Controller) applyGlobalConfig(cfg config.Getter, gerritClient *client.Client, lastSyncTracker *syncTime, cookiefilePath, tokenPathOverride string) {
	c.applyGlobalConfigOnce(cfg, gerritClient, lastSyncTracker, cookiefilePath, tokenPathOverride)

	go func() {
		for {
			c.applyGlobalConfigOnce(cfg, gerritClient, lastSyncTracker, cookiefilePath, tokenPathOverride)
			// No need to spin constantly, give it a break. It's ok that config change has one second delay.
			time.Sleep(time.Second)
		}
	}()
}

func (c *Controller) applyGlobalConfigOnce(cfg config.Getter, gerritClient *client.Client, lastSyncTracker *syncTime, cookiefilePath, tokenPathOverride string) {
	orgReposConfig := cfg().Gerrit.OrgReposConfig
	if orgReposConfig == nil {
		return
	}
	// Use globally defined gerrit repos if present
	if err := gerritClient.UpdateClients(orgReposConfig.AllRepos()); err != nil {
		logrus.WithError(err).Error("Updating clients.")
	}
	if err := lastSyncTracker.update(orgReposConfig.AllRepos()); err != nil {
		logrus.WithError(err).Error("Syncing states.")
	}

	c.lock.Lock()
	// Updates a map, lock to make sure it's thread safe.
	c.projectsOptOutHelp = orgReposConfig.OptOutHelpRepos()
	c.lock.Unlock()
	// Authenticate creates a goroutine for rotating token secrets when called the first
	// time, afterwards it only authenticate once.
	gerritClient.Authenticate(cookiefilePath, tokenPathOverride)
}

// Helper function to create the cache used for InRepoConfig. Currently only attempts to create cache and returns nil if failed.
func createCache(cloneURI *url.URL, cookieFilePath string, cacheSize int, configAgent *config.Agent) (cache *config.InRepoConfigCache, err error) {
	opts := git.ClientFactoryOpts{
		CloneURI:       cloneURI.String(),
		Host:           cloneURI.Host,
		CookieFilePath: cookieFilePath,
	}
	gc, err := git.NewClientFactory(opts.Apply)
	if err != nil {
		return nil, fmt.Errorf("failed to create Gerrit Client for InRepoConfig: %v", err)
	}
	// Initialize cache for fetching Presubmit and Postsubmit information. If
	// the cache cannot be initialized, exit with an error.
	cache, err = config.NewInRepoConfigCache(
		cacheSize,
		configAgent,
		config.NewInRepoConfigGitCache(gc))
	// If we cannot initialize the cache, exit with an error.
	if err != nil {
		return nil, fmt.Errorf("unable to initialize in-repo-config-cache with size %d: %v", cacheSize, err)
	}
	return cache, nil
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

		cloneURI, err := makeCloneURI(instance, change.Project)
		if err != nil {
			log.WithError(err).Error("makeCloneURI.")
		}

		c.repoCacheMapMux.Lock()
		cache, ok := c.repoCacheMap[cloneURI.String()]
		if !ok {
			if cache, err = createCache(cloneURI, c.cookieFilePath, c.cacheSize, c.configAgent); err != nil {
				c.repoCacheMapMux.Unlock()
				wg.Done()
				log.WithError(err).Error("create repo cache.")
				return
			}
			c.repoCacheMap[cloneURI.String()] = cache
		}
		c.repoCacheMapMux.Unlock()

		result := client.ResultSuccess
		if err := c.processChange(log, instance, change, cloneURI, cache); err != nil {
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

		var wg sync.WaitGroup
		wg.Add(len(changes))

		changeChan := make(chan Change)
		for i := 1; i < c.workerPoolSize; i++ {
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

func makeCloneURI(instance, project string) (*url.URL, error) {
	u, err := url.Parse(instance)
	if err != nil {
		return nil, fmt.Errorf("instance %s is not a url: %w", instance, err)
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
func listChangedFiles(changeInfo client.ChangeInfo) config.ChangedFilesProvider {
	return func() ([]string, error) {
		var changed []string
		revision := changeInfo.Revisions[changeInfo.CurrentRevision]
		for file := range revision.Files {
			changed = append(changed, file)
		}
		return changed, nil
	}
}

func createRefs(reviewHost string, change client.ChangeInfo, cloneURI *url.URL, baseSHA string) (prowapi.Refs, error) {
	rev, ok := change.Revisions[change.CurrentRevision]
	if !ok {
		return prowapi.Refs{}, fmt.Errorf("cannot find current revision for change %v", change.ID)
	}
	var codeHost string // Something like https://android.googlesource.com
	parts := strings.SplitN(reviewHost, ".", 2)
	codeHost = strings.TrimSuffix(parts[0], "-review")
	if len(parts) > 1 {
		codeHost += "." + parts[1]
	}
	refs := prowapi.Refs{
		Org:      cloneURI.Host,  // Something like android-review.googlesource.com
		Repo:     change.Project, // Something like platform/build
		BaseRef:  change.Branch,
		BaseSHA:  baseSHA,
		CloneURI: cloneURI.String(), // Something like https://android-review.googlesource.com/platform/build
		RepoLink: fmt.Sprintf("%s/%s", codeHost, change.Project),
		BaseLink: fmt.Sprintf("%s/%s/+/%s", codeHost, change.Project, baseSHA),
		Pulls: []prowapi.Pull{
			{
				Number:     change.Number,
				Author:     rev.Commit.Author.Name,
				SHA:        change.CurrentRevision,
				Ref:        rev.Ref,
				Link:       fmt.Sprintf("%s/c/%s/+/%d", reviewHost, change.Project, change.Number),
				CommitLink: fmt.Sprintf("%s/%s/+/%s", codeHost, change.Project, change.CurrentRevision),
				AuthorLink: fmt.Sprintf("%s/q/%s", reviewHost, rev.Commit.Author.Email),
			},
		},
	}
	return refs, nil
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
func (c *Controller) processChange(logger logrus.FieldLogger, instance string, change client.ChangeInfo, cloneURI *url.URL, cache *config.InRepoConfigCache) error {
	baseSHA, err := c.gc.GetBranchRevision(instance, change.Project, change.Branch)
	trimmedHostPath := cloneURI.Host + "/" + cloneURI.Path
	if err != nil {
		return fmt.Errorf("GetBranchRevision: %w", err)
	}

	type triggeredJob struct {
		name   string
		report bool
	}
	var triggeredJobs []triggeredJob
	triggerTimes := map[string]time.Time{}

	refs, err := createRefs(instance, change, cloneURI, baseSHA)
	if err != nil {
		return fmt.Errorf("createRefs from %s at %s: %w", cloneURI, baseSHA, err)
	}

	type jobSpec struct {
		spec   prowapi.ProwJobSpec
		labels map[string]string
	}

	var jobSpecs []jobSpec

	changedFiles := listChangedFiles(change)

	switch change.Status {
	case client.Merged:
		var postsubmits []config.Postsubmit
		for attempt := 0; attempt < inRepoConfigRetries; attempt++ {
			postsubmits, err = cache.GetPostsubmits(trimmedHostPath, func() (string, error) { return baseSHA, nil }, func() (string, error) { return change.CurrentRevision, nil })
			if err == nil {
				break
			}
		}
		if err := c.handleInRepoConfigError(err, instance, change); err != nil {
			return err
		}
		postsubmits = append(postsubmits, c.config().PostsubmitsStatic[cloneURI.String()]...)
		for _, postsubmit := range postsubmits {
			if shouldRun, err := postsubmit.ShouldRun(change.Branch, changedFiles); err != nil {
				return fmt.Errorf("failed to determine if postsubmit %q should run: %w", postsubmit.Name, err)
			} else if shouldRun {
				if change.Submitted != nil {
					triggerTimes[postsubmit.Name] = change.Submitted.Time
				}
				jobSpecs = append(jobSpecs, jobSpec{
					spec:   pjutil.PostsubmitSpec(postsubmit, refs),
					labels: postsubmit.Labels,
				})
			}
		}
	case client.New:
		var presubmits []config.Presubmit
		for attempt := 0; attempt < inRepoConfigRetries; attempt++ {
			presubmits, err = cache.GetPresubmits(trimmedHostPath, func() (string, error) { return baseSHA, nil }, func() (string, error) { return change.CurrentRevision, nil })
			if err == nil {
				break
			}
		}
		if err := c.handleInRepoConfigError(err, instance, change); err != nil {
			return err
		}
		presubmits = append(presubmits, c.config().PresubmitsStatic[cloneURI.String()]...)

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
		toTrigger, err := pjutil.FilterPresubmits(pjutil.NewAggregateFilter(filters), listChangedFiles(change), change.Branch, presubmits, logger)
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
				runWithTestAllNames, optionalJobsCommands, requiredJobsCommands, err := pjutil.AvailablePresubmits(listChangedFiles(change), cloneURI.Host, change.Project, change.Branch, presubmits, logger.WithField("help", true))
				if err != nil {
					return err
				}
				message := pjutil.HelpMessage(cloneURI.Host, change.Project, change.Branch, note, runWithTestAllNames, optionalJobsCommands, requiredJobsCommands)
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
				spec:   pjutil.PresubmitSpec(presubmit, refs),
				labels: presubmit.Labels,
			})
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
		labels[client.GerritPatchset] = strconv.Itoa(change.Revisions[change.CurrentRevision].Number)

		if _, ok := labels[client.GerritReportLabel]; !ok {
			logger.WithField("job", jSpec.spec.Job).Debug("Job uses default value of 'Code-Review' for 'prow.k8s.io/gerrit-report-label' label. This default will removed in March 2022.")
			labels[client.GerritReportLabel] = client.CodeReview
		}

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
