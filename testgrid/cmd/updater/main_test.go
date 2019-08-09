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

	"k8s.io/test-infra/testgrid/metadata/junit"
	"k8s.io/test-infra/testgrid/state"
)

func TestExtractRows(t *testing.T) {
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
		t.Run(tc.name, func(t *testing.T) {
			rows := map[string][]Row{}

			suites, err := junit.Parse([]byte(tc.content))
			if err == nil {
				rows = extractRows(suites, tc.metadata)
			}
			switch {
			case err == nil && tc.err:
				t.Error("failed to raise an error")
			case err != nil && !tc.err:
				t.Errorf("unexpected err: %v", err)
			case len(rows) > len(tc.rows):
				t.Errorf("extra rows: actual %v != expected %v", rows, tc.rows)
			default:
				for target, expectedRows := range tc.rows {
					actualRows, ok := rows[target]
					if !ok {
						t.Errorf("missing row %s", target)
						continue
					} else if len(actualRows) != len(expectedRows) {
						t.Errorf("bad results for %s: actual %v != expected %v", target, actualRows, expectedRows)
						continue
					}
					for i, er := range expectedRows {
						ar := actualRows[i]
						if er.Result != ar.Result {
							t.Errorf("%s %d actual %v != expected %v", target, i, ar.Result, er.Result)
						}

						if len(ar.Metrics) > len(er.Metrics) {
							t.Errorf("extra %s %d metrics: actual %v != expected %v", target, i, ar.Metrics, er.Metrics)
						} else {
							for m, ev := range er.Metrics {
								if av, ok := ar.Metrics[m]; !ok {
									t.Errorf("%s %d missing %s metric", target, i, m)
								} else if ev != av {
									t.Errorf("%s %d bad %s metric: actual %f != expected %f", target, i, m, av, ev)
								}
							}
						}

						if len(ar.Metadata) > len(er.Metadata) {
							t.Errorf("extra %s %d metadata: actual %v != expected %v", target, i, ar.Metadata, er.Metadata)
						} else {
							for m, ev := range er.Metadata {
								if av, ok := ar.Metadata[m]; !ok {
									t.Errorf("%s %d missing %s metadata", target, i, m)
								} else if ev != av {
									t.Errorf("%s %d bad %s metadata: actual %s != expected %s", target, i, m, av, ev)
								}
							}
						}
					}
				}
			}
		})
	}
}

func TestMarshalGrid(t *testing.T) {
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
