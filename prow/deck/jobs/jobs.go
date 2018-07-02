/*
Copyright 2018 The Kubernetes Authors.

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

// Package jobs implements methods on job information used by Prow component deck
package jobs

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
)

const (
	period = 30 * time.Second
)

// Job holds information about a job prow is running/has run.
// TODO(#5216): Remove this, and all associated machinery.
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

type serviceClusterClient interface {
	GetLog(pod string) ([]byte, error)
	ListPods(selector string) ([]kube.Pod, error)
	ListProwJobs(selector string) ([]kube.ProwJob, error)
}

type PodLogClient interface {
	GetLog(pod string) ([]byte, error)
}

type ConfigAgent interface {
	Config() *config.Config
}

// NewJobAgent is a JobAgent constructor
func NewJobAgent(kc serviceClusterClient, plClients map[string]PodLogClient, ca ConfigAgent) *JobAgent {
	return &JobAgent{
		kc:   kc,
		pkcs: plClients,
		c:    ca,
	}
}

// JobAgent creates lists of jobs, updates their status and returns their run logs.
type JobAgent struct {
	kc        serviceClusterClient
	pkcs      map[string]PodLogClient
	c         ConfigAgent
	prowJobs  []kube.ProwJob
	jobs      []Job
	jobsMap   map[string]Job                     // pod name -> Job
	jobsIDMap map[string]map[string]kube.ProwJob // job name -> id -> ProwJob
	mut       sync.Mutex
}

// Start will start the job and periodically update it.
func (ja *JobAgent) Start() {
	ja.tryUpdate()
	go func() {
		t := time.Tick(period)
		for range t {
			ja.tryUpdate()
		}
	}()
}

// Jobs returns a thread-safe snapshot of the current job state.
func (ja *JobAgent) Jobs() []Job {
	ja.mut.Lock()
	defer ja.mut.Unlock()
	res := make([]Job, len(ja.jobs))
	copy(res, ja.jobs)
	return res
}

// ProwJobs returns a thread-safe snapshot of the current prow jobs.
func (ja *JobAgent) ProwJobs() []kube.ProwJob {
	ja.mut.Lock()
	defer ja.mut.Unlock()
	res := make([]kube.ProwJob, len(ja.prowJobs))
	copy(res, ja.prowJobs)
	return res
}

var jobNameRE = regexp.MustCompile(`^([\w-]+)-(\d+)$`)

// GetJobLog returns the job logs, works for both kubernetes and jenkins agent types.
func (ja *JobAgent) GetJobLog(job, id string) ([]byte, error) {
	var j kube.ProwJob
	ja.mut.Lock()
	idMap, ok := ja.jobsIDMap[job]
	if ok {
		j, ok = idMap[id]
	}
	ja.mut.Unlock()
	if !ok {
		return nil, fmt.Errorf("no such job found: %s (id: %s)", job, id)
	}
	if j.Spec.Agent == kube.KubernetesAgent {
		client, ok := ja.pkcs[j.ClusterAlias()]
		if !ok {
			return nil, fmt.Errorf("cannot get logs for prowjob %q with agent %q: unknown cluster alias %q", j.ObjectMeta.Name, j.Spec.Agent, j.ClusterAlias())
		}
		return client.GetLog(j.Status.PodName)
	}
	for _, agentToTmpl := range ja.c.Config().Deck.ExternalAgentLogs {
		if agentToTmpl.Agent != string(j.Spec.Agent) {
			continue
		}
		if !agentToTmpl.Selector.Matches(labels.Set(j.ObjectMeta.Labels)) {
			continue
		}
		var b bytes.Buffer
		if err := agentToTmpl.URLTemplate.Execute(&b, &j); err != nil {
			return nil, fmt.Errorf("cannot execute URL template for prowjob %q with agent %q: %v", j.ObjectMeta.Name, j.Spec.Agent, err)
		}
		resp, err := http.Get(b.String())
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		return ioutil.ReadAll(resp.Body)
	}
	return nil, fmt.Errorf("cannot get logs for prowjob %q with agent %q: the agent is missing from the prow config file", j.ObjectMeta.Name, j.Spec.Agent)
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
	pjs, err := ja.kc.ListProwJobs(kube.EmptySelector)
	if err != nil {
		return err
	}
	var njs []Job
	njsMap := make(map[string]Job)
	njsIDMap := make(map[string]map[string]kube.ProwJob)
	for _, j := range pjs {
		ft := time.Time{}
		if j.Status.CompletionTime != nil {
			ft = j.Status.CompletionTime.Time
		}
		buildID := j.Status.BuildID
		nj := Job{
			Type:    string(j.Spec.Type),
			Job:     j.Spec.Job,
			Context: j.Spec.Context,
			Agent:   j.Spec.Agent,
			ProwJob: j.ObjectMeta.Name,
			BuildID: buildID,

			Started:     fmt.Sprintf("%d", j.Status.StartTime.Time.Unix()),
			State:       string(j.Status.State),
			Description: j.Status.Description,
			PodName:     j.Status.PodName,
			URL:         j.Status.URL,

			st: j.Status.StartTime.Time,
			ft: ft,
		}
		if !nj.ft.IsZero() {
			nj.Finished = nj.ft.Format(time.RFC3339Nano)
			duration := nj.ft.Sub(nj.st)
			duration -= duration % time.Second // strip fractional seconds
			nj.Duration = duration.String()
		}
		if j.Spec.Refs != nil {
			nj.Repo = fmt.Sprintf("%s/%s", j.Spec.Refs.Org, j.Spec.Refs.Repo)
			nj.Refs = j.Spec.Refs.String()
			nj.BaseRef = j.Spec.Refs.BaseRef
			nj.BaseSHA = j.Spec.Refs.BaseSHA
			if len(j.Spec.Refs.Pulls) == 1 {
				nj.Number = j.Spec.Refs.Pulls[0].Number
				nj.Author = j.Spec.Refs.Pulls[0].Author
				nj.PullSHA = j.Spec.Refs.Pulls[0].SHA
			}
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
	ja.prowJobs = pjs
	ja.jobs = njs
	ja.jobsMap = njsMap
	ja.jobsIDMap = njsIDMap
	return nil
}
