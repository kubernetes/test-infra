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
	"k8s.io/test-infra/prow/scheduler/strategy"
)

func TestPassthrough(t *testing.T) {
	for _, tc := range []struct {
		name       string
		pj         *prowv1.ProwJob
		wantResult strategy.Result
	}{
		{
			name:       "Get the same cluster found on a Prowjob",
			pj:         &prowv1.ProwJob{Spec: prowv1.ProwJobSpec{Cluster: "build-cluster"}},
			wantResult: strategy.Result{Cluster: "build-cluster"},
		},
		{
			name:       "Empty cluster in, empty cluster out",
			pj:         &prowv1.ProwJob{},
			wantResult: strategy.Result{},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			passthrough := strategy.Passthrough{}
			result, err := passthrough.Schedule(context.TODO(), tc.pj)

			if err != nil {
				t.Errorf("Unexpected error: %s", err)
			}

			if diff := cmp.Diff(tc.wantResult, result); diff != "" {
				t.Errorf("Unexpected result: %s", diff)
			}
		})
	}
}
