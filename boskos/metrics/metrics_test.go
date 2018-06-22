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
	"reflect"
	"testing"

	"k8s.io/test-infra/boskos/common"
)

func TestFilterMetric(t *testing.T) {

	testCases := []struct {
		name      string
		states    []string
		src, dest map[string]int
	}{
		{
			name:   "noOther",
			states: []string{common.Dirty, common.Cleaning, common.Busy, common.Free},
			src: map[string]int{
				common.Dirty:    10,
				common.Cleaning: 2,
				common.Busy:     5,
				common.Free:     3,
			},
			dest: map[string]int{
				common.Dirty:    10,
				common.Cleaning: 2,
				common.Busy:     5,
				common.Free:     3,
				common.Other:    0,
			},
		},
		{
			name:   "multipleOther",
			states: []string{common.Dirty, common.Cleaning, common.Busy, common.Free},
			src: map[string]int{
				common.Dirty:    10,
				common.Cleaning: 2,
				common.Busy:     5,
				common.Free:     3,
				"test":          10,
				"new":           14,
			},
			dest: map[string]int{
				common.Dirty:    10,
				common.Cleaning: 2,
				common.Busy:     5,
				common.Free:     3,
				common.Other:    24,
			},
		},
		{
			name:   "multipleOtherNoLeased",
			states: []string{common.Dirty, common.Cleaning, common.Busy, common.Free, common.Leased},
			src: map[string]int{
				common.Dirty:    10,
				common.Cleaning: 2,
				common.Busy:     5,
				common.Free:     3,
				"test":          10,
				"new":           14,
			},
			dest: map[string]int{
				common.Dirty:    10,
				common.Cleaning: 2,
				common.Busy:     5,
				common.Free:     3,
				common.Leased:   0,
				common.Other:    24,
			},
		},
		{
			name:   "NoOtherLeased",
			states: []string{common.Dirty, common.Cleaning, common.Busy, common.Free, common.Leased},
			src: map[string]int{
				common.Dirty:    10,
				common.Cleaning: 2,
				common.Busy:     5,
				common.Free:     3,
				common.Leased:   10,
			},
			dest: map[string]int{
				common.Dirty:    10,
				common.Cleaning: 2,
				common.Busy:     5,
				common.Free:     3,
				common.Leased:   10,
				common.Other:    0,
			},
		},
	}

	for _, tc := range testCases {
		test := func(t *testing.T) {
			states = tc.states
			dest := filterMetrics(tc.src)
			if !reflect.DeepEqual(dest, tc.dest) {
				t.Errorf("dest: %v is different than expected %v", dest, tc.dest)
			}
		}
		t.Run(tc.name, test)
	}
}
