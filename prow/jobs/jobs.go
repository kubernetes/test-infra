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

// Presubmit is the job-specific trigger info.
type Presubmit struct {
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
	// Only run against these branches. Default is all branches.
	Branches []string `json:"branches"`
	// Kubernetes pod spec.
	Spec *kube.PodSpec `json:"spec,omitempty"`

	// We'll set this when we load it.
	re *regexp.Regexp
}

func (ps Presubmit) RunsAgainstBranch(branch string) bool {
	if len(ps.Branches) == 0 {
		return true
	}
	for _, b := range ps.Branches {
		if b == branch {
			return true
		}
	}
	return false
}

// Postsubmit runs on a timer.
type Postsubmit struct {
	Name string        `json:"name"`
	Spec *kube.PodSpec `json:"spec,omitempty"`
}

type JobAgent struct {
	mut sync.Mutex
	// Repo FullName (eg "kubernetes/kubernetes") -> []Job
	presubmits  map[string][]Presubmit
	postsubmits map[string][]Postsubmit
}

func (ja *JobAgent) Start(pre, post string) error {
	if err := ja.LoadOnce(pre, post); err != nil {
		return err
	}
	ticker := time.Tick(1 * time.Minute)
	go func() {
		for range ticker {
			ja.tryLoad(pre, post)
		}
	}()
	return nil
}

func (ja *JobAgent) SetPresubmits(jobs map[string][]Presubmit) error {
	ja.mut.Lock()
	defer ja.mut.Unlock()
	nj := map[string][]Presubmit{}
	for k, v := range jobs {
		nj[k] = make([]Presubmit, len(v))
		copy(nj[k], v)
		for i := range v {
			if re, err := regexp.Compile(v[i].Trigger); err != nil {
				return err
			} else {
				nj[k][i].re = re
			}
		}
	}
	ja.presubmits = nj
	return nil
}

func (ja *JobAgent) LoadOnce(pre, post string) error {
	ja.mut.Lock()
	defer ja.mut.Unlock()
	if err := ja.loadPresubmits(pre); err != nil {
		return err
	}
	return ja.loadPostsubmits(post)
}

func (ja *JobAgent) MatchingPresubmits(fullRepoName, body string, testAll *regexp.Regexp) []Presubmit {
	ja.mut.Lock()
	defer ja.mut.Unlock()
	var result []Presubmit
	ott := testAll.MatchString(body)
	if jobs, ok := ja.presubmits[fullRepoName]; ok {
		for _, job := range jobs {
			if job.re.MatchString(body) || (ott && job.AlwaysRun) {
				result = append(result, job)
			}
		}
	}
	return result
}

func (ja *JobAgent) AllPresubmits(fullRepoName string) []Presubmit {
	ja.mut.Lock()
	defer ja.mut.Unlock()
	res := make([]Presubmit, len(ja.presubmits[fullRepoName]))
	copy(res, ja.presubmits[fullRepoName])
	return res
}

func (ja *JobAgent) GetPresubmit(repo, job string) (bool, Presubmit) {
	ja.mut.Lock()
	defer ja.mut.Unlock()
	for _, j := range ja.presubmits[repo] {
		if j.Name == job {
			return true, j
		}
	}
	return false, Presubmit{}
}

func (ja *JobAgent) AllPostsubmits(fullRepoName string) []Postsubmit {
	ja.mut.Lock()
	defer ja.mut.Unlock()
	res := make([]Postsubmit, len(ja.postsubmits[fullRepoName]))
	copy(res, ja.postsubmits[fullRepoName])
	return res
}

func (ja *JobAgent) GetPostsubmit(repo, job string) (bool, Postsubmit) {
	ja.mut.Lock()
	defer ja.mut.Unlock()
	for _, j := range ja.postsubmits[repo] {
		if j.Name == job {
			return true, j
		}
	}
	return false, Postsubmit{}
}

// Hold the lock.
func (ja *JobAgent) loadPresubmits(path string) error {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	nj := map[string][]Presubmit{}
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
	ja.presubmits = nj
	return nil
}

// Hold the lock.
func (ja *JobAgent) loadPostsubmits(path string) error {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	nj := map[string][]Postsubmit{}
	if err := yaml.Unmarshal(b, &nj); err != nil {
		return err
	}
	ja.postsubmits = nj
	return nil
}

func (ja *JobAgent) tryLoad(pre, post string) {
	ja.mut.Lock()
	defer ja.mut.Unlock()
	if err := ja.loadPresubmits(pre); err != nil {
		logrus.WithField("path", pre).WithError(err).Error("Error loading presubmits.")
	}
	if err := ja.loadPostsubmits(post); err != nil {
		logrus.WithField("path", post).WithError(err).Error("Error loading postsubmits.")
	}
}
