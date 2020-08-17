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
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/clock"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func TestProwJobLifecycleCollectorUpdate(t *testing.T) {
	fakeClock := clock.NewFakeClock(time.Now())
	defaultCompleteTime := v1.NewTime(fakeClock.Now().Add(1 * time.Hour))
	defaultPendingTime := v1.NewTime(fakeClock.Now().Add(30 * time.Minute))
	type args struct {
		oldJob *prowapi.ProwJob
		newJob *prowapi.ProwJob
	}
	type expected struct {
		collected []dto.Metric
	}
	tests := []struct {
		oldJobStates []prowapi.ProwJobState
		newJobStates []prowapi.ProwJobState
		name         string
		args         args
		expected     expected
	}{
		{name: "should not collect job without history when no state change was observed for state %s to %s",
			oldJobStates: []prowapi.ProwJobState{
				prowapi.TriggeredState,
				prowapi.PendingState,
				prowapi.SuccessState,
				prowapi.FailureState,
				prowapi.ErrorState,
				prowapi.AbortedState,
			},
			newJobStates: []prowapi.ProwJobState{
				prowapi.TriggeredState,
				prowapi.PendingState,
				prowapi.SuccessState,
				prowapi.FailureState,
				prowapi.ErrorState,
				prowapi.AbortedState,
			},
			args: args{
				oldJob: &prowapi.ProwJob{
					ObjectMeta: v1.ObjectMeta{
						UID:               "1234",
						CreationTimestamp: v1.NewTime(fakeClock.Now()),
					},
					Status: prowapi.ProwJobStatus{
						CompletionTime: &defaultCompleteTime,
						PendingTime:    &defaultPendingTime,
					},
				},
				newJob: &prowapi.ProwJob{
					ObjectMeta: v1.ObjectMeta{
						UID:               "1234",
						CreationTimestamp: v1.NewTime(fakeClock.Now()),
					},
					Status: prowapi.ProwJobStatus{
						CompletionTime: &defaultCompleteTime,
						PendingTime:    &defaultPendingTime,
					},
				},
			}, expected: expected{
				collected: nil,
			}},
		{name: "should collect job transitions for transitions from %s to  %s",
			oldJobStates: []prowapi.ProwJobState{
				prowapi.TriggeredState,
				prowapi.TriggeredState,
				prowapi.TriggeredState,
				prowapi.PendingState,
				prowapi.PendingState,
				prowapi.PendingState,
				prowapi.PendingState,
			},
			newJobStates: []prowapi.ProwJobState{
				prowapi.PendingState,
				prowapi.ErrorState,
				prowapi.AbortedState,
				prowapi.SuccessState,
				prowapi.FailureState,
				prowapi.ErrorState,
				prowapi.AbortedState,
			},
			args: args{
				oldJob: &prowapi.ProwJob{
					ObjectMeta: v1.ObjectMeta{
						UID:               "1234",
						CreationTimestamp: v1.NewTime(fakeClock.Now().Add(-time.Hour)),
					},
					Status: prowapi.ProwJobStatus{
						CompletionTime: &defaultCompleteTime,
						PendingTime:    &defaultPendingTime,
						State:          prowapi.TriggeredState,
					},
				},
				newJob: &prowapi.ProwJob{
					ObjectMeta: v1.ObjectMeta{
						UID:               "1234",
						Name:              "testjob",
						Namespace:         "testnamespace",
						CreationTimestamp: v1.NewTime(fakeClock.Now().Add(-time.Hour)),
					},
					Spec: prowapi.ProwJobSpec{
						Job:  "testjob",
						Type: prowapi.PeriodicJob,
						Refs: &prowapi.Refs{
							Org:     "testorg",
							Repo:    "testrepo",
							BaseRef: "master",
						},
					},
					Status: prowapi.ProwJobStatus{
						State:          prowapi.PendingState,
						CompletionTime: &defaultCompleteTime,
						PendingTime:    &defaultPendingTime,
					},
				},
			}, expected: expected{
				collected: []dto.Metric{{Label: []*dto.LabelPair{
					toLabelPair("base_ref", "master"),
					toLabelPair("job_name", "testjob"),
					toLabelPair("job_namespace", "testnamespace"),
					toLabelPair("last_state", string(prowapi.TriggeredState)),
					toLabelPair("org", "testorg"),
					toLabelPair("repo", "testrepo"),
					toLabelPair("state", string(prowapi.PendingState)),
					toLabelPair("type", string(prowapi.PeriodicJob)),
				}}},
			}},
		{name: "should collect job transitions for transitions from %s to  %s from extraRefs",
			oldJobStates: []prowapi.ProwJobState{
				prowapi.TriggeredState,
				prowapi.TriggeredState,
				prowapi.TriggeredState,
				prowapi.PendingState,
				prowapi.PendingState,
				prowapi.PendingState,
				prowapi.PendingState,
			},
			newJobStates: []prowapi.ProwJobState{
				prowapi.PendingState,
				prowapi.ErrorState,
				prowapi.AbortedState,
				prowapi.SuccessState,
				prowapi.FailureState,
				prowapi.ErrorState,
				prowapi.AbortedState,
			},
			args: args{
				oldJob: &prowapi.ProwJob{
					ObjectMeta: v1.ObjectMeta{
						UID:               "1234",
						CreationTimestamp: v1.NewTime(fakeClock.Now().Add(-time.Hour)),
					},
					Status: prowapi.ProwJobStatus{
						State:          prowapi.TriggeredState,
						CompletionTime: &defaultCompleteTime,
						PendingTime:    &defaultPendingTime,
					},
				},
				newJob: &prowapi.ProwJob{
					ObjectMeta: v1.ObjectMeta{
						UID:               "1234",
						Name:              "testjob",
						Namespace:         "testnamespace",
						CreationTimestamp: v1.NewTime(fakeClock.Now().Add(-time.Hour)),
					},
					Spec: prowapi.ProwJobSpec{
						Job:  "testjob",
						Type: prowapi.PeriodicJob,
						ExtraRefs: []prowapi.Refs{{
							Org:     "testorg",
							Repo:    "testrepo",
							BaseRef: "master",
						}},
					},
					Status: prowapi.ProwJobStatus{
						CompletionTime: &defaultCompleteTime,
						PendingTime:    &defaultPendingTime,
						State:          prowapi.PendingState,
					},
				},
			}, expected: expected{
				collected: []dto.Metric{{Label: []*dto.LabelPair{
					toLabelPair("base_ref", "master"),
					toLabelPair("job_name", "testjob"),
					toLabelPair("job_namespace", "testnamespace"),
					toLabelPair("last_state", string(prowapi.TriggeredState)),
					toLabelPair("org", "testorg"),
					toLabelPair("repo", "testrepo"),
					toLabelPair("state", string(prowapi.PendingState)),
					toLabelPair("type", string(prowapi.PeriodicJob)),
				}}},
			}},
	}
	for _, tt := range tests {
		for x := 0; x < len(tt.oldJobStates); x++ {
			t.Run(fmt.Sprintf(tt.name, tt.oldJobStates[x], tt.newJobStates[x]), func(t *testing.T) {
				histogramVec := newHistogramVec()
				tt.args.oldJob.Status.State = tt.oldJobStates[x]
				tt.args.newJob.Status.State = tt.newJobStates[x]
				update(histogramVec, tt.args.oldJob, tt.args.newJob)
				assertMetrics(t, collect(histogramVec), tt.expected.collected, tt.oldJobStates[x], tt.newJobStates[x])
			})
		}
	}
}

func collect(histogram *prometheus.HistogramVec) []dto.Metric {
	metrics := make(chan prometheus.Metric, 1000)
	histogram.Collect(metrics)
	close(metrics)
	var collected []dto.Metric
	for metric := range metrics {
		m := dto.Metric{}
		metric.Write(&m)
		collected = append(collected, m)
	}
	return collected
}

func assertMetrics(t *testing.T, actual, expected []dto.Metric, lastState prowapi.ProwJobState, state prowapi.ProwJobState) {
	if len(actual) != len(expected) {
		t.Errorf("actual length differs from expected: %v, %v", len(actual), len(expected))
		return
	}
	for x := 0; x < len(actual); x++ {
		expected[x].Label[3] = toLabelPair("last_state", string(lastState))
		expected[x].Label[6] = toLabelPair("state", string(state))
		if !reflect.DeepEqual(actual[x].Label, expected[x].Label) {
			t.Errorf("actual differs from expected:\n%s", cmp.Diff(expected[x].Label, actual[x].Label))
		}
	}
}

func toLabelPair(name, value string) *dto.LabelPair {
	return &dto.LabelPair{Name: &name, Value: &value}
}
