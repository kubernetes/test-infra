/*
Copyright 2024 The Kubernetes Authors.

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

package scheduler_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	testingclient "k8s.io/client-go/testing"
	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/client/clientset/versioned/scheme"
	"k8s.io/test-infra/prow/scheduler"
	"k8s.io/test-infra/prow/scheduler/strategy"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type fakeStrategy struct {
	cluster string
	err     error
}

func (fs *fakeStrategy) Schedule(context.Context, *prowv1.ProwJob) (strategy.Result, error) {
	return strategy.Result{Cluster: fs.cluster}, fs.err
}

// Alright our controller-runtime dependency is old as hell so I have to
// implement interceptors on my own.
type fakeTracker struct {
	testingclient.ObjectTracker
	errors map[string]error
}

func (ft *fakeTracker) Get(gvr schema.GroupVersionResource, ns, name string) (runtime.Object, error) {
	if err, exists := ft.errors["GET"]; exists {
		return nil, err
	}
	return ft.ObjectTracker.Get(gvr, ns, name)
}

func (ft *fakeTracker) Update(gvr schema.GroupVersionResource, obj runtime.Object, ns string) error {
	if err, exists := ft.errors["UPDATE"]; exists {
		return err
	}
	return ft.ObjectTracker.Update(gvr, obj, ns)
}

func TestReconcile(t *testing.T) {
	for _, tc := range []struct {
		name            string
		pj              *prowv1.ProwJob
		request         reconcile.Request
		cluster         string
		schedulingError error
		clientErrors    map[string]error
		wantPJ          *prowv1.ProwJob
		wantError       error
	}{
		{
			name:    "Successfully assign a cluster",
			pj:      &prowv1.ProwJob{ObjectMeta: v1.ObjectMeta{Name: "pj", Namespace: "ns", ResourceVersion: "1"}},
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "pj", Namespace: "ns"}},
			cluster: "foo",
			wantPJ: &prowv1.ProwJob{
				ObjectMeta: v1.ObjectMeta{Name: "pj", Namespace: "ns", ResourceVersion: "2"},
				Spec:       prowv1.ProwJobSpec{Cluster: "foo"},
				Status:     prowv1.ProwJobStatus{State: prowv1.TriggeredState},
			},
		},
		{
			name:    "Skip ProwJob not found",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "pj", Namespace: "ns"}},
		},
		{
			name:         "Error getting Prowjob",
			request:      reconcile.Request{NamespacedName: types.NamespacedName{Name: "pj", Namespace: "ns"}},
			clientErrors: map[string]error{"GET": errors.New("expected")},
			wantError:    errors.New("get prowjob pj: expected"),
		},
		{
			name:    "Error patching Prowjob",
			request: reconcile.Request{NamespacedName: types.NamespacedName{Name: "pj", Namespace: "ns"}},
			pj:      &prowv1.ProwJob{ObjectMeta: v1.ObjectMeta{Name: "pj", Namespace: "ns", ResourceVersion: "1"}},
			cluster: "foo",
			wantPJ: &prowv1.ProwJob{
				ObjectMeta: v1.ObjectMeta{Name: "pj", Namespace: "ns", ResourceVersion: "2"},
				Spec:       prowv1.ProwJobSpec{Cluster: "foo"},
				Status:     prowv1.ProwJobStatus{State: prowv1.TriggeredState},
			},
			clientErrors: map[string]error{"UPDATE": errors.New("expected")},
			wantError:    errors.New("patch prowjob: expected"),
		},
		{
			name:            "Scheduling error",
			pj:              &prowv1.ProwJob{ObjectMeta: v1.ObjectMeta{Name: "pj", Namespace: "ns", ResourceVersion: "1"}},
			request:         reconcile.Request{NamespacedName: types.NamespacedName{Name: "pj", Namespace: "ns"}},
			schedulingError: errors.New("expected"),
			wantError:       errors.New("schedule prowjob pj: expected"),
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tracker := testingclient.NewObjectTracker(scheme.Scheme, scheme.Codecs.UniversalDecoder())
			fakeTracker := fakeTracker{ObjectTracker: tracker, errors: tc.clientErrors}

			builder := fakectrlruntimeclient.NewClientBuilder().WithObjectTracker(&fakeTracker)
			// Builder doesn't like nil
			if tc.pj != nil {
				builder = builder.WithObjects(tc.pj)
			}
			pjClient := builder.Build()

			r := scheduler.NewReconciler(pjClient, &fakeStrategy{cluster: tc.cluster, err: tc.schedulingError})
			_, err := r.Reconcile(context.TODO(), tc.request)

			if tc.wantError != nil && err != nil {
				if tc.wantError.Error() != err.Error() {
					t.Errorf("Expected error %s but got %s", tc.wantError, err)
				}
				return
			} else if tc.wantError != nil && err == nil {
				t.Errorf("Expected error %s but got nil", tc.wantError)
				return
			} else if tc.wantError == nil && err != nil {
				t.Errorf("Expected error nil but got %s", err)
				return
			}

			pjs := prowv1.ProwJobList{}
			if err := pjClient.List(context.TODO(), &pjs); err != nil {
				// It's just not supposed to happen
				t.Fatalf("Couldn't get PJs from the fake client: %s", err)
			}

			if tc.wantPJ != nil {
				if len(pjs.Items) != 1 {
					t.Errorf("Expected 1 ProwJob but got %d", len(pjs.Items))
					return
				}
				if diff := cmp.Diff(tc.wantPJ, &pjs.Items[0]); diff != "" {
					t.Errorf("Unexpected ProwJob: %s", diff)
				}
			}
		})
	}
}
