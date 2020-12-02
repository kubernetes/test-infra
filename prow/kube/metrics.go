/*
Copyright 2017 The Kubernetes Authors.

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

package kube

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/version"
)

var (
	metricLabels = []string{
		// namespace of the job
		"job_namespace",
		// name of the job
		"job_name",
		// type of the prowjob: presubmit, postsubmit, periodic, batch
		"type",
		// state of the prowjob: triggered, pending, success, failure, aborted, error
		"state",
		// the org of the prowjob's repo
		"org",
		// the prowjob's repo
		"repo",
		// the base_ref of the prowjob's repo
		"base_ref",
		// the cluster the job runs on
		"cluster",
	}
	prowJobs = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "prowjobs",
		Help: "Number of prowjobs in the system",
	}, metricLabels)
	prowJobTransitions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "prowjob_state_transitions",
		Help: "Number of prowjobs transitioning states",
	}, metricLabels)
	prowVersion = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prow_version",
		Help: "Prow version",
	})
)

type jobLabel struct {
	jobNamespace string
	jobName      string
	jobType      string
	state        string
	org          string
	repo         string
	baseRef      string
	cluster      string
}

func (jl *jobLabel) values() []string {
	return []string{jl.jobNamespace, jl.jobName, jl.jobType, jl.state, jl.org, jl.repo, jl.baseRef, jl.cluster}
}

func init() {
	prometheus.MustRegister(prowJobs)
	prometheus.MustRegister(prowJobTransitions)
	prometheus.MustRegister(prowVersion)
}

func getJobLabelMap(pjs []prowapi.ProwJob) map[jobLabel]float64 {
	jobLabelMap := make(map[jobLabel]float64)

	for _, pj := range pjs {
		jobLabelMap[getJobLabel(pj)]++
	}
	return jobLabelMap
}

func getJobLabel(pj prowapi.ProwJob) jobLabel {
	jl := jobLabel{jobNamespace: pj.Namespace, jobName: pj.Spec.Job, jobType: string(pj.Spec.Type), state: string(pj.Status.State), cluster: pj.Spec.Cluster}

	if pj.Spec.Refs != nil {
		jl.org = pj.Spec.Refs.Org
		jl.repo = pj.Spec.Refs.Repo
		jl.baseRef = pj.Spec.Refs.BaseRef
	} else if len(pj.Spec.ExtraRefs) > 0 {
		jl.org = pj.Spec.ExtraRefs[0].Org
		jl.repo = pj.Spec.ExtraRefs[0].Repo
		jl.baseRef = pj.Spec.ExtraRefs[0].BaseRef
	}

	return jl
}

type jobIdentifier struct {
	jobLabel
	buildId string
}

func getJobIdentifier(pj prowapi.ProwJob) jobIdentifier {
	return jobIdentifier{
		jobLabel: getJobLabel(pj),
		buildId:  pj.Status.BuildID,
	}
}

// previousStates records the prowJobs we were called with previously
var previousStates map[jobIdentifier]prowapi.ProwJobState

// GatherProwJobMetrics gathers prometheus metrics for prowjobs.
// Not threadsafe, ensure this is called serially.
func GatherProwJobMetrics(l *logrus.Entry, current []prowapi.ProwJob) {
	// This may be racing with the prometheus server but we need to remove
	// stale metrics like triggered or pending jobs that are now complete.
	prowJobs.Reset()

	// record prow version
	version, err := version.VersionTimestamp()
	if err != nil {
		// Not worth panicking
		l.WithError(err).Debug("Failed to get version timestamp")
		prowVersion.Set(-1)
	} else {
		prowVersion.Set(float64(version))
	}

	// record the current state of ProwJob CRs on the system
	for jl, count := range getJobLabelMap(current) {
		prowJobs.WithLabelValues(jl.values()...).Set(count)
	}

	// record state transitions since the last time we were called
	currentStates := map[jobIdentifier]prowapi.ProwJobState{}
	for _, pj := range current {
		ji := getJobIdentifier(pj)
		state := prowapi.ProwJobState(ji.state)
		currentStates[ji] = state

		if previousState, seen := previousStates[ji]; !seen || previousState != state {
			prowJobTransitions.WithLabelValues(ji.values()...).Inc()
		}
	}

	previousStates = currentStates
}
