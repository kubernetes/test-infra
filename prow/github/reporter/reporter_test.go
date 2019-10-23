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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/gerrit/client"
)

func TestShouldReport(t *testing.T) {
	var testcases = []struct {
		name        string
		pj          v1.ProwJob
		report      bool
		reportAgent v1.ProwJobAgent
	}{
		{
			name: "should not report skip report job",
			pj: v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type:   v1.PresubmitJob,
					Report: false,
				},
			},
			report: false,
		},
		{
			name: "should not report periodic job",
			pj: v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type:   v1.PeriodicJob,
					Report: true,
				},
			},
			report: false,
		},
		{
			name: "should report postsubmit job",
			pj: v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type:   v1.PostsubmitJob,
					Report: true,
				},
			},
			report: true,
		},
		{
			name: "should not report batch job",
			pj: v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type:   v1.BatchJob,
					Report: true,
				},
			},
			report: false,
		},
		{
			name: "should report presubmit job",
			pj: v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type:   v1.PresubmitJob,
					Report: true,
				},
			},
			report: true,
		},
		{
			name: "knative only, don't report kubernetes agent job",
			pj: v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type:   v1.PresubmitJob,
					Agent:  v1.KubernetesAgent,
					Report: true,
				},
			},
			report:      false,
			reportAgent: v1.KnativeBuildAgent,
		},
		{
			name: "knative only, report knative agent job",
			pj: v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Type:   v1.PresubmitJob,
					Agent:  v1.KnativeBuildAgent,
					Report: true,
				},
			},
			report:      true,
			reportAgent: v1.KnativeBuildAgent,
		},
		{
			name: "github should not report gerrit jobs",
			pj: v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritReportLabel: "plus-one-this-gerrit-label-please",
					},
				},
				Spec: v1.ProwJobSpec{
					Type:   v1.PresubmitJob,
					Report: true,
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewReporter(nil, nil, tc.reportAgent)
			if r := c.ShouldReport(&tc.pj); r == tc.report {
				return
			}
			if tc.report {
				t.Error("failed to report")
			} else {
				t.Error("unexpectedly reported")
			}
		})
	}
}
