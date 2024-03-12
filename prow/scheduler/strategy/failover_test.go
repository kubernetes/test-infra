/*
Copyright 2024 The Kubernetes Authors.

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

package strategy_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/scheduler/strategy"
)

func TestFailover(t *testing.T) {
	for _, tc := range []struct {
		name         string
		mappings     map[string]string
		pj           *prowv1.ProwJob
		wantDecision strategy.Result
	}{
		{
			name:         "Replace broken cluster",
			mappings:     map[string]string{"broken": "running"},
			pj:           &prowv1.ProwJob{Spec: prowv1.ProwJobSpec{Cluster: "broken"}},
			wantDecision: strategy.Result{Cluster: "running"},
		},
		{
			name:         "Do not replace",
			mappings:     map[string]string{"broken": "running"},
			pj:           &prowv1.ProwJob{Spec: prowv1.ProwJobSpec{Cluster: "a-cluster"}},
			wantDecision: strategy.Result{Cluster: "a-cluster"},
		},
		{
			name:         "No mappings, do not replace",
			pj:           &prowv1.ProwJob{Spec: prowv1.ProwJobSpec{Cluster: "a-cluster"}},
			wantDecision: strategy.Result{Cluster: "a-cluster"},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			failover := strategy.NewFailover(config.FailoverScheduling{ClusterMappings: tc.mappings})

			d, err := failover.Schedule(context.TODO(), tc.pj)
			if err != nil {
				t.Fatalf("Unexpected error: %s", err)
			}

			if diff := cmp.Diff(tc.wantDecision, d); diff != "" {
				t.Errorf("Unexpected decisions: %s", diff)
			}
		})
	}
}
