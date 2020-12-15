/*
Copyright 2020 The Kubernetes Authors.

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
	"flag"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func newGkeDeployerFactory(instance_group_prefix string) (*gkeDeployer, error) {
	flag.Set("gke-environment", "staging")
	if instance_group_prefix != "" {
		flag.Set("gke-instance-group-prefix", instance_group_prefix)
	} else {
		flag.Set("gke-instance-group-prefix", "gke")
	}

	return newGKE(
		"gke",
		"project",
		"",
		"region",
		"network",
		"image",
		"imageFamily",
		"imageProject",
		"cluster",
		"sshProxyInstanceName",
		new(string),
		new(string),
	)
}

func TestParseInstanceGroupsFromGcloud(t *testing.T) {
	urlTemplate := `https://www.googleapis.com/compute/v1/projects/test-project/zones/%s/instanceGroupManagers/%s-default-pool-6f8ec327-grp`
	zoneA := "us-central1-a"
	zoneB := "us-central1-b"
	zoneC := "us-central1-c"
	urlZoneADefault := fmt.Sprintf(urlTemplate, zoneA, "gke")
	urlZoneBDefault := fmt.Sprintf(urlTemplate, zoneB, "gke")
	urlZoneCDefault := fmt.Sprintf(urlTemplate, zoneC, "gke")
	urlZoneATest := fmt.Sprintf(urlTemplate, zoneA, "test")
	urlZoneBTest := fmt.Sprintf(urlTemplate, zoneB, "test")
	urlZoneCTest := fmt.Sprintf(urlTemplate, zoneC, "test")

	cases := []struct {
		name                  string
		input                 string
		instance_group_prefix string
		expected              []*ig
		expectedError         bool
	}{
		// Zonal
		{
			name:                  "One instance group with default prefix",
			input:                 urlZoneADefault,
			instance_group_prefix: "",
			expected: []*ig{
				{
					path: `zones/us-central1-a/instanceGroupManagers/gke-default-pool-6f8ec327-grp`,
					zone: "us-central1-a",
					name: `gke-default-pool-6f8ec327-grp`,
					uniq: `6f8ec327`,
				},
			},
			expectedError: false,
		},
		{
			name:                  "One instance group with default prefix, but not matching instance_group_prefix",
			input:                 urlZoneADefault,
			instance_group_prefix: "test",
			expected:              nil,
			expectedError:         true,
		},
		{
			name:                  "One instance group with test prefix",
			input:                 urlZoneATest,
			instance_group_prefix: "test",
			expected: []*ig{
				{
					path: `zones/us-central1-a/instanceGroupManagers/test-default-pool-6f8ec327-grp`,
					zone: "us-central1-a",
					name: `test-default-pool-6f8ec327-grp`,
					uniq: `6f8ec327`,
				},
			},
			expectedError: false,
		},
		// Regional
		{
			name:                  "Three instance groups with default prefix",
			input:                 strings.Join([]string{urlZoneADefault, urlZoneBDefault, urlZoneCDefault}, ";"),
			instance_group_prefix: "",
			expected: []*ig{
				{
					path: `zones/us-central1-a/instanceGroupManagers/gke-default-pool-6f8ec327-grp`,
					zone: "us-central1-a",
					name: `gke-default-pool-6f8ec327-grp`,
					uniq: `6f8ec327`,
				},
				{
					path: `zones/us-central1-b/instanceGroupManagers/gke-default-pool-6f8ec327-grp`,
					zone: "us-central1-b",
					name: `gke-default-pool-6f8ec327-grp`,
					uniq: `6f8ec327`,
				},
				{
					path: `zones/us-central1-c/instanceGroupManagers/gke-default-pool-6f8ec327-grp`,
					zone: "us-central1-c",
					name: `gke-default-pool-6f8ec327-grp`,
					uniq: `6f8ec327`,
				},
			},
			expectedError: false,
		},
		{
			name:                  "Three instance groups with default prefix, but not matching instance_group_prefix",
			input:                 strings.Join([]string{urlZoneADefault, urlZoneBDefault, urlZoneCDefault}, ";"),
			instance_group_prefix: "test",
			expected:              nil,
			expectedError:         true,
		},
		{
			name:                  "Three instance groups with test prefix",
			input:                 strings.Join([]string{urlZoneATest, urlZoneBTest, urlZoneCTest}, ";"),
			instance_group_prefix: "test",
			expected: []*ig{
				{
					path: `zones/us-central1-a/instanceGroupManagers/test-default-pool-6f8ec327-grp`,
					zone: "us-central1-a",
					name: `test-default-pool-6f8ec327-grp`,
					uniq: `6f8ec327`,
				},
				{
					path: `zones/us-central1-b/instanceGroupManagers/test-default-pool-6f8ec327-grp`,
					zone: "us-central1-b",
					name: `test-default-pool-6f8ec327-grp`,
					uniq: `6f8ec327`,
				},
				{
					path: `zones/us-central1-c/instanceGroupManagers/test-default-pool-6f8ec327-grp`,
					zone: "us-central1-c",
					name: `test-default-pool-6f8ec327-grp`,
					uniq: `6f8ec327`,
				},
			},
			expectedError: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deployer, err := newGkeDeployerFactory(tc.instance_group_prefix)
			if err != nil {
				t.Errorf("Error creating gke deployer: %v", err)
			}
			instanceGroups, err := deployer.parseInstanceGroupsFromGcloud(tc.input)
			if err != nil && tc.expectedError == false {
				t.Errorf("Returned error, expected some output. Error: %v, expected: %v", err, tc.expected)
			} else if !reflect.DeepEqual(instanceGroups, tc.expected) {
				t.Errorf("Not equal: %v %v", instanceGroups, tc.expected)
			}
		})
	}
}
