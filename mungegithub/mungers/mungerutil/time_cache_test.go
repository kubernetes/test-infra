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

package mungerutil

import (
	"testing"
	"time"
)

type testLTG struct {
	number int
	label  string
	time   time.Time
}

func (t *testLTG) Number() int { return t.number }
func (t *testLTG) FirstLabelTime(label string) *time.Time {
	if label == t.label {
		return &t.time
	}
	return nil
}

func TestFirstLabelCache(t *testing.T) {
	timeA := time.Now()
	timeB := timeA.Add(time.Minute)
	table := []struct {
		obj    testLTG
		expect time.Time
	}{
		// Returns the time and caches it.
		{testLTG{1, "lgtm", timeA}, timeA},

		// Returns zero time, does not cache.
		{testLTG{2, "blah", timeA}, time.Time{}},

		// Returns old cached value. (Note, in reality the values we
		// cache should not change.)
		{testLTG{1, "lgtm", timeB}, timeA},

		// The empty time was not cached.
		{testLTG{2, "lgtm", timeB}, timeB},
	}

	cache := NewLabelTimeCache("lgtm")
	for i, tt := range table {
		got, ok := cache.FirstLabelTime(&tt.obj)
		if e := (tt.expect != time.Time{}); e != ok {
			t.Errorf("%v: Expected %v, got %v", i, e, ok)
			continue
		}
		if got != tt.expect {
			t.Errorf("%v: Expected %v, got %v", i, tt.expect, got)
		}
	}
}
