/*
Copyright 2018 The Kubernetes Authors.

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
	"testing"

	"k8s.io/test-infra/testgrid/state"
)

func Test_ExtractRows(t *testing.T) {
	cases := []struct {
		name    string
		content string
		results map[string]state.Row_Result
		metrics map[string]map[string]float64
		err     bool
	}{
		{
			name: "basic testsuite",
			content: `
			  <testsuite>
			    <testcase name="good"/>
			    <testcase name="bad"><failure/></testcase>
			    <testcase name="skip"><skipped/></testcase>
			  </testsuite>`,
			results: map[string]state.Row_Result{
				"good": state.Row_PASS,
				"bad":  state.Row_FAIL,
			},
		},
		{
			name: "basic testsuites",
			content: `
			  <testsuites>
			  <testsuite>
			    <testcase name="good"/>
			  </testsuite>
			  <testsuite>
			    <testcase name="bad"><failure/></testcase>
			    <testcase name="skip"><skipped/></testcase>
			  </testsuite>
			  </testsuites>`,
			results: map[string]state.Row_Result{
				"good": state.Row_PASS,
				"bad":  state.Row_FAIL,
			},
		},
		{
			name: "basic timing",
			content: `
			  <testsuite>
			    <testcase name="slow" time="100.1" />
			    <testcase name="slow-failure" time="123456789">
			      <failure>terrible</failure>
			    </testcase>
			    <testcase name="fast" time="0.0001" />
			    <testcase name="nothing-elapsed" time="0" />
			  </testsuite>`,
			results: map[string]state.Row_Result{
				"slow":            state.Row_PASS,
				"slow-failure":    state.Row_FAIL,
				"fast":            state.Row_PASS,
				"nothing-elapsed": state.Row_PASS,
			},
			metrics: map[string]map[string]float64{
				"slow":         {elapsedKey: 100.1},
				"slow-failure": {elapsedKey: 123456789},
				"fast":         {elapsedKey: 0.0001},
			},
		},
	}

	for _, tc := range cases {
		results := map[string]state.Row_Result{}
		metrics := map[string]map[string]float64{}

		err := extractRows([]byte(tc.content), results, metrics)
		switch {
		case err == nil && tc.err:
			t.Errorf("%s: failed to raise an error", tc.name)
		case err != nil && !tc.err:
			t.Errorf("%s: unexpected err: %v", tc.name, err)
		case len(results) > len(tc.results):
			t.Errorf("%s: extra results: actual %v != expected %v", tc.name, results, tc.results)
		case len(metrics) > len(tc.metrics):
			t.Errorf("%s: extra metrics: actual %v != expected %v", tc.name, metrics, tc.metrics)
		default:
			for target, er := range tc.results {
				if ar, ok := results[target]; !ok {
					t.Errorf("%s: missing result: %s", tc.name, target)
				} else if ar != er {
					t.Errorf("%s: %s actual %s != expected %s", tc.name, target, ar, er)
				}
			}
			for target, ems := range tc.metrics {
				ams, ok := metrics[target]
				switch {
				case !ok:
					t.Errorf("%s: missing metrics for %s", tc.name, target)
				case len(ams) > len(ems):
					t.Errorf("%s: extra metrics for %s: actual %v != expected %v", tc.name, target, ams, ems)
				default:
					for name, ev := range ems {
						if av, ok := ams[name]; !ok {
							t.Errorf("%s: missing %s in %s", tc.name, target, name)
						} else if av != ev {
							t.Errorf("%s: %s %s actual %f != expected %f", tc.name, target, name, av, ev)
						}
					}
				}
			}
		}
	}
}
