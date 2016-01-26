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

package jenkins

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/golang/glog"
)

// JenkinsClient is how we talk to the Jenkins instance
type JenkinsClient struct {
	Host string
}

// Queue has information about the last completed builg and the last stable build
type Queue struct {
	Builds             []Build `json:"builds"`
	LastCompletedBuild Build   `json:"lastCompletedBuild"`
	LastStableBuild    Build   `json:"lastStableBuild"`
}

// Build has information about a specific build
type Build struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
}

// Job containers information about a job
type Job struct {
	Result    string `json:"result"`
	ID        string `json:"id"`
	Timestamp int    `json:"timestamp"`
}

// IsStable is really is success, but maybe there is a way to make it look
// at multiple runs...
func (j Job) IsStable() bool {
	return j.Result == "SUCCESS"
}

func (j *JenkinsClient) request(path string) ([]byte, error) {
	url := j.Host + path
	glog.V(3).Infof("Hitting: %s", url)
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s %d for %s", res.Status, res.StatusCode, url)
	}
	return ioutil.ReadAll(res.Body)
}

// GetConsoleLog downloads the logs for a particular job and build number
func (j *JenkinsClient) GetConsoleLog(name string, build int) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/job/%s/%d/consoleText", j.Host, name, build)
	glog.V(3).Infof("Hitting: %s", url)
	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("unexpected status: %s %d for %s", res.Status, res.StatusCode, url)
	}
	return res.Body, nil
}

// GetJob will get information about a single job
func (j *JenkinsClient) GetJob(name string) (*Queue, error) {
	data, err := j.request("/job/" + name + "/api/json")
	if err != nil {
		return nil, err
	}
	glog.V(8).Infof("Got data: %s", string(data))
	q := &Queue{}
	if err := json.Unmarshal(data, q); err != nil {
		return nil, err
	}
	return q, nil
}

// GetLastCompletedBuild does just that
func (j *JenkinsClient) GetLastCompletedBuild(name string) (*Job, error) {
	data, err := j.request("/job/" + name + "/lastCompletedBuild/api/json")
	if err != nil {
		return nil, err
	}
	glog.V(8).Infof("Got data: %s", string(data))
	job := &Job{}
	if err := json.Unmarshal(data, job); err != nil {
		return nil, err
	}
	return job, nil
}
