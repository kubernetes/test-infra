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
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	github_util "k8s.io/contrib/mungegithub/github"
	"k8s.io/contrib/mungegithub/pulls/jenkins"

	"github.com/golang/glog"
	github_api "github.com/google/go-github/github"
)

// PRInfo explicitly collects the fields we wish to encode via JSON.
type PRInfo struct {
	Number *int    `json:"number"`
	URL    *string `json:"html_url"`
}

func prInfo(pr *github_api.PullRequest) *PRInfo {
	var out PRInfo
	if pr != nil {
		out.Number = pr.Number
		out.URL = pr.HTMLURL
	}
	return &out
}

// ExternalState is the information used by the web frontend
type ExternalState struct {
	// exported so that the json marshaller will print them
	CurrentPR   *PRInfo
	Message     []string
	Err         error
	BuildStatus map[string]string
	Whitelist   []string
}

// E2ETester is the object which will contact a jenkins instance and get
// information about recent jobs
type E2ETester struct {
	JenkinsHost         string
	JenkinsJobs         []string
	DontRequireE2ELabel string
	sync.Mutex
	State  *ExternalState
	Config *github_util.Config
}

func (e *E2ETester) msg(msg string, args ...interface{}) {
	e.Lock()
	defer e.Unlock()
	if len(e.State.Message) >= 50 {
		e.State.Message = e.State.Message[1:]
	}
	expanded := fmt.Sprintf(msg, args...)
	e.State.Message = append(e.State.Message, fmt.Sprintf("%v: %v", time.Now().UTC(), expanded))
	glog.V(2).Info(expanded)
}

func (e *E2ETester) error(err error) {
	e.Lock()
	defer e.Unlock()
	e.State.Err = err
}

// SetWhitelist will set the complete whitelist in the ExternalState.
func (e *E2ETester) SetWhitelist(whitelist []string) {
	e.Lock()
	defer e.Unlock()
	e.State.Whitelist = whitelist
}

func (e *E2ETester) locked(f func()) {
	e.Lock()
	defer e.Unlock()
	f()
}

func (e *E2ETester) setBuildStatus(build, status string) {
	e.locked(func() { e.State.BuildStatus[build] = status })
}

func (e *E2ETester) checkBuilds() (allStable bool) {
	// Test if the build is stable in Jenkins
	jenkinsClient := &jenkins.JenkinsClient{Host: e.JenkinsHost}
	allStable = true
	for _, build := range e.JenkinsJobs {
		e.msg("Checking build stability for %s", build)
		stable, err := jenkinsClient.IsBuildStable(build)
		if err != nil {
			e.msg("Error checking build %v: %v", build, err)
			e.setBuildStatus(build, "Error checking: "+err.Error())
			allStable = false
			continue
		}
		if stable {
			e.setBuildStatus(build, "Stable")
		} else {
			e.setBuildStatus(build, "Not Stable")
			allStable = false
		}
	}
	return allStable
}

func (e *E2ETester) waitForStableBuilds() {
	for !e.checkBuilds() {
		e.msg("Not all builds stable. Checking again in 30s")
		time.Sleep(30 * time.Second)
	}
}

// Run is called on a potentially mergable PR. It will do two things
//  - check if the internal e2e jobs are successful
//  - run the github e2e job again and merge if it is successful
func (e *E2ETester) Run(pr *github_api.PullRequest, issue *github_api.Issue) error {
	e.locked(func() { e.State.CurrentPR = prInfo(pr) })
	defer e.locked(func() { e.State.CurrentPR = nil })
	e.msg("Considering PR %d", *pr.Number)

	e.waitForStableBuilds()

	// if there is a 'e2e-not-required' label, just merge it.
	if len(e.DontRequireE2ELabel) > 0 && github_util.HasLabel(issue.Labels, e.DontRequireE2ELabel) {
		e.msg("Merging %d since %s is set", *pr.Number, e.DontRequireE2ELabel)
		return e.MergePR(pr, "submit-queue")
	}

	body := "@k8s-bot test this [submit-queue is verifying that this PR is safe to merge]"
	if err := e.Config.WriteComment(*pr.Number, body); err != nil {
		e.error(err)
		return err
	}

	// Wait for the build to start
	_ = e.Config.WaitForPending(pr)
	_ = e.Config.WaitForNotPending(pr)

	// Wait for the status to go back to 'success'
	if ok := e.Config.IsStatusSuccess(pr, []string{}); !ok {
		e.msg("Status after build is not 'success', skipping PR %d", *pr.Number)
		return nil
	}
	return e.Config.MergePR(pr, "submit-queue")
}

func (e *E2ETester) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	var (
		data []byte
		err  error
	)
	e.locked(func() {
		if e.State != nil {
			data, err = json.MarshalIndent(e.State, "", "\t")
		} else {
			data = []byte("{}")
		}
	})

	if err != nil {
		glog.Errorf("Failed to encode status: %#v %v", e.State, err)
		res.Header().Set("Content-type", "text/plain")
		res.WriteHeader(http.StatusInternalServerError)
		res.Write([]byte(err.Error()))
		res.Write([]byte(fmt.Sprintf("%#v", e.State)))
	} else {
		res.Header().Set("Content-type", "application/json")
		res.WriteHeader(http.StatusOK)
		res.Write(data)
	}
}
