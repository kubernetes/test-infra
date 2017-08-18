/*
Copyright 2017 The Kubernetes Authors.

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

package npj

import (
	"reflect"
	"testing"

	"k8s.io/test-infra/prow/kube"
)

func TestEnvironmentForSpec(t *testing.T) {
	var tests = []struct {
		name     string
		spec     kube.ProwJobSpec
		expected map[string]string
	}{
		{
			name: "periodic job",
			spec: kube.ProwJobSpec{
				Type: kube.PeriodicJob,
				Job:  "job-name",
			},
			expected: map[string]string{
				"JOB_NAME": "job-name",
			},
		},
		{
			name: "postsubmit job",
			spec: kube.ProwJobSpec{
				Type: kube.PostsubmitJob,
				Job:  "job-name",
				Refs: kube.Refs{
					Org:     "org-name",
					Repo:    "repo-name",
					BaseRef: "base-ref",
					BaseSHA: "base-sha",
				},
			},
			expected: map[string]string{
				"JOB_NAME":      "job-name",
				"REPO_OWNER":    "org-name",
				"REPO_NAME":     "repo-name",
				"PULL_BASE_REF": "base-ref",
				"PULL_BASE_SHA": "base-sha",
				"PULL_REFS":     "base-ref:base-sha",
			},
		},
		{
			name: "batch job",
			spec: kube.ProwJobSpec{
				Type: kube.BatchJob,
				Job:  "job-name",
				Refs: kube.Refs{
					Org:     "org-name",
					Repo:    "repo-name",
					BaseRef: "base-ref",
					BaseSHA: "base-sha",
					Pulls: []kube.Pull{{
						Number: 1,
						Author: "author-name",
						SHA:    "pull-sha",
					}, {
						Number: 2,
						Author: "other-author-name",
						SHA:    "second-pull-sha",
					}},
				},
			},
			expected: map[string]string{
				"JOB_NAME":      "job-name",
				"REPO_OWNER":    "org-name",
				"REPO_NAME":     "repo-name",
				"PULL_BASE_REF": "base-ref",
				"PULL_BASE_SHA": "base-sha",
				"PULL_REFS":     "base-ref:base-sha,1:pull-sha,2:second-pull-sha",
			},
		},
		{
			name: "presubmit job",
			spec: kube.ProwJobSpec{
				Type: kube.PresubmitJob,
				Job:  "job-name",
				Refs: kube.Refs{
					Org:     "org-name",
					Repo:    "repo-name",
					BaseRef: "base-ref",
					BaseSHA: "base-sha",
					Pulls: []kube.Pull{{
						Number: 1,
						Author: "author-name",
						SHA:    "pull-sha",
					}},
				},
			},
			expected: map[string]string{
				"JOB_NAME":      "job-name",
				"REPO_OWNER":    "org-name",
				"REPO_NAME":     "repo-name",
				"PULL_BASE_REF": "base-ref",
				"PULL_BASE_SHA": "base-sha",
				"PULL_REFS":     "base-ref:base-sha,1:pull-sha",
				"PULL_NUMBER":   "1",
				"PULL_PULL_SHA": "pull-sha",
			},
		},
	}

	for _, test := range tests {
		if actual, expected := EnvForSpec(test.spec), test.expected; !reflect.DeepEqual(actual, expected) {
			t.Errorf("%s: got environment:\n\t%v\n\tbut expected:\n\t%v", test.name, actual, expected)
		}
	}
}
