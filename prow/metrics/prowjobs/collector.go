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
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
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
	histogramVec := newGenericProwJobHistogramVec()
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

func newGenericProwJobHistogramVec() *prometheus.HistogramVec {
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

func newPlankProwJobHistogramVec() *prometheus.HistogramVec {
	return prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "plank_pod_runtime_seconds",
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
			// state of the prowjob pod: scheduled, initialized, run, postprocessed, succeeded, failed, unknown
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

func updatePlank(histogramVec *prometheus.HistogramVec, jobInformer cache.SharedIndexInformer, oldPod *v1.Pod, newPod *v1.Pod) {
	uid := newPod.ObjectMeta.Labels["prow.k8s.io/id"]
	if uid == "" {
		return
	}
	jobs, err := jobInformer.GetIndexer().Index("uid", uid)
	if err != nil {
		logrus.WithError(err).Error("Failed to fetch prowJob from informer")
		return
	}
	if len(jobs) == 0 {
		return
	}

	if len(jobs) > 1 {
		logrus.Error("Found more than one prowjob for pod")
		return
	}
	job := jobs[0].(*prowapi.ProwJob)

	if job.Status.PodName != newPod.Name {
		// we don't care about this pod anymore
		return
	}

	if newPod.DeletionTimestamp != nil {
		// TODO, we will have to check for this on the ProwJob
		return
	}

	oldState, oldTime := determineState(oldPod)
	newState, newTime := determineState(newPod)

	if oldState == newState {
		return
	}

	labels := getPlankJobLabel(job, newState)
	histogram, err := histogramVec.GetMetricWithLabelValues(labels.values()...)
	if err != nil {
		logrus.WithError(err).Error("Failed to get a histogram for a prowjob")
		return
	}
	histogram.Observe(newTime.Sub(oldTime.Time).Seconds())
}

func determineState(pod *v1.Pod) (state string, timestamp metav1.Time) {
	state = "scheduling"
	timestamp = pod.CreationTimestamp
	if scheduledCondition := getCondition(pod, v1.PodScheduled); scheduledCondition != nil {
		state = "initializing"
		timestamp = scheduledCondition.LastTransitionTime
		if initContainerStatus := getInitContainerTerminationStatus(pod, "initupload"); initContainerStatus != nil {
			timestamp = initContainerStatus.FinishedAt
			if initContainerStatus.ExitCode == 0 {
				state = "running"
			} else {
				state = "init_failed"
			}
		}
		if testContainerStatus := getContainerTerminationStatus(pod, "test"); testContainerStatus != nil {
			timestamp = testContainerStatus.FinishedAt
			if testContainerStatus.ExitCode == 0 {
				state = "succeeded"
			} else {
				state = "failed"
			}
			if sidecarStatus := getContainerTerminationStatus(pod, "sidecar"); sidecarStatus != nil {
				timestamp = sidecarStatus.FinishedAt
				if sidecarStatus.ExitCode == 0 {
					state = "postprocessing_succeeded"
				} else {
					state = "postprocessing_failed"
				}
			}
		}
	}
	return
}

// NewPlankJobLifecycleHistogramVec creates histograms which can track the timespan between ProwJob state transitions when plank is the agent.
// The histograms are based on the job name, the old job state and the new job state.
// Data is collected by hooking itself into the prowjob informer and checking pending substates from pods, which belong to the job.
// The collector will never record the same state transition twice, even if reboots happen.
func NewPlankJobLifecycleHistogramVec(jobInformer cache.SharedIndexInformer, podInformer cache.SharedIndexInformer) *prometheus.HistogramVec {
	jobInformer.AddIndexers(map[string]cache.IndexFunc{"uid": MetaUIDIndexFunc})
	histogramVec := newPlankProwJobHistogramVec()
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(oldPod, newPod interface{}) {
			updatePlank(histogramVec, jobInformer, oldPod.(*v1.Pod), newPod.(*v1.Pod))
		},
	})
	return histogramVec
}

func MetaUIDIndexFunc(obj interface{}) ([]string, error) {
	meta, err := meta.Accessor(obj)
	if err != nil {
		return []string{""}, fmt.Errorf("object has no meta: %v", err)
	}
	return []string{string(meta.GetUID())}, nil
}

func getCondition(pod *v1.Pod, conditionType v1.PodConditionType) *v1.PodCondition {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

func getContainerTerminationStatus(pod *v1.Pod, containerName string) *v1.ContainerStateTerminated {
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Name == containerName {
			return containerStatus.State.Terminated
		}
	}
	return nil
}
func getInitContainerTerminationStatus(pod *v1.Pod, containerName string) *v1.ContainerStateTerminated {
	for _, containerStatus := range pod.Status.InitContainerStatuses {
		if containerStatus.Name == containerName {
			return containerStatus.State.Terminated
		}
	}
	return nil
}

func getPlankJobLabel(newJob *prowapi.ProwJob, state string) jobLabel {
	jl := jobLabel{
		jobNamespace: newJob.Namespace,
		jobName:      newJob.Spec.Job,
		jobType:      string(newJob.Spec.Type),
		state:        state,
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
