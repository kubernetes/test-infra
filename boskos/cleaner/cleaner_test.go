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

package cleaner

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/crds"
	"k8s.io/test-infra/boskos/mason"
	"k8s.io/test-infra/boskos/ranch"
)

const (
	testOwner      = "cleaner"
	testWaitPeriod = time.Millisecond
	testTTL        = time.Millisecond
	testNS         = "test"
)

type releasedResource struct {
	name, state string
}

// fakeBoskos implements boskosClient
var _ boskosClient = &fakeBoskos{}

type fakeBoskos struct {
	ranch *ranch.Ranch
}

// Create a fake client
func createFakeBoskos(objects ...runtime.Object) (*ranch.Storage, boskosClient, chan releasedResource) {
	for _, obj := range objects {
		obj.(metav1.Object).SetNamespace(testNS)
	}
	names := make(chan releasedResource, 100)
	s := ranch.NewStorage(context.Background(), fakectrlruntimeclient.NewFakeClient(objects...), testNS)
	r, _ := ranch.NewRanch("", s, testTTL)

	return s, &fakeBoskos{ranch: r}, names
}

func (fb *fakeBoskos) Acquire(rtype, state, dest string) (*common.Resource, error) {
	crdRes, err := fb.ranch.Acquire(rtype, state, dest, testOwner, "")
	if err != nil {
		return nil, err
	}
	res := crdRes.ToResource()
	return &res, nil
}

func (fb *fakeBoskos) AcquireByState(state, dest string, names []string) ([]common.Resource, error) {
	resList, err := fb.ranch.AcquireByState(state, dest, testOwner, names)
	// Not an oversight, this should return resources even on error
	var res []common.Resource
	for _, item := range resList {
		res = append(res, item.ToResource())
	}

	return res, err
}

func (fb *fakeBoskos) ReleaseOne(name, dest string) error {
	return fb.ranch.Release(name, dest, testOwner)
}

func (fb *fakeBoskos) UpdateOne(name, state string, userData *common.UserData) error {
	return fb.ranch.Update(name, testOwner, state, userData)
}

func (fb *fakeBoskos) ReleaseAll(state string) error {
	// not used in this test
	return nil
}

func testResource(name, rType, state, owner string, leasedResources []string) *crds.ResourceObject {
	res := common.NewResource(name, rType, state, owner, time.Now())
	res.UserData = &common.UserData{}
	res.UserData.Set(mason.LeasedResources, &leasedResources)
	return crds.FromResource(res)
}

func testDRLC(rType string) common.DynamicResourceLifeCycle {
	drlc := common.DynamicResourceLifeCycle{
		Type:     rType,
		MinCount: 10,
		MaxCount: 20,
	}
	return drlc
}

func TestRecycleResources(t *testing.T) {
	for _, tc := range []struct {
		name           string
		resources      []runtime.Object
		expectedStates map[string]string
	}{
		{
			name: "nothingToDo",
			resources: []runtime.Object{
				testResource("static_3", "static", common.Free, "", nil),
			},
			expectedStates: map[string]string{
				"static_3": common.Free,
			},
		},
		{
			name: "noLeasedResources",
			resources: []runtime.Object{
				testResource("static_1", "static", "dynamic_1", "", nil),
				testResource("static_2", "static", "dynamic_1", "", nil),
				testResource("static_3", "static", common.Free, "", nil),
				testResource("dynamic_1", "dynamic", common.Free, "", []string{"static_1", "static_2"}),
				testResource("dynamic_2", "dynamic", common.ToBeDeleted, "", nil),
				&crds.DRLCObject{
					ObjectMeta: metav1.ObjectMeta{Name: "dynamic"},
					Spec: crds.DRLCSpec{
						MinCount: 2,
						MaxCount: 2,
					},
				},
			},
			expectedStates: map[string]string{
				"static_1":  "dynamic_1",
				"static_2":  "dynamic_1",
				"static_3":  common.Free,
				"dynamic_1": common.Free,
				"dynamic_2": common.Tombstone,
			},
		},
		{
			name: "leasedResources",
			resources: []runtime.Object{
				testResource("static_1", "static", "dynamic_1", "", nil),
				testResource("static_2", "static", "dynamic_1", "", nil),
				testResource("static_3", "static", "dynamic_2", "", nil),
				testResource("dynamic_1", "dynamic", common.ToBeDeleted, "", []string{"static_1", "static_2"}),
				testResource("dynamic_2", "dynamic", common.ToBeDeleted, "", []string{"static_3"}),
				&crds.DRLCObject{
					ObjectMeta: metav1.ObjectMeta{Name: "dynamic"},
					Spec: crds.DRLCSpec{
						MinCount: 2,
						MaxCount: 2,
					},
				},
			},
			expectedStates: map[string]string{
				"static_1":  common.Dirty,
				"static_2":  common.Dirty,
				"static_3":  common.Dirty,
				"dynamic_1": common.Tombstone,
				"dynamic_2": common.Tombstone,
			},
		},
		{
			name: "missingLeasedResource",
			resources: []runtime.Object{
				testResource("static_1", "static", "dynamic_1", "", nil),
				testResource("static_2", "static", common.Free, "", nil),
				testResource("static_3", "static", common.Free, "", nil),
				testResource("dynamic_1", "dynamic", common.ToBeDeleted, "", []string{"static_1", "static_2"}),
				testResource("dynamic_2", "dynamic", common.ToBeDeleted, "", []string{"static_3"}),
				&crds.DRLCObject{
					ObjectMeta: metav1.ObjectMeta{Name: "dynamic"},
					Spec: crds.DRLCSpec{
						MinCount: 2,
						MaxCount: 2,
					},
				},
			},
			expectedStates: map[string]string{
				"static_1":  common.Dirty,
				"static_2":  common.Free,
				"static_3":  common.Free,
				"dynamic_1": common.Tombstone,
				"dynamic_2": common.Tombstone,
			},
		},
	} {
		t.Run(tc.name, func(t1 *testing.T) {
			rStorage, mClient, _ := createFakeBoskos(tc.resources...)
			c := NewCleaner(5, mClient, testWaitPeriod, rStorage)
			c.Start()
			time.Sleep(50 * time.Millisecond)
			for name, state := range tc.expectedStates {
				existingRes, err := rStorage.GetResource(name)
				if err != nil {
					t1.Errorf("unable to find resource %s. %v", name, err)
				}
				if existingRes.Status.State != state {
					t1.Errorf("resource %s state %s does not match expected %s", name, existingRes.Status.State, state)
				}
			}
			// Terminating cleaner
			done := make(chan bool)
			go func() {
				c.Stop()
				done <- true
			}()
			select {
			case <-time.After(50 * time.Millisecond):
				t1.Errorf("unable to stop cleaner")
			case <-done:
			}
		})
	}
}
