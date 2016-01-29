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
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"k8s.io/kubernetes/pkg/util/sets"

	"k8s.io/contrib/mungegithub/github"
	"k8s.io/contrib/mungegithub/mungers/e2e"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

const (
	needsOKToMergeLabel = "needs-ok-to-merge"
	e2eNotRequiredLabel = "e2e-not-required"
	claYes              = "cla: yes"
	claHuman            = "cla: human-approved"

	jenkinsE2EContext  = "Jenkins GCE e2e"
	jenkinsUnitContext = "Jenkins unit/integration"
	travisContext      = "continuous-integration/travis-ci/pr"
	sqContext          = "Submit Queue"
)

var (
	_ = fmt.Print
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

// SubmitQueue will merge PR which meet a set of requirements.
//  PR must have LGTM after the last commit
//  PR must have passed all github CI checks
//  if user not in whitelist PR must have "ok-to-merge"
//  The google internal jenkins instance must be passing the JenkinsJobs e2e tests
type SubmitQueue struct {
	githubConfig           *github.Config
	JenkinsJobs            []string
	JenkinsHost            string
	Whitelist              string
	WhitelistOverride      string
	Committers             string
	Address                string
	E2EStatusContext       string
	UnitStatusContext      string
	RequiredStatusContexts []string
	WWWRoot                string

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

	// Every time a PR is added to githubE2EQueue also notify the channel
	githubE2EWakeup  chan bool
	githubE2ERunning *github.MungeObject         // protect by sync.Mutex!
	githubE2EQueue   map[int]*github.MungeObject // protected by sync.Mutex!

	e2e *e2e.E2ETester
}

func init() {
	RegisterMungerOrDie(&SubmitQueue{})
}

// Name is the name usable in --pr-mungers
func (sq SubmitQueue) Name() string { return "submit-queue" }

// Initialize will initialize the munger
func (sq *SubmitQueue) Initialize(config *github.Config) error {
	sq.Lock()
	defer sq.Unlock()

	sq.githubConfig = config
	if len(sq.JenkinsHost) == 0 {
		glog.Fatalf("--jenkins-host is required.")
	}

	e2e := &e2e.E2ETester{
		JenkinsJobs: sq.JenkinsJobs,
		JenkinsHost: sq.JenkinsHost,
		BuildStatus: map[string]e2e.BuildInfo{},
	}
	sq.e2e = e2e
	if len(sq.Address) > 0 {
		if len(sq.WWWRoot) > 0 {
			http.Handle("/", http.FileServer(http.Dir(sq.WWWRoot)))
		}
		http.HandleFunc("/prs", func(w http.ResponseWriter, r *http.Request) {
			sq.servePRs(w, r)
		})
		http.HandleFunc("/history", func(w http.ResponseWriter, r *http.Request) {
			sq.serveHistory(w, r)
		})
		http.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
			sq.serveUsers(w, r)
		})
		http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
			sq.serveBotStats(w, r)
		})
		http.HandleFunc("/github-e2e-queue", func(w http.ResponseWriter, r *http.Request) {
			sq.serveGithubE2EStatus(w, r)
		})
		http.HandleFunc("/google-internal-ci", func(w http.ResponseWriter, r *http.Request) {
			sq.serveGoogleInternalStatus(w, r)
		})
		go http.ListenAndServe(sq.Address, nil)
	}
	sq.prStatus = map[string]submitStatus{}
	sq.lastPRStatus = map[string]submitStatus{}

	sq.githubE2EWakeup = make(chan bool, 1000)
	sq.githubE2EQueue = map[int]*github.MungeObject{}

	go sq.handleGithubE2EAndMerge()
	go sq.updateGoogleE2ELoop()
	return nil
}

// EachLoop is called at the start of every munge loop
func (sq *SubmitQueue) EachLoop() error {
	sq.Lock()
	defer sq.Unlock()
	sq.RefreshWhitelist()
	sq.lastPRStatus = sq.prStatus
	sq.prStatus = map[string]submitStatus{}
	return nil
}

// AddFlags will add any request flags to the cobra `cmd`
func (sq *SubmitQueue) AddFlags(cmd *cobra.Command, config *github.Config) {
	cmd.Flags().StringSliceVar(&sq.JenkinsJobs, "jenkins-jobs", []string{
		"kubernetes-build",
		"kubernetes-test-go",
		"kubernetes-e2e-gce",
		"kubernetes-e2e-gce-slow",
		"kubernetes-e2e-gke",
		"kubernetes-e2e-gke-slow",
		"kubernetes-e2e-gce-scalability",
		"kubernetes-kubemark-gce",
	}, "Comma separated list of jobs in Jenkins to use for stability testing")
	cmd.Flags().StringVar(&sq.JenkinsHost, "jenkins-host", "http://jenkins-master:8080", "The URL for the jenkins job to watch")
	cmd.Flags().StringSliceVar(&sq.RequiredStatusContexts, "required-contexts", []string{travisContext}, "Comma separate list of status contexts required for a PR to be considered ok to merge")
	cmd.Flags().StringVar(&sq.Address, "address", ":8080", "The address to listen on for HTTP Status")
	cmd.Flags().StringVar(&sq.E2EStatusContext, "e2e-status-context", jenkinsE2EContext, "The name of the github status context for the e2e PR Builder")
	cmd.Flags().StringVar(&sq.UnitStatusContext, "unit-status-context", jenkinsUnitContext, "The name of the github status context for the unit PR Builder")
	cmd.Flags().StringVar(&sq.WWWRoot, "www", "www", "Path to static web files to serve from the webserver")
	sq.addWhitelistCommand(cmd, config)
}

// This serves little purpose other than to show updates every minute in the
// web UI. Stable() will get called as needed against individual PRs as well.
func (sq *SubmitQueue) updateGoogleE2ELoop() {
	for {
		if !sq.e2e.Stable() {
			sq.flushGithubE2EQueue(e2eFailure)
		}
		time.Sleep(1 * time.Minute)
	}

}

func objToStatusPullRequest(obj *github.MungeObject) *statusPullRequest {
	if obj == nil {
		return &statusPullRequest{}
	}
	return &statusPullRequest{
		Number:    *obj.Issue.Number,
		URL:       *obj.Issue.HTMLURL,
		Title:     *obj.Issue.Title,
		Login:     *obj.Issue.User.Login,
		AvatarURL: *obj.Issue.User.AvatarURL,
	}
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
// but used instead of manipulating the prStatus directly as sq.Lock() must be
// called when manipulating that structure
// `obj` is the active github object
// `reason` is the new 'status' for this object
// `record` is wether we should show this status on the web page or not
//    In general we do not show the status updates for PRs which didn't reach the
//    're-run github e2e' state as these are more obvious, change less, and don't
//    seem to ever confuse people.
func (sq *SubmitQueue) SetMergeStatus(obj *github.MungeObject, reason string, record bool) {
	glog.V(4).Infof("SubmitQueue not merging %d because %q", *obj.Issue.Number, reason)
	submitStatus := submitStatus{
		Time:              time.Now(),
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

	if record {
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

func (sq *SubmitQueue) getBotStats() []byte {
	sq.Lock()
	defer sq.Unlock()
	stats := sq.githubConfig.GetDebugStats()
	return sq.marshal(stats)
}

func (sq *SubmitQueue) getGoogleInternalStatus() []byte {
	sq.Lock()
	defer sq.Unlock()
	return sq.marshal(sq.e2e.GetBuildStatus())
}

const (
	unknown                 = "unknown failure"
	noCLA                   = "PR does not have " + claYes + " or " + claHuman
	noLGTM                  = "PR does not have LGTM."
	needsok                 = "PR does not have 'ok-to-merge' label"
	lgtmEarly               = "The PR was changed after the LGTM label was added."
	unmergeable             = "PR is unable to be automatically merged. Needs rebase."
	undeterminedMergability = "Unable to determine is PR is mergeable. Will try again later."
	ciFailure               = "Github CI tests are not green."
	e2eFailure              = "The e2e tests are failing. The entire submit queue is blocked."
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

// Munge is the workhorse the will actually make updates to the PR
func (sq *SubmitQueue) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	e2e := sq.e2e
	userSet := sq.userWhitelist

	if !obj.HasLabels([]string{claYes}) && !obj.HasLabels([]string{claHuman}) {
		sq.SetMergeStatus(obj, noCLA, false)
		return
	}

	if mergeable, err := obj.IsMergeable(); err != nil {
		sq.SetMergeStatus(obj, undeterminedMergability, false)
		return
	} else if !mergeable {
		sq.SetMergeStatus(obj, unmergeable, false)
		return
	}

	// Validate the status information for this PR
	contexts := sq.requiredStatusContexts(obj)
	if ok := obj.IsStatusSuccess(contexts); !ok {
		sq.SetMergeStatus(obj, ciFailure, false)
		return
	}

	if !obj.HasLabel(sq.WhitelistOverride) && !userSet.Has(*obj.Issue.User.Login) {
		if !obj.HasLabel(needsOKToMergeLabel) {
			obj.AddLabels([]string{needsOKToMergeLabel})
			body := "The author of this PR is not in the whitelist for merge, can one of the admins add the 'ok-to-merge' label?"
			obj.WriteComment(body)
		}
		sq.SetMergeStatus(obj, needsok, false)
		return
	}

	// Tidy up the issue list.
	if obj.HasLabel(needsOKToMergeLabel) {
		obj.RemoveLabel(needsOKToMergeLabel)
	}

	if !obj.HasLabels([]string{"lgtm"}) {
		sq.SetMergeStatus(obj, noLGTM, false)
		return
	}

	lastModifiedTime := obj.LastModifiedTime()
	lgtmTime := obj.LabelTime("lgtm")

	if lastModifiedTime == nil || lgtmTime == nil {
		glog.Errorf("PR %d was unable to determine when LGTM was added or when last modified", *obj.Issue.Number)
		sq.SetMergeStatus(obj, unknown, false)
		return
	}

	if lastModifiedTime.After(*lgtmTime) {
		sq.SetMergeStatus(obj, lgtmEarly, false)
		return
	}

	if !e2e.Stable() {
		sq.flushGithubE2EQueue(e2eFailure)
		sq.SetMergeStatus(obj, e2eFailure, false)
		return
	}

	// if there is a 'e2e-not-required' label, just merge it.
	if obj.HasLabel(e2eNotRequiredLabel) {
		obj.MergePR("submit-queue")
		sq.SetMergeStatus(obj, merged, true)
		return
	}

	added := false
	sq.Lock()
	if _, ok := sq.githubE2EQueue[*obj.Issue.Number]; !ok {
		sq.githubE2EQueue[*obj.Issue.Number] = obj
		sq.githubE2EWakeup <- true
		added = true
	}
	sq.Unlock()
	if added {
		sq.SetMergeStatus(obj, ghE2EQueued, true)
	}

	return
}

// If the PR was put in the github e2e queue on a current pass, but now we don't
// think it should be in the e2e queue, remove it. MUST be called with sq.Lock()
// held.
func (sq *SubmitQueue) cleanupOldE2E(obj *github.MungeObject, reason string) {
	switch reason {
	case ghE2EQueued:
	case ghE2EWaitingStart:
	case ghE2ERunning:
		// Do nothing
	default:
		delete(sq.githubE2EQueue, *obj.Issue.Number)
	}

}

// flushGithubE2EQueue will rmeove all entries from the build queue and will mark them
// as failed with the given reason. We do not need to flush the githubE2EWakeup
// channel as that just causes handleGithubE2EAndMerge() to wake up. And if it
// wakes up a few extra times, who cares.
func (sq *SubmitQueue) flushGithubE2EQueue(reason string) {
	objs := []*github.MungeObject{}
	sq.Lock()
	for _, obj := range sq.githubE2EQueue {
		objs = append(objs, obj)
	}
	sq.Unlock()
	for _, obj := range objs {
		sq.SetMergeStatus(obj, reason, true)
	}
}

type queueSorter []*github.MungeObject

func (s queueSorter) Len() int      { return len(s) }
func (s queueSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s queueSorter) Less(i, j int) bool {
	a := s[i]
	b := s[j]

	aPrio := a.Priority()
	bPrio := b.Priority()

	// eparis randomly decided that unlabel issues count at p3
	if aPrio == math.MaxInt32 {
		aPrio = 3
	}
	if bPrio == math.MaxInt32 {
		bPrio = 3
	}

	if aPrio < bPrio {
		return true
	} else if aPrio > bPrio {
		return false
	}

	return *a.Issue.Number < *b.Issue.Number
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
		// Wait until something is ready to be processed
		select {
		case _ = <-sq.githubE2EWakeup:
		}

		sq.Lock()
		// Could happen if the same PR was added twice. It will only be
		// in the map one time, but it will be in the channel twice.
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
	_, err := obj.RefreshPR()
	if err != nil {
		glog.Errorf("%d: unknown err: %v", *obj.Issue.Number, err)
		sq.SetMergeStatus(obj, unknown, true)
		return
	}

	if m, err := obj.IsMerged(); err != nil {
		glog.Errorf("%d: unknown err: %v", *obj.Issue.Number, err)
		sq.SetMergeStatus(obj, unknown, true)
		return
	} else if m {
		sq.SetMergeStatus(obj, merged, true)
		return
	}

	if mergeable, err := obj.IsMergeable(); err != nil {
		sq.SetMergeStatus(obj, undeterminedMergability, true)
		return
	} else if !mergeable {
		sq.SetMergeStatus(obj, unmergeable, true)
		return
	}

	body := "@k8s-bot test this [submit-queue is verifying that this PR is safe to merge]"
	if err := obj.WriteComment(body); err != nil {
		glog.Errorf("%d: unknown err: %v", *obj.Issue.Number, err)
		sq.SetMergeStatus(obj, unknown, true)
		return
	}

	// Wait for the build to start
	sq.SetMergeStatus(obj, ghE2EWaitingStart, true)
	err = obj.WaitForPending([]string{sq.E2EStatusContext, sq.UnitStatusContext})
	if err != nil {
		s := fmt.Sprintf("Failed waiting for PR to start testing: %v", err)
		sq.SetMergeStatus(obj, s, true)
		return
	}

	// Wait for the status to go back to something other than pending
	sq.SetMergeStatus(obj, ghE2ERunning, true)
	err = obj.WaitForNotPending([]string{sq.E2EStatusContext, sq.UnitStatusContext})
	if err != nil {
		s := fmt.Sprintf("Failed waiting for PR to finish testing: %v", err)
		sq.SetMergeStatus(obj, s, true)
		return
	}

	// Check if the thing we care about is success
	if ok := obj.IsStatusSuccess([]string{sq.E2EStatusContext, sq.UnitStatusContext}); !ok {
		sq.SetMergeStatus(obj, ghE2EFailed, true)
		return
	}

	if !sq.e2e.Stable() {
		sq.flushGithubE2EQueue(e2eFailure)
		sq.SetMergeStatus(obj, e2eFailure, true)
		return
	}

	obj.MergePR("submit-queue")
	sq.SetMergeStatus(obj, merged, true)
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

func (sq *SubmitQueue) serveBotStats(res http.ResponseWriter, req *http.Request) {
	data := sq.getBotStats()
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
