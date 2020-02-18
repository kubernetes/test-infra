/*
Copyright 2017 The Kubernetes Authors.

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

package handlers

import (
	"fmt"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/test-infra/boskos/client"
	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/crds"
	"k8s.io/test-infra/boskos/ranch"
)

func makeTestBoskos(r *ranch.Ranch) *httptest.Server {
	handler := NewBoskosHandler(r)
	return httptest.NewServer(handler)
}

func TestAcquireUpdate(t *testing.T) {
	var testcases = []struct {
		name     string
		resource *crds.ResourceObject
	}{
		{
			name:     "noInfo",
			resource: newResource("test", "type", common.Dirty, "", time.Time{}),
		},
		{
			name: "existingInfo",
			resource: &crds.ResourceObject{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: crds.ResourceSpec{
					Type: "type",
				},
				Status: crds.ResourceStatus{
					State:    common.Dirty,
					UserData: common.UserDataFromMap(common.UserDataMap{"test": "old"}),
				},
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			r := MakeTestRanch([]runtime.Object{tc.resource})
			boskos := makeTestBoskos(r)
			owner := "owner"
			client, err := client.NewClient(owner, boskos.URL, "", "")
			if err != nil {
				t.Fatalf("failed to create the Boskos client")
			}
			userData := common.UserDataFromMap(common.UserDataMap{"test": "new"})

			newState := "acquired"
			receivedRes, err := client.Acquire(tc.resource.Spec.Type, tc.resource.Status.State, newState)
			if err != nil {
				t.Fatalf("unable to acquire resource: %v", err)

			}
			if receivedRes.State != newState || receivedRes.Owner != owner {
				t.Errorf("resource should match. Expected \n%v, received \n%v", tc.resource, receivedRes)
			}
			if err = client.UpdateOne(receivedRes.Name, receivedRes.State, userData); err != nil {
				t.Errorf("unable to update resource. %v", err)
			}
			boskos.Close()
			updatedResource, err := r.Storage.GetResource(tc.resource.Name)
			if err != nil {
				t.Error("unable to list resources")
			}
			if !reflect.DeepEqual(updatedResource.UserData.ToMap(), userData.ToMap()) {
				t.Errorf("info should match. Expected \n%v, received \n%v", userData.ToMap(), updatedResource.UserData.ToMap())
			}
		})
	}
}

func TestAcquireByState(t *testing.T) {
	newState := "newState"
	owner := "owner"
	var testcases = []struct {
		name, state string
		resources   []runtime.Object
		expected    []common.Resource
		err         error
		names       []string
	}{
		{
			name:  "noNames",
			state: "state1",
			resources: []runtime.Object{
				newResource("test", "type", common.Dirty, "", time.Time{}),
			},
			err: fmt.Errorf("status 400 Bad Request, status code 400"),
		},
		{
			name:  "noState",
			names: []string{"test"},
			state: "state1",
			resources: []runtime.Object{
				newResource("test", "type", common.Dirty, "", time.Time{}),
			},
			err: fmt.Errorf("resources not found"),
		},
		{
			name:  "existing",
			names: []string{"test2", "test3"},
			state: "state1",
			resources: []runtime.Object{
				newResource("test1", "type1", common.Dirty, "", time.Time{}),
				newResource("test2", "type2", "state1", "", time.Time{}),
				newResource("test3", "type3", "state1", "", time.Time{}),
				newResource("test4", "type4", common.Dirty, "", time.Time{}),
			},
			expected: []common.Resource{
				common.NewResource("test2", "type2", newState, owner, fakeNow),
				common.NewResource("test3", "type3", newState, owner, fakeNow),
			},
		},
		{
			name:  "alreadyOwned",
			names: []string{"test2", "test3"},
			state: "state1",
			resources: []runtime.Object{
				newResource("test1", "type1", common.Dirty, "", time.Time{}),
				newResource("test2", "type2", "state1", "foo", time.Time{}),
				newResource("test3", "type3", "state1", "foo", time.Time{}),
				newResource("test4", "type4", common.Dirty, "", time.Time{}),
			},
			err: fmt.Errorf("resources not found"),
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			r := MakeTestRanch(tc.resources)
			boskos := makeTestBoskos(r)
			client, err := client.NewClient(owner, boskos.URL, "", "")
			if err != nil {
				t.Fatalf("failed to create the Boskos client")
			}
			receivedRes, err := client.AcquireByState(tc.state, newState, tc.names)
			boskos.Close()
			if !reflect.DeepEqual(err, tc.err) {
				t.Fatalf("tc: %s - errors don't match, expected %v, received\n %v", tc.name, tc.err, err)
			}
			sort.Sort(common.ResourceByName(receivedRes))
			if !reflect.DeepEqual(receivedRes, tc.expected) {
				t.Errorf("tc: %s - resources should match. Expected \n%v, received \n%v", tc.name, tc.expected, receivedRes)
			}
		})
	}
}

func TestClientServerUpdate(t *testing.T) {
	owner := "owner"

	newResourceWithUD := func(name, rtype, state, owner string, t time.Time, ud common.UserDataMap) common.Resource {
		res := common.NewResource(name, rtype, state, owner, t)
		res.UserData = common.UserDataFromMap(ud)
		return res
	}
	newCRDResourceWithUD := func(name, rtype, state, owner string, t time.Time, ud common.UserDataMap) *crds.ResourceObject {
		res := newResource(name, rtype, state, owner, t)
		res.Status.UserData = common.UserDataFromMap(ud)
		return res
	}

	initialState := "state1"
	finalState := "state2"
	rType := "type"
	resourceName := "test"

	var testcases = []struct {
		name     string
		resource *crds.ResourceObject
		expected common.Resource
		err      error
		names    []string
		ud       common.UserDataMap
	}{
		{
			name:     "noUserData",
			resource: newResource(resourceName, rType, initialState, "", time.Time{}),
			expected: common.NewResource(resourceName, rType, finalState, owner, fakeNow),
		},
		{
			name:     "userData",
			resource: newResource(resourceName, rType, initialState, "", time.Time{}),
			expected: newResourceWithUD(resourceName, rType, finalState, owner, fakeNow, common.UserDataMap{"custom": "custom"}),
			ud:       common.UserDataMap{"custom": "custom"},
		},
		{
			name:     "newUserData",
			resource: newCRDResourceWithUD(resourceName, rType, initialState, "", fakeNow, common.UserDataMap{"1": "1"}),
			expected: newResourceWithUD(resourceName, rType, finalState, owner, fakeNow, common.UserDataMap{"1": "1", "2": "2"}),
			ud:       common.UserDataMap{"2": "2"},
		},
		{
			name:     "OverRideUserData",
			resource: newCRDResourceWithUD(resourceName, rType, initialState, "", fakeNow, common.UserDataMap{"1": "1"}),
			expected: newResourceWithUD(resourceName, rType, finalState, owner, fakeNow, common.UserDataMap{"1": "2"}),
			ud:       common.UserDataMap{"1": "2"},
		},
		{
			name:     "DeleteUserData",
			resource: newCRDResourceWithUD(resourceName, rType, initialState, "", fakeNow, common.UserDataMap{"1": "1", "2": "2"}),
			expected: newResourceWithUD(resourceName, rType, finalState, owner, fakeNow, common.UserDataMap{"2": "2"}),
			ud:       common.UserDataMap{"1": ""},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			r := MakeTestRanch([]runtime.Object{tc.resource})
			boskos := makeTestBoskos(r)
			client, err := client.NewClient(owner, boskos.URL, "", "")
			if err != nil {
				t.Fatalf("failed to create the Boskos client")
			}
			_, err = client.Acquire(rType, initialState, finalState)
			if err != nil {
				t.Errorf("failed to acquire resource")
			}
			err = client.UpdateOne(resourceName, finalState, common.UserDataFromMap(tc.ud))
			boskos.Close()
			if !reflect.DeepEqual(err, tc.err) {
				t.Fatalf("tc: %s - errors don't match, expected %v, received\n %v", tc.name, tc.err, err)
			}
			receivedRes, _ := r.Storage.GetResource(tc.resource.Name)
			if !reflect.DeepEqual(receivedRes.UserData.ToMap(), tc.expected.UserData.ToMap()) {
				t.Errorf("tc: %s - resources user data should match. Expected \n%v, received \n%v", tc.name, tc.expected.UserData.ToMap(), receivedRes.UserData.ToMap())
			}
			// Hack: remove UserData to be able to compare since we already compared it before.
			receivedRes.UserData = nil
			tc.expected.UserData = nil
			if !reflect.DeepEqual(receivedRes, tc.expected) {
				t.Errorf("tc: %s - resources should match. Expected \n%v, received \n%v", tc.name, tc.expected, receivedRes)
			}
		})
	}
}

func newResource(name, rtype, state, owner string, t time.Time) *crds.ResourceObject {
	if state == "" {
		state = common.Free
	}

	return &crds.ResourceObject{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: crds.ResourceSpec{
			Type: rtype,
		},
		Status: crds.ResourceStatus{
			State:      state,
			Owner:      owner,
			LastUpdate: t,
			UserData:   &common.UserData{},
		},
	}
}
