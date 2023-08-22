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
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
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
		{0, 0, "100.00%"},
	}

	for _, test := range tests {
		got := getPercentage(test.int1, test.int2)
		if got != test.want {
			t.Errorf("Expected: %v, got: %v", test.want, got)
		}
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		s        string
		list     []string
		expected bool
	}{
		{"testString", []string{"test", "string"}, true},
		{"testString", []string{"none", "of", "these"}, false},
		{"", []string{"none", "of", "these"}, false},
	}

	for _, tt := range tests {
		result := containsAny(tt.s, tt.list)
		if result != tt.expected {
			t.Errorf("for input %q and list %v, expected %v, got %v", tt.s, tt.list, tt.expected, result)
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

func TestContainsDisallowedLabel(t *testing.T) {
	tests := []struct {
		labels   map[string]string
		expected bool
	}{
		{map[string]string{"name": "value"}, false},
		{map[string]string{"credName": "value"}, true},
		{map[string]string{"name": "credValue"}, false},
		{map[string]string{}, false},
	}

	for _, tt := range tests {
		result := containsDisallowedLabel(tt.labels)
		if result != tt.expected {
			t.Errorf("for labels %v, expected %v, got %v", tt.labels, tt.expected, result)
		}
	}
}

func TestContainsDisallowedAttributes(t *testing.T) {
	tests := []struct {
		container v1.Container
		expected  bool
	}{
		{
			v1.Container{
				Env:     []v1.EnvVar{{Name: "NAME", Value: "value"}},
				Args:    []string{"arg1", "arg2"},
				Command: []string{"cmd1", "cmd2"},
			},
			false,
		},
		{
			v1.Container{
				Env:     []v1.EnvVar{{Name: "credName", Value: "value"}},
				Args:    []string{"arg1", "arg2"},
				Command: []string{"cmd1", "cmd2"},
			},
			true,
		},
		{
			v1.Container{
				Env:     []v1.EnvVar{{Name: "NAME", Value: "value"}},
				Args:    []string{"arg1", "gcloud"},
				Command: []string{"cmd1", "cmd2"},
			},
			true,
		},
	}

	for _, tt := range tests {
		result := containsDisallowedAttributes(tt.container)
		if result != tt.expected {
			t.Errorf("for container %v, expected %v, got %v", tt.container, tt.expected, result)
		}
	}
}

func TestContainsDisallowedVolumeMount(t *testing.T) {
	tests := []struct {
		volumeMounts []v1.VolumeMount
		expected     bool
	}{
		{
			[]v1.VolumeMount{
				{Name: "name1", MountPath: "/path/to/dir1"},
				{Name: "name2", MountPath: "/path/to/dir2"},
			},
			false,
		},
		{
			[]v1.VolumeMount{
				{Name: "credName", MountPath: "/path/to/dir1"},
				{Name: "name2", MountPath: "/path/to/dir2"},
			},
			true,
		},
		{
			[]v1.VolumeMount{
				{Name: "name1", MountPath: "/path/to/credDir"},
				{Name: "name2", MountPath: "/path/to/dir2"},
			},
			true,
		},
	}

	for _, tt := range tests {
		result := containsDisallowedVolumeMount(tt.volumeMounts)
		if result != tt.expected {
			t.Errorf("for volumeMounts %v, expected %v, got %v", tt.volumeMounts, tt.expected, result)
		}
	}
}

func TestContainsDisallowedVolume(t *testing.T) {
	tests := []struct {
		volumes  []v1.Volume
		expected bool
	}{
		{
			[]v1.Volume{{Name: "name1"}, {Name: "name2"}},
			false,
		},
		{
			[]v1.Volume{{Name: "credName"}, {Name: "name2"}},
			true,
		},
	}

	for _, tt := range tests {
		result := containsDisallowedVolume(tt.volumes)
		if result != tt.expected {
			t.Errorf("for volumes %v, expected %v, got %v", tt.volumes, tt.expected, result)
		}
	}
}

func TestCheckIfEligable(t *testing.T) {
	tests := []struct {
		job      cfg.JobBase
		expected bool
	}{
		// Add various JobBase configurations and test the expected outcome
		// For example:
		{
			cfg.JobBase{Cluster: "test-infra-trusted"},
			true,
		},
		// ... other test cases ...
	}

	for _, tt := range tests {
		result := checkIfEligible(tt.job)
		if result != tt.expected {
			t.Errorf("for job %v, expected %v, got %v", tt.job, tt.expected, result)
		}
	}
}

func TestAllStaticJobs(t *testing.T) {
	tests := []struct {
		config   *cfg.Config
		expected map[string][]cfg.JobBase
	}{
		// Scenario: No jobs in any of the sources
		{
			&cfg.Config{JobConfig: cfg.JobConfig{}},
			map[string][]cfg.JobBase{},
		},
		// Scenario: One job in PresubmitsStatic
		{
			&cfg.Config{
				JobConfig: cfg.JobConfig{
					PresubmitsStatic: map[string][]cfg.Presubmit{
						"path/to/repo1": {{JobBase: cfg.JobBase{Name: "job1"}}},
					},
				},
			},
			map[string][]cfg.JobBase{"to": {{Name: "job1"}}},
		},
		// Scenario: Multiple jobs in PresubmitsStatic for multiple repos
		{
			&cfg.Config{
				JobConfig: cfg.JobConfig{
					PresubmitsStatic: map[string][]cfg.Presubmit{
						"path/to/repo1": {
							{JobBase: cfg.JobBase{Name: "job1"}},
							{JobBase: cfg.JobBase{Name: "job2"}},
						},
						"path/for/repo2": {
							{JobBase: cfg.JobBase{Name: "jobA"}},
						},
					},
				},
			},
			map[string][]cfg.JobBase{
				"to":  {{Name: "job1"}, {Name: "job2"}},
				"for": {{Name: "jobA"}},
			},
		},
		// Scenario: One job in PostsubmitsStatic
		{
			&cfg.Config{
				JobConfig: cfg.JobConfig{
					PostsubmitsStatic: map[string][]cfg.Postsubmit{
						"path/to/repo1": {{JobBase: cfg.JobBase{Name: "postJob1"}}},
					},
				},
			},
			map[string][]cfg.JobBase{"to": {{Name: "postJob1"}}},
		},
		// Scenario: Multiple jobs in PostsubmitsStatic for multiple repos
		{
			&cfg.Config{
				JobConfig: cfg.JobConfig{
					PostsubmitsStatic: map[string][]cfg.Postsubmit{
						"path/to/repo1": {
							{JobBase: cfg.JobBase{Name: "postJob1"}},
							{JobBase: cfg.JobBase{Name: "postJob2"}},
						},
						"path/for/repo2": {
							{JobBase: cfg.JobBase{Name: "postJobA"}},
						},
					},
				},
			},
			map[string][]cfg.JobBase{
				"to":  {{Name: "postJob1"}, {Name: "postJob2"}},
				"for": {{Name: "postJobA"}},
			},
		},
	}

	for _, tt := range tests {
		result := allStaticJobs(tt.config)
		// DeepEqual check for map equality
		if !reflect.DeepEqual(result, tt.expected) {
			t.Errorf("for config %v, expected %v, got %v", tt.config, tt.expected, result)
		}
	}
}
