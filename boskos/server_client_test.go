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

package main

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"net/http/httptest"

	"sort"

	"k8s.io/test-infra/boskos/client"
	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/ranch"
)

// json does not serialized time with nanosecond precision
func now() time.Time {
	format := "2006-01-02 15:04:05.000"
	now, _ := time.Parse(format, time.Now().Format(format))
	return now
}

func makeTestBoskos(r *ranch.Ranch) *httptest.Server {
	handler := NewBoskosHandler(r)
	return httptest.NewServer(handler)
}

func TestAcquireUpdate(t *testing.T) {
	var testcases = []struct {
		name     string
		resource common.Resource
	}{
		{
			name:     "noInfo",
			resource: common.NewResource("test", "type", common.Dirty, "", time.Time{}),
		},
		{
			name: "existingInfo",
			resource: common.Resource{
				Type:     "type",
				Name:     "test",
				State:    common.Dirty,
				UserData: common.UserDataFromMap(common.UserDataMap{"test": "old"}),
			},
		},
	}
	for _, tc := range testcases {
		r := MakeTestRanch([]common.Resource{tc.resource})
		boskos := makeTestBoskos(r)
		owner := "owner"
		client := client.NewClient(owner, boskos.URL)
		userData := common.UserDataFromMap(common.UserDataMap{"test": "new"})

		newState := "acquired"
		receivedRes, err := client.Acquire(tc.resource.Type, tc.resource.State, newState)
		if err != nil {
			t.Error("unable to acquire resource")
			continue
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
	}
}

func TestAcquireByState(t *testing.T) {
	newState := "newState"
	owner := "owner"
	var testcases = []struct {
		name, state         string
		resources, expected []common.Resource
		err                 error
		names               []string
	}{
		{
			name:  "noNames",
			state: "state1",
			resources: []common.Resource{
				common.NewResource("test", "type", common.Dirty, "", time.Time{}),
			},
			err: fmt.Errorf("status 400 Bad Request, status code 400"),
		},
		{
			name:  "noState",
			names: []string{"test"},
			state: "state1",
			resources: []common.Resource{
				common.NewResource("test", "type", common.Dirty, "", time.Time{}),
			},
			err: fmt.Errorf("resources not found"),
		},
		{
			name:  "existing",
			names: []string{"test2", "test3"},
			state: "state1",
			resources: []common.Resource{
				common.NewResource("test1", "type1", common.Dirty, "", time.Time{}),
				common.NewResource("test2", "type2", "state1", "", time.Time{}),
				common.NewResource("test3", "type3", "state1", "", time.Time{}),
				common.NewResource("test4", "type4", common.Dirty, "", time.Time{}),
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
			resources: []common.Resource{
				common.NewResource("test1", "type1", common.Dirty, "", time.Time{}),
				common.NewResource("test2", "type2", "state1", "foo", time.Time{}),
				common.NewResource("test3", "type3", "state1", "foo", time.Time{}),
				common.NewResource("test4", "type4", common.Dirty, "", time.Time{}),
			},
			err: fmt.Errorf("resources not found"),
		},
	}
	for _, tc := range testcases {
		r := MakeTestRanch(tc.resources)
		boskos := makeTestBoskos(r)
		client := client.NewClient(owner, boskos.URL)
		receivedRes, err := client.AcquireByState(tc.state, newState, tc.names)
		boskos.Close()
		if !reflect.DeepEqual(err, tc.err) {
			t.Errorf("tc: %s - errors don't match, expected %v, received\n %v", tc.name, tc.err, err)
			continue
		}
		sort.Sort(common.ResourceByName(receivedRes))
		if !reflect.DeepEqual(receivedRes, tc.expected) {
			t.Errorf("tc: %s - resources should match. Expected \n%v, received \n%v", tc.name, tc.expected, receivedRes)
		}
	}
}

func TestClientServerUpdate(t *testing.T) {
	owner := "owner"

	newResourceWithUD := func(name, rtype, state, owner string, t time.Time, ud common.UserDataMap) common.Resource {
		res := common.NewResource(name, rtype, state, owner, t)
		res.UserData = common.UserDataFromMap(ud)
		return res
	}

	initialState := "state1"
	finalState := "state2"
	rType := "type"
	resourceName := "test"

	var testcases = []struct {
		name               string
		resource, expected common.Resource
		err                error
		names              []string
		ud                 common.UserDataMap
	}{
		{
			name:     "noUserData",
			resource: common.NewResource(resourceName, rType, initialState, "", time.Time{}),
			expected: common.NewResource(resourceName, rType, finalState, owner, fakeNow),
		},
		{
			name:     "userData",
			resource: common.NewResource(resourceName, rType, initialState, "", time.Time{}),
			expected: newResourceWithUD(resourceName, rType, finalState, owner, fakeNow, common.UserDataMap{"custom": "custom"}),
			ud:       common.UserDataMap{"custom": "custom"},
		},
		{
			name:     "newUserData",
			resource: newResourceWithUD(resourceName, rType, initialState, "", fakeNow, common.UserDataMap{"1": "1"}),
			expected: newResourceWithUD(resourceName, rType, finalState, owner, fakeNow, common.UserDataMap{"1": "1", "2": "2"}),
			ud:       common.UserDataMap{"2": "2"},
		},
		{
			name:     "OverRideUserData",
			resource: newResourceWithUD(resourceName, rType, initialState, "", fakeNow, common.UserDataMap{"1": "1"}),
			expected: newResourceWithUD(resourceName, rType, finalState, owner, fakeNow, common.UserDataMap{"1": "2"}),
			ud:       common.UserDataMap{"1": "2"},
		},
		{
			name:     "DeleteUserData",
			resource: newResourceWithUD(resourceName, rType, initialState, "", fakeNow, common.UserDataMap{"1": "1", "2": "2"}),
			expected: newResourceWithUD(resourceName, rType, finalState, owner, fakeNow, common.UserDataMap{"2": "2"}),
			ud:       common.UserDataMap{"1": ""},
		},
	}
	for _, tc := range testcases {
		r := MakeTestRanch([]common.Resource{tc.resource})
		boskos := makeTestBoskos(r)
		client := client.NewClient(owner, boskos.URL)
		_, err := client.Acquire(rType, initialState, finalState)
		if err != nil {
			t.Errorf("failed to acquire resource")
		}
		err = client.UpdateOne(resourceName, finalState, common.UserDataFromMap(tc.ud))
		boskos.Close()
		if !reflect.DeepEqual(err, tc.err) {
			t.Errorf("tc: %s - errors don't match, expected %v, received\n %v", tc.name, tc.err, err)
			continue
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
	}
}
