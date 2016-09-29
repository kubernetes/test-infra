/*
Copyright 2016 The Kubernetes Authors.

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

package main

import (
	"io/ioutil"
	"regexp"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// JenkinsJob is the job-specific trigger info.
type JenkinsJob struct {
	// eg kubernetes-pull-build-test-e2e-gce
	Name string `yaml:"name"`
	// Run for every PR, or only when a comment triggers it.
	AlwaysRun bool `yaml:"always_run"`
	// Context line for GitHub status.
	Context string `yaml:"context"`
	// eg @k8s-bot e2e test this
	Trigger string `yaml:"trigger"`
	// Valid rerun command to give users. Must match Trigger.
	RerunCommand string `yaml:"rerun_command"`

	// We'll set this when we load it. "-" means ignore.
	re *regexp.Regexp `yaml:"-"`
}

type JobAgent struct {
	mut sync.Mutex
	// Repo FullName (eg "kubernetes/kubernetes") -> []JenkinsJob
	jobs map[string][]JenkinsJob
}

func (ja *JobAgent) Start(path string) {
	ja.tryLoad(path)
	ticker := time.Tick(1 * time.Minute)
	go func() {
		for range ticker {
			ja.tryLoad(path)
		}
	}()
}

func (ja *JobAgent) MatchingJobs(fullRepoName, body string) []JenkinsJob {
	ja.mut.Lock()
	defer ja.mut.Unlock()
	var result []JenkinsJob
	ott := okToTest.MatchString(body)
	if jobs, ok := ja.jobs[fullRepoName]; ok {
		for _, job := range jobs {
			if job.re.MatchString(body) || (ott && job.AlwaysRun) {
				result = append(result, job)
			}
		}
	}
	return result
}

func (ja *JobAgent) AllJobs(fullRepoName string) []JenkinsJob {
	ja.mut.Lock()
	defer ja.mut.Unlock()
	res := make([]JenkinsJob, len(ja.jobs[fullRepoName]))
	copy(res, ja.jobs[fullRepoName])
	return res
}

// Hold the lock.
func (ja *JobAgent) load(path string) error {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	nj := map[string][]JenkinsJob{}
	if err := yaml.Unmarshal(b, &nj); err != nil {
		return err
	}
	for k, v := range nj {
		for i, j := range v {
			if re, err := regexp.Compile(j.Trigger); err == nil {
				nj[k][i].re = re
			} else {
				return err
			}
		}
	}
	ja.jobs = nj
	return nil
}

func (ja *JobAgent) tryLoad(path string) {
	ja.mut.Lock()
	defer ja.mut.Unlock()
	if err := ja.load(path); err != nil {
		logrus.WithField("path", path).WithError(err).Error("Error loading config.")
	}
}

var okToTest = regexp.MustCompile(`(?m)^(@k8s-bot )?ok to test\r?$`)
