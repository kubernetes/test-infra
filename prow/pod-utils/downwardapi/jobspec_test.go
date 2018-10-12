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

package downwardapi

import (
	"reflect"
	"testing"

	"k8s.io/test-infra/prow/kube"
)

func TestEnvironmentForSpec(t *testing.T) {
	var tests = []struct {
		name     string
		spec     JobSpec
		expected map[string]string
	}{
		{
			name: "periodic job",
			spec: JobSpec{
				Type:      kube.PeriodicJob,
				Job:       "job-name",
				BuildID:   "0",
				ProwJobID: "prowjob",
			},
			expected: map[string]string{
				"JOB_NAME":    "job-name",
				"BUILD_ID":    "0",
				"PROW_JOB_ID": "prowjob",
				"JOB_TYPE":    "periodic",
				"JOB_SPEC":    `{"type":"periodic","job":"job-name","buildid":"0","prowjobid":"prowjob","refs":{}}`,
			},
		},
		{
			name: "postsubmit job",
			spec: JobSpec{
				Type:      kube.PostsubmitJob,
				Job:       "job-name",
				BuildID:   "0",
				ProwJobID: "prowjob",
				Refs: kube.Refs{
					Org:     "org-name",
					Repo:    "repo-name",
					BaseRef: "base-ref",
					BaseSHA: "base-sha",
				},
			},
			expected: map[string]string{
				"JOB_NAME":      "job-name",
				"BUILD_ID":      "0",
				"PROW_JOB_ID":   "prowjob",
				"JOB_TYPE":      "postsubmit",
				"JOB_SPEC":      `{"type":"postsubmit","job":"job-name","buildid":"0","prowjobid":"prowjob","refs":{"org":"org-name","repo":"repo-name","base_ref":"base-ref","base_sha":"base-sha"}}`,
				"REPO_OWNER":    "org-name",
				"REPO_NAME":     "repo-name",
				"PULL_BASE_REF": "base-ref",
				"PULL_BASE_SHA": "base-sha",
				"PULL_REFS":     "base-ref:base-sha",
			},
		},
		{
			name: "batch job",
			spec: JobSpec{
				Type:      kube.BatchJob,
				Job:       "job-name",
				BuildID:   "0",
				ProwJobID: "prowjob",
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
				"BUILD_ID":      "0",
				"PROW_JOB_ID":   "prowjob",
				"JOB_TYPE":      "batch",
				"JOB_SPEC":      `{"type":"batch","job":"job-name","buildid":"0","prowjobid":"prowjob","refs":{"org":"org-name","repo":"repo-name","base_ref":"base-ref","base_sha":"base-sha","pulls":[{"number":1,"author":"author-name","sha":"pull-sha"},{"number":2,"author":"other-author-name","sha":"second-pull-sha"}]}}`,
				"REPO_OWNER":    "org-name",
				"REPO_NAME":     "repo-name",
				"PULL_BASE_REF": "base-ref",
				"PULL_BASE_SHA": "base-sha",
				"PULL_REFS":     "base-ref:base-sha,1:pull-sha,2:second-pull-sha",
			},
		},
		{
			name: "presubmit job",
			spec: JobSpec{
				Type:      kube.PresubmitJob,
				Job:       "job-name",
				BuildID:   "0",
				ProwJobID: "prowjob",
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
				"BUILD_ID":      "0",
				"PROW_JOB_ID":   "prowjob",
				"JOB_TYPE":      "presubmit",
				"JOB_SPEC":      `{"type":"presubmit","job":"job-name","buildid":"0","prowjobid":"prowjob","refs":{"org":"org-name","repo":"repo-name","base_ref":"base-ref","base_sha":"base-sha","pulls":[{"number":1,"author":"author-name","sha":"pull-sha"}]}}`,
				"REPO_OWNER":    "org-name",
				"REPO_NAME":     "repo-name",
				"PULL_BASE_REF": "base-ref",
				"PULL_BASE_SHA": "base-sha",
				"PULL_REFS":     "base-ref:base-sha,1:pull-sha",
				"PULL_NUMBER":   "1",
				"PULL_PULL_SHA": "pull-sha",
			},
		},
		{
			name: "kubernetes agent",
			spec: JobSpec{
				Type:      kube.PeriodicJob,
				Job:       "job-name",
				BuildID:   "0",
				ProwJobID: "prowjob",
				agent:     kube.KubernetesAgent,
			},
			expected: map[string]string{
				"JOB_NAME":     "job-name",
				"BUILD_ID":     "0",
				"PROW_JOB_ID":  "prowjob",
				"BUILD_NUMBER": "0",
				"JOB_TYPE":     "periodic",
				"JOB_SPEC":     `{"type":"periodic","job":"job-name","buildid":"0","prowjobid":"prowjob","refs":{}}`,
			},
		},
		{
			name: "jenkins agent",
			spec: JobSpec{
				Type:      kube.PeriodicJob,
				Job:       "job-name",
				BuildID:   "0",
				ProwJobID: "prowjob",
				agent:     kube.JenkinsAgent,
			},
			expected: map[string]string{
				"JOB_NAME":    "job-name",
				"BUILD_ID":    "0",
				"PROW_JOB_ID": "prowjob",
				"JOB_TYPE":    "periodic",
				"JOB_SPEC":    `{"type":"periodic","job":"job-name","buildid":"0","prowjobid":"prowjob","refs":{}}`,
			},
		},
	}

	for _, test := range tests {
		env, err := EnvForSpec(test.spec)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", test.name, err)
		}
		if actual, expected := env, test.expected; !reflect.DeepEqual(actual, expected) {
			t.Errorf("%s: got environment:\n\t%v\n\tbut expected:\n\t%v", test.name, actual, expected)
		}
	}
}
