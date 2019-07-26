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

package ranch

import (
	"fmt"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/crds"
)

var (
	startTime = fakeTime(time.Now())
	fakeNow   = fakeTime(startTime.Add(time.Second))
)

type nameGenerator struct {
	lock  sync.Mutex
	index int
}

func (g *nameGenerator) name() string {
	g.lock.Lock()
	defer g.lock.Unlock()
	g.index++
	return fmt.Sprintf("new-dynamic-res-%d", g.index)
}

// json does not serialized time with nanosecond precision
func fakeTime(t time.Time) time.Time {
	format := "2006-01-02 15:04:05.000"
	now, _ := time.Parse(format, t.Format(format))
	return now
}

func MakeTestRanch(resources []common.Resource, dResources []common.DynamicResourceLifeCycle) *Ranch {
	rs := crds.NewCRDStorage(crds.NewTestResourceClient())
	lfs := crds.NewCRDStorage(crds.NewTestDRLCClient())
	s, _ := NewStorage(rs, lfs, "")
	s.now = func() time.Time {
		return fakeNow
	}
	nameGen := &nameGenerator{}
	s.generateName = nameGen.name
	for _, res := range resources {
		s.AddResource(res)
	}
	for _, res := range dResources {
		s.AddDynamicResourceLifeCycle(res)
	}
	r, _ := NewRanch("", s, testTTL)
	r.now = func() time.Time {
		return fakeNow
	}
	return r
}

func AreErrorsEqual(got error, expect error) bool {
	if got == nil && expect == nil {
		return true
	}

	if got == nil || expect == nil {
		return false
	}

	switch got.(type) {
	case *OwnerNotMatch:
		if o, ok := expect.(*OwnerNotMatch); ok {
			if o.request == got.(*OwnerNotMatch).request && o.owner == got.(*OwnerNotMatch).owner {
				return true
			}
		}
		return false
	case *ResourceNotFound:
		if o, ok := expect.(*ResourceNotFound); ok {
			if o.name == got.(*ResourceNotFound).name {
				return true
			}
		}
		return false
	case *ResourceTypeNotFound:
		if o, ok := expect.(*ResourceTypeNotFound); ok {
			if o.rType == got.(*ResourceTypeNotFound).rType {
				return true
			}
		}
		return false
	case *StateNotMatch:
		if o, ok := expect.(*StateNotMatch); ok {
			if o.expect == got.(*StateNotMatch).expect && o.current == got.(*StateNotMatch).current {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func TestAcquire(t *testing.T) {
	var testcases = []struct {
		name      string
		resources []common.Resource
		owner     string
		rtype     string
		state     string
		dest      string
		expectErr error
	}{
		{
			name:      "ranch has no resource",
			resources: []common.Resource{},
			owner:     "user",
			rtype:     "t",
			state:     "s",
			dest:      "d",
			expectErr: &ResourceTypeNotFound{"t"},
		},
		{
			name: "no match type",
			resources: []common.Resource{
				common.NewResource("res", "wrong", "s", "", startTime),
			},
			owner:     "user",
			rtype:     "t",
			state:     "s",
			dest:      "d",
			expectErr: &ResourceTypeNotFound{"t"},
		},
		{
			name: "no match state",
			resources: []common.Resource{
				common.NewResource("res", "t", "wrong", "", startTime),
			},
			owner:     "user",
			rtype:     "t",
			state:     "s",
			dest:      "d",
			expectErr: &ResourceNotFound{"t"},
		},
		{
			name: common.Busy,
			resources: []common.Resource{
				common.NewResource("res", "t", "s", "foo", startTime),
			},
			owner:     "user",
			rtype:     "t",
			state:     "s",
			dest:      "d",
			expectErr: &ResourceNotFound{"t"},
		},
		{
			name: "ok",
			resources: []common.Resource{
				common.NewResource("res", "t", "s", "", startTime),
			},
			owner:     "user",
			rtype:     "t",
			state:     "s",
			dest:      "d",
			expectErr: nil,
		},
	}

	for _, tc := range testcases {
		c := MakeTestRanch(tc.resources, nil)
		res, err := c.Acquire(tc.rtype, tc.state, tc.dest, tc.owner, "")
		if !AreErrorsEqual(err, tc.expectErr) {
			t.Errorf("%s - Got error %v, expected error %v", tc.name, err, tc.expectErr)
			continue
		}

		resources, err2 := c.Storage.GetResources()
		if err2 != nil {
			t.Errorf("failed to get resources")
			continue
		}

		if err == nil {
			if res.State != tc.dest {
				t.Errorf("%s - Wrong final state. Got %v, expected %v", tc.name, res.State, tc.dest)
			}
			if !reflect.DeepEqual(*res, resources[0]) {
				t.Errorf("%s - Wrong resource. Got %v, expected %v", tc.name, res, resources[0])
			} else if !res.LastUpdate.After(startTime) {
				t.Errorf("%s - LastUpdate did not update.", tc.name)
			}
		} else {
			for _, res := range resources {
				if res.LastUpdate != startTime {
					t.Errorf("%s - LastUpdate should not update. Got %v, expected %v", tc.name, resources[0].LastUpdate, startTime)
				}
			}
		}
	}
}

func TestAcquirePriority(t *testing.T) {
	now := time.Now()
	expiredFuture := now.Add(2 * testTTL)
	owner := "tester"
	res := common.NewResource("res", "type", common.Free, "", now)
	r := MakeTestRanch(nil, nil)
	r.requestMgr.now = func() time.Time { return now }

	// Setting Priority, this request will fail
	if _, err := r.Acquire(res.Type, res.State, common.Dirty, owner, "request_id_1"); err == nil {
		t.Errorf("should fail as there are not resource available")
	}
	r.Storage.AddResource(res)
	// Attempting to acquire this resource without priority
	if _, err := r.Acquire(res.Type, res.State, common.Dirty, owner, ""); err == nil {
		t.Errorf("should fail as there is only resource, and it is prioritizes to request_id_1")
	}
	// Attempting to acquire this resource with priority, which will set a place in the queue
	if _, err := r.Acquire(res.Type, res.State, common.Dirty, owner, "request_id_2"); err == nil {
		t.Errorf("should fail as there is only resource, and it is prioritizes to request_id_1")
	}
	// Attempting with the first request
	if _, err := r.Acquire(res.Type, res.State, common.Dirty, owner, "request_id_1"); err != nil {
		t.Errorf("should succeed since the request priority should match its rank in the queue. got %v", err)
	}
	r.Release(res.Name, common.Free, "tester")
	// Attempting with the first request
	if _, err := r.Acquire(res.Type, res.State, common.Dirty, owner, "request_id_1"); err == nil {
		t.Errorf("should not succeed since this request has already been fulfilled")
	}
	// Attempting to acquire this resource without priority
	if _, err := r.Acquire(res.Type, res.State, common.Dirty, owner, ""); err == nil {
		t.Errorf("should fail as request_id_2 has rank 1 now")
	}
	r.requestMgr.cleanup(expiredFuture)
	// Attempting to acquire this resource without priority
	if _, err := r.Acquire(res.Type, res.State, common.Dirty, owner, ""); err != nil {
		t.Errorf("request_id_2 expired, this should work now, got %v", err)
	}
}

func TestAcquireRoundRobin(t *testing.T) {
	var resources []common.Resource
	for i := 1; i < 5; i++ {
		resources = append(resources, common.NewResource("res-1", "t", "s", "", startTime))
	}

	results := map[string]int{}

	c := MakeTestRanch(resources, nil)
	for i := 0; i < 4; i++ {
		res, err := c.Acquire("t", "s", "d", "foo", "")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		_, found := results[res.Name]
		if found {
			t.Errorf("resource %s was used more than once", res.Name)
		}
		c.Release(res.Name, "s", "foo")
	}
}

func TestAcquireOnDemand(t *testing.T) {
	owner := "tester"
	rType := "dr"
	requestID1 := "req1234"
	requestID2 := "req12345"
	requestID3 := "req123456"
	now := time.Now()
	dRLCs := []common.DynamicResourceLifeCycle{
		{
			Type:         rType,
			MinCount:     0,
			MaxCount:     2,
			InitialState: common.Dirty,
		},
	}
	// Not adding any resources to start with
	c := MakeTestRanch(nil, dRLCs)
	c.now = func() time.Time { return now }
	// First acquire should trigger a creation
	if _, err := c.Acquire(rType, common.Free, common.Busy, owner, requestID1); err == nil {
		t.Errorf("should fail since there is not resource yet")
	}
	if resources, err := c.Storage.GetResources(); err != nil {
		t.Error(err)
	} else if len(resources) != 1 {
		t.Errorf("A resource should have been created")
	}
	// Attempting to create another resource
	if _, err := c.Acquire(rType, common.Free, common.Busy, owner, requestID1); err == nil {
		t.Errorf("should succeed since the created is dirty")
	}
	if resources, err := c.Storage.GetResources(); err != nil {
		t.Error(err)
	} else if len(resources) != 1 {
		t.Errorf("No new resource should have been created")
	}
	// Creating another
	if _, err := c.Acquire(rType, common.Free, common.Busy, owner, requestID2); err == nil {
		t.Errorf("should succeed since the created is dirty")
	}
	if resources, err := c.Storage.GetResources(); err != nil {
		t.Error(err)
	} else if len(resources) != 2 {
		t.Errorf("Another resource should have been created")
	}
	// Attempting to create another
	if _, err := c.Acquire(rType, common.Free, common.Busy, owner, requestID3); err == nil {
		t.Errorf("should fail since there is not resource yet")
	}
	resources, err := c.Storage.GetResources()
	if err != nil {
		t.Error(err)
	} else if len(resources) != 2 {
		t.Errorf("No other resource should have been created")
	}
	for _, res := range resources {
		c.Storage.DeleteResource(res.Name)
	}
	if _, err := c.Acquire(rType, common.Free, common.Busy, owner, ""); err == nil {
		t.Errorf("should fail since there is not resource yet")
	}
	if resources, err := c.Storage.GetResources(); err != nil {
		t.Error(err)
	} else if len(resources) != 0 {
		t.Errorf("No new resource should have been created")
	}
}

func TestRelease(t *testing.T) {
	var lifespan = time.Minute
	updatedRes := common.NewResource("res", "t", "d", "", fakeNow)
	expirationDate := fakeTime(fakeNow.Add(lifespan))
	updatedRes.ExpirationDate = &expirationDate
	var testcases = []struct {
		name        string
		resource    common.Resource
		dResource   common.DynamicResourceLifeCycle
		resName     string
		owner       string
		dest        string
		expectErr   error
		expectedRes common.Resource
	}{
		{
			name:        "ranch has no resource",
			resource:    common.Resource{},
			resName:     "res",
			owner:       "user",
			dest:        "d",
			expectErr:   &ResourceNotFound{"res"},
			expectedRes: common.Resource{},
		},
		{
			name:        "wrong owner",
			resource:    common.NewResource("res", "t", "s", "merlin", startTime),
			resName:     "res",
			owner:       "user",
			dest:        "d",
			expectErr:   &OwnerNotMatch{"merlin", "user"},
			expectedRes: common.NewResource("res", "t", "s", "merlin", startTime),
		},
		{
			name:        "no match name",
			resource:    common.NewResource("foo", "t", "s", "merlin", startTime),
			resName:     "res",
			owner:       "user",
			dest:        "d",
			expectErr:   &ResourceNotFound{"res"},
			expectedRes: common.Resource{},
		},
		{
			name:        "ok",
			resource:    common.NewResource("res", "t", "s", "merlin", startTime),
			resName:     "res",
			owner:       "merlin",
			dest:        "d",
			expectErr:   nil,
			expectedRes: common.NewResource("res", "t", "d", "", fakeNow),
		},
		{
			name:     "ok - has dynamic resource lf no lifespan",
			resource: common.NewResource("res", "t", "s", "merlin", startTime),
			dResource: common.DynamicResourceLifeCycle{
				Type: "t",
			},
			resName:     "res",
			owner:       "merlin",
			dest:        "d",
			expectErr:   nil,
			expectedRes: common.NewResource("res", "t", "d", "", fakeNow),
		},
		{
			name:     "ok - has dynamic resource lf with lifespan",
			resource: common.NewResource("res", "t", "s", "merlin", startTime),
			dResource: common.DynamicResourceLifeCycle{
				Type:     "t",
				LifeSpan: &lifespan,
			},
			resName:     "res",
			owner:       "merlin",
			dest:        "d",
			expectErr:   nil,
			expectedRes: updatedRes,
		},
	}

	for _, tc := range testcases {
		c := MakeTestRanch([]common.Resource{tc.resource}, []common.DynamicResourceLifeCycle{tc.dResource})
		releaseErr := c.Release(tc.resName, tc.dest, tc.owner)
		if !AreErrorsEqual(releaseErr, tc.expectErr) {
			t.Errorf("%s - Got error %v, expected error %v", tc.name, releaseErr, tc.expectErr)
			continue
		}
		res, _ := c.Storage.GetResource(tc.resName)
		if !reflect.DeepEqual(res, tc.expectedRes) {
			t.Errorf("Test %v: got %v, expected %v", tc.name, res, tc.expectedRes)
		}
	}
}

func TestReset(t *testing.T) {
	var testcases = []struct {
		name       string
		resources  []common.Resource
		rtype      string
		state      string
		dest       string
		expire     time.Duration
		hasContent bool
	}{

		{
			name: "empty - has no owner",
			resources: []common.Resource{
				common.NewResource("res", "t", "s", "", startTime.Add(-time.Minute*20)),
			},
			rtype:  "t",
			state:  "s",
			expire: time.Minute * 10,
			dest:   "d",
		},
		{
			name: "empty - not expire",
			resources: []common.Resource{
				common.NewResource("res", "t", "s", "", startTime),
			},
			rtype:  "t",
			state:  "s",
			expire: time.Minute * 10,
			dest:   "d",
		},
		{
			name: "empty - no match type",
			resources: []common.Resource{
				common.NewResource("res", "wrong", "s", "", startTime.Add(-time.Minute*20)),
			},
			rtype:  "t",
			state:  "s",
			expire: time.Minute * 10,
			dest:   "d",
		},
		{
			name: "empty - no match state",
			resources: []common.Resource{
				common.NewResource("res", "t", "wrong", "", startTime.Add(-time.Minute*20)),
			},
			rtype:  "t",
			state:  "s",
			expire: time.Minute * 10,
			dest:   "d",
		},
		{
			name: "ok",
			resources: []common.Resource{
				common.NewResource("res", "t", "s", "user", startTime.Add(-time.Minute*20)),
			},
			rtype:      "t",
			state:      "s",
			expire:     time.Minute * 10,
			dest:       "d",
			hasContent: true,
		},
	}

	for _, tc := range testcases {
		c := MakeTestRanch(tc.resources, nil)
		rmap, err := c.Reset(tc.rtype, tc.state, tc.expire, tc.dest)
		if err != nil {
			t.Errorf("failed to reset %v", err)
		}

		if !tc.hasContent {
			if len(rmap) != 0 {
				t.Errorf("%s - Expect empty map. Got %v", tc.name, rmap)
			}
		} else {
			if owner, ok := rmap["res"]; !ok || owner != "user" {
				t.Errorf("%s - Expect res - user. Got %v", tc.name, rmap)
			}
			resources, err := c.Storage.GetResources()
			if err != nil {
				t.Errorf("failed to get resources")
				continue
			}
			if !resources[0].LastUpdate.After(startTime) {
				t.Errorf("%s - LastUpdate did not update.", tc.name)
			}
		}
	}
}

func TestUpdate(t *testing.T) {
	var testcases = []struct {
		name      string
		resources []common.Resource
		resName   string
		owner     string
		state     string
		expectErr error
	}{
		{
			name:      "ranch has no resource",
			resources: []common.Resource{},
			resName:   "res",
			owner:     "user",
			state:     "s",
			expectErr: &ResourceNotFound{"res"},
		},
		{
			name: "wrong owner",
			resources: []common.Resource{
				common.NewResource("res", "t", "s", "merlin", startTime),
			},
			resName:   "res",
			owner:     "user",
			state:     "s",
			expectErr: &OwnerNotMatch{"merlin", "user"},
		},
		{
			name: "wrong state",
			resources: []common.Resource{
				common.NewResource("res", "t", "s", "merlin", startTime),
			},
			resName:   "res",
			owner:     "merlin",
			state:     "foo",
			expectErr: &StateNotMatch{"s", "foo"},
		},
		{
			name: "no matched resource",
			resources: []common.Resource{
				common.NewResource("foo", "t", "s", "merlin", startTime),
			},
			resName:   "res",
			owner:     "merlin",
			state:     "s",
			expectErr: &ResourceNotFound{"res"},
		},
		{
			name: "ok",
			resources: []common.Resource{
				common.NewResource("res", "t", "s", "merlin", startTime),
			},
			resName: "res",
			owner:   "merlin",
			state:   "s",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			c := MakeTestRanch(tc.resources, nil)
			err := c.Update(tc.resName, tc.owner, tc.state, nil)
			if !AreErrorsEqual(err, tc.expectErr) {
				t.Fatalf("Got error %v, expected error %v", err, tc.expectErr)
			}

			resources, err2 := c.Storage.GetResources()
			if err2 != nil {
				t.Fatalf("failed to get resources")
			}

			if err == nil {
				if resources[0].Owner != tc.owner {
					t.Errorf("%s - Wrong owner after release. Got %v, expected %v", tc.name, resources[0].Owner, tc.owner)
				} else if resources[0].State != tc.state {
					t.Errorf("%s - Wrong state after release. Got %v, expected %v", tc.name, resources[0].State, tc.state)
				} else if !resources[0].LastUpdate.After(startTime) {
					t.Errorf("%s - LastUpdate did not update.", tc.name)
				}
			} else {
				for _, res := range resources {
					if res.LastUpdate != startTime {
						t.Errorf("%s - LastUpdate should not update. Got %v, expected %v", tc.name, resources[0].LastUpdate, startTime)
					}
				}
			}
		})
	}
}

func TestMetric(t *testing.T) {
	var testcases = []struct {
		name         string
		resources    []common.Resource
		metricType   string
		expectErr    error
		expectMetric common.Metric
	}{
		{
			name:       "ranch has no resource",
			resources:  []common.Resource{},
			metricType: "t",
			expectErr:  &ResourceNotFound{"t"},
		},
		{
			name: "no matching resource",
			resources: []common.Resource{
				common.NewResource("res", "t", "s", "merlin", time.Now()),
			},
			metricType: "foo",
			expectErr:  &ResourceNotFound{"foo"},
		},
		{
			name: "one resource",
			resources: []common.Resource{
				common.NewResource("res", "t", "s", "merlin", time.Now()),
			},
			metricType: "t",
			expectMetric: common.Metric{
				Type: "t",
				Current: map[string]int{
					"s": 1,
				},
				Owners: map[string]int{
					"merlin": 1,
				},
			},
		},
		{
			name: "multiple resources",
			resources: []common.Resource{
				common.NewResource("res-1", "t", "s", "merlin", time.Now()),
				common.NewResource("res-2", "t", "p", "pony", time.Now()),
				common.NewResource("res-3", "t", "s", "pony", time.Now()),
				common.NewResource("res-4", "foo", "s", "pony", time.Now()),
				common.NewResource("res-5", "t", "d", "merlin", time.Now()),
			},
			metricType: "t",
			expectMetric: common.Metric{
				Type: "t",
				Current: map[string]int{
					"s": 2,
					"d": 1,
					"p": 1,
				},
				Owners: map[string]int{
					"merlin": 2,
					"pony":   2,
				},
			},
		},
	}

	for _, tc := range testcases {
		c := MakeTestRanch(tc.resources, nil)
		metric, err := c.Metric(tc.metricType)
		if !AreErrorsEqual(err, tc.expectErr) {
			t.Errorf("%s - Got error %v, expected error %v", tc.name, err, tc.expectErr)
			continue
		}

		if err == nil {
			if !reflect.DeepEqual(metric, tc.expectMetric) {
				t.Errorf("%s - wrong metric, got %v, want %v", tc.name, metric, tc.expectMetric)
			}
		}
	}
}

func setExpiration(res common.Resource, exp time.Time) common.Resource {
	res.ExpirationDate = &exp
	return res
}

func TestSyncResources(t *testing.T) {
	var testcases = []struct {
		name                    string
		currentRes, expectedRes []common.Resource
		currentLCs, expectedLCs []common.DynamicResourceLifeCycle
		config                  *common.BoskosConfig
	}{
		{
			name: "migration from mason resource to dynamic resource does not delete resource",
			currentRes: []common.Resource{
				common.NewResource("res-1", "t", "", "", startTime),
				common.NewResource("dt_1", "mason", "", "", startTime),
				common.NewResource("dt_2", "mason", "", "", startTime),
			},
			config: &common.BoskosConfig{
				Resources: []common.ResourceEntry{
					{
						Type:  "t",
						Names: []string{"res-1"},
					},
					{
						Type:     "mason",
						MinCount: 2,
						MaxCount: 4,
					},
				},
			},
			expectedRes: []common.Resource{
				common.NewResource("res-1", "t", common.Free, "", startTime),
				common.NewResource("dt_1", "mason", common.Free, "", startTime),
				common.NewResource("dt_2", "mason", common.Free, "", startTime),
			},
			expectedLCs: []common.DynamicResourceLifeCycle{
				{
					Type:     "mason",
					MinCount: 2,
					MaxCount: 4,
				},
			},
		},
		{
			name: "empty",
		},
		{
			name: "append",
			currentRes: []common.Resource{
				common.NewResource("res-1", "t", "", "", startTime),
			},
			config: &common.BoskosConfig{
				Resources: []common.ResourceEntry{
					{
						Type:  "t",
						Names: []string{"res-1", "res-2"},
					},
					{
						Type:     "dt",
						MinCount: 1,
						MaxCount: 2,
					},
				},
			},
			expectedRes: []common.Resource{
				common.NewResource("res-1", "t", common.Free, "", startTime),
				common.NewResource("res-2", "t", common.Free, "", fakeNow),
				common.NewResource("new-dynamic-res-1", "dt", common.Free, "", fakeNow),
			},
			expectedLCs: []common.DynamicResourceLifeCycle{
				{
					Type:     "dt",
					MinCount: 1,
					MaxCount: 2,
				},
			},
		},
		{
			name: "should not change anything",
			currentRes: []common.Resource{
				common.NewResource("res-1", "t", "", "", startTime),
				common.NewResource("dt_1", "dt", "", "", startTime),
			},
			currentLCs: []common.DynamicResourceLifeCycle{
				{
					Type:     "dt",
					MinCount: 1,
					MaxCount: 2,
				},
			},
			config: &common.BoskosConfig{
				Resources: []common.ResourceEntry{
					{
						Type:  "t",
						Names: []string{"res-1"},
					},
					{
						Type:     "dt",
						MinCount: 1,
						MaxCount: 2,
					},
				},
			},
			expectedRes: []common.Resource{
				common.NewResource("res-1", "t", "", "", startTime),
				common.NewResource("dt_1", "dt", "", "", startTime),
			},
			expectedLCs: []common.DynamicResourceLifeCycle{
				{
					Type:     "dt",
					MinCount: 1,
					MaxCount: 2,
				},
			},
		},
		{
			name: "delete, lifecycle should not delete dynamic res until all associated resources are gone",
			currentRes: []common.Resource{
				common.NewResource("res", "t", "", "", startTime),
				common.NewResource("dt_1", "dt", "", "", startTime),
			},
			currentLCs: []common.DynamicResourceLifeCycle{
				{
					Type:     "dt",
					MinCount: 1,
					MaxCount: 2,
				},
			},
			config: &common.BoskosConfig{},
			expectedRes: []common.Resource{
				common.NewResource("dt_1", "dt", common.ToBeDeleted, "", fakeNow),
			},
			expectedLCs: []common.DynamicResourceLifeCycle{
				{
					Type:     "dt",
					MinCount: 1,
					MaxCount: 2,
				},
			},
		},
		{
			name: "delete, life cycle should be deleted as all resources are deleted",
			currentLCs: []common.DynamicResourceLifeCycle{
				{
					Type:     "dt",
					MinCount: 1,
					MaxCount: 2,
				},
			},
			config: &common.BoskosConfig{},
		},
		{
			name: "delete busy",
			currentRes: []common.Resource{
				common.NewResource("res", "t", common.Busy, "o", startTime),
				common.NewResource("dt_1", "dt", common.Busy, "o", startTime),
			},
			currentLCs: []common.DynamicResourceLifeCycle{
				{
					Type:     "dt",
					MinCount: 1,
					MaxCount: 2,
				},
			},
			config: &common.BoskosConfig{},
			expectedRes: []common.Resource{
				common.NewResource("res", "t", common.Busy, "o", startTime),
				common.NewResource("dt_1", "dt", common.Busy, "o", startTime),
			},
			expectedLCs: []common.DynamicResourceLifeCycle{
				{
					Type:     "dt",
					MinCount: 1,
					MaxCount: 2,
				},
			},
		},
		{
			name: "append and delete",
			currentRes: []common.Resource{
				common.NewResource("res-1", "t", common.Tombstone, "", startTime),
				common.NewResource("dt_1", "dt", common.ToBeDeleted, "", startTime),
				common.NewResource("dt_2", "dt", "", "", startTime),
				common.NewResource("dt_3", "dt", "", "", startTime),
			},
			currentLCs: []common.DynamicResourceLifeCycle{
				{
					Type:     "dt",
					MinCount: 1,
					MaxCount: 3,
				},
			},
			config: &common.BoskosConfig{
				Resources: []common.ResourceEntry{
					{
						Type:  "t",
						Names: []string{"res-2"},
					},
					{
						Type:     "dt",
						MinCount: 1,
						MaxCount: 2,
					},
					{
						Type:     "dt2",
						MinCount: 1,
						MaxCount: 2,
					},
				},
			},
			expectedRes: []common.Resource{
				common.NewResource("res-2", "t", common.Free, "", fakeNow),
				common.NewResource("dt_1", "dt", common.ToBeDeleted, "", startTime),
				common.NewResource("dt_2", "dt", common.Free, "", startTime),
				common.NewResource("dt_3", "dt", common.Free, "", startTime),
				common.NewResource("new-dynamic-res-1", "dt2", common.Free, "", fakeNow),
			},
			expectedLCs: []common.DynamicResourceLifeCycle{
				{
					Type:     "dt",
					MinCount: 1,
					MaxCount: 2,
				},
				{
					Type:     "dt2",
					MinCount: 1,
					MaxCount: 2,
				},
			},
		},
		{
			name: "append and delete busy",
			currentRes: []common.Resource{
				common.NewResource("res-1", "t", common.Busy, "o", startTime),
				common.NewResource("dt_1", "dt", "", "", startTime),
				common.NewResource("dt_2", "dt", common.Tombstone, "", startTime),
				common.NewResource("dt_3", "dt", common.Busy, "o", startTime),
			},
			currentLCs: []common.DynamicResourceLifeCycle{
				{
					Type:     "dt",
					MinCount: 1,
					MaxCount: 3,
				},
			},
			config: &common.BoskosConfig{
				Resources: []common.ResourceEntry{
					{
						Type:  "t",
						Names: []string{"res-2"},
					},
					{
						Type:     "dt",
						MinCount: 1,
						MaxCount: 2,
					},
					{
						Type:     "dt2",
						MinCount: 1,
						MaxCount: 2,
					},
				},
			},
			expectedRes: []common.Resource{
				common.NewResource("res-1", "t", common.Busy, "o", startTime),
				common.NewResource("res-2", "t", common.Free, "", fakeNow),
				common.NewResource("dt_1", "dt", common.Free, "", startTime),
				common.NewResource("dt_3", "dt", common.Busy, "o", startTime),
				common.NewResource("new-dynamic-res-1", "dt2", common.Free, "", fakeNow),
			},
			expectedLCs: []common.DynamicResourceLifeCycle{
				{
					Type:     "dt",
					MinCount: 1,
					MaxCount: 2,
				},
				{
					Type:     "dt2",
					MinCount: 1,
					MaxCount: 2,
				},
			},
		},
		{
			name: "append/delete mixed type",
			currentRes: []common.Resource{
				common.NewResource("res-1", "t", common.Tombstone, "", startTime),
			},
			config: &common.BoskosConfig{
				Resources: []common.ResourceEntry{
					{
						Type:  "t",
						Names: []string{"res-2"},
					},
					{
						Type:  "t2",
						Names: []string{"res-3"},
					},
				},
			},
			expectedRes: []common.Resource{
				common.NewResource("res-2", "t", "free", "", fakeNow),
				common.NewResource("res-3", "t2", "free", "", fakeNow),
			},
		},
		{
			name: "delete expired resource",
			currentRes: []common.Resource{
				setExpiration(
					common.NewResource("dt_1", "dt", "", "", startTime),
					startTime),
				common.NewResource("dt_2", "dt", "", "", startTime),
				setExpiration(
					common.NewResource("dt_3", "dt", common.Tombstone, "", startTime),
					startTime),
				common.NewResource("dt_4", "dt", "", "", startTime),
			},
			currentLCs: []common.DynamicResourceLifeCycle{
				{
					Type:     "dt",
					MinCount: 2,
					MaxCount: 4,
				},
			},
			config: &common.BoskosConfig{
				Resources: []common.ResourceEntry{
					{
						Type:     "dt",
						MinCount: 2,
						MaxCount: 4,
					},
				},
			},
			expectedRes: []common.Resource{
				setExpiration(
					common.NewResource("dt_1", "dt", common.ToBeDeleted, "", fakeNow),
					startTime),
				common.NewResource("dt_2", "dt", common.Free, "", startTime),
				common.NewResource("dt_4", "dt", common.Free, "", startTime),
			},
			expectedLCs: []common.DynamicResourceLifeCycle{
				{
					Type:     "dt",
					MinCount: 2,
					MaxCount: 4,
				},
			},
		},
		{
			name: "delete expired resource / do not delete busy",
			currentRes: []common.Resource{
				setExpiration(
					common.NewResource("dt_1", "dt", common.Tombstone, "", startTime),
					startTime),
				common.NewResource("dt_2", "dt", "", "", startTime),
				setExpiration(
					common.NewResource("dt_3", "dt", common.Busy, "o", startTime),
					startTime),
				common.NewResource("dt_4", "dt", common.Busy, "o", startTime),
			},
			currentLCs: []common.DynamicResourceLifeCycle{
				{
					Type:     "dt",
					MinCount: 1,
					MaxCount: 4,
				},
			},
			config: &common.BoskosConfig{
				Resources: []common.ResourceEntry{
					{
						Type:     "dt",
						MinCount: 1,
						MaxCount: 3,
					},
				},
			},
			expectedRes: []common.Resource{
				common.NewResource("dt_2", "dt", common.Free, "", startTime),
				setExpiration(
					common.NewResource("dt_3", "dt", common.Busy, "o", startTime),
					startTime),
				common.NewResource("dt_4", "dt", common.Busy, "o", startTime),
			},
			expectedLCs: []common.DynamicResourceLifeCycle{
				{
					Type:     "dt",
					MinCount: 1,
					MaxCount: 3,
				},
			},
		},
		{
			name: "delete expired resource, recreate up to Min",
			currentRes: []common.Resource{
				setExpiration(
					common.NewResource("dt_1", "dt", "", "", startTime),
					startTime),
				common.NewResource("dt_2", "dt", "", "", startTime),
				setExpiration(
					common.NewResource("dt_3", "dt", common.Tombstone, "", startTime),
					startTime),
				common.NewResource("dt_4", "dt", "", "", startTime),
			},
			currentLCs: []common.DynamicResourceLifeCycle{
				{
					Type:     "dt",
					MinCount: 4,
					MaxCount: 6,
				},
			},
			config: &common.BoskosConfig{
				Resources: []common.ResourceEntry{
					{
						Type:     "dt",
						MinCount: 4,
						MaxCount: 6,
					},
				},
			},
			expectedRes: []common.Resource{
				setExpiration(
					common.NewResource("dt_1", "dt", common.ToBeDeleted, "", fakeNow),
					startTime),
				common.NewResource("new-dynamic-res-1", "dt", common.Free, "", fakeNow),
				common.NewResource("dt_2", "dt", common.Free, "", startTime),
				common.NewResource("dt_4", "dt", common.Free, "", startTime),
			},
			expectedLCs: []common.DynamicResourceLifeCycle{
				{
					Type:     "dt",
					MinCount: 4,
					MaxCount: 6,
				},
			},
		},
	}

	for _, tc := range testcases {
		c := MakeTestRanch(tc.currentRes, tc.currentLCs)
		c.Storage.SyncResources(tc.config)
		resources, err := c.Storage.GetResources()
		if err != nil {
			t.Errorf("failed to get resources")
			continue
		}
		sort.Stable(common.ResourceByName(resources))
		sort.Stable(common.ResourceByName(tc.expectedRes))
		if !reflect.DeepEqual(resources, tc.expectedRes) {
			t.Errorf("Test %v: \n got \t\t%v, \n expected \t%v", tc.name, resources, tc.expectedRes)
		}
		lfs, err := c.Storage.GetDynamicResourceLifeCycles()
		sort.SliceStable(lfs, func(i, j int) bool {
			{
				return lfs[i].GetName() < lfs[j].GetName()
			}
		})
		sort.SliceStable(tc.expectedLCs, func(i, j int) bool {
			{
				return tc.expectedLCs[i].GetName() < tc.expectedLCs[j].GetName()
			}
		})
		if !reflect.DeepEqual(lfs, tc.expectedLCs) {
			t.Errorf("Test %v: \n got \t\t%v, \n expected %v", tc.name, lfs, tc.expectedLCs)
		}
	}
}
