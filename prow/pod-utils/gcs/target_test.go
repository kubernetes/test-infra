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

package gcs

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/equality"

	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
)

func TestPathForSpec(t *testing.T) {
	testCases := []struct {
		name     string
		spec     *downwardapi.JobSpec
		builder  RepoPathBuilder
		expected string
	}{
		{
			name: "periodic",
			spec: &downwardapi.JobSpec{
				Type:    kube.PeriodicJob,
				Job:     "job",
				BuildID: "number",
			},
			expected: "logs/job/number",
		},
		{
			name: "postsubmit",
			spec: &downwardapi.JobSpec{Type: kube.PostsubmitJob,
				Job:     "job",
				BuildID: "number",
			},
			expected: "logs/job/number",
		},
		{
			name: "batch",
			spec: &downwardapi.JobSpec{Type: kube.BatchJob,
				Job:     "job",
				BuildID: "number",
			},
			expected: "pr-logs/pull/batch/job/number",
		},
		{
			name: "presubmit full default legacy",
			spec: &downwardapi.JobSpec{
				Type:    kube.PresubmitJob,
				Job:     "job",
				BuildID: "number",
				Refs: kube.Refs{
					Org:  "org",
					Repo: "repo",
					Pulls: []kube.Pull{
						{
							Number: 1,
						},
					},
				},
			},
			builder:  NewLegacyRepoPathBuilder("org", "repo"),
			expected: "pr-logs/pull/1/job/number",
		},
		{
			name: "presubmit default org legacy",
			spec: &downwardapi.JobSpec{
				Type:    kube.PresubmitJob,
				Job:     "job",
				BuildID: "number",
				Refs: kube.Refs{
					Org:  "org",
					Repo: "repo",
					Pulls: []kube.Pull{
						{
							Number: 1,
						},
					},
				},
			},
			builder:  NewLegacyRepoPathBuilder("org", "other"),
			expected: "pr-logs/pull/repo/1/job/number",
		},
		{
			name: "presubmit nondefault legacy",
			spec: &downwardapi.JobSpec{
				Type:    kube.PresubmitJob,
				Job:     "job",
				BuildID: "number",
				Refs: kube.Refs{
					Org:  "org",
					Repo: "repo",
					Pulls: []kube.Pull{
						{
							Number: 1,
						},
					},
				},
			},
			builder:  NewLegacyRepoPathBuilder("some", "other"),
			expected: "pr-logs/pull/org_repo/1/job/number",
		},
	}

	for _, test := range testCases {
		if expected, actual := test.expected, PathForSpec(test.spec, test.builder); expected != actual {
			t.Errorf("%s: expected path %q but got %q", test.name, expected, actual)
		}
	}
}

func TestAliasForSpec(t *testing.T) {
	testCases := []struct {
		name     string
		spec     *downwardapi.JobSpec
		expected string
	}{
		{
			name:     "periodic",
			spec:     &downwardapi.JobSpec{Type: kube.PeriodicJob},
			expected: "",
		},
		{
			name:     "batch",
			spec:     &downwardapi.JobSpec{Type: kube.BatchJob},
			expected: "",
		},
		{
			name:     "postsubmit",
			spec:     &downwardapi.JobSpec{Type: kube.PostsubmitJob},
			expected: "",
		},
		{
			name: "presubmit",
			spec: &downwardapi.JobSpec{
				Type:    kube.PresubmitJob,
				Job:     "job",
				BuildID: "number",
			},
			expected: "pr-logs/directory/job/number.txt",
		},
	}

	for _, test := range testCases {
		if expected, actual := test.expected, AliasForSpec(test.spec); expected != actual {
			t.Errorf("%s: expected alias %q but got %q", test.name, expected, actual)
		}
	}
}

func TestLatestBuildForSpec(t *testing.T) {
	testCases := []struct {
		name     string
		spec     *downwardapi.JobSpec
		builder  RepoPathBuilder
		expected []string
	}{
		{
			name: "presubmit - no strategy",
			spec: &downwardapi.JobSpec{
				Type: kube.PresubmitJob,
				Job:  "pull-kubernetes-unit",
				Refs: kube.Refs{Org: "kubernetes", Repo: "test-infra", Pulls: []kube.Pull{{Number: 1234}}},
			},
			expected: []string{"pr-logs/directory/pull-kubernetes-unit/latest-build.txt"},
		},
		{
			name: "presubmit - explicit strategy",
			spec: &downwardapi.JobSpec{
				Type: kube.PresubmitJob,
				Job:  "pull-kubernetes-unit",
				Refs: kube.Refs{Org: "kubernetes", Repo: "test-infra", Pulls: []kube.Pull{{Number: 1234}}},
			},
			builder: NewExplicitRepoPathBuilder(),
			expected: []string{
				"pr-logs/directory/pull-kubernetes-unit/latest-build.txt",
				"pr-logs/pull/kubernetes_test-infra/1234/pull-kubernetes-unit/latest-build.txt",
			},
		},
		{
			name: "presubmit - legacy strategy",
			spec: &downwardapi.JobSpec{
				Type: kube.PresubmitJob,
				Job:  "pull-kubernetes-unit",
				Refs: kube.Refs{Org: "kubernetes", Repo: "test-infra", Pulls: []kube.Pull{{Number: 1234}}},
			},
			builder: NewLegacyRepoPathBuilder("kubernetes", "test-infra"),
			expected: []string{
				"pr-logs/directory/pull-kubernetes-unit/latest-build.txt",
				"pr-logs/pull/1234/pull-kubernetes-unit/latest-build.txt",
			},
		},
		{
			name: "presubmit - single strategy",
			spec: &downwardapi.JobSpec{
				Type: kube.PresubmitJob,
				Job:  "pull-kubernetes-unit",
				Refs: kube.Refs{Org: "kubernetes", Repo: "test-infra", Pulls: []kube.Pull{{Number: 1234}}},
			},
			builder: NewSingleDefaultRepoPathBuilder("defaultorg", "defaultrepo"),
			expected: []string{
				"pr-logs/directory/pull-kubernetes-unit/latest-build.txt",
				"pr-logs/pull/kubernetes_test-infra/1234/pull-kubernetes-unit/latest-build.txt",
			},
		},
		{
			name:     "batch",
			spec:     &downwardapi.JobSpec{Type: kube.BatchJob, Job: "pull-kubernetes-unit"},
			expected: []string{"pr-logs/directory/pull-kubernetes-unit/latest-build.txt"},
		},
		{
			name:     "postsubmit",
			spec:     &downwardapi.JobSpec{Type: kube.PostsubmitJob, Job: "ci-kubernetes-unit"},
			expected: []string{"logs/ci-kubernetes-unit/latest-build.txt"},
		},
		{
			name:     "periodic",
			spec:     &downwardapi.JobSpec{Type: kube.PeriodicJob, Job: "ci-kubernetes-periodic"},
			expected: []string{"logs/ci-kubernetes-periodic/latest-build.txt"},
		},
	}

	for _, test := range testCases {
		actual := LatestBuildForSpec(test.spec, test.builder)
		if !equality.Semantic.DeepEqual(actual, test.expected) {
			t.Errorf("%s: expected path %q but got %q", test.name, test.expected, actual)
		}
	}
}

func TestRootForSpec(t *testing.T) {
	testCases := []struct {
		name     string
		spec     *downwardapi.JobSpec
		expected string
	}{
		{
			name:     "presubmit",
			spec:     &downwardapi.JobSpec{Type: kube.PresubmitJob, Job: "pull-kubernetes-unit"},
			expected: "pr-logs/directory/pull-kubernetes-unit",
		},
		{
			name:     "batch",
			spec:     &downwardapi.JobSpec{Type: kube.BatchJob, Job: "pull-kubernetes-unit"},
			expected: "pr-logs/directory/pull-kubernetes-unit",
		},
		{
			name:     "postsubmit",
			spec:     &downwardapi.JobSpec{Type: kube.PostsubmitJob, Job: "ci-kubernetes-unit"},
			expected: "logs/ci-kubernetes-unit",
		},
		{
			name:     "periodic",
			spec:     &downwardapi.JobSpec{Type: kube.PeriodicJob, Job: "ci-kubernetes-periodic"},
			expected: "logs/ci-kubernetes-periodic",
		},
	}

	for _, test := range testCases {
		if expected, actual := test.expected, RootForSpec(test.spec); expected != actual {
			t.Errorf("%s: expected path %q but got %q", test.name, expected, actual)
		}
	}
}

func TestNewLegacyRepoPathBuilder(t *testing.T) {
	testCases := []struct {
		name        string
		defaultOrg  string
		defaultRepo string
		org         string
		repo        string
		expected    string
	}{
		{
			name:        "default org and repo",
			defaultOrg:  "org",
			defaultRepo: "repo",
			org:         "org",
			repo:        "repo",
			expected:    "",
		},
		{
			name:        "default repo",
			defaultOrg:  "org",
			defaultRepo: "repo",
			org:         "other",
			repo:        "repo",
			expected:    "other_repo",
		},
		{
			name:        "default org",
			defaultOrg:  "org",
			defaultRepo: "repo",
			org:         "org",
			repo:        "other",
			expected:    "other",
		},
		{
			name:        "non-default",
			defaultOrg:  "org",
			defaultRepo: "repo",
			org:         "other",
			repo:        "wild",
			expected:    "other_wild",
		},
		{
			name:        "gerrit",
			defaultOrg:  "org",
			defaultRepo: "repo",
			org:         "gerrit",
			repo:        "foo/bar",
			expected:    "gerrit_foo_bar",
		},
	}

	for _, test := range testCases {
		builder := NewLegacyRepoPathBuilder(test.defaultOrg, test.defaultRepo)
		if expected, actual := test.expected, builder(test.org, test.repo); expected != actual {
			t.Errorf("%s: expected legacy repo path builder to create path segment %q but got %q", test.name, expected, actual)
		}
	}
}

func TestNewSingleDefaultRepoPathBuilder(t *testing.T) {
	testCases := []struct {
		name        string
		defaultOrg  string
		defaultRepo string
		org         string
		repo        string
		expected    string
	}{
		{
			name:        "default org and repo",
			defaultOrg:  "org",
			defaultRepo: "repo",
			org:         "org",
			repo:        "repo",
			expected:    "",
		},
		{
			name:        "default repo",
			defaultOrg:  "org",
			defaultRepo: "repo",
			org:         "other",
			repo:        "repo",
			expected:    "other_repo",
		},
		{
			name:        "default org",
			defaultOrg:  "org",
			defaultRepo: "repo",
			org:         "org",
			repo:        "other",
			expected:    "org_other",
		},
		{
			name:        "non-default",
			defaultOrg:  "org",
			defaultRepo: "repo",
			org:         "other",
			repo:        "wild",
			expected:    "other_wild",
		},
		{
			name:        "gerrit",
			defaultOrg:  "org",
			defaultRepo: "repo",
			org:         "gerrit",
			repo:        "foo/bar",
			expected:    "gerrit_foo_bar",
		},
	}

	for _, test := range testCases {
		builder := NewSingleDefaultRepoPathBuilder(test.defaultOrg, test.defaultRepo)
		if expected, actual := test.expected, builder(test.org, test.repo); expected != actual {
			t.Errorf("%s: expected single default repo path builder to create path segment %q but got %q", test.name, expected, actual)
		}
	}
}

func TestNewExplicitRepoPathBuilder(t *testing.T) {
	testCases := []struct {
		name     string
		org      string
		repo     string
		expected string
	}{
		{
			name:     "default org and repo",
			org:      "org",
			repo:     "repo",
			expected: "org_repo",
		},
		{
			name:     "gerrit",
			org:      "gerrit",
			repo:     "foo/bar",
			expected: "gerrit_foo_bar",
		},
	}

	for _, tc := range testCases {
		if expected, actual := tc.expected, NewExplicitRepoPathBuilder()(tc.org, tc.repo); expected != actual {
			t.Errorf("tc %s: expected explicit repo path builder to create path segment %q but got %q", tc.name, expected, actual)
		}
	}
}
