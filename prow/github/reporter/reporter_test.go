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

package reporter

import (
	"testing"

	"k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func TestShouldReport(t *testing.T) {
	var testcases = []struct {
		name   string
		pj     *v1.ProwJob
		report bool
	}{
		{
			name: "should not report skip report job",
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type:   v1.PresubmitJob,
					Report: false,
				},
			},
			report: false,
		},
		{
			name: "should not report periodic job",
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type:   v1.PeriodicJob,
					Report: true,
				},
			},
			report: false,
		},
		{
			name: "should not report postsubmit job",
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type:   v1.PostsubmitJob,
					Report: true,
				},
			},
			report: false,
		},
		{
			name: "should not report batch job",
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type:   v1.BatchJob,
					Report: true,
				},
			},
			report: false,
		},
		{
			name: "only report presubmit job",
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type:   v1.PresubmitJob,
					Report: true,
				},
			},
			report: true,
		},
	}

	c := NewReporter(nil, nil)

	for _, tc := range testcases {
		r := c.ShouldReport(tc.pj)

		if r != tc.report {
			t.Errorf("Unexpected result from test: %s.\nExpected: %v\nGot: %v",
				tc.name, tc.report, r)
		}
	}
}
