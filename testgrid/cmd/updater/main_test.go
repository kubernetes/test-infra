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
	"reflect"
	"testing"

	"github.com/golang/protobuf/proto"
	"k8s.io/test-infra/testgrid/state"
)

func Test_ValidateName(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		context   string
		timestamp string
		thread    string
		empty     bool
	}{

		{
			name:  "not junit",
			input: "./started.json",
			empty: true,
		},
		{
			name:  "forgot suffix",
			input: "./junit",
			empty: true,
		},
		{
			name:  "basic",
			input: "./junit.xml",
		},
		{
			name:    "context",
			input:   "./junit_hello world isn't-this exciting!.xml",
			context: "hello world isn't-this exciting!",
		},
		{
			name:    "numeric context",
			input:   "./junit_12345.xml",
			context: "12345",
		},
		{
			name:    "context and thread",
			input:   "./junit_context_12345.xml",
			context: "context",
			thread:  "12345",
		},
		{
			name:      "context and timestamp",
			input:     "./junit_context_20180102-1234.xml",
			context:   "context",
			timestamp: "20180102-1234",
		},
		{
			name:      "context thread timestamp",
			input:     "./junit_context_20180102-1234_5555.xml",
			context:   "context",
			timestamp: "20180102-1234",
			thread:    "5555",
		},
	}

	for _, tc := range cases {
		actual := ValidateName(tc.input)
		switch {
		case actual == nil && !tc.empty:
			t.Errorf("%s: unexpected nil map", tc.name)
		case actual != nil && tc.empty:
			t.Errorf("%s: should not have returned a map: %v", tc.name, actual)
		case actual != nil:
			for k, expected := range map[string]string{
				"Context":   tc.context,
				"Thread":    tc.thread,
				"Timestamp": tc.timestamp,
			} {
				if a, ok := actual[k]; !ok {
					t.Errorf("%s: missing key %s", tc.name, k)
				} else if a != expected {
					t.Errorf("%s: %s actual %s != expected %s", tc.name, k, a, expected)
				}
			}
		}
	}

}

func Test_ExtractRows(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		metadata map[string]string
		rows     map[string][]Row
		err      bool
	}{
		{
			name:    "not xml",
			content: `{"super": 123}`,
			err:     true,
		},
		{
			name:    "not junit",
			content: `<amazing><content/></amazing>`,
			err:     true,
		},
		{
			name: "basic testsuite",
			content: `
			  <testsuite>
			    <testcase name="good"/>
			    <testcase name="bad"><failure/></testcase>
			    <testcase name="skip"><skipped/></testcase>
			  </testsuite>`,
			rows: map[string][]Row{
				"good": {
					{
						Result: state.Row_PASS,
						Metadata: map[string]string{
							"Tests name": "good",
						},
					},
				},
				"bad": {
					{
						Result: state.Row_FAIL,
						Metadata: map[string]string{
							"Tests name": "bad",
						},
					},
				},
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
			rows: map[string][]Row{
				"good": {
					{
						Result: state.Row_PASS,
						Metadata: map[string]string{
							"Tests name": "good",
						},
					},
				},
				"bad": {
					{
						Result: state.Row_FAIL,
						Metadata: map[string]string{
							"Tests name": "bad",
						},
					},
				},
			},
		},
		{
			name: "suite name",
			content: `
			  <testsuite name="hello">
			    <testcase name="world" />
			  </testsuite>`,
			rows: map[string][]Row{
				"hello.world": {
					{
						Result: state.Row_PASS,
						Metadata: map[string]string{
							"Tests name": "hello.world",
						},
					},
				},
			},
		},
		{
			name: "duplicate target names",
			content: `
			  <testsuite>
			    <testcase name="multi">
			      <failure>doh</failure>
		            </testcase>
			    <testcase name="multi" />
			  </testsuite>`,
			rows: map[string][]Row{
				"multi": {
					{
						Result: state.Row_FAIL,
						Metadata: map[string]string{
							"Tests name": "multi",
						},
					},
					{
						Result: state.Row_PASS,
						Metadata: map[string]string{
							"Tests name": "multi",
						},
					},
				},
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
			rows: map[string][]Row{
				"slow": {
					{
						Result:  state.Row_PASS,
						Metrics: map[string]float64{elapsedKey: 100.1},
						Metadata: map[string]string{
							"Tests name": "slow",
						},
					},
				},
				"slow-failure": {
					{
						Result:  state.Row_FAIL,
						Metrics: map[string]float64{elapsedKey: 123456789},
						Metadata: map[string]string{
							"Tests name": "slow-failure",
						},
					},
				},
				"fast": {
					{
						Result:  state.Row_PASS,
						Metrics: map[string]float64{elapsedKey: 0.0001},
						Metadata: map[string]string{
							"Tests name": "fast",
						},
					},
				},
				"nothing-elapsed": {
					{
						Result: state.Row_PASS,
						Metadata: map[string]string{
							"Tests name": "nothing-elapsed",
						},
					},
				},
			},
		},
		{
			name: "add metadata",
			content: `
			  <testsuite>
			    <testcase name="fancy" />
			    <testcase name="ketchup" />
			  </testsuite>`,
			metadata: map[string]string{
				"Context":   "debian",
				"Timestamp": "1234",
				"Thread":    "7",
			},
			rows: map[string][]Row{
				"fancy": {
					{
						Result: state.Row_PASS,
						Metadata: map[string]string{
							"Tests name": "fancy",
							"Context":    "debian",
							"Timestamp":  "1234",
							"Thread":     "7",
						},
					},
				},
				"ketchup": {
					{
						Result: state.Row_PASS,
						Metadata: map[string]string{
							"Tests name": "ketchup",
							"Context":    "debian",
							"Timestamp":  "1234",
							"Thread":     "7",
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		rows := map[string][]Row{}

		rows, err := extractRows([]byte(tc.content), tc.metadata)
		switch {
		case err == nil && tc.err:
			t.Errorf("%s: failed to raise an error", tc.name)
		case err != nil && !tc.err:
			t.Errorf("%s: unexpected err: %v", tc.name, err)
		case len(rows) > len(tc.rows):
			t.Errorf("%s: extra rows: actual %v != expected %v", tc.name, rows, tc.rows)
		default:
			for target, expectedRows := range tc.rows {
				actualRows, ok := rows[target]
				if !ok {
					t.Errorf("%s: missing row %s", tc.name, target)
					continue
				} else if len(actualRows) != len(expectedRows) {
					t.Errorf("%s: bad results for %s: actual %v != expected %v", tc.name, target, actualRows, expectedRows)
					continue
				}
				for i, er := range expectedRows {
					ar := actualRows[i]
					if er.Result != ar.Result {
						t.Errorf("%s: %s %d actual %v != expected %v", tc.name, target, i, ar.Result, er.Result)
					}

					if len(ar.Metrics) > len(er.Metrics) {
						t.Errorf("%s: extra %s %d metrics: actual %v != expected %v", tc.name, target, i, ar.Metrics, er.Metrics)
					} else {
						for m, ev := range er.Metrics {
							if av, ok := ar.Metrics[m]; !ok {
								t.Errorf("%s: %s %d missing %s metric", tc.name, target, i, m)
							} else if ev != av {
								t.Errorf("%s: %s %d bad %s metric: actual %f != expected %f", tc.name, target, i, m, av, ev)
							}
						}
					}

					if len(ar.Metadata) > len(er.Metadata) {
						t.Errorf("%s: extra %s %d metadata: actual %v != expected %v", tc.name, target, i, ar.Metadata, er.Metadata)
					} else {
						for m, ev := range er.Metadata {
							if av, ok := ar.Metadata[m]; !ok {
								t.Errorf("%s: %s %d missing %s metadata", tc.name, target, i, m)
							} else if ev != av {
								t.Errorf("%s: %s %d bad %s metadata: actual %s != expected %s", tc.name, target, i, m, av, ev)
							}
						}
					}
				}
			}
		}
	}
}

func Test_MarshalGrid(t *testing.T) {
	g1 := state.Grid{
		Columns: []*state.Column{
			{Build: "alpha"},
			{Build: "second"},
		},
	}
	g2 := state.Grid{
		Columns: []*state.Column{
			{Build: "first"},
			{Build: "second"},
		},
	}

	b1, e1 := marshalGrid(g1)
	b2, e2 := marshalGrid(g2)
	uncompressed, e1a := proto.Marshal(&g1)

	switch {
	case e1 != nil, e2 != nil:
		t.Errorf("unexpected error %v %v %v", e1, e2, e1a)
	}

	if reflect.DeepEqual(b1, b2) {
		t.Errorf("unexpected equality %v == %v", b1, b2)
	}

	if reflect.DeepEqual(b1, uncompressed) {
		t.Errorf("should be compressed but is not: %v", b1)
	}
}
