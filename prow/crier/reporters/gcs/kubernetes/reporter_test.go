/*
Copyright 2020 The Kubernetes Authors.

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

package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/crier/reporters/gcs/internal/testutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func TestShouldReport(t *testing.T) {
	tests := []struct {
		name                  string
		agent                 prowv1.ProwJobAgent
		isComplete            bool
		hasNoPendingTimestamp bool
		hasBuildID            bool
		shouldReport          bool
	}{
		{
			name:         "completed kubernetes tests are reported",
			agent:        prowv1.KubernetesAgent,
			isComplete:   true,
			hasBuildID:   true,
			shouldReport: true,
		},
		{
			name:         "pending job is reported",
			agent:        prowv1.KubernetesAgent,
			isComplete:   false,
			hasBuildID:   true,
			shouldReport: true,
		},
		{
			name:                  "not yet pending job is not reported",
			agent:                 prowv1.KubernetesAgent,
			isComplete:            false,
			hasNoPendingTimestamp: true,
			hasBuildID:            true,
			shouldReport:          false,
		},
		{
			name:         "complete non-kubernetes tests are not reported",
			agent:        prowv1.JenkinsAgent,
			isComplete:   true,
			hasBuildID:   true,
			shouldReport: false,
		},
		{
			name:         "incomplete non-kubernetes tests are not reported",
			agent:        prowv1.JenkinsAgent,
			isComplete:   false,
			hasBuildID:   true,
			shouldReport: false,
		},
		{
			name:         "complete kubernetes tests with no build ID are not reported",
			agent:        prowv1.KubernetesAgent,
			isComplete:   true,
			hasBuildID:   false,
			shouldReport: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pj := &prowv1.ProwJob{
				Spec: prowv1.ProwJobSpec{
					Agent: tc.agent,
				},
				Status: prowv1.ProwJobStatus{
					State:     prowv1.PendingState,
					StartTime: metav1.Time{Time: time.Now()},
				},
			}
			if tc.isComplete {
				pj.Status.State = prowv1.SuccessState
				pj.Status.CompletionTime = &metav1.Time{Time: time.Now()}
			}
			if tc.hasBuildID {
				pj.Status.BuildID = "123456789"
			}
			if !tc.hasNoPendingTimestamp {
				pj.Status.PendingTime = &metav1.Time{}
			}

			kgr := internalNew(testutil.Fca{}.Config, nil, nil, 1.0, false)
			shouldReport := kgr.ShouldReport(logrus.NewEntry(logrus.StandardLogger()), pj)
			if shouldReport != tc.shouldReport {
				t.Errorf("Expected ShouldReport() to return %v, but got %v", tc.shouldReport, shouldReport)
			}
		})
	}
}

type testResourceGetter struct {
	namespace string
	cluster   string
	pod       *v1.Pod
	events    []v1.Event
	patchData string
	patchType types.PatchType
}

func (rg testResourceGetter) GetPod(_ context.Context, cluster, namespace, name string) (*v1.Pod, error) {
	if rg.cluster != cluster {
		return nil, fmt.Errorf("expected cluster %q but got cluster %q", rg.cluster, cluster)
	}
	if rg.namespace != namespace {
		return nil, fmt.Errorf("expected namespace %q but got namespace %q", rg.namespace, namespace)
	}
	if rg.pod == nil {
		return nil, errors.New("no such pod")
	}
	if rg.pod.ObjectMeta.Name != name {
		return nil, fmt.Errorf("expected name %q, but got name %q", rg.pod.ObjectMeta.Name, name)
	}
	return rg.pod, nil
}

func (rg testResourceGetter) GetEvents(cluster, namespace string, pod *v1.Pod) ([]v1.Event, error) {
	if rg.cluster != cluster {
		return nil, fmt.Errorf("expected cluster %q but got cluster %q", rg.cluster, cluster)
	}
	if rg.namespace != namespace {
		return nil, fmt.Errorf("expected namespace %q but got namespace %q", rg.namespace, namespace)
	}
	if pod == nil {
		return nil, errors.New("expected non-nil pod")
	}
	if pod != rg.pod {
		return nil, errors.New("got the wrong pod")
	}
	return rg.events, nil
}

func (rg testResourceGetter) PatchPod(ctx context.Context, cluster, namespace, name string, pt types.PatchType, data []byte) error {
	if _, err := rg.GetPod(ctx, cluster, namespace, name); err != nil {
		return err
	}
	if rg.patchType != pt {
		return fmt.Errorf("expected patch type %s, got patchType %s", rg.patchData, pt)
	}
	if diff := cmp.Diff(string(data), rg.patchData); diff != "" {
		return fmt.Errorf("patch differs from expected patch: %s", diff)
	}

	return nil
}

func TestReportPodInfo(t *testing.T) {
	tests := []struct {
		name                    string
		pjName                  string
		pjComplete              bool
		pjPending               bool
		pjState                 prowv1.ProwJobState
		pod                     *v1.Pod
		events                  []v1.Event
		dryRun                  bool
		expectReport            bool
		expectErr               bool
		expectedPatch           string
		expectedReconcileResult *reconcile.Result
	}{
		{
			name:       "prowjob picks up pod and events",
			pjName:     "ba123965-4fd4-421f-8509-7590c129ab69",
			pjComplete: true,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ba123965-4fd4-421f-8509-7590c129ab69",
					Namespace: "test-pods",
					Labels:    map[string]string{"created-by-prow": "true"},
				},
			},
			events: []v1.Event{
				{
					Type:    "Warning",
					Message: "Some event",
				},
			},
			expectReport: true,
		},
		{
			name:       "prowjob with no events reports pod",
			pjName:     "ba123965-4fd4-421f-8509-7590c129ab69",
			pjComplete: true,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ba123965-4fd4-421f-8509-7590c129ab69",
					Namespace: "test-pods",
					Labels:    map[string]string{"created-by-prow": "true"},
				},
			},
			expectReport: true,
		},
		{
			name:         "prowjob with no pod reports nothing but does not error",
			pjName:       "ba123965-4fd4-421f-8509-7590c129ab69",
			pjComplete:   true,
			expectReport: false,
		},
		{
			name:       "nothing is reported in dryrun mode",
			pjName:     "ba123965-4fd4-421f-8509-7590c129ab69",
			pjComplete: true,
			dryRun:     true,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ba123965-4fd4-421f-8509-7590c129ab69",
					Namespace: "test-pods",
					Labels:    map[string]string{"created-by-prow": "true"},
				},
			},
			events: []v1.Event{
				{
					Type:    "Warning",
					Message: "Some event",
				},
			},
			expectReport: false,
		},
		{
			name:       "Pending incomplete prowjob gets finalizer and is not reported",
			pjName:     "ba123965-4fd4-421f-8509-7590c129ab69",
			pjPending:  true,
			pjComplete: false,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ba123965-4fd4-421f-8509-7590c129ab69",
					Namespace: "test-pods",
					Labels:    map[string]string{"created-by-prow": "true"},
				},
			},
			expectReport:  false,
			expectedPatch: `{"metadata":{"finalizers":["prow.x-k8s.io/gcsk8sreporter"]}}`,
		},
		{
			name:   "Finalizer is not added to deleted pod",
			pjName: "ba123965-4fd4-421f-8509-7590c129ab69",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers:        []string{"gcsk8sreporter"},
					Name:              "ba123965-4fd4-421f-8509-7590c129ab69",
					Namespace:         "test-pods",
					Labels:            map[string]string{"created-by-prow": "true"},
					DeletionTimestamp: func() *metav1.Time { t := metav1.Now(); return &t }(),
				},
			},
			expectReport:  false,
			expectedPatch: `{"metadata":{"finalizers":null}}`,
		},
		{
			name:       "Finalizer is removed from complete pod",
			pjName:     "ba123965-4fd4-421f-8509-7590c129ab69",
			pjPending:  false,
			pjComplete: true,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{"gcsk8sreporter"},
					Name:       "ba123965-4fd4-421f-8509-7590c129ab69",
					Namespace:  "test-pods",
					Labels:     map[string]string{"created-by-prow": "true"},
				},
			},
			expectReport:  true,
			expectedPatch: `{"metadata":{"finalizers":null}}`,
		},
		{
			name:                    "RequeueAfter is returned for incomplete aborted job and nothing happens",
			pjName:                  "ba123965-4fd4-421f-8509-7590c129ab69",
			pjState:                 prowv1.AbortedState,
			pjPending:               false,
			pjComplete:              false,
			expectReport:            false,
			expectedReconcileResult: &reconcile.Result{RequeueAfter: 10 * time.Second},
		},
		{
			name:       "Completed aborted job is reported",
			pjName:     "ba123965-4fd4-421f-8509-7590c129ab69",
			pjState:    prowv1.AbortedState,
			pjPending:  false,
			pjComplete: true,
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{"gcsk8sreporter"},
					Name:       "ba123965-4fd4-421f-8509-7590c129ab69",
					Namespace:  "test-pods",
					Labels:     map[string]string{"created-by-prow": "true"},
				},
			},
			expectReport:  true,
			expectedPatch: `{"metadata":{"finalizers":null}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pj := &prowv1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: tc.pjName,
				},
				Spec: prowv1.ProwJobSpec{
					Agent:   prowv1.KubernetesAgent,
					Cluster: "the-build-cluster",
					Type:    prowv1.PeriodicJob,
				},
				Status: prowv1.ProwJobStatus{
					State:     prowv1.SuccessState,
					StartTime: metav1.Time{Time: time.Now()},
					BuildID:   "12345",
				},
			}
			if tc.pjComplete {
				pj.Status.CompletionTime = &metav1.Time{Time: time.Now()}
			}
			if tc.pjPending {
				pj.Status.PendingTime = &metav1.Time{}
			}
			if tc.pjState != "" {
				pj.Status.State = tc.pjState
			}

			fca := testutil.Fca{C: config.Config{ProwConfig: config.ProwConfig{
				PodNamespace: "test-pods",
				Plank: config.Plank{
					DefaultDecorationConfigs: map[string]*prowv1.DecorationConfig{"*": {
						GCSConfiguration: &prowv1.GCSConfiguration{
							Bucket:       "kubernetes-jenkins",
							PathPrefix:   "some-prefix",
							PathStrategy: prowv1.PathStrategyLegacy,
							DefaultOrg:   "kubernetes",
							DefaultRepo:  "kubernetes",
						},
					}},
				},
			}}}

			rg := testResourceGetter{
				namespace: "test-pods",
				cluster:   "the-build-cluster",
				pod:       tc.pod,
				events:    tc.events,
				patchData: tc.expectedPatch,
				patchType: types.MergePatchType,
			}
			author := &testutil.TestAuthor{}
			reporter := internalNew(fca.Config, author, rg, 1.0, tc.dryRun)
			reconcileResult, err := reporter.report(logrus.NewEntry(logrus.StandardLogger()), pj)

			if tc.expectErr {
				if err == nil {
					t.Fatal("Expected an error, but didn't get one")
				}
				return
			} else if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if diff := cmp.Diff(reconcileResult, tc.expectedReconcileResult); diff != "" {
				t.Errorf("reconcileResult differs from expected reconcileResult: %s", diff)
			}

			if !tc.expectReport {
				if author.AlreadyUsed {
					t.Fatalf("Expected nothing to be written, but something was written to %q:\n\n%s", author.Path, string(author.Content))
				}
				return
			}

			var result PodReport
			err = json.Unmarshal(author.Content, &result)
			if err != nil {
				t.Fatalf("Couldn't unmarshal reported JSON: %v", err)
			}

			if !cmp.Equal(result.Pod, tc.pod) {
				t.Errorf("Got mismatching pods:\n%s", cmp.Diff(tc.pod, result.Pod))
			}
			if !cmp.Equal(result.Events, tc.events) {
				t.Errorf("Got mismatching events:\n%s", cmp.Diff(tc.events, result.Events))
			}
		})
	}
}
