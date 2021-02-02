/*
Copyright 2020 The Kubernetes Authors.

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

package summarize

import (
	"os"
	"strings"
	"testing"
)

func TestClusterTest(t *testing.T) {
	type testCase struct {
		name      string
		arguments []failure
		want      failuresGroup
	}
	testCases := make([]testCase, 0, 3)

	// Make sure small strings aren't equal, even with tiny differences
	f1 := failure{FailureText: "exit 1"}
	f2 := failure{FailureText: "exit 2"}
	testCases = append(testCases, testCase{
		"Small strings, slight difference",
		[]failure{f1, f2},
		failuresGroup{
			f1.FailureText: []failure{
				f1,
			},
			f2.FailureText: []failure{
				f2,
			},
		},
	})

	// Longer strings with tiny differences should be equal, however
	f3 := failure{FailureText: "long message immediately preceding exit code 1"}
	f4 := failure{FailureText: "long message immediately preceding exit code 2"}
	testCases = append(testCases, testCase{
		"Long strings, slight diference",
		[]failure{f3, f4},
		failuresGroup{
			f3.FailureText: []failure{
				f3,
				f4,
			},
		},
	})

	// Generate some very long strings
	var builder strings.Builder
	builder.Grow(4 * 399) // Allocate enough memory (4 characters in "1 2 ")
	for i := 0; i < 399; i++ {
		builder.WriteString("1 2 ")
	}
	generatedString := builder.String()

	// Test these new failures together with f1
	f5 := failure{FailureText: generatedString + "1 2 "}
	f6 := failure{FailureText: generatedString + "3 4 "}
	testCases = append(testCases, testCase{
		"Huge strings, slight diference",
		[]failure{f1, f5, f6},
		failuresGroup{
			f1.FailureText: []failure{
				f1,
			},
			f5.FailureText: []failure{
				f5,
				f6,
			},
		},
	})

	// Run the tests
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := clusterTest(tc.arguments)

			if !tc.want.equal(&got) {
				t.Errorf("clusterTest(%#v) = %#v, wanted %#v", tc.arguments, got, tc.want)
			}
		})
	}
}

func TestClusterGlobal(t *testing.T) {
	t.Run("Normal", func(t *testing.T) {
		f1 := failure{FailureText: "exit 1"}
		f2 := failure{FailureText: "exit 1"}
		f3 := failure{FailureText: "exit 1"}

		argument := nestedFailuresGroups{
			"test a": failuresGroup{
				"exit 1": []failure{f1, f2},
			},
			"test b": failuresGroup{
				"exit 1": []failure{f3},
			},
		}

		want := nestedFailuresGroups{
			"exit 1": failuresGroup{
				"test a": []failure{f1, f2},
				"test b": []failure{f3},
			},
		}

		got := clusterGlobal(argument, nil, false)

		if !want.equal(&got) {
			t.Errorf("clusterGlobal(%#v) = %#v, wanted %#v", argument, got, want)
		}
	})

	// Make sure clusters are stable when provided with previous seeds
	t.Run("With previous seed", func(t *testing.T) {
		// Remove any memoization files after the test is done so as not to taint future tests
		defer os.Remove("memo_cluster_global.json")

		textOld := "some long failure message that changes occasionally foo"
		textNew := strings.Replace(textOld, "foo", "bar", -1)

		f1 := failure{FailureText: textNew}

		argument := nestedFailuresGroups{"test a": failuresGroup{textNew: []failure{f1}}}
		previous := []jsonCluster{{Key: textOld}}

		want := nestedFailuresGroups{textOld: failuresGroup{"test a": []failure{f1}}}

		got := clusterGlobal(argument, previous, true)

		if !want.equal(&got) {
			t.Errorf("clusterGlobal(%#v, %#v) = %#v, wanted %#v", argument, previous, got, want)
		}
	})
}
