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
	"testing"

	"github.com/GoogleCloudPlatform/testgrid/resultstore"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/diff"
)

func TestProwJobName(t *testing.T) {
	cases := []struct {
		name            string
		url             string
		expectedJobName string
	}{
		{
			name:            "wrong url with only bucket name returns empty job name",
			url:             "gs://bucket",
			expectedJobName: "",
		},
		{
			name:            "wrong url path with no job name returns empty job name",
			url:             "gs://bucket/logs",
			expectedJobName: "",
		},
		{
			name:            "wrong url path with no job name and trailing slash returns empty job name",
			url:             "gs://bucket/logs/",
			expectedJobName: "",
		},
		{
			name:            "good job name",
			url:             "gs://bucket/logs/jobA/1234567890123456789",
			expectedJobName: "jobA",
		},
		{
			name:            "good job name with trailing slash",
			url:             "gs://bucket/logs/jobA/1234567890123456789/",
			expectedJobName: "jobA",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			urlPath, err := gcs.NewPath(tc.url)
			if err != nil {
				t.Errorf("incorrect url: %v", err)
			}

			job := prowJobName(*urlPath)
			if job != tc.expectedJobName {
				t.Errorf("incorrect job name: got %q, expected %q", job, tc.expectedJobName)
			}
		})
	}
}

func TestStartedReposToProperties(t *testing.T) {
	cases := []struct {
		name     string
		repos    map[string]string
		expected []resultstore.Property
	}{
		{
			name: "empty repos convert to no properties",
		},
		{
			name: "convert repos to properties",
			repos: map[string]string{
				"org1/repo1": "master",
				"org2/repo2": "branch1",
			},
			expected: []resultstore.Property{
				{
					Key:   "Org",
					Value: "org1",
				},
				{
					Key:   "Branch",
					Value: "master",
				},
				{
					Key:   "Repo",
					Value: "repo1",
				},
				{
					Key:   "Repo",
					Value: "org1/repo1",
				},
				{
					Key:   "Repo",
					Value: "org1/repo1:master",
				},
				{
					Key:   "Org",
					Value: "org2",
				},
				{
					Key:   "Branch",
					Value: "branch1",
				},
				{
					Key:   "Repo",
					Value: "repo2",
				},
				{
					Key:   "Repo",
					Value: "org2/repo2",
				},
				{
					Key:   "Repo",
					Value: "org2/repo2:branch1",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			properties := startedReposToProperties(tc.repos)
			if !equality.Semantic.DeepEqual(properties, tc.expected) {
				t.Errorf(diff.ObjectReflectDiff(properties, tc.expected))
			}
		})
	}
}
