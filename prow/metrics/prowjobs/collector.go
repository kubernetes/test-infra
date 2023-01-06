/*
Copyright 2019 The Kubernetes Authors.

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

package prowjobs

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/tools/cache"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func update(histogramVec *prometheus.HistogramVec, oldJob *prowapi.ProwJob, newJob *prowapi.ProwJob) {
	if oldJob == nil || oldJob.Status.State == newJob.Status.State {
		return
	}

	var oldTime *metav1.Time
	var newTime *metav1.Time

	switch oldJob.Status.State {
	case prowapi.TriggeredState:
		oldTime = &oldJob.CreationTimestamp
	case prowapi.PendingState:
		oldTime = oldJob.Status.PendingTime
	}

	switch newJob.Status.State {
	case prowapi.FailureState, prowapi.SuccessState, prowapi.ErrorState, prowapi.AbortedState:
		newTime = newJob.Status.CompletionTime
	case prowapi.PendingState:
		newTime = newJob.Status.PendingTime
	}

	if oldTime == nil || newTime == nil {
		return
	}

	labels := getJobLabel(oldJob, newJob)
	histogram, err := histogramVec.GetMetricWithLabelValues(labels.values()...)
	if err != nil {
		logrus.WithError(err).Error("Failed to get a histogram for a prowjob")
		return
	}
	histogram.Observe(newTime.Sub(oldTime.Time).Seconds())
}

// NewProwJobLifecycleHistogramVec creates histograms which can track the timespan between ProwJob state transitions.
// The histograms are based on the job name, the old job state and the new job state.
// Data is collected by hooking itself into the prowjob informer.
// The collector will never record the same state transition twice, even if reboots happen.
func NewProwJobLifecycleHistogramVec(informer cache.SharedIndexInformer) *prometheus.HistogramVec {
	histogramVec := newHistogramVec()
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldJob, newJob interface{}) {
			update(histogramVec, oldJob.(*prowapi.ProwJob), newJob.(*prowapi.ProwJob))
		},
	})
	return histogramVec
}

func getJobLabel(oldJob *prowapi.ProwJob, newJob *prowapi.ProwJob) jobLabel {
	jl := jobLabel{
		jobNamespace: newJob.Namespace,
		jobName:      newJob.Spec.Job,
		jobType:      string(newJob.Spec.Type),
		state:        string(newJob.Status.State),
		last_state:   string(oldJob.Status.State),
	}

	if newJob.Spec.Refs != nil {
		jl.org = newJob.Spec.Refs.Org
		jl.repo = newJob.Spec.Refs.Repo
		jl.baseRef = newJob.Spec.Refs.BaseRef
	} else if len(newJob.Spec.ExtraRefs) > 0 {
		jl.org = newJob.Spec.ExtraRefs[0].Org
		jl.repo = newJob.Spec.ExtraRefs[0].Repo
		jl.baseRef = newJob.Spec.ExtraRefs[0].BaseRef
	}

	return jl
}

type jobLabel struct {
	jobNamespace string
	jobName      string
	jobType      string
	last_state   string
	state        string
	org          string
	repo         string
	baseRef      string
}

func (jl *jobLabel) values() []string {
	return []string{jl.jobNamespace, jl.jobName, jl.jobType, jl.last_state, jl.state, jl.org, jl.repo, jl.baseRef}
}

func newHistogramVec() *prometheus.HistogramVec {
	return prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "prow_job_runtime_seconds",
			Buckets: []float64{
				time.Minute.Seconds() / 2,
				(1 * time.Minute).Seconds(),
				(2 * time.Minute).Seconds(),
				(5 * time.Minute).Seconds(),
				(10 * time.Minute).Seconds(),
				(1 * time.Hour).Seconds() / 2,
				(1 * time.Hour).Seconds(),
				(2 * time.Hour).Seconds(),
				(3 * time.Hour).Seconds(),
				(4 * time.Hour).Seconds(),
				(5 * time.Hour).Seconds(),
				(6 * time.Hour).Seconds(),
				(7 * time.Hour).Seconds(),
				(8 * time.Hour).Seconds(),
				(9 * time.Hour).Seconds(),
				(10 * time.Hour).Seconds(),
			},
		},
		[]string{
			// namespace of the job
			"job_namespace",
			// name of the job
			"job_name",
			// type of the prowjob: presubmit, postsubmit, periodic, batch
			"type",
			// last state of the prowjob: triggered, pending, success, failure, aborted, error
			"last_state",
			// state of the prowjob: triggered, pending, success, failure, aborted, error
			"state",
			// the org of the prowjob's repo
			"org",
			// the prowjob's repo
			"repo",
			// the base_ref of the prowjob's repo
			"base_ref",
		},
	)
}
