/*
Copyright 2023 The Kubernetes Authors.

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

	cfg "k8s.io/test-infra/prow/config"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		config  Config
		wantErr bool
	}{
		{config: Config{configPath: ""}, wantErr: true},
		{config: Config{configPath: "path/to/config"}, wantErr: false},
	}

	for _, test := range tests {
		err := test.config.validate()
		if (err != nil) != test.wantErr {
			t.Errorf("Expected error: %v, got: %v", test.wantErr, err != nil)
		}
	}
}

func TestGetPercentage(t *testing.T) {
	tests := []struct {
		int1 int
		int2 int
		want string
	}{
		{10, 100, "10.00%"},
		{1, 3, "33.33%"},
		{5, 5, "100.00%"},
	}

	for _, test := range tests {
		got := getPercentage(test.int1, test.int2)
		if got != test.want {
			t.Errorf("Expected: %v, got: %v", test.want, got)
		}
	}
}

func TestGetSortedKeys(t *testing.T) {
	m := map[string][]string{
		"b": {},
		"a": {},
		"c": {},
	}
	want := []string{"a", "b", "c"}

	got := getSortedKeys(m)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Expected: %v, got: %v", want, got)
			break
		}
	}
}

func TestGetClusterStatistics(t *testing.T) {
	jobs := map[string][]cfg.JobBase{}
	jobs["default"] = []cfg.JobBase{
		{Cluster: "default", Name: "job1"},
		{Cluster: "default", Name: "job2"},
		{Cluster: "cluster2", Name: "job3"},
	}

	want := map[string][]string{
		"default":  {"job1", "job2"},
		"cluster2": {"job3"},
	}

	got := getClusterStatistics(jobs)
	for key := range want {
		for i := range want[key] {
			if got[key][i] != want[key][i] {
				t.Errorf("Expected: %v, got: %v", want, got)
				break
			}
		}
	}
}
