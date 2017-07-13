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
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/jenkins"
	"k8s.io/test-infra/prow/kube"
)

const (
	period = 30 * time.Second
)

type Job struct {
	Type        string            `json:"type"`
	Repo        string            `json:"repo"`
	Refs        string            `json:"refs"`
	BaseRef     string            `json:"base_ref"`
	BaseSHA     string            `json:"base_sha"`
	PullSHA     string            `json:"pull_sha"`
	Number      int               `json:"number"`
	Author      string            `json:"author"`
	Job         string            `json:"job"`
	BuildID     string            `json:"build_id"`
	Context     string            `json:"context"`
	Started     string            `json:"started"`
	Finished    string            `json:"finished"`
	Duration    string            `json:"duration"`
	State       string            `json:"state"`
	Description string            `json:"description"`
	URL         string            `json:"url"`
	PodName     string            `json:"pod_name"`
	Agent       kube.ProwJobAgent `json:"agent"`
	ProwJob     string            `json:"prow_job"`

	st time.Time
	ft time.Time
}

type JobAgent struct {
	kc        *kube.Client
	pkc       *kube.Client
	jc        *jenkins.Client
	jobs      []Job
	jobsMap   map[string]Job                     // pod name -> Job
	jobsIDMap map[string]map[string]kube.ProwJob // job name -> id -> ProwJob
	mut       sync.Mutex
}

func (ja *JobAgent) Start() {
	ja.tryUpdate()
	go func() {
		t := time.Tick(period)
		for range t {
			ja.tryUpdate()
		}
	}()
}

func (ja *JobAgent) Jobs() []Job {
	ja.mut.Lock()
	defer ja.mut.Unlock()
	res := make([]Job, len(ja.jobs))
	copy(res, ja.jobs)
	return res
}

var jobNameRE = regexp.MustCompile(`^([\w-]+)-(\d+)$`)

// TODO(#3402): Remove this.
func (ja *JobAgent) GetLog(name string) ([]byte, error) {
	ja.mut.Lock()
	job, ok := ja.jobsMap[name]
	ja.mut.Unlock() // unlock now-- getting the log takes a while!
	if !ok {
		return nil, fmt.Errorf("no such job %s", name)
	}
	if job.Agent == kube.KubernetesAgent {
		// running on Kubernetes
		return ja.pkc.GetLog(name)
	} else if ja.jc != nil && job.Agent == kube.JenkinsAgent {
		// running on Jenkins
		m := jobNameRE.FindStringSubmatch(name)
		if m == nil {
			return nil, fmt.Errorf("invalid job name %s", name)
		}
		number, err := strconv.Atoi(m[2])
		if err != nil {
			return nil, err
		}
		return ja.jc.GetLog(m[1], number)
	}
	return nil, fmt.Errorf("cannot get log for %s", name)
}

func (ja *JobAgent) GetJobLog(job, id string) ([]byte, error) {
	var j kube.ProwJob
	ja.mut.Lock()
	idMap, ok := ja.jobsIDMap[job]
	if ok {
		j, ok = idMap[id]
	}
	ja.mut.Unlock()
	if !ok {
		return nil, fmt.Errorf("no such job %s %s", job, id)
	}
	if j.Spec.Agent == kube.KubernetesAgent {
		return ja.pkc.GetLog(j.Status.PodName)
	}
	num, err := strconv.Atoi(id)
	if err != nil {
		return nil, err
	}
	return ja.jc.GetLog(job, num)
}

func (ja *JobAgent) tryUpdate() {
	if err := ja.update(); err != nil {
		logrus.WithError(err).Warning("Error updating job list.")
	}
}

type byStartTime []Job

func (a byStartTime) Len() int           { return len(a) }
func (a byStartTime) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byStartTime) Less(i, j int) bool { return a[i].st.After(a[j].st) }

func (ja *JobAgent) update() error {
	pjs, err := ja.kc.ListProwJobs(nil)
	if err != nil {
		return err
	}
	var njs []Job
	njsMap := make(map[string]Job)
	njsIDMap := make(map[string]map[string]kube.ProwJob)
	for _, j := range pjs {
		buildID := j.Status.BuildID
		if j.Spec.Agent == kube.JenkinsAgent {
			buildID = j.Status.JenkinsBuildID
		}
		nj := Job{
			Type:    string(j.Spec.Type),
			Repo:    fmt.Sprintf("%s/%s", j.Spec.Refs.Org, j.Spec.Refs.Repo),
			Refs:    j.Spec.Refs.String(),
			BaseRef: j.Spec.Refs.BaseRef,
			BaseSHA: j.Spec.Refs.BaseSHA,
			Job:     j.Spec.Job,
			Context: j.Spec.Context,
			Agent:   j.Spec.Agent,
			ProwJob: j.Metadata.Name,
			BuildID: buildID,

			Started:     j.Status.StartTime.Format(time.Stamp),
			State:       string(j.Status.State),
			Description: j.Status.Description,
			PodName:     j.Status.PodName,
			URL:         j.Status.URL,

			st: j.Status.StartTime,
			ft: j.Status.CompletionTime,
		}
		if !nj.ft.IsZero() {
			nj.Finished = nj.ft.Format("15:04:05")
			duration := nj.ft.Sub(nj.st)
			duration -= duration % time.Second // strip fractional seconds
			nj.Duration = duration.String()
		}
		if len(j.Spec.Refs.Pulls) == 1 {
			nj.Number = j.Spec.Refs.Pulls[0].Number
			nj.Author = j.Spec.Refs.Pulls[0].Author
			nj.PullSHA = j.Spec.Refs.Pulls[0].SHA
		}
		njs = append(njs, nj)
		if nj.PodName != "" {
			njsMap[nj.PodName] = nj
		}
		if _, ok := njsIDMap[j.Spec.Job]; !ok {
			njsIDMap[j.Spec.Job] = make(map[string]kube.ProwJob)
		}
		njsIDMap[j.Spec.Job][buildID] = j
	}
	sort.Sort(byStartTime(njs))

	ja.mut.Lock()
	defer ja.mut.Unlock()
	ja.jobs = njs
	ja.jobsMap = njsMap
	return nil
}
