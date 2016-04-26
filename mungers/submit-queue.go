/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"k8s.io/kubernetes/pkg/util"
	"k8s.io/kubernetes/pkg/util/sets"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"
	"k8s.io/contrib/mungegithub/mungers/e2e"
	fake_e2e "k8s.io/contrib/mungegithub/mungers/e2e/fake"
	"k8s.io/contrib/test-utils/utils"

	"github.com/NYTimes/gziphandler"
	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

const (
	lgtmLabel           = "lgtm"
	okToMergeLabel      = "ok-to-merge"
	needsOKToMergeLabel = "needs-ok-to-merge"
	e2eNotRequiredLabel = "e2e-not-required"
	doNotMergeLabel     = "do-not-merge"
	claYesLabel         = "cla: yes"
	claHumanLabel       = "cla: human-approved"

	jenkinsE2EContext  = "Jenkins GCE e2e"
	jenkinsUnitContext = "Jenkins unit/integration"
	travisContext      = "continuous-integration/travis-ci/pr"
	sqContext          = "Submit Queue"

	e2eNotRequiredMergePriority = -1 // used for e2eNotRequiredLabel
	defaultMergePriority        = 3  // when an issue is unlabeled

	githubE2EPollTime = 30 * time.Second

	notInWhitelistBody = "The author of this PR is not in the whitelist for merge, can one of the admins add the '" + okToMergeLabel + "' label?"
)

var (
	_                     = fmt.Print
	verifySafeToMergeBody = fmt.Sprintf("@%s test this [submit-queue is verifying that this PR is safe to merge]", jenkinsBotName)
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
}

type userInfo struct {
	Login     string
	AvatarURL string
	Access    string
}

type e2eQueueStatus struct {
	E2ERunning *statusPullRequest
	E2EQueue   []*statusPullRequest
}

type submitQueueStatus struct {
	PRStatus map[string]submitStatus
}

// Information about the e2e test health. Call updateHealth on the SubmitQueue
// at roughly constant intervals to keep this up to date. The mergeable fraction
// of time for the queue as a whole and the individual jobs will then be
// NumStable[PerJob] / TotalLoops.
type submitQueueHealth struct {
	StartTime       time.Time
	TotalLoops      int
	NumStable       int
	NumStablePerJob map[string]int
}

// information about the sq itself including how fast things are merging and
// how long since the last merge
type submitQueueStats struct {
	LastMergeTime time.Time
	MergeRate     float64
}

// SubmitQueue will merge PR which meet a set of requirements.
//  PR must have LGTM after the last commit
//  PR must have passed all github CI checks
//  if user not in whitelist PR must have okToMergeLabel"
//  The google internal jenkins instance must be passing the JobNames e2e tests
type SubmitQueue struct {
	githubConfig       *github.Config
	JobNames           []string
	WeakStableJobNames []string

	// If FakeE2E is true, don't try to connect to JenkinsHost, all jobs are passing.
	FakeE2E     bool
	JenkinsHost string

	Whitelist              string
	Committers             string
	E2EStatusContext       string
	UnitStatusContext      string
	RequiredStatusContexts []string

	// additionalUserWhitelist are non-committer users believed safe
	additionalUserWhitelist *sets.String
	// CommitterList are static here in case they can't be gotten dynamically;
	// they do not need to be whitelisted.
	committerList *sets.String

	// userWhitelist is the combination of committers and additional which
	// we actully use
	userWhitelist *sets.String

	sync.Mutex
	lastPRStatus  map[string]submitStatus
	prStatus      map[string]submitStatus // protected by sync.Mutex
	userInfo      map[string]userInfo     //proteted by sync.Mutex
	statusHistory []submitStatus          // protected by sync.Mutex

	clock         util.Clock
	lastMergeTime time.Time
	mergeRate     float64 // per 24 hours

	githubE2ERunning  *github.MungeObject         // protect by sync.Mutex!
	githubE2EQueue    map[int]*github.MungeObject // protected by sync.Mutex!
	githubE2EPollTime time.Duration

	lastE2EStable bool // was e2e stable last time they were checked, protect by sync.Mutex
	e2e           e2e.E2ETester
	health        submitQueueHealth
}

func init() {
	clock := util.RealClock{}
	sq := &SubmitQueue{
		clock:          clock,
		lastMergeTime:  clock.Now(),
		lastE2EStable:  true,
		prStatus:       map[string]submitStatus{},
		lastPRStatus:   map[string]submitStatus{},
		githubE2EQueue: map[int]*github.MungeObject{},
	}
	RegisterMungerOrDie(sq)
	RegisterStaleComments(sq)
}

// Name is the name usable in --pr-mungers
func (sq SubmitQueue) Name() string { return "submit-queue" }

// RequiredFeatures is a slice of 'features' that must be provided
func (sq SubmitQueue) RequiredFeatures() []string { return []string{} }

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
// ours isn't time series data so I vary the smoothing factor based on how long
// its been since the last entry. See the comments on the `getSmoothFactor` for
// a discussion of why.
//    This whole thing was dreamed up by eparis one weekend via a combination
//    of guess-and-test and intuition. Someone who knows about this stuff
//    is likely to laugh at the naivete. Point him to where someone intelligent
//    has thought about this stuff and he will gladly do something smart.
func calcMergeRate(oldRate float64, last, now time.Time) float64 {
	since := now.Sub(last)
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

// updates a smoothed rate at which PRs are merging per day.
// returns 'Now()' and the rate.
func (sq *SubmitQueue) updateMergeRate() {
	now := sq.clock.Now()

	sq.mergeRate = calcMergeRate(sq.mergeRate, sq.lastMergeTime, now)
	sq.lastMergeTime = now
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
	return sq.internalInitialize(config, features, utils.GoogleBucketURL)
}

// internalInitialize will initialize the munger for the given GCS bucket url.
func (sq *SubmitQueue) internalInitialize(config *github.Config, features *features.Features, GCSBucketUrl string) error {
	sq.Lock()
	defer sq.Unlock()

	sq.githubConfig = config
	if len(sq.JenkinsHost) == 0 {
		glog.Fatalf("--jenkins-host is required.")
	}

	if sq.FakeE2E {
		sq.e2e = &fake_e2e.FakeE2ETester{
			JobNames:           sq.JobNames,
			WeakStableJobNames: sq.WeakStableJobNames,
		}
	} else {
		sq.e2e = &e2e.RealE2ETester{
			JobNames:             sq.JobNames,
			JenkinsHost:          sq.JenkinsHost,
			WeakStableJobNames:   sq.WeakStableJobNames,
			BuildStatus:          map[string]e2e.BuildInfo{},
			GoogleGCSBucketUtils: utils.NewUtils(GCSBucketUrl),
		}
	}

	if len(config.Address) > 0 {
		if len(config.WWWRoot) > 0 {
			http.Handle("/", gziphandler.GzipHandler(http.FileServer(http.Dir(config.WWWRoot))))
		}
		http.Handle("/prs", gziphandler.GzipHandler(http.HandlerFunc(sq.servePRs)))
		http.Handle("/history", gziphandler.GzipHandler(http.HandlerFunc(sq.serveHistory)))
		http.Handle("/users", gziphandler.GzipHandler(http.HandlerFunc(sq.serveUsers)))
		http.Handle("/github-e2e-queue", gziphandler.GzipHandler(http.HandlerFunc(sq.serveGithubE2EStatus)))
		http.Handle("/google-internal-ci", gziphandler.GzipHandler(http.HandlerFunc(sq.serveGoogleInternalStatus)))
		http.Handle("/merge-info", gziphandler.GzipHandler(http.HandlerFunc(sq.serveMergeInfo)))
		http.Handle("/priority-info", gziphandler.GzipHandler(http.HandlerFunc(sq.servePriorityInfo)))
		http.Handle("/health", gziphandler.GzipHandler(http.HandlerFunc(sq.serveHealth)))
		http.Handle("/sq-stats", gziphandler.GzipHandler(http.HandlerFunc(sq.serveSQStats)))
		config.ServeDebugStats("/stats")
		go http.ListenAndServe(config.Address, nil)
	}

	if sq.githubE2EPollTime == 0 {
		sq.githubE2EPollTime = githubE2EPollTime
	}

	sq.health.StartTime = sq.clock.Now()
	sq.health.NumStablePerJob = map[string]int{}

	go sq.handleGithubE2EAndMerge()
	go sq.updateGoogleE2ELoop()
	return nil
}

// EachLoop is called at the start of every munge loop
func (sq *SubmitQueue) EachLoop() error {
	sq.Lock()
	sq.updateHealth()
	sq.RefreshWhitelist()
	sq.lastPRStatus = sq.prStatus
	sq.prStatus = map[string]submitStatus{}

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
	return nil
}

// AddFlags will add any request flags to the cobra `cmd`
func (sq *SubmitQueue) AddFlags(cmd *cobra.Command, config *github.Config) {
	cmd.Flags().StringSliceVar(&sq.JobNames, "jenkins-jobs", []string{
		"kubelet-gce-e2e-ci",
		"kubernetes-build",
		"kubernetes-test-go",
		"kubernetes-e2e-gce",
		"kubernetes-e2e-gce-slow",
		"kubernetes-e2e-gke",
		"kubernetes-e2e-gke-slow",
		"kubernetes-e2e-gce-scalability",
		"kubernetes-kubemark-5-gce",
	}, "Comma separated list of jobs in Jenkins to use for stability testing")
	cmd.Flags().StringSliceVar(&sq.WeakStableJobNames, "weak-stable-jobs",
		[]string{"kubernetes-kubemark-500-gce"},
		"Comma separated list of jobs in Jenkins to use for stability testing that needs only weak success")
	cmd.Flags().StringVar(&sq.JenkinsHost, "jenkins-host", "http://jenkins-master:8080", "The URL for the jenkins job to watch")
	cmd.Flags().StringSliceVar(&sq.RequiredStatusContexts, "required-contexts", []string{}, "Comma separate list of status contexts required for a PR to be considered ok to merge")
	cmd.Flags().StringVar(&sq.E2EStatusContext, "e2e-status-context", jenkinsE2EContext, "The name of the github status context for the e2e PR Builder")
	cmd.Flags().StringVar(&sq.UnitStatusContext, "unit-status-context", jenkinsUnitContext, "The name of the github status context for the unit PR Builder")
	cmd.Flags().BoolVar(&sq.FakeE2E, "fake-e2e", false, "Whether to use a fake for testing E2E stability.")
	sq.addWhitelistCommand(cmd, config)
}

// Hold the lock
func (sq *SubmitQueue) updateHealth() {
	sq.health.TotalLoops++
	if sq.e2e.Stable() {
		sq.health.NumStable++
	}
	for job, status := range sq.e2e.GetBuildStatus() {
		if _, ok := sq.health.NumStablePerJob[job]; !ok {
			sq.health.NumStablePerJob[job] = 0
		}
		if status.Status == "Stable" {
			sq.health.NumStablePerJob[job]++
		}
	}
}

func (sq *SubmitQueue) e2eStable() bool {
	wentStable := false
	wentUnstable := false

	stable := sq.e2e.GCSBasedStable()
	jenkinsStable := sq.e2e.Stable()

	if stable != jenkinsStable {
		glog.Errorf("GCS stable check returned different value than Jenkins: %v vs %v.", stable, jenkinsStable)
	}

	weakStable := sq.e2e.GCSWeakStable()
	if !weakStable {
		glog.Errorf("E2E is not stable because weak stable check failed.")
	}

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
		_ = sq.e2eStable()
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
	pr, err := obj.GetPR()
	if err != nil {
		return &res
	}
	if pr.Additions != nil {
		res.Additions = *pr.Additions
	}
	if pr.Deletions != nil {
		res.Deletions = *pr.Deletions
	}

	prio, ok := obj.Annotations["priority"]
	if !ok {
		var prio string
		p := priority(obj)
		if p == e2eNotRequiredMergePriority {
			prio = e2eNotRequiredLabel
		} else {
			prio = fmt.Sprintf("P%d", p) // store it a P1, P2, P3.  Not just 1,2,3
		}
		obj.Annotations["priority"] = prio
	}
	if prio != "" {
		res.ExtraInfo = append(res.ExtraInfo, prio)
	}

	milestone, ok := obj.Annotations["milestone"]
	if !ok {
		milestone = obj.ReleaseMilestone()
		obj.Annotations["milestone"] = milestone
	}
	if milestone != "" {
		res.ExtraInfo = append(res.ExtraInfo, milestone)
	}
	return &res
}

func reasonToState(reason string) string {
	switch reason {
	case merged:
		return "success"
	case e2eFailure:
		return "success"
	case ghE2EQueued:
		return "success"
	case ghE2EWaitingStart:
		return "success"
	case ghE2ERunning:
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

	status := obj.GetStatus(sqContext)
	if status == nil || *status.Description != reason {
		state := reasonToState(reason)
		url := fmt.Sprintf("http://submit-queue.k8s.io/#?prDisplay=%d&historyDisplay=%d", *obj.Issue.Number, *obj.Issue.Number)
		_ = obj.SetStatus(state, url, reason, sqContext)
	}

	sq.Lock()
	defer sq.Unlock()

	// If we are currently retesting E2E the normal munge loop might find
	// that the ci tests are not green. That's normal and expected and we
	// should just ignore that status update entirely.
	if sq.githubE2ERunning != nil && *sq.githubE2ERunning.Issue.Number == *obj.Issue.Number && reason == ciFailure {
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
		glog.Errorf("Unable to Marshal Status: %v: %v", sq.statusHistory, err)
		return nil
	}
	return b
}

func (sq *SubmitQueue) getUserInfo() []byte {
	sq.Lock()
	defer sq.Unlock()
	return sq.marshal(sq.userInfo)
}

func (sq *SubmitQueue) getQueueHistory() []byte {
	sq.Lock()
	defer sq.Unlock()
	return sq.marshal(sq.statusHistory)
}

// GetQueueStatus returns a json representation of the state of the submit
// queue. This can be used to generate web pages about the submit queue.
func (sq *SubmitQueue) getQueueStatus() []byte {
	status := submitQueueStatus{}
	sq.Lock()
	defer sq.Unlock()
	outputStatus := sq.lastPRStatus
	for key, value := range sq.prStatus {
		outputStatus[key] = value
	}
	status.PRStatus = outputStatus

	return sq.marshal(status)
}

func (sq *SubmitQueue) getGithubE2EStatus() []byte {
	sq.Lock()
	defer sq.Unlock()
	status := e2eQueueStatus{
		E2EQueue:   sq.getE2EQueueStatus(),
		E2ERunning: objToStatusPullRequest(sq.githubE2ERunning),
	}
	return sq.marshal(status)
}

func (sq *SubmitQueue) getGoogleInternalStatus() []byte {
	sq.Lock()
	defer sq.Unlock()
	return sq.marshal(sq.e2e.GetBuildStatus())
}

func (sq *SubmitQueue) getHealth() []byte {
	sq.Lock()
	defer sq.Unlock()
	return sq.marshal(sq.health)
}

const (
	unknown                 = "unknown failure"
	noCLA                   = "PR does not have " + claYesLabel + " or " + claHumanLabel
	noLGTM                  = "PR does not have LGTM."
	needsok                 = "PR does not have '" + okToMergeLabel + "' label"
	lgtmEarly               = "The PR was changed after the LGTM label was added."
	unmergeable             = "PR is unable to be automatically merged. Needs rebase."
	undeterminedMergability = "Unable to determine is PR is mergeable. Will try again later."
	noMerge                 = "Will not auto merge because " + doNotMergeLabel + " is present"
	ciFailure               = "Github CI tests are not green."
	e2eFailure              = "The e2e tests are failing. The entire submit queue is blocked."
	e2eRecover              = "The e2e tests started passing. The submit queue is unblocked."
	merged                  = "MERGED!"
	ghE2EQueued             = "Queued to run github e2e tests a second time."
	ghE2EWaitingStart       = "Requested and waiting for github e2e test to start running a second time."
	ghE2ERunning            = "Running github e2e tests a second time."
	ghE2EFailed             = "Second github e2e run failed."
)

func (sq *SubmitQueue) requiredStatusContexts(obj *github.MungeObject) []string {
	contexts := sq.RequiredStatusContexts
	if len(sq.E2EStatusContext) > 0 && !obj.HasLabel(e2eNotRequiredLabel) {
		contexts = append(contexts, sq.E2EStatusContext)
	}
	if len(sq.UnitStatusContext) > 0 {
		contexts = append(contexts, sq.UnitStatusContext)
	}
	return contexts
}

// validForMerge is the base logic about what PR can be automatically merged.
// PRs must pass this logic to be placed on the queue and they must pass this
// logic a second time to be retested/merged after they get to the top of
// the queue.
//
// If you update the logic PLEASE PLEASE PLEASE update serveMergeInfo() as well.
func (sq *SubmitQueue) validForMerge(obj *github.MungeObject) bool {
	// Can't merge an issue!
	if !obj.IsPR() {
		return false
	}

	// Can't merge something already merged.
	if m, err := obj.IsMerged(); err != nil {
		glog.Errorf("%d: unknown err: %v", *obj.Issue.Number, err)
		sq.SetMergeStatus(obj, unknown)
		return false
	} else if m {
		sq.SetMergeStatus(obj, merged)
		return false
	}

	userSet := sq.userWhitelist

	// Must pass CLA checks
	if !obj.HasLabel(claYesLabel) && !obj.HasLabel(claHumanLabel) {
		sq.SetMergeStatus(obj, noCLA)
		return false
	}

	// Obviously must be mergeable
	if mergeable, err := obj.IsMergeable(); err != nil {
		sq.SetMergeStatus(obj, undeterminedMergability)
		return false
	} else if !mergeable {
		sq.SetMergeStatus(obj, unmergeable)
		return false
	}

	// Validate the status information for this PR
	contexts := sq.requiredStatusContexts(obj)
	if ok := obj.IsStatusSuccess(contexts); !ok {
		sq.SetMergeStatus(obj, ciFailure)
		return false
	}

	// The user either must be on the whitelist or have ok-to-merge
	if !obj.HasLabel(okToMergeLabel) && !userSet.Has(*obj.Issue.User.Login) {
		if !obj.HasLabel(needsOKToMergeLabel) {
			obj.AddLabels([]string{needsOKToMergeLabel})
			obj.WriteComment(notInWhitelistBody)
		}
		sq.SetMergeStatus(obj, needsok)
		return false
	}

	// Tidy up the issue list.
	if obj.HasLabel(needsOKToMergeLabel) {
		obj.RemoveLabel(needsOKToMergeLabel)
	}

	// Clearly
	if !obj.HasLabel(lgtmLabel) {
		sq.SetMergeStatus(obj, noLGTM)
		return false
	}

	// PR cannot change since LGTM was added
	lastModifiedTime := obj.LastModifiedTime()
	lgtmTime := obj.LabelTime(lgtmLabel)

	if lastModifiedTime == nil || lgtmTime == nil {
		glog.Errorf("PR %d was unable to determine when LGTM was added or when last modified", *obj.Issue.Number)
		sq.SetMergeStatus(obj, unknown)
		return false
	}

	if lastModifiedTime.After(*lgtmTime) {
		sq.SetMergeStatus(obj, lgtmEarly)
		return false
	}

	// PR cannot have the label which prevents merging.
	if obj.HasLabel(doNotMergeLabel) {
		sq.SetMergeStatus(obj, noMerge)
		return false
	}

	return true
}

// Munge is the workhorse the will actually make updates to the PR
func (sq *SubmitQueue) Munge(obj *github.MungeObject) {
	if !sq.validForMerge(obj) {
		return
	}

	added := false
	sq.Lock()
	if _, ok := sq.githubE2EQueue[*obj.Issue.Number]; !ok {
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

// If the PR was put in the github e2e queue previously, but now we don't
// think it should be in the e2e queue, remove it. MUST be called with sq.Lock()
// held.
func (sq *SubmitQueue) cleanupOldE2E(obj *github.MungeObject, reason string) {
	switch reason {
	case e2eFailure:
	case ghE2EQueued:
	case ghE2EWaitingStart:
	case ghE2ERunning:
		// Do nothing
	case ciFailure:
		// ciFailure is intersting. If the PR is being actively retested and then the
		// time based loop finds the same PR it will try to set ciFailure. We should in fact
		// not ever call this function in this case, but if we do call here, log it.
		if sq.githubE2ERunning != nil && *sq.githubE2ERunning.Issue.Number == *obj.Issue.Number {
			glog.Errorf("Trying to clean up %d due to ciFailure while it is being tested")
			return
		}
		fallthrough
	default:
		if sq.githubE2ERunning != nil && *sq.githubE2ERunning.Issue.Number == *obj.Issue.Number {
			sq.githubE2ERunning = nil
		}
		delete(sq.githubE2EQueue, *obj.Issue.Number)
	}

}

func priority(obj *github.MungeObject) int {
	// jump to the front of the queue if you don't need retested
	if obj.HasLabel(e2eNotRequiredLabel) {
		return e2eNotRequiredMergePriority
	}

	prio := obj.Priority()
	// eparis randomly decided that unlabel issues count at p3
	if prio == math.MaxInt32 {
		return defaultMergePriority
	}
	return prio
}

type queueSorter []*github.MungeObject

func (s queueSorter) Len() int      { return len(s) }
func (s queueSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// If you update the function PLEASE PLEASE PLEASE also update servePriorityInfo()
func (s queueSorter) Less(i, j int) bool {
	a := s[i]
	b := s[j]

	aPrio := priority(a)
	bPrio := priority(b)

	if aPrio < bPrio {
		return true
	} else if aPrio > bPrio {
		return false
	}

	aDue := a.ReleaseMilestoneDue()
	bDue := b.ReleaseMilestoneDue()

	if aDue.Before(bDue) {
		return true
	} else if aDue.After(bDue) {
		return false
	}

	return *a.Issue.Number < *b.Issue.Number
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
	sort.Sort(queueSorter(prs))

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
		if l == 0 || !sq.e2eStable() {
			time.Sleep(sq.githubE2EPollTime)
			continue
		}

		sq.Lock()
		if len(sq.githubE2EQueue) == 0 {
			sq.Unlock()
			continue
		}
		keys := sq.orderedE2EQueue()
		obj := sq.githubE2EQueue[keys[0]]
		sq.githubE2ERunning = obj
		sq.Unlock()

		// re-test and maybe merge
		sq.doGithubE2EAndMerge(obj)

		// remove it from the map after we finish testing
		sq.Lock()
		sq.githubE2ERunning = nil
		delete(sq.githubE2EQueue, keys[0])
		sq.Unlock()
	}
}

func (sq *SubmitQueue) doGithubE2EAndMerge(obj *github.MungeObject) {
	err := obj.Refresh()
	if err != nil {
		glog.Errorf("%d: unknown err: %v", *obj.Issue.Number, err)
		sq.SetMergeStatus(obj, unknown)
		return
	}

	if !sq.validForMerge(obj) {
		return
	}

	if obj.HasLabel(e2eNotRequiredLabel) {
		obj.MergePR("submit-queue")
		sq.SetMergeStatus(obj, merged)
		return
	}

	if err := obj.WriteComment(verifySafeToMergeBody); err != nil {
		glog.Errorf("%d: unknown err: %v", *obj.Issue.Number, err)
		sq.SetMergeStatus(obj, unknown)
		return
	}

	// Wait for the build to start
	sq.SetMergeStatus(obj, ghE2EWaitingStart)
	err = obj.WaitForPending([]string{sq.E2EStatusContext, sq.UnitStatusContext})
	if err != nil {
		s := fmt.Sprintf("Failed waiting for PR to start testing: %v", err)
		sq.SetMergeStatus(obj, s)
		return
	}

	// Wait for the status to go back to something other than pending
	sq.SetMergeStatus(obj, ghE2ERunning)
	err = obj.WaitForNotPending([]string{sq.E2EStatusContext, sq.UnitStatusContext})
	if err != nil {
		s := fmt.Sprintf("Failed waiting for PR to finish testing: %v", err)
		sq.SetMergeStatus(obj, s)
		return
	}

	// Check if the thing we care about is success
	if ok := obj.IsStatusSuccess([]string{sq.E2EStatusContext, sq.UnitStatusContext}); !ok {
		sq.SetMergeStatus(obj, ghE2EFailed)
		return
	}

	if !sq.e2eStable() {
		sq.SetMergeStatus(obj, e2eFailure)
		return
	}

	obj.MergePR("submit-queue")
	sq.updateMergeRate()
	sq.SetMergeStatus(obj, merged)
	return
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

func (sq *SubmitQueue) serveUsers(res http.ResponseWriter, req *http.Request) {
	data := sq.getUserInfo()
	sq.serve(data, res, req)
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

func (sq *SubmitQueue) serveGoogleInternalStatus(res http.ResponseWriter, req *http.Request) {
	data := sq.getGoogleInternalStatus()
	sq.serve(data, res, req)
}

func (sq *SubmitQueue) serveHealth(res http.ResponseWriter, req *http.Request) {
	data := sq.getHealth()
	sq.serve(data, res, req)
}

func (sq *SubmitQueue) serveSQStats(res http.ResponseWriter, req *http.Request) {
	data := submitQueueStats{
		LastMergeTime: sq.lastMergeTime,
		MergeRate:     sq.calcMergeRateWithTail(),
	}
	sq.serve(sq.marshal(data), res, req)
}

func (sq *SubmitQueue) serveMergeInfo(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-type", "text/plain")
	res.WriteHeader(http.StatusOK)
	var out bytes.Buffer
	out.WriteString("PRs must meet the following set of conditions to be considered for automatic merging by the submit queue.")
	out.WriteString("<ol>")
	out.WriteString(fmt.Sprintf("<li>The PR must have the label %q or %q</li>", claYesLabel, claHumanLabel))
	out.WriteString("<li>The PR must be mergeable. aka cannot need a rebase</li>")
	contexts := sq.RequiredStatusContexts
	exceptStr := ""
	if len(sq.E2EStatusContext) > 0 {
		contexts = append(contexts, sq.E2EStatusContext)
		exceptStr = fmt.Sprintf("Note: %q is not required if the PR has the %q label", sq.E2EStatusContext, e2eNotRequiredLabel)
	}
	if len(sq.UnitStatusContext) > 0 {
		contexts = append(contexts, sq.UnitStatusContext)
	}
	if len(contexts) > 0 {
		out.WriteString("<li>All of the following github statuses must be green")
		out.WriteString("<ul>")
		for _, context := range contexts {
			out.WriteString(fmt.Sprintf("<li>%s</li>", context))
		}
		out.WriteString("</ul>")
		out.WriteString(fmt.Sprintf("%s</li>", exceptStr))
	}
	out.WriteString(fmt.Sprintf("<li>The PR either needs the label %q or the creator of the PR must be in the 'Users' list seen on the 'Info' tab.</li>", okToMergeLabel))
	out.WriteString(fmt.Sprintf(`<li>The PR must have the %q label</li>`, lgtmLabel))
	out.WriteString(fmt.Sprintf("<li>The PR must not have been updated since the %q label was applied</li>", lgtmLabel))
	out.WriteString(fmt.Sprintf("<li>The PR must not have the %q label</li>", doNotMergeLabel))
	out.WriteString(`</ol><br>`)
	out.WriteString("The PR can then be queued to re-test before merge. Once it reaches the top of the queue all of the above conditions must be true but so must the following:")
	out.WriteString("<ol>")
	out.WriteString(fmt.Sprintf("<li>All of the <a href=http://submit-queue.k8s.io/#/e2e>continuously running e2e tests</a> must be passing</li>"))
	out.WriteString(fmt.Sprintf("<li>The %s tests must pass a second time<br>", sq.E2EStatusContext))
	out.WriteString(fmt.Sprintf("Note: The %s tests are not required if the %q label is present</li>", sq.E2EStatusContext, e2eNotRequiredLabel))
	out.WriteString("</ol>")
	out.WriteString("And then the PR will be merged!!")
	res.Write(out.Bytes())
}

func (sq *SubmitQueue) servePriorityInfo(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-type", "text/plain")
	res.WriteHeader(http.StatusOK)
	res.Write([]byte(`The merge queue is sorted by the following. If there is a tie in any test the next test will be used. A P0 will always come before a P1, no matter how the other tests compare.
<ol>
  <li>Priority
    <ul>
      <li>Determined by a label of the form 'priority/pX'
      <li>P0 -&gt; P1 -&gt; P2</li>
      <li>A PR with no priority label is considered equal to a P3</li>
      <li>A PR with the '` + e2eNotRequiredLabel + `' label will come first, before even P0</li>
    </ul>
  </li>
  <li>Release milestone due date
    <ul>
      <li>Release milestones are of the form vX.Y where X and Y are integers</li>
      <li>Other milestones are ignored.
      <li>PR with no release milestone will be considered after any PR with a milestone</li>
    </ul>
  </li>
  <li>PR number</li>
</ol> `))
}

func (sq *SubmitQueue) isStaleWhitelistComment(obj *github.MungeObject, comment githubapi.IssueComment) bool {
	if !mergeBotComment(comment) {
		return false
	}
	if *comment.Body != notInWhitelistBody {
		return false
	}
	stale := obj.HasLabel(okToMergeLabel)
	if stale {
		glog.V(6).Infof("Found stale SubmitQueue Whitelist comment")
	}
	return stale
}

func (sq *SubmitQueue) isStaleSafeToMergeComment(obj *github.MungeObject, comment githubapi.IssueComment) bool {
	if !mergeBotComment(comment) {
		return false
	}
	if *comment.Body != verifySafeToMergeBody {
		return false
	}
	stale := commentBeforeLastCI(obj, comment)
	if stale {
		glog.V(6).Infof("Found stale SubmitQueue safe to merge comment")
	}
	return stale
}

func (sq *SubmitQueue) isStaleComment(obj *github.MungeObject, comment githubapi.IssueComment) bool {
	return sq.isStaleWhitelistComment(obj, comment) || sq.isStaleSafeToMergeComment(obj, comment)
}

// StaleComments returns a slice of stale comments
func (sq *SubmitQueue) StaleComments(obj *github.MungeObject, comments []githubapi.IssueComment) []githubapi.IssueComment {
	return forEachCommentTest(obj, comments, sq.isStaleComment)
}
