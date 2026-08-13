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

package diff

import (
	"testing"

	"k8s.io/test-infra/gopherage/pkg/cov/junit/calculation"
)

func TestMakeTable(t *testing.T) {
	type args struct {
		baseCovList       *calculation.CoverageList
		newCovList        *calculation.CoverageList
		jobName           string
		coverageThreshold float32
	}
	tests := []struct {
		name              string
		args              args
		wantRes           string
		wantIsCoverageLow bool
	}{
		{
			name: "A",
			args: args{
				baseCovList: &calculation.CoverageList{
					Group: []calculation.Coverage{
						{Name: "a", NumCoveredStmts: 10, NumAllStmts: 100},
						{Name: "a2", NumCoveredStmts: 12, NumAllStmts: 100},
						{Name: "c", NumCoveredStmts: 20, NumAllStmts: 100},
						{Name: "d", NumCoveredStmts: 30, NumAllStmts: 100},
					},
				},
				newCovList: &calculation.CoverageList{
					Group: []calculation.Coverage{
						{Name: "a", NumCoveredStmts: 5, NumAllStmts: 100},
						{Name: "b", NumCoveredStmts: 10, NumAllStmts: 100},
						{Name: "c", NumCoveredStmts: 20, NumAllStmts: 100},
						{Name: "d", NumCoveredStmts: 40, NumAllStmts: 100},
					},
				},
				jobName:           "example-coverage-test",
				coverageThreshold: 30,
			},
			wantRes: "a | 10.0% | 5.0% | -5.0\n" +
				"b | Does not exist | 10.0% | \n" +
				"d | 30.0% | 40.0% | 10.0",
			wantIsCoverageLow: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRes, gotIsCoverageLow := makeTable(tt.args.baseCovList, tt.args.newCovList, tt.args.coverageThreshold)
			if gotRes != tt.wantRes {
				t.Errorf("makeTable() gotRes = %v, want %v", gotRes, tt.wantRes)
			}
			if gotIsCoverageLow != tt.wantIsCoverageLow {
				t.Errorf("makeTable() gotIsCoverageLow = %v, want %v", gotIsCoverageLow, tt.wantIsCoverageLow)
			}
		})
	}
}
