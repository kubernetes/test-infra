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
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	buildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	duckv1alpha1 "github.com/knative/pkg/apis/duck/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	fake_prowjobclient "k8s.io/test-infra/prow/client/clientset/versioned/fake"
	prowjoblistv1 "k8s.io/test-infra/prow/client/listers/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pod-utils/decorate"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/wrapper"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func (r *fakeReconciler) now() metav1.Time {
	fmt.Println(r.nows)
	return r.nows
}

const fakePJCtx = "prow-context"
const fakePJNS = "prow-job"

func (r *fakeReconciler) getProwJob(name string) (*prowjobv1.ProwJob, error) {
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

func (r *fakeReconciler) terminateDupProwJobs(ctx string, namespace string) error {
	return nil
}

func (r *fakeReconciler) updateProwJob(pj *prowjobv1.ProwJob) (*prowjobv1.ProwJob, error) {
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
	r.jobs[k] = *pj
	return pj, nil
}

func (r *fakeReconciler) getBuild(context, namespace, name string) (*buildv1alpha1.Build, error) {
	if namespace == errorGetBuild {
		return nil, errors.New("injected create build error")
	}
	k := toKey(context, namespace, name)
	b, present := r.builds[k]
	if !present {
		return nil, apierrors.NewNotFound(buildv1alpha1.Resource("Build"), name)
	}
	return &b, nil
}
func (r *fakeReconciler) deleteBuild(context, namespace, name string) error {
	if namespace == errorDeleteBuild {
		return errors.New("injected create build error")
	}
	k := toKey(context, namespace, name)
	if _, present := r.builds[k]; !present {
		return apierrors.NewNotFound(buildv1alpha1.Resource("Build"), name)
	}
	delete(r.builds, k)
	return nil
}

func (r *fakeReconciler) createBuild(context, namespace string, b *buildv1alpha1.Build) (*buildv1alpha1.Build, error) {
	if b == nil {
		return nil, errors.New("nil build")
	}
	if namespace == errorCreateBuild {
		return nil, errors.New("injected create build error")
	}
	k := toKey(context, namespace, b.Name)
	if _, alreadyExists := r.builds[k]; alreadyExists {
		return nil, apierrors.NewAlreadyExists(prowjobv1.Resource("ProwJob"), b.Name)
	}
	r.builds[k] = *b
	return b, nil
}

const randomBuildID = "so-many-builds"
const randomBuildURL = "random://url"

func (r *fakeReconciler) buildID(pj prowjobv1.ProwJob) (string, string, error) {
	return randomBuildID, randomBuildURL, nil
}

func (r *fakeReconciler) defaultBuildTimeout() time.Duration {
	return time.Hour
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
			name:    "enqueue build directly",
			context: "hey",
			obj: &buildv1alpha1.Build{
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

func TestTerminateDupProwJobs(t *testing.T) {
	if err := buildv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		t.Fatalf("failed to add buildv1alpha1 to scheme: %v", err)
	}
	now := time.Now()
	nowFn := func() *metav1.Time {
		reallyNow := metav1.NewTime(now)
		return &reallyNow
	}
	cases := []struct {
		name string

		useAllowCancellations bool
		allowCancellations    bool
		pjs                   []prowjobv1.ProwJob
		builds                []buildv1alpha1.Build

		abortedPJs     sets.String
		expectedBuilds sets.String
	}{
		{
			name:                  "terminates all duplicated jobs and all builds",
			useAllowCancellations: true,
			allowCancellations:    true,
			pjs: []prowjobv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KnativeBuildAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KnativeBuildAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "older", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KnativeBuildAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-2 * time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "older-k8s-agent", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KubernetesAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-2 * time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "completed", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KnativeBuildAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime:      metav1.NewTime(now.Add(-2 * time.Hour)),
						CompletionTime: nowFn(),
					},
				},
			},
			builds: []buildv1alpha1.Build{
				{ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS}},
				{ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS}},
				{ObjectMeta: metav1.ObjectMeta{Name: "older", Namespace: fakePJNS}},
			},
			abortedPJs:     sets.NewString("old", "older"),
			expectedBuilds: sets.NewString("newest"),
		},
		{
			name:                  "terminates all duplicated jobs and available builds",
			useAllowCancellations: true,
			allowCancellations:    true,
			pjs: []prowjobv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KnativeBuildAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KnativeBuildAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "older", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KnativeBuildAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-2 * time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "older-k8s-agent", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KubernetesAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-2 * time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "completed", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KnativeBuildAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime:      metav1.NewTime(now.Add(-2 * time.Hour)),
						CompletionTime: nowFn(),
					},
				},
			},
			builds: []buildv1alpha1.Build{
				{ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS}},
				{ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS}},
			},
			abortedPJs:     sets.NewString("old", "older"),
			expectedBuilds: sets.NewString("newest"),
		},
		{
			name:                  "terminates all duplicated jobs without deleting the builds (allowCancellations set)",
			useAllowCancellations: true,
			allowCancellations:    false,
			pjs: []prowjobv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KnativeBuildAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KnativeBuildAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "older", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KnativeBuildAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-2 * time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "older-k8s-agent", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KubernetesAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-2 * time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "completed", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KnativeBuildAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime:      metav1.NewTime(now.Add(-2 * time.Hour)),
						CompletionTime: nowFn(),
					},
				},
			},
			builds: []buildv1alpha1.Build{
				{ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS}},
				{ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS}},
				{ObjectMeta: metav1.ObjectMeta{Name: "older", Namespace: fakePJNS}},
			},
			abortedPJs:     sets.NewString("old", "older"),
			expectedBuilds: sets.NewString("newest", "old", "older"),
		},
		{
			name:                  "terminates all duplicated jobs without deleting the builds (allowCancellations feature disabled)",
			useAllowCancellations: false,
			allowCancellations:    true,
			pjs: []prowjobv1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KnativeBuildAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Minute)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KnativeBuildAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "older", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KnativeBuildAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-2 * time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "older-k8s-agent", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KubernetesAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime: metav1.NewTime(now.Add(-2 * time.Hour)),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "completed", Namespace: fakePJNS},
					Spec: prowjobv1.ProwJobSpec{
						Agent: prowjobv1.KnativeBuildAgent,
						Type:  prowjobv1.PresubmitJob,
						Job:   "j1",
						Refs: &prowjobv1.Refs{
							Repo:  "test",
							Pulls: []prowjobv1.Pull{{Number: 1}},
						},
					},
					Status: prowjobv1.ProwJobStatus{
						StartTime:      metav1.NewTime(now.Add(-2 * time.Hour)),
						CompletionTime: nowFn(),
					},
				},
			},
			builds: []buildv1alpha1.Build{
				{ObjectMeta: metav1.ObjectMeta{Name: "newest", Namespace: fakePJNS}},
				{ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: fakePJNS}},
				{ObjectMeta: metav1.ObjectMeta{Name: "older", Namespace: fakePJNS}},
			},
			abortedPJs:     sets.NewString("old", "older"),
			expectedBuilds: sets.NewString("newest", "old", "older"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var prowJobs []runtime.Object
			for i := range tc.pjs {
				prowJobs = append(prowJobs, &tc.pjs[i])
			}
			pjc := fake_prowjobclient.NewSimpleClientset(prowJobs...)
			var builds []runtime.Object
			for i := range tc.builds {
				builds = append(builds, &tc.builds[i])
			}
			buildClient := fakectrlruntimeclient.NewFakeClient(builds...)

			agent := &config.Agent{}
			config := &config.Config{
				ProwConfig: config.ProwConfig{
					ProwJobNamespace: fakePJNS,
					Plank: config.Plank{
						Controller: config.Controller{
							AllowCancellations: tc.allowCancellations,
						},
					},
				},
			}
			agent.Set(config)

			indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
			for _, pj := range tc.pjs {
				err := indexer.Add(pj.DeepCopy())
				if err != nil {
					t.Fatalf("%s: error updating the lister index with prow job %q", err, pj.GetName())
				}
			}
			pjl := prowjoblistv1.NewProwJobLister(indexer)

			c := controller{
				config: agent.Config,
				builds: map[string]buildConfig{
					fakePJCtx: {
						client: buildClient,
					},
				},
				pjc:                   pjc,
				pjLister:              pjl,
				useAllowCancellations: tc.useAllowCancellations,
			}

			if err := c.terminateDupProwJobs(fakePJCtx, fakePJNS); err != nil {
				t.Fatalf("%s: error terminating duplicated prow jobs: %v", tc.name, err)
			}

			abortedPJs := sets.NewString()
			pjs, err := pjc.ProwV1().ProwJobs(fakePJNS).List(metav1.ListOptions{})
			if err != nil {
				t.Fatalf("%s: error listing the prow jobs: %v", tc.name, err)
			}
			for _, j := range pjs.Items {
				if j.Status.State == prowjobv1.AbortedState {
					abortedPJs.Insert(j.GetName())
				}
			}
			if missing := tc.abortedPJs.Difference(abortedPJs); missing.Len() > 0 {
				t.Errorf("%s: did not aborted expected prow jobs: %v", tc.name, missing.List())
			}
			if extra := abortedPJs.Difference(tc.abortedPJs); extra.Len() > 0 {
				t.Errorf("%s: found unexpectedly aborted prow jobs: %v", tc.name, extra.List())
			}

			foundBuilds := sets.NewString()
			buildList := &buildv1alpha1.BuildList{}
			if err := buildClient.List(context.Background(), buildList); err != nil {
				t.Fatalf("%s: error list the builds: %v", tc.name, err)
			}
			for _, b := range buildList.Items {
				foundBuilds.Insert(b.GetName())
			}
			if missing := tc.expectedBuilds.Difference(foundBuilds); missing.Len() > 0 {
				t.Errorf("%s: did not deleted the expected builds: %v", tc.name, missing.List())
			}
			if extra := foundBuilds.Difference(tc.expectedBuilds); extra.Len() > 0 {
				t.Errorf("%s: found unexpectedly deleted builds: %v", tc.name, extra.List())
			}
		})
	}
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
	defaultTimeout := 1 * time.Hour
	cases := []struct {
		name          string
		namespace     string
		context       string
		observedJob   *prowjobv1.ProwJob
		observedBuild *buildv1alpha1.Build
		expectedJob   func(prowjobv1.ProwJob, buildv1alpha1.Build) prowjobv1.ProwJob
		expectedBuild func(prowjobv1.ProwJob, buildv1alpha1.Build) buildv1alpha1.Build
		err           bool
	}{
		{
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
					BuildID:     randomBuildID,
					URL:         randomBuildURL,
				}
				return pj
			},
			expectedBuild: func(pj prowjobv1.ProwJob, _ buildv1alpha1.Build) buildv1alpha1.Build {
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Status.BuildID = randomBuildID
				b, err := makeBuild(pj, defaultTimeout)
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
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.BuildSpec = &buildv1alpha1.BuildSpec{}
				pj.Status.BuildID = randomBuildID
				b, err := makeBuild(pj, defaultTimeout)
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
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.BuildSpec = &buildv1alpha1.BuildSpec{}
				pj.Status.BuildID = randomBuildID
				b, err := makeBuild(pj, defaultTimeout)
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
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.BuildSpec = &buildv1alpha1.BuildSpec{}
				pj.Status.BuildID = randomBuildID
				b, err := makeBuild(pj, defaultTimeout)
				if err != nil {
					panic(err)
				}
				delete(b.Labels, kube.CreatedByProw)
				return b
			}(),
			expectedBuild: noBuildChange,
		},
		{
			name:    "delete prow builds in the wrong cluster",
			context: "wrong-cluster",
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:   prowjobv1.KnativeBuildAgent,
					Cluster: "target-cluster",
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
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.Agent = prowjobv1.KnativeBuildAgent
				pj.Spec.BuildSpec = &buildSpec
				pj.Status.BuildID = randomBuildID
				b, err := makeBuild(pj, defaultTimeout)
				if err != nil {
					panic(err)
				}
				return b
			}(),
			expectedJob: noJobChange,
		},
		{
			name:    "ignore random builds in the wrong cluster",
			context: "wrong-cluster",
			observedJob: &prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Agent:   prowjobv1.KnativeBuildAgent,
					Cluster: "target-cluster",
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
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.Agent = prowjobv1.KnativeBuildAgent
				pj.Spec.BuildSpec = &buildSpec
				pj.Status.BuildID = randomBuildID
				b, err := makeBuild(pj, defaultTimeout)
				if err != nil {
					panic(err)
				}
				delete(b.Labels, kube.CreatedByProw)
				return b
			}(),
			expectedJob:   noJobChange,
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
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.Agent = prowjobv1.KnativeBuildAgent
				pj.Spec.BuildSpec = &buildSpec
				pj.Status.BuildID = randomBuildID
				b, err := makeBuild(pj, defaultTimeout)
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
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.Agent = prowjobv1.KnativeBuildAgent
				pj.Spec.BuildSpec = &buildSpec
				pj.Status.BuildID = randomBuildID
				b, err := makeBuild(pj, defaultTimeout)
				if err != nil {
					panic(err)
				}
				b.Status.SetCondition(&duckv1alpha1.Condition{
					Type:    buildv1alpha1.BuildSucceeded,
					Message: "hello",
				})
				b.Status.StartTime = now.DeepCopy()
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
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.Agent = prowjobv1.KnativeBuildAgent
				pj.Spec.BuildSpec = &buildSpec
				pj.Status.BuildID = randomBuildID
				b, err := makeBuild(pj, defaultTimeout)
				if err != nil {
					panic(err)
				}
				b.Status.SetCondition(&duckv1alpha1.Condition{
					Type:    buildv1alpha1.BuildSucceeded,
					Status:  corev1.ConditionTrue,
					Message: "hello",
				})
				b.Status.CompletionTime = now.DeepCopy()
				b.Status.StartTime = now.DeepCopy()
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
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.Agent = prowjobv1.KnativeBuildAgent
				pj.Spec.BuildSpec = &buildSpec
				pj.Status.BuildID = randomBuildID
				b, err := makeBuild(pj, defaultTimeout)
				if err != nil {
					panic(err)
				}
				b.Status.SetCondition(&duckv1alpha1.Condition{
					Type:    buildv1alpha1.BuildSucceeded,
					Status:  corev1.ConditionFalse,
					Message: "hello",
				})
				b.Status.StartTime = now.DeepCopy()
				b.Status.CompletionTime = now.DeepCopy()
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
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.Agent = prowjobv1.KnativeBuildAgent
				pj.Spec.BuildSpec = &buildSpec
				pj.Status.BuildID = randomBuildID
				b, err := makeBuild(pj, defaultTimeout)
				if err != nil {
					panic(err)
				}
				b.Status.SetCondition(&duckv1alpha1.Condition{
					Type:    buildv1alpha1.BuildSucceeded,
					Status:  corev1.ConditionTrue,
					Message: "hello",
				})
				b.Status.CompletionTime = now.DeepCopy()
				b.Status.StartTime = now.DeepCopy()
				return b
			}(),
		},
		{
			name:      "error when we cannot delete build",
			namespace: errorDeleteBuild,
			err:       true,
			observedBuild: func() *buildv1alpha1.Build {
				pj := prowjobv1.ProwJob{}
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.BuildSpec = &buildv1alpha1.BuildSpec{}
				pj.Status.BuildID = randomBuildID
				b, err := makeBuild(pj, defaultTimeout)
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
				pj.Spec.Type = prowjobv1.PeriodicJob
				pj.Spec.Agent = prowjobv1.KnativeBuildAgent
				pj.Spec.BuildSpec = &buildSpec
				pj.Status.BuildID = randomBuildID
				b, err := makeBuild(pj, defaultTimeout)
				if err != nil {
					panic(err)
				}
				b.Status.SetCondition(&duckv1alpha1.Condition{
					Type:    buildv1alpha1.BuildSucceeded,
					Status:  corev1.ConditionTrue,
					Message: "hello",
				})
				b.Status.CompletionTime = now.DeepCopy()
				b.Status.StartTime = now.DeepCopy()
				return b
			}(),
		},
	}

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
			bk := toKey(tc.context, tc.namespace, name)
			jk := toKey(fakePJCtx, fakePJNS, name)
			r := &fakeReconciler{
				jobs:   map[string]prowjobv1.ProwJob{},
				builds: map[string]buildv1alpha1.Build{},
				nows:   now,
			}
			if j := tc.observedJob; j != nil {
				j.Name = name
				j.Spec.Type = prowjobv1.PeriodicJob
				r.jobs[jk] = *j
			}
			if b := tc.observedBuild; b != nil {
				b.Name = name
				r.builds[bk] = *b
			}
			expectedJobs := map[string]prowjobv1.ProwJob{}
			if j := tc.expectedJob; j != nil {
				expectedJobs[jk] = j(r.jobs[jk], r.builds[bk])
			}
			expectedBuilds := map[string]buildv1alpha1.Build{}
			if b := tc.expectedBuild; b != nil {
				expectedBuilds[bk] = b(r.jobs[jk], r.builds[bk])
			}
			err := reconcile(r, bk)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Error("failed to receive expected error")
			case !equality.Semantic.DeepEqual(r.jobs, expectedJobs):
				t.Errorf("prowjobs do not match:\n%s", diff.ObjectReflectDiff(expectedJobs, r.jobs))
			case !equality.Semantic.DeepEqual(r.builds, expectedBuilds):
				t.Errorf("builds do not match:\n%s", diff.ObjectReflectDiff(expectedBuilds, r.builds))
			}
		})
	}

}

func TestDefaultArguments(t *testing.T) {
	cases := []struct {
		name     string
		t        buildv1alpha1.TemplateInstantiationSpec
		env      map[string]string
		expected buildv1alpha1.TemplateInstantiationSpec
	}{
		{
			name: "nothing set works",
		},
		{
			name: "add env",
			env: map[string]string{
				"hello": "world",
			},
			expected: buildv1alpha1.TemplateInstantiationSpec{
				Arguments: []buildv1alpha1.ArgumentSpec{{Name: "hello", Value: "world"}},
			},
		},
		{
			name: "do not override env",
			t: buildv1alpha1.TemplateInstantiationSpec{
				Arguments: []buildv1alpha1.ArgumentSpec{
					{Name: "ignore", Value: "this"},
					{Name: "keep", Value: "original value"},
				},
			},
			env: map[string]string{
				"hello": "world",
				"keep":  "should not see this",
			},
			expected: buildv1alpha1.TemplateInstantiationSpec{
				Arguments: []buildv1alpha1.ArgumentSpec{
					{Name: "ignore", Value: "this"},
					{Name: "keep", Value: "original value"},
					{Name: "hello", Value: "world"},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			templ := tc.t
			defaultArguments(&templ, tc.env)
			if !equality.Semantic.DeepEqual(templ, tc.expected) {
				t.Errorf("builds do not match:\n%s", diff.ObjectReflectDiff(&tc.expected, templ))
			}

		})
	}
}

func TestDefaultEnv(t *testing.T) {
	cases := []struct {
		name     string
		c        corev1.Container
		env      map[string]string
		expected corev1.Container
	}{
		{
			name: "nothing set works",
		},
		{
			name: "add env",
			env: map[string]string{
				"hello": "world",
			},
			expected: corev1.Container{
				Env: []corev1.EnvVar{{Name: "hello", Value: "world"}},
			},
		},
		{
			name: "do not override env",
			c: corev1.Container{
				Env: []corev1.EnvVar{
					{Name: "ignore", Value: "this"},
					{Name: "keep", Value: "original value"},
				},
			},
			env: map[string]string{
				"hello": "world",
				"keep":  "should not see this",
			},
			expected: corev1.Container{
				Env: []corev1.EnvVar{
					{Name: "ignore", Value: "this"},
					{Name: "keep", Value: "original value"},
					{Name: "hello", Value: "world"},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.c
			defaultEnv(&c, tc.env)
			if !equality.Semantic.DeepEqual(c, tc.expected) {
				t.Errorf("builds do not match:\n%s", diff.ObjectReflectDiff(&tc.expected, c))
			}
		})
	}
}

func TestInjectSource(t *testing.T) {
	cases := []struct {
		name     string
		build    buildv1alpha1.Build
		pj       prowjobv1.ProwJob
		expected func(*buildv1alpha1.Build, prowjobv1.ProwJob)
		err      bool
	}{
		{
			name: "do nothing when source is set",
			build: buildv1alpha1.Build{
				Spec: buildv1alpha1.BuildSpec{
					Source: &buildv1alpha1.SourceSpec{},
				},
			},
		},
		{
			name: "do nothing when no refs are set",
			pj: prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					Type: prowjobv1.PeriodicJob,
				},
			},
		},
		{
			name: "inject source, volumes, workdir when refs are set",
			build: buildv1alpha1.Build{
				Spec: buildv1alpha1.BuildSpec{
					Steps: []corev1.Container{
						{}, // Override
						{WorkingDir: "do not override"},
					},
					Template: &buildv1alpha1.TemplateInstantiationSpec{},
				},
			},
			pj: prowjobv1.ProwJob{
				Spec: prowjobv1.ProwJobSpec{
					ExtraRefs: []prowjobv1.Refs{{Org: "hi", Repo: "there"}},
					DecorationConfig: &prowjobv1.DecorationConfig{
						UtilityImages: &prowjobv1.UtilityImages{},
					},
				},
			},
			expected: func(b *buildv1alpha1.Build, pj prowjobv1.ProwJob) {
				src, _, cloneVolumes, err := decorate.CloneRefs(pj, codeMount, logMount)
				if err != nil {
					t.Fatalf("failed to make clonerefs container: %v", err)
				}
				src.Name = ""
				b.Spec.Volumes = append(b.Spec.Volumes, cloneVolumes...)
				b.Spec.Source = &buildv1alpha1.SourceSpec{
					Custom: src,
				}
				wd := workDir(pj.Spec.ExtraRefs[0])
				b.Spec.Template.Arguments = append(b.Spec.Template.Arguments, wd)
				b.Spec.Steps[0].WorkingDir = wd.Value

			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expected := tc.build
			if tc.expected != nil {
				tc.expected(&expected, tc.pj)
			}

			actual := &tc.build
			_, err := injectSource(actual, tc.pj)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Error("failed to return expected error")
			case !equality.Semantic.DeepEqual(actual, &expected):
				t.Errorf("builds do not match:\n%s", diff.ObjectReflectDiff(&expected, actual))
			}
		})
	}
}

func TestBuildMeta(t *testing.T) {
	cases := []struct {
		name     string
		pj       prowjobv1.ProwJob
		expected func(prowjobv1.ProwJob, *metav1.ObjectMeta)
	}{
		{
			name: "Use pj.Spec.Namespace for build namespace",
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
			actual := buildMeta(tc.pj)
			if !equality.Semantic.DeepEqual(actual, expected) {
				t.Errorf("build meta does not match:\n%s", diff.ObjectReflectDiff(expected, actual))
			}
		})
	}
}

func TestMakeBuild(t *testing.T) {
	cases := []struct {
		name string
		job  func(prowjobv1.ProwJob) prowjobv1.ProwJob
		err  bool
	}{
		{
			name: "reject empty prow job",
			job:  func(_ prowjobv1.ProwJob) prowjobv1.ProwJob { return prowjobv1.ProwJob{} },
			err:  true,
		},
		{
			name: "reject decorate prow job with BuildTemplate",
			job: func(pj prowjobv1.ProwJob) prowjobv1.ProwJob {
				pj.Spec.BuildSpec.Template = &buildv1alpha1.TemplateInstantiationSpec{}
				pj.Spec.DecorationConfig = &prowjobv1.DecorationConfig{
					UtilityImages: &prowjobv1.UtilityImages{},
					Timeout:       &prowjobv1.Duration{Duration: 0},
					GracePeriod:   &prowjobv1.Duration{Duration: 0},
				}
				return pj
			},
			err: true,
		},
		{
			name: "return valid build with valid prowjob",
		},
		{
			name: "configure source when refs are set",
			job: func(pj prowjobv1.ProwJob) prowjobv1.ProwJob {
				pj.Spec.ExtraRefs = []prowjobv1.Refs{{Org: "bonus"}}
				pj.Spec.DecorationConfig = &prowjobv1.DecorationConfig{
					UtilityImages: &prowjobv1.UtilityImages{},
					Timeout:       &prowjobv1.Duration{Duration: 0},
					GracePeriod:   &prowjobv1.Duration{Duration: 0},
				}
				return pj
			},
		},
		{
			name: "do not override source when set",
			job: func(pj prowjobv1.ProwJob) prowjobv1.ProwJob {
				pj.Spec.ExtraRefs = []prowjobv1.Refs{{Org: "bonus"}}
				pj.Spec.DecorationConfig = &prowjobv1.DecorationConfig{
					UtilityImages: &prowjobv1.UtilityImages{},
					Timeout:       &prowjobv1.Duration{Duration: 0},
					GracePeriod:   &prowjobv1.Duration{Duration: 0},
				}
				pj.Spec.BuildSpec.Source = &buildv1alpha1.SourceSpec{}
				return pj
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defaultTimeout := 1 * time.Hour
			pj := prowjobv1.ProwJob{}
			pj.Name = "world"
			pj.Namespace = "hello"
			pj.Spec.Type = prowjobv1.PeriodicJob
			pj.Spec.BuildSpec = &buildv1alpha1.BuildSpec{}
			pj.Spec.BuildSpec.Steps = append(pj.Spec.BuildSpec.Steps, corev1.Container{})
			pj.Status.BuildID = randomBuildID
			if tc.job != nil {
				pj = tc.job(pj)
			}
			originalSpec := pj.Spec.DeepCopy()
			actual, err := makeBuild(pj, defaultTimeout)
			if err != nil {
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
				return
			} else if tc.err {
				t.Error("failed to receive expected error")
			}
			if !equality.Semantic.DeepEqual(pj.Spec, *originalSpec) {
				t.Errorf("makeBuild changed the ProwJob spec:\n:%s", diff.ObjectReflectDiff(*originalSpec, pj.Spec))
			}

			expected := buildv1alpha1.Build{
				ObjectMeta: buildMeta(pj),
				Spec:       *originalSpec.BuildSpec.DeepCopy(),
			}
			env, err := buildEnv(pj, randomBuildID)
			if err != nil {
				t.Fatalf("failed to create expected build env: %v", err)
			}
			injectEnvironment(&expected, env)
			injected, err := injectSource(&expected, pj)
			if err != nil {
				t.Fatalf("failed to inject expected source: %v", err)
			}
			injectTimeout(&expected.Spec, pj.Spec.DecorationConfig, defaultTimeout)
			if pj.Spec.DecorationConfig != nil {
				if err = decorateBuild(&expected.Spec, env[downwardapi.JobSpecEnv], *pj.Spec.DecorationConfig, injected); err != nil {
					t.Fatalf("failed to decorate: %v", err)
				}
			}
			if !equality.Semantic.DeepEqual(actual, &expected) {
				t.Errorf("builds do not match:\n%s", diff.ObjectReflectDiff(&expected, actual))
			}
		})
	}
}

func TestDecorateSteps(t *testing.T) {
	var dc prowjobv1.DecorationConfig
	dc.Timeout = &prowjobv1.Duration{Duration: 10 * time.Minute}
	dc.GracePeriod = &prowjobv1.Duration{Duration: 5 * time.Minute}
	_, tm := tools()
	tm.Name += "not-static"
	tm.MountPath += "fancy"
	actual := []corev1.Container{
		{
			Name: "leave-name-alone",
		},
		{},
		{
			Name: "this-one-too",
		},
	}
	entries, err := decorateSteps(actual, dc, tm)
	if err != nil {
		t.Errorf("decorate steps: %v", err)
	}
	expected := []corev1.Container{
		{
			Name: "leave-name-alone",
		},
		{},
		{
			Name: "this-one-too",
		},
	}
	expected[1].Name = "step-1"
	o1, err := decorate.InjectEntrypoint(&expected[0], dc.Timeout.Get(), dc.GracePeriod.Get(), expected[0].Name, "", true, logMount, tm)
	if err != nil {
		t.Fatalf("inject expected 0: %v", err)
	}
	o2, err := decorate.InjectEntrypoint(&expected[1], dc.Timeout.Get(), dc.GracePeriod.Get(), expected[1].Name, o1.MarkerFile, true, logMount, tm)
	if err != nil {
		t.Fatalf("inject expected 1: %v", err)
	}
	o3, err := decorate.InjectEntrypoint(&expected[2], dc.Timeout.Get(), dc.GracePeriod.Get(), expected[2].Name, o2.MarkerFile, true, logMount, tm)
	if err != nil {
		t.Fatalf("inject expected 2: %v", err)
	}
	expectedEntries := []wrapper.Options{*o1, *o2, *o3}
	if !equality.Semantic.DeepEqual(expectedEntries, entries) {
		t.Errorf("entries do not match:\n%s", diff.ObjectReflectDiff(expectedEntries, entries))
	}
	if !equality.Semantic.DeepEqual(expected, actual) {
		t.Errorf("steps do not match:\n%s", diff.ObjectReflectDiff(expected, actual))
	}
}

func TestInjectedSteps(t *testing.T) {
	const ejs = "fake-job-spec"
	dc := prowjobv1.DecorationConfig{
		UtilityImages: &prowjobv1.UtilityImages{},
	}
	gcsVol, gcsMount, gcsOptions := decorate.GCSOptions(dc, false)
	_, tm := tools()

	cases := []struct {
		name     string
		src      bool
		entries  []wrapper.Options
		expected func(entries []wrapper.Options) ([]corev1.Container, *corev1.Container, *corev1.Volume, error)
	}{
		{
			name: "add logMount to init upload when using source",
			src:  true,
			expected: func(entries []wrapper.Options) ([]corev1.Container, *corev1.Container, *corev1.Volume, error) {
				iu, err := decorate.InitUpload(dc.UtilityImages.InitUpload, gcsOptions, gcsMount, &logMount, nil, ejs)
				if err != nil {
					t.Fatalf("failed to create init upload: %v", err)
				}
				before := []corev1.Container{decorate.PlaceEntrypoint(dc.UtilityImages.Entrypoint, tm), *iu}
				after, err := decorate.Sidecar(dc.UtilityImages.Sidecar, gcsOptions, gcsMount, logMount, nil, ejs, decorate.RequirePassingEntries, entries...)
				if err != nil {
					t.Fatalf("failed to create sidecar: %v", err)
				}
				return before, after, gcsVol, nil
			},
		},
		{
			name: "do not add logMount to init upload when not using source",
			expected: func(entries []wrapper.Options) ([]corev1.Container, *corev1.Container, *corev1.Volume, error) {
				iu, err := decorate.InitUpload(dc.UtilityImages.InitUpload, gcsOptions, gcsMount, nil, nil, ejs)
				if err != nil {
					t.Fatalf("failed to create init upload: %v", err)
				}
				before := []corev1.Container{decorate.PlaceEntrypoint(dc.UtilityImages.Entrypoint, tm), *iu}
				after, err := decorate.Sidecar(dc.UtilityImages.Sidecar, gcsOptions, gcsMount, logMount, nil, ejs, decorate.RequirePassingEntries, entries...)
				if err != nil {
					t.Fatalf("failed to create sidecar: %v", err)
				}
				return before, after, gcsVol, nil
			},
		},
		{
			name: "sidecar includes all entries",
			entries: []wrapper.Options{
				{
					MarkerFile: "foo",
				},
				{
					Args: []string{"bar"},
				},
				{
					ProcessLog: "whatever",
				},
				{
					MetadataFile: "something",
				},
			},
			expected: func(entries []wrapper.Options) ([]corev1.Container, *corev1.Container, *corev1.Volume, error) {
				iu, err := decorate.InitUpload(dc.UtilityImages.InitUpload, gcsOptions, gcsMount, nil, nil, ejs)
				if err != nil {
					t.Fatalf("failed to create init upload: %v", err)
				}
				before := []corev1.Container{decorate.PlaceEntrypoint(dc.UtilityImages.Entrypoint, tm), *iu}
				after, err := decorate.Sidecar(dc.UtilityImages.Sidecar, gcsOptions, gcsMount, logMount, nil, ejs, decorate.RequirePassingEntries, entries...)
				if err != nil {
					t.Fatalf("failed to create sidecar: %v", err)
				}
				return before, after, gcsVol, nil
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before, after, vol, err := injectedSteps(ejs, dc, tc.src, tm, tc.entries)
			expectedBefore, expectedAfter, expectedVol, expectedErr := tc.expected(tc.entries)
			if !equality.Semantic.DeepEqual(expectedBefore, before) {
				t.Errorf("before does not match:\n%s", diff.ObjectReflectDiff(expectedBefore, before))
			}
			if !equality.Semantic.DeepEqual(expectedAfter, after) {
				t.Errorf("after does not match:\n%s", diff.ObjectReflectDiff(expectedAfter, after))
			}
			if !equality.Semantic.DeepEqual(expectedVol, vol) {
				t.Errorf("vol does not match:\n%s", diff.ObjectReflectDiff(expectedVol, vol))
			}
			if (err == nil) != (expectedErr == nil) {
				t.Errorf("%v != expected %v", err, expectedErr)
			}

		})
	}
}

func TestInjectTimeout(t *testing.T) {
	defaultTimeout := config.DefaultJobTimeout
	var infiniteTimeout time.Duration
	a := 5 * time.Minute
	b := 10 * time.Hour

	cases := []struct {
		name             string
		buildTimeout     *time.Duration
		decoratedTimeout *time.Duration
		expected         *time.Duration
	}{
		{
			name:     "set the default timeout when decoration is unset",
			expected: &defaultTimeout,
		},
		{
			name:         "do not change set timeout (decoration unset)",
			buildTimeout: &a,
			expected:     &a,
		},
		{
			name:             "do not change set timeout",
			buildTimeout:     &a,
			decoratedTimeout: &b,
			expected:         &a,
		},
		{
			name:             "change timeout when unset and decorated set",
			decoratedTimeout: &b,
			expected:         &b,
		},
		{
			name:             "set the default timeout when unset and decorated timeout is zero",
			decoratedTimeout: &infiniteTimeout,
			expected:         &defaultTimeout,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var dur *metav1.Duration
			if tc.buildTimeout != nil {
				md := metav1.Duration{
					Duration: *tc.buildTimeout,
				}
				dur = &md
			}

			dc := prowjobv1.DecorationConfig{
				UtilityImages: &prowjobv1.UtilityImages{},
				Timeout: &prowjobv1.Duration{
					Duration: defaultTimeout,
				},
			}
			if tc.decoratedTimeout != nil {
				dc.Timeout.Duration = *tc.decoratedTimeout
			}
			actual := buildv1alpha1.BuildSpec{Timeout: dur}
			injectTimeout(&actual, &dc, defaultTimeout)
			if (actual.Timeout == nil) != (tc.expected == nil) {
				t.Errorf("%v != expected %v", actual.Timeout, tc.expected)
			} else if actual.Timeout != nil && actual.Timeout.Duration != *tc.expected {
				t.Errorf("%v != expected %v", actual.Timeout.Duration, *tc.expected)
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
		bc := duckv1alpha1.Condition{
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
				Status: duckv1alpha1.Status{
					Conditions: []duckv1alpha1.Condition{
						{
							Type:    buildv1alpha1.BuildSucceeded,
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
			input: buildv1alpha1.BuildStatus{
				Status: duckv1alpha1.Status{
					Conditions: []duckv1alpha1.Condition{
						{
							Type:    buildv1alpha1.BuildSucceeded,
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
			name: "unstarted job returns triggered/initializing",
			input: buildv1alpha1.BuildStatus{
				Status: duckv1alpha1.Status{
					Conditions: []duckv1alpha1.Condition{
						{
							Type:    buildv1alpha1.BuildSucceeded,
							Status:  corev1.ConditionUnknown,
							Message: "hola",
						},
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
				StartTime: now.DeepCopy(),
				Status: duckv1alpha1.Status{
					Conditions: []duckv1alpha1.Condition{
						{
							Type:    buildv1alpha1.BuildSucceeded,
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
			name: "builds with unknown success status are still running",
			input: buildv1alpha1.BuildStatus{
				StartTime:      now.DeepCopy(),
				CompletionTime: later.DeepCopy(),
				Status: duckv1alpha1.Status{
					Conditions: []duckv1alpha1.Condition{
						{
							Type:    buildv1alpha1.BuildSucceeded,
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
			name: "completed builds without a succeeded condition end in error",
			input: buildv1alpha1.BuildStatus{
				StartTime:      now.DeepCopy(),
				CompletionTime: later.DeepCopy(),
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
			tc.input.Conditions = []duckv1alpha1.Condition{cond}
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
