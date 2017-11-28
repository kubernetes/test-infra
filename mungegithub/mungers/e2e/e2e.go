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

package e2e

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/contrib/test-utils/utils"
	cache "k8s.io/test-infra/mungegithub/mungers/flakesync"
	"k8s.io/test-infra/mungegithub/options"

	"io/ioutil"

	"github.com/golang/glog"
)

// E2ETester can be queried for E2E job stability.
type E2ETester interface {
	LoadNonBlockingStatus()
	GetBuildStatus() map[string]BuildInfo
	Flakes() cache.Flakes
}

// BuildInfo tells the build ID and the build success
type BuildInfo struct {
	Status string
	ID     string
}

// RealE2ETester is the object which will get status from a google bucket
// information about recent jobs
type RealE2ETester struct {
	Opts                *options.Options
	NonBlockingJobNames *[]string

	sync.Mutex
	BuildStatus          map[string]BuildInfo // protect by mutex
	GoogleGCSBucketUtils *utils.Utils

	flakeCache        *cache.Cache
	resolutionTracker *ResolutionTracker
}

// HTTPHandlerInstaller is anything that can hook up HTTP requests to handlers.
// Used for installing admin functions.
type HTTPHandlerInstaller interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

// Init does construction-- call once it after setting the public fields of 'e'.
// adminMux may be nil, in which case handlers for the resolution tracker won't
// be installed.
func (e *RealE2ETester) Init(adminMux HTTPHandlerInstaller) *RealE2ETester {
	e.flakeCache = cache.NewCache(e.getGCSResult)
	e.resolutionTracker = NewResolutionTracker()
	if adminMux != nil {
		adminMux.HandleFunc("/api/mark-resolved", e.resolutionTracker.SetHTTP)
		adminMux.HandleFunc("/api/is-resolved", e.resolutionTracker.GetHTTP)
		adminMux.HandleFunc("/api/list-resolutions", e.resolutionTracker.ListHTTP)
	}
	return e
}

func (e *RealE2ETester) locked(f func()) {
	e.Lock()
	defer e.Unlock()
	f()
}

// GetBuildStatus returns the build status. This map is a copy and is thus safe
// for the caller to use in any way.
func (e *RealE2ETester) GetBuildStatus() map[string]BuildInfo {
	e.Lock()
	defer e.Unlock()
	out := map[string]BuildInfo{}
	for k, v := range e.BuildStatus {
		out[k] = v
	}
	return out
}

// Flakes returns a sorted list of current flakes.
func (e *RealE2ETester) Flakes() cache.Flakes {
	return e.flakeCache.Flakes()
}

func (e *RealE2ETester) setBuildStatus(build, status string, id string) {
	e.Lock()
	defer e.Unlock()
	e.BuildStatus[build] = BuildInfo{Status: status, ID: id}
}

const (
	// ExpectedXMLHeader is the expected header of junit_XX.xml file
	ExpectedXMLHeader = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>"
)

// GetBuildResult returns (or gets) the cached result of the job and build. Public.
func (e *RealE2ETester) GetBuildResult(job string, number int) (*cache.Result, error) {
	return e.flakeCache.Get(cache.Job(job), cache.Number(number))
}

func (e *RealE2ETester) getGCSResult(j cache.Job, n cache.Number) (*cache.Result, error) {
	// The difference between pre- and post-submit tests is that in the
	// former, we look for flakes when they pass, and in the latter, we
	// look for flakes when they fail. This is because presubmit tests will
	// run multiple times and pass if at least one run passed, but
	// postsubmit tests run each test only once. For postsubmit tests, we
	// detect flakiness by comparing between runs, but that's not possible
	// for presubmit tests, because the PR author might have actually
	// broken something.
	if strings.Contains(string(j), "pull") {
		return e.getGCSPresubmitResult(j, n)
	}
	return e.getGCSPostsubmitResult(j, n)
}

func (e *RealE2ETester) getGCSPostsubmitResult(j cache.Job, n cache.Number) (*cache.Result, error) {
	stable, err := e.GoogleGCSBucketUtils.CheckFinishedStatus(string(j), int(n))
	if err != nil {
		glog.V(4).Infof("Error looking up job: %v, build number: %v", j, n)
		// Not actually fatal!
	}
	r := &cache.Result{
		Job:    j,
		Number: n,
		// TODO: StartTime:
	}
	if stable {
		r.Status = cache.ResultStable
		return r, nil
	}

	// This isn't stable-- see if we can find a reason.
	thisFailures, err := e.failureReasons(string(j), int(n), true)
	if err != nil {
		glog.V(4).Infof("Error looking up job failure reasons: %v, build number: %v: %v", j, n, err)
		thisFailures = nil // ensure we fall through
	}
	if len(thisFailures) == 0 {
		r.Status = cache.ResultFailed
		// Don't return any flake information, to reduce flake noise -- getting an issue opened
		// for every failed run without logs is not useful.
		return r, nil
	}

	r.Flakes = map[cache.Test]string{}
	for testName, reason := range thisFailures {
		r.Flakes[cache.Test(testName)] = reason
	}

	r.Status = cache.ResultFlaky
	return r, nil
}

func (e *RealE2ETester) getGCSPresubmitResult(j cache.Job, n cache.Number) (*cache.Result, error) {
	stable, err := e.GoogleGCSBucketUtils.CheckFinishedStatus(string(j), int(n))
	if err != nil {
		return nil, fmt.Errorf("error looking up job: %v, build number: %v", j, n)
	}
	r := &cache.Result{
		Status: cache.ResultStable,
		Job:    j,
		Number: n,
	}
	if !stable {
		r.Status = cache.ResultFailed
		// We do *not* add a "run completely broken" flake entry since
		// this is presumably the author's fault, and we don't want to
		// file issues for things like that.
		return r, nil
	}

	// Check to see if there were any individual failures (even though the
	// run as a whole succeeded).
	thisFailures, err := e.failureReasons(string(j), int(n), true)
	if err != nil {
		glog.V(2).Infof("Error looking up job failure reasons: %v, build number: %v: %v", j, n, err)
		return r, nil
	}
	if len(thisFailures) == 0 {
		glog.V(2).Infof("No flakes in %v/%v.", j, n)
		return r, nil
	}

	r.Flakes = map[cache.Test]string{}
	for testName, reason := range thisFailures {
		r.Flakes[cache.Test(testName)] = reason
	}

	r.Status = cache.ResultFlaky
	return r, nil
}

func (e *RealE2ETester) checkPassFail(job string, number int) (stable, ignorableFlakes bool) {
	if e.resolutionTracker.Resolved(cache.Job(job), cache.Number(number)) {
		e.setBuildStatus(job, "Problem Resolved", strconv.Itoa(number))
		return true, true
	}

	thisResult, err := e.GetBuildResult(job, number)
	if err != nil || thisResult.Status == cache.ResultFailed {
		glog.V(4).Infof("Found unstable job: %v, build number: %v: (err: %v) %#v", job, number, err, thisResult)
		e.setBuildStatus(job, "Not Stable", strconv.Itoa(number))
		return false, false
	}

	if thisResult.Status == cache.ResultStable {
		e.setBuildStatus(job, "Stable", strconv.Itoa(number))
		return true, false
	}

	lastResult, err := e.GetBuildResult(job, number-1)
	if err != nil || lastResult.Status == cache.ResultFailed {
		glog.V(4).Infof("prev job doesn't help: %v, build number: %v (the previous build); (err %v) %#v", job, number-1, err, lastResult)
		e.setBuildStatus(job, "Not Stable", strconv.Itoa(number))
		return true, false
	}

	if lastResult.Status == cache.ResultStable {
		e.setBuildStatus(job, "Ignorable flake", strconv.Itoa(number))
		return true, true
	}

	intersection := sets.NewString()
	for testName := range thisResult.Flakes {
		if _, ok := lastResult.Flakes[testName]; ok {
			intersection.Insert(string(testName))
		}
	}
	if len(intersection) == 0 {
		glog.V(2).Infof("Ignoring failure of %v/%v since it didn't happen the previous run this run = %v; prev run = %v.", job, number, thisResult.Flakes, lastResult.Flakes)
		e.setBuildStatus(job, "Ignorable flake", strconv.Itoa(number))
		return true, true
	}
	glog.V(2).Infof("Failure of %v/%v is legit. Tests that failed multiple times in a row: %v", job, number, intersection)
	e.setBuildStatus(job, "Not Stable", strconv.Itoa(number))
	return false, false
}

// LatestRunOfJob returns the number of the most recent completed run of the given job.
func (e *RealE2ETester) LatestRunOfJob(jobName string) (int, error) {
	return e.GoogleGCSBucketUtils.GetLastestBuildNumberFromJenkinsGoogleBucket(jobName)
}

// LoadNonBlockingStatus gets the build stability status for all the NonBlockingJobNames.
func (e *RealE2ETester) LoadNonBlockingStatus() {
	e.Opts.Lock()
	jobs := *e.NonBlockingJobNames
	e.Opts.Unlock()
	for _, job := range jobs {
		lastBuildNumber, err := e.GoogleGCSBucketUtils.GetLastestBuildNumberFromJenkinsGoogleBucket(job)
		glog.V(4).Infof("Checking status of %v, %v", job, lastBuildNumber)
		if err != nil {
			glog.Errorf("Error while getting data for %v: %v", job, err)
			e.setBuildStatus(job, "[nonblocking] Not Stable", strconv.Itoa(lastBuildNumber))
			continue
		}

		if thisResult, err := e.GetBuildResult(job, lastBuildNumber); err != nil || thisResult.Status != cache.ResultStable {
			e.setBuildStatus(job, "[nonblocking] Not Stable", strconv.Itoa(lastBuildNumber))
		} else {
			e.setBuildStatus(job, "[nonblocking] Stable", strconv.Itoa(lastBuildNumber))
		}
	}
}

func getJUnitFailures(r io.Reader) (failures map[string]string, err error) {
	type Testcase struct {
		Name      string `xml:"name,attr"`
		ClassName string `xml:"classname,attr"`
		Failure   string `xml:"failure"`
	}
	type Testsuite struct {
		TestCount int        `xml:"tests,attr"`
		FailCount int        `xml:"failures,attr"`
		Testcases []Testcase `xml:"testcase"`
	}
	type Testsuites struct {
		TestSuites []Testsuite `xml:"testsuite"`
	}
	var testSuiteList []Testsuite
	failures = map[string]string{}
	testSuites := &Testsuites{}
	testSuite := &Testsuite{}
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return failures, err
	}
	// first try to parse the result with <testsuites> as top tag
	err = xml.Unmarshal(b, testSuites)
	if err == nil && len(testSuites.TestSuites) > 0 {
		testSuiteList = testSuites.TestSuites
	} else {
		// second try to parse the result with <testsuite> as top tag
		err = xml.Unmarshal(b, testSuite)
		if err != nil {
			return nil, err
		}
		testSuiteList = []Testsuite{*testSuite}
	}
	for _, ts := range testSuiteList {
		for _, tc := range ts.Testcases {
			if tc.Failure != "" {
				failures[fmt.Sprintf("%v {%v}", tc.Name, tc.ClassName)] = tc.Failure
			}
		}
	}
	return failures, nil
}

// If completeList is true, collect every failure reason. Otherwise exit as soon as you see any failure.
func (e *RealE2ETester) failureReasons(job string, buildNumber int, completeList bool) (failedTests map[string]string, err error) {
	failuresFromResp := func(resp *http.Response) (failures map[string]string, err error) {
		defer resp.Body.Close()
		return getJUnitFailures(resp.Body)
	}
	failedTests = map[string]string{}

	// junit file prefix
	prefix := "artifacts/junit"
	junitList, err := e.GoogleGCSBucketUtils.ListFilesInBuild(job, buildNumber, prefix)
	if err != nil {
		glog.Errorf("Failed to list junit files for %v/%v/%v: %v", job, buildNumber, prefix, err)
	}

	// If we're here it means that build failed, so we need to look for a reason
	// by iterating over junit*.xml files and look for failures
	for _, filePath := range junitList {
		// if do not need complete list and we already have failed tests, then return
		if !completeList && len(failedTests) > 0 {
			break
		}
		if !strings.HasSuffix(filePath, ".xml") {
			continue
		}
		split := strings.Split(filePath, "/")
		junitFilePath := fmt.Sprintf("artifacts/%s", split[len(split)-1])
		response, err := e.GoogleGCSBucketUtils.GetFileFromJenkinsGoogleBucket(job, buildNumber, junitFilePath)
		if err != nil {
			return nil, fmt.Errorf("error while getting data for %v/%v/%v: %v", job, buildNumber, junitFilePath, err)
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			break
		}
		failures, err := failuresFromResp(response) // closes response.Body for us
		if err != nil {
			return nil, fmt.Errorf("failed to read the response for %v/%v/%v: %v", job, buildNumber, junitFilePath, err)
		}
		for k, v := range failures {
			failedTests[k] = v
		}
	}

	return failedTests, nil
}
