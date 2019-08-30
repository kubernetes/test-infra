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
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	coreapi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
)

const (
	period = 30 * time.Second
)

var (
	errProwjobNotFound = errors.New("prowjob not found")
)

func IsErrProwJobNotFound(err error) bool {
	return err == errProwjobNotFound
}

type serviceClusterClient interface {
	ListProwJobs(selector string) ([]prowapi.ProwJob, error)
}

// PodLogClient is an interface for interacting with the pod logs.
type PodLogClient interface {
	GetLogs(name string, opts *coreapi.PodLogOptions) ([]byte, error)
}

// NewJobAgent is a JobAgent constructor.
func NewJobAgent(kc serviceClusterClient, plClients map[string]PodLogClient, cfg config.Getter) *JobAgent {
	return &JobAgent{
		kc:     kc,
		pkcs:   plClients,
		config: cfg,
	}
}

// JobAgent creates lists of jobs, updates their status and returns their run logs.
type JobAgent struct {
	kc        serviceClusterClient
	pkcs      map[string]PodLogClient
	config    config.Getter
	prowJobs  []prowapi.ProwJob
	jobsIDMap map[string]map[string]prowapi.ProwJob // job name -> id -> ProwJob
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

// ProwJobs returns a thread-safe snapshot of the current prow jobs.
func (ja *JobAgent) ProwJobs() []prowapi.ProwJob {
	ja.mut.Lock()
	defer ja.mut.Unlock()
	res := make([]prowapi.ProwJob, len(ja.prowJobs))
	copy(res, ja.prowJobs)
	return res
}

var jobNameRE = regexp.MustCompile(`^([\w-]+)-(\d+)$`)

// GetProwJob finds the corresponding Prowjob resource from the provided job name and build ID
func (ja *JobAgent) GetProwJob(job, id string) (prowapi.ProwJob, error) {
	if ja == nil {
		return prowapi.ProwJob{}, fmt.Errorf("Prow job agent doesn't exist (are you running locally?)")
	}
	var j prowapi.ProwJob
	ja.mut.Lock()
	idMap, ok := ja.jobsIDMap[job]
	if ok {
		j, ok = idMap[id]
	}
	ja.mut.Unlock()
	if !ok {
		return prowapi.ProwJob{}, errProwjobNotFound
	}
	return j, nil
}

// GetJobLog returns the job logs, works for both kubernetes and jenkins agent types.
func (ja *JobAgent) GetJobLog(job, id string) ([]byte, error) {
	j, err := ja.GetProwJob(job, id)
	if err != nil {
		return nil, fmt.Errorf("error getting prowjob: %v", err)
	}
	if j.Spec.Agent == prowapi.KubernetesAgent {
		client, ok := ja.pkcs[j.ClusterAlias()]
		if !ok {
			return nil, fmt.Errorf("cannot get logs for prowjob %q with agent %q: unknown cluster alias %q", j.ObjectMeta.Name, j.Spec.Agent, j.ClusterAlias())
		}
		return client.GetLogs(j.Status.PodName, &coreapi.PodLogOptions{Container: kube.TestContainerName})
	}
	for _, agentToTmpl := range ja.config().Deck.ExternalAgentLogs {
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

type byPJStartTime []prowapi.ProwJob

func (a byPJStartTime) Len() int      { return len(a) }
func (a byPJStartTime) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byPJStartTime) Less(i, j int) bool {
	return a[i].Status.StartTime.Time.After(a[j].Status.StartTime.Time)
}

func (ja *JobAgent) update() error {
	pjs, err := ja.kc.ListProwJobs(labels.Everything().String())
	if err != nil {
		return err
	}
	pjsIDMap := make(map[string]map[string]prowapi.ProwJob)
	for _, j := range pjs {
		buildID := j.Status.BuildID

		if _, ok := pjsIDMap[j.Spec.Job]; !ok {
			pjsIDMap[j.Spec.Job] = make(map[string]prowapi.ProwJob)
		}

		pjsIDMap[j.Spec.Job][buildID] = j
	}

	sort.Sort(byPJStartTime(pjs))

	ja.mut.Lock()
	defer ja.mut.Unlock()
	ja.prowJobs = pjs
	ja.jobsIDMap = pjsIDMap
	return nil
}
