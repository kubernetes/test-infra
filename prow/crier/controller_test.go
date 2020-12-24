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

package crier

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

const reporterName = "fakeReporter"

// Fake Reporter
// Sets: Which jobs should be reported
// Asserts: Which jobs are actually reported
type fakeReporter struct {
	reported         []string
	shouldReportFunc func(pj *prowv1.ProwJob) bool
	res              *reconcile.Result
	err              error
}

func (f *fakeReporter) Report(_ context.Context, _ *logrus.Entry, pj *prowv1.ProwJob) ([]*prowv1.ProwJob, *reconcile.Result, error) {
	f.reported = append(f.reported, pj.Spec.Job)
	return []*prowv1.ProwJob{pj}, f.res, f.err
}

func (f *fakeReporter) GetName() string {
	return reporterName
}

func (f *fakeReporter) ShouldReport(_ context.Context, _ *logrus.Entry, pj *prowv1.ProwJob) bool {
	return f.shouldReportFunc(pj)
}

func TestReconcile(t *testing.T) {

	const toReconcile = "foo"
	tests := []struct {
		name              string
		job               *prowv1.ProwJob
		enablementChecker func(org, repo string) bool
		shouldReport      bool
		result            *reconcile.Result
		reportErr         error

		expectResult  reconcile.Result
		expectReport  bool
		expectPatch   bool
		expectedError error
	}{
		{
			name: "reports/patches known job",
			job: &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Job:    "foo",
					Report: true,
				},
				Status: prowv1.ProwJobStatus{
					State: prowv1.TriggeredState,
				},
			},
			shouldReport: true,
			expectReport: true,
			expectPatch:  true,
		},
		{
			name: "reports/patches job whose org/repo in refs enabled",
			job: &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Job:    "foo",
					Report: true,
					Refs:   &prowv1.Refs{Org: "org", Repo: "repo"},
				},
				Status: prowv1.ProwJobStatus{
					State: prowv1.TriggeredState,
				},
			},
			enablementChecker: func(org, repo string) bool { return org == "org" && repo == "repo" },
			shouldReport:      true,
			expectReport:      true,
			expectPatch:       true,
		},
		{
			name: "reports/patches job whose org/repo in extra refs enabled",
			job: &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Job:       "foo",
					Report:    true,
					ExtraRefs: []prowv1.Refs{{Org: "org", Repo: "repo"}},
				},
				Status: prowv1.ProwJobStatus{
					State: prowv1.TriggeredState,
				},
			},
			enablementChecker: func(org, repo string) bool { return org == "org" && repo == "repo" },
			shouldReport:      true,
			expectReport:      true,
			expectPatch:       true,
		},
		{
			name: "reports/patches job whose org/repo in extra refs and refs have conflicting settings",
			job: &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Job:       "foo",
					Report:    true,
					Refs:      &prowv1.Refs{Org: "org", Repo: "repo"},
					ExtraRefs: []prowv1.Refs{{Org: "other-org", Repo: "other-repo"}},
				},
				Status: prowv1.ProwJobStatus{
					State: prowv1.TriggeredState,
				},
			},
			enablementChecker: func(org, repo string) bool { return org == "org" && repo == "repo" },
			shouldReport:      true,
			expectReport:      true,
			expectPatch:       true,
		},
		{
			name: "doesn't reports/patches job whose org/repo is not enabled",
			job: &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Job:    "foo",
					Report: true,
					Refs:   &prowv1.Refs{Org: "org", Repo: "repo"},
				},
				Status: prowv1.ProwJobStatus{
					State: prowv1.TriggeredState,
				},
			},
			enablementChecker: func(_, _ string) bool { return false },
			shouldReport:      false,
			expectReport:      false,
			expectPatch:       false,
		},
		{
			name: "doesn't report when it shouldn't",
			job: &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Job:    "foo",
					Report: true,
				},
				Status: prowv1.ProwJobStatus{
					State: prowv1.TriggeredState,
				},
			},
			shouldReport: false,
			expectReport: false,
		},
		{
			name:         "doesn't report nonexistant job",
			shouldReport: true,
			expectReport: false,
		},
		{
			name: "doesn't report when SkipReport=true (i.e. Spec.Report=false)",
			job: &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Job:    "foo",
					Report: false,
				},
			},
			shouldReport: true,
			expectReport: false,
		},
		{
			name:         "doesn't report empty job",
			job:          &prowv1.ProwJob{},
			shouldReport: true,
			expectReport: false,
		},
		{
			name: "previously-reported job isn't reported",
			job: &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Job:    "foo",
					Report: true,
				},
				Status: prowv1.ProwJobStatus{
					State: prowv1.TriggeredState,
					PrevReportStates: map[string]prowv1.ProwJobState{
						reporterName: prowv1.TriggeredState,
					},
				},
			},
			shouldReport: true,
			expectReport: false,
		},
		{
			name: "error is returned",
			job: &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Job:    "foo",
					Report: true,
				},
				Status: prowv1.ProwJobStatus{
					State: prowv1.TriggeredState,
				},
			},
			shouldReport:  true,
			reportErr:     errors.New("some-err"),
			expectedError: fmt.Errorf("failed to report job: %w", errors.New("some-err")),
		},
		{
			name: "*reconcile.Result is returned, prowjob is not updated",
			job: &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Job:    "foo",
					Report: true,
				},
				Status: prowv1.ProwJobStatus{
					State: prowv1.TriggeredState,
				},
			},
			shouldReport: true,
			result:       &reconcile.Result{RequeueAfter: time.Minute},
			expectResult: reconcile.Result{RequeueAfter: time.Minute},
			expectReport: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			rp := fakeReporter{
				shouldReportFunc: func(*prowv1.ProwJob) bool {
					return test.shouldReport
				},
				res: test.result,
				err: test.reportErr,
			}

			var prowjobs []ctrlruntimeclient.Object
			if test.job != nil {
				prowjobs = append(prowjobs, test.job)
				test.job.Name = toReconcile
			}
			cs := &patchTrackingClient{Client: fakectrlruntimeclient.NewFakeClient(prowjobs...)}
			r := &reconciler{
				pjclientset:       cs,
				reporter:          &rp,
				enablementChecker: test.enablementChecker,
			}

			result, err := r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: toReconcile}})
			if !reflect.DeepEqual(err, test.expectedError) {
				t.Fatalf("actual err %v differs from expected err %v", err, test.expectedError)
			}
			if err != nil {
				return
			}
			if diff := cmp.Diff(result, test.expectResult); diff != "" {
				t.Errorf("result differs from expected result: %s", diff)
			}

			var expectReports []string
			if test.expectReport {
				expectReports = []string{toReconcile}
			}
			if !reflect.DeepEqual(expectReports, rp.reported) {
				t.Errorf("mismatch report: wants %v, got %v", expectReports, rp.reported)
			}

			if (cs.patches != 0) != test.expectPatch {
				if test.expectPatch {
					t.Error("expected patch, but didn't get it")
				} else {
					t.Error("got unexpected patch")
				}
			}
		})
	}
}

type patchTrackingClient struct {
	ctrlruntimeclient.Client
	patches int
}

func (c *patchTrackingClient) Patch(ctx context.Context, obj ctrlruntimeclient.Object, patch ctrlruntimeclient.Patch, opts ...ctrlruntimeclient.PatchOption) error {
	c.patches++
	return c.Client.Patch(ctx, obj, patch, opts...)
}
