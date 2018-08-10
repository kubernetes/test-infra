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

package main

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"

	buildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	errorGetProwJob    = "error-get-prowjob"
	errorGetBuild      = "error-get-build"
	errorDeleteBuild   = "error-delete-build"
	errorCreateBuild   = "error-create-build"
	errorUpdateProwJob = "error-update-prowjob"
)

type fakeReconciler struct {
	jobs   map[string]prowjobv1.ProwJob
	builds map[string]buildv1alpha1.Build
	nows   metav1.Time
}

func key(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "/" + name
}

func (r *fakeReconciler) now() metav1.Time {
	fmt.Println(r.nows)
	return r.nows
}

func (r *fakeReconciler) getProwJob(namespace, name string) (*prowjobv1.ProwJob, error) {
	if namespace == errorGetProwJob {
		return nil, errors.New("injected create build error")
	}
	k := key(namespace, name)
	pj, present := r.jobs[k]
	if !present {
		return nil, apierrors.NewNotFound(prowjobv1.Resource("ProwJob"), name)
	}
	return &pj, nil
}
func (r *fakeReconciler) getBuild(namespace, name string) (*buildv1alpha1.Build, error) {
	if namespace == errorGetBuild {
		return nil, errors.New("injected create build error")
	}
	k := key(namespace, name)
	b, present := r.builds[k]
	if !present {
		return nil, apierrors.NewNotFound(buildv1alpha1.Resource("Build"), name)
	}
	return &b, nil
}
func (r *fakeReconciler) deleteBuild(namespace, name string) error {
	if namespace == errorDeleteBuild {
		return errors.New("injected create build error")
	}
	k := key(namespace, name)
	if _, present := r.builds[k]; !present {
		return apierrors.NewNotFound(buildv1alpha1.Resource("Build"), name)
	}
	delete(r.builds, k)
	return nil
}

func (r *fakeReconciler) createBuild(namespace string, b *buildv1alpha1.Build) (*buildv1alpha1.Build, error) {
	if b == nil {
		return nil, errors.New("nil build")
	}
	if namespace == errorCreateBuild {
		return nil, errors.New("injected create build error")
	}
	k := key(namespace, b.Name)
	if _, alreadyExists := r.builds[k]; alreadyExists {
		return nil, apierrors.NewAlreadyExists(prowjobv1.Resource("ProwJob"), b.Name)
	}
	r.builds[k] = *b
	return b, nil
}

func (r *fakeReconciler) updateProwJob(namespace string, pj *prowjobv1.ProwJob) (*prowjobv1.ProwJob, error) {
	if pj == nil {
		return nil, errors.New("nil prowjob")
	}
	if namespace == errorUpdateProwJob {
		return nil, errors.New("injected update prowjob error")
	}
	k := key(namespace, pj.Name)
	if _, present := r.jobs[k]; !present {
		return nil, apierrors.NewNotFound(prowjobv1.Resource("ProwJob"), pj.Name)
	}
	r.jobs[k] = *pj
	return pj, nil
}

func TestReconcile(t *testing.T) {
	now := metav1.Now()
	buildSpec := buildv1alpha1.BuildSpec{}
	noJobChange := func(pj prowjobv1.ProwJob, _ buildv1alpha1.Build) prowjobv1.ProwJob {
		return pj
	}
	noBuildChange := func(_ prowjobv1.ProwJob, b buildv1alpha1.Build) buildv1alpha1.Build {
		return b
	}
	cases := []struct {
		name          string
		namespace     string
		observedJob   *prowjobv1.ProwJob
		observedBuild *buildv1alpha1.Build
		expectedJob   func(prowjobv1.ProwJob, buildv1alpha1.Build) prowjobv1.ProwJob
		expectedBuild func(prowjobv1.ProwJob, buildv1alpha1.Build) buildv1alpha1.Build
		err           bool
	}{{
		name: "new prow job creates build",
		observedJob: &prowjobv1.ProwJob{
			Spec: prowjobv1.ProwJobSpec{
				Agent:     prowjobv1.KnativeBuildAgent,
				BuildSpec: &buildSpec,
			},
		},
		expectedJob: func(pj prowjobv1.ProwJob, _ buildv1alpha1.Build) prowjobv1.ProwJob {
			pj.Status = prowjobv1.ProwJobStatus{
				StartTime:   now,
				State:       prowjobv1.TriggeredState,
				Description: descScheduling,
			}
			return pj
		},
		expectedBuild: func(pj prowjobv1.ProwJob, _ buildv1alpha1.Build) buildv1alpha1.Build {
			b, err := makeBuild(pj)
			if err != nil {
				panic(err)
			}
			return *b
		},
	},
		{
			name: "do not create build for failed prowjob",
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:     prowjobv1.KnativeBuildAgent,
					BuildSpec: &buildSpec,
				},
				Status: prowjobv1.ProwJobStatus{
					State: prowjobv1.FailureState,
				},
			},
			expectedJob: noJobChange,
		},
		{
			name: "do not create build for successful prowjob",
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:     prowjobv1.KnativeBuildAgent,
					BuildSpec: &buildSpec,
				},
				Status: prowjobv1.ProwJobStatus{
					State: prowjobv1.SuccessState,
				},
			},
			expectedJob: noJobChange,
		},
		{
			name: "do not create build for aborted prowjob",
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:     prowjobv1.KnativeBuildAgent,
					BuildSpec: &buildSpec,
				},
				Status: prowjobv1.ProwJobStatus{
					State: prowjobv1.AbortedState,
				},
			},
			expectedJob: noJobChange,
		},
		{
			name: "delete build after deleting prowjob",
			observedBuild: func() *buildv1alpha1.Build {
				pj := prowjobv1.ProwJob{}
				pj.Spec.BuildSpec = &buildv1alpha1.BuildSpec{}
				b, err := makeBuild(pj)
				if err != nil {
					panic(err)
				}
				return b
			}(),
		},
		{
			name: "do not delete deleted builds",
			observedBuild: func() *buildv1alpha1.Build {
				pj := prowjobv1.ProwJob{}
				pj.Spec.BuildSpec = &buildv1alpha1.BuildSpec{}
				b, err := makeBuild(pj)
				b.DeletionTimestamp = &now
				if err != nil {
					panic(err)
				}
				return b
			}(),
			expectedBuild: noBuildChange,
		},
		{
			name: "only delete builds created by controller",
			observedBuild: func() *buildv1alpha1.Build {
				pj := prowjobv1.ProwJob{}
				pj.Spec.BuildSpec = &buildv1alpha1.BuildSpec{}
				b, err := makeBuild(pj)
				if err != nil {
					panic(err)
				}
				b.OwnerReferences = nil
				return b
			}(),
			expectedBuild: noBuildChange,
		},
		{
			name: "update job status if build resets",
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent: prowjobv1.KnativeBuildAgent,
					BuildSpec: &buildv1alpha1.BuildSpec{
						ServiceAccountName: "robot",
					},
				},
				Status: prowjobv1.ProwJobStatus{
					State:       prowjobv1.PendingState,
					StartTime:   metav1.Now(),
					Description: "fancy",
				},
			},
			observedBuild: func() *buildv1alpha1.Build {
				pj := prowjobv1.ProwJob{}
				pj.Spec.Agent = prowjobv1.KnativeBuildAgent
				pj.Spec.BuildSpec = &buildSpec
				b, err := makeBuild(pj)
				if err != nil {
					panic(err)
				}
				return b
			}(),
			expectedJob: func(pj prowjobv1.ProwJob, _ buildv1alpha1.Build) prowjobv1.ProwJob {
				pj.Status.State = prowjobv1.TriggeredState
				pj.Status.Description = descScheduling
				return pj
			},
			expectedBuild: noBuildChange,
		},
		{
			name: "prowjob goes pending when build starts",
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:     prowjobv1.KnativeBuildAgent,
					BuildSpec: &buildSpec,
				},
				Status: prowjobv1.ProwJobStatus{
					State:       prowjobv1.TriggeredState,
					Description: "fancy",
				},
			},
			observedBuild: func() *buildv1alpha1.Build {
				pj := prowjobv1.ProwJob{}
				pj.Spec.Agent = prowjobv1.KnativeBuildAgent
				pj.Spec.BuildSpec = &buildSpec
				b, err := makeBuild(pj)
				if err != nil {
					panic(err)
				}
				b.Status.SetCondition(&buildv1alpha1.BuildCondition{
					Type:    buildv1alpha1.BuildSucceeded,
					Message: "hello",
				})
				b.Status.StartTime = now
				return b
			}(),
			expectedJob: func(pj prowjobv1.ProwJob, _ buildv1alpha1.Build) prowjobv1.ProwJob {
				pj.Status = prowjobv1.ProwJobStatus{
					StartTime:   now,
					State:       prowjobv1.PendingState,
					Description: "hello",
				}
				return pj
			},
			expectedBuild: noBuildChange,
		},
		{
			name: "prowjob succeeds when build succeeds",
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:     prowjobv1.KnativeBuildAgent,
					BuildSpec: &buildSpec,
				},
				Status: prowjobv1.ProwJobStatus{
					State:       prowjobv1.PendingState,
					Description: "fancy",
				},
			},
			observedBuild: func() *buildv1alpha1.Build {
				pj := prowjobv1.ProwJob{}
				pj.Spec.Agent = prowjobv1.KnativeBuildAgent
				pj.Spec.BuildSpec = &buildSpec
				b, err := makeBuild(pj)
				if err != nil {
					panic(err)
				}
				b.Status.SetCondition(&buildv1alpha1.BuildCondition{
					Type:    buildv1alpha1.BuildSucceeded,
					Status:  corev1.ConditionTrue,
					Message: "hello",
				})
				b.Status.CompletionTime = now
				b.Status.StartTime = now
				return b
			}(),
			expectedJob: func(pj prowjobv1.ProwJob, _ buildv1alpha1.Build) prowjobv1.ProwJob {
				pj.Status = prowjobv1.ProwJobStatus{
					StartTime:      now,
					CompletionTime: &now,
					State:          prowjobv1.SuccessState,
					Description:    "hello",
				}
				return pj
			},
			expectedBuild: noBuildChange,
		},
		{
			name: "prowjob fails when build fails",
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:     prowjobv1.KnativeBuildAgent,
					BuildSpec: &buildSpec,
				},
				Status: prowjobv1.ProwJobStatus{
					State:       prowjobv1.PendingState,
					Description: "fancy",
				},
			},
			observedBuild: func() *buildv1alpha1.Build {
				pj := prowjobv1.ProwJob{}
				pj.Spec.Agent = prowjobv1.KnativeBuildAgent
				pj.Spec.BuildSpec = &buildSpec
				b, err := makeBuild(pj)
				if err != nil {
					panic(err)
				}
				b.Status.SetCondition(&buildv1alpha1.BuildCondition{
					Type:    buildv1alpha1.BuildSucceeded,
					Status:  corev1.ConditionFalse,
					Message: "hello",
				})
				b.Status.StartTime = now
				b.Status.CompletionTime = now
				return b
			}(),
			expectedJob: func(pj prowjobv1.ProwJob, _ buildv1alpha1.Build) prowjobv1.ProwJob {
				pj.Status = prowjobv1.ProwJobStatus{
					StartTime:      now,
					CompletionTime: &now,
					State:          prowjobv1.FailureState,
					Description:    "hello",
				}
				return pj
			},
			expectedBuild: noBuildChange,
		},
		{
			name:      "error when we cannot get prowjob",
			namespace: errorGetProwJob,
			err:       true,
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:     prowjobv1.KnativeBuildAgent,
					BuildSpec: &buildSpec,
				},
				Status: prowjobv1.ProwJobStatus{
					State:       prowjobv1.PendingState,
					Description: "fancy",
				},
			},
		},
		{
			name:      "error when we cannot get build",
			namespace: errorGetBuild,
			err:       true,
			observedBuild: func() *buildv1alpha1.Build {
				pj := prowjobv1.ProwJob{}
				pj.Spec.Agent = prowjobv1.KnativeBuildAgent
				pj.Spec.BuildSpec = &buildSpec
				b, err := makeBuild(pj)
				if err != nil {
					panic(err)
				}
				b.Status.SetCondition(&buildv1alpha1.BuildCondition{
					Type:    buildv1alpha1.BuildSucceeded,
					Status:  corev1.ConditionTrue,
					Message: "hello",
				})
				b.Status.CompletionTime = now
				b.Status.StartTime = now
				return b
			}(),
		},
		{
			name:      "error when we cannot delete build",
			namespace: errorDeleteBuild,
			err:       true,
			observedBuild: func() *buildv1alpha1.Build {
				pj := prowjobv1.ProwJob{}
				pj.Spec.BuildSpec = &buildv1alpha1.BuildSpec{}
				b, err := makeBuild(pj)
				if err != nil {
					panic(err)
				}
				return b
			}(),
		},
		{
			name:      "error when we cannot create build",
			namespace: errorCreateBuild,
			err:       true,
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:     prowjobv1.KnativeBuildAgent,
					BuildSpec: &buildSpec,
				},
			},
			expectedJob: func(pj prowjobv1.ProwJob, _ buildv1alpha1.Build) prowjobv1.ProwJob {
				pj.Status = prowjobv1.ProwJobStatus{
					StartTime:   now,
					State:       prowjobv1.TriggeredState,
					Description: descScheduling,
				}
				return pj
			},
		},
		{
			name: "error when buildspec is nil",
			err:  true,
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:     prowjobv1.KnativeBuildAgent,
					BuildSpec: nil,
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
					Agent:     prowjobv1.KnativeBuildAgent,
					BuildSpec: &buildSpec,
				},
				Status: prowjobv1.ProwJobStatus{
					State:       prowjobv1.PendingState,
					Description: "fancy",
				},
			},
			observedBuild: func() *buildv1alpha1.Build {
				pj := prowjobv1.ProwJob{}
				pj.Spec.Agent = prowjobv1.KnativeBuildAgent
				pj.Spec.BuildSpec = &buildSpec
				b, err := makeBuild(pj)
				if err != nil {
					panic(err)
				}
				b.Status.SetCondition(&buildv1alpha1.BuildCondition{
					Type:    buildv1alpha1.BuildSucceeded,
					Status:  corev1.ConditionTrue,
					Message: "hello",
				})
				b.Status.CompletionTime = now
				b.Status.StartTime = now
				return b
			}(),
		}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const name = "the-object-name"
			k := key(tc.namespace, name)
			r := &fakeReconciler{
				jobs:   map[string]prowjobv1.ProwJob{},
				builds: map[string]buildv1alpha1.Build{},
				nows:   now,
			}
			if j := tc.observedJob; j != nil {
				j.Name = name
				r.jobs[k] = *j
			}
			if b := tc.observedBuild; b != nil {
				b.Name = name
				r.builds[k] = *b
			}
			expectedJobs := map[string]prowjobv1.ProwJob{}
			if j := tc.expectedJob; j != nil {
				expectedJobs[k] = j(r.jobs[k], r.builds[k])
			}
			expectedBuilds := map[string]buildv1alpha1.Build{}
			if b := tc.expectedBuild; b != nil {
				expectedBuilds[k] = b(r.jobs[k], r.builds[k])
			}
			err := reconcile(r, k)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Error("failed to receive expected error")
			case !reflect.DeepEqual(r.jobs, expectedJobs):
				t.Errorf("prowjobs do not match: %#v != %#v %t", r.jobs, expectedJobs, r.jobs[k].Status.StartTime == expectedJobs[k].Status.StartTime)
			case !reflect.DeepEqual(r.builds, expectedBuilds):
				t.Errorf("builds do not match: %#v != %#v", r.builds, expectedBuilds)
			}
		})
	}

}

func TestMakeBuild(t *testing.T) {
	if _, err := makeBuild(prowjobv1.ProwJob{}); err == nil {
		t.Error("failed to receive expected error")
	}
	pj := prowjobv1.ProwJob{}
	pj.Name = "world"
	pj.Namespace = "hello"
	pj.Spec.BuildSpec = &buildv1alpha1.BuildSpec{}
	switch b, err := makeBuild(pj); {
	case err != nil:
		t.Errorf("unexpected error: %v", err)
	case !reflect.DeepEqual(b.Spec, *pj.Spec.BuildSpec):
		t.Errorf("buildspecs do not match %#v != expected %#v", b.Spec, pj.Spec.BuildSpec)
	case len(b.OwnerReferences) == 0:
		t.Error("failed to find owner references")
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
		bc := buildv1alpha1.BuildCondition{
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
		input    buildv1alpha1.BuildStatus
		state    prowjobv1.ProwJobState
		desc     string
		fallback string
	}{
		{
			name:  "empty conditions returns triggered/scheduling",
			state: prowjobv1.TriggeredState,
			desc:  descScheduling,
		},
		{
			name: "truly succeeded state returns success",
			input: buildv1alpha1.BuildStatus{
				Conditions: []buildv1alpha1.BuildCondition{
					{
						Type:    buildv1alpha1.BuildSucceeded,
						Status:  corev1.ConditionTrue,
						Message: "fancy",
					},
				},
			},
			state:    prowjobv1.SuccessState,
			desc:     "fancy",
			fallback: descSucceeded,
		},
		{
			name: "falsely succeeded state returns failure",
			input: buildv1alpha1.BuildStatus{
				Conditions: []buildv1alpha1.BuildCondition{
					{
						Type:    buildv1alpha1.BuildSucceeded,
						Status:  corev1.ConditionFalse,
						Message: "weird",
					},
				},
			},
			state:    prowjobv1.FailureState,
			desc:     "weird",
			fallback: descFailed,
		},
		{
			name: "unstarted job returns triggered/initializing",
			input: buildv1alpha1.BuildStatus{
				Conditions: []buildv1alpha1.BuildCondition{
					{
						Type:    buildv1alpha1.BuildSucceeded,
						Status:  corev1.ConditionUnknown,
						Message: "hola",
					},
				},
			},
			state:    prowjobv1.TriggeredState,
			desc:     "hola",
			fallback: descInitializing,
		},
		{
			name: "unfinished job returns running",
			input: buildv1alpha1.BuildStatus{
				StartTime: now,
				Conditions: []buildv1alpha1.BuildCondition{
					{
						Type:    buildv1alpha1.BuildSucceeded,
						Status:  corev1.ConditionUnknown,
						Message: "hola",
					},
				},
			},
			state:    prowjobv1.PendingState,
			desc:     "hola",
			fallback: descRunning,
		},
		{
			name: "expect a finished job to have a success status",
			input: buildv1alpha1.BuildStatus{
				StartTime:      now,
				CompletionTime: later,
				Conditions: []buildv1alpha1.BuildCondition{
					{
						Type:    buildv1alpha1.BuildSucceeded,
						Status:  corev1.ConditionUnknown,
						Message: "hola",
					},
				},
			},
			state:    prowjobv1.ErrorState,
			desc:     "hola",
			fallback: descUnknown,
		},
		{
			name: "expect a finished job to have a condition",
			input: buildv1alpha1.BuildStatus{
				StartTime:      now,
				CompletionTime: later,
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
			tc.input.Conditions = []buildv1alpha1.BuildCondition{cond}
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
