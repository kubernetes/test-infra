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

package metrics

import (
	"reflect"
	"testing"

	"k8s.io/utils/diff"
)

func TestPowersOfTwoBetween(t *testing.T) {
	var testCases = []struct {
		name     string
		min, max float64
		powers   []float64
	}{
		{
			name:   "bounds are powers",
			min:    2,
			max:    32,
			powers: []float64{2, 4, 8, 16, 32},
		},
		{
			name:   "bounds are integers",
			min:    1,
			max:    33,
			powers: []float64{1, 2, 4, 8, 16, 32, 33},
		},
		{
			name:   "bounds are <1",
			min:    0.05,
			max:    0.5,
			powers: []float64{0.05, 0.0625, 0.125, 0.25, 0.5},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if actual, expected := powersOfTwoBetween(testCase.min, testCase.max), testCase.powers; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect powers between (%v,%v): %s", testCase.name, testCase.min, testCase.max, diff.ObjectReflectDiff(actual, expected))
			}
		})
	}
}
