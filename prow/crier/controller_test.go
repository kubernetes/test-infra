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
	"reflect"
	"sort"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/client/clientset/versioned/fake"
	prowLister "k8s.io/test-infra/prow/client/listers/prowjobs/v1"
	"k8s.io/test-infra/prow/kube"
)

const (
	testTimeout    = time.Second
	controllerName = "CrierTest"
	reporterName   = "fakeReporter"
)

// Fake Reporter
// Sets: Which jobs should be reported
// Asserts: Which jobs are actually reported
type fakeReporter struct {
	reported         []string
	shouldReportFunc func(pj *prowv1.ProwJob) bool
}

func (f *fakeReporter) Report(pj *prowv1.ProwJob) ([]*prowv1.ProwJob, error) {
	f.reported = append(f.reported, pj.Spec.Job)
	return []*prowv1.ProwJob{pj}, nil
}

func (f *fakeReporter) GetName() string {
	return reporterName
}

func (f *fakeReporter) ShouldReport(pj *prowv1.ProwJob) bool {
	return f.shouldReportFunc(pj)
}

// Fake Informer
// Sets: The Prow Job Test Cases
type fakeInformer struct {
	jobs map[string]*prowv1.ProwJob
}

func (f fakeInformer) Get(name string) (*prowv1.ProwJob, error) {
	pj, found := f.jobs[name]
	if !found {
		var s schema.GroupResource
		return nil, errors.NewNotFound(s, "Can't Find ProwJob")
	}
	return pj, nil
}

func (f fakeInformer) ProwJobs(namespace string) prowLister.ProwJobNamespaceLister {
	return f
}

func (f fakeInformer) Informer() cache.SharedIndexInformer {
	return f
}

func (f fakeInformer) Lister() prowLister.ProwJobLister {
	return f
}

func (fakeInformer) HasSynced() bool {
	return true
}

func (fakeInformer) AddEventHandler(handler cache.ResourceEventHandler)                        {}
func (fakeInformer) Run(stopCh <-chan struct{})                                                {}
func (fakeInformer) AddEventHandlerWithResyncPeriod(cache.ResourceEventHandler, time.Duration) {}
func (fakeInformer) GetStore() cache.Store                                                     { return nil }
func (fakeInformer) GetController() cache.Controller                                           { return nil }
func (fakeInformer) LastSyncResourceVersion() string                                           { return "" }
func (fakeInformer) AddIndexers(indexers cache.Indexers) error                                 { return nil }
func (fakeInformer) GetIndexer() cache.Indexer                                                 { return nil }
func (fakeInformer) List(selector labels.Selector) (ret []*prowv1.ProwJob, err error) {
	return nil, nil
}

func TestController_Run(t *testing.T) {
	tests := []struct {
		name         string
		jobsOnQueue  []string
		knownJobs    map[string]*prowv1.ProwJob
		shouldReport bool

		expectReports []string
		expectPatch   bool
	}{
		{
			name:        "reports/patches known job",
			jobsOnQueue: []string{"foo"},
			knownJobs: map[string]*prowv1.ProwJob{
				"foo": {
					Spec: prowv1.ProwJobSpec{
						Job:    "foo",
						Report: true,
					},
					Status: prowv1.ProwJobStatus{
						State: prowv1.TriggeredState,
					},
				},
			},
			shouldReport:  true,
			expectReports: []string{"foo"},
			expectPatch:   true,
		},
		{
			name:        "doesn't report when it shouldn't",
			jobsOnQueue: []string{"foo"},
			knownJobs: map[string]*prowv1.ProwJob{
				"foo": {
					Spec: prowv1.ProwJobSpec{
						Job:    "foo",
						Report: true,
					},
					Status: prowv1.ProwJobStatus{
						State: prowv1.TriggeredState,
					},
				},
			},
			shouldReport: false,
		},
		{
			name:         "doesn't report nonexistant job",
			jobsOnQueue:  []string{"foo"},
			knownJobs:    map[string]*prowv1.ProwJob{},
			shouldReport: true,
		},
		//{
		//	name: "nil job panics",
		//	jobsOnQueue: []string{"foo"},
		//	knownJobs: map[string]*prowv1.ProwJob{
		//		"foo" : nil,
		//	},
		//	shouldReport: func(*prowv1.ProwJob) bool {
		//		return true
		//	},
		//},
		{
			name:        "doesn't report when SkipReport=true (i.e. Spec.Report=false)",
			jobsOnQueue: []string{"foo"},
			knownJobs: map[string]*prowv1.ProwJob{
				"foo": {
					Spec: prowv1.ProwJobSpec{
						Job:    "foo",
						Report: false,
					},
				},
			},
			shouldReport: false,
		},
		{
			name:        "doesn't report empty job",
			jobsOnQueue: []string{"foo"},
			knownJobs: map[string]*prowv1.ProwJob{
				"foo": {},
			},
			shouldReport: true,
		},
		{
			name:        "duplicate jobs report once",
			jobsOnQueue: []string{"foo", "foo", "foo"},
			knownJobs: map[string]*prowv1.ProwJob{
				"foo": {
					Spec: prowv1.ProwJobSpec{
						Job:    "foo",
						Report: true,
					},
					Status: prowv1.ProwJobStatus{
						State: prowv1.TriggeredState,
					},
				},
			},
			shouldReport:  true,
			expectReports: []string{"foo"},
			expectPatch:   true,
		},
		{
			name:        "previously-reported job isn't reported",
			jobsOnQueue: []string{"foo"},
			knownJobs: map[string]*prowv1.ProwJob{
				"foo": {
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
			},
			shouldReport: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			q := kube.RateLimiter(controllerName)
			for _, job := range test.jobsOnQueue {
				q.Add(job)
			}

			inf := fakeInformer{
				jobs: test.knownJobs,
			}

			rp := fakeReporter{
				shouldReportFunc: func(*prowv1.ProwJob) bool {
					return test.shouldReport
				},
			}

			var prowjobs []runtime.Object
			for _, job := range test.knownJobs {
				prowjobs = append(prowjobs, job)
			}
			cs := fake.NewSimpleClientset(prowjobs...)
			nmwrk := 2
			c := NewController(cs, q, inf, &rp, nmwrk)

			done := make(chan struct{}, 1)
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				c.Run(ctx)
				close(done)
			}()

			wait.Poll(10*time.Millisecond, testTimeout, func() (done bool, err error) {
				return c.queue.Len() == 0, nil
			})

			cancel()
			<-done

			if c.queue.Len() != 0 {
				t.Errorf("%d messages were unconsumed", c.queue.Len())
			}

			sort.Strings(test.expectReports)
			sort.Strings(rp.reported)
			if !reflect.DeepEqual(test.expectReports, rp.reported) {
				t.Errorf("mismatch report: wants %v, got %v", test.expectReports, rp.reported)
			}

			if (len(cs.Actions()) != 0) != test.expectPatch {
				if test.expectPatch {
					t.Errorf("expected patch, but didn't get it")
				} else {
					t.Errorf("patch: did not expect %v", cs.Actions())
				}
			}
		})
	}
}
