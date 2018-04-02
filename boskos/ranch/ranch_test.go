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
	"reflect"
	"sort"
	"testing"
	"time"

	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/boskos/crds"
)

func MakeTestRanch(resources []common.Resource) *Ranch {
	rs := crds.NewCRDStorage(crds.NewTestResourceClient())
	s, _ := NewStorage(rs, "")
	for _, res := range resources {
		s.AddResource(res)
	}
	r, _ := NewRanch("", s)
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
	FakeNow := time.Now()
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
			expectErr: &ResourceNotFound{"t"},
		},
		{
			name: "no match type",
			resources: []common.Resource{
				common.NewResource("res", "wrong", "s", "", FakeNow),
			},
			owner:     "user",
			rtype:     "t",
			state:     "s",
			dest:      "d",
			expectErr: &ResourceNotFound{"t"},
		},
		{
			name: "no match state",
			resources: []common.Resource{
				common.NewResource("res", "t", "wrong", "", FakeNow),
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
				common.NewResource("res", "t", "s", "foo", FakeNow),
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
				common.NewResource("res", "t", "s", "", FakeNow),
			},
			owner:     "user",
			rtype:     "t",
			state:     "s",
			dest:      "d",
			expectErr: nil,
		},
	}

	for _, tc := range testcases {
		c := MakeTestRanch(tc.resources)
		res, err := c.Acquire(tc.rtype, tc.state, tc.dest, tc.owner)
		if !AreErrorsEqual(err, tc.expectErr) {
			t.Errorf("%s - Got error %v, expect error %v", tc.name, err, tc.expectErr)
			continue
		}

		resources, err2 := c.Storage.GetResources()
		if err2 != nil {
			t.Errorf("failed to get resources")
			continue
		}

		if err == nil {
			if res.State != tc.dest {
				t.Errorf("%s - Wrong final state. Got %v, expect %v", tc.name, res.State, tc.dest)
			}
			if !reflect.DeepEqual(*res, resources[0]) {
				t.Errorf("%s - Wrong resource. Got %v, expect %v", tc.name, res, resources[0])
			} else if !res.LastUpdate.After(FakeNow) {
				t.Errorf("%s - LastUpdate did not update.", tc.name)
			}
		} else {
			for _, res := range resources {
				if res.LastUpdate != FakeNow {
					t.Errorf("%s - LastUpdate should not update. Got %v, expect %v", tc.name, resources[0].LastUpdate, FakeNow)
				}
			}
		}
	}
}

func TestAcquireRoundRobin(t *testing.T) {
	FakeNow := time.Now()
	var resources []common.Resource
	for i := 1; i < 5; i++ {
		resources = append(resources, common.NewResource("res-1", "t", "s", "", FakeNow))
	}

	results := map[string]int{}

	c := MakeTestRanch(resources)
	for i := 0; i < 4; i++ {
		res, err := c.Acquire("t", "s", "d", "foo")
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

func TestRelease(t *testing.T) {
	FakeNow := time.Now()
	var testcases = []struct {
		name      string
		resources []common.Resource
		resName   string
		owner     string
		dest      string
		expectErr error
	}{
		{
			name:      "ranch has no resource",
			resources: []common.Resource{},
			resName:   "res",
			owner:     "user",
			dest:      "d",
			expectErr: &ResourceNotFound{"res"},
		},
		{
			name: "wrong owner",
			resources: []common.Resource{
				common.NewResource("res", "t", "s", "merlin", FakeNow),
			},
			resName:   "res",
			owner:     "user",
			dest:      "d",
			expectErr: &OwnerNotMatch{"merlin", "user"},
		},
		{
			name: "no match name",
			resources: []common.Resource{
				common.NewResource("foo", "t", "s", "merlin", FakeNow),
			},
			resName:   "res",
			owner:     "user",
			dest:      "d",
			expectErr: &ResourceNotFound{"res"},
		},
		{
			name: "ok",
			resources: []common.Resource{
				common.NewResource("res", "t", "s", "merlin", FakeNow),
			},
			resName:   "res",
			owner:     "merlin",
			dest:      "d",
			expectErr: nil,
		},
	}

	for _, tc := range testcases {
		c := MakeTestRanch(tc.resources)
		err := c.Release(tc.resName, tc.dest, tc.owner)
		if !AreErrorsEqual(err, tc.expectErr) {
			t.Errorf("%s - Got error %v, expect error %v", tc.name, err, tc.expectErr)
			continue
		}
		resources, err2 := c.Storage.GetResources()
		if err2 != nil {
			t.Errorf("failed to get resources")
			continue
		}
		if err == nil {
			if resources[0].Owner != "" {
				t.Errorf("%s - Wrong owner after release. Got %v, expect empty", tc.name, resources[0].Owner)
			} else if resources[0].State != tc.dest {
				t.Errorf("%s - Wrong state after release. Got %v, expect %v", tc.name, resources[0].State, tc.dest)
			} else if !resources[0].LastUpdate.After(FakeNow) {
				t.Errorf("%s - LastUpdate did not update.", tc.name)
			}
		} else {
			for _, res := range resources {
				if res.LastUpdate != FakeNow {
					t.Errorf("%s - LastUpdate should not update. Got %v, expect %v", tc.name, resources[0].LastUpdate, FakeNow)
				}
			}
		}
	}
}

func TestReset(t *testing.T) {
	FakeNow := time.Now()

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
				common.NewResource("res", "t", "s", "", FakeNow.Add(-time.Minute*20)),
			},
			rtype:  "t",
			state:  "s",
			expire: time.Minute * 10,
			dest:   "d",
		},
		{
			name: "empty - not expire",
			resources: []common.Resource{
				common.NewResource("res", "t", "s", "", FakeNow),
			},
			rtype:  "t",
			state:  "s",
			expire: time.Minute * 10,
			dest:   "d",
		},
		{
			name: "empty - no match type",
			resources: []common.Resource{
				common.NewResource("res", "wrong", "s", "", FakeNow.Add(-time.Minute*20)),
			},
			rtype:  "t",
			state:  "s",
			expire: time.Minute * 10,
			dest:   "d",
		},
		{
			name: "empty - no match state",
			resources: []common.Resource{
				common.NewResource("res", "t", "wrong", "", FakeNow.Add(-time.Minute*20)),
			},
			rtype:  "t",
			state:  "s",
			expire: time.Minute * 10,
			dest:   "d",
		},
		{
			name: "ok",
			resources: []common.Resource{
				common.NewResource("res", "t", "s", "user", FakeNow.Add(-time.Minute*20)),
			},
			rtype:      "t",
			state:      "s",
			expire:     time.Minute * 10,
			dest:       "d",
			hasContent: true,
		},
	}

	for _, tc := range testcases {
		c := MakeTestRanch(tc.resources)
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
			if !resources[0].LastUpdate.After(FakeNow) {
				t.Errorf("%s - LastUpdate did not update.", tc.name)
			}
		}
	}
}

func TestUpdate(t *testing.T) {
	FakeNow := time.Now()

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
				common.NewResource("res", "t", "s", "merlin", FakeNow),
			},
			resName:   "res",
			owner:     "user",
			state:     "s",
			expectErr: &OwnerNotMatch{"user", "merlin"},
		},
		{
			name: "wrong state",
			resources: []common.Resource{
				common.NewResource("res", "t", "s", "merlin", FakeNow),
			},
			resName:   "res",
			owner:     "merlin",
			state:     "foo",
			expectErr: &StateNotMatch{"s", "foo"},
		},
		{
			name: "no matched resource",
			resources: []common.Resource{
				common.NewResource("foo", "t", "s", "merlin", FakeNow),
			},
			resName:   "res",
			owner:     "merlin",
			state:     "s",
			expectErr: &ResourceNotFound{"res"},
		},
		{
			name: "ok",
			resources: []common.Resource{
				common.NewResource("res", "t", "s", "merlin", FakeNow),
			},
			resName: "res",
			owner:   "merlin",
			state:   "s",
		},
	}

	for _, tc := range testcases {
		c := MakeTestRanch(tc.resources)
		err := c.Update(tc.resName, tc.owner, tc.state, nil)
		if !AreErrorsEqual(err, tc.expectErr) {
			t.Errorf("%s - Got error %v, expect error %v", tc.name, err, tc.expectErr)
			continue
		}

		resources, err2 := c.Storage.GetResources()
		if err2 != nil {
			t.Errorf("failed to get resources")
			continue
		}

		if err == nil {
			if resources[0].Owner != tc.owner {
				t.Errorf("%s - Wrong owner after release. Got %v, expect %v", tc.name, resources[0].Owner, tc.owner)
			} else if resources[0].State != tc.state {
				t.Errorf("%s - Wrong state after release. Got %v, expect %v", tc.name, resources[0].State, tc.state)
			} else if !resources[0].LastUpdate.After(FakeNow) {
				t.Errorf("%s - LastUpdate did not update.", tc.name)
			}
		} else {
			for _, res := range resources {
				if res.LastUpdate != FakeNow {
					t.Errorf("%s - LastUpdate should not update. Got %v, expect %v", tc.name, resources[0].LastUpdate, FakeNow)
				}
			}
		}
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
		c := MakeTestRanch(tc.resources)
		metric, err := c.Metric(tc.metricType)
		if !AreErrorsEqual(err, tc.expectErr) {
			t.Errorf("%s - Got error %v, expect error %v", tc.name, err, tc.expectErr)
			continue
		}

		if err == nil {
			if !reflect.DeepEqual(metric, tc.expectMetric) {
				t.Errorf("%s - wrong metric, got %v, want %v", tc.name, metric, tc.expectMetric)
			}
		}
	}
}

func TestSyncResources(t *testing.T) {
	var testcases = []struct {
		name   string
		oldRes []common.Resource
		newRes []common.Resource
		expect []common.Resource
	}{
		{
			name: "empty",
		},
		{
			name: "append",
			newRes: []common.Resource{
				common.NewResource("res", "t", "", "", time.Time{}),
			},
			expect: []common.Resource{
				common.NewResource("res", "t", common.Free, "", time.Time{}),
			},
		},
		{
			name: "should not have a type change",
			oldRes: []common.Resource{
				common.NewResource("res", "t", "", "", time.Time{}),
			},
			newRes: []common.Resource{
				common.NewResource("res", "d", "", "", time.Time{}),
			},
			expect: []common.Resource{
				common.NewResource("res", "t", "", "", time.Time{}),
			},
		},
		{
			name: "delete",
			oldRes: []common.Resource{
				common.NewResource("res", "t", "", "", time.Time{}),
			},
		},
		{
			name: "delete busy",
			oldRes: []common.Resource{
				common.NewResource("res", "t", common.Busy, "o", time.Time{}),
			},
			expect: []common.Resource{
				common.NewResource("res", "t", common.Busy, "o", time.Time{}),
			},
		},
		{
			name: "append and delete",
			oldRes: []common.Resource{
				common.NewResource("res-1", "t", "", "", time.Time{}),
			},
			newRes: []common.Resource{
				common.NewResource("res-2", "t", "", "", time.Time{}),
			},
			expect: []common.Resource{
				common.NewResource("res-2", "t", common.Free, "", time.Time{}),
			},
		},
		{
			name: "append and delete busy",
			oldRes: []common.Resource{
				common.NewResource("res-1", "t", common.Busy, "o", time.Time{}),
			},
			newRes: []common.Resource{
				common.NewResource("res-2", "t", "", "", time.Time{}),
			},
			expect: []common.Resource{
				common.NewResource("res-1", "t", common.Busy, "o", time.Time{}),
				common.NewResource("res-2", "t", common.Free, "", time.Time{}),
			},
		},
		{
			name: "append/delete mixed type",
			oldRes: []common.Resource{
				common.NewResource("res-1", "t", "", "", time.Time{}),
			},
			newRes: []common.Resource{
				common.NewResource("res-2", "t", "", "", time.Time{}),
				common.NewResource("res-3", "t2", "", "", time.Time{}),
			},
			expect: []common.Resource{
				common.NewResource("res-2", "t", "free", "", time.Time{}),
				common.NewResource("res-3", "t2", "free", "", time.Time{}),
			},
		},
	}

	for _, tc := range testcases {
		c := MakeTestRanch(tc.oldRes)
		c.Storage.SyncResources(tc.newRes)
		resources, err := c.Storage.GetResources()
		if err != nil {
			t.Errorf("failed to get resources")
			continue
		}
		sort.Stable(common.ResourceByName(resources))
		sort.Stable(common.ResourceByName(tc.expect))
		if !reflect.DeepEqual(resources, tc.expect) {
			t.Errorf("Test %v: got %v, expect %v", tc.name, resources, tc.expect)
		}
	}
}
