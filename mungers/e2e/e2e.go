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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"

	"k8s.io/contrib/mungegithub/mungers/jenkins"
	"k8s.io/contrib/test-utils/utils"

	"github.com/golang/glog"
)

// BuildInfo tells the build ID and the build success
type BuildInfo struct {
	Status string
	ID     string
}

// E2ETester is the object which will contact a jenkins instance and get
// information about recent jobs
type E2ETester struct {
	JenkinsHost        string
	JobNames           []string
	WeakStableJobNames []string

	sync.Mutex
	BuildStatus map[string]BuildInfo // protect by mutex
}

func (e *E2ETester) locked(f func()) {
	e.Lock()
	defer e.Unlock()
	f()
}

// GetBuildStatus returns the build status. This map is a copy and is thus safe
// for the caller to use in any way.
func (e *E2ETester) GetBuildStatus() map[string]BuildInfo {
	e.Lock()
	defer e.Unlock()
	out := map[string]BuildInfo{}
	for k, v := range e.BuildStatus {
		out[k] = v
	}
	return out
}

func (e *E2ETester) setBuildStatus(build, status string, id string) {
	e.Lock()
	defer e.Unlock()
	e.BuildStatus[build] = BuildInfo{Status: status, ID: id}
}

// Stable is called to make sure all of the jenkins jobs are stable
func (e *E2ETester) Stable() bool {
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
			allStable = false
		}
	}
	return allStable
}

const (
	successString     = "SUCCESS"
	expectedXMLHeader = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>"
)

type finishedFile struct {
	Result    string `json:"result"`
	Timestamp uint64 `json:"timestamp"`
}

func checkFinishedStatus(job string, lastBuildNumber int) bool {
	response, err := utils.GetFileFromJenkinsGoogleBucket(job, lastBuildNumber, "finished.json")
	if err != nil {
		glog.Errorf("Error while getting data for %v/%v/%v: %v", job, lastBuildNumber, "finished.json", err)
		return false
	}

	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		glog.Infof("Got a non-success response %v while reading data for %v/%v/%v", response.StatusCode, job, lastBuildNumber, "finished.json")
		return false
	}
	result := finishedFile{}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		glog.Errorf("Failed to read the response for %v/%v/%v: %v", job, lastBuildNumber, "finished.json", err)
		return false
	}
	err = json.Unmarshal(body, &result)
	if err != nil {
		glog.Errorf("Failed to unmarshal %v: %v", string(body), err)
		return false
	}
	return result.Result == successString
}

// GCSBasedStable is a version of Stable function that depends on files stored in GCS instead of Jenkis
func (e *E2ETester) GCSBasedStable() bool {
	for _, job := range e.JobNames {
		lastBuildNumber, err := utils.GetLastestBuildNumberFromJenkinsGoogleBucket(job)
		glog.V(4).Infof("Checking status of %v, %v", job, lastBuildNumber)
		if err != nil {
			glog.Errorf("Error while getting data for %v: %v", job, err)
			continue
		}
		if !checkFinishedStatus(job, lastBuildNumber) {
			return false
		}
	}

	return true
}

// GCSWeakStable is a version of GCSBasedStable with a slightly relaxed condition.
// This function says that e2e's are unstable only if there were real test failures
// (i.e. there was a test that failed, so no timeouts/cluster startup failures counts),
// or test failed for any reason 3 times in a row.
func (e *E2ETester) GCSWeakStable() bool {
	for _, job := range e.WeakStableJobNames {
		lastBuildNumber, err := utils.GetLastestBuildNumberFromJenkinsGoogleBucket(job)
		glog.Infof("Checking status of %v, %v", job, lastBuildNumber)
		if err != nil {
			glog.Errorf("Error while getting data for %v: %v", job, err)
			continue
		}
		if checkFinishedStatus(job, lastBuildNumber) {
			continue
		}

		// If we're here it means that build failed, so we need to look for a reason
		// by iterating over junit_XX.xml files and look for failures
		i := 0
		for {
			i++
			path := fmt.Sprintf("artifacts/junit_%02d.xml", i)
			response, err := utils.GetFileFromJenkinsGoogleBucket(job, lastBuildNumber, path)
			if err != nil {
				glog.Errorf("Error while getting data for %v/%v/%v: %v", job, lastBuildNumber, path, err)
				continue
			}
			if response.StatusCode != 200 {
				break
			}
			defer response.Body.Close()
			reader := bufio.NewReader(response.Body)
			body, err := reader.ReadString('\n')
			if err != nil {
				glog.Errorf("Failed to read the response for %v/%v/%v: %v", job, lastBuildNumber, path, err)
				continue
			}
			if strings.TrimSpace(body) != expectedXMLHeader {
				glog.Errorf("Invalid header for %v/%v/%v: %v, expected %v", job, lastBuildNumber, path, body, expectedXMLHeader)
				continue
			}
			body, err = reader.ReadString('\n')
			if err != nil {
				glog.Errorf("Failed to read the response for %v/%v/%v: %v", job, lastBuildNumber, path, err)
				continue
			}
			numberOfTests := 0
			nubmerOfFailures := 0
			timestamp := 0.0
			fmt.Sscanf(strings.TrimSpace(body), "<testsuite tests=\"%d\" failures=\"%d\" time=\"%f\">", &numberOfTests, &nubmerOfFailures, &timestamp)
			glog.V(4).Infof("%v, numberOfTests: %v, numberOfFailures: %v", string(body), numberOfTests, nubmerOfFailures)
			if nubmerOfFailures > 0 {
				glog.V(4).Infof("Found failure in %v for job %v build number %v", path, job, lastBuildNumber)
				return false
			}
		}

		// If we're here it means that we weren't able to find a test that failed, which means that the reason of build failure is comming from the infrastructure
		// Check results of previous two builds.
		if !checkFinishedStatus(job, lastBuildNumber-1) && !checkFinishedStatus(job, lastBuildNumber-2) {
			return false
		}
	}
	return true
}
