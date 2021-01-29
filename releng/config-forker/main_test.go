/*
Copyright 2019 The Kubernetes Authors.

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

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/diff"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
)

func TestCleanAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    map[string]string
	}{
		{
			name:        "no annotations",
			annotations: map[string]string{},
			expected:    map[string]string{},
		},
		{
			name: "only removable annotations",
			annotations: map[string]string{
				forkAnnotation:        "true",
				replacementAnnotation: "foo -> bar",
			},
			expected: map[string]string{},
		},
		{
			name: "all our annotations",
			annotations: map[string]string{
				forkAnnotation:             "true",
				replacementAnnotation:      "foo -> bar",
				periodicIntervalAnnotation: "2h",
			},
			expected: map[string]string{
				periodicIntervalAnnotation: "2h",
			},
		},
		{
			name: "foreign annotations",
			annotations: map[string]string{
				"someOtherAnnotation": "pony party",
			},
			expected: map[string]string{
				"someOtherAnnotation": "pony party",
			},
		},
		{
			name: "blank periodic annotations",
			annotations: map[string]string{
				periodicIntervalAnnotation: "",
				cronAnnotation:             "",
			},
			expected: map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oldAnnotations := map[string]string{}
			for k, v := range tc.annotations {
				oldAnnotations[k] = v
			}
			result := cleanAnnotations(tc.annotations)
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Expected result %#v', got %#v", tc.expected, result)
			}
			if !reflect.DeepEqual(oldAnnotations, tc.annotations) {
				t.Errorf("Input annotations map changed: used to be %#v, now %#v", oldAnnotations, tc.annotations)
			}
		})
	}
}

func TestEvaluateTemplate(t *testing.T) {
	const expected = "Foo Substitution! Baz"
	result, err := evaluateTemplate("Foo {{.Bar}} Baz", map[string]string{"Bar": "Substitution!"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != expected {
		t.Errorf("Expected result %q, got %q", expected, result)
	}
}

func TestPerformArgReplacements(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		replacements string
		expected     []string
		expectErr    bool
	}{
		{
			name:         "nil arguments remain nil",
			args:         nil,
			replacements: "foo -> bar",
			expected:     nil,
		},
		{
			name:         "empty arguments remain empty",
			args:         []string{},
			replacements: "foo -> bar",
			expected:     []string{},
		},
		{
			name:         "empty replacements do nothing",
			args:         []string{"foo", "bar"},
			replacements: "",
			expected:     []string{"foo", "bar"},
		},
		{
			name:         "simple replacement works",
			args:         []string{"foos", "bars", "bazzes"},
			replacements: "foo -> bar",
			expected:     []string{"bars", "bars", "bazzes"},
		},
		{
			name:         "multiple replacements work",
			args:         []string{"some foos", "bars", "bazzes"},
			replacements: "foo -> bar, bars -> bazzes",
			expected:     []string{"some bars", "bazzes", "bazzes"},
		},
		{
			name:         "template expansion works",
			args:         []string{"version: special one"},
			replacements: "special one -> {{.Version}}",
			expected:     []string{"version: 1.15"},
		},
		{
			name:         "invalid template is an error",
			args:         []string{"version: special one"},
			replacements: "special one -> {{Version}}",
			expectErr:    true,
		},
		{
			name:         "unparseable replacements are an error",
			args:         []string{"foo"},
			replacements: "foo -> bar, baz",
			expectErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := performReplacement(tc.args, "1.15", tc.replacements)
			if err != nil {
				if !tc.expectErr {
					t.Fatalf("Unexpected error: %v", err)
				}
				return
			}
			if tc.expectErr {
				t.Fatalf("Expected an error, but got %v", result)
			}
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Expected result %v, but got %v instead", tc.expected, result)
			}
		})
	}
}

func TestPerformArgDeletions(t *testing.T) {
	tests := []struct {
		name      string
		args      map[string]string
		deletions string
		expected  map[string]string
	}{
		{
			name:      "nil arguments remain nil",
			args:      nil,
			deletions: "",
			expected:  nil,
		},
		{
			name:      "empty arguments remain empty",
			args:      map[string]string{},
			deletions: "",
			expected:  map[string]string{},
		},
		{
			name:      "empty deletions do nothing",
			args:      map[string]string{"foo": "bar"},
			deletions: "",
			expected:  map[string]string{"foo": "bar"},
		},
		{
			name:      "simple deletion works",
			args:      map[string]string{"foo": "bar", "baz": "baz2"},
			deletions: "foo",
			expected:  map[string]string{"baz": "baz2"},
		},
		{
			name:      "multiple deletions work",
			args:      map[string]string{"foo": "bar", "baz": "baz2", "hello": "world"},
			deletions: "foo, baz",
			expected:  map[string]string{"hello": "world"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := performDeletion(tc.args, tc.deletions)
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Expected result %v, but got %v instead", tc.expected, result)
			}
		})
	}
}

func TestFixImage(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		expected string
	}{
		{
			name:     "replaces -master with -[version]",
			image:    "kubekins-e2e-blahblahblah-master",
			expected: "kubekins-e2e-blahblahblah-1.15",
		},
		{
			name:     "does nothing with non-master images",
			image:    "golang:1.12.2",
			expected: "golang:1.12.2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if result := fixImage(tc.image, "1.15"); result != tc.expected {
				t.Errorf("Expected %q, but got %q", tc.expected, result)
			}
		})
	}
}

func TestFixBootstrapArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name:     "replaces repo with no branch specified",
			args:     []string{"--repo=k8s.io/kubernetes"},
			expected: []string{"--repo=k8s.io/kubernetes=release-1.15"},
		},
		{
			name:     "replaces repo with master branch specified",
			args:     []string{"--repo=k8s.io/kubernetes=master"},
			expected: []string{"--repo=k8s.io/kubernetes=release-1.15"},
		},
		{
			name:     "replaces master branch with release branch",
			args:     []string{"--branch=master"},
			expected: []string{"--branch=release-1.15"},
		},
		{
			name:     "replaces both repo and branch",
			args:     []string{"--repo=k8s.io/kubernetes=master", "--branch=master"},
			expected: []string{"--repo=k8s.io/kubernetes=release-1.15", "--branch=release-1.15"},
		},
		{
			name:     "doesn't replace other repos",
			args:     []string{"--repo=k8s.io/test-infra"},
			expected: []string{"--repo=k8s.io/test-infra"},
		},
		{
			name:     "doesn't touch other flags",
			args:     []string{"--foo=bar", "--branch=master"},
			expected: []string{"--foo=bar", "--branch=release-1.15"},
		},
		{
			name:     "nil args are still nil",
			args:     nil,
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if result := fixBootstrapArgs(tc.args, "1.15"); !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Expected args %v, but got %v instead", tc.expected, result)
			}
		})
	}
}

func TestFixExtraRefs(t *testing.T) {
	tests := []struct {
		name     string
		refs     []prowapi.Refs
		expected []prowapi.Refs
	}{
		{
			name:     "replaces kubernetes master with release branch",
			refs:     []prowapi.Refs{{Org: "kubernetes", Repo: "kubernetes", BaseRef: "master"}},
			expected: []prowapi.Refs{{Org: "kubernetes", Repo: "kubernetes", BaseRef: "release-1.15"}},
		},
		{
			name:     "ignores kubernetes repos other than kubernetes",
			refs:     []prowapi.Refs{{Org: "kubernetes", Repo: "test-infra", BaseRef: "master"}},
			expected: []prowapi.Refs{{Org: "kubernetes", Repo: "test-infra", BaseRef: "master"}},
		},
		{
			name:     "ignores repos called kubernetes in other orgs",
			refs:     []prowapi.Refs{{Org: "Katharine", Repo: "kubernetes", BaseRef: "master"}},
			expected: []prowapi.Refs{{Org: "Katharine", Repo: "kubernetes", BaseRef: "master"}},
		},
		{
			name:     "ignores non-master branches",
			refs:     []prowapi.Refs{{Org: "kubernetes", Repo: "kubernetes", BaseRef: "other-branch"}},
			expected: []prowapi.Refs{{Org: "kubernetes", Repo: "kubernetes", BaseRef: "other-branch"}},
		},
		{
			name:     "handles multiple refs",
			refs:     []prowapi.Refs{{Org: "kubernetes", Repo: "test-infra", BaseRef: "master"}, {Org: "kubernetes", Repo: "kubernetes", BaseRef: "master"}},
			expected: []prowapi.Refs{{Org: "kubernetes", Repo: "test-infra", BaseRef: "master"}, {Org: "kubernetes", Repo: "kubernetes", BaseRef: "release-1.15"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if result := fixExtraRefs(tc.refs, "1.15"); !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Result does not match expected. Difference:\n%s", diff.ObjectDiff(tc.expected, result))
			}
		})
	}
}

func TestFixEnvVars(t *testing.T) {
	tests := []struct {
		name     string
		vars     []v1.EnvVar
		expected []v1.EnvVar
	}{
		{
			name:     "replaces BRANCH-like jobs referencing master",
			vars:     []v1.EnvVar{{Name: "KUBERNETES_BRANCH", Value: "master"}},
			expected: []v1.EnvVar{{Name: "KUBERNETES_BRANCH", Value: "release-1.15"}},
		},
		{
			name:     "ignores BRANCH-like jobs referencing something else",
			vars:     []v1.EnvVar{{Name: "KUBERNETES_BRANCH", Value: "something-else"}},
			expected: []v1.EnvVar{{Name: "KUBERNETES_BRANCH", Value: "something-else"}},
		},
		{
			name:     "names are case-insensitive",
			vars:     []v1.EnvVar{{Name: "branch", Value: "master"}},
			expected: []v1.EnvVar{{Name: "branch", Value: "release-1.15"}},
		},
		{
			name:     "other vars are not touched",
			vars:     []v1.EnvVar{{Name: "foo", Value: "bar"}, {Name: "baz", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{Key: "baz-secret"}}}},
			expected: []v1.EnvVar{{Name: "foo", Value: "bar"}, {Name: "baz", ValueFrom: &v1.EnvVarSource{SecretKeyRef: &v1.SecretKeySelector{Key: "baz-secret"}}}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if result := fixEnvVars(tc.vars, "1.15"); !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Result does not match expected. Difference:\n%s", diff.ObjectDiff(tc.expected, result))
			}
		})
	}
}

func TestFixTestgridAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    map[string]string
		isPresubmit bool
	}{
		{
			name:        "remove presubmit additions to dashboards",
			annotations: map[string]string{testgridDashboardsAnnotation: "sig-release-master-blocking, google-unit"},
			expected:    map[string]string{},
			isPresubmit: true,
		},
		{
			name:        "periodic updates master-blocking to point at 1.15-blocking",
			annotations: map[string]string{testgridDashboardsAnnotation: "sig-release-master-blocking"},
			expected:    map[string]string{testgridDashboardsAnnotation: "sig-release-1.15-blocking"},
			isPresubmit: false,
		},
		{
			name:        "periodic updates with no dashboard annotation points to job-config-errors",
			annotations: map[string]string{},
			expected:    map[string]string{testgridDashboardsAnnotation: "sig-release-job-config-errors"},
			isPresubmit: false,
		},
		{
			name:        "drop 'description'",
			annotations: map[string]string{descriptionAnnotation: "some description"},
			expected:    map[string]string{},
			isPresubmit: true,
		},
		{
			name:        "update tab names",
			annotations: map[string]string{testgridTabNameAnnotation: "foo master"},
			expected:    map[string]string{testgridDashboardsAnnotation: "sig-release-job-config-errors", testgridTabNameAnnotation: "foo 1.15"},
			isPresubmit: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := fixTestgridAnnotations(tc.annotations, "1.15", tc.isPresubmit)
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("Result does not match expected. Difference:\n%s", diff.ObjectDiff(tc.expected, result))
			}
		})
	}
}

func TestGenerateNameVariant(t *testing.T) {
	tests := []struct {
		name          string
		testName      string
		genericSuffix bool
		expected      string
	}{
		{
			name:     "jobs ending in -master have it replaced with a version",
			testName: "ci-party-master",
			expected: "ci-party-1-15",
		},
		{
			name:          "generic jobs ending in -master have it replaced with -beta",
			testName:      "ci-party-master",
			genericSuffix: true,
			expected:      "ci-party-beta",
		},
		{
			name:     "jobs not ending in in -master have a number appended",
			testName: "ci-party",
			expected: "ci-party-1-15",
		},
		{
			name:          "generic jobs not ending in in -master have 'beta' appended",
			testName:      "ci-party",
			genericSuffix: true,
			expected:      "ci-party-beta",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if result := generateNameVariant(tc.testName, "1.15", tc.genericSuffix); result != tc.expected {
				t.Errorf("Expected name %q, but got name %q.", tc.expected, result)
			}
		})
	}
}

func TestGeneratePresubmits(t *testing.T) {
	presubmits := map[string][]config.Presubmit{
		"kubernetes/kubernetes": {
			{
				JobBase: config.JobBase{
					Name:        "pull-kubernetes-e2e",
					Annotations: map[string]string{forkAnnotation: "true"},
				},
				Brancher: config.Brancher{
					SkipBranches: []string{`release-\d\.\d`},
				},
			},
			{
				JobBase: config.JobBase{
					Name: "pull-replace-some-things",
					Annotations: map[string]string{
						forkAnnotation:                 "true",
						replacementAnnotation:          "foo -> {{.Version}}",
						"testgrid-generate-test-group": "true",
						"some-annotation":              "yup",
					},
					Spec: &v1.PodSpec{
						Containers: []v1.Container{
							{
								Image: "gcr.io/k8s-testimages/kubekins-e2e:blahblahblah-master",
								Args:  []string{"--repo=k8s.io/kubernetes", "--something=foo"},
								Env:   []v1.EnvVar{{Name: "BRANCH", Value: "master"}},
							},
						},
					},
				},
				Brancher: config.Brancher{
					SkipBranches: []string{`release-\d\.\d`},
				},
			},
			{
				JobBase: config.JobBase{
					Name:        "pull-not-forked",
					Annotations: map[string]string{"foo": "bar"},
				},
			},
		},
	}

	expected := map[string][]config.Presubmit{
		"kubernetes/kubernetes": {
			{
				JobBase: config.JobBase{
					Name:        "pull-kubernetes-e2e",
					Annotations: map[string]string{},
				},
				Brancher: config.Brancher{
					Branches: []string{"release-1.15"},
				},
			},
			{
				JobBase: config.JobBase{
					Name: "pull-replace-some-things",
					Annotations: map[string]string{
						"some-annotation": "yup",
					},
					Spec: &v1.PodSpec{
						Containers: []v1.Container{
							{
								Image: "gcr.io/k8s-testimages/kubekins-e2e:blahblahblah-1.15",
								Args:  []string{"--repo=k8s.io/kubernetes", "--something=1.15"},
								Env:   []v1.EnvVar{{Name: "BRANCH", Value: "release-1.15"}},
							},
						},
					},
				},
				Brancher: config.Brancher{
					Branches: []string{"release-1.15"},
				},
			},
		},
	}

	result, err := generatePresubmits(config.JobConfig{PresubmitsStatic: presubmits}, "1.15")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Result does not match expected. Difference:\n%s", diff.ObjectDiff(expected, result))
	}
}

func TestGeneratePeriodics(t *testing.T) {
	yes := true
	periodics := []config.Periodic{
		{
			Cron: "0 * * * *",
			JobBase: config.JobBase{
				Name:        "some-forked-periodic-master",
				Annotations: map[string]string{forkAnnotation: "true"},
			},
		},
		{
			Cron: "0 * * * *",
			JobBase: config.JobBase{
				Name:        "some-generic-periodic-master",
				Annotations: map[string]string{forkAnnotation: "true", suffixAnnotation: "true"},
			},
		},
		{
			Cron: "0 * * * *",
			JobBase: config.JobBase{
				Name: "generic-periodic-with-deletions-master",
				Annotations: map[string]string{
					forkAnnotation:     "true",
					deletionAnnotation: "preset-e2e-scalability-periodics-master",
					suffixAnnotation:   "true",
				},
				Labels: map[string]string{
					"preset-e2e-scalability-periodics":        "true",
					"preset-e2e-scalability-periodics-master": "true",
				},
			},
		},
		{
			Interval: "1h",
			JobBase: config.JobBase{
				Name: "periodic-with-replacements",
				Annotations: map[string]string{
					forkAnnotation:             "true",
					periodicIntervalAnnotation: "6h 12h 24h 24h",
					replacementAnnotation:      "stable -> {{.Version}}",
				},
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Image: "gcr.io/k8s-testinfra/kubekins-e2e:blahblahblah-master",
							Args:  []string{"--repo=k8s.io/kubernetes", "--version=stable"},
							Env:   []v1.EnvVar{{Name: "BRANCH", Value: "master"}},
						},
					},
				},
			},
			Tags: []string{"ver: stable"},
		},
		{
			Interval: "2h",
			JobBase: config.JobBase{
				Name: "decorated-periodic",
				Annotations: map[string]string{
					forkAnnotation: "true",
				},
				UtilityConfig: config.UtilityConfig{
					Decorate:  &yes,
					ExtraRefs: []prowapi.Refs{{Org: "kubernetes", Repo: "kubernetes", BaseRef: "master"}},
				},
			},
		},
		{
			JobBase: config.JobBase{
				Name: "not-forked",
			},
		},
	}

	expected := []config.Periodic{
		{
			Cron: "0 * * * *",
			JobBase: config.JobBase{
				Name:        "some-forked-periodic-1-15",
				Annotations: map[string]string{testgridDashboardsAnnotation: "sig-release-job-config-errors"},
			},
		},
		{
			Cron: "0 * * * *",
			JobBase: config.JobBase{
				Name:        "some-generic-periodic-beta",
				Annotations: map[string]string{suffixAnnotation: "true", testgridDashboardsAnnotation: "sig-release-job-config-errors"},
			},
		},
		{
			Cron: "0 * * * *",
			JobBase: config.JobBase{
				Name:        "generic-periodic-with-deletions-beta",
				Annotations: map[string]string{suffixAnnotation: "true", testgridDashboardsAnnotation: "sig-release-job-config-errors"},
				Labels: map[string]string{
					"preset-e2e-scalability-periodics": "true",
				},
			},
		},
		{
			Interval: "6h",
			JobBase: config.JobBase{
				Name: "periodic-with-replacements-1-15",
				Annotations: map[string]string{
					periodicIntervalAnnotation:   "12h 24h 24h",
					testgridDashboardsAnnotation: "sig-release-job-config-errors",
				},
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Image: "gcr.io/k8s-testinfra/kubekins-e2e:blahblahblah-1.15",
							Args:  []string{"--repo=k8s.io/kubernetes=release-1.15", "--version=1.15"},
							Env:   []v1.EnvVar{{Name: "BRANCH", Value: "release-1.15"}},
						},
					},
				},
			},
			Tags: []string{"ver: 1.15"},
		},
		{
			Interval: "2h",
			JobBase: config.JobBase{
				Name:        "decorated-periodic-1-15",
				Annotations: map[string]string{testgridDashboardsAnnotation: "sig-release-job-config-errors"},
				UtilityConfig: config.UtilityConfig{
					Decorate:  &yes,
					ExtraRefs: []prowapi.Refs{{Org: "kubernetes", Repo: "kubernetes", BaseRef: "release-1.15"}},
				},
			},
		},
	}

	result, err := generatePeriodics(config.JobConfig{Periodics: periodics}, "1.15")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Result does not match expected. Difference:\n%s\n", cmp.Diff(expected, result, cmpopts.IgnoreUnexported(config.Periodic{})))
	}
}

func TestGeneratePostsubmits(t *testing.T) {
	postsubmits := map[string][]config.Postsubmit{
		"kubernetes/kubernetes": {
			{
				JobBase: config.JobBase{
					Name:        "post-kubernetes-e2e",
					Annotations: map[string]string{forkAnnotation: "true"},
				},
				Brancher: config.Brancher{
					SkipBranches: []string{`release-\d\.\d`},
				},
			},
			{
				JobBase: config.JobBase{
					Name: "post-kubernetes-generic",
					Annotations: map[string]string{
						forkAnnotation:               "true",
						suffixAnnotation:             "true",
						testgridDashboardsAnnotation: "sig-release-master-blocking, google-unit",
					},
				},
				Brancher: config.Brancher{
					SkipBranches: []string{`release-\d\.\d`},
				},
			},
			{
				JobBase: config.JobBase{
					Name: "post-kubernetes-generic-will-end-up-in-all",
					Annotations: map[string]string{
						forkAnnotation:               "true",
						suffixAnnotation:             "true",
						testgridDashboardsAnnotation: "google-unit",
					},
				},
				Brancher: config.Brancher{
					SkipBranches: []string{`release-\d\.\d`},
				},
			},
			{
				JobBase: config.JobBase{
					Name: "post-replace-some-things-master",
					Annotations: map[string]string{
						forkAnnotation:        "true",
						replacementAnnotation: "foo -> {{.Version}}",
						"some-annotation":     "yup",
					},
					Spec: &v1.PodSpec{
						Containers: []v1.Container{
							{
								Image: "gcr.io/k8s-testimages/kubekins-e2e:blahblahblah-master",
								Args:  []string{"--repo=k8s.io/kubernetes", "--something=foo"},
								Env:   []v1.EnvVar{{Name: "BRANCH", Value: "master"}},
							},
						},
					},
				},
				Brancher: config.Brancher{
					SkipBranches: []string{`release-\d\.\d`},
				},
			},
			{
				JobBase: config.JobBase{
					Name:        "post-not-forked",
					Annotations: map[string]string{"foo": "bar"},
				},
			},
		},
	}

	expected := map[string][]config.Postsubmit{
		"kubernetes/kubernetes": {
			{
				JobBase: config.JobBase{
					Name:        "post-kubernetes-e2e-1-15",
					Annotations: map[string]string{testgridDashboardsAnnotation: "sig-release-job-config-errors"},
				},
				Brancher: config.Brancher{
					Branches: []string{"release-1.15"},
				},
			},
			{
				JobBase: config.JobBase{
					Name: "post-kubernetes-generic-beta",
					Annotations: map[string]string{
						suffixAnnotation:             "true",
						testgridDashboardsAnnotation: "sig-release-1.15-blocking, google-unit",
					},
				},
				Brancher: config.Brancher{
					Branches: []string{"release-1.15"},
				},
			},
			{
				JobBase: config.JobBase{
					Name: "post-kubernetes-generic-will-end-up-in-all-beta",
					Annotations: map[string]string{
						suffixAnnotation:             "true",
						testgridDashboardsAnnotation: "google-unit, sig-release-job-config-errors",
					},
				},
				Brancher: config.Brancher{
					Branches: []string{"release-1.15"},
				},
			},
			{
				JobBase: config.JobBase{
					Name: "post-replace-some-things-1-15",
					Annotations: map[string]string{
						"some-annotation":            "yup",
						testgridDashboardsAnnotation: "sig-release-job-config-errors",
					},
					Spec: &v1.PodSpec{
						Containers: []v1.Container{
							{
								Image: "gcr.io/k8s-testimages/kubekins-e2e:blahblahblah-1.15",
								Args:  []string{"--repo=k8s.io/kubernetes", "--something=1.15"},
								Env:   []v1.EnvVar{{Name: "BRANCH", Value: "release-1.15"}},
							},
						},
					},
				},
				Brancher: config.Brancher{
					Branches: []string{"release-1.15"},
				},
			},
		},
	}

	result, err := generatePostsubmits(config.JobConfig{PostsubmitsStatic: postsubmits}, "1.15")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Result does not match expected. Difference:\n%s", diff.ObjectDiff(expected, result))
	}
}
