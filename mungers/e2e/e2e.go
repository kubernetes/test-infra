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

package e2e

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"

	cache "k8s.io/contrib/mungegithub/mungers/flakesync"
	"k8s.io/contrib/test-utils/utils"
	"k8s.io/kubernetes/pkg/util/sets"

	"github.com/golang/glog"
	"strings"
)

// E2ETester can be queried for E2E job stability.
type E2ETester interface {
	GCSBasedStable() (stable, ignorableFlakes bool)
	GCSWeakStable() bool
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
	JobNames           []string
	WeakStableJobNames []string

	sync.Mutex
	BuildStatus          map[string]BuildInfo // protect by mutex
	GoogleGCSBucketUtils *utils.Utils

	flakeCache *cache.Cache
}

// Init does construction-- call once it after setting the public fields of 'e'.
func (e *RealE2ETester) Init() *RealE2ETester {
	e.flakeCache = cache.NewCache(e.getGCSResult)
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
		return r, nil
	}

	r.Flakes = map[cache.Test]string{}
	for testName, reason := range thisFailures {
		r.Flakes[cache.Test(testName)] = reason
	}

	r.Status = cache.ResultFlaky
	return r, nil
}

// GCSBasedStable is a version of Stable function that depends on files stored in GCS instead of Jenkis
func (e *RealE2ETester) GCSBasedStable() (allStable, ignorableFlakes bool) {
	allStable = true

	for _, job := range e.JobNames {
		lastBuildNumber, err := e.GoogleGCSBucketUtils.GetLastestBuildNumberFromJenkinsGoogleBucket(job)
		glog.V(4).Infof("Checking status of %v, %v", job, lastBuildNumber)
		if err != nil {
			glog.Errorf("Error while getting data for %v: %v", job, err)
			e.setBuildStatus(job, "Not Stable", strconv.Itoa(lastBuildNumber))
			continue
		}

		thisResult, err := e.GetBuildResult(job, lastBuildNumber)
		if err != nil || thisResult.Status == cache.ResultFailed {
			glog.V(4).Infof("Found unstable job: %v, build number: %v: (err: %v) %#v", job, lastBuildNumber, err, thisResult)
			e.setBuildStatus(job, "Not Stable", strconv.Itoa(lastBuildNumber))
			allStable = false
			continue
		}

		if thisResult.Status == cache.ResultStable {
			e.setBuildStatus(job, "Stable", strconv.Itoa(lastBuildNumber))
			continue
		}

		lastResult, err := e.GetBuildResult(job, lastBuildNumber-1)
		if err != nil || lastResult.Status == cache.ResultFailed {
			glog.V(4).Infof("prev job doesn't help: %v, build number: %v (the previous build); (err %v) %#v", job, lastBuildNumber-1, err, lastResult)
			allStable = false
			e.setBuildStatus(job, "Not Stable", strconv.Itoa(lastBuildNumber))
			continue
		}

		if lastResult.Status == cache.ResultStable {
			ignorableFlakes = true
			e.setBuildStatus(job, "Ignorable flake", strconv.Itoa(lastBuildNumber))
			continue
		}

		intersection := sets.NewString()
		for testName := range thisResult.Flakes {
			if _, ok := lastResult.Flakes[testName]; ok {
				intersection.Insert(string(testName))
			}
		}
		if len(intersection) == 0 {
			glog.V(2).Infof("Ignoring failure of %v/%v since it didn't happen the previous run this run = %v; prev run = %v.", job, lastBuildNumber, thisResult.Flakes, lastResult.Flakes)
			ignorableFlakes = true
			e.setBuildStatus(job, "Ignorable flake", strconv.Itoa(lastBuildNumber))
			continue
		}
		glog.V(2).Infof("Failure of %v/%v is legit. Tests that failed multiple times in a row: %v", job, lastBuildNumber, intersection)
		allStable = false
		e.setBuildStatus(job, "Not Stable", strconv.Itoa(lastBuildNumber))
	}

	return allStable, ignorableFlakes
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
	ts := &Testsuite{}
	// TODO: this full parse is a bit slower than the old scanf routine--
	// could switch back for the case where we only care whether there was
	// a failure or not if that is an issue in practice.
	err = xml.NewDecoder(r).Decode(ts)
	if err != nil {
		return failures, err
	}
	if ts.FailCount == 0 {
		return nil, nil
	}
	failures = map[string]string{}
	for _, tc := range ts.Testcases {
		if tc.Failure != "" {
			failures[fmt.Sprintf("%v {%v}", tc.Name, tc.ClassName)] = tc.Failure
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

// GCSWeakStable is a version of GCSBasedStable with a slightly relaxed condition.
// This function says that e2e's are unstable only if there were real test failures
// (i.e. there was a test that failed, so no timeouts/cluster startup failures counts),
// or test failed for any reason 3 times in a row.
func (e *RealE2ETester) GCSWeakStable() bool {
	allStable := true
	for _, job := range e.WeakStableJobNames {
		lastBuildNumber, err := e.GoogleGCSBucketUtils.GetLastestBuildNumberFromJenkinsGoogleBucket(job)
		glog.V(4).Infof("Checking status of %v, %v", job, lastBuildNumber)
		if err != nil {
			glog.Errorf("Error while getting data for %v: %v", job, err)
			e.setBuildStatus(job, "Not Stable", strconv.Itoa(lastBuildNumber))
			continue
		}
		if stable, err := e.GoogleGCSBucketUtils.CheckFinishedStatus(job, lastBuildNumber); stable && err == nil {
			e.setBuildStatus(job, "Stable", strconv.Itoa(lastBuildNumber))
			continue
		}

		failures, err := e.failureReasons(job, lastBuildNumber, false)
		if err != nil {
			glog.Errorf("Error while getting data for %v/%v: %v", job, lastBuildNumber, err)
			e.setBuildStatus(job, "Not Stable", strconv.Itoa(lastBuildNumber))
			continue
		}

		thisStable := len(failures) == 0

		if thisStable == false {
			allStable = false
			e.setBuildStatus(job, "Not Stable", strconv.Itoa(lastBuildNumber))
			glog.Infof("WeakStable failed because found a failure in JUnit file for build %v; %v and possibly more failed", lastBuildNumber, failures)
			continue
		}

		// If we're here it means that we weren't able to find a test that failed, which means that the reason of build failure is comming from the infrastructure
		// Check results of previous two builds.
		unstable := make([]int, 0)
		if stable, err := e.GoogleGCSBucketUtils.CheckFinishedStatus(job, lastBuildNumber-1); !stable || err != nil {
			unstable = append(unstable, lastBuildNumber-1)
		}
		if stable, err := e.GoogleGCSBucketUtils.CheckFinishedStatus(job, lastBuildNumber-2); !stable || err != nil {
			unstable = append(unstable, lastBuildNumber-2)
		}
		if len(unstable) > 1 {
			e.setBuildStatus(job, "Not Stable", strconv.Itoa(lastBuildNumber))
			allStable = false
			glog.Infof("WeakStable failed because found a weak failure in build %v and builds %v failed.", lastBuildNumber, unstable)
			continue
		}
		e.setBuildStatus(job, "Stable", strconv.Itoa(lastBuildNumber))
	}
	return allStable
}
