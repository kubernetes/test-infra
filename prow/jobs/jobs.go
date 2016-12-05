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

package jobs

import (
	"io/ioutil"
	"regexp"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/ghodss/yaml"

	"k8s.io/test-infra/prow/kube"
)

// JenkinsJob is the job-specific trigger info.
type JenkinsJob struct {
	// eg kubernetes-pull-build-test-e2e-gce
	Name string `json:"name"`
	// Run for every PR, or only when a comment triggers it.
	AlwaysRun bool `json:"always_run"`
	// Context line for GitHub status.
	Context string `json:"context"`
	// eg @k8s-bot e2e test this
	Trigger string `json:"trigger"`
	// Valid rerun command to give users. Must match Trigger.
	RerunCommand string `json:"rerun_command"`
	// Whether or not to skip commenting and setting status on GitHub.
	SkipReport bool `json:"skip_report"`
	// Kubernetes pod spec.
	Spec *kube.PodSpec `json:"spec,omitempty"`

	// We'll set this when we load it.
	re *regexp.Regexp
}

type JobAgent struct {
	mut sync.Mutex
	// Repo FullName (eg "kubernetes/kubernetes") -> []JenkinsJob
	jobs map[string][]JenkinsJob
}

func (ja *JobAgent) Start(path string) error {
	if err := ja.LoadOnce(path); err != nil {
		return err
	}
	ticker := time.Tick(1 * time.Minute)
	go func() {
		for range ticker {
			ja.tryLoad(path)
		}
	}()
	return nil
}

func (ja *JobAgent) SetJobs(jobs map[string][]JenkinsJob) error {
	ja.mut.Lock()
	defer ja.mut.Unlock()
	nj := map[string][]JenkinsJob{}
	for k, v := range jobs {
		nj[k] = make([]JenkinsJob, len(v))
		copy(nj[k], v)
		for i := range v {
			if re, err := regexp.Compile(v[i].Trigger); err != nil {
				return err
			} else {
				nj[k][i].re = re
			}
		}
	}
	ja.jobs = nj
	return nil
}

func (ja *JobAgent) LoadOnce(path string) error {
	ja.mut.Lock()
	defer ja.mut.Unlock()
	return ja.load(path)
}

func (ja *JobAgent) MatchingJobs(fullRepoName, body string, testAll *regexp.Regexp) []JenkinsJob {
	ja.mut.Lock()
	defer ja.mut.Unlock()
	var result []JenkinsJob
	ott := testAll.MatchString(body)
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

func (ja *JobAgent) GetJob(repo, job string) (bool, JenkinsJob) {
	ja.mut.Lock()
	defer ja.mut.Unlock()
	for _, j := range ja.jobs[repo] {
		if j.Name == job {
			return true, j
		}
	}
	return false, JenkinsJob{}
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
