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
	"sync"

	"k8s.io/contrib/mungegithub/mungers/jenkins"

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
	JenkinsHost string
	JenkinsJobs []string

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
	for _, build := range e.JenkinsJobs {
		glog.V(2).Infof("Checking build stability for %s", build)
		job, err := jenkinsClient.GetLastCompletedBuild(build)
		if err != nil {
			glog.Errorf("Error checking build %v : %v", build, err)
			e.setBuildStatus(build, "Error checking: "+err.Error(), "0")
			allStable = false
			continue
		}
		if job.IsStable() {
			e.setBuildStatus(build, "Stable", job.ID)
		} else {
			e.setBuildStatus(build, "Not Stable", job.ID)
			allStable = false
		}
	}
	return allStable
}
