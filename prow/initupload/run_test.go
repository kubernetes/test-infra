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

package initupload

import (
	"reflect"
	"testing"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/pod-utils/clone"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/gcs"
)

func TestSpecToStarted(t *testing.T) {
	var tests = []struct {
		name         string
		spec         downwardapi.JobSpec
		cloneRecords []clone.Record
		expected     gcs.Started
	}{
		{
			name: "Refs with Pull",
			spec: downwardapi.JobSpec{
				Refs: &prowapi.Refs{
					Org:     "kubernetes",
					Repo:    "test-infra",
					BaseRef: "master",
					BaseSHA: "deadbeef",
					Pulls: []prowapi.Pull{
						{
							Number: 123,
							SHA:    "abcd1234",
						},
					},
				},
			},
			expected: gcs.Started{
				Pull:                  "123",
				DeprecatedRepoVersion: "abcd1234",
				RepoCommit:            "abcd1234",
				Repos: map[string]string{
					"kubernetes/test-infra": "master:deadbeef,123:abcd1234",
				},
			},
		},
		{
			name: "Refs with BaseRef only",
			spec: downwardapi.JobSpec{
				Refs: &prowapi.Refs{
					Org:     "kubernetes",
					Repo:    "test-infra",
					BaseRef: "master",
				},
			},
			expected: gcs.Started{
				DeprecatedRepoVersion: "master",
				RepoCommit:            "master",
				Repos: map[string]string{
					"kubernetes/test-infra": "master",
				},
			},
		},
		{
			name: "Refs with BaseSHA and ExtraRef",
			spec: downwardapi.JobSpec{
				Refs: &prowapi.Refs{
					Org:     "kubernetes",
					Repo:    "test-infra",
					BaseRef: "master",
					BaseSHA: "deadbeef",
				},
				ExtraRefs: []prowapi.Refs{
					{
						Org:     "kubernetes",
						Repo:    "release",
						BaseRef: "v1.10",
					},
				},
			},
			expected: gcs.Started{
				DeprecatedRepoVersion: "deadbeef",
				RepoCommit:            "deadbeef",
				Repos: map[string]string{
					"kubernetes/test-infra": "master:deadbeef",
					"kubernetes/release":    "v1.10",
				},
			},
		},
		{
			name: "Refs with ExtraRef and cloneRecords containing a final SHA",
			spec: downwardapi.JobSpec{
				Refs: &prowapi.Refs{
					Org:     "kubernetes",
					Repo:    "test-infra",
					BaseRef: "master",
				},
				ExtraRefs: []prowapi.Refs{
					{
						Org:     "kubernetes",
						Repo:    "release",
						BaseRef: "v1.10",
					},
				},
			},
			cloneRecords: []clone.Record{{
				Refs: prowapi.Refs{
					Org:     "kubernetes",
					Repo:    "test-infra",
					BaseRef: "master",
				},
				FinalSHA: "aaaaaaaa",
			}},
			expected: gcs.Started{
				DeprecatedRepoVersion: "aaaaaaaa",
				RepoCommit:            "aaaaaaaa",
				Repos: map[string]string{
					"kubernetes/test-infra": "master",
					"kubernetes/release":    "v1.10",
				},
			},
		},
		{
			name: "Refs with only ExtraRef and cloneRecords containing a final SHA",
			spec: downwardapi.JobSpec{
				ExtraRefs: []prowapi.Refs{
					{
						Org:     "kubernetes",
						Repo:    "release",
						BaseRef: "v1.10",
					},
				},
			},
			cloneRecords: []clone.Record{{
				Refs: prowapi.Refs{
					Org:     "kubernetes",
					Repo:    "release",
					BaseRef: "v1.10",
				},
				FinalSHA: "aaaaaaaa",
			}},
			expected: gcs.Started{
				DeprecatedRepoVersion: "aaaaaaaa",
				RepoCommit:            "aaaaaaaa",
				Repos: map[string]string{
					"kubernetes/release": "v1.10",
				},
			},
		},
	}

	for _, test := range tests {
		actual, expected := specToStarted(&test.spec, test.cloneRecords), test.expected
		expected.Timestamp = actual.Timestamp
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("%s: got started: %#v, but expected: %#v", test.name, actual, expected)
		}
	}
}
