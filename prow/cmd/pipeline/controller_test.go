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

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	pipelinev1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/test-infra/prow/config"

	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pod-utils/decorate"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"

	"knative.dev/pkg/apis"
	duckv1beta1 "knative.dev/pkg/apis/duck/v1beta1"
)

const (
	errorGetProwJob        = "error-get-prowjob"
	errorGetPipelineRun    = "error-get-pipeline"
	errorDeletePipelineRun = "error-delete-pipeline"
	errorCreatePipelineRun = "error-create-pipeline"
	errorUpdateProwJob     = "error-update-prowjob"
	pipelineID             = "123"
)

type fakeReconciler struct {
	jobs      map[string]prowjobv1.ProwJob
	pipelines map[string]pipelinev1beta1.PipelineRun
	nows      metav1.Time
}

func (r *fakeReconciler) now() metav1.Time {
	fmt.Println(r.nows)
	return r.nows
}

const fakePJCtx = "prow-context"
const fakePJNS = "prow-job"

func (r *fakeReconciler) getProwJob(name string) (*prowjobv1.ProwJob, error) {
	logrus.Debugf("getProwJob: name=%s", name)
	if name == errorGetProwJob {
		return nil, errors.New("injected get prowjob error")
	}
	k := toKey(fakePJCtx, fakePJNS, name)
	pj, present := r.jobs[k]
	if !present {
		return nil, apierrors.NewNotFound(prowjobv1.Resource("ProwJob"), name)
	}
	return &pj, nil
}

func (r *fakeReconciler) patchProwJob(pj *prowjobv1.ProwJob, newpj *prowjobv1.ProwJob) (*prowjobv1.ProwJob, error) {
	logrus.Debugf("patchProwJob: name=%s", pj.GetName())
	if pj.Name == errorUpdateProwJob {
		return nil, errors.New("injected update prowjob error")
	}
	if pj == nil {
		return nil, errors.New("nil prowjob")
	}
	k := toKey(fakePJCtx, fakePJNS, pj.Name)
	if _, present := r.jobs[k]; !present {
		return nil, apierrors.NewNotFound(prowjobv1.Resource("ProwJob"), pj.Name)
	}
	r.jobs[k] = *newpj
	return newpj, nil
}

func (r *fakeReconciler) getPipelineRun(context, namespace, name string) (*pipelinev1beta1.PipelineRun, error) {
	logrus.Debugf("getPipelineRun: ctx=%s, ns=%s, name=%s", context, namespace, name)
	if namespace == errorGetPipelineRun {
		return nil, errors.New("injected create pipeline error")
	}
	k := toKey(context, namespace, name)
	p, present := r.pipelines[k]
	if !present {
		return nil, apierrors.NewNotFound(pipelinev1beta1.Resource("PipelineRun"), name)
	}
	return &p, nil
}

func (r *fakeReconciler) deletePipelineRun(context, namespace, name string) error {
	logrus.Debugf("deletePipelineRun: ctx=%s, ns=%s, name=%s", context, namespace, name)
	if namespace == errorDeletePipelineRun {
		return errors.New("injected create pipeline error")
	}
	k := toKey(context, namespace, name)
	if _, present := r.pipelines[k]; !present {
		return apierrors.NewNotFound(pipelinev1beta1.Resource("PipelineRun"), name)
	}
	delete(r.pipelines, k)
	return nil
}

func (r *fakeReconciler) createPipelineRun(context, namespace string, p *pipelinev1beta1.PipelineRun) (*pipelinev1beta1.PipelineRun, error) {
	logrus.Debugf("createPipelineRun: ctx=%s, ns=%s", context, namespace)
	if p == nil {
		return nil, errors.New("nil pipeline")
	}
	if namespace == errorCreatePipelineRun {
		return nil, errors.New("injected create pipeline error")
	}
	k := toKey(context, namespace, p.Name)
	if _, alreadyExists := r.pipelines[k]; alreadyExists {
		return nil, apierrors.NewAlreadyExists(prowjobv1.Resource("ProwJob"), p.Name)
	}
	r.pipelines[k] = *p
	return p, nil
}

func (r *fakeReconciler) pipelineID(pj prowjobv1.ProwJob) (string, string, error) {
	return pipelineID, "", nil
}

type fakeLimiter struct {
	added string
}

func (fl *fakeLimiter) ShutDown() {}
func (fl *fakeLimiter) ShuttingDown() bool {
	return false
}
func (fl *fakeLimiter) Get() (interface{}, bool) {
	return "not implemented", true
}
func (fl *fakeLimiter) Done(interface{})   {}
func (fl *fakeLimiter) Forget(interface{}) {}
func (fl *fakeLimiter) AddRateLimited(a interface{}) {
	fl.added = a.(string)
}
func (fl *fakeLimiter) Add(a interface{}) {
	fl.added = a.(string)
}
func (fl *fakeLimiter) AddAfter(a interface{}, d time.Duration) {
	fl.added = a.(string)
}
func (fl *fakeLimiter) Len() int {
	return 0
}
func (fl *fakeLimiter) NumRequeues(item interface{}) int {
	return 0
}

func TestEnqueueKey(t *testing.T) {
	cases := []struct {
		name     string
		context  string
		obj      interface{}
		expected string
	}{
		{
			name:    "enqueue pipeline directly",
			context: "hey",
			obj: &pipelinev1beta1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Name:      "bar",
				},
			},
			expected: toKey("hey", "foo", "bar"),
		},
		{
			name:    "enqueue prowjob's spec namespace",
			context: "rolo",
			obj: &prowjobv1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "dude",
				},
				Spec: prowjobv1.ProwJobSpec{
					Namespace: "tomassi",
				},
			},
			expected: toKey("rolo", "tomassi", "dude"),
		},
		{
			name:    "ignore random object",
			context: "foo",
			obj:     "bar",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var fl fakeLimiter
			c := controller{
				workqueue: &fl,
			}
			c.enqueueKey(tc.context, tc.obj)
			if !reflect.DeepEqual(fl.added, tc.expected) {
				t.Errorf("%q != expected %q", fl.added, tc.expected)
			}
		})
	}
}

func TestReconcile(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)
	now := metav1.Now()
	pipelineSpec := pipelinev1beta1.PipelineRunSpec{}
	noJobChange := func(pj prowjobv1.ProwJob, _ pipelinev1beta1.PipelineRun) prowjobv1.ProwJob {
		return pj
	}
	noPipelineRunChange := func(_ prowjobv1.ProwJob, p pipelinev1beta1.PipelineRun) pipelinev1beta1.PipelineRun {
		return p
	}
	cases := []struct {
		name                string
		namespace           string
		context             string
		observedJob         *prowjobv1.ProwJob
		observedPipelineRun *pipelinev1beta1.PipelineRun
		expectedJob         func(prowjobv1.ProwJob, pipelinev1beta1.PipelineRun) prowjobv1.ProwJob
		expectedPipelineRun func(prowjobv1.ProwJob, pipelinev1beta1.PipelineRun) pipelinev1beta1.PipelineRun
		err                 bool
	}{
		{
			name: "new prow job creates pipeline",
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:           prowjobv1.TektonAgent,
					PipelineRunSpec: &pipelineSpec,
				},
				Status: prowjobv1.ProwJobStatus{
					BuildID: pipelineID,
				},
			},
			expectedJob: func(pj prowjobv1.ProwJob, _ pipelinev1beta1.PipelineRun) prowjobv1.ProwJob {
				pj.Status = prowjobv1.ProwJobStatus{
					StartTime:   now,
					PendingTime: &now,
					State:       prowjobv1.PendingState,
					Description: descScheduling,
					BuildID:     pipelineID,
				}
				return pj
			},
			expectedPipelineRun: func(pj prowjobv1.ProwJob, _ pipelinev1beta1.PipelineRun) pipelinev1beta1.PipelineRun {
				pj.Spec.Type = prowjobv1.PeriodicJob
				p, err := makePipelineRun(pj)
				if err != nil {
					panic(err)
				}
				return *p
			},
		},
		{
			name: "do not create pipeline run for failed prowjob",
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:           prowjobv1.TektonAgent,
					PipelineRunSpec: &pipelineSpec,
				},
				Status: prowjobv1.ProwJobStatus{
					State:   prowjobv1.FailureState,
					BuildID: pipelineID,
				},
			},
			expectedJob: noJobChange,
		},
		{
			name: "do not create pipeline run for successful prowjob",
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:           prowjobv1.TektonAgent,
					PipelineRunSpec: &pipelineSpec,
				},
				Status: prowjobv1.ProwJobStatus{
					State: prowjobv1.SuccessState,
				},
			},
			expectedJob: noJobChange,
		},
		{
			name: "do not create pipeline run for aborted prowjob",
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:           prowjobv1.TektonAgent,
					PipelineRunSpec: &pipelineSpec,
				},
				Status: prowjobv1.ProwJobStatus{
					State:   prowjobv1.AbortedState,
					BuildID: pipelineID,
				},
			},
			expectedJob: noJobChange,
		},
		{
			name: "delete pipeline run after deleting prowjob",
			observedPipelineRun: func() *pipelinev1beta1.PipelineRun {
				pj := prowjobv1.ProwJob{}
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.PipelineRunSpec = &pipelinev1beta1.PipelineRunSpec{}
				pj.Status.BuildID = pipelineID
				p, err := makePipelineRun(pj)
				if err != nil {
					panic(err)
				}
				return p
			}(),
		},
		{
			name: "do not delete deleted pipeline runs",
			observedPipelineRun: func() *pipelinev1beta1.PipelineRun {
				pj := prowjobv1.ProwJob{}
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.PipelineRunSpec = &pipelinev1beta1.PipelineRunSpec{}
				pj.Status.BuildID = pipelineID
				p, err := makePipelineRun(pj)
				p.DeletionTimestamp = &now
				if err != nil {
					panic(err)
				}
				return p
			}(),
			expectedPipelineRun: noPipelineRunChange,
		},
		{
			name: "only delete pipeline runs created by controller",
			observedPipelineRun: func() *pipelinev1beta1.PipelineRun {
				pj := prowjobv1.ProwJob{}
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.PipelineRunSpec = &pipelinev1beta1.PipelineRunSpec{}
				pj.Status.BuildID = pipelineID
				p, err := makePipelineRun(pj)
				if err != nil {
					panic(err)
				}
				delete(p.Labels, kube.CreatedByProw)
				return p
			}(),
			expectedPipelineRun: noPipelineRunChange,
		},
		{
			name:    "delete prow pipeline runs in the wrong cluster",
			context: "wrong-cluster",
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:   prowjobv1.TektonAgent,
					Cluster: "target-cluster",
					PipelineRunSpec: &pipelinev1beta1.PipelineRunSpec{
						ServiceAccountName: "robot",
					},
				},
				Status: prowjobv1.ProwJobStatus{
					State:       prowjobv1.PendingState,
					StartTime:   metav1.Now(),
					Description: "fancy",
				},
			},
			observedPipelineRun: func() *pipelinev1beta1.PipelineRun {
				pj := prowjobv1.ProwJob{}
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.Agent = prowjobv1.TektonAgent
				pj.Spec.PipelineRunSpec = &pipelineSpec
				pj.Status.BuildID = pipelineID
				p, err := makePipelineRun(pj)
				if err != nil {
					panic(err)
				}
				return p
			}(),
			expectedJob: noJobChange,
		},
		{
			name:    "ignore random pipeline run in the wrong cluster",
			context: "wrong-cluster",
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:   prowjobv1.TektonAgent,
					Cluster: "target-cluster",
					PipelineRunSpec: &pipelinev1beta1.PipelineRunSpec{
						ServiceAccountName: "robot",
					},
				},
				Status: prowjobv1.ProwJobStatus{
					State:       prowjobv1.PendingState,
					StartTime:   metav1.Now(),
					Description: "fancy",
				},
			},
			observedPipelineRun: func() *pipelinev1beta1.PipelineRun {
				pj := prowjobv1.ProwJob{}
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.Agent = prowjobv1.TektonAgent
				pj.Spec.PipelineRunSpec = &pipelineSpec
				pj.Status.BuildID = pipelineID
				p, err := makePipelineRun(pj)
				if err != nil {
					panic(err)
				}
				delete(p.Labels, kube.CreatedByProw)
				return p
			}(),
			expectedJob:         noJobChange,
			expectedPipelineRun: noPipelineRunChange,
		},
		{
			name: "update job status if pipeline run resets",
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent: prowjobv1.TektonAgent,
					PipelineRunSpec: &pipelinev1beta1.PipelineRunSpec{
						ServiceAccountName: "robot",
					},
				},
				Status: prowjobv1.ProwJobStatus{
					State:       prowjobv1.PendingState,
					StartTime:   metav1.Now(),
					Description: "fancy",
				},
			},
			observedPipelineRun: func() *pipelinev1beta1.PipelineRun {
				pj := prowjobv1.ProwJob{}
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.Agent = prowjobv1.TektonAgent
				pj.Spec.PipelineRunSpec = &pipelinev1beta1.PipelineRunSpec{
					ServiceAccountName: "robot",
				}
				pj.Status.BuildID = pipelineID
				p, err := makePipelineRun(pj)
				if err != nil {
					panic(err)
				}
				return p
			}(),
			expectedJob: func(pj prowjobv1.ProwJob, _ pipelinev1beta1.PipelineRun) prowjobv1.ProwJob {
				pj.Status.State = prowjobv1.PendingState
				pj.Status.Description = descScheduling
				return pj
			},
			expectedPipelineRun: noPipelineRunChange,
		},
		{
			name: "prowjob goes pending when pipeline run starts",
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:           prowjobv1.TektonAgent,
					PipelineRunSpec: &pipelineSpec,
				},
				Status: prowjobv1.ProwJobStatus{
					State:       prowjobv1.PendingState,
					Description: "fancy",
				},
			},
			observedPipelineRun: func() *pipelinev1beta1.PipelineRun {
				pj := prowjobv1.ProwJob{}
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.Agent = prowjobv1.TektonAgent
				pj.Spec.PipelineRunSpec = &pipelineSpec
				pj.Status.BuildID = pipelineID
				p, err := makePipelineRun(pj)
				if err != nil {
					panic(err)
				}
				p.Status.SetCondition(&apis.Condition{
					Type:    apis.ConditionReady,
					Message: "hello",
				})
				return p
			}(),
			expectedJob: func(pj prowjobv1.ProwJob, _ pipelinev1beta1.PipelineRun) prowjobv1.ProwJob {
				pj.Status = prowjobv1.ProwJobStatus{
					StartTime:   now,
					State:       prowjobv1.PendingState,
					Description: "scheduling",
				}
				return pj
			},
			expectedPipelineRun: noPipelineRunChange,
		},
		{
			name: "prowjob succeeds when run pipeline succeeds",
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:           prowjobv1.TektonAgent,
					PipelineRunSpec: &pipelineSpec,
				},
				Status: prowjobv1.ProwJobStatus{
					State:       prowjobv1.PendingState,
					Description: "fancy",
				},
			},
			observedPipelineRun: func() *pipelinev1beta1.PipelineRun {
				pj := prowjobv1.ProwJob{}
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.Agent = prowjobv1.TektonAgent
				pj.Spec.PipelineRunSpec = &pipelineSpec
				pj.Status.BuildID = pipelineID
				p, err := makePipelineRun(pj)
				if err != nil {
					panic(err)
				}
				p.Status.SetCondition(&apis.Condition{
					Type:    apis.ConditionSucceeded,
					Status:  corev1.ConditionTrue,
					Message: "hello",
				})
				return p
			}(),
			expectedJob: func(pj prowjobv1.ProwJob, _ pipelinev1beta1.PipelineRun) prowjobv1.ProwJob {
				pj.Status = prowjobv1.ProwJobStatus{
					StartTime:      now,
					CompletionTime: &now,
					State:          prowjobv1.SuccessState,
					Description:    "hello",
				}
				return pj
			},
			expectedPipelineRun: noPipelineRunChange,
		},
		{
			name: "prowjob fails when pipeline run fails",
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:           prowjobv1.TektonAgent,
					PipelineRunSpec: &pipelineSpec,
				},
				Status: prowjobv1.ProwJobStatus{
					State:       prowjobv1.PendingState,
					Description: "fancy",
				},
			},
			observedPipelineRun: func() *pipelinev1beta1.PipelineRun {
				pj := prowjobv1.ProwJob{}
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.Agent = prowjobv1.TektonAgent
				pj.Spec.PipelineRunSpec = &pipelineSpec
				pj.Status.BuildID = pipelineID
				p, err := makePipelineRun(pj)
				if err != nil {
					panic(err)
				}
				p.Status.SetCondition(&apis.Condition{
					Type:    apis.ConditionSucceeded,
					Status:  corev1.ConditionFalse,
					Message: "hello",
				})
				return p
			}(),
			expectedJob: func(pj prowjobv1.ProwJob, _ pipelinev1beta1.PipelineRun) prowjobv1.ProwJob {
				pj.Status = prowjobv1.ProwJobStatus{
					StartTime:      now,
					CompletionTime: &now,
					State:          prowjobv1.FailureState,
					Description:    "hello",
				}
				return pj
			},
			expectedPipelineRun: noPipelineRunChange,
		},
		{
			name:      "error when we cannot get prowjob",
			namespace: errorGetProwJob,
			err:       true,
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:           prowjobv1.TektonAgent,
					PipelineRunSpec: &pipelineSpec,
				},
				Status: prowjobv1.ProwJobStatus{
					State:       prowjobv1.PendingState,
					Description: "fancy",
				},
			},
		},
		{
			name:      "error when we cannot get pipeline run",
			namespace: errorGetPipelineRun,
			err:       true,
			observedPipelineRun: func() *pipelinev1beta1.PipelineRun {
				pj := prowjobv1.ProwJob{}
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.Agent = prowjobv1.TektonAgent
				pj.Spec.PipelineRunSpec = &pipelineSpec
				pj.Status.BuildID = pipelineID
				p, err := makePipelineRun(pj)
				if err != nil {
					panic(err)
				}
				p.Status.SetCondition(&apis.Condition{
					Type:    apis.ConditionSucceeded,
					Status:  corev1.ConditionTrue,
					Message: "hello",
				})
				return p
			}(),
		},
		{
			name:      "error when we cannot delete pipeline run",
			namespace: errorDeletePipelineRun,
			err:       true,
			observedPipelineRun: func() *pipelinev1beta1.PipelineRun {
				pj := prowjobv1.ProwJob{}
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.PipelineRunSpec = &pipelinev1beta1.PipelineRunSpec{}
				pj.Status.BuildID = pipelineID
				p, err := makePipelineRun(pj)
				if err != nil {
					panic(err)
				}
				return p
			}(),
		},
		{
			name:      "set prow job in error state when we cannot create pipeline run",
			namespace: errorCreatePipelineRun,
			err:       false,
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:           prowjobv1.TektonAgent,
					PipelineRunSpec: &pipelineSpec,
				},
			},
			expectedJob: func(pj prowjobv1.ProwJob, _ pipelinev1beta1.PipelineRun) prowjobv1.ProwJob {
				pj.Status = prowjobv1.ProwJobStatus{
					BuildID:        pipelineID,
					StartTime:      now,
					CompletionTime: &now,
					State:          prowjobv1.ErrorState,
					Description:    "start pipeline: injected create pipeline error",
				}
				return pj
			},
		},
		{
			name: "error when pipelinerunspec is nil",
			err:  true,
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:           prowjobv1.TektonAgent,
					PipelineRunSpec: nil,
				},
				Status: prowjobv1.ProwJobStatus{
					State: prowjobv1.TriggeredState,
				},
			},
		},
		{
			name:      "error when we cannot update prowjob",
			namespace: errorUpdateProwJob,
			err:       true,
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:           prowjobv1.TektonAgent,
					PipelineRunSpec: &pipelineSpec,
				},
				Status: prowjobv1.ProwJobStatus{
					State:       prowjobv1.PendingState,
					Description: "fancy",
				},
			},
			observedPipelineRun: func() *pipelinev1beta1.PipelineRun {
				pj := prowjobv1.ProwJob{}
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.Agent = prowjobv1.TektonAgent
				pj.Spec.PipelineRunSpec = &pipelineSpec
				pj.Status.BuildID = pipelineID
				p, err := makePipelineRun(pj)
				if err != nil {
					panic(err)
				}
				p.Status.SetCondition(&apis.Condition{
					Type:    apis.ConditionSucceeded,
					Status:  corev1.ConditionTrue,
					Message: "hello",
				})
				return p
			}(),
		}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name := "the-object-name"
			// prowjobs all live in the same ns, so use name for injecting errors
			if tc.namespace == errorGetProwJob {
				name = errorGetProwJob
			} else if tc.namespace == errorUpdateProwJob {
				name = errorUpdateProwJob
			}
			if tc.context == "" {
				tc.context = kube.DefaultClusterAlias
			}
			r := &fakeReconciler{
				jobs:      map[string]prowjobv1.ProwJob{},
				pipelines: map[string]pipelinev1beta1.PipelineRun{},
				nows:      now,
			}

			jk := toKey(fakePJCtx, fakePJNS, name)
			if j := tc.observedJob; j != nil {
				j.Name = name
				j.Spec.Type = prowjobv1.PeriodicJob
				r.jobs[jk] = *j
			}
			pk := toKey(tc.context, tc.namespace, name)
			if p := tc.observedPipelineRun; p != nil {
				p.Name = name
				p.Labels[kube.ProwJobIDLabel] = name
				r.pipelines[pk] = *p
			}

			expectedJobs := map[string]prowjobv1.ProwJob{}
			if j := tc.expectedJob; j != nil {
				expectedJobs[jk] = j(r.jobs[jk], r.pipelines[pk])
			}
			expectedPipelineRuns := map[string]pipelinev1beta1.PipelineRun{}
			if p := tc.expectedPipelineRun; p != nil {
				expectedPipelineRuns[pk] = p(r.jobs[jk], r.pipelines[pk])
			}

			tk := toKey(tc.context, tc.namespace, name)
			err := reconcile(r, tk)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Error("failed to receive expected error")
			case !equality.Semantic.DeepEqual(r.jobs, expectedJobs):
				t.Errorf("prowjobs do not match:\n%s", diff.ObjectReflectDiff(expectedJobs, r.jobs))
			case !equality.Semantic.DeepEqual(r.pipelines, expectedPipelineRuns):
				t.Errorf("pipelineruns do not match:\n%s", diff.ObjectReflectDiff(expectedPipelineRuns, r.pipelines))
			}
		})
	}

}

func TestPipelineMeta(t *testing.T) {
	cases := []struct {
		name     string
		pj       prowjobv1.ProwJob
		expected func(prowjobv1.ProwJob, *metav1.ObjectMeta)
	}{
		{
			name: "Use pj.Spec.Namespace for pipeline namespace",
			pj: prowjobv1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "whatever",
					Namespace: "wrong",
				},
				Spec: prowjobv1.ProwJobSpec{
					Namespace: "correct",
				},
			},
			expected: func(pj prowjobv1.ProwJob, meta *metav1.ObjectMeta) {
				meta.Name = pj.Name
				meta.Namespace = pj.Spec.Namespace
				meta.Labels, meta.Annotations = decorate.LabelsAndAnnotationsForJob(pj)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var expected metav1.ObjectMeta
			tc.expected(tc.pj, &expected)
			actual := pipelineMeta(tc.pj.Name, tc.pj)
			if !equality.Semantic.DeepEqual(actual, expected) {
				t.Errorf("pipeline meta does not match:\n%s", diff.ObjectReflectDiff(expected, actual))
			}
		})
	}
}

func TestMakePipelineGitResource(t *testing.T) {
	pj := prowjobv1.ProwJob{}
	pj.Name = "hello"
	pj.Namespace = "world"

	cases := []struct {
		name             string
		refs             prowjobv1.Refs
		expectedURL      string
		expectedRevision string
	}{
		{
			name: "use clone URI field (prioritized) and base ref name",
			refs: prowjobv1.Refs{
				CloneURI: "https://source.host/test/test.git",
				RepoLink: "don't use me",
				Org:      "or",
				Repo:     "me",
				BaseRef:  "feature-branch",
			},
			expectedURL:      "https://source.host/test/test.git",
			expectedRevision: "feature-branch",
		},
		{
			name: "use repo link field and base SHA",
			refs: prowjobv1.Refs{
				RepoLink: "https://source.host/test/test",
				BaseSHA:  "DEADBEEF",
			},
			expectedURL:      "https://source.host/test/test.git",
			expectedRevision: "DEADBEEF",
		},
		{
			name: "use default clone URI (from org repo) and pull sha",
			refs: prowjobv1.Refs{
				Org:  "o",
				Repo: "r",
				Pulls: []prowjobv1.Pull{
					{
						SHA: "pull-sha",
					},
				},
			},
			expectedURL:      "https://github.com/o/r.git",
			expectedRevision: "pull-sha",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resourceName := "resource-name"
			actual := makePipelineGitResource(resourceName, tc.refs, pj)
			expected := &pipelinev1beta1.PipelineResource{
				ObjectMeta: pipelineMeta(resourceName, pj),
				Spec: pipelinev1beta1.PipelineResourceSpec{
					Type: pipelinev1beta1.PipelineResourceTypeGit,
					Params: []pipelinev1beta1.ResourceParam{
						{
							Name:  "url",
							Value: tc.expectedURL,
						},
						{
							Name:  "revision",
							Value: tc.expectedRevision,
						},
					},
				},
			}

			if !equality.Semantic.DeepEqual(actual, expected) {
				t.Errorf("pipelineresources do not match:\n%s", diff.ObjectReflectDiff(expected, actual))
			}
		})
	}
}

func TestMakeResources(t *testing.T) {
	cases := []struct {
		name        string
		job         func(prowjobv1.ProwJob) prowjobv1.ProwJob
		pipelineRun func(pipelinev1beta1.PipelineRun) pipelinev1beta1.PipelineRun
		err         bool
	}{
		{
			name: "reject empty prow job",
			job:  func(_ prowjobv1.ProwJob) prowjobv1.ProwJob { return prowjobv1.ProwJob{} },
			err:  true,
		},
		{
			name: "return valid pipeline with valid prowjob",
		},
		{
			name: "configure implicit git repository",
			job: func(pj prowjobv1.ProwJob) prowjobv1.ProwJob {
				pj.Spec.Type = prowjobv1.PresubmitJob
				pj.Spec.Refs = &prowjobv1.Refs{
					CloneURI: "https://source.host/test/test.git",
					BaseRef:  "feature-branch",
					Pulls: []prowjobv1.Pull{
						{
							Number: 1,
						},
					},
				}
				pj.Spec.PipelineRunSpec.Resources = []pipelinev1beta1.PipelineResourceBinding{
					{
						Name:        "implicit git resource",
						ResourceRef: &pipelinev1beta1.PipelineResourceRef{Name: config.ProwImplicitGitResource},
					},
				}
				return pj
			},
			pipelineRun: func(pr pipelinev1beta1.PipelineRun) pipelinev1beta1.PipelineRun {
				pr.Spec.Params[4].Value = pipelinev1beta1.ArrayOrString{
					Type:      pipelinev1beta1.ParamTypeString,
					StringVal: string(prowjobv1.PresubmitJob),
				}
				pr.Spec.Params = append(pr.Spec.Params,
					pipelinev1beta1.Param{
						Name: "PULL_BASE_REF",
						Value: pipelinev1beta1.ArrayOrString{
							Type:      pipelinev1beta1.ParamTypeString,
							StringVal: "feature-branch",
						},
					},
					pipelinev1beta1.Param{
						Name: "PULL_BASE_SHA",
						Value: pipelinev1beta1.ArrayOrString{
							Type:      pipelinev1beta1.ParamTypeString,
							StringVal: "",
						},
					},
					pipelinev1beta1.Param{
						Name: "PULL_NUMBER",
						Value: pipelinev1beta1.ArrayOrString{
							Type:      pipelinev1beta1.ParamTypeString,
							StringVal: "1",
						},
					},
					pipelinev1beta1.Param{
						Name: "PULL_PULL_SHA",
						Value: pipelinev1beta1.ArrayOrString{
							Type:      pipelinev1beta1.ParamTypeString,
							StringVal: "",
						},
					},
					pipelinev1beta1.Param{
						Name: "PULL_REFS",
						Value: pipelinev1beta1.ArrayOrString{
							Type:      pipelinev1beta1.ParamTypeString,
							StringVal: "feature-branch,1:",
						},
					},
					pipelinev1beta1.Param{
						Name: "REPO_NAME",
						Value: pipelinev1beta1.ArrayOrString{
							Type:      pipelinev1beta1.ParamTypeString,
							StringVal: "",
						},
					},
					pipelinev1beta1.Param{
						Name: "REPO_OWNER",
						Value: pipelinev1beta1.ArrayOrString{
							Type:      pipelinev1beta1.ParamTypeString,
							StringVal: "",
						},
					},
				)
				pr.Spec.Resources = []pipelinev1beta1.PipelineResourceBinding{
					{
						Name: "implicit git resource",
						ResourceSpec: &pipelinev1beta1.PipelineResourceSpec{
							Type: "git",
							Params: []pipelinev1beta1.ResourceParam{
								{Name: "url", Value: "https://source.host/test/test.git"},
								{Name: "revision", Value: "pull/1/head"},
							},
						},
					},
				}
				return pr
			},
		},
		{
			name: "configure sources when extra refs are configured",
			job: func(pj prowjobv1.ProwJob) prowjobv1.ProwJob {
				pj.Spec.ExtraRefs = []prowjobv1.Refs{{Org: "org0"}, {Org: "org1"}}
				pj.Spec.PipelineRunSpec.Resources = []pipelinev1beta1.PipelineResourceBinding{
					{
						Name:        "git resource A",
						ResourceRef: &pipelinev1beta1.PipelineResourceRef{Name: "PROW_EXTRA_GIT_REF_0"},
					},
					{
						Name:        "git resource B",
						ResourceRef: &pipelinev1beta1.PipelineResourceRef{Name: "PROW_EXTRA_GIT_REF_1"},
					},
				}
				return pj
			},
			pipelineRun: func(pr pipelinev1beta1.PipelineRun) pipelinev1beta1.PipelineRun {
				pr.Spec.Resources = []pipelinev1beta1.PipelineResourceBinding{
					{
						Name: "git resource A",
						ResourceSpec: &pipelinev1beta1.PipelineResourceSpec{
							Type: "git",
							Params: []pipelinev1beta1.ResourceParam{
								{Name: "url", Value: "https://github.com/org0/.git"},
								{Name: "revision"},
							},
						},
					},
					{
						Name: "git resource B",
						ResourceSpec: &pipelinev1beta1.PipelineResourceSpec{
							Type: "git",
							Params: []pipelinev1beta1.ResourceParam{
								{Name: "url", Value: "https://github.com/org1/.git"},
								{Name: "revision"},
							},
						},
					},
				}
				return pr
			},
		},
		{
			name: "do not override unrelated git resources",
			job: func(pj prowjobv1.ProwJob) prowjobv1.ProwJob {
				pj.Spec.PipelineRunSpec.Resources = []pipelinev1beta1.PipelineResourceBinding{
					{
						Name:        "git resource A",
						ResourceRef: &pipelinev1beta1.PipelineResourceRef{Name: "PROW_EXTRA_GIT_REF_LOL_JK"},
					},
					{
						Name:        "git resource B",
						ResourceRef: &pipelinev1beta1.PipelineResourceRef{Name: "some-other-ref"},
					},
				}
				return pj
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const randomPipelineRunID = "so-many-pipelines"
			pj := prowjobv1.ProwJob{}
			pj.Name = "world"
			pj.Namespace = "hello"
			pj.Spec.Type = prowjobv1.PeriodicJob
			pj.Spec.Job = "ci-job"
			pj.Spec.PipelineRunSpec = &pipelinev1beta1.PipelineRunSpec{}
			pj.Status.BuildID = randomPipelineRunID

			if tc.job != nil {
				pj = tc.job(pj)
			}

			actualRun, err := makePipelineRun(pj)
			if err != nil {
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
				return
			} else if tc.err {
				t.Error("failed to receive expected error")
			}

			jobSpecRaw, err := json.Marshal(downwardapi.NewJobSpec(pj.Spec, randomPipelineRunID, pj.Name))
			if err != nil {
				t.Errorf("failed to marshal job spec: %v", err)
			}
			expectedRun := pipelinev1beta1.PipelineRun{
				ObjectMeta: pipelineMeta(pj.Name, pj),
				Spec:       *pj.Spec.PipelineRunSpec,
			}
			expectedRun.Spec.Params = []pipelinev1beta1.Param{
				{
					Name: "BUILD_ID",
					Value: pipelinev1beta1.ArrayOrString{
						Type:      pipelinev1beta1.ParamTypeString,
						StringVal: randomPipelineRunID,
					},
				},
				{
					Name: "CI",
					Value: pipelinev1beta1.ArrayOrString{
						Type:      pipelinev1beta1.ParamTypeString,
						StringVal: "true",
					},
				},
				{
					Name: "JOB_NAME",
					Value: pipelinev1beta1.ArrayOrString{
						Type:      pipelinev1beta1.ParamTypeString,
						StringVal: pj.Spec.Job,
					},
				},
				{
					Name: "JOB_SPEC",
					Value: pipelinev1beta1.ArrayOrString{
						Type:      pipelinev1beta1.ParamTypeString,
						StringVal: string(jobSpecRaw),
					},
				},
				{
					Name: "JOB_TYPE",
					Value: pipelinev1beta1.ArrayOrString{
						Type:      pipelinev1beta1.ParamTypeString,
						StringVal: string(prowjobv1.PeriodicJob),
					},
				},
				{
					Name: "PROW_JOB_ID",
					Value: pipelinev1beta1.ArrayOrString{
						Type:      pipelinev1beta1.ParamTypeString,
						StringVal: pj.Name,
					},
				},
			}
			if tc.pipelineRun != nil {
				expectedRun = tc.pipelineRun(expectedRun)
			}

			if !equality.Semantic.DeepEqual(actualRun, &expectedRun) {
				t.Errorf("pipelineruns do not match:\n%s", diff.ObjectReflectDiff(&expectedRun, actualRun))
			}
		})
	}
}

func TestDescription(t *testing.T) {
	cases := []struct {
		name     string
		message  string
		reason   string
		fallback string
		expected string
	}{
		{
			name:     "prefer message over reason or fallback",
			message:  "hello",
			reason:   "world",
			fallback: "doh",
			expected: "hello",
		},
		{
			name:     "prefer reason over fallback",
			reason:   "world",
			fallback: "other",
			expected: "world",
		},
		{
			name:     "use fallback if nothing else set",
			fallback: "fancy",
			expected: "fancy",
		},
	}

	for _, tc := range cases {
		bc := apis.Condition{
			Message: tc.message,
			Reason:  tc.reason,
		}
		if actual := description(bc, tc.fallback); actual != tc.expected {
			t.Errorf("%s: actual %q != expected %q", tc.name, actual, tc.expected)
		}
	}
}

func TestProwJobStatus(t *testing.T) {
	now := metav1.Now()
	later := metav1.NewTime(now.Time.Add(1 * time.Hour))
	cases := []struct {
		name     string
		input    pipelinev1beta1.PipelineRunStatus
		state    prowjobv1.ProwJobState
		desc     string
		fallback string
	}{
		{
			name:  "empty conditions returns pending/scheduling",
			state: prowjobv1.PendingState,
			desc:  descScheduling,
		},
		{
			name: "truly succeeded state returns success",
			input: pipelinev1beta1.PipelineRunStatus{
				Status: duckv1beta1.Status{
					Conditions: []apis.Condition{
						{
							Type:    apis.ConditionSucceeded,
							Status:  corev1.ConditionTrue,
							Message: "fancy",
						},
					},
				},
			},
			state:    prowjobv1.SuccessState,
			desc:     "fancy",
			fallback: descSucceeded,
		},
		{
			name: "falsely succeeded state returns failure",
			input: pipelinev1beta1.PipelineRunStatus{
				Status: duckv1beta1.Status{
					Conditions: []apis.Condition{
						{
							Type:    apis.ConditionSucceeded,
							Status:  corev1.ConditionFalse,
							Message: "weird",
						},
					},
				},
			},
			state:    prowjobv1.FailureState,
			desc:     "weird",
			fallback: descFailed,
		},
		{
			name: "unstarted job returns pending/initializing",
			input: pipelinev1beta1.PipelineRunStatus{
				Status: duckv1beta1.Status{
					Conditions: []apis.Condition{
						{
							Type:    apis.ConditionSucceeded,
							Status:  corev1.ConditionUnknown,
							Message: "hola",
						},
					},
				},
			},
			state:    prowjobv1.PendingState,
			desc:     "hola",
			fallback: descInitializing,
		},
		{
			name: "unfinished job returns running",
			input: pipelinev1beta1.PipelineRunStatus{
				PipelineRunStatusFields: pipelinev1beta1.PipelineRunStatusFields{
					StartTime: now.DeepCopy(),
				},
				Status: duckv1beta1.Status{
					Conditions: []apis.Condition{
						{
							Type:    apis.ConditionSucceeded,
							Status:  corev1.ConditionUnknown,
							Message: "hola",
						},
					},
				},
			},
			state:    prowjobv1.PendingState,
			desc:     "hola",
			fallback: descRunning,
		},
		{
			name: "pipelines with unknown success status are still running",
			input: pipelinev1beta1.PipelineRunStatus{
				PipelineRunStatusFields: pipelinev1beta1.PipelineRunStatusFields{
					StartTime:      now.DeepCopy(),
					CompletionTime: later.DeepCopy(),
				},
				Status: duckv1beta1.Status{
					Conditions: []apis.Condition{
						{
							Type:    apis.ConditionSucceeded,
							Status:  corev1.ConditionUnknown,
							Message: "hola",
						},
					},
				},
			},
			state:    prowjobv1.PendingState,
			desc:     "hola",
			fallback: descRunning,
		},
		{
			name: "completed pipelines without a succeeded condition end in error",
			input: pipelinev1beta1.PipelineRunStatus{
				PipelineRunStatusFields: pipelinev1beta1.PipelineRunStatusFields{
					StartTime:      now.DeepCopy(),
					CompletionTime: later.DeepCopy(),
				},
			},
			state: prowjobv1.ErrorState,
			desc:  descMissingCondition,
		},
	}

	for _, tc := range cases {
		if len(tc.fallback) > 0 {
			tc.desc = tc.fallback
			tc.fallback = ""
			tc.name += " [fallback]"
			cond := tc.input.Conditions[0]
			cond.Message = ""
			tc.input.Conditions = []apis.Condition{cond}
			cases = append(cases, tc)
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, desc := prowJobStatus(tc.input)
			if state != tc.state {
				t.Errorf("state %q != expected %q", state, tc.state)
			}
			if desc != tc.desc {
				t.Errorf("description %q != expected %q", desc, tc.desc)
			}
		})
	}
}
