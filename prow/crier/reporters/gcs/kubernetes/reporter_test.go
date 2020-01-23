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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/crier/reporters/gcs/internal/testutil"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func TestShouldReport(t *testing.T) {
	tests := []struct {
		name         string
		agent        prowv1.ProwJobAgent
		isComplete   bool
		shouldReport bool
	}{
		{
			name:         "completed kubernetes tests are reported",
			agent:        prowv1.KubernetesAgent,
			isComplete:   true,
			shouldReport: true,
		},
		{
			name:         "incomplete kubernetes tests are not reported",
			agent:        prowv1.KubernetesAgent,
			isComplete:   false,
			shouldReport: false,
		},
		{
			name:         "complete non-kubernetes tests are not reported",
			agent:        prowv1.JenkinsAgent,
			isComplete:   true,
			shouldReport: false,
		},
		{
			name:         "incomplete non-kubernetes tests are not reported",
			agent:        prowv1.JenkinsAgent,
			isComplete:   false,
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

			kgr := internalNew(testutil.Fca{}.Config, nil, nil, 1.0, false)
			shouldReport := kgr.ShouldReport(pj)
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
}

func (rg testResourceGetter) GetPod(cluster, namespace, name string) (*v1.Pod, error) {
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

func TestReportPodInfo(t *testing.T) {
	tests := []struct {
		name         string
		pjName       string
		pod          *v1.Pod
		events       []v1.Event
		dryRun       bool
		expectReport bool
		expectErr    bool
	}{
		{
			name:   "prowjob picks up pod and events",
			pjName: "ba123965-4fd4-421f-8509-7590c129ab69",
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
			name:   "prowjob with no events reports pod",
			pjName: "ba123965-4fd4-421f-8509-7590c129ab69",
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
			expectReport: false,
		},
		{
			name:   "nothing is reported in dryrun mode",
			pjName: "ba123965-4fd4-421f-8509-7590c129ab69",
			dryRun: true,
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
					State:          prowv1.SuccessState,
					StartTime:      metav1.Time{Time: time.Now()},
					CompletionTime: &metav1.Time{Time: time.Now()},
					BuildID:        "12345",
				},
			}

			fca := testutil.Fca{C: config.Config{ProwConfig: config.ProwConfig{
				PodNamespace: "the-test-namespace",
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
				namespace: "the-test-namespace",
				cluster:   "the-build-cluster",
				pod:       tc.pod,
				events:    tc.events,
			}
			author := &testutil.TestAuthor{}
			ctx := context.Background()
			reporter := internalNew(fca.Config, author, rg, 1.0, tc.dryRun)
			err := reporter.reportPodInfo(ctx, pj)

			if tc.expectErr {
				if err == nil {
					t.Fatal("Expected an error, but didn't get one")
				}
				return
			} else if err != nil {
				t.Fatalf("Unexpected error: %v", err)
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
