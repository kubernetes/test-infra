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
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/go-test/deep"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/test-infra/boskos/client"
	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/crds"
	"k8s.io/test-infra/boskos/ranch"
)

func makeTestBoskos(t *testing.T, r *ranch.Ranch) *httptest.Server {
	handler := &testMuxWrapper{t: t, ServeMux: NewBoskosHandler(r)}
	return httptest.NewServer(handler)
}

type testMuxWrapper struct {
	t *testing.T
	*http.ServeMux
	requstCount int
}

func (tmw *testMuxWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tmw.requstCount++
	requestBody, err := ioutil.ReadAll(r.Body)
	if err != nil {
		tmw.t.Fatalf("failed to read request body: %v", err)
	}
	r.Body = ioutil.NopCloser(bytes.NewBuffer(requestBody))
	if err := compareWithFixture(fmt.Sprintf("%s-request-%d", tmw.t.Name(), tmw.requstCount), requestBody); err != nil {
		tmw.t.Errorf("data differs from fixture: %v", err)
	}
	tmw.ServeMux.ServeHTTP(&bodyLoggingHTTPWriter{t: tmw.t, ResponseWriter: w}, r)
}

type bodyLoggingHTTPWriter struct {
	t *testing.T
	http.ResponseWriter
}

func (blhw *bodyLoggingHTTPWriter) Write(data []byte) (int, error) {
	if err := compareWithFixture(blhw.t.Name()+"-response", data); err != nil {
		blhw.t.Errorf("data differs from fixture: %v", err)
	}
	return blhw.ResponseWriter.Write(data)
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
			boskos := makeTestBoskos(t, r)
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
			if !reflect.DeepEqual(updatedResource.Status.UserData.ToMap(), userData.ToMap()) {
				t.Errorf("info should match. Expected \n%v, received \n%v", userData.ToMap(), updatedResource.Status.UserData.ToMap())
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
			boskos := makeTestBoskos(t, r)
			client, err := client.NewClient(owner, boskos.URL, "", "")
			if err != nil {
				t.Fatalf("failed to create the Boskos client")
			}
			receivedRes, err := client.AcquireByState(tc.state, newState, tc.names)
			boskos.Close()
			if fmt.Sprintf("%v", tc.err) != fmt.Sprintf("%v", err) {
				t.Fatalf("tc: %s - errors don't match, expected %v, received\n %v", tc.name, tc.err, err)
			}
			sort.Sort(common.ResourceByName(receivedRes))

			// Make sure the comparison doesn't bail on nil != empty
			for idx := range tc.expected {
				if tc.expected[idx].UserData == nil {
					tc.expected[idx].UserData = &common.UserData{}
				}
			}
			if diff := deep.Equal(receivedRes, tc.expected); diff != nil {
				t.Errorf("receivedRes differ from expected, diff: %v", diff)
			}
		})
	}
}

func TestClientServerUpdate(t *testing.T) {
	owner := "owner"

	newResourceWithUD := func(name, rtype, state, owner string, t time.Time, ud common.UserDataMap) *crds.ResourceObject {
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
		expected *crds.ResourceObject
		err      error
		names    []string
		ud       common.UserDataMap
	}{
		{
			name:     "noUserData",
			resource: newResource(resourceName, rType, initialState, "", time.Time{}),
			expected: newResource(resourceName, rType, finalState, owner, fakeNow),
		},
		{
			name:     "userData",
			resource: newResource(resourceName, rType, initialState, "", time.Time{}),
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
		t.Run(tc.name, func(t *testing.T) {
			r := MakeTestRanch([]runtime.Object{tc.resource})
			boskos := makeTestBoskos(t, r)
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
			if !reflect.DeepEqual(receivedRes.Status.UserData.ToMap(), tc.expected.Status.UserData.ToMap()) {
				t.Errorf("tc: %s - resources user data should match. Expected \n%v, received \n%v", tc.name, tc.expected.Status.UserData.ToMap(), receivedRes.Status.UserData.ToMap())
			}
			tc.expected.Namespace = "test"
			if diff := diffResourceObjects(receivedRes, tc.expected); diff != nil {
				t.Errorf("receivedRes differs from expected, diff: %v", diff)
			}
		})
	}
}

func diffResourceObjects(a, b *crds.ResourceObject) []string {
	a.TypeMeta = metav1.TypeMeta{}
	b.TypeMeta = metav1.TypeMeta{}
	a.ResourceVersion = "0"
	b.ResourceVersion = "0"
	return deep.Equal(a, b)
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
