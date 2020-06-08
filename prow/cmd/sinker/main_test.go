/*
Copyright 2016 The Kubernetes Authors.

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
	"flag"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	corev1api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	corev1fake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/kube"
)

const (
	maxProwJobAge    = 2 * 24 * time.Hour
	maxPodAge        = 12 * time.Hour
	terminatedPodTTL = 30 * time.Minute // must be less than maxPodAge
)

type fca struct {
	c *config.Config
}

func newFakeConfigAgent() *fca {
	return &fca{
		c: &config.Config{
			ProwConfig: config.ProwConfig{
				ProwJobNamespace: "ns",
				PodNamespace:     "ns",
				Sinker: config.Sinker{
					MaxProwJobAge:    &metav1.Duration{Duration: maxProwJobAge},
					MaxPodAge:        &metav1.Duration{Duration: maxPodAge},
					TerminatedPodTTL: &metav1.Duration{Duration: terminatedPodTTL},
				},
			},
			JobConfig: config.JobConfig{
				Periodics: []config.Periodic{
					{JobBase: config.JobBase{Name: "retester"}},
				},
			},
		},
	}

}

func (f *fca) Config() *config.Config {
	return f.c
}

func startTime(s time.Time) *metav1.Time {
	start := metav1.NewTime(s)
	return &start
}

type unreachableCluster struct{}

func (unreachableCluster) Delete(name string, options *metav1.DeleteOptions) error {
	return fmt.Errorf("I can't hear you.")
}
func (unreachableCluster) List(opts metav1.ListOptions) (*corev1api.PodList, error) {
	return nil, fmt.Errorf("I can't hear you.")
}

func TestClean(t *testing.T) {

	pods := []runtime.Object{
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "job-running-pod-failed",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw:  "true",
					kube.ProwJobIDLabel: "job-running",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodFailed,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "job-running-pod-succeeded",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw:  "true",
					kube.ProwJobIDLabel: "job-running",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodSucceeded,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "job-complete-pod-failed",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw:  "true",
					kube.ProwJobIDLabel: "job-complete",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodFailed,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "job-complete-pod-succeeded",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw:  "true",
					kube.ProwJobIDLabel: "job-complete",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodSucceeded,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "job-complete-pod-pending",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw:  "true",
					kube.ProwJobIDLabel: "job-complete",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodPending,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "job-unknown-pod-pending",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw:  "true",
					kube.ProwJobIDLabel: "job-unknown",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodPending,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "job-unknown-pod-failed",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw:  "true",
					kube.ProwJobIDLabel: "job-unknown",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodFailed,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "job-unknown-pod-succeeded",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw:  "true",
					kube.ProwJobIDLabel: "job-unknown",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodSucceeded,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-failed",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodFailed,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-succeeded",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodSucceeded,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-just-complete",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodSucceeded,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-pending",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodPending,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-pending-abort",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodPending,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "new-failed",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodFailed,
				StartTime: startTime(time.Now().Add(-10 * time.Second)),
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "new-running-no-pj",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodRunning,
				StartTime: startTime(time.Now().Add(-10 * time.Second)),
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-running",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodRunning,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unrelated-failed",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw: "not really",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodFailed,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unrelated-complete",
				Namespace: "ns",
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodSucceeded,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ttl-expired",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodFailed,
				StartTime: startTime(time.Now().Add(-terminatedPodTTL * 2)),
				ContainerStatuses: []corev1api.ContainerStatus{
					{
						State: corev1api.ContainerState{
							Terminated: &corev1api.ContainerStateTerminated{
								FinishedAt: metav1.Time{Time: time.Now().Add(-terminatedPodTTL).Add(-time.Second)},
							},
						},
					},
				},
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ttl-not-expired",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodFailed,
				StartTime: startTime(time.Now().Add(-terminatedPodTTL * 2)),
				ContainerStatuses: []corev1api.ContainerStatus{
					{
						State: corev1api.ContainerState{
							Terminated: &corev1api.ContainerStateTerminated{
								FinishedAt: metav1.Time{Time: time.Now().Add(-terminatedPodTTL).Add(-time.Second)},
							},
						},
					},
					{
						State: corev1api.ContainerState{
							Terminated: &corev1api.ContainerStateTerminated{
								FinishedAt: metav1.Time{Time: time.Now().Add(-time.Second)},
							},
						},
					},
				},
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "completed-prowjob-ttl-expired-while-pod-still-pending",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodPending,
				StartTime: startTime(time.Now().Add(-terminatedPodTTL * 2)),
				ContainerStatuses: []corev1api.ContainerStatus{
					{
						State: corev1api.ContainerState{
							Waiting: &corev1api.ContainerStateWaiting{
								Reason: "ImgPullBackoff",
							},
						},
					},
				},
			},
		},
	}
	deletedPods := sets.NewString(
		"job-complete-pod-failed",
		"job-complete-pod-pending",
		"job-complete-pod-succeeded",
		"job-unknown-pod-failed",
		"job-unknown-pod-pending",
		"job-unknown-pod-succeeded",
		"new-running-no-pj",
		"old-failed",
		"old-succeeded",
		"old-pending-abort",
		"old-running",
		"ttl-expired",
		"completed-prowjob-ttl-expired-while-pod-still-pending",
	)
	setComplete := func(d time.Duration) *metav1.Time {
		completed := metav1.NewTime(time.Now().Add(d))
		return &completed
	}
	prowJobs := []runtime.Object{
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "job-complete",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
				CompletionTime: setComplete(-time.Second),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "job-running",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime: metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-failed",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
				CompletionTime: setComplete(-time.Second),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-succeeded",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
				CompletionTime: setComplete(-time.Second),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-just-complete",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime: metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-complete",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
				CompletionTime: setComplete(-time.Second),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-incomplete",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime: metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-pending",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime: metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-pending-abort",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
				CompletionTime: setComplete(-time.Second),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "new",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime: metav1.NewTime(time.Now().Add(-time.Second)),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "newer-periodic",
				Namespace: "ns",
			},
			Spec: prowv1.ProwJobSpec{
				Type: prowv1.PeriodicJob,
				Job:  "retester",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
				CompletionTime: setComplete(-time.Second),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "new-failed",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime: metav1.NewTime(time.Now().Add(-time.Minute)),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "older-periodic",
				Namespace: "ns",
			},
			Spec: prowv1.ProwJobSpec{
				Type: prowv1.PeriodicJob,
				Job:  "retester",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Minute)),
				CompletionTime: setComplete(-time.Minute),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "oldest-periodic",
				Namespace: "ns",
			},
			Spec: prowv1.ProwJobSpec{
				Type: prowv1.PeriodicJob,
				Job:  "retester",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Hour)),
				CompletionTime: setComplete(-time.Hour),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-failed-trusted",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-maxProwJobAge).Add(-time.Second)),
				CompletionTime: setComplete(-time.Second),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ttl-expired",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-terminatedPodTTL * 2)),
				CompletionTime: setComplete(-terminatedPodTTL - time.Second),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ttl-not-expired",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-terminatedPodTTL * 2)),
				CompletionTime: setComplete(-time.Second),
			},
		},
		&prowv1.ProwJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "completed-prowjob-ttl-expired-while-pod-still-pending",
				Namespace: "ns",
			},
			Status: prowv1.ProwJobStatus{
				StartTime:      metav1.NewTime(time.Now().Add(-terminatedPodTTL * 2)),
				CompletionTime: setComplete(-terminatedPodTTL - time.Second),
			},
		},
	}
	deletedProwJobs := sets.NewString(
		"job-complete",
		"old-failed",
		"old-succeeded",
		"old-complete",
		"old-pending-abort",
		"older-periodic",
		"oldest-periodic",
		"old-failed-trusted",
	)
	podsTrusted := []runtime.Object{
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old-failed-trusted",
				Namespace: "ns",
				Labels: map[string]string{
					kube.CreatedByProw: "true",
				},
			},
			Status: corev1api.PodStatus{
				Phase:     corev1api.PodFailed,
				StartTime: startTime(time.Now().Add(-maxPodAge).Add(-time.Second)),
			},
		},
	}
	deletedPodsTrusted := sets.NewString("old-failed-trusted")

	fpjc := fakectrlruntimeclient.NewFakeClient(prowJobs...)
	fkc := []*corev1fake.Clientset{corev1fake.NewSimpleClientset(pods...), corev1fake.NewSimpleClientset(podsTrusted...)}
	fpc := []podInterface{unreachableCluster{}}
	for _, fakeClient := range fkc {
		fpc = append(fpc, fakeClient.CoreV1().Pods("ns"))
	}
	// Run
	c := controller{
		logger:        logrus.WithField("component", "sinker"),
		prowJobClient: fpjc,
		podClients:    fpc,
		config:        newFakeConfigAgent().Config,
	}
	c.clean()
	assertSetsEqual(deletedPods, getDeletedObjectNames(fkc[0].Fake.Actions()), t, "did not delete correct Pods")
	assertSetsEqual(deletedPodsTrusted, getDeletedObjectNames(fkc[1].Fake.Actions()), t, "did not delete correct trusted Pods")

	remainingProwJobs := &prowv1.ProwJobList{}
	if err := fpjc.List(context.Background(), remainingProwJobs); err != nil {
		t.Fatalf("failed to get remaining prowjobs: %v", err)
	}
	actuallyDeletedProwJobs := sets.String{}
	for _, initalProwJob := range prowJobs {
		actuallyDeletedProwJobs.Insert(initalProwJob.(metav1.Object).GetName())
	}
	for _, remainingProwJob := range remainingProwJobs.Items {
		actuallyDeletedProwJobs.Delete(remainingProwJob.Name)
	}
	assertSetsEqual(deletedProwJobs, actuallyDeletedProwJobs, t, "did not delete correct ProwJobs")
}

func getDeletedObjectNames(actions []clienttesting.Action) sets.String {
	names := sets.NewString()
	for _, action := range actions {
		switch action := action.(type) {
		case clienttesting.DeleteActionImpl:
			names.Insert(action.Name)
		}
	}
	return names
}

func assertSetsEqual(expected, actual sets.String, t *testing.T, prefix string) {
	if expected.Equal(actual) {
		return
	}

	if missing := expected.Difference(actual); missing.Len() > 0 {
		t.Errorf("%s: missing expected: %v", prefix, missing.List())
	}
	if extra := actual.Difference(expected); extra.Len() > 0 {
		t.Errorf("%s: found unexpected: %v", prefix, extra.List())
	}
}

func TestFlags(t *testing.T) {
	cases := []struct {
		name     string
		args     map[string]string
		del      sets.String
		expected func(*options)
		err      bool
	}{
		{
			name: "minimal flags work",
		},
		{
			name: "explicitly set --config-path",
			args: map[string]string{
				"--config-path": "/random/path",
			},
			expected: func(o *options) {
				o.configPath = "/random/path"
			},
		},
		{
			name: "expicitly set --dry-run=false",
			args: map[string]string{
				"--dry-run": "false",
			},
			expected: func(o *options) {
				o.dryRun = flagutil.Bool{
					Explicit: true,
				}
			},
		},
		{
			name: "--dry-run=true requires --deck-url",
			args: map[string]string{
				"--dry-run":  "true",
				"--deck-url": "",
			},
			err: true,
		},
		{
			name: "explicitly set --dry-run=true",
			args: map[string]string{
				"--dry-run":  "true",
				"--deck-url": "http://whatever",
			},
			expected: func(o *options) {
				o.dryRun = flagutil.Bool{
					Value:    true,
					Explicit: true,
				}
				o.kubernetes.DeckURI = "http://whatever"
			},
		},
		{
			name: "dry run defaults to false", // TODO(fejta): change to true in April
			del:  sets.NewString("--dry-run"),
			expected: func(o *options) {
				o.dryRun = flagutil.Bool{}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expected := &options{
				configPath: "yo",
				dryRun: flagutil.Bool{
					Explicit: true,
				},
				instrumentationOptions: flagutil.InstrumentationOptions{
					MetricsPort: flagutil.DefaultMetricsPort,
					PProfPort:   flagutil.DefaultPProfPort,
				},
			}
			if tc.expected != nil {
				tc.expected(expected)
			}

			argMap := map[string]string{
				"--config-path": "yo",
				"--dry-run":     "false",
			}
			for k, v := range tc.args {
				argMap[k] = v
			}
			for k := range tc.del {
				delete(argMap, k)
			}

			var args []string
			for k, v := range argMap {
				args = append(args, k+"="+v)
			}
			fs := flag.NewFlagSet("fake-flags", flag.PanicOnError)
			actual := gatherOptions(fs, args...)
			switch err := actual.Validate(); {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("failed to receive expected error")
			case !reflect.DeepEqual(*expected, actual):
				t.Errorf("%#v != expected %#v", actual, *expected)
			}
		})
	}
}

func TestDeletePodToleratesNotFound(t *testing.T) {
	client := corev1fake.NewSimpleClientset().CoreV1().Pods("default")
	m := &sinkerReconciliationMetrics{}
	c := &controller{}
	l := logrus.NewEntry(logrus.New())

	c.deletePod(l, "i-do-not-exist", "reason", client, m)

	if n := len(m.podRemovalErrors); n != 0 {
		t.Errorf("Expected no pod removal errors, got %v", m.podRemovalErrors)
	}
}
