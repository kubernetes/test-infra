/*
Copyright 2015 The Kubernetes Authors.

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

package mungers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	utilclock "k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/contrib/test-utils/utils"
	"k8s.io/test-infra/mungegithub/features"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/mungeopts"
	"k8s.io/test-infra/mungegithub/mungers/e2e"
	fake_e2e "k8s.io/test-infra/mungegithub/mungers/e2e/fake"
	"k8s.io/test-infra/mungegithub/mungers/mungerutil"
	"k8s.io/test-infra/mungegithub/mungers/shield"
	"k8s.io/test-infra/mungegithub/options"
	"k8s.io/test-infra/mungegithub/sharedmux"

	"github.com/NYTimes/gziphandler"
	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	approvedLabel                  = "approved"
	lgtmLabel                      = "lgtm"
	retestNotRequiredLabel         = "retest-not-required"
	retestNotRequiredDocsOnlyLabel = "retest-not-required-docs-only"
	doNotMergeLabel                = "do-not-merge"
	wipLabel                       = "do-not-merge/work-in-progress"
	holdLabel                      = "do-not-merge/hold"
	releaseNoteLabelNeeded         = "do-not-merge/release-note-label-needed"
	cherrypickUnapprovedLabel      = "do-not-merge/cherry-pick-not-approved"
	cncfClaYesLabel                = "cncf-cla: yes"
	cncfClaNoLabel                 = "cncf-cla: no"
	claHumanLabel                  = "cla: human-approved"
	criticalFixLabel               = "queue/critical-fix"
	blocksOthersLabel              = "queue/blocks-others"
	fixLabel                       = "queue/fix"
	multirebaseLabel               = "queue/multiple-rebases"

	sqContext = "Submit Queue"

	githubE2EPollTime = 30 * time.Second
)

var (
	// This MUST cause a RETEST of everything in the mungeopts.RequiredContexts.Retest
	newRetestBody = "/test all [submit-queue is verifying that this PR is safe to merge]"

	// this is the order in which labels will be compared for queue priority
	labelPriorities = []string{criticalFixLabel, retestNotRequiredLabel, retestNotRequiredDocsOnlyLabel, multirebaseLabel, fixLabel, blocksOthersLabel}
	// high priority labels are checked before the release
	lastHighPriorityLabel = 2 // retestNotRequiredDocsOnlyLabel
)

type submitStatus struct {
	Time time.Time
	statusPullRequest
	Reason string
}

type statusPullRequest struct {
	Number    int
	URL       string
	Title     string
	Login     string
	AvatarURL string
	Additions int
	Deletions int
	ExtraInfo []string
	BaseRef   string
}

type e2eQueueStatus struct {
	E2ERunning  *statusPullRequest
	E2EQueue    []*statusPullRequest
	BatchStatus *submitQueueBatchStatus
}

type submitQueueStatus struct {
	PRStatus map[string]submitStatus
}

// Information about the e2e test health. Call updateHealth on the SubmitQueue
// at roughly constant intervals to keep this up to date. The mergeable fraction
// of time for the queue as a whole and the individual jobs will then be
// NumStable[PerJob] / TotalLoops.
type submitQueueHealth struct {
	TotalLoops       int
	NumStable        int
	NumStablePerJob  map[string]int
	MergePossibleNow bool
}

// Generate health information using a queue of healthRecords. The bools are
// true for stable and false otherwise.
type healthRecord struct {
	Time    time.Time
	Overall bool
	Jobs    map[string]bool
}

// information about the sq itself including how fast things are merging and
// how long since the last merge
type submitQueueStats struct {
	Added              int // Number of items added to the queue since restart
	FlakesIgnored      int
	Initialized        bool // true if we've made at least one complete pass
	InstantMerges      int  // Number of merges without retests required
	BatchMerges        int  // Number of merges caused by batch
	LastMergeTime      time.Time
	MergeRate          float64
	MergesSinceRestart int
	Removed            int // Number of items dequeued since restart
	RetestsAvoided     int
	StartTime          time.Time
	Tested             int // Number of e2e tests completed
}

// pull-request that has been tested as successful, but interrupted because head flaked
type submitQueueInterruptedObject struct {
	obj *github.MungeObject
	// If these two items match when we're about to kick off a retest, it's safe to skip the retest.
	interruptedMergeHeadSHA string
	interruptedMergeBaseSHA string
}

// Contains metadata about this instance of the submit queue such as URLs.
// Consumed by the template system.
type submitQueueMetadata struct {
	ProjectName string

	ChartURL string
	// chartURL is an option storage location. It is distinct from ChartURL
	// since the public variables are used asynchronously by a fileserver
	// and updates to the options values should not cause a race condition.
	chartURL string

	RepoPullURL string
	ProwURL     string
}

type submitQueueBatchStatus struct {
	Error   map[string]string
	Running *prowJob
}

type prometheusMetrics struct {
	Blocked       prometheus.Gauge
	OpenPRs       prometheus.Gauge
	QueuedPRs     prometheus.Gauge
	MergeCount    prometheus.Counter
	LastMergeTime prometheus.Gauge
}

var (
	sqPromMetrics = prometheusMetrics{
		Blocked: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "submitqueue_blocked",
			Help: "The submit-queue is currently blocked",
		}),
		OpenPRs: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "submitqueue_open_pullrequests_total",
			Help: "Number of open pull-requests",
		}),
		QueuedPRs: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "submitqueue_queued_pullrequests_total",
			Help: "Number of pull-requests queued",
		}),
		MergeCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "submitqueue_merge_total",
			Help: "Number of merges done",
		}),
		LastMergeTime: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "submitqueue_time_of_last_merge",
			Help: "Time of last merge",
		}),
	}
)

// marshaled in serveCIStatus
type jobStatus struct {
	State   string `json:"state"`
	BuildID string `json:"build_id"`
	URL     string `json:"url"`
}

// SubmitQueue will merge PR which meet a set of requirements.
//  PR must have LGTM after the last commit
//  PR must have passed all github CI checks
//  The google internal jenkins instance must be passing the BlockingJobNames e2e tests
type SubmitQueue struct {
	githubConfig        *github.Config
	opts                *options.Options
	NonBlockingJobNames []string

	GateApproved                 bool
	GateCLA                      bool
	GateGHReviewApproved         bool
	GateGHReviewChangesRequested bool

	// AdditionalRequiredLabels is a set of additional labels required for merging
	// on top of the existing required ("lgtm", "approved", "cncf-cla: yes").
	AdditionalRequiredLabels []string

	// BlockingLabels is a set of labels that forces the submit queue to ignore
	// pull requests.
	BlockingLabels []string

	// If FakeE2E is true, don't try to connect to JenkinsHost, all jobs are passing.
	FakeE2E bool

	// All valid cla labels
	ClaYesLabels []string

	DoNotMergeMilestones []string

	Metadata  submitQueueMetadata
	AdminPort int

	sync.Mutex
	prStatus       map[string]submitStatus // protected by sync.Mutex
	statusHistory  []submitStatus          // protected by sync.Mutex
	lastClosedTime time.Time

	clock         utilclock.Clock
	startTime     time.Time // when the queue started (duh)
	lastMergeTime time.Time
	totalMerges   int32
	mergeRate     float64 // per 24 hours
	loopStarts    int32   // if > 1, then we must have made a complete pass.

	githubE2ERunning   *github.MungeObject         // protect by sync.Mutex!
	githubE2EQueue     map[int]*github.MungeObject // protected by sync.Mutex!
	githubE2EPollTime  time.Duration
	lgtmTimeCache      *mungerutil.LabelTimeCache
	githubE2ELastPRNum int

	lastE2EStable bool // was e2e stable last time they were checked, protect by sync.Mutex
	e2e           e2e.E2ETester

	interruptedObj *submitQueueInterruptedObject
	flakesIgnored  int32 // Increments for each merge while 1+ job is flaky
	instantMerges  int32 // Increments whenever we merge without retesting
	batchMerges    int32 // Increments whenever we merge because of a batch
	prsAdded       int32 // Increments whenever an items queues
	prsRemoved     int32 // Increments whenever an item dequeues
	prsTested      int32 // Number of prs that completed second testing
	retestsAvoided int32 // Increments whenever we skip due to head not changing.

	health        submitQueueHealth
	healthHistory []healthRecord

	emergencyMergeStopFlag int32

	features *features.Features

	mergeLock    sync.Mutex // acquired when attempting to merge a specific PR
	ProwURL      string     // prow base page
	BatchEnabled bool
	ContextURL   string
	batchStatus  submitQueueBatchStatus
	ciStatus     map[string]map[string]jobStatus // type (eg batch) : job : status

	// MergeToMasterMessage is an extra message when PR is merged to master branch,
	// it must not end in a period.
	MergeToMasterMessage string
}

func init() {
	clock := utilclock.RealClock{}
	prometheus.MustRegister(sqPromMetrics.Blocked)
	prometheus.MustRegister(sqPromMetrics.OpenPRs)
	prometheus.MustRegister(sqPromMetrics.QueuedPRs)
	prometheus.MustRegister(sqPromMetrics.MergeCount)
	prometheus.MustRegister(sqPromMetrics.LastMergeTime)
	sq := &SubmitQueue{
		clock:          clock,
		startTime:      clock.Now(),
		lastMergeTime:  clock.Now(),
		lastE2EStable:  true,
		prStatus:       map[string]submitStatus{},
		githubE2EQueue: map[int]*github.MungeObject{},
	}
	RegisterMungerOrDie(sq)
	RegisterStaleIssueComments(sq)
}

// Name is the name usable in --pr-mungers
func (sq *SubmitQueue) Name() string { return "submit-queue" }

// RequiredFeatures is a slice of 'features' that must be provided
func (sq *SubmitQueue) RequiredFeatures() []string {
	return []string{features.BranchProtectionFeature, features.ServerFeatureName}
}

func (sq *SubmitQueue) emergencyMergeStop() bool {
	return atomic.LoadInt32(&sq.emergencyMergeStopFlag) != 0
}

func (sq *SubmitQueue) setEmergencyMergeStop(stopMerges bool) {
	if stopMerges {
		atomic.StoreInt32(&sq.emergencyMergeStopFlag, 1)
	} else {
		atomic.StoreInt32(&sq.emergencyMergeStopFlag, 0)
	}
}

// EmergencyStopHTTP sets the emergency stop flag. It expects the path of
// req.URL to contain either "emergency/stop", "emergency/resume", or "emergency/status".
func (sq *SubmitQueue) EmergencyStopHTTP(res http.ResponseWriter, req *http.Request) {
	switch {
	case strings.Contains(req.URL.Path, "emergency/stop"):
		sq.setEmergencyMergeStop(true)
	case strings.Contains(req.URL.Path, "emergency/resume"):
		sq.setEmergencyMergeStop(false)
	case strings.Contains(req.URL.Path, "emergency/status"):
	default:
		http.NotFound(res, req)
		return
	}
	sq.serve(sq.marshal(struct{ EmergencyInProgress bool }{sq.emergencyMergeStop()}), res, req)
}

func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}

func toFixed(num float64) float64 {
	output := math.Pow(10, float64(3))
	return float64(round(num*output)) / output
}

// This is the calculation of the exponential smoothing factor. It tries to
// make sure that if we get lots of fast merges we don't race the 'daily'
// avg really high really fast. But more importantly it means that if merges
// start going slowly the 'daily' average will get pulled down a lot by one
// slow merge instead of requiring numerous merges to get pulled down
func getSmoothFactor(dur time.Duration) float64 {
	hours := dur.Hours()
	smooth := .155*math.Log(hours) + .422
	if smooth < .1 {
		return .1
	}
	if smooth > .999 {
		return .999
	}
	return smooth
}

// This calculates an exponentially smoothed merge Rate based on the formula
//   newRate = (1-smooth)oldRate + smooth*newRate
// Which is really great and simple for constant time series data. But of course
// ours isn't time series data, so I vary the smoothing factor based on how long
// it has been since the last entry. See the comments on the `getSmoothFactor` for
// a discussion of why.
//    This whole thing was dreamed up by eparis one weekend via a combination
//    of guess-and-test and intuition. Someone who knows about this stuff
//    is likely to laugh at the naivete. Point him to where someone intelligent
//    has thought about this stuff and he will gladly do something smart.
// Merges that took less than 5 minutes are ignored completely for the rate
// calculation.
func calcMergeRate(oldRate float64, last, now time.Time) float64 {
	since := now.Sub(last)
	if since <= 5*time.Minute {
		// retest-not-required PR merges shouldn't affect our best
		// guess about the rate.
		return oldRate
	}
	var rate float64
	if since == 0 {
		rate = 96
	} else {
		rate = 24.0 * time.Hour.Hours() / since.Hours()
	}
	smoothingFactor := getSmoothFactor(since)
	mergeRate := ((1.0 - smoothingFactor) * oldRate) + (smoothingFactor * rate)
	return toFixed(mergeRate)
}

// Updates a smoothed rate at which PRs are merging per day.
// Updates merge stats. Should be called once for every merge.
func (sq *SubmitQueue) updateMergeRate() {
	now := sq.clock.Now()
	sq.mergeRate = calcMergeRate(sq.mergeRate, sq.lastMergeTime, now)

	// Update stats
	sqPromMetrics.MergeCount.Inc()
	atomic.AddInt32(&sq.totalMerges, 1)
	sq.lastMergeTime = now
	sqPromMetrics.LastMergeTime.Set(float64(sq.lastMergeTime.Unix()))
}

// This calculated the smoothed merge rate BUT it looks at the time since
// the last merge vs 'Now'. If we have not passed the next 'expected' time
// for a merge this just returns previous calculations. If 'Now' is later
// than we would expect given the existing mergeRate then pretend a merge
// happened right now and return the new merge rate. This way the merge rate
// is lower even if no merge has happened in a long time.
func (sq *SubmitQueue) calcMergeRateWithTail() float64 {
	now := sq.clock.Now()

	if sq.mergeRate == 0 {
		return 0
	}
	// Figure out when we think the next merge would happen given the history
	next := time.Duration(24/sq.mergeRate*time.Hour.Hours()) * time.Hour
	expectedMergeTime := sq.lastMergeTime.Add(next)

	// If we aren't there yet, just return the history
	if !now.After(expectedMergeTime) {
		return sq.mergeRate
	}

	// Pretend as though a merge happened right now to pull down the rate
	return calcMergeRate(sq.mergeRate, sq.lastMergeTime, now)
}

// Initialize will initialize the munger
func (sq *SubmitQueue) Initialize(config *github.Config, features *features.Features) error {
	sq.features = features
	return sq.internalInitialize(config, features, "")
}

// internalInitialize will initialize the munger.
// if overrideURL is specified, will create testUtils
func (sq *SubmitQueue) internalInitialize(config *github.Config, features *features.Features, overrideURL string) error {
	sq.Lock()
	defer sq.Unlock()

	// initialize to invalid pr number
	sq.githubE2ELastPRNum = -1

	sq.Metadata.ChartURL = sq.Metadata.chartURL
	sq.Metadata.ProwURL = sq.ProwURL
	sq.Metadata.RepoPullURL = fmt.Sprintf("https://github.com/%s/%s/pulls/", config.Org, config.Project)
	sq.Metadata.ProjectName = strings.Title(config.Project)
	sq.githubConfig = config

	if sq.BatchEnabled && sq.ProwURL == "" {
		return errors.New("batch merges require prow-url to be set")
	}

	// TODO: This is not how injection for tests should work.
	if sq.FakeE2E {
		sq.e2e = &fake_e2e.FakeE2ETester{}
	} else {
		var gcs *utils.Utils
		if overrideURL != "" {
			gcs = utils.NewTestUtils("bucket", "logs", overrideURL)
		} else {
			gcs = utils.NewWithPresubmitDetection(
				mungeopts.GCS.BucketName, mungeopts.GCS.LogDir,
				mungeopts.GCS.PullKey, mungeopts.GCS.PullLogDir,
			)
		}

		sq.e2e = (&e2e.RealE2ETester{
			Opts:                 sq.opts,
			NonBlockingJobNames:  &sq.NonBlockingJobNames,
			BuildStatus:          map[string]e2e.BuildInfo{},
			GoogleGCSBucketUtils: gcs,
		}).Init(sharedmux.Admin)
	}

	sq.lgtmTimeCache = mungerutil.NewLabelTimeCache(lgtmLabel)

	if features.Server.Enabled {
		features.Server.Handle("/prs", gziphandler.GzipHandler(http.HandlerFunc(sq.servePRs)))
		features.Server.Handle("/history", gziphandler.GzipHandler(http.HandlerFunc(sq.serveHistory)))
		features.Server.Handle("/github-e2e-queue", gziphandler.GzipHandler(http.HandlerFunc(sq.serveGithubE2EStatus)))
		features.Server.Handle("/merge-info", gziphandler.GzipHandler(http.HandlerFunc(sq.serveMergeInfo)))
		features.Server.Handle("/priority-info", gziphandler.GzipHandler(http.HandlerFunc(sq.servePriorityInfo)))
		features.Server.Handle("/health", gziphandler.GzipHandler(http.HandlerFunc(sq.serveHealth)))
		features.Server.Handle("/health.svg", gziphandler.GzipHandler(http.HandlerFunc(sq.serveHealthSVG)))
		features.Server.Handle("/sq-stats", gziphandler.GzipHandler(http.HandlerFunc(sq.serveSQStats)))
		features.Server.Handle("/flakes", gziphandler.GzipHandler(http.HandlerFunc(sq.serveFlakes)))
		features.Server.Handle("/metadata", gziphandler.GzipHandler(http.HandlerFunc(sq.serveMetadata)))
		if sq.BatchEnabled {
			features.Server.Handle("/batch", gziphandler.GzipHandler(http.HandlerFunc(sq.serveBatch)))
		}
		// this endpoint is useless without access to prow
		if sq.ProwURL != "" {
			features.Server.Handle("/ci-status", gziphandler.GzipHandler(http.HandlerFunc(sq.serveCIStatus)))
		}
	}

	sharedmux.Admin.HandleFunc("/api/emergency/stop", sq.EmergencyStopHTTP)
	sharedmux.Admin.HandleFunc("/api/emergency/resume", sq.EmergencyStopHTTP)
	sharedmux.Admin.HandleFunc("/api/emergency/status", sq.EmergencyStopHTTP)

	if sq.githubE2EPollTime == 0 {
		sq.githubE2EPollTime = githubE2EPollTime
	}

	sq.healthHistory = make([]healthRecord, 0)

	go sq.handleGithubE2EAndMerge()
	go sq.updateGoogleE2ELoop()
	if sq.BatchEnabled {
		go sq.handleGithubE2EBatchMerge()
	}
	if sq.ProwURL != "" {
		go sq.monitorProw()
	}

	if sq.AdminPort != 0 {
		go http.ListenAndServe(fmt.Sprintf("0.0.0.0:%v", sq.AdminPort), sharedmux.Admin)
	}
	return nil
}

// EachLoop is called at the start of every munge loop
func (sq *SubmitQueue) EachLoop() error {
	issues := []*githubapi.Issue{}
	if !sq.lastClosedTime.IsZero() {
		listOpts := &githubapi.IssueListByRepoOptions{
			State: "closed",
			Since: sq.lastClosedTime,
		}
		var err error
		issues, err = sq.githubConfig.ListAllIssues(listOpts)
		if err != nil {
			return err
		}
	} else {
		sq.lastClosedTime = time.Now()
	}

	sq.Lock()
	for _, issue := range issues {
		if issue.ClosedAt != nil && issue.ClosedAt.After(sq.lastClosedTime) {
			sq.lastClosedTime = *issue.ClosedAt
		}
		delete(sq.prStatus, strconv.Itoa(*issue.Number))
	}

	sq.updateHealth()
	sqPromMetrics.OpenPRs.Set(float64(len(sq.prStatus)))
	sqPromMetrics.QueuedPRs.Set(float64(len(sq.githubE2EQueue)))

	objs := []*github.MungeObject{}
	for _, obj := range sq.githubE2EQueue {
		objs = append(objs, obj)
	}
	sq.Unlock()

	for _, obj := range objs {
		obj.Refresh()
		// This should recheck it and clean up the queue, we don't care about the result
		_ = sq.validForMerge(obj)
	}
	atomic.AddInt32(&sq.loopStarts, 1)
	return nil
}

// RegisterOptions registers options for this munger; returns any that require a restart when changed.
func (sq *SubmitQueue) RegisterOptions(opts *options.Options) sets.String {
	sq.opts = opts
	opts.RegisterStringSlice(&sq.NonBlockingJobNames, "nonblocking-jobs", []string{}, "Comma separated list of jobs that don't block merges, but will have status reported and issues filed.")
	opts.RegisterStringSlice(&sq.AdditionalRequiredLabels, "additional-required-labels", []string{}, "Comma separated list of labels required for merging PRs on top of the existing required.")
	opts.RegisterStringSlice(&sq.BlockingLabels, "blocking-labels", []string{}, "Comma separated list of labels required to miss from PRs in order to consider them mergeable.")
	opts.RegisterBool(&sq.FakeE2E, "fake-e2e", false, "Whether to use a fake for testing E2E stability.")
	opts.RegisterStringSlice(&sq.DoNotMergeMilestones, "do-not-merge-milestones", []string{}, "List of milestones which, when applied, will cause the PR to not be merged.")
	opts.RegisterInt(&sq.AdminPort, "admin-port", 9999, "If non-zero, will serve administrative actions on this port.")
	opts.RegisterString(&sq.Metadata.chartURL, "chart-url", "", "URL to access the submit-queue instance's health charts.")
	opts.RegisterString(&sq.ProwURL, "prow-url", "", "Prow deployment base URL to read batch results and direct users to.")
	opts.RegisterBool(&sq.BatchEnabled, "batch-enabled", false, "Do batch merges (requires prow/splice coordination).")
	opts.RegisterString(&sq.ContextURL, "context-url", "", "URL where the submit queue is serving - used in Github status contexts.")
	opts.RegisterBool(&sq.GateApproved, "gate-approved", false, "Gate on approved label.")
	opts.RegisterBool(&sq.GateCLA, "gate-cla", false, "Gate on cla labels.")
	opts.RegisterString(&sq.MergeToMasterMessage, "merge-to-master-message", "", "Extra message when PR is merged to master branch.")
	opts.RegisterBool(&sq.GateGHReviewApproved, "gh-review-approved", false, "Gate github review, approve")
	opts.RegisterBool(&sq.GateGHReviewChangesRequested, "gh-review-changes-requested", false, "Gate github review, changes request")
	opts.RegisterStringSlice(&sq.ClaYesLabels, "cla-yes-labels", []string{cncfClaYesLabel, claHumanLabel}, "Comma separated list of labels that would be counted as valid cla labels")

	opts.RegisterUpdateCallback(func(changed sets.String) error {
		if changed.HasAny("prow-url", "batch-enabled") {
			if sq.BatchEnabled && sq.ProwURL == "" {
				return fmt.Errorf("batch merges require prow-url to be set")
			}
		}
		if changed.HasAny("gate-cla", "cla-yes-labels") {
			if sq.GateCLA && len(sq.ClaYesLabels) == 0 {
				return fmt.Errorf("gating cla require at least one cla yes label. Default are %s and %s", cncfClaYesLabel, claHumanLabel)
			}
		}
		return nil
	})

	return sets.NewString(
		"batch-enabled", // Need to start or kill batch processing.
		"context-url",   // Need to remunge all PRs to update statuses with new url.
		"admin-port",    // Need to restart server on new port.
		// For the following: need to restart fileserver.
		"chart-url",
		// For the following: need to re-initialize e2e which is used by other goroutines.
		"fake-e2e",
		"gcs-bucket",
		"gcs-logs-dir",
		"pull-logs-dir",
		"pull-key",
		// For the following: need to remunge all PRs if changed from true to false.
		"gate-cla",
		"gate-approved",
		// Need to remunge all PRs if anything changes in the following sets.
		"additional-required-labels",
		"blocking-labels",
		"cla-yes-labels",
		"required-retest-contexts",
	)
}

// Hold the lock
func (sq *SubmitQueue) updateHealth() {
	// Remove old entries from the front.
	for len(sq.healthHistory) > 0 && time.Since(sq.healthHistory[0].Time).Hours() > 24.0 {
		sq.healthHistory = sq.healthHistory[1:]
	}
	// Make the current record
	emergencyStop := sq.emergencyMergeStop()
	newEntry := healthRecord{
		Time:    time.Now(),
		Overall: !emergencyStop,
		Jobs:    map[string]bool{},
	}
	for job, status := range sq.e2e.GetBuildStatus() {
		// Ignore flakes.
		newEntry.Jobs[job] = status.Status != "Not Stable"
	}
	if emergencyStop {
		// invent an "emergency stop" job that's failing.
		newEntry.Jobs["Emergency Stop"] = false
	}
	sq.healthHistory = append(sq.healthHistory, newEntry)
	// Now compute the health structure so we don't have to do it on page load
	sq.health.TotalLoops = len(sq.healthHistory)
	sq.health.NumStable = 0
	sq.health.NumStablePerJob = map[string]int{}
	sq.health.MergePossibleNow = !emergencyStop
	if sq.health.MergePossibleNow {
		sqPromMetrics.Blocked.Set(0)
	} else {
		sqPromMetrics.Blocked.Set(1)
	}
	for _, record := range sq.healthHistory {
		if record.Overall {
			sq.health.NumStable++
		}
		for job, stable := range record.Jobs {
			if _, ok := sq.health.NumStablePerJob[job]; !ok {
				sq.health.NumStablePerJob[job] = 0
			}
			if stable {
				sq.health.NumStablePerJob[job]++
			}
		}
	}
}

func (sq *SubmitQueue) monitorProw() {
	nonBlockingJobNames := make(map[string]bool)
	requireRetestJobNames := make(map[string]bool)

	for {
		sq.opts.Lock()
		for _, jobName := range sq.NonBlockingJobNames {
			nonBlockingJobNames[jobName] = true
		}
		for _, jobName := range mungeopts.RequiredContexts.Retest {
			requireRetestJobNames[jobName] = true
		}
		url := sq.ProwURL + "/data.js"

		currentPR := -1
		if sq.githubE2ERunning != nil {
			currentPR = *sq.githubE2ERunning.Issue.Number
		}
		sq.opts.Unlock()

		lastPR := sq.githubE2ELastPRNum
		// get current job info from prow
		allJobs, err := getJobs(url)
		if err != nil {
			glog.Errorf("Error reading batch jobs from Prow URL %v: %v", url, err)
			time.Sleep(time.Minute)
			continue
		}
		// TODO: copy these from sq first instead
		ciStatus := make(map[string]map[string]jobStatus)
		ciLatest := make(map[string]map[string]time.Time)

		for _, job := range allJobs {
			if job.Finished == "" || job.BuildID == "" {
				continue
			}
			// type/category
			key := job.Type + "/"
			// the most recent submit-queue PR(s)
			if job.Number == currentPR || job.Number == lastPR {
				key += "single"
			} else if nonBlockingJobNames[job.Job] {
				key += "nonblocking"
			} else if requireRetestJobNames[job.Job] {
				key += "requiredretest"
			}

			ft, err := time.Parse(time.RFC3339Nano, job.Finished)
			if err != nil {
				glog.Errorf("Error parsing job finish time %s: %v", job.Finished, err)
				continue
			}

			if _, ok := ciLatest[key]; !ok {
				ciLatest[key] = make(map[string]time.Time)
				ciStatus[key] = make(map[string]jobStatus)
			}
			latest, ok := ciLatest[key][job.Job]

			// TODO: flake cache?
			if !ok || latest.Before(ft) {
				ciLatest[key][job.Job] = ft
				ciStatus[key][job.Job] = jobStatus{
					State:   job.State,
					BuildID: job.BuildID,
					URL:     job.URL,
				}
			}
		}

		sq.Lock()
		sq.ciStatus = ciStatus
		sq.Unlock()

		time.Sleep(time.Minute)
	}
}

func (sq *SubmitQueue) e2eStable(aboutToMerge bool) bool {
	wentStable := false
	wentUnstable := false

	sq.e2e.LoadNonBlockingStatus()
	stable := !sq.emergencyMergeStop()

	sq.Lock()
	last := sq.lastE2EStable
	if last && !stable {
		wentUnstable = true
	} else if !last && stable {
		wentStable = true
	}
	sq.lastE2EStable = stable
	sq.Unlock()

	reason := ""
	avatar := ""
	if wentStable {
		reason = e2eRecover
		avatar = "success.png"
	} else if wentUnstable {
		reason = e2eFailure
		avatar = "error.png"
	}
	if reason != "" {
		submitStatus := submitStatus{
			Time: sq.clock.Now(),
			statusPullRequest: statusPullRequest{
				Title:     reason,
				AvatarURL: avatar,
			},
			Reason: reason,
		}
		sq.Lock()
		sq.statusHistory = append(sq.statusHistory, submitStatus)
		sq.Unlock()
	}
	return stable
}

// This serves little purpose other than to show updates every minute in the
// web UI. Stable() will get called as needed against individual PRs as well.
func (sq *SubmitQueue) updateGoogleE2ELoop() {
	for {
		_ = sq.e2eStable(false)
		time.Sleep(1 * time.Minute)
	}
}

func objToStatusPullRequest(obj *github.MungeObject) *statusPullRequest {
	if obj == nil {
		return &statusPullRequest{}
	}
	res := statusPullRequest{
		Number:    *obj.Issue.Number,
		URL:       *obj.Issue.HTMLURL,
		Title:     *obj.Issue.Title,
		Login:     *obj.Issue.User.Login,
		AvatarURL: *obj.Issue.User.AvatarURL,
	}
	pr, ok := obj.GetPR()
	if !ok {
		return &res
	}
	if pr.Additions != nil {
		res.Additions = *pr.Additions
	}
	if pr.Deletions != nil {
		res.Deletions = *pr.Deletions
	}
	if pr.Base != nil && pr.Base.Ref != nil {
		res.BaseRef = *pr.Base.Ref
	}

	labelPriority := labelPriority(obj)
	if labelPriority <= lastHighPriorityLabel {
		res.ExtraInfo = append(res.ExtraInfo, labelPriorities[labelPriority])
	}

	milestone, ok := obj.Annotations["milestone"]
	if !ok {
		milestone, _ = obj.ReleaseMilestone()
		obj.Annotations["milestone"] = milestone
	}
	if milestone != "" {
		res.ExtraInfo = append(res.ExtraInfo, milestone)
	}

	if labelPriority > lastHighPriorityLabel && labelPriority < len(labelPriorities) {
		res.ExtraInfo = append(res.ExtraInfo, labelPriorities[labelPriority])
	}

	return &res
}

func reasonToState(reason string) string {
	switch reason {
	case merged, mergedByHand, mergedSkippedRetest, mergedBatch:
		return "success"
	case e2eFailure, ghE2EQueued, ghE2EWaitingStart, ghE2ERunning:
		return "success"
	case unknown:
		return "failure"
	default:
		return "pending"
	}
}

// SetMergeStatus will set the status given a particular PR. This function should
// be used instead of manipulating the prStatus directly as sq.Lock() must be
// called when manipulating that structure
// `obj` is the active github object
// `reason` is the new 'status' for this object
func (sq *SubmitQueue) SetMergeStatus(obj *github.MungeObject, reason string) {
	glog.V(4).Infof("SubmitQueue not merging %d because %q", *obj.Issue.Number, reason)
	submitStatus := submitStatus{
		Time:              sq.clock.Now(),
		statusPullRequest: *objToStatusPullRequest(obj),
		Reason:            reason,
	}

	status, ok := obj.GetStatus(sqContext)
	if !ok || status == nil || *status.Description != reason {
		state := reasonToState(reason)
		sq.opts.Lock()
		contextURL := sq.ContextURL
		sq.opts.Unlock()
		url := fmt.Sprintf("%s/#/prs?prDisplay=%d&historyDisplay=%d", contextURL, *obj.Issue.Number, *obj.Issue.Number)
		_ = obj.SetStatus(state, url, reason, sqContext)
	}

	sq.Lock()
	defer sq.Unlock()

	// If we are currently retesting E2E the normal munge loop might find
	// that the ci tests are not green. That's normal and expected and we
	// should just ignore that status update entirely.
	if sq.githubE2ERunning != nil && *sq.githubE2ERunning.Issue.Number == *obj.Issue.Number && strings.HasPrefix(reason, ciFailure) {
		return
	}

	if sq.onQueue(obj) {
		sq.statusHistory = append(sq.statusHistory, submitStatus)
		if len(sq.statusHistory) > 128 {
			sq.statusHistory = sq.statusHistory[1:]
		}
	}
	sq.prStatus[strconv.Itoa(*obj.Issue.Number)] = submitStatus
	sq.cleanupOldE2E(obj, reason)
}

// setContextFailedStatus calls SetMergeStatus after determining a particular github status
// which is failed.
func (sq *SubmitQueue) setContextFailedStatus(obj *github.MungeObject, contexts []string) {
	for i, context := range contexts {
		contextSlice := contexts[i : i+1]
		success, ok := obj.IsStatusSuccess(contextSlice)
		if ok && success {
			continue
		}
		failMsg := fmt.Sprintf(ciFailureFmt, context)
		sq.SetMergeStatus(obj, failMsg)
		return
	}
	glog.Errorf("Inside setContextFailedStatus() but none of the status's failed! %d: %v", obj.Number(), contexts)
	sq.SetMergeStatus(obj, ciFailure)
}

// sq.Lock() MUST be held!
func (sq *SubmitQueue) getE2EQueueStatus() []*statusPullRequest {
	queue := []*statusPullRequest{}
	keys := sq.orderedE2EQueue()
	for _, k := range keys {
		obj := sq.githubE2EQueue[k]
		request := objToStatusPullRequest(obj)
		queue = append(queue, request)
	}
	return queue
}

func (sq *SubmitQueue) marshal(data interface{}) []byte {
	b, err := json.Marshal(data)
	if err != nil {
		glog.Errorf("Unable to Marshal data: %#v: %v", data, err)
		return nil
	}
	return b
}

func (sq *SubmitQueue) getQueueHistory() []byte {
	sq.Lock()
	defer sq.Unlock()
	return sq.marshal(sq.statusHistory)
}

// GetQueueStatus returns a json representation of the state of the submit
// queue. This can be used to generate web pages about the submit queue.
func (sq *SubmitQueue) getQueueStatus() []byte {
	status := submitQueueStatus{PRStatus: map[string]submitStatus{}}
	sq.Lock()
	defer sq.Unlock()

	for key, value := range sq.prStatus {
		status.PRStatus[key] = value
	}
	return sq.marshal(status)
}

func (sq *SubmitQueue) getGithubE2EStatus() []byte {
	sq.Lock()
	defer sq.Unlock()
	status := e2eQueueStatus{
		E2EQueue:    sq.getE2EQueueStatus(),
		E2ERunning:  objToStatusPullRequest(sq.githubE2ERunning),
		BatchStatus: &sq.batchStatus,
	}
	return sq.marshal(status)
}

func noMergeMessage(label string) string {
	return "Will not auto merge because " + label + " is present"
}

func noAdditionalLabelMessage(label string) string {
	return "Will not auto merge because " + label + " is missing"
}

const (
	unknown                  = "unknown failure"
	noCLA                    = "PR is missing CLA label; needs one from the following list:"
	noLGTM                   = "PR does not have " + lgtmLabel + " label."
	noApproved               = "PR does not have " + approvedLabel + " label."
	lgtmEarly                = "The PR was changed after the " + lgtmLabel + " label was added."
	unmergeable              = "PR is unable to be automatically merged. Needs rebase."
	undeterminedMergability  = "Unable to determine is PR is mergeable. Will try again later."
	ciFailure                = "Required Github CI test is not green"
	ciFailureFmt             = ciFailure + ": %s"
	e2eFailure               = "The e2e tests are failing. The entire submit queue is blocked."
	e2eRecover               = "The e2e tests started passing. The submit queue is unblocked."
	merged                   = "MERGED!"
	mergedSkippedRetest      = "MERGED! (skipped retest because of label)"
	mergedBatch              = "MERGED! (batch)"
	mergedByHand             = "MERGED! (by hand outside of submit queue)"
	ghE2EQueued              = "Queued to run github e2e tests a second time."
	ghE2EWaitingStart        = "Requested and waiting for github e2e test to start running a second time."
	ghE2ERunning             = "Running github e2e tests a second time."
	ghE2EFailed              = "Second github e2e run failed."
	unmergeableMilestone     = "Milestone is for a future release and cannot be merged"
	headCommitChanged        = "This PR has changed since we ran the tests"
	ghReviewStateUnclear     = "Cannot get gh reviews status"
	ghReviewApproved         = "This pr has no Github review \"approved\"."
	ghReviewChangesRequested = "Reviewer(s) requested changes through github review process."
)

// validForMergeExt is the base logic about what PR can be automatically merged.
// PRs must pass this logic to be placed on the queue and they must pass this
// logic a second time to be retested/merged after they get to the top of
// the queue.
//
// checkStatus is true if the PR should only merge if the appropriate Github status
// checks are passing.
//
// If you update the logic PLEASE PLEASE PLEASE update serveMergeInfo() as well.
func (sq *SubmitQueue) validForMergeExt(obj *github.MungeObject, checkStatus bool) bool {
	// Can't merge an issue!
	if !obj.IsPR() {
		return false
	}

	// Can't merge something already merged.
	if m, ok := obj.IsMerged(); !ok {
		glog.Errorf("%d: unknown err", *obj.Issue.Number)
		sq.SetMergeStatus(obj, unknown)
		return false
	} else if m {
		sq.SetMergeStatus(obj, mergedByHand)
		return false
	}

	// Lock to get options since we may be running on a goroutine besides the main one.
	sq.opts.Lock()
	gateCLA := sq.GateCLA
	gateApproved := sq.GateApproved
	doNotMergeMilestones := sq.DoNotMergeMilestones
	mergeContexts := mungeopts.RequiredContexts.Merge
	retestContexts := mungeopts.RequiredContexts.Retest
	additionalLabels := sq.AdditionalRequiredLabels
	blockingLabels := sq.BlockingLabels
	claYesLabels := sq.ClaYesLabels
	sq.opts.Unlock()

	milestone := obj.Issue.Milestone
	title := ""
	// Net set means the empty milestone, ""
	if milestone != nil && milestone.Title != nil {
		title = *milestone.Title
	}
	for _, blocked := range doNotMergeMilestones {
		if title == blocked || (title == "" && blocked == "NO-MILESTONE") {
			sq.SetMergeStatus(obj, unmergeableMilestone)
			return false
		}
	}

	// Must pass CLA checks
	if gateCLA {
		for i, l := range claYesLabels {
			if obj.HasLabel(l) {
				break
			}
			if i == len(claYesLabels)-1 {
				sq.SetMergeStatus(obj, fmt.Sprintf("%s %q", noCLA, claYesLabels))
				return false
			}
		}
	}

	// Obviously must be mergeable
	if mergeable, ok := obj.IsMergeable(); !ok {
		sq.SetMergeStatus(obj, undeterminedMergability)
		return false
	} else if !mergeable {
		sq.SetMergeStatus(obj, unmergeable)
		return false
	}

	// Validate the status information for this PR
	if checkStatus {
		if len(mergeContexts) > 0 {
			if success, ok := obj.IsStatusSuccess(mergeContexts); !ok || !success {
				sq.setContextFailedStatus(obj, mergeContexts)
				return false
			}
		}
		if len(retestContexts) > 0 {
			if success, ok := obj.IsStatusSuccess(retestContexts); !ok || !success {
				sq.setContextFailedStatus(obj, retestContexts)
				return false
			}
		}
	}

	if sq.GateGHReviewApproved || sq.GateGHReviewChangesRequested {
		if approvedReview, changesRequestedReview, ok := obj.CollectGHReviewStatus(); !ok {
			sq.SetMergeStatus(obj, ghReviewStateUnclear)
			return false
		} else if len(approvedReview) == 0 && sq.GateGHReviewApproved {
			sq.SetMergeStatus(obj, ghReviewApproved)
			return false
		} else if len(changesRequestedReview) > 0 && sq.GateGHReviewChangesRequested {
			sq.SetMergeStatus(obj, ghReviewChangesRequested)
			return false
		}
	}

	if !obj.HasLabel(lgtmLabel) {
		sq.SetMergeStatus(obj, noLGTM)
		return false
	}

	// PR cannot change since LGTM was added
	if after, ok := obj.ModifiedAfterLabeled(lgtmLabel); !ok {
		sq.SetMergeStatus(obj, unknown)
		return false
	} else if after {
		sq.SetMergeStatus(obj, lgtmEarly)
		return false
	}

	if gateApproved {
		if !obj.HasLabel(approvedLabel) {
			sq.SetMergeStatus(obj, noApproved)
			return false
		}
	}

	// PR cannot have any labels which prevent merging.
	for _, label := range []string{
		cherrypickUnapprovedLabel,
		blockedPathsLabel,
		releaseNoteLabelNeeded,
		doNotMergeLabel,
		wipLabel,
		holdLabel,
	} {
		if obj.HasLabel(label) {
			sq.SetMergeStatus(obj, noMergeMessage(label))
			return false
		}
	}

	for _, label := range additionalLabels {
		if !obj.HasLabel(label) {
			sq.SetMergeStatus(obj, noAdditionalLabelMessage(label))
			return false
		}
	}

	for _, label := range blockingLabels {
		if obj.HasLabel(label) {
			sq.SetMergeStatus(obj, noMergeMessage(label))
			return false
		}
	}

	return true
}

func (sq *SubmitQueue) validForMerge(obj *github.MungeObject) bool {
	return sq.validForMergeExt(obj, true)
}

// Munge is the workhorse the will actually make updates to the PR
func (sq *SubmitQueue) Munge(obj *github.MungeObject) {
	if !sq.validForMerge(obj) {
		return
	}

	added := false
	sq.Lock()
	if _, ok := sq.githubE2EQueue[*obj.Issue.Number]; !ok {
		atomic.AddInt32(&sq.prsAdded, 1)
		added = true
	}
	// Add this most-recent object in place of the existing object. It will
	// have more up2date information. Even though we explicitly refresh the
	// PR information before do anything with it, this allow things like the
	// queue order to change dynamically as labels are added/removed.
	sq.githubE2EQueue[*obj.Issue.Number] = obj
	sq.Unlock()
	if added {
		sq.SetMergeStatus(obj, ghE2EQueued)
	}

	return
}

func (sq *SubmitQueue) deleteQueueItem(obj *github.MungeObject) {
	if sq.onQueue(obj) {
		atomic.AddInt32(&sq.prsRemoved, 1)
	}
	delete(sq.githubE2EQueue, *obj.Issue.Number)
}

// If the PR was put in the github e2e queue previously, but now we don't
// think it should be in the e2e queue, remove it. MUST be called with sq.Lock()
// held.
func (sq *SubmitQueue) cleanupOldE2E(obj *github.MungeObject, reason string) {
	switch {
	case reason == e2eFailure:
	case reason == ghE2EQueued:
	case reason == ghE2EWaitingStart:
	case reason == ghE2ERunning:
		// Do nothing
	case strings.HasPrefix(reason, ciFailure):
		// ciFailure is intersting. If the PR is being actively retested and then the
		// time based loop finds the same PR it will try to set ciFailure. We should in fact
		// not ever call this function in this case, but if we do call here, log it.
		if sq.githubE2ERunning != nil && *sq.githubE2ERunning.Issue.Number == *obj.Issue.Number {
			glog.Errorf("Trying to clean up %d due to ciFailure while it is being tested", *obj.Issue.Number)
			return
		}
		fallthrough
	default:
		if sq.githubE2ERunning != nil && *sq.githubE2ERunning.Issue.Number == *obj.Issue.Number {
			sq.githubE2ERunning = nil
		}
		sq.deleteQueueItem(obj)
	}

}

func labelPriority(obj *github.MungeObject) int {
	for i, label := range labelPriorities {
		if obj.HasLabel(label) {
			return i
		}
	}
	return len(labelPriorities)
}

func compareHighPriorityLabels(a *github.MungeObject, b *github.MungeObject) int {
	aPrio := labelPriority(a)
	bPrio := labelPriority(b)

	if aPrio > lastHighPriorityLabel && bPrio > lastHighPriorityLabel {
		return 0
	}
	return aPrio - bPrio
}

func compareLowPriorityLabels(a *github.MungeObject, b *github.MungeObject) int {
	aPrio := labelPriority(a)
	bPrio := labelPriority(b)

	return aPrio - bPrio
}

type queueSorter struct {
	queue          []*github.MungeObject
	labelTimeCache *mungerutil.LabelTimeCache
}

func (s queueSorter) Len() int      { return len(s.queue) }
func (s queueSorter) Swap(i, j int) { s.queue[i], s.queue[j] = s.queue[j], s.queue[i] }

// If you update the function PLEASE PLEASE PLEASE also update servePriorityInfo()
func (s queueSorter) Less(i, j int) bool {
	a := s.queue[i]
	b := s.queue[j]

	if c := compareHighPriorityLabels(a, b); c < 0 {
		return true
	} else if c > 0 {
		return false
	}

	aDue, _ := a.ReleaseMilestoneDue()
	bDue, _ := b.ReleaseMilestoneDue()

	if aDue.Before(bDue) {
		return true
	} else if aDue.After(bDue) {
		return false
	}

	if c := compareLowPriorityLabels(a, b); c < 0 {
		return true
	} else if c > 0 {
		return false
	}

	aTime, aOK := s.labelTimeCache.FirstLabelTime(a)
	bTime, bOK := s.labelTimeCache.FirstLabelTime(b)

	// Shouldn't really happen since these have been LGTMed to be
	// in the queue at all. But just in case, .
	if !aOK && bOK {
		return false
	} else if aOK && !bOK {
		return true
	} else if !aOK && !bOK {
		return false
	}

	return aTime.Before(bTime)
}

// onQueue just tells if a PR is already on the queue.
// sq.Lock() must be held
func (sq *SubmitQueue) onQueue(obj *github.MungeObject) bool {
	for _, queueObj := range sq.githubE2EQueue {
		if *queueObj.Issue.Number == *obj.Issue.Number {
			return true
		}

	}
	return false
}

// sq.Lock() better held!!!
func (sq *SubmitQueue) orderedE2EQueue() []int {
	prs := []*github.MungeObject{}
	for _, obj := range sq.githubE2EQueue {
		prs = append(prs, obj)
	}
	sort.Sort(queueSorter{prs, sq.lgtmTimeCache})

	var ordered []int
	for _, obj := range prs {
		ordered = append(ordered, *obj.Issue.Number)
	}
	return ordered
}

// handleGithubE2EAndMerge waits for PRs that are ready to re-run the github
// e2e tests, runs the test, and then merges if everything was successful.
func (sq *SubmitQueue) handleGithubE2EAndMerge() {
	for {
		sq.Lock()
		l := len(sq.githubE2EQueue)
		sq.Unlock()
		// Wait until something is ready to be processed
		if l == 0 {
			time.Sleep(sq.githubE2EPollTime)
			continue
		}

		obj := sq.selectPullRequest()
		if obj == nil {
			continue
		}

		// only critical fixes can be merged if postsubmits are failing
		if !sq.e2eStable(false) && !obj.HasLabel(criticalFixLabel) {
			time.Sleep(sq.githubE2EPollTime)
			continue
		}

		// re-test and maybe merge
		remove := sq.doGithubE2EAndMerge(obj)
		if remove {
			// remove it from the map after we finish testing
			sq.Lock()
			if sq.githubE2ERunning != nil {
				sq.githubE2ELastPRNum = *sq.githubE2ERunning.Issue.Number
			}
			sq.githubE2ERunning = nil
			sq.deleteQueueItem(obj)
			sq.Unlock()
		}
	}
}

func (sq *SubmitQueue) mergePullRequest(obj *github.MungeObject, msg, extra string) bool {
	isMaster, _ := obj.IsForBranch("master")
	if isMaster {
		sq.opts.Lock()
		if sq.MergeToMasterMessage != "" {
			extra = extra + ". " + sq.MergeToMasterMessage
		}
		sq.opts.Unlock()
	}
	ok := obj.MergePR("submit-queue" + extra)
	if !ok {
		return ok
	}
	sq.SetMergeStatus(obj, msg)
	sq.updateMergeRate()
	return true
}

func (sq *SubmitQueue) selectPullRequest() *github.MungeObject {
	if sq.interruptedObj != nil {
		return sq.interruptedObj.obj
	}
	sq.Lock()
	defer sq.Unlock()
	if len(sq.githubE2EQueue) == 0 {
		return nil
	}
	keys := sq.orderedE2EQueue()
	obj := sq.githubE2EQueue[keys[0]]
	if sq.githubE2ERunning != nil {
		sq.githubE2ELastPRNum = *sq.githubE2ERunning.Issue.Number
	}
	sq.githubE2ERunning = obj

	return obj
}

func (interruptedObj *submitQueueInterruptedObject) hasSHAChanged() bool {
	headSHA, baseRef, gotHeadSHA := interruptedObj.obj.GetHeadAndBase()
	if !gotHeadSHA {
		return true
	}

	baseSHA, gotBaseSHA := interruptedObj.obj.GetSHAFromRef(baseRef)
	if !gotBaseSHA {
		return true
	}

	return interruptedObj.interruptedMergeBaseSHA != baseSHA ||
		interruptedObj.interruptedMergeHeadSHA != headSHA
}

func newInterruptedObject(obj *github.MungeObject) *submitQueueInterruptedObject {
	if headSHA, baseRef, gotHeadSHA := obj.GetHeadAndBase(); !gotHeadSHA {
		return nil
	} else if baseSHA, gotBaseSHA := obj.GetSHAFromRef(baseRef); !gotBaseSHA {
		return nil
	} else {
		return &submitQueueInterruptedObject{obj, headSHA, baseSHA}
	}
}

// Returns true if we can discard the PR from the queue, false if we must keep it for later.
// If you modify this, consider modifying doBatchMerge too.
func (sq *SubmitQueue) doGithubE2EAndMerge(obj *github.MungeObject) bool {
	interruptedObj := sq.interruptedObj
	sq.interruptedObj = nil

	ok := obj.Refresh()
	if !ok {
		glog.Errorf("%d: unknown err", *obj.Issue.Number)
		sq.SetMergeStatus(obj, unknown)
		return true
	}

	if !sq.validForMerge(obj) {
		return true
	}

	if obj.HasLabel(retestNotRequiredLabel) || obj.HasLabel(retestNotRequiredDocsOnlyLabel) {
		atomic.AddInt32(&sq.instantMerges, 1)
		sq.mergePullRequest(obj, mergedSkippedRetest, "")
		return true
	}

	sha, _, ok := obj.GetHeadAndBase()
	if !ok {
		glog.Errorf("%d: Unable to get SHA", *obj.Issue.Number)
		sq.SetMergeStatus(obj, unknown)
		return true
	}
	if interruptedObj != nil {
		if interruptedObj.hasSHAChanged() {
			// This PR will have to be rested.
			// Make sure we don't have higher priority first.
			return false
		}
		glog.Infof("Skipping retest since head and base sha match previous attempt!")
		atomic.AddInt32(&sq.retestsAvoided, 1)
	} else {
		if sq.retestPR(obj) {
			return true
		}

		ok := obj.Refresh()
		if !ok {
			sq.SetMergeStatus(obj, unknown)
			return true
		}
	}

	sq.mergeLock.Lock()
	defer sq.mergeLock.Unlock()

	// We shouldn't merge if it's not valid anymore
	if !sq.validForMerge(obj) {
		glog.Errorf("%d: Not mergeable anymore. Do not merge.", *obj.Issue.Number)
		return true
	}

	if newSha, _, ok := obj.GetHeadAndBase(); !ok {
		glog.Errorf("%d: Unable to get SHA", *obj.Issue.Number)
		sq.SetMergeStatus(obj, unknown)
		return true
	} else if newSha != sha {
		glog.Errorf("%d: Changed while running the test. Do not merge.", *obj.Issue.Number)
		sq.SetMergeStatus(obj, headCommitChanged)
		return false
	}

	if !sq.e2eStable(true) && !obj.HasLabel(criticalFixLabel) {
		if sq.validForMerge(obj) {
			sq.interruptedObj = newInterruptedObject(obj)
		}
		sq.SetMergeStatus(obj, e2eFailure)
		return true
	}

	sq.mergePullRequest(obj, merged, "")
	return true
}

// Returns true if merge status changes, and false otherwise.
func (sq *SubmitQueue) retestPR(obj *github.MungeObject) bool {
	sq.opts.Lock()
	retestContexts := mungeopts.RequiredContexts.Retest
	sq.opts.Unlock()

	if len(retestContexts) == 0 {
		return false
	}

	if err := obj.WriteComment(newRetestBody); err != nil {
		glog.Errorf("%d: unknown err: %v", *obj.Issue.Number, err)
		sq.SetMergeStatus(obj, unknown)
		return true
	}

	// Wait for the retest to start
	sq.SetMergeStatus(obj, ghE2EWaitingStart)
	atomic.AddInt32(&sq.prsTested, 1)
	sq.opts.Lock()
	prMaxWaitTime := mungeopts.PRMaxWaitTime
	sq.opts.Unlock()
	done := obj.WaitForPending(retestContexts, prMaxWaitTime)
	if !done {
		sq.SetMergeStatus(obj, fmt.Sprintf("Timed out waiting for PR %d to start testing", obj.Number()))
		return true
	}

	// Wait for the status to go back to something other than pending
	sq.SetMergeStatus(obj, ghE2ERunning)
	done = obj.WaitForNotPending(retestContexts, prMaxWaitTime)
	if !done {
		sq.SetMergeStatus(obj, fmt.Sprintf("Timed out waiting for PR %d to finish testing", obj.Number()))
		return true
	}

	// Check if the thing we care about is success
	if success, ok := obj.IsStatusSuccess(retestContexts); !success || !ok {
		sq.SetMergeStatus(obj, ghE2EFailed)
		return true
	}

	// no action taken.
	return false
}

func (sq *SubmitQueue) serve(data []byte, res http.ResponseWriter, req *http.Request) {
	if data == nil {
		res.Header().Set("Content-type", "text/plain")
		res.WriteHeader(http.StatusInternalServerError)
	} else {
		res.Header().Set("Content-type", "application/json")
		res.WriteHeader(http.StatusOK)
		res.Write(data)
	}
}

func (sq *SubmitQueue) serveHistory(res http.ResponseWriter, req *http.Request) {
	data := sq.getQueueHistory()
	sq.serve(data, res, req)
}

func (sq *SubmitQueue) servePRs(res http.ResponseWriter, req *http.Request) {
	data := sq.getQueueStatus()
	sq.serve(data, res, req)
}

func (sq *SubmitQueue) serveGithubE2EStatus(res http.ResponseWriter, req *http.Request) {
	data := sq.getGithubE2EStatus()
	sq.serve(data, res, req)
}

func (sq *SubmitQueue) serveCIStatus(res http.ResponseWriter, req *http.Request) {
	sq.Lock()
	data := sq.marshal(sq.ciStatus)
	sq.Unlock()
	sq.serve(data, res, req)
}

func (sq *SubmitQueue) serveHealth(res http.ResponseWriter, req *http.Request) {
	sq.Lock()
	data := sq.marshal(sq.health)
	sq.Unlock()
	sq.serve(data, res, req)
}

func (sq *SubmitQueue) serveSQStats(res http.ResponseWriter, req *http.Request) {
	data := submitQueueStats{
		Added:              int(atomic.LoadInt32(&sq.prsAdded)),
		FlakesIgnored:      int(atomic.LoadInt32(&sq.flakesIgnored)),
		Initialized:        atomic.LoadInt32(&sq.loopStarts) > 1,
		InstantMerges:      int(atomic.LoadInt32(&sq.instantMerges)),
		BatchMerges:        int(atomic.LoadInt32(&sq.batchMerges)),
		LastMergeTime:      sq.lastMergeTime,
		MergeRate:          sq.calcMergeRateWithTail(),
		MergesSinceRestart: int(atomic.LoadInt32(&sq.totalMerges)),
		Removed:            int(atomic.LoadInt32(&sq.prsRemoved)),
		RetestsAvoided:     int(atomic.LoadInt32(&sq.retestsAvoided)),
		StartTime:          sq.startTime,
		Tested:             int(atomic.LoadInt32(&sq.prsTested)),
	}
	sq.serve(sq.marshal(data), res, req)
}

func (sq *SubmitQueue) serveFlakes(res http.ResponseWriter, req *http.Request) {
	data := sq.e2e.Flakes()
	sq.serve(mungerutil.PrettyMarshal(data), res, req)
}

func (sq *SubmitQueue) serveMetadata(res http.ResponseWriter, req *http.Request) {
	sq.Lock()
	data := sq.marshal(sq.Metadata)
	sq.Unlock()
	sq.serve(data, res, req)
}

func (sq *SubmitQueue) serveBatch(res http.ResponseWriter, req *http.Request) {
	sq.serve(sq.marshal(sq.batchStatus), res, req)
}

func (sq *SubmitQueue) serveMergeInfo(res http.ResponseWriter, req *http.Request) {
	// Lock to get options since we are not running in the main goroutine.
	sq.opts.Lock()
	doNotMergeMilestones := sq.DoNotMergeMilestones
	additionalLabels := sq.AdditionalRequiredLabels
	blockingLabels := sq.BlockingLabels
	gateApproved := sq.GateApproved
	gateCLA := sq.GateCLA
	mergeContexts := mungeopts.RequiredContexts.Merge
	retestContexts := mungeopts.RequiredContexts.Retest
	claYesLabels := sq.ClaYesLabels
	sq.opts.Unlock()

	res.Header().Set("Content-type", "text/plain")
	res.WriteHeader(http.StatusOK)
	var out bytes.Buffer
	out.WriteString("PRs must meet the following set of conditions to be considered for automatic merging by the submit queue.")
	out.WriteString("<ol>")
	if gateCLA {
		out.WriteString(fmt.Sprintf("<li>The PR must have one of the following labels: %q </li>", claYesLabels))
	}
	out.WriteString("<li>The PR must be mergeable. aka cannot need a rebase</li>")
	if len(mergeContexts) > 0 || len(retestContexts) > 0 {
		out.WriteString("<li>All of the following github statuses must be green")
		out.WriteString("<ul>")
		for _, context := range mergeContexts {
			out.WriteString(fmt.Sprintf("<li>%s</li>", context))
		}
		for _, context := range retestContexts {
			out.WriteString(fmt.Sprintf("<li>%s</li>", context))
		}
		out.WriteString("</ul>")
	}
	out.WriteString(fmt.Sprintf("<li>The PR cannot have any of the following milestones: %q</li>", doNotMergeMilestones))
	out.WriteString(fmt.Sprintf(`<li>The PR must have the %q label</li>`, lgtmLabel))
	out.WriteString(fmt.Sprintf("<li>The PR must not have been updated since the %q label was applied</li>", lgtmLabel))
	if gateApproved {
		out.WriteString(fmt.Sprintf(`<li>The PR must have the %q label</li>`, approvedLabel))
	}
	if len(additionalLabels) > 0 {
		out.WriteString(fmt.Sprintf(`<li>The PR must have the following labels: %q</li>`, additionalLabels))
	}
	if len(blockingLabels) > 0 {
		out.WriteString(fmt.Sprintf(`<li>The PR must not have the following labels: %q</li>`, blockingLabels))
	}
	out.WriteString(`<li>The PR must not have the any labels starting with "do-not-merge"</li>`)
	out.WriteString(`</ol><br>`)
	out.WriteString("The PR can then be queued to re-test before merge. Once it reaches the top of the queue all of the above conditions must be true but so must the following:")
	out.WriteString("<ol>")
	if len(retestContexts) > 0 {
		out.WriteString("<li>All of the following tests must pass a second time")
		out.WriteString("<ul>")
		for _, context := range retestContexts {
			out.WriteString(fmt.Sprintf("<li>%s</li>", context))
		}
		out.WriteString("</ul>")
		out.WriteString(fmt.Sprintf("Unless the %q or %q label is present</li>", retestNotRequiredLabel, retestNotRequiredDocsOnlyLabel))
	}
	out.WriteString("</ol>")
	out.WriteString("And then the PR will be merged!!")
	res.Write(out.Bytes())
}

func writeLabel(label string, res http.ResponseWriter) {
	out := fmt.Sprintf(`  <li>%q label
    <ul>
      <li>A PR with %q will come next</li>
    </ul>
  </li>
`, label, label)
	res.Write([]byte(out))
}

func (sq *SubmitQueue) servePriorityInfo(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-type", "text/plain")
	res.WriteHeader(http.StatusOK)
	res.Write([]byte(`The merge queue is sorted by the following. If there is a tie in any test the next test will be used.
<ol>
  <li>'` + criticalFixLabel + `' label
    <ul>
      <li>A PR with '` + criticalFixLabel + `' will come first</li>
      <li>A PR with '` + criticalFixLabel + `' will merge even if the e2e tests are blocked</li>
    </ul>
  </li>
`))
	for i := 1; i <= lastHighPriorityLabel; i++ {
		writeLabel(labelPriorities[i], res)
	}
	res.Write([]byte(`  <li>Release milestone due date
    <ul>
      <li>Release milestones are of the form vX.Y where X and Y are integers</li>
      <li>The release milestore must have a due date set to affect queue order</li>
      <li>Other milestones are ignored</li>
    </ul>
  </li>
`))
	for i := lastHighPriorityLabel + 1; i < len(labelPriorities); i++ {
		writeLabel(labelPriorities[i], res)
	}
	res.Write([]byte(`  <li>First time at which the LGTM label was applied.
    <ul>
      <li>This means all PRs start at the bottom of the queue (within their priority and milestone bands, of course) and progress towards the top.</li>
    </ul>
  </li>
</ol> `))
}

func (sq *SubmitQueue) getHealthSVG() []byte {
	sq.Lock()
	defer sq.Unlock()
	blocked := false
	blockingJobs := make([]string, 0)
	blocked = !sq.health.MergePossibleNow
	status := "running"
	color := "brightgreen"
	if blocked {
		status = "blocked"
		color = "red"
		for job, status := range sq.e2e.GetBuildStatus() {
			if status.Status == "Not Stable" {
				job = strings.Replace(job, "kubernetes-", "", -1)
				blockingJobs = append(blockingJobs, job)
			}
		}
		sort.Strings(blockingJobs)
		if len(blockingJobs) > 3 {
			blockingJobs = append(blockingJobs[:3], "...")
		}
		if len(blockingJobs) > 0 {
			status += " by " + strings.Join(blockingJobs, ", ")
		}
	}
	return shield.Make("queue", status, color)
}

func (sq *SubmitQueue) serveHealthSVG(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-type", "image/svg+xml")
	res.Header().Set("Cache-Control", "max-age=60")
	res.WriteHeader(http.StatusOK)
	res.Write(sq.getHealthSVG())
}

func (sq *SubmitQueue) isStaleIssueComment(obj *github.MungeObject, comment *githubapi.IssueComment) bool {
	if !obj.IsRobot(comment.User) {
		return false
	}
	if *comment.Body != newRetestBody {
		return false
	}
	stale := commentBeforeLastCI(obj, comment, mungeopts.RequiredContexts.Retest)
	if stale {
		glog.V(6).Infof("Found stale SubmitQueue safe to merge comment")
	}
	return stale
}

// StaleIssueComments returns a slice of stale issue comments.
func (sq *SubmitQueue) StaleIssueComments(obj *github.MungeObject, comments []*githubapi.IssueComment) []*githubapi.IssueComment {
	return forEachCommentTest(obj, comments, sq.isStaleIssueComment)
}
