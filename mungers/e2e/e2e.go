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
	"bufio"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"k8s.io/contrib/mungegithub/mungers/jenkins"
	"k8s.io/contrib/test-utils/utils"

	"github.com/golang/glog"
)

// E2ETester can be queried for E2E job stability.
type E2ETester interface {
	GCSBasedStable() bool
	GCSWeakStable() bool
	GetBuildStatus() map[string]BuildInfo
	Stable() bool
}

// BuildInfo tells the build ID and the build success
type BuildInfo struct {
	Status string
	ID     string
}

// RealE2ETester is the object which will contact a jenkins instance and get
// information about recent jobs
type RealE2ETester struct {
	JenkinsHost        string
	JobNames           []string
	WeakStableJobNames []string

	sync.Mutex
	BuildStatus          map[string]BuildInfo // protect by mutex
	GoogleGCSBucketUtils *utils.Utils
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

func (e *RealE2ETester) setBuildStatus(build, status string, id string) {
	e.Lock()
	defer e.Unlock()
	e.BuildStatus[build] = BuildInfo{Status: status, ID: id}
}

// Stable is called to make sure all of the jenkins jobs are stable
func (e *RealE2ETester) Stable() bool {
	// Test if the build is stable in Jenkins
	jenkinsClient := &jenkins.JenkinsClient{Host: e.JenkinsHost}

	allStable := true
	for _, job := range e.JobNames {
		glog.V(2).Infof("Checking build stability for %s", job)
		build, err := jenkinsClient.GetLastCompletedBuild(job)
		if err != nil {
			glog.Errorf("Error checking job %v : %v", job, err)
			e.setBuildStatus(job, "Error checking: "+err.Error(), "0")
			allStable = false
			continue
		}
		if build.IsStable() {
			e.setBuildStatus(job, "Stable", build.ID)
		} else {
			e.setBuildStatus(job, "Not Stable", build.ID)
			glog.Infof("Jenkis based check for %v build %v returned false", job, build.ID)
			allStable = false
		}
	}
	return allStable
}

const (
	// ExpectedXMLHeader is the expected header of junit_XX.xml file
	ExpectedXMLHeader = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>"
)

// GCSBasedStable is a version of Stable function that depends on files stored in GCS instead of Jenkis
func (e *RealE2ETester) GCSBasedStable() bool {
	allStable := true

	for _, job := range e.JobNames {
		thisStable := true
		lastBuildNumber, err := e.GoogleGCSBucketUtils.GetLastestBuildNumberFromJenkinsGoogleBucket(job)
		glog.V(4).Infof("Checking status of %v, %v", job, lastBuildNumber)
		if err != nil {
			glog.Errorf("Error while getting data for %v: %v", job, err)
			e.setBuildStatus(job, "Not Stable", strconv.Itoa(lastBuildNumber))
			thisStable = false
		}
		if stable, err := e.GoogleGCSBucketUtils.CheckFinishedStatus(job, lastBuildNumber); !stable || err != nil {
			glog.V(4).Infof("Found unstable job: %v, build number: %v", job, lastBuildNumber)
			e.setBuildStatus(job, "Not Stable", strconv.Itoa(lastBuildNumber))
			thisStable = false
		}
		if thisStable {
			e.setBuildStatus(job, "Stable", strconv.Itoa(lastBuildNumber))
		} else {
			allStable = false
		}
	}

	return allStable
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

		// If we're here it means that build failed, so we need to look for a reason
		// by iterating over junit_XX.xml files and look for failures
		i := 1
		path := fmt.Sprintf("artifacts/junit_%02d.xml", i)
		response, err := e.GoogleGCSBucketUtils.GetFileFromJenkinsGoogleBucket(job, lastBuildNumber, path)
		if err != nil {
			glog.Errorf("Error while getting data for %v/%v/%v: %v", job, lastBuildNumber, path, err)
			e.setBuildStatus(job, "Not Stable", strconv.Itoa(lastBuildNumber))
			continue
		}
		defer response.Body.Close()
		thisStable := true
		for response.StatusCode == http.StatusOK {
			reader := bufio.NewReader(response.Body)
			body, err := reader.ReadString('\n')
			if err != nil {
				glog.Errorf("Failed to read the response for %v/%v/%v: %v", job, lastBuildNumber, path, err)
				thisStable = false
				break
			}
			if strings.TrimSpace(body) != ExpectedXMLHeader {
				glog.Errorf("Invalid header for %v/%v/%v: %v, expected %v", job, lastBuildNumber, path, body, ExpectedXMLHeader)
				thisStable = false
				break
			}
			body, err = reader.ReadString('\n')
			if err != nil {
				glog.Errorf("Failed to read the response for %v/%v/%v: %v", job, lastBuildNumber, path, err)
				thisStable = false
				break
			}
			numberOfTests := 0
			nubmerOfFailures := 0
			timestamp := 0.0
			fmt.Sscanf(strings.TrimSpace(body), "<testsuite tests=\"%d\" failures=\"%d\" time=\"%f\">", &numberOfTests, &nubmerOfFailures, &timestamp)
			glog.V(4).Infof("%v, numberOfTests: %v, numberOfFailures: %v", string(body), numberOfTests, nubmerOfFailures)
			if nubmerOfFailures > 0 {
				glog.V(4).Infof("Found failure in %v for job %v build number %v", path, job, lastBuildNumber)
				thisStable = false
				break
			}

			i++
			path = fmt.Sprintf("artifacts/junit_%02d.xml", i)
			response, err = e.GoogleGCSBucketUtils.GetFileFromJenkinsGoogleBucket(job, lastBuildNumber, path)
			if err != nil {
				glog.Errorf("Error while getting data for %v/%v/%v: %v", job, lastBuildNumber, path, err)
				continue
			}
			defer response.Body.Close()
		}

		if thisStable == false {
			allStable = false
			e.setBuildStatus(job, "Not Stable", strconv.Itoa(lastBuildNumber))
			glog.Infof("WeakStable failed because found a failure in JUnit file for build %v", lastBuildNumber)
			continue
		}

		// If we're here it means that we weren't able to find a test that failed, which means that the reason of build failure is comming from the infrastructure
		// Check results of previous two builds.
		if stable, err := e.GoogleGCSBucketUtils.CheckFinishedStatus(job, lastBuildNumber-1); !stable || err != nil {
			e.setBuildStatus(job, "Not Stable", strconv.Itoa(lastBuildNumber))
			allStable = false
			glog.Infof("WeakStable failed because found a weak failure in build %v and build %v failed.", lastBuildNumber, lastBuildNumber-1)
			continue
		}
		if stable, err := e.GoogleGCSBucketUtils.CheckFinishedStatus(job, lastBuildNumber-2); !stable || err != nil {
			e.setBuildStatus(job, "Not Stable", strconv.Itoa(lastBuildNumber))
			allStable = false
			glog.Infof("WeakStable failed because found a weak failure in build %v and build %v failed.", lastBuildNumber, lastBuildNumber-2)
			continue
		}

		e.setBuildStatus(job, "Stable", strconv.Itoa(lastBuildNumber))
	}
	return allStable
}
