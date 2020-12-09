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

package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"text/template"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	pipelinev1alpha1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	v1 "k8s.io/api/core/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	utilpointer "k8s.io/utils/pointer"

	"k8s.io/test-infra/pkg/genyaml"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config/secret"
	gerrit "k8s.io/test-infra/prow/gerrit/client"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pod-utils/decorate"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
)

func TestDefaultJobBase(t *testing.T) {
	bar := "bar"
	filled := JobBase{
		Agent:     "foo",
		Namespace: &bar,
		Cluster:   "build",
	}
	cases := []struct {
		name     string
		config   ProwConfig
		base     func(j *JobBase)
		expected func(j *JobBase)
	}{
		{
			name: "no changes when fields are already set",
		},
		{
			name: "empty agent results in kubernetes",
			base: func(j *JobBase) {
				j.Agent = ""
			},
			expected: func(j *JobBase) {
				j.Agent = string(prowapi.KubernetesAgent)
			},
		},
		{
			name: "nil namespace becomes PodNamespace",
			config: ProwConfig{
				PodNamespace:     "pod-namespace",
				ProwJobNamespace: "wrong",
			},
			base: func(j *JobBase) {
				j.Namespace = nil
			},
			expected: func(j *JobBase) {
				p := "pod-namespace"
				j.Namespace = &p
			},
		},
		{
			name: "empty namespace becomes PodNamespace",
			config: ProwConfig{
				PodNamespace:     "new-pod-namespace",
				ProwJobNamespace: "still-wrong",
			},
			base: func(j *JobBase) {
				var empty string
				j.Namespace = &empty
			},
			expected: func(j *JobBase) {
				p := "new-pod-namespace"
				j.Namespace = &p
			},
		},
		{
			name: "empty cluster becomes DefaultClusterAlias",
			base: func(j *JobBase) {
				j.Cluster = ""
			},
			expected: func(j *JobBase) {
				j.Cluster = kube.DefaultClusterAlias
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := filled
			if tc.base != nil {
				tc.base(&actual)
			}
			expected := actual
			if tc.expected != nil {
				tc.expected(&expected)
			}
			tc.config.defaultJobBase(&actual)
			if !reflect.DeepEqual(actual, expected) {
				t.Errorf("expected %#v\n!=\nactual %#v", expected, actual)
			}
		})
	}
}

func TestSpyglassConfig(t *testing.T) {
	testCases := []struct {
		name                 string
		spyglassConfig       string
		expectedViewers      map[string][]string
		expectedRegexMatches map[string][]string
		expectedSizeLimit    int64
		expectError          bool
	}{
		{
			name: "Default: build log, metadata, junit",
			spyglassConfig: `
deck:
  spyglass:
    size_limit: 500e+6
    viewers:
      "started.json|finished.json":
      - "metadata"
      "build-log.txt":
      - "buildlog"
      "artifacts/junit.*\\.xml":
      - "junit"
`,
			expectedViewers: map[string][]string{
				"started.json|finished.json": {"metadata"},
				"build-log.txt":              {"buildlog"},
				"artifacts/junit.*\\.xml":    {"junit"},
			},
			expectedRegexMatches: map[string][]string{
				"started.json|finished.json": {"started.json", "finished.json"},
				"build-log.txt":              {"build-log.txt"},
				"artifacts/junit.*\\.xml":    {"artifacts/junit01.xml", "artifacts/junit_runner.xml"},
			},
			expectedSizeLimit: 500e6,
			expectError:       false,
		},
		{
			name: "Backwards compatibility",
			spyglassConfig: `
deck:
  spyglass:
    size_limit: 500e+6
    viewers:
      "started.json|finished.json":
      - "metadata-viewer"
      "build-log.txt":
      - "build-log-viewer"
      "artifacts/junit.*\\.xml":
      - "junit-viewer"
`,
			expectedViewers: map[string][]string{
				"started.json|finished.json": {"metadata"},
				"build-log.txt":              {"buildlog"},
				"artifacts/junit.*\\.xml":    {"junit"},
			},
			expectedSizeLimit: 500e6,
			expectError:       false,
		},
		{
			name: "Invalid spyglass size limit",
			spyglassConfig: `
deck:
  spyglass:
    size_limit: -4
    viewers:
      "started.json|finished.json":
      - "metadata-viewer"
      "build-log.txt":
      - "build-log-viewer"
      "artifacts/junit.*\\.xml":
      - "junit-viewer"
`,
			expectError: true,
		},
		{
			name: "Invalid Spyglass regexp",
			spyglassConfig: `
deck:
  spyglass:
    size_limit: 5
    viewers:
      "started.json\|]finished.json":
      - "metadata-viewer"
`,
			expectError: true,
		},
		{
			name: "Invalid Spyglass gcs browser web prefix",
			spyglassConfig: `
deck:
  spyglass:
    gcs_browser_prefix: https://gcsweb.k8s.io/gcs/
    gcs_browser_prefixes:
      '*': https://gcsweb.k8s.io/gcs/
`,
			expectError: true,
		},
	}
	for _, tc := range testCases {
		// save the config
		spyglassConfigDir, err := ioutil.TempDir("", "spyglassConfig")
		if err != nil {
			t.Fatalf("fail to make tempdir: %v", err)
		}
		defer os.RemoveAll(spyglassConfigDir)

		spyglassConfig := filepath.Join(spyglassConfigDir, "config.yaml")
		if err := ioutil.WriteFile(spyglassConfig, []byte(tc.spyglassConfig), 0666); err != nil {
			t.Fatalf("fail to write spyglass config: %v", err)
		}

		cfg, err := Load(spyglassConfig, "")
		if (err != nil) != tc.expectError {
			t.Fatalf("tc %s: expected error: %v, got: %v, error: %v", tc.name, tc.expectError, (err != nil), err)
		}

		if err != nil {
			continue
		}
		got := cfg.Deck.Spyglass.Viewers
		for re, viewNames := range got {
			expected, ok := tc.expectedViewers[re]
			if !ok {
				t.Errorf("With re %s, got %s, was not found in expected.", re, viewNames)
				continue
			}
			if !reflect.DeepEqual(expected, viewNames) {
				t.Errorf("With re %s, got %s, expected view name %s", re, viewNames, expected)
			}

		}
		for re, viewNames := range tc.expectedViewers {
			gotNames, ok := got[re]
			if !ok {
				t.Errorf("With re %s, expected %s, was not found in got.", re, viewNames)
				continue
			}
			if !reflect.DeepEqual(gotNames, viewNames) {
				t.Errorf("With re %s, got %s, expected view name %s", re, gotNames, viewNames)
			}
		}

		for expectedRegex, matches := range tc.expectedRegexMatches {
			compiledRegex, ok := cfg.Deck.Spyglass.RegexCache[expectedRegex]
			if !ok {
				t.Errorf("tc %s, regex %s was not found in the spyglass regex cache", tc.name, expectedRegex)
				continue
			}
			for _, match := range matches {
				if !compiledRegex.MatchString(match) {
					t.Errorf("tc %s expected compiled regex %s to match %s, did not match.", tc.name, expectedRegex, match)
				}
			}

		}
		if cfg.Deck.Spyglass.SizeLimit != tc.expectedSizeLimit {
			t.Errorf("%s expected SizeLimit %d, got %d", tc.name, tc.expectedSizeLimit, cfg.Deck.Spyglass.SizeLimit)
		}
	}

}

func TestGetGCSBrowserPrefix(t *testing.T) {
	testCases := []struct {
		id       string
		config   Spyglass
		expected string
	}{
		{
			id: "only default",
			config: Spyglass{
				GCSBrowserPrefixes: map[string]string{
					"*": "https://default.com/gcs/",
				},
			},
			expected: "https://default.com/gcs/",
		},
		{
			id: "org exists",
			config: Spyglass{
				GCSBrowserPrefixes: map[string]string{
					"org": "https://org.com/gcs/",
				},
			},
			expected: "https://org.com/gcs/",
		},
		{
			id: "repo exists",
			config: Spyglass{
				GCSBrowserPrefixes: map[string]string{
					"org/repo": "https://repo.com/gcs/",
				},
			},
			expected: "https://repo.com/gcs/",
		},
	}

	for _, tc := range testCases {
		actual := tc.config.GCSBrowserPrefixes.GetGCSBrowserPrefix("org", "repo")
		if !reflect.DeepEqual(actual, tc.expected) {
			t.Fatalf("%s", cmp.Diff(tc.expected, actual))
		}
	}
}

func TestDecorationRawYaml(t *testing.T) {
	var testCases = []struct {
		name        string
		expectError bool
		rawConfig   string
		expected    *prowapi.DecorationConfig
	}{
		{
			name:        "no default",
			expectError: true,
			rawConfig: `
periodics:
- name: kubernetes-defaulted-decoration
  interval: 1h
  decorate: true
  spec:
    containers:
    - image: golang:latest
      args:
      - "test"
      - "./..."`,
		},
		{
			name: "with bad default",
			rawConfig: `
plank:
  default_decoration_configs:
    '*':
      timeout: 2h
      grace_period: 15s
      utility_images:
      # clonerefs: "clonerefs:default"
        initupload: "initupload:default"
        entrypoint: "entrypoint:default"
        sidecar: "sidecar:default"
      gcs_configuration:
        bucket: "default-bucket"
        path_strategy: "legacy"
        default_org: "kubernetes"
        default_repo: "kubernetes"
      gcs_credentials_secret: "default-service-account"

periodics:
- name: kubernetes-defaulted-decoration
  interval: 1h
  decorate: true
  spec:
    containers:
    - image: golang:latest
      args:
      - "test"
      - "./..."`,
			expectError: true,
		},
		{
			name: "repo should inherit from default config",
			rawConfig: `
plank:
  default_decoration_configs:
    '*':
      timeout: 2h
      grace_period: 15s
      utility_images:
        clonerefs: "clonerefs:default"
        initupload: "initupload:default"
        entrypoint: "entrypoint:default"
        sidecar: "sidecar:default"
      gcs_configuration:
        bucket: "default-bucket"
        path_strategy: "legacy"
        default_org: "kubernetes"
        default_repo: "kubernetes"
      gcs_credentials_secret: "default-service-account"
    'org/inherit':
      timeout: 2h
      grace_period: 15s
      utility_images: {}
      gcs_configuration:
        bucket: "default-bucket"
        path_strategy: "legacy"
        default_org: "kubernetes"
        default_repo: "kubernetes"
      gcs_credentials_secret: "default-service-account"
periodics:
- name: kubernetes-defaulted-decoration
  interval: 1h
  decorate: true
  spec:
    containers:
    - image: golang:latest
      args:
      - "test"
      - "./..."`,
		},
		{
			name: "with default and repo, use default",
			rawConfig: `
plank:
  default_decoration_configs:
    '*':
      timeout: 2h
      grace_period: 15s
      utility_images:
        clonerefs: "clonerefs:default"
        initupload: "initupload:default"
        entrypoint: "entrypoint:default"
        sidecar: "sidecar:default"
      gcs_configuration:
        bucket: "default-bucket"
        path_strategy: "legacy"
        default_org: "kubernetes"
        default_repo: "kubernetes"
      gcs_credentials_secret: "default-service-account"
    'random/repo':
      timeout: 2h
      grace_period: 15s
      utility_images:
        clonerefs: "clonerefs:random"
        initupload: "initupload:random"
        entrypoint: "entrypoint:random"
        sidecar: "sidecar:org"
      gcs_configuration:
        bucket: "ignore"
        path_strategy: "legacy"
        default_org: "random"
        default_repo: "repo"
      gcs_credentials_secret: "random-service-account"

periodics:
- name: kubernetes-defaulted-decoration
  interval: 1h
  decorate: true
  spec:
    containers:
    - image: golang:latest
      args:
      - "test"
      - "./..."`,
			expected: &prowapi.DecorationConfig{
				Timeout:     &prowapi.Duration{Duration: 2 * time.Hour},
				GracePeriod: &prowapi.Duration{Duration: 15 * time.Second},
				UtilityImages: &prowapi.UtilityImages{
					CloneRefs:  "clonerefs:default",
					InitUpload: "initupload:default",
					Entrypoint: "entrypoint:default",
					Sidecar:    "sidecar:default",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "default-bucket",
					PathStrategy: prowapi.PathStrategyLegacy,
					DefaultOrg:   "kubernetes",
					DefaultRepo:  "kubernetes",
				},
				GCSCredentialsSecret: "default-service-account",
			},
		},
		{
			name: "with default, no explicit decorate",
			rawConfig: `
plank:
  default_decoration_configs:
    '*':
      timeout: 2h
      grace_period: 15s
      utility_images:
        clonerefs: "clonerefs:default"
        initupload: "initupload:default"
        entrypoint: "entrypoint:default"
        sidecar: "sidecar:default"
      gcs_configuration:
        bucket: "default-bucket"
        path_strategy: "legacy"
        default_org: "kubernetes"
        default_repo: "kubernetes"
      gcs_credentials_secret: "default-service-account"

periodics:
- name: kubernetes-defaulted-decoration
  interval: 1h
  decorate: true
  spec:
    containers:
    - image: golang:latest
      args:
      - "test"
      - "./..."`,
			expected: &prowapi.DecorationConfig{
				Timeout:     &prowapi.Duration{Duration: 2 * time.Hour},
				GracePeriod: &prowapi.Duration{Duration: 15 * time.Second},
				UtilityImages: &prowapi.UtilityImages{
					CloneRefs:  "clonerefs:default",
					InitUpload: "initupload:default",
					Entrypoint: "entrypoint:default",
					Sidecar:    "sidecar:default",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "default-bucket",
					PathStrategy: prowapi.PathStrategyLegacy,
					DefaultOrg:   "kubernetes",
					DefaultRepo:  "kubernetes",
				},
				GCSCredentialsSecret: "default-service-account",
			},
		},
		{
			name: "with default, has explicit decorate",
			rawConfig: `
plank:
  default_decoration_configs:
    '*':
      timeout: 2h
      grace_period: 15s
      utility_images:
        clonerefs: "clonerefs:default"
        initupload: "initupload:default"
        entrypoint: "entrypoint:default"
        sidecar: "sidecar:default"
      gcs_configuration:
        bucket: "default-bucket"
        path_strategy: "legacy"
        default_org: "kubernetes"
        default_repo: "kubernetes"
      gcs_credentials_secret: "default-service-account"

periodics:
- name: kubernetes-defaulted-decoration
  interval: 1h
  decorate: true
  decoration_config:
    timeout: 1
    grace_period: 1
    utility_images:
      clonerefs: "clonerefs:explicit"
      initupload: "initupload:explicit"
      entrypoint: "entrypoint:explicit"
      sidecar: "sidecar:explicit"
    gcs_configuration:
      bucket: "explicit-bucket"
      path_strategy: "explicit"
    gcs_credentials_secret: "explicit-service-account"
  spec:
    containers:
    - image: golang:latest
      args:
      - "test"
      - "./..."`,
			expected: &prowapi.DecorationConfig{
				Timeout:     &prowapi.Duration{Duration: 1 * time.Nanosecond},
				GracePeriod: &prowapi.Duration{Duration: 1 * time.Nanosecond},
				UtilityImages: &prowapi.UtilityImages{
					CloneRefs:  "clonerefs:explicit",
					InitUpload: "initupload:explicit",
					Entrypoint: "entrypoint:explicit",
					Sidecar:    "sidecar:explicit",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "explicit-bucket",
					PathStrategy: prowapi.PathStrategyExplicit,
					DefaultOrg:   "kubernetes",
					DefaultRepo:  "kubernetes",
				},
				GCSCredentialsSecret: "explicit-service-account",
			},
		},
		{
			name: "with default, configures bucket explicitly",
			rawConfig: `
plank:
  default_decoration_configs:
    '*':
      timeout: 2h
      grace_period: 15s
      utility_images:
        clonerefs: "clonerefs:default"
        initupload: "initupload:default"
        entrypoint: "entrypoint:default"
        sidecar: "sidecar:default"
      gcs_configuration:
        bucket: "default-bucket"
        path_strategy: "legacy"
        default_org: "kubernetes"
        default_repo: "kubernetes"
        mediaTypes:
          log: text/plain
      gcs_credentials_secret: "default-service-account"

periodics:
- name: kubernetes-defaulted-decoration
  interval: 1h
  decorate: true
  decoration_config:
    gcs_configuration:
      bucket: "explicit-bucket"
    gcs_credentials_secret: "explicit-service-account"
  spec:
    containers:
    - image: golang:latest
      args:
      - "test"
      - "./..."`,
			expected: &prowapi.DecorationConfig{
				Timeout:     &prowapi.Duration{Duration: 2 * time.Hour},
				GracePeriod: &prowapi.Duration{Duration: 15 * time.Second},
				UtilityImages: &prowapi.UtilityImages{
					CloneRefs:  "clonerefs:default",
					InitUpload: "initupload:default",
					Entrypoint: "entrypoint:default",
					Sidecar:    "sidecar:default",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "explicit-bucket",
					PathStrategy: prowapi.PathStrategyLegacy,
					DefaultOrg:   "kubernetes",
					DefaultRepo:  "kubernetes",
					MediaTypes:   map[string]string{"log": "text/plain"},
				},
				GCSCredentialsSecret: "explicit-service-account",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// save the config
			prowConfigDir, err := ioutil.TempDir("", "prowConfig")
			if err != nil {
				t.Fatalf("fail to make tempdir: %v", err)
			}
			defer os.RemoveAll(prowConfigDir)

			prowConfig := filepath.Join(prowConfigDir, "config.yaml")
			if err := ioutil.WriteFile(prowConfig, []byte(tc.rawConfig), 0666); err != nil {
				t.Fatalf("fail to write prow config: %v", err)
			}

			cfg, err := Load(prowConfig, "")
			if tc.expectError && err == nil {
				t.Errorf("tc %s: Expect error, but got nil", tc.name)
			} else if !tc.expectError && err != nil {
				t.Fatalf("tc %s: Expect no error, but got error %v", tc.name, err)
			}

			if tc.expected != nil {
				if len(cfg.Periodics) != 1 {
					t.Fatalf("tc %s: Expect to have one periodic job, got none", tc.name)
				}

				if diff := cmp.Diff(cfg.Periodics[0].DecorationConfig, tc.expected, cmpopts.EquateEmpty()); diff != "" {
					t.Errorf("got diff: %s", diff)
				}
			}
		})
	}
}

func TestValidateAgent(t *testing.T) {
	jenk := string(prowjobv1.JenkinsAgent)
	k := string(prowjobv1.KubernetesAgent)
	ns := "default"
	base := JobBase{
		Agent:     k,
		Namespace: &ns,
		Spec:      &v1.PodSpec{},
		UtilityConfig: UtilityConfig{
			DecorationConfig: &prowapi.DecorationConfig{},
		},
	}

	cases := []struct {
		name string
		base func(j *JobBase)
		pass bool
	}{
		{
			name: "accept unknown agent",
			base: func(j *JobBase) {
				j.Agent = "random-agent"
			},
			pass: true,
		},
		{
			name: "kubernetes agent requires spec",
			base: func(j *JobBase) {
				j.Spec = nil
			},
		},
		{
			name: "non-nil namespace required",
			base: func(j *JobBase) {
				j.Namespace = nil
			},
		},
		{
			name: "filled namespace required",
			base: func(j *JobBase) {
				var s string
				j.Namespace = &s
			},
		},
		{
			name: "custom namespace requires knative-build agent",
			base: func(j *JobBase) {
				s := "custom-namespace"
				j.Namespace = &s
			},
		},
		{
			name: "accept kubernetes agent",
			pass: true,
		},
		{
			name: "accept kubernetes agent without decoration",
			base: func(j *JobBase) {
				j.DecorationConfig = nil
			},
			pass: true,
		},
		{
			name: "accept jenkins agent",
			base: func(j *JobBase) {
				j.Agent = jenk
				j.Spec = nil
				j.DecorationConfig = nil
			},
			pass: true,
		},
		{
			name: "error_on_eviction allowed for kubernetes agent",
			base: func(j *JobBase) {
				j.ErrorOnEviction = true
			},
			pass: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jb := base
			if tc.base != nil {
				tc.base(&jb)
			}
			switch err := validateAgent(jb, ns); {
			case err == nil && !tc.pass:
				t.Error("validation failed to raise an error")
			case err != nil && tc.pass:
				t.Errorf("validation should have passed, got: %v", err)
			}
		})
	}
}

func TestValidatePodSpec(t *testing.T) {
	periodEnv := sets.NewString(downwardapi.EnvForType(prowapi.PeriodicJob)...)
	postEnv := sets.NewString(downwardapi.EnvForType(prowapi.PostsubmitJob)...)
	preEnv := sets.NewString(downwardapi.EnvForType(prowapi.PresubmitJob)...)
	cases := []struct {
		name    string
		jobType prowapi.ProwJobType
		spec    func(s *v1.PodSpec)
		noSpec  bool
		pass    bool
	}{
		{
			name:   "allow nil spec",
			noSpec: true,
			pass:   true,
		},
		{
			name: "happy case",
			pass: true,
		},
		{
			name: "reject init containers",
			spec: func(s *v1.PodSpec) {
				s.InitContainers = []v1.Container{
					{},
				}
			},
		},
		{
			name: "reject 0 containers",
			spec: func(s *v1.PodSpec) {
				s.Containers = nil
			},
		},
		{
			name: "reject 2 containers",
			spec: func(s *v1.PodSpec) {
				s.Containers = append(s.Containers, v1.Container{})
			},
		},
		{
			name:    "reject reserved presubmit env",
			jobType: prowapi.PresubmitJob,
			spec: func(s *v1.PodSpec) {
				// find a presubmit value
				for n := range preEnv.Difference(postEnv).Difference(periodEnv) {

					s.Containers[0].Env = append(s.Containers[0].Env, v1.EnvVar{Name: n, Value: "whatever"})
				}
				if len(s.Containers[0].Env) == 0 {
					t.Fatal("empty env")
				}
			},
		},
		{
			name:    "reject reserved postsubmit env",
			jobType: prowapi.PostsubmitJob,
			spec: func(s *v1.PodSpec) {
				// find a postsubmit value
				for n := range postEnv.Difference(periodEnv) {

					s.Containers[0].Env = append(s.Containers[0].Env, v1.EnvVar{Name: n, Value: "whatever"})
				}
				if len(s.Containers[0].Env) == 0 {
					t.Fatal("empty env")
				}
			},
		},
		{
			name:    "reject reserved periodic env",
			jobType: prowapi.PeriodicJob,
			spec: func(s *v1.PodSpec) {
				// find a postsubmit value
				for n := range periodEnv {

					s.Containers[0].Env = append(s.Containers[0].Env, v1.EnvVar{Name: n, Value: "whatever"})
				}
				if len(s.Containers[0].Env) == 0 {
					t.Fatal("empty env")
				}
			},
		},
		{
			name: "reject reserved mount name",
			spec: func(s *v1.PodSpec) {
				s.Containers[0].VolumeMounts = append(s.Containers[0].VolumeMounts, v1.VolumeMount{
					Name:      decorate.VolumeMounts().List()[0],
					MountPath: "/whatever",
				})
			},
		},
		{
			name: "reject reserved mount path",
			spec: func(s *v1.PodSpec) {
				s.Containers[0].VolumeMounts = append(s.Containers[0].VolumeMounts, v1.VolumeMount{
					Name:      "fun",
					MountPath: decorate.VolumeMountPathsOnTestContainer().List()[0],
				})
			},
		},
		{
			name: "accept conflicting mount path parent",
			spec: func(s *v1.PodSpec) {
				s.Containers[0].VolumeMounts = append(s.Containers[0].VolumeMounts, v1.VolumeMount{
					Name:      "foo",
					MountPath: filepath.Dir(decorate.VolumeMountPathsOnTestContainer().List()[0]),
				})
			},
		},
		{
			name: "accept conflicting mount path child",
			spec: func(s *v1.PodSpec) {
				s.Containers[0].VolumeMounts = append(s.Containers[0].VolumeMounts, v1.VolumeMount{
					Name:      "foo",
					MountPath: filepath.Join(decorate.VolumeMountPathsOnTestContainer().List()[0], "extra"),
				})
			},
		},
		{
			name: "reject reserved volume",
			spec: func(s *v1.PodSpec) {
				s.Volumes = append(s.Volumes, v1.Volume{Name: decorate.VolumeMounts().List()[0]})
			},
		},
		{
			name: "reject duplicate env",
			spec: func(s *v1.PodSpec) {
				s.Containers[0].Env = append(s.Containers[0].Env, v1.EnvVar{Name: "foo", Value: "bar"})
				s.Containers[0].Env = append(s.Containers[0].Env, v1.EnvVar{Name: "foo", Value: "baz"})
			},
		},
		{
			name: "reject duplicate volume",
			spec: func(s *v1.PodSpec) {
				s.Volumes = append(s.Volumes, v1.Volume{Name: "foo"})
				s.Volumes = append(s.Volumes, v1.Volume{Name: "foo"})
			},
		},
		{
			name: "reject undefined volume reference",
			spec: func(s *v1.PodSpec) {
				s.Containers[0].VolumeMounts = append(s.Containers[0].VolumeMounts, v1.VolumeMount{Name: "foo", MountPath: "/not-used-by-decoration-utils"})
			},
		},
	}

	spec := v1.PodSpec{
		Containers: []v1.Container{
			{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jt := prowapi.PresubmitJob
			if tc.jobType != "" {
				jt = tc.jobType
			}
			current := spec.DeepCopy()
			if tc.noSpec {
				current = nil
			} else if tc.spec != nil {
				tc.spec(current)
			}
			switch err := validatePodSpec(jt, current, true); {
			case err == nil && !tc.pass:
				t.Error("validation failed to raise an error")
			case err != nil && tc.pass:
				t.Errorf("validation should have passed, got: %v", err)
			}
		})
	}
}

func TestValidatePipelineRunSpec(t *testing.T) {
	cases := []struct {
		name      string
		jobType   prowapi.ProwJobType
		spec      func(s *pipelinev1alpha1.PipelineRunSpec)
		extraRefs []prowapi.Refs
		noSpec    bool
		pass      bool
	}{
		{
			name:   "allow nil spec",
			noSpec: true,
			pass:   true,
		},
		{
			name: "happy case",
			pass: true,
		},
		{
			name:    "reject implicit ref for periodic",
			jobType: prowapi.PeriodicJob,
			spec: func(s *pipelinev1alpha1.PipelineRunSpec) {
				s.Resources = append(s.Resources, pipelinev1alpha1.PipelineResourceBinding{
					Name:        "git ref",
					ResourceRef: &pipelinev1alpha1.PipelineResourceRef{Name: "PROW_IMPLICIT_GIT_REF"},
				})
			},
			pass: false,
		},
		{
			name:    "allow implicit ref for presubmit",
			jobType: prowapi.PresubmitJob,
			spec: func(s *pipelinev1alpha1.PipelineRunSpec) {
				s.Resources = append(s.Resources, pipelinev1alpha1.PipelineResourceBinding{
					Name:        "git ref",
					ResourceRef: &pipelinev1alpha1.PipelineResourceRef{Name: "PROW_IMPLICIT_GIT_REF"},
				})
			},
			pass: true,
		},
		{
			name:    "allow implicit ref for postsubmit",
			jobType: prowapi.PostsubmitJob,
			spec: func(s *pipelinev1alpha1.PipelineRunSpec) {
				s.Resources = append(s.Resources, pipelinev1alpha1.PipelineResourceBinding{
					Name:        "git ref",
					ResourceRef: &pipelinev1alpha1.PipelineResourceRef{Name: "PROW_IMPLICIT_GIT_REF"},
				})
			},
			pass: true,
		},
		{
			name: "reject extra refs usage with no extra refs",
			spec: func(s *pipelinev1alpha1.PipelineRunSpec) {
				s.Resources = append(s.Resources, pipelinev1alpha1.PipelineResourceBinding{
					Name:        "git ref",
					ResourceRef: &pipelinev1alpha1.PipelineResourceRef{Name: "PROW_EXTRA_GIT_REF_0"},
				})
			},
			pass: false,
		},
		{
			name: "allow extra refs usage with extra refs",
			spec: func(s *pipelinev1alpha1.PipelineRunSpec) {
				s.Resources = append(s.Resources, pipelinev1alpha1.PipelineResourceBinding{
					Name:        "git ref",
					ResourceRef: &pipelinev1alpha1.PipelineResourceRef{Name: "PROW_EXTRA_GIT_REF_0"},
				})
			},
			extraRefs: []prowapi.Refs{{Org: "o", Repo: "r"}},
			pass:      true,
		},
		{
			name: "reject wrong extra refs index usage",
			spec: func(s *pipelinev1alpha1.PipelineRunSpec) {
				s.Resources = append(s.Resources, pipelinev1alpha1.PipelineResourceBinding{
					Name:        "git ref",
					ResourceRef: &pipelinev1alpha1.PipelineResourceRef{Name: "PROW_EXTRA_GIT_REF_1"},
				})
			},
			extraRefs: []prowapi.Refs{{Org: "o", Repo: "r"}},
			pass:      false,
		},
		{
			name:      "reject extra refs without usage",
			extraRefs: []prowapi.Refs{{Org: "o", Repo: "r"}},
			pass:      false,
		},
		{
			name: "allow unrelated resource refs",
			spec: func(s *pipelinev1alpha1.PipelineRunSpec) {
				s.Resources = append(s.Resources, pipelinev1alpha1.PipelineResourceBinding{
					Name:        "git ref",
					ResourceRef: &pipelinev1alpha1.PipelineResourceRef{Name: "some-other-ref"},
				})
			},
			pass: true,
		},
		{
			name: "reject leading zeros when extra ref usage is otherwise valid",
			spec: func(s *pipelinev1alpha1.PipelineRunSpec) {
				s.Resources = append(s.Resources, pipelinev1alpha1.PipelineResourceBinding{
					Name:        "git ref",
					ResourceRef: &pipelinev1alpha1.PipelineResourceRef{Name: "PROW_EXTRA_GIT_REF_000"},
				})
			},
			extraRefs: []prowapi.Refs{{Org: "o", Repo: "r"}},
			pass:      false,
		},
	}

	spec := pipelinev1alpha1.PipelineRunSpec{}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jt := prowapi.PresubmitJob
			if tc.jobType != "" {
				jt = tc.jobType
			}
			current := spec.DeepCopy()
			if tc.noSpec {
				current = nil
			} else if tc.spec != nil {
				tc.spec(current)
			}
			switch err := ValidatePipelineRunSpec(jt, tc.extraRefs, current); {
			case err == nil && !tc.pass:
				t.Error("validation failed to raise an error")
			case err != nil && tc.pass:
				t.Errorf("validation should have passed, got: %v", err)
			}
		})
	}
}

func TestValidateDecoration(t *testing.T) {
	defCfg := prowapi.DecorationConfig{
		UtilityImages: &prowjobv1.UtilityImages{
			CloneRefs:  "clone-me",
			InitUpload: "upload-me",
			Entrypoint: "enter-me",
			Sidecar:    "official-drink-of-the-org",
		},
		GCSCredentialsSecret: "upload-secret",
		GCSConfiguration: &prowjobv1.GCSConfiguration{
			PathStrategy: prowjobv1.PathStrategyExplicit,
			DefaultOrg:   "so-org",
			DefaultRepo:  "very-repo",
		},
	}
	cases := []struct {
		name      string
		container v1.Container
		config    *prowapi.DecorationConfig
		pass      bool
	}{
		{
			name: "allow no decoration",
			pass: true,
		},
		{
			name:   "happy case with cmd",
			config: &defCfg,
			container: v1.Container{
				Command: []string{"hello", "world"},
			},
			pass: true,
		},
		{
			name:   "happy case with args",
			config: &defCfg,
			container: v1.Container{
				Args: []string{"hello", "world"},
			},
			pass: true,
		},
		{
			name:   "reject invalid decoration config",
			config: &prowapi.DecorationConfig{},
			container: v1.Container{
				Command: []string{"hello", "world"},
			},
		},
		{
			name:   "reject container that has no cmd, no args",
			config: &defCfg,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			switch err := validateDecoration(tc.container, tc.config); {
			case err == nil && !tc.pass:
				t.Error("validation failed to raise an error")
			case err != nil && tc.pass:
				t.Errorf("validation should have passed, got: %v", err)
			}
		})
	}
}

func TestValidateLabels(t *testing.T) {
	cases := []struct {
		name   string
		labels map[string]string
		pass   bool
	}{
		{
			name: "happy case",
			pass: true,
		},
		{
			name: "reject reserved label",
			labels: map[string]string{
				decorate.Labels()[0]: "anything",
			},
		},
		{
			name: "reject bad label key",
			labels: map[string]string{
				"_underscore-prefix": "annoying",
			},
		},
		{
			name: "reject bad label value",
			labels: map[string]string{
				"whatever": "_private-is-rejected",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			switch err := validateLabels(tc.labels); {
			case err == nil && !tc.pass:
				t.Error("validation failed to raise an error")
			case err != nil && tc.pass:
				t.Errorf("validation should have passed, got: %v", err)
			}
		})
	}
}

func TestValidateMultipleContainers(t *testing.T) {
	ka := string(prowjobv1.KubernetesAgent)
	yes := true
	defCfg := prowapi.DecorationConfig{
		UtilityImages: &prowjobv1.UtilityImages{
			CloneRefs:  "clone-me",
			InitUpload: "upload-me",
			Entrypoint: "enter-me",
			Sidecar:    "official-drink-of-the-org",
		},
		GCSCredentialsSecret: "upload-secret",
		GCSConfiguration: &prowjobv1.GCSConfiguration{
			PathStrategy: prowjobv1.PathStrategyExplicit,
			DefaultOrg:   "so-org",
			DefaultRepo:  "very-repo",
		},
	}
	goodSpec := v1.PodSpec{
		Containers: []v1.Container{
			{
				Name:    "test1",
				Command: []string{"hello", "world"},
			},
			{
				Name: "test2",
				Args: []string{"hello", "world"},
			},
		},
	}
	ns := "target-namespace"
	cases := []struct {
		name string
		base JobBase
		pass bool
	}{
		{
			name: "valid kubernetes job with multiple containers",
			base: JobBase{
				Name:          "name",
				Agent:         ka,
				UtilityConfig: UtilityConfig{Decorate: &yes, DecorationConfig: &defCfg},
				Spec:          &goodSpec,
				Namespace:     &ns,
			},
			pass: true,
		},
		{
			name: "invalid: containers with no cmd or args",
			base: JobBase{
				Name:          "name",
				Agent:         ka,
				UtilityConfig: UtilityConfig{Decorate: &yes, DecorationConfig: &defCfg},
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "test1",
						},
						{
							Name: "test2",
						},
					},
				},
				Namespace: &ns,
			},
		},
		{
			name: "invalid: containers with no names",
			base: JobBase{
				Name:          "name",
				Agent:         ka,
				UtilityConfig: UtilityConfig{Decorate: &yes, DecorationConfig: &defCfg},
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Command: []string{"hello", "world"},
						},
						{
							Args: []string{"hello", "world"},
						},
					},
				},
				Namespace: &ns,
			},
		},
		{
			name: "invalid: no decoration enabled",
			base: JobBase{
				Name:      "name",
				Agent:     ka,
				Spec:      &goodSpec,
				Namespace: &ns,
			},
		},
		{
			name: "invalid: container names reserved for decoration",
			base: JobBase{
				Name:          "name",
				Agent:         ka,
				UtilityConfig: UtilityConfig{Decorate: &yes, DecorationConfig: &defCfg},
				Spec: &v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:    "place-entrypoint",
							Command: []string{"hello", "world"},
						},
						{
							Name: "sidecar",
							Args: []string{"hello", "world"},
						},
					},
				}, Namespace: &ns,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			switch err := validateJobBase(tc.base, prowjobv1.PresubmitJob, ns); {
			case err == nil && !tc.pass:
				t.Error("validation failed to raise an error")
			case err != nil && tc.pass:
				t.Errorf("validation should have passed, got: %v", err)
			}
		})
	}
}

func TestValidateJobBase(t *testing.T) {
	ka := string(prowjobv1.KubernetesAgent)
	ja := string(prowjobv1.JenkinsAgent)
	goodSpec := v1.PodSpec{
		Containers: []v1.Container{
			{},
		},
	}
	ns := "target-namespace"
	cases := []struct {
		name string
		base JobBase
		pass bool
	}{
		{
			name: "valid kubernetes job",
			base: JobBase{
				Name:      "name",
				Agent:     ka,
				Spec:      &goodSpec,
				Namespace: &ns,
			},
			pass: true,
		},
		{
			name: "valid jenkins job",
			base: JobBase{
				Name:      "name",
				Agent:     ja,
				Namespace: &ns,
			},
			pass: true,
		},
		{
			name: "invalid concurrency",
			base: JobBase{
				Name:           "name",
				MaxConcurrency: -1,
				Agent:          ka,
				Spec:           &goodSpec,
				Namespace:      &ns,
			},
		},
		{
			name: "invalid pod spec",
			base: JobBase{
				Name:      "name",
				Agent:     ka,
				Namespace: &ns,
				Spec:      &v1.PodSpec{}, // no containers
			},
		},
		{
			name: "invalid decoration",
			base: JobBase{
				Name:  "name",
				Agent: ka,
				Spec:  &goodSpec,
				UtilityConfig: UtilityConfig{
					DecorationConfig: &prowjobv1.DecorationConfig{}, // missing many fields
				},
				Namespace: &ns,
			},
		},
		{
			name: "invalid labels",
			base: JobBase{
				Name:  "name",
				Agent: ka,
				Spec:  &goodSpec,
				Labels: map[string]string{
					"_leading_underscore": "_rejected",
				},
				Namespace: &ns,
			},
		},
		{
			name: "invalid name",
			base: JobBase{
				Name:      "a/b",
				Agent:     ka,
				Spec:      &goodSpec,
				Namespace: &ns,
			},
			pass: false,
		},
		{
			name: "valid complex name",
			base: JobBase{
				Name:      "a-b.c",
				Agent:     ka,
				Spec:      &goodSpec,
				Namespace: &ns,
			},
			pass: true,
		},
		{
			name: "invalid rerun_permissions",
			base: JobBase{
				RerunAuthConfig: &prowapi.RerunAuthConfig{
					AllowAnyone: true,
					GitHubUsers: []string{"user"},
				},
			},
			pass: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			switch err := validateJobBase(tc.base, prowjobv1.PresubmitJob, ns); {
			case err == nil && !tc.pass:
				t.Error("validation failed to raise an error")
			case err != nil && tc.pass:
				t.Errorf("validation should have passed, got: %v", err)
			}
		})
	}
}

func TestValidateDeck(t *testing.T) {
	boolTrue := true
	boolFalse := false
	cases := []struct {
		name        string
		deck        Deck
		expectedErr string
	}{
		{
			name:        "empty Deck is valid",
			deck:        Deck{},
			expectedErr: "",
		},
		{
			name:        "AdditionalAllowedBuckets has items, SkipStoragePathValidation is false => no errors",
			deck:        Deck{SkipStoragePathValidation: &boolFalse, AdditionalAllowedBuckets: []string{"foo", "bar", "batz"}},
			expectedErr: "",
		},
		{
			name:        "AdditionalAllowedBuckets has items, SkipStoragePathValidation is default value => error",
			deck:        Deck{AdditionalAllowedBuckets: []string{"hello", "world"}},
			expectedErr: "skip_storage_path_validation is enabled",
		},
		{
			name:        "AdditionalAllowedBuckets has items, SkipStoragePathValidation is true => error",
			deck:        Deck{SkipStoragePathValidation: &boolTrue, AdditionalAllowedBuckets: []string{"hello", "world"}},
			expectedErr: "skip_storage_path_validation is enabled",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expectingErr := len(tc.expectedErr) > 0
			err := tc.deck.Validate()
			if expectingErr && err == nil {
				t.Fatalf("expecting error (%v), but did not get an error", tc.expectedErr)
			}
			if !expectingErr && err != nil {
				t.Fatalf("not expecting error, but got an error: %v", err)
			}
			if expectingErr && err != nil && !strings.Contains(err.Error(), tc.expectedErr) {
				t.Fatalf("expected error (%v), but got unknown error, instead: %v", tc.expectedErr, err)
			}
		})
	}
}

func TestValidateRefs(t *testing.T) {
	cases := []struct {
		name      string
		extraRefs []prowapi.Refs
		expected  error
	}{
		{
			name: "validation error for extra ref specifying the same repo for which the job is configured",
			extraRefs: []prowapi.Refs{
				{
					Org:  "org",
					Repo: "repo",
				},
			},
			expected: fmt.Errorf("Invalid job test on repo org/repo: the following refs specified more than once: %s",
				"org/repo"),
		},
		{
			name: "validation error lists all duplications",
			extraRefs: []prowapi.Refs{
				{
					Org:  "org",
					Repo: "repo",
				},
				{
					Org:  "org",
					Repo: "foo",
				},
				{
					Org:  "org",
					Repo: "bar",
				},
				{
					Org:  "org",
					Repo: "foo",
				},
			},
			expected: fmt.Errorf("Invalid job test on repo org/repo: the following refs specified more than once: %s",
				"org/foo,org/repo"),
		},
		{
			name: "no errors if there are no duplications",
			extraRefs: []prowapi.Refs{
				{
					Org:  "org",
					Repo: "foo",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			job := JobBase{
				Name: "test",
				UtilityConfig: UtilityConfig{
					ExtraRefs: tc.extraRefs,
				},
			}
			if err := ValidateRefs("org/repo", job); !reflect.DeepEqual(err, tc.expected) {
				t.Errorf("expected %#v\n!=\nactual %#v", tc.expected, err)
			}
		})
	}
}

func TestValidateReportingWithGerritLabel(t *testing.T) {
	cases := []struct {
		name     string
		labels   map[string]string
		reporter Reporter
		expected error
	}{
		{
			name: "no errors if job is set to report",
			reporter: Reporter{
				Context: "context",
			},
			labels: map[string]string{
				gerrit.GerritReportLabel: "label",
			},
		},
		{
			name:     "no errors if Gerrit report label is not defined",
			reporter: Reporter{SkipReport: true},
			labels: map[string]string{
				"label": "value",
			},
		},
		{
			name:     "no errors if job is set to skip report and Gerrit report label is empty",
			reporter: Reporter{SkipReport: true},
			labels: map[string]string{
				gerrit.GerritReportLabel: "",
			},
		},
		{
			name:     "error if job is set to skip report and Gerrit report label is set to non-empty",
			reporter: Reporter{SkipReport: true},
			labels: map[string]string{
				gerrit.GerritReportLabel: "label",
			},
			expected: fmt.Errorf("Gerrit report label %s set to non-empty string but job is configured to skip reporting.", gerrit.GerritReportLabel),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := JobBase{
				Name:   "test-job",
				Labels: tc.labels,
			}
			presubmits := []Presubmit{
				{
					JobBase:  base,
					Reporter: tc.reporter,
				},
			}
			var expected error
			if tc.expected != nil {
				expected = fmt.Errorf("invalid presubmit job %s: %v", "test-job", tc.expected)
			}
			if err := validatePresubmits(presubmits, "default-namespace"); !reflect.DeepEqual(err, utilerrors.NewAggregate([]error{expected})) {
				t.Errorf("did not get expected validation result:\n%v", cmp.Diff(expected, err))
			}

			postsubmits := []Postsubmit{
				{
					JobBase:  base,
					Reporter: tc.reporter,
				},
			}
			if tc.expected != nil {
				expected = fmt.Errorf("invalid postsubmit job %s: %v", "test-job", tc.expected)
			}
			if err := validatePostsubmits(postsubmits, "default-namespace"); !reflect.DeepEqual(err, utilerrors.NewAggregate([]error{expected})) {
				t.Errorf("did not get expected validation result:\n%v", cmp.Diff(expected, err))
			}
		})
	}
}

// integration test for fake config loading
func TestValidConfigLoading(t *testing.T) {
	var testCases = []struct {
		name               string
		prowConfig         string
		jobConfigs         []string
		expectError        bool
		expectPodNameSpace string
		expectEnv          map[string][]v1.EnvVar
		verify             func(*Config) error
	}{
		{
			name:       "one config",
			prowConfig: ``,
		},
		{
			name:       "reject invalid kubernetes periodic",
			prowConfig: ``,
			jobConfigs: []string{
				`
periodics:
- interval: 10m
  agent: kubernetes
  build_spec:
  name: foo`,
			},
			expectError: true,
		},
		{
			name:       "one periodic",
			prowConfig: ``,
			jobConfigs: []string{
				`
periodics:
- interval: 10m
  agent: kubernetes
  name: foo
  spec:
    containers:
    - image: alpine`,
			},
		},
		{
			name:       "one periodic no agent, should default",
			prowConfig: ``,
			jobConfigs: []string{
				`
periodics:
- interval: 10m
  name: foo
  spec:
    containers:
    - image: alpine`,
			},
		},
		{
			name:       "two periodics",
			prowConfig: ``,
			jobConfigs: []string{
				`
periodics:
- interval: 10m
  agent: kubernetes
  name: foo
  spec:
    containers:
    - image: alpine`,
				`
periodics:
- interval: 10m
  agent: kubernetes
  name: bar
  spec:
    containers:
    - image: alpine`,
			},
		},
		{
			name:       "duplicated periodics",
			prowConfig: ``,
			jobConfigs: []string{
				`
periodics:
- interval: 10m
  agent: kubernetes
  name: foo
  spec:
    containers:
    - image: alpine`,
				`
periodics:
- interval: 10m
  agent: kubernetes
  name: foo
  spec:
    containers:
    - image: alpine`,
			},
			expectError: true,
		},
		{
			name:       "one presubmit no context should default",
			prowConfig: ``,
			jobConfigs: []string{
				`
presubmits:
  foo/bar:
  - agent: kubernetes
    name: presubmit-bar
    spec:
      containers:
      - image: alpine`,
			},
		},
		{
			name:       "one presubmit no agent should default",
			prowConfig: ``,
			jobConfigs: []string{
				`
presubmits:
  foo/bar:
  - context: bar
    name: presubmit-bar
    spec:
      containers:
      - image: alpine`,
			},
		},
		{
			name:       "one presubmit, ok",
			prowConfig: ``,
			jobConfigs: []string{
				`
presubmits:
  foo/bar:
  - agent: kubernetes
    name: presubmit-bar
    context: bar
    spec:
      containers:
      - image: alpine`,
			},
		},
		{
			name:       "two presubmits",
			prowConfig: ``,
			jobConfigs: []string{
				`
presubmits:
  foo/bar:
  - agent: kubernetes
    name: presubmit-bar
    context: bar
    spec:
      containers:
      - image: alpine`,
				`
presubmits:
  foo/baz:
  - agent: kubernetes
    name: presubmit-baz
    context: baz
    spec:
      containers:
      - image: alpine`,
			},
		},
		{
			name:       "dup presubmits, one file",
			prowConfig: ``,
			jobConfigs: []string{
				`
presubmits:
  foo/bar:
  - agent: kubernetes
    name: presubmit-bar
    context: bar
    spec:
      containers:
      - image: alpine
  - agent: kubernetes
    name: presubmit-bar
    context: bar
    spec:
      containers:
      - image: alpine`,
			},
			expectError: true,
		},
		{
			name:       "dup presubmits, two files",
			prowConfig: ``,
			jobConfigs: []string{
				`
presubmits:
  foo/bar:
  - agent: kubernetes
    name: presubmit-bar
    context: bar
    spec:
      containers:
      - image: alpine`,
				`
presubmits:
  foo/bar:
  - agent: kubernetes
    context: bar
    name: presubmit-bar
    spec:
      containers:
      - image: alpine`,
			},
			expectError: true,
		},
		{
			name:       "dup presubmits not the same branch, two files",
			prowConfig: ``,
			jobConfigs: []string{
				`
presubmits:
  foo/bar:
  - agent: kubernetes
    name: presubmit-bar
    context: bar
    branches:
    - master
    spec:
      containers:
      - image: alpine`,
				`
presubmits:
  foo/bar:
  - agent: kubernetes
    context: bar
    branches:
    - other
    name: presubmit-bar
    spec:
      containers:
      - image: alpine`,
			},
			expectError: false,
		},
		{
			name: "dup presubmits main file",
			prowConfig: `
presubmits:
  foo/bar:
  - agent: kubernetes
    name: presubmit-bar
    context: bar
    spec:
      containers:
      - image: alpine
  - agent: kubernetes
    context: bar
    name: presubmit-bar
    spec:
      containers:
      - image: alpine`,
			expectError: true,
		},
		{
			name: "dup presubmits main file not on the same branch",
			prowConfig: `
presubmits:
  foo/bar:
  - agent: kubernetes
    name: presubmit-bar
    context: bar
    branches:
    - other
    spec:
      containers:
      - image: alpine
  - agent: kubernetes
    context: bar
    branches:
    - master
    name: presubmit-bar
    spec:
      containers:
      - image: alpine`,
			expectError: false,
		},

		{
			name:       "one postsubmit, ok",
			prowConfig: ``,
			jobConfigs: []string{
				`
postsubmits:
  foo/bar:
  - agent: kubernetes
    name: postsubmit-bar
    spec:
      containers:
      - image: alpine`,
			},
		},
		{
			name:       "one postsubmit no agent, should default",
			prowConfig: ``,
			jobConfigs: []string{
				`
postsubmits:
  foo/bar:
  - name: postsubmit-bar
    spec:
      containers:
      - image: alpine`,
			},
		},
		{
			name:       "two postsubmits",
			prowConfig: ``,
			jobConfigs: []string{
				`
postsubmits:
  foo/bar:
  - agent: kubernetes
    name: postsubmit-bar
    spec:
      containers:
      - image: alpine`,
				`
postsubmits:
  foo/baz:
  - agent: kubernetes
    name: postsubmit-baz
    spec:
      containers:
      - image: alpine`,
			},
		},
		{
			name:       "dup postsubmits, one file",
			prowConfig: ``,
			jobConfigs: []string{
				`
postsubmits:
  foo/bar:
  - agent: kubernetes
    name: postsubmit-bar
    spec:
      containers:
      - image: alpine
  - agent: kubernetes
    name: postsubmit-bar
    spec:
      containers:
      - image: alpine`,
			},
			expectError: true,
		},
		{
			name:       "dup postsubmits, two files",
			prowConfig: ``,
			jobConfigs: []string{
				`
postsubmits:
  foo/bar:
  - agent: kubernetes
    name: postsubmit-bar
    spec:
      containers:
      - image: alpine`,
				`
postsubmits:
  foo/bar:
  - agent: kubernetes
    name: postsubmit-bar
    spec:
      containers:
      - image: alpine`,
			},
			expectError: true,
		},
		{
			name: "test valid presets in main config",
			prowConfig: `
presets:
- labels:
    preset-baz: "true"
  env:
  - name: baz
    value: fejtaverse`,
			jobConfigs: []string{
				`periodics:
- interval: 10m
  agent: kubernetes
  name: foo
  labels:
    preset-baz: "true"
  spec:
    containers:
    - image: alpine`,
				`
periodics:
- interval: 10m
  agent: kubernetes
  name: bar
  labels:
    preset-baz: "true"
  spec:
    containers:
    - image: alpine`,
			},
			expectEnv: map[string][]v1.EnvVar{
				"foo": {
					{
						Name:  "baz",
						Value: "fejtaverse",
					},
				},
				"bar": {
					{
						Name:  "baz",
						Value: "fejtaverse",
					},
				},
			},
		},
		{
			name:       "test valid presets in job configs",
			prowConfig: ``,
			jobConfigs: []string{
				`
presets:
- labels:
    preset-baz: "true"
  env:
  - name: baz
    value: fejtaverse
periodics:
- interval: 10m
  agent: kubernetes
  name: foo
  labels:
    preset-baz: "true"
  spec:
    containers:
    - image: alpine`,
				`
periodics:
- interval: 10m
  agent: kubernetes
  name: bar
  labels:
    preset-baz: "true"
  spec:
    containers:
    - image: alpine`,
			},
			expectEnv: map[string][]v1.EnvVar{
				"foo": {
					{
						Name:  "baz",
						Value: "fejtaverse",
					},
				},
				"bar": {
					{
						Name:  "baz",
						Value: "fejtaverse",
					},
				},
			},
		},
		{
			name: "test valid presets in both main & job configs",
			prowConfig: `
presets:
- labels:
    preset-baz: "true"
  env:
  - name: baz
    value: fejtaverse`,
			jobConfigs: []string{
				`
presets:
- labels:
    preset-k8s: "true"
  env:
  - name: k8s
    value: kubernetes
periodics:
- interval: 10m
  agent: kubernetes
  name: foo
  labels:
    preset-baz: "true"
    preset-k8s: "true"
  spec:
    containers:
    - image: alpine`,
				`
periodics:
- interval: 10m
  agent: kubernetes
  name: bar
  labels:
    preset-baz: "true"
  spec:
    containers:
    - image: alpine`,
			},
			expectEnv: map[string][]v1.EnvVar{
				"foo": {
					{
						Name:  "baz",
						Value: "fejtaverse",
					},
					{
						Name:  "k8s",
						Value: "kubernetes",
					},
				},
				"bar": {
					{
						Name:  "baz",
						Value: "fejtaverse",
					},
				},
			},
		},
		{
			name:       "decorated periodic missing `command`",
			prowConfig: ``,
			jobConfigs: []string{
				`
periodics:
- interval: 10m
  agent: kubernetes
  name: foo
  decorate: true
  spec:
    containers:
    - image: alpine`,
			},
			expectError: true,
		},
		{
			name: "all repos contains repos from tide, presubmits and postsubmits",
			prowConfig: `
tide:
  queries:
  - repos:
    - stranded/fish`,
			jobConfigs: []string{`
presubmits:
  k/k:
  - name: my-job
    spec:
      containers:
      - name: lost-vessel
        image: vessel:latest
        command: ["ride"]`,
				`
postsubmits:
  k/test-infra:
  - name: my-job
    spec:
      containers:
      - name: lost-vessel
        image: vessel:latest
        command: ["ride"]`,
			},
			verify: func(c *Config) error {
				if diff := c.AllRepos.Difference(sets.NewString("k/k", "k/test-infra", "stranded/fish")); len(diff) != 0 {
					return fmt.Errorf("expected no diff, got %q", diff)
				}
				return nil
			},
		},
		{
			name: "no jobs doesn't make AllRepos a nilpointer",
			verify: func(c *Config) error {
				if c.AllRepos == nil {
					return errors.New("config.AllRepos is nil")
				}
				return nil
			},
		},
		{
			name: "prowYAMLGetter gets set",
			verify: func(c *Config) error {
				if c.ProwYAMLGetter == nil {
					return errors.New("config.ProwYAMLGetter is nil")
				}
				return nil
			},
		},
		{
			name: "InRepoConfigAllowedClusters gets defaulted if unset",
			verify: func(c *Config) error {
				if len(c.InRepoConfig.AllowedClusters) != 1 ||
					len(c.InRepoConfig.AllowedClusters["*"]) != 1 ||
					c.InRepoConfig.AllowedClusters["*"][0] != kube.DefaultClusterAlias {
					return fmt.Errorf("expected c.InRepoConfig.AllowedClusters to contain exactly one global entry to allow the buildcluster, was %v", c.InRepoConfig.AllowedClusters)
				}
				return nil
			},
		},
		{
			name: "InRepoConfigAllowedClusters gets defaulted if no global setting",
			prowConfig: `
in_repo_config:
  allowed_clusters:
    foo/bar: ["my-cluster"]
`,
			verify: func(c *Config) error {
				if len(c.InRepoConfig.AllowedClusters) != 2 ||
					len(c.InRepoConfig.AllowedClusters["*"]) != 1 ||
					c.InRepoConfig.AllowedClusters["*"][0] != kube.DefaultClusterAlias {
					return fmt.Errorf("expected c.InRepoConfig.AllowedClusters to contain exactly one global entry to allow the buildcluster, was %v", c.InRepoConfig.AllowedClusters)
				}
				return nil
			},
		},
		{
			name: "InRepoConfigAllowedClusters doesn't get overwritten",
			prowConfig: `
in_repo_config:
  allowed_clusters:
    foo/bar: ["my-cluster"]
`,
			verify: func(c *Config) error {
				if len(c.InRepoConfig.AllowedClusters) != 2 ||
					len(c.InRepoConfig.AllowedClusters["foo/bar"]) != 1 ||
					c.InRepoConfig.AllowedClusters["foo/bar"][0] != "my-cluster" {
					return fmt.Errorf("expected c.InRepoConfig.AllowedClusters to contain exactly one entry for foo/bar, was %v", c.InRepoConfig.AllowedClusters)
				}
				return nil
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// save the config
			prowConfigDir, err := ioutil.TempDir("", "prowConfig")
			if err != nil {
				t.Fatalf("fail to make tempdir: %v", err)
			}
			defer os.RemoveAll(prowConfigDir)

			prowConfig := filepath.Join(prowConfigDir, "config.yaml")
			if err := ioutil.WriteFile(prowConfig, []byte(tc.prowConfig), 0666); err != nil {
				t.Fatalf("fail to write prow config: %v", err)
			}

			jobConfig := ""
			if len(tc.jobConfigs) > 0 {
				jobConfigDir, err := ioutil.TempDir("", "jobConfig")
				if err != nil {
					t.Fatalf("fail to make tempdir: %v", err)
				}
				defer os.RemoveAll(jobConfigDir)

				// cover both job config as a file & a dir
				if len(tc.jobConfigs) == 1 {
					// a single file
					jobConfig = filepath.Join(jobConfigDir, "config.yaml")
					if err := ioutil.WriteFile(jobConfig, []byte(tc.jobConfigs[0]), 0666); err != nil {
						t.Fatalf("fail to write job config: %v", err)
					}
				} else {
					// a dir
					jobConfig = jobConfigDir
					for idx, config := range tc.jobConfigs {
						subConfig := filepath.Join(jobConfigDir, fmt.Sprintf("config_%d.yaml", idx))
						if err := ioutil.WriteFile(subConfig, []byte(config), 0666); err != nil {
							t.Fatalf("fail to write job config: %v", err)
						}
					}
				}
			}

			cfg, err := Load(prowConfig, jobConfig)
			if tc.expectError && err == nil {
				t.Errorf("tc %s: Expect error, but got nil", tc.name)
			} else if !tc.expectError && err != nil {
				t.Errorf("tc %s: Expect no error, but got error %v", tc.name, err)
			}

			if err == nil {
				if tc.expectPodNameSpace == "" {
					tc.expectPodNameSpace = "default"
				}

				if cfg.PodNamespace != tc.expectPodNameSpace {
					t.Errorf("tc %s: Expect PodNamespace %s, but got %v", tc.name, tc.expectPodNameSpace, cfg.PodNamespace)
				}

				if len(tc.expectEnv) > 0 {
					for _, j := range cfg.AllStaticPresubmits(nil) {
						if envs, ok := tc.expectEnv[j.Name]; ok {
							if !reflect.DeepEqual(envs, j.Spec.Containers[0].Env) {
								t.Errorf("tc %s: expect env %v for job %s, got %+v", tc.name, envs, j.Name, j.Spec.Containers[0].Env)
							}
						}
					}

					for _, j := range cfg.AllStaticPostsubmits(nil) {
						if envs, ok := tc.expectEnv[j.Name]; ok {
							if !reflect.DeepEqual(envs, j.Spec.Containers[0].Env) {
								t.Errorf("tc %s: expect env %v for job %s, got %+v", tc.name, envs, j.Name, j.Spec.Containers[0].Env)
							}
						}
					}

					for _, j := range cfg.AllPeriodics() {
						if envs, ok := tc.expectEnv[j.Name]; ok {
							if !reflect.DeepEqual(envs, j.Spec.Containers[0].Env) {
								t.Errorf("tc %s: expect env %v for job %s, got %+v", tc.name, envs, j.Name, j.Spec.Containers[0].Env)
							}
						}
					}
				}
			}

			if tc.verify != nil {
				if err := tc.verify(cfg); err != nil {
					t.Fatalf("verify failed:  %v", err)
				}
			}
		})
	}
}

func TestBrancher_Intersects(t *testing.T) {
	testCases := []struct {
		name   string
		a, b   Brancher
		result bool
	}{
		{
			name: "TwodifferentBranches",
			a: Brancher{
				Branches: []string{"a"},
			},
			b: Brancher{
				Branches: []string{"b"},
			},
		},
		{
			name: "Opposite",
			a: Brancher{
				SkipBranches: []string{"b"},
			},
			b: Brancher{
				Branches: []string{"b"},
			},
		},
		{
			name:   "BothRunOnAllBranches",
			a:      Brancher{},
			b:      Brancher{},
			result: true,
		},
		{
			name: "RunsOnAllBranchesAndSpecified",
			a:    Brancher{},
			b: Brancher{
				Branches: []string{"b"},
			},
			result: true,
		},
		{
			name: "SkipBranchesAndSet",
			a: Brancher{
				SkipBranches: []string{"a", "b", "c"},
			},
			b: Brancher{
				Branches: []string{"a"},
			},
		},
		{
			name: "SkipBranchesAndSet",
			a: Brancher{
				Branches: []string{"c"},
			},
			b: Brancher{
				Branches: []string{"a"},
			},
		},
		{
			name: "BothSkipBranches",
			a: Brancher{
				SkipBranches: []string{"a", "b", "c"},
			},
			b: Brancher{
				SkipBranches: []string{"d", "e", "f"},
			},
			result: true,
		},
		{
			name: "BothSkipCommonBranches",
			a: Brancher{
				SkipBranches: []string{"a", "b", "c"},
			},
			b: Brancher{
				SkipBranches: []string{"b", "e", "f"},
			},
			result: true,
		},
		{
			name: "NoIntersectionBecauseRegexSkip",
			a: Brancher{
				SkipBranches: []string{`release-\d+\.\d+`},
			},
			b: Brancher{
				Branches: []string{`release-1.14`, `release-1.13`},
			},
			result: false,
		},
		{
			name: "IntersectionDespiteRegexSkip",
			a: Brancher{
				SkipBranches: []string{`release-\d+\.\d+`},
			},
			b: Brancher{
				Branches: []string{`release-1.14`, `master`},
			},
			result: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(st *testing.T) {
			a, err := setBrancherRegexes(tc.a)
			if err != nil {
				st.Fatalf("Failed to set brancher A regexes: %v", err)
			}
			b, err := setBrancherRegexes(tc.b)
			if err != nil {
				st.Fatalf("Failed to set brancher B regexes: %v", err)
			}
			r1 := a.Intersects(b)
			r2 := b.Intersects(a)
			for _, result := range []bool{r1, r2} {
				if result != tc.result {
					st.Errorf("Expected %v got %v", tc.result, result)
				}
			}
		})
	}
}

// Integration test for fake secrets loading in a secret agent.
// Checking also if the agent changes the secret's values as expected.
func TestSecretAgentLoading(t *testing.T) {
	tempTokenValue := "121f3cb3e7f70feeb35f9204f5a988d7292c7ba1"
	changedTokenValue := "121f3cb3e7f70feeb35f9204f5a988d7292c7ba0"

	// Creating a temporary directory.
	secretDir, err := ioutil.TempDir("", "secretDir")
	if err != nil {
		t.Fatalf("fail to create a temporary directory: %v", err)
	}
	defer os.RemoveAll(secretDir)

	// Create the first temporary secret.
	firstTempSecret := filepath.Join(secretDir, "firstTempSecret")
	if err := ioutil.WriteFile(firstTempSecret, []byte(tempTokenValue), 0666); err != nil {
		t.Fatalf("fail to write secret: %v", err)
	}

	// Create the second temporary secret.
	secondTempSecret := filepath.Join(secretDir, "secondTempSecret")
	if err := ioutil.WriteFile(secondTempSecret, []byte(tempTokenValue), 0666); err != nil {
		t.Fatalf("fail to write secret: %v", err)
	}

	tempSecrets := []string{firstTempSecret, secondTempSecret}
	// Starting the agent and add the two temporary secrets.
	secretAgent := &secret.Agent{}
	if err := secretAgent.Start(tempSecrets); err != nil {
		t.Fatalf("Error starting secrets agent. %v", err)
	}

	// Check if the values are as expected.
	for _, tempSecret := range tempSecrets {
		tempSecretValue := secretAgent.GetSecret(tempSecret)
		if string(tempSecretValue) != tempTokenValue {
			t.Fatalf("In secret %s it was expected %s but found %s",
				tempSecret, tempTokenValue, tempSecretValue)
		}
	}

	// Change the values of the files.
	if err := ioutil.WriteFile(firstTempSecret, []byte(changedTokenValue), 0666); err != nil {
		t.Fatalf("fail to write secret: %v", err)
	}
	if err := ioutil.WriteFile(secondTempSecret, []byte(changedTokenValue), 0666); err != nil {
		t.Fatalf("fail to write secret: %v", err)
	}

	retries := 10
	var errors []string

	// Check if the values changed as expected.
	for _, tempSecret := range tempSecrets {
		// Reset counter
		counter := 0
		for counter <= retries {
			tempSecretValue := secretAgent.GetSecret(tempSecret)
			if string(tempSecretValue) != changedTokenValue {
				if counter == retries {
					errors = append(errors, fmt.Sprintf("In secret %s it was expected %s but found %s\n",
						tempSecret, changedTokenValue, tempSecretValue))
				} else {
					// Secret agent needs some time to update the values. So wait and retry.
					time.Sleep(400 * time.Millisecond)
				}
			} else {
				break
			}
			counter++
		}
	}

	if len(errors) > 0 {
		t.Fatal(errors)
	}

}

func TestValidGitHubReportType(t *testing.T) {
	var testCases = []struct {
		name        string
		prowConfig  string
		expectError bool
		expectTypes []prowapi.ProwJobType
	}{
		{
			name:        "empty config should default to report for both presubmit and postsubmit",
			prowConfig:  ``,
			expectTypes: []prowapi.ProwJobType{prowapi.PresubmitJob, prowapi.PostsubmitJob},
		},
		{
			name: "reject unsupported job types",
			prowConfig: `
github_reporter:
  job_types_to_report:
  - presubmit
  - batch
`,
			expectError: true,
		},
		{
			name: "accept valid job types",
			prowConfig: `
github_reporter:
  job_types_to_report:
  - presubmit
  - postsubmit
`,
			expectTypes: []prowapi.ProwJobType{prowapi.PresubmitJob, prowapi.PostsubmitJob},
		},
	}

	for _, tc := range testCases {
		// save the config
		prowConfigDir, err := ioutil.TempDir("", "prowConfig")
		if err != nil {
			t.Fatalf("fail to make tempdir: %v", err)
		}
		defer os.RemoveAll(prowConfigDir)

		prowConfig := filepath.Join(prowConfigDir, "config.yaml")
		if err := ioutil.WriteFile(prowConfig, []byte(tc.prowConfig), 0666); err != nil {
			t.Fatalf("fail to write prow config: %v", err)
		}

		cfg, err := Load(prowConfig, "")
		if tc.expectError && err == nil {
			t.Errorf("tc %s: Expect error, but got nil", tc.name)
		} else if !tc.expectError && err != nil {
			t.Errorf("tc %s: Expect no error, but got error %v", tc.name, err)
		}

		if err == nil {
			if !reflect.DeepEqual(cfg.GitHubReporter.JobTypesToReport, tc.expectTypes) {
				t.Errorf("tc %s: expected %#v\n!=\nactual %#v", tc.name, tc.expectTypes, cfg.GitHubReporter.JobTypesToReport)
			}
		}
	}
}

func TestValidRerunAuthConfig(t *testing.T) {
	var testCases = []struct {
		name        string
		prowConfig  string
		expectError bool
	}{
		{
			name: "valid rerun auth config",
			prowConfig: `
deck:
  rerun_auth_config:
    allow_anyone: false
    github_users:
    - someperson
    - someotherperson
`,
			expectError: false,
		},
		{
			name: "allow anyone and whitelist specified",
			prowConfig: `
deck:
  rerun_auth_config:
    allow_anyone: true
    github_users:
    - someperson
    - anotherperson
`,
			expectError: true,
		},
		{
			name: "empty config",
			prowConfig: `
deck:
  rerun_auth_config:
`,
			expectError: false,
		},
		{
			name: "allow anyone with empty whitelist",
			prowConfig: `
deck:
  rerun_auth_config:
    allow_anyone: true
    github_users:
`,
			expectError: false,
		},
	}

	for _, tc := range testCases {
		// save the config
		prowConfigDir, err := ioutil.TempDir("", "prowConfig")
		if err != nil {
			t.Fatalf("fail to make tempdir: %v", err)
		}
		defer os.RemoveAll(prowConfigDir)

		prowConfig := filepath.Join(prowConfigDir, "config.yaml")
		if err := ioutil.WriteFile(prowConfig, []byte(tc.prowConfig), 0666); err != nil {
			t.Fatalf("fail to write prow config: %v", err)
		}

		_, err = Load(prowConfig, "")
		if tc.expectError && err == nil {
			t.Errorf("tc %s: Expect error, but got nil", tc.name)
		} else if !tc.expectError && err != nil {
			t.Errorf("tc %s: Expect no error, but got error %v", tc.name, err)
		}
	}
}

func TestRerunAuthConfigsGetRerunAuthConfig(t *testing.T) {
	var testCases = []struct {
		name     string
		configs  RerunAuthConfigs
		refs     *prowapi.Refs
		expected prowapi.RerunAuthConfig
	}{
		{
			name:     "default to an empty config",
			configs:  RerunAuthConfigs{},
			refs:     &prowapi.Refs{Org: "my-default-org", Repo: "my-default-repo"},
			expected: prowapi.RerunAuthConfig{},
		},
		{
			name:     "unknown org or org/repo return wildcard",
			configs:  RerunAuthConfigs{"*": prowapi.RerunAuthConfig{GitHubUsers: []string{"clarketm"}}},
			refs:     &prowapi.Refs{Org: "my-default-org", Repo: "my-default-repo"},
			expected: prowapi.RerunAuthConfig{GitHubUsers: []string{"clarketm"}},
		},
		{
			name:     "no refs return wildcard",
			configs:  RerunAuthConfigs{"*": prowapi.RerunAuthConfig{GitHubUsers: []string{"leonardo"}}},
			refs:     nil,
			expected: prowapi.RerunAuthConfig{GitHubUsers: []string{"leonardo"}},
		},
		{
			name: "use org if defined",
			configs: RerunAuthConfigs{
				"*":                prowapi.RerunAuthConfig{GitHubUsers: []string{"clarketm"}},
				"istio":            prowapi.RerunAuthConfig{GitHubUsers: []string{"scoobydoo"}},
				"istio/test-infra": prowapi.RerunAuthConfig{GitHubUsers: []string{"billybob"}},
			},
			refs:     &prowapi.Refs{Org: "istio", Repo: "istio"},
			expected: prowapi.RerunAuthConfig{GitHubUsers: []string{"scoobydoo"}},
		},
		{
			name: "use org/repo if defined",
			configs: RerunAuthConfigs{
				"*":           prowapi.RerunAuthConfig{GitHubUsers: []string{"clarketm"}},
				"istio/istio": prowapi.RerunAuthConfig{GitHubUsers: []string{"skywalker"}},
			},
			refs:     &prowapi.Refs{Org: "istio", Repo: "istio"},
			expected: prowapi.RerunAuthConfig{GitHubUsers: []string{"skywalker"}},
		},
		{
			name: "org/repo takes precedence over org",
			configs: RerunAuthConfigs{
				"*":           prowapi.RerunAuthConfig{GitHubUsers: []string{"clarketm"}},
				"istio":       prowapi.RerunAuthConfig{GitHubUsers: []string{"scrappydoo"}},
				"istio/istio": prowapi.RerunAuthConfig{GitHubUsers: []string{"airbender"}},
			},
			refs:     &prowapi.Refs{Org: "istio", Repo: "istio"},
			expected: prowapi.RerunAuthConfig{GitHubUsers: []string{"airbender"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			if actual := tc.configs.GetRerunAuthConfig(tc.refs); !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("Expected %v, got %v", tc.expected, actual)
			}
		})
	}
}

func TestMergeCommitTemplateLoading(t *testing.T) {
	var testCases = []struct {
		name        string
		prowConfig  string
		expectError bool
		expect      map[string]TideMergeCommitTemplate
	}{
		{
			name: "no template",
			prowConfig: `
tide:
  merge_commit_template:
`,
			expect: nil,
		},
		{
			name: "empty template",
			prowConfig: `
tide:
  merge_commit_template:
    kubernetes/ingress:
`,
			expect: map[string]TideMergeCommitTemplate{
				"kubernetes/ingress": {},
			},
		},
		{
			name: "two proper templates",
			prowConfig: `
tide:
  merge_commit_template:
    kubernetes/ingress:
      title: "{{ .Title }}"
      body: "{{ .Body }}"
`,
			expect: map[string]TideMergeCommitTemplate{
				"kubernetes/ingress": {
					TitleTemplate: "{{ .Title }}",
					BodyTemplate:  "{{ .Body }}",
					Title:         template.Must(template.New("CommitTitle").Parse("{{ .Title }}")),
					Body:          template.Must(template.New("CommitBody").Parse("{{ .Body }}")),
				},
			},
		},
		{
			name: "only title template",
			prowConfig: `
tide:
  merge_commit_template:
    kubernetes/ingress:
      title: "{{ .Title }}"
`,
			expect: map[string]TideMergeCommitTemplate{
				"kubernetes/ingress": {
					TitleTemplate: "{{ .Title }}",
					BodyTemplate:  "",
					Title:         template.Must(template.New("CommitTitle").Parse("{{ .Title }}")),
					Body:          nil,
				},
			},
		},
		{
			name: "only body template",
			prowConfig: `
tide:
  merge_commit_template:
    kubernetes/ingress:
      body: "{{ .Body }}"
`,
			expect: map[string]TideMergeCommitTemplate{
				"kubernetes/ingress": {
					TitleTemplate: "",
					BodyTemplate:  "{{ .Body }}",
					Title:         nil,
					Body:          template.Must(template.New("CommitBody").Parse("{{ .Body }}")),
				},
			},
		},
		{
			name: "malformed title template",
			prowConfig: `
tide:
  merge_commit_template:
    kubernetes/ingress:
      title: "{{ .Title"
`,
			expectError: true,
		},
		{
			name: "malformed body template",
			prowConfig: `
tide:
  merge_commit_template:
    kubernetes/ingress:
      body: "{{ .Body"
`,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		// save the config
		prowConfigDir, err := ioutil.TempDir("", "prowConfig")
		if err != nil {
			t.Fatalf("fail to make tempdir: %v", err)
		}
		defer os.RemoveAll(prowConfigDir)

		prowConfig := filepath.Join(prowConfigDir, "config.yaml")
		if err := ioutil.WriteFile(prowConfig, []byte(tc.prowConfig), 0666); err != nil {
			t.Fatalf("fail to write prow config: %v", err)
		}

		cfg, err := Load(prowConfig, "")
		if tc.expectError && err == nil {
			t.Errorf("tc %s: Expect error, but got nil", tc.name)
		} else if !tc.expectError && err != nil {
			t.Errorf("tc %s: Expect no error, but got error %v", tc.name, err)
		}

		if err == nil {
			if !reflect.DeepEqual(cfg.Tide.MergeTemplate, tc.expect) {
				t.Errorf("tc %s: expected %#v\n!=\nactual %#v", tc.name, tc.expect, cfg.Tide.MergeTemplate)
			}
		}
	}
}

func TestPlankJobURLPrefix(t *testing.T) {
	testCases := []struct {
		name                 string
		plank                Plank
		refs                 *prowapi.Refs
		expectedJobURLPrefix string
	}{
		{
			name:                 "Nil refs returns default JobURLPrefix",
			plank:                Plank{JobURLPrefixConfig: map[string]string{"*": "https://my-prow"}},
			expectedJobURLPrefix: "https://my-prow",
		},
		{
			name: "No matching refs returns default JobURLPrefx",
			plank: Plank{
				JobURLPrefixConfig: map[string]string{
					"*":      "https://my-prow",
					"my-org": "https://my-alternate-prow",
				},
			},
			refs:                 &prowapi.Refs{Org: "my-default-org", Repo: "my-default-repo"},
			expectedJobURLPrefix: "https://my-prow",
		},
		{
			name: "Matching repo returns JobURLPrefix from repo",
			plank: Plank{
				JobURLPrefixConfig: map[string]string{
					"*":                        "https://my-prow",
					"my-alternate-org":         "https://my-third-prow",
					"my-alternate-org/my-repo": "https://my-alternate-prow",
				},
			},
			refs:                 &prowapi.Refs{Org: "my-alternate-org", Repo: "my-repo"},
			expectedJobURLPrefix: "https://my-alternate-prow",
		},
		{
			name: "Matching org and not matching repo returns JobURLPrefix from org",
			plank: Plank{
				JobURLPrefixConfig: map[string]string{
					"*":                        "https://my-prow",
					"my-alternate-org":         "https://my-third-prow",
					"my-alternate-org/my-repo": "https://my-alternate-prow",
				},
			},
			refs:                 &prowapi.Refs{Org: "my-alternate-org", Repo: "my-second-repo"},
			expectedJobURLPrefix: "https://my-third-prow",
		},
		{
			name: "Matching org without url returns default JobURLPrefix",
			plank: Plank{
				JobURLPrefixConfig: map[string]string{
					"*":                        "https://my-prow",
					"my-alternate-org/my-repo": "https://my-alternate-prow",
				},
			},
			refs:                 &prowapi.Refs{Org: "my-alternate-org", Repo: "my-second-repo"},
			expectedJobURLPrefix: "https://my-prow",
		},
		{
			name:                 "gcs/ suffix in JobURLPrefix will be automatically trimmed",
			plank:                Plank{JobURLPrefixConfig: map[string]string{"*": "https://my-prow/view/gcs/"}},
			expectedJobURLPrefix: "https://my-prow/view/",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if prefix := tc.plank.GetJobURLPrefix(tc.refs); prefix != tc.expectedJobURLPrefix {
				t.Errorf("expected JobURLPrefix to be %q but was %q", tc.expectedJobURLPrefix, prefix)
			}
		})
	}
}

func TestValidateComponentConfig(t *testing.T) {
	boolTrue := true
	boolFalse := false
	testCases := []struct {
		name        string
		config      *Config
		errExpected bool
	}{
		{
			name: "Valid default URL, no err",
			config: &Config{ProwConfig: ProwConfig{Plank: Plank{
				JobURLPrefixConfig: map[string]string{"*": "https://my-prow"}}}},
			errExpected: false,
		},
		{
			name: "Invalid default URL, err",
			config: &Config{ProwConfig: ProwConfig{Plank: Plank{
				JobURLPrefixConfig: map[string]string{"*": "https:// my-prow"}}}},
			errExpected: true,
		},
		{
			name: "Org config, valid URLs, no err",
			config: &Config{ProwConfig: ProwConfig{Plank: Plank{
				JobURLPrefixConfig: map[string]string{
					"*":      "https://my-prow",
					"my-org": "https://my-alternate-prow",
				},
			}}},
			errExpected: false,
		},
		{
			name: "Org override, invalid default jobURLPrefix URL, err",
			config: &Config{ProwConfig: ProwConfig{Plank: Plank{
				JobURLPrefixConfig: map[string]string{
					"*":      "https:// my-prow",
					"my-org": "https://my-alternate-prow",
				},
			}}},
			errExpected: true,
		},
		{
			name: "Org override, invalid org URL, err",
			config: &Config{ProwConfig: ProwConfig{Plank: Plank{
				JobURLPrefixConfig: map[string]string{
					"*":      "https://my-prow",
					"my-org": "https:// my-alternate-prow",
				},
			}}},
			errExpected: true,
		},
		{
			name: "Org override, invalid URLs, err",
			config: &Config{ProwConfig: ProwConfig{Plank: Plank{
				JobURLPrefixConfig: map[string]string{
					"*":      "https:// my-prow",
					"my-org": "https:// my-alternate-prow",
				},
			}}},
			errExpected: true,
		},
		{
			name: "Repo override, valid URLs, no err",
			config: &Config{ProwConfig: ProwConfig{Plank: Plank{
				JobURLPrefixConfig: map[string]string{
					"*":              "https://my-prow",
					"my-org":         "https://my-alternate-prow",
					"my-org/my-repo": "https://my-third-prow",
				}}}},
			errExpected: false,
		},
		{
			name: "Repo override, invalid repo URL, err",
			config: &Config{ProwConfig: ProwConfig{Plank: Plank{
				JobURLPrefixConfig: map[string]string{
					"*":              "https://my-prow",
					"my-org":         "https://my-alternate-prow",
					"my-org/my-repo": "https:// my-third-prow",
				}}}},
			errExpected: true,
		},
		{
			name: "Both RerunAuthConfig and RerunAuthConfigs are invalid, err",
			config: &Config{ProwConfig: ProwConfig{Deck: Deck{
				RerunAuthConfig:  &prowapi.RerunAuthConfig{AllowAnyone: true},
				RerunAuthConfigs: RerunAuthConfigs{"*": prowapi.RerunAuthConfig{AllowAnyone: true}},
			}}},
			errExpected: true,
		},
		{
			name: "RerunAuthConfig and not RerunAuthConfigs is valid, no err",
			config: &Config{ProwConfig: ProwConfig{Deck: Deck{
				RerunAuthConfig: &prowapi.RerunAuthConfig{AllowAnyone: false, GitHubUsers: []string{"grantsmith"}},
			}}},
			errExpected: false,
		},
		{
			name: "RerunAuthConfig only and validation fails, err",
			config: &Config{ProwConfig: ProwConfig{Deck: Deck{
				RerunAuthConfig: &prowapi.RerunAuthConfig{AllowAnyone: true, GitHubUsers: []string{"grantsmith"}},
			}}},
			errExpected: true,
		},
		{
			name: "RerunAuthConfigs and not RerunAuthConfig is valid, no err",
			config: &Config{ProwConfig: ProwConfig{Deck: Deck{
				RerunAuthConfigs: RerunAuthConfigs{
					"*":                     prowapi.RerunAuthConfig{AllowAnyone: true},
					"kubernetes":            prowapi.RerunAuthConfig{GitHubUsers: []string{"easterbunny"}},
					"kubernetes/kubernetes": prowapi.RerunAuthConfig{GitHubOrgs: []string{"kubernetes", "kubernetes-sigs"}},
				},
			}}},
			errExpected: false,
		},
		{
			name: "RerunAuthConfigs only and validation fails, err",
			config: &Config{ProwConfig: ProwConfig{Deck: Deck{
				RerunAuthConfigs: RerunAuthConfigs{
					"*":                     prowapi.RerunAuthConfig{AllowAnyone: true},
					"kubernetes":            prowapi.RerunAuthConfig{GitHubUsers: []string{"easterbunny"}},
					"kubernetes/kubernetes": prowapi.RerunAuthConfig{AllowAnyone: true, GitHubOrgs: []string{"kubernetes", "kubernetes-sigs"}},
				},
			}}},
			errExpected: true,
		},
		{
			name: "SkipStoragePathValidation true and AdditionalAllowedBuckets empty, no err",
			config: &Config{ProwConfig: ProwConfig{Deck: Deck{
				SkipStoragePathValidation: &boolTrue,
				AdditionalAllowedBuckets:  []string{},
			}}},
			errExpected: false,
		},
		{
			name: "SkipStoragePathValidation true and AdditionalAllowedBuckets non-empty, err",
			config: &Config{ProwConfig: ProwConfig{Deck: Deck{
				SkipStoragePathValidation: &boolTrue,
				AdditionalAllowedBuckets: []string{
					"foo",
					"bar",
				},
			}}},
			errExpected: true,
		},
		{
			name: "SkipStoragePathValidation false and AdditionalAllowedBuckets non-empty, no err",
			config: &Config{ProwConfig: ProwConfig{Deck: Deck{
				SkipStoragePathValidation: &boolFalse,
				AdditionalAllowedBuckets: []string{
					"foo",
					"bar",
				},
			}}},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if hasErr := tc.config.validateComponentConfig() != nil; hasErr != tc.errExpected {
				t.Errorf("expected err: %t but was %t", tc.errExpected, hasErr)
			}
		})
	}
}

func TestSlackReporterValidation(t *testing.T) {
	testCases := []struct {
		name            string
		config          func() Config
		successExpected bool
	}{
		{
			name: "Valid config w/ slack_reporter - no error",
			config: func() Config {
				slack := &SlackReporter{
					Channel: "my-channel",
				}
				return Config{
					ProwConfig: ProwConfig{
						SlackReporter: slack,
					},
				}
			},
			successExpected: true,
		},
		{
			name: "Valid config w/ wildcard slack_reporter_configs - no error",
			config: func() Config {
				slackCfg := map[string]SlackReporter{
					"*": {
						Channel: "my-channel",
					},
				}
				return Config{
					ProwConfig: ProwConfig{
						SlackReporterConfigs: slackCfg,
					},
				}
			},
			successExpected: true,
		},
		{
			name: "Valid config w/ org/repo slack_reporter_configs - no error",
			config: func() Config {
				slackCfg := map[string]SlackReporter{
					"istio/proxy": {
						Channel: "my-channel",
					},
				}
				return Config{
					ProwConfig: ProwConfig{
						SlackReporterConfigs: slackCfg,
					},
				}
			},
			successExpected: true,
		},
		{
			name: "Valid config w/ repo slack_reporter_configs - no error",
			config: func() Config {
				slackCfg := map[string]SlackReporter{
					"proxy": {
						Channel: "my-channel",
					},
				}
				return Config{
					ProwConfig: ProwConfig{
						SlackReporterConfigs: slackCfg,
					},
				}
			},
			successExpected: true,
		},
		{
			name: "Invalid config b/c both slack_reporter and slack_reporter_configs - error",
			config: func() Config {
				slack := &SlackReporter{
					Channel: "my-channel",
				}
				slackCfg := map[string]SlackReporter{
					"*": {
						Channel: "my-channel",
					},
				}
				return Config{
					ProwConfig: ProwConfig{
						SlackReporter:        slack,
						SlackReporterConfigs: slackCfg,
					},
				}
			},
			successExpected: false,
		},
		{
			name: "No channel w/ slack_reporter - error",
			config: func() Config {
				slack := &SlackReporter{}
				return Config{
					ProwConfig: ProwConfig{
						SlackReporter: slack,
					},
				}
			},
			successExpected: false,
		},
		{
			name: "No channel w/ slack_reporter_configs - error",
			config: func() Config {
				slackCfg := map[string]SlackReporter{
					"*": {
						JobTypesToReport: []prowapi.ProwJobType{"presubmit"},
					},
				}
				return Config{
					ProwConfig: ProwConfig{
						SlackReporterConfigs: slackCfg,
					},
				}
			},
			successExpected: false,
		},
		{
			name: "Empty config - no error",
			config: func() Config {
				slackCfg := map[string]SlackReporter{}
				return Config{
					ProwConfig: ProwConfig{
						SlackReporterConfigs: slackCfg,
					},
				}
			},
			successExpected: true,
		},
		{
			name: "Invalid template - error",
			config: func() Config {
				slackCfg := map[string]SlackReporter{
					"*": {
						Channel:        "my-channel",
						ReportTemplate: "{{ if .Spec.Name}}",
					},
				}
				return Config{
					ProwConfig: ProwConfig{
						SlackReporterConfigs: slackCfg,
					},
				}
			},
			successExpected: false,
		},
		{
			name: "Template accessed invalid property - error",
			config: func() Config {
				slackCfg := map[string]SlackReporter{
					"*": {
						Channel:        "my-channel",
						ReportTemplate: "{{ .Undef}}",
					},
				}
				return Config{
					ProwConfig: ProwConfig{
						SlackReporterConfigs: slackCfg,
					},
				}
			},
			successExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.config()
			if err := cfg.validateComponentConfig(); (err == nil) != tc.successExpected {
				t.Errorf("Expected success=%t but got err=%v", tc.successExpected, err)
			}
			if tc.successExpected {
				for _, config := range cfg.SlackReporterConfigs {
					if config.ReportTemplate == "" {
						t.Errorf("expected default ReportTemplate to be set")
					}
					if config.Channel == "" {
						t.Errorf("expected Channel to be required")
					}
				}
			}
		})
	}
}
func TestManagedHmacEntityValidation(t *testing.T) {
	testCases := []struct {
		name       string
		prowConfig Config
		shouldFail bool
	}{
		{
			name:       "Missing managed HmacEntities",
			prowConfig: Config{ProwConfig: ProwConfig{ManagedWebhooks: ManagedWebhooks{}}},
			shouldFail: false,
		},
		{
			name: "Config with all valid dates",
			prowConfig: Config{ProwConfig: ProwConfig{
				ManagedWebhooks: ManagedWebhooks{
					OrgRepoConfig: map[string]ManagedWebhookInfo{
						"foo/bar": {TokenCreatedAfter: time.Now()},
						"foo/baz": {TokenCreatedAfter: time.Now()},
					},
				},
			}},
			shouldFail: false,
		},
		{
			name: "Config with one invalid dates",
			prowConfig: Config{ProwConfig: ProwConfig{
				ManagedWebhooks: ManagedWebhooks{
					OrgRepoConfig: map[string]ManagedWebhookInfo{
						"foo/bar": {TokenCreatedAfter: time.Now()},
						"foo/baz": {TokenCreatedAfter: time.Now().Add(time.Hour)},
					},
				},
			}},
			shouldFail: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := tc.prowConfig.validateComponentConfig()
			if tc.shouldFail != (err != nil) {
				t.Errorf("%s: Unexpected outcome. Error expected %v, Error found %s", tc.name, tc.shouldFail, err)
			}

		})
	}
}
func TestValidateTriggering(t *testing.T) {
	testCases := []struct {
		name        string
		presubmit   Presubmit
		errExpected bool
	}{
		{
			name: "Trigger set, rerun command unset, err",
			presubmit: Presubmit{
				Trigger: "my-trigger",
				Reporter: Reporter{
					Context: "my-context",
				},
			},
			errExpected: true,
		},
		{
			name: "Triger unset, rerun command set, err",
			presubmit: Presubmit{
				RerunCommand: "my-rerun-command",
				Reporter: Reporter{
					Context: "my-context",
				},
			},
			errExpected: true,
		},
		{
			name: "Both trigger and rerun command set, no err",
			presubmit: Presubmit{
				Trigger:      "my-trigger",
				RerunCommand: "my-rerun-command",
				Reporter: Reporter{
					Context: "my-context",
				},
			},
			errExpected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTriggering(tc.presubmit)
			if err != nil != tc.errExpected {
				t.Errorf("Expected err: %t but got err %v", tc.errExpected, err)
			}
		})
	}
}

func TestRefGetterForGitHubPullRequest(t *testing.T) {
	testCases := []struct {
		name   string
		rg     *RefGetterForGitHubPullRequest
		verify func(*RefGetterForGitHubPullRequest) error
	}{
		{
			name: "Existing PullRequest is returned",
			rg:   &RefGetterForGitHubPullRequest{pr: &github.PullRequest{ID: 123456}},
			verify: func(rg *RefGetterForGitHubPullRequest) error {
				if rg.pr == nil || rg.pr.ID != 123456 {
					return fmt.Errorf("Expected refGetter to contain pr with id 123456, pr was %v", rg.pr)
				}
				return nil
			},
		},
		{
			name: "PullRequest is fetched, stored and returned",
			rg: &RefGetterForGitHubPullRequest{
				ghc: &fakegithub.FakeClient{
					PullRequests: map[int]*github.PullRequest{0: {ID: 123456}}},
			},
			verify: func(rg *RefGetterForGitHubPullRequest) error {
				pr, err := rg.PullRequest()
				if err != nil {
					return fmt.Errorf("failed to fetch PullRequest: %v", err)
				}
				if rg.pr == nil || rg.pr.ID != 123456 {
					return fmt.Errorf("expected agent to contain pr with id 123456, pr was %v", rg.pr)
				}
				if pr.ID != 123456 {
					return fmt.Errorf("expected returned pr.ID to be 123456, was %d", pr.ID)
				}
				return nil
			},
		},
		{
			name: "Existing baseSHA is returned",
			rg:   &RefGetterForGitHubPullRequest{baseSHA: "12345", pr: &github.PullRequest{}},
			verify: func(rg *RefGetterForGitHubPullRequest) error {
				baseSHA, err := rg.BaseSHA()
				if err != nil {
					return fmt.Errorf("error calling baseSHA: %v", err)
				}
				if rg.baseSHA != "12345" {
					return fmt.Errorf("expected agent baseSHA to be 12345, was %q", rg.baseSHA)
				}
				if baseSHA != "12345" {
					return fmt.Errorf("expected returned baseSHA to be 12345, was %q", baseSHA)
				}
				return nil
			},
		},
		{
			name: "BaseSHA is fetched, stored and returned",
			rg: &RefGetterForGitHubPullRequest{
				ghc: &fakegithub.FakeClient{
					PullRequests: map[int]*github.PullRequest{0: {}},
				},
			},
			verify: func(rg *RefGetterForGitHubPullRequest) error {
				baseSHA, err := rg.BaseSHA()
				if err != nil {
					return fmt.Errorf("expected err to be nil, was %v", err)
				}
				if rg.baseSHA != fakegithub.TestRef {
					return fmt.Errorf("expected baseSHA on agent to be %q, was %q", fakegithub.TestRef, rg.baseSHA)
				}
				if baseSHA != fakegithub.TestRef {
					return fmt.Errorf("expected returned baseSHA to be %q, was %q", fakegithub.TestRef, baseSHA)
				}
				return nil
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.rg.lock = &sync.Mutex{}
			if err := tc.verify(tc.rg); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSetDecorationDefaults(t *testing.T) {
	yes := true
	no := false
	testCases := []struct {
		id            string
		repo          string
		config        *Config
		utilityConfig UtilityConfig
		expected      *prowapi.DecorationConfig
	}{
		{
			id:            "no dc in presubmit or in plank's config, expect no changes",
			utilityConfig: UtilityConfig{Decorate: &yes},
			config:        &Config{ProwConfig: ProwConfig{}},
			expected:      nil,
		},
		{
			id:            "no dc in presubmit or in plank's by repo config, expect plank's defaults",
			utilityConfig: UtilityConfig{Decorate: &yes},
			config: &Config{
				ProwConfig: ProwConfig{
					Plank: Plank{
						DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
							"*": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test",
									InitUpload: "initupload:test",
									Entrypoint: "entrypoint:test",
									Sidecar:    "sidecar:test",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket",
									PathStrategy: "single",
									DefaultOrg:   "org",
									DefaultRepo:  "repo",
								},
								GCSCredentialsSecret: "credentials-gcs",
							},
						},
					},
				},
			},
			expected: &prowapi.DecorationConfig{
				UtilityImages: &prowapi.UtilityImages{
					CloneRefs:  "clonerefs:test",
					InitUpload: "initupload:test",
					Entrypoint: "entrypoint:test",
					Sidecar:    "sidecar:test",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "test-bucket",
					PathStrategy: "single",
					DefaultOrg:   "org",
					DefaultRepo:  "repo",
				},
				GCSCredentialsSecret: "credentials-gcs",
			},
		},
		{
			id:            "no dc in presubmit, part of plank's by repo config, expect merged by repo config and defaults",
			utilityConfig: UtilityConfig{Decorate: &yes},
			repo:          "org/repo",
			config: &Config{
				ProwConfig: ProwConfig{
					Plank: Plank{
						DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
							"*": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test",
									InitUpload: "initupload:test",
									Entrypoint: "entrypoint:test",
									Sidecar:    "sidecar:test",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket",
									PathStrategy: "single",
									DefaultOrg:   "org",
									DefaultRepo:  "repo",
								},
								GCSCredentialsSecret: "credentials-gcs",
							},
							"org/repo": {
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket-by-repo",
									PathStrategy: "single-by-repo",
									DefaultOrg:   "org-by-repo",
									DefaultRepo:  "repo-by-repo",
								},
							},
						},
					},
				},
			},
			expected: &prowapi.DecorationConfig{
				UtilityImages: &prowapi.UtilityImages{
					CloneRefs:  "clonerefs:test",
					InitUpload: "initupload:test",
					Entrypoint: "entrypoint:test",
					Sidecar:    "sidecar:test",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "test-bucket-by-repo",
					PathStrategy: "single-by-repo",
					DefaultOrg:   "org-by-repo",
					DefaultRepo:  "repo-by-repo",
				},
				GCSCredentialsSecret: "credentials-gcs",
			},
		},
		{
			id:   "dc in presubmit and plank's defaults, expect presubmit's dc",
			repo: "org/repo",
			utilityConfig: UtilityConfig{
				Decorate: &yes,
				DecorationConfig: &prowapi.DecorationConfig{
					UtilityImages: &prowapi.UtilityImages{
						CloneRefs:  "clonerefs:test-from-ps",
						InitUpload: "initupload:test-from-ps",
						Entrypoint: "entrypoint:test-from-ps",
						Sidecar:    "sidecar:test-from-ps",
					},
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket:       "test-bucket-from-ps",
						PathStrategy: "single-from-ps",
						DefaultOrg:   "org-from-ps",
						DefaultRepo:  "repo-from-ps",
					},
					GCSCredentialsSecret: "credentials-gcs-from-ps",
				},
			},
			config: &Config{
				ProwConfig: ProwConfig{
					Plank: Plank{
						DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
							"*": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test",
									InitUpload: "initupload:test",
									Entrypoint: "entrypoint:test",
									Sidecar:    "sidecar:test",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket",
									PathStrategy: "single",
									DefaultOrg:   "org",
									DefaultRepo:  "repo",
								},
								GCSCredentialsSecret: "credentials-gcs",
							},
						},
					},
				},
			},
			expected: &prowapi.DecorationConfig{
				UtilityImages: &prowapi.UtilityImages{
					CloneRefs:  "clonerefs:test-from-ps",
					InitUpload: "initupload:test-from-ps",
					Entrypoint: "entrypoint:test-from-ps",
					Sidecar:    "sidecar:test-from-ps",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "test-bucket-from-ps",
					PathStrategy: "single-from-ps",
					DefaultOrg:   "org-from-ps",
					DefaultRepo:  "repo-from-ps",
				},
				GCSCredentialsSecret: "credentials-gcs-from-ps",
			},
		},
		{
			id:   "dc in presubmit, plank's by repo config and defaults, expected presubmit's dc",
			repo: "org/repo",
			utilityConfig: UtilityConfig{
				Decorate: &yes,
				DecorationConfig: &prowapi.DecorationConfig{
					UtilityImages: &prowapi.UtilityImages{
						CloneRefs:  "clonerefs:test-from-ps",
						InitUpload: "initupload:test-from-ps",
						Entrypoint: "entrypoint:test-from-ps",
						Sidecar:    "sidecar:test-from-ps",
					},
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket:       "test-bucket-from-ps",
						PathStrategy: "single-from-ps",
						DefaultOrg:   "org-from-ps",
						DefaultRepo:  "repo-from-ps",
					},
					GCSCredentialsSecret: "credentials-gcs-from-ps",
				},
			},
			config: &Config{
				ProwConfig: ProwConfig{
					Plank: Plank{
						DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
							"*": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test",
									InitUpload: "initupload:test",
									Entrypoint: "entrypoint:test",
									Sidecar:    "sidecar:test",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket",
									PathStrategy: "single",
									DefaultOrg:   "org",
									DefaultRepo:  "repo",
								},
								GCSCredentialsSecret: "credentials-gcs",
							},
							"org/repo": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test-by-repo",
									InitUpload: "initupload:test-by-repo",
									Entrypoint: "entrypoint:test-by-repo",
									Sidecar:    "sidecar:test-by-repo",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket-by-repo",
									PathStrategy: "single",
									DefaultOrg:   "org-test",
									DefaultRepo:  "repo-test",
								},
								GCSCredentialsSecret: "credentials-gcs",
							},
						},
					},
				},
			},
			expected: &prowapi.DecorationConfig{
				UtilityImages: &prowapi.UtilityImages{
					CloneRefs:  "clonerefs:test-from-ps",
					InitUpload: "initupload:test-from-ps",
					Entrypoint: "entrypoint:test-from-ps",
					Sidecar:    "sidecar:test-from-ps",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "test-bucket-from-ps",
					PathStrategy: "single-from-ps",
					DefaultOrg:   "org-from-ps",
					DefaultRepo:  "repo-from-ps",
				},
				GCSCredentialsSecret: "credentials-gcs-from-ps",
			},
		},
		{
			id:            "no dc in presubmit, dc in plank's by repo config and defaults, expect by repo config's dc",
			repo:          "org/repo",
			utilityConfig: UtilityConfig{Decorate: &yes},
			config: &Config{
				ProwConfig: ProwConfig{
					Plank: Plank{
						DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
							"*": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test",
									InitUpload: "initupload:test",
									Entrypoint: "entrypoint:test",
									Sidecar:    "sidecar:test",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket",
									PathStrategy: "single",
									DefaultOrg:   "org",
									DefaultRepo:  "repo",
								},
								GCSCredentialsSecret: "credentials-gcs",
							},
							"org/repo": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test-by-repo",
									InitUpload: "initupload:test-by-repo",
									Entrypoint: "entrypoint:test-by-repo",
									Sidecar:    "sidecar:test-by-repo",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket-by-repo",
									PathStrategy: "single-by-repo",
									DefaultOrg:   "org-by-repo",
									DefaultRepo:  "repo-by-repo",
								},
								GCSCredentialsSecret: "credentials-gcs-by-repo",
							},
						},
					},
				},
			},
			expected: &prowapi.DecorationConfig{
				UtilityImages: &prowapi.UtilityImages{
					CloneRefs:  "clonerefs:test-by-repo",
					InitUpload: "initupload:test-by-repo",
					Entrypoint: "entrypoint:test-by-repo",
					Sidecar:    "sidecar:test-by-repo",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "test-bucket-by-repo",
					PathStrategy: "single-by-repo",
					DefaultOrg:   "org-by-repo",
					DefaultRepo:  "repo-by-repo",
				},
				GCSCredentialsSecret: "credentials-gcs-by-repo",
			},
		},
		{
			id:            "no dc in presubmit, dc in plank's by repo config and defaults, expect by org config's dc",
			repo:          "org/repo",
			utilityConfig: UtilityConfig{Decorate: &yes},
			config: &Config{
				ProwConfig: ProwConfig{
					Plank: Plank{
						DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
							"*": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test",
									InitUpload: "initupload:test",
									Entrypoint: "entrypoint:test",
									Sidecar:    "sidecar:test",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket",
									PathStrategy: "single",
									DefaultOrg:   "org",
									DefaultRepo:  "repo",
								},
								GCSCredentialsSecret: "credentials-gcs",
							},
							"org": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test-by-org",
									InitUpload: "initupload:test-by-org",
									Entrypoint: "entrypoint:test-by-org",
									Sidecar:    "sidecar:test-by-org",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket-by-org",
									PathStrategy: "single-by-org",
									DefaultOrg:   "org-by-org",
									DefaultRepo:  "repo-by-org",
								},
								GCSCredentialsSecret: "credentials-gcs-by-org",
							},
						},
					},
				},
			},
			expected: &prowapi.DecorationConfig{
				UtilityImages: &prowapi.UtilityImages{
					CloneRefs:  "clonerefs:test-by-org",
					InitUpload: "initupload:test-by-org",
					Entrypoint: "entrypoint:test-by-org",
					Sidecar:    "sidecar:test-by-org",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "test-bucket-by-org",
					PathStrategy: "single-by-org",
					DefaultOrg:   "org-by-org",
					DefaultRepo:  "repo-by-org",
				},
				GCSCredentialsSecret: "credentials-gcs-by-org",
			},
		},
		{
			id:            "no dc in presubmit, dc in plank's by repo config and defaults, expect by * config's dc",
			repo:          "org/repo",
			utilityConfig: UtilityConfig{Decorate: &yes},
			config: &Config{
				ProwConfig: ProwConfig{
					Plank: Plank{
						DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
							"*": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test-by-*",
									InitUpload: "initupload:test-by-*",
									Entrypoint: "entrypoint:test-by-*",
									Sidecar:    "sidecar:test-by-*",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket-by-*",
									PathStrategy: "single-by-*",
									DefaultOrg:   "org-by-*",
									DefaultRepo:  "repo-by-*",
								},
								GCSCredentialsSecret: "credentials-gcs-by-*",
							},
						},
					},
				},
			},
			expected: &prowapi.DecorationConfig{
				UtilityImages: &prowapi.UtilityImages{
					CloneRefs:  "clonerefs:test-by-*",
					InitUpload: "initupload:test-by-*",
					Entrypoint: "entrypoint:test-by-*",
					Sidecar:    "sidecar:test-by-*",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "test-bucket-by-*",
					PathStrategy: "single-by-*",
					DefaultOrg:   "org-by-*",
					DefaultRepo:  "repo-by-*",
				},
				GCSCredentialsSecret: "credentials-gcs-by-*",
			},
		},

		{
			id:            "no dc in presubmit, dc in plank's by repo config org and org/repo co-exists, expect by org/repo config's dc",
			repo:          "org/repo",
			utilityConfig: UtilityConfig{Decorate: &yes},
			config: &Config{
				ProwConfig: ProwConfig{
					Plank: Plank{
						DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
							"*": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test-by-*",
									InitUpload: "initupload:test-by-*",
									Entrypoint: "entrypoint:test-by-*",
									Sidecar:    "sidecar:test-by-*",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket-by-*",
									PathStrategy: "single-by-*",
									DefaultOrg:   "org-by-*",
									DefaultRepo:  "repo-by-*",
								},
								GCSCredentialsSecret: "credentials-gcs-by-*",
							},
							"org": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test-by-org",
									InitUpload: "initupload:test-by-org",
									Entrypoint: "entrypoint:test-by-org",
									Sidecar:    "sidecar:test-by-org",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket-by-org",
									PathStrategy: "single-by-org",
									DefaultOrg:   "org-by-org",
									DefaultRepo:  "repo-by-org",
								},
								GCSCredentialsSecret: "credentials-gcs-by-org",
							},
							"org/repo": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test-by-org-repo",
									InitUpload: "initupload:test-by-org-repo",
									Entrypoint: "entrypoint:test-by-org-repo",
									Sidecar:    "sidecar:test-by-org-repo",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket-by-org-repo",
									PathStrategy: "single-by-org-repo",
									DefaultOrg:   "org-by-org-repo",
									DefaultRepo:  "repo-by-org-repo",
								},
								GCSCredentialsSecret: "credentials-gcs-by-org-repo",
							},
						},
					},
				},
			},
			expected: &prowapi.DecorationConfig{
				UtilityImages: &prowapi.UtilityImages{
					CloneRefs:  "clonerefs:test-by-org-repo",
					InitUpload: "initupload:test-by-org-repo",
					Entrypoint: "entrypoint:test-by-org-repo",
					Sidecar:    "sidecar:test-by-org-repo",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "test-bucket-by-org-repo",
					PathStrategy: "single-by-org-repo",
					DefaultOrg:   "org-by-org-repo",
					DefaultRepo:  "repo-by-org-repo",
				},
				GCSCredentialsSecret: "credentials-gcs-by-org-repo",
			},
		},

		{
			id:            "no dc in presubmit, dc in plank's by repo config with org and * to co-exists, expect by 'org' config's dc",
			repo:          "org/repo",
			utilityConfig: UtilityConfig{Decorate: &yes},
			config: &Config{
				ProwConfig: ProwConfig{
					Plank: Plank{
						DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
							"*": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test-by-*",
									InitUpload: "initupload:test-by-*",
									Entrypoint: "entrypoint:test-by-*",
									Sidecar:    "sidecar:test-by-*",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket-by-*",
									PathStrategy: "single-by-*",
									DefaultOrg:   "org-by-*",
									DefaultRepo:  "repo-by-*",
								},
								GCSCredentialsSecret: "credentials-gcs-by-*",
							},
							"org": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test-by-org",
									InitUpload: "initupload:test-by-org",
									Entrypoint: "entrypoint:test-by-org",
									Sidecar:    "sidecar:test-by-org",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket-by-org",
									PathStrategy: "single-by-org",
									DefaultOrg:   "org-by-org",
									DefaultRepo:  "repo-by-org",
								},
								GCSCredentialsSecret: "credentials-gcs-by-org",
							},
						},
					},
				},
			},
			expected: &prowapi.DecorationConfig{
				UtilityImages: &prowapi.UtilityImages{
					CloneRefs:  "clonerefs:test-by-org",
					InitUpload: "initupload:test-by-org",
					Entrypoint: "entrypoint:test-by-org",
					Sidecar:    "sidecar:test-by-org",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "test-bucket-by-org",
					PathStrategy: "single-by-org",
					DefaultOrg:   "org-by-org",
					DefaultRepo:  "repo-by-org",
				},
				GCSCredentialsSecret: "credentials-gcs-by-org",
			},
		},
		{
			id: "decorate_all_jobs set, no dc in presubmit or in plank's by repo config, expect plank's defaults",
			config: &Config{
				JobConfig: JobConfig{
					DecorateAllJobs: true,
				},
				ProwConfig: ProwConfig{
					Plank: Plank{
						DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
							"*": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test",
									InitUpload: "initupload:test",
									Entrypoint: "entrypoint:test",
									Sidecar:    "sidecar:test",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket",
									PathStrategy: "single",
									DefaultOrg:   "org",
									DefaultRepo:  "repo",
								},
								GCSCredentialsSecret: "credentials-gcs",
							},
						},
					},
				},
			},
			expected: &prowapi.DecorationConfig{
				UtilityImages: &prowapi.UtilityImages{
					CloneRefs:  "clonerefs:test",
					InitUpload: "initupload:test",
					Entrypoint: "entrypoint:test",
					Sidecar:    "sidecar:test",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "test-bucket",
					PathStrategy: "single",
					DefaultOrg:   "org",
					DefaultRepo:  "repo",
				},
				GCSCredentialsSecret: "credentials-gcs",
			},
		},
		{
			id:            "opt out of decorate_all_jobs by setting decorated to false",
			utilityConfig: UtilityConfig{Decorate: &no},
			config: &Config{
				JobConfig: JobConfig{
					DecorateAllJobs: true,
				},
				ProwConfig: ProwConfig{
					Plank: Plank{
						DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
							"*": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test",
									InitUpload: "initupload:test",
									Entrypoint: "entrypoint:test",
									Sidecar:    "sidecar:test",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket",
									PathStrategy: "single",
									DefaultOrg:   "org",
									DefaultRepo:  "repo",
								},
								GCSCredentialsSecret: "credentials-gcs",
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			presubmit := &Presubmit{JobBase: JobBase{UtilityConfig: tc.utilityConfig}}
			postsubmit := &Postsubmit{JobBase: JobBase{UtilityConfig: tc.utilityConfig}}

			setPresubmitDecorationDefaults(tc.config, presubmit, tc.repo)
			if diff := cmp.Diff(presubmit.DecorationConfig, tc.expected, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("presubmit: %s", diff)
			}

			setPostsubmitDecorationDefaults(tc.config, postsubmit, tc.repo)
			if diff := cmp.Diff(postsubmit.DecorationConfig, tc.expected, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("postsubmit: %s", diff)
			}
		})
	}
}

func TestSetPeriodicDecorationDefaults(t *testing.T) {
	yes := true
	no := false
	testCases := []struct {
		id            string
		config        *Config
		utilityConfig UtilityConfig
		expected      *prowapi.DecorationConfig
	}{
		{
			id: "extraRefs[0] not defined, changes from defaultDecorationConfigs[*] expected",
			config: &Config{
				ProwConfig: ProwConfig{
					Plank: Plank{
						DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
							"*": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test-by-*",
									InitUpload: "initupload:test-by-*",
									Entrypoint: "entrypoint:test-by-*",
									Sidecar:    "sidecar:test-by-*",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket-by-*",
									PathStrategy: "single-by-*",
									DefaultOrg:   "org-by-*",
									DefaultRepo:  "repo-by-*",
								},
								GCSCredentialsSecret: "credentials-gcs-by-*",
							},
						},
					},
				},
			},
			utilityConfig: UtilityConfig{Decorate: &yes},
			expected: &prowapi.DecorationConfig{
				UtilityImages: &prowapi.UtilityImages{
					CloneRefs:  "clonerefs:test-by-*",
					InitUpload: "initupload:test-by-*",
					Entrypoint: "entrypoint:test-by-*",
					Sidecar:    "sidecar:test-by-*",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "test-bucket-by-*",
					PathStrategy: "single-by-*",
					DefaultOrg:   "org-by-*",
					DefaultRepo:  "repo-by-*",
				},
				GCSCredentialsSecret: "credentials-gcs-by-*",
			},
		},
		{
			id: "extraRefs[0] defined, only 'org` exists in config, changes from defaultDecorationConfigs[org] expected",
			config: &Config{
				ProwConfig: ProwConfig{
					Plank: Plank{
						DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
							"*": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test-by-*",
									InitUpload: "initupload:test-by-*",
									Entrypoint: "entrypoint:test-by-*",
									Sidecar:    "sidecar:test-by-*",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket-by-*",
									PathStrategy: "single-by-*",
									DefaultOrg:   "org-by-*",
									DefaultRepo:  "repo-by-*",
								},
								GCSCredentialsSecret: "credentials-gcs-by-*",
							},
							"org": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test-by-org",
									InitUpload: "initupload:test-by-org",
									Entrypoint: "entrypoint:test-by-org",
									Sidecar:    "sidecar:test-by-org",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket-by-org",
									PathStrategy: "single-by-org",
									DefaultOrg:   "org-by-org",
									DefaultRepo:  "repo-by-org",
								},
								GCSCredentialsSecret: "credentials-gcs-by-org",
							},
						},
					},
				},
			},
			utilityConfig: UtilityConfig{
				Decorate: &yes,
				ExtraRefs: []prowapi.Refs{
					{
						Org:  "org",
						Repo: "repo",
					},
				},
			},
			expected: &prowapi.DecorationConfig{
				UtilityImages: &prowapi.UtilityImages{
					CloneRefs:  "clonerefs:test-by-org",
					InitUpload: "initupload:test-by-org",
					Entrypoint: "entrypoint:test-by-org",
					Sidecar:    "sidecar:test-by-org",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "test-bucket-by-org",
					PathStrategy: "single-by-org",
					DefaultOrg:   "org-by-org",
					DefaultRepo:  "repo-by-org",
				},
				GCSCredentialsSecret: "credentials-gcs-by-org",
			},
		},
		{
			id: "extraRefs[0] defined and org/repo of defaultDecorationConfigs exists, changes from defaultDecorationConfigs[org/repo] expected",
			config: &Config{
				ProwConfig: ProwConfig{
					Plank: Plank{
						DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
							"*": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test-by-*",
									InitUpload: "initupload:test-by-*",
									Entrypoint: "entrypoint:test-by-*",
									Sidecar:    "sidecar:test-by-*",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket-by-*",
									PathStrategy: "single-by-*",
									DefaultOrg:   "org-by-*",
									DefaultRepo:  "repo-by-*",
								},
								GCSCredentialsSecret: "credentials-gcs-by-*",
							},
							"org/repo": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test-by-org-repo",
									InitUpload: "initupload:test-by-org-repo",
									Entrypoint: "entrypoint:test-by-org-repo",
									Sidecar:    "sidecar:test-by-org-repo",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket-by-org-repo",
									PathStrategy: "single-by-org-repo",
									DefaultOrg:   "org-by-org-repo",
									DefaultRepo:  "repo-by-org-repo",
								},
								GCSCredentialsSecret: "credentials-gcs-by-org-repo",
							},
						},
					},
				},
			},
			utilityConfig: UtilityConfig{
				Decorate: &yes,
				ExtraRefs: []prowapi.Refs{
					{
						Org:  "org",
						Repo: "repo",
					},
				},
			},
			expected: &prowapi.DecorationConfig{
				UtilityImages: &prowapi.UtilityImages{
					CloneRefs:  "clonerefs:test-by-org-repo",
					InitUpload: "initupload:test-by-org-repo",
					Entrypoint: "entrypoint:test-by-org-repo",
					Sidecar:    "sidecar:test-by-org-repo",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "test-bucket-by-org-repo",
					PathStrategy: "single-by-org-repo",
					DefaultOrg:   "org-by-org-repo",
					DefaultRepo:  "repo-by-org-repo",
				},
				GCSCredentialsSecret: "credentials-gcs-by-org-repo",
			},
		},
		{
			id: "decorate_all_jobs set, plank's default decoration config expected",
			config: &Config{
				JobConfig: JobConfig{
					DecorateAllJobs: true,
				},
				ProwConfig: ProwConfig{
					Plank: Plank{
						DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
							"*": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test-by-*",
									InitUpload: "initupload:test-by-*",
									Entrypoint: "entrypoint:test-by-*",
									Sidecar:    "sidecar:test-by-*",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket-by-*",
									PathStrategy: "single-by-*",
									DefaultOrg:   "org-by-*",
									DefaultRepo:  "repo-by-*",
								},
								GCSCredentialsSecret: "credentials-gcs-by-*",
							},
						},
					},
				},
			},
			expected: &prowapi.DecorationConfig{
				UtilityImages: &prowapi.UtilityImages{
					CloneRefs:  "clonerefs:test-by-*",
					InitUpload: "initupload:test-by-*",
					Entrypoint: "entrypoint:test-by-*",
					Sidecar:    "sidecar:test-by-*",
				},
				GCSConfiguration: &prowapi.GCSConfiguration{
					Bucket:       "test-bucket-by-*",
					PathStrategy: "single-by-*",
					DefaultOrg:   "org-by-*",
					DefaultRepo:  "repo-by-*",
				},
				GCSCredentialsSecret: "credentials-gcs-by-*",
			},
		},
		{
			id:            "opt out of decorate_all_jobs by specifying undecorated",
			utilityConfig: UtilityConfig{Decorate: &no},
			config: &Config{
				JobConfig: JobConfig{
					DecorateAllJobs: true,
				},
				ProwConfig: ProwConfig{
					Plank: Plank{
						DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
							"*": {
								UtilityImages: &prowapi.UtilityImages{
									CloneRefs:  "clonerefs:test-by-*",
									InitUpload: "initupload:test-by-*",
									Entrypoint: "entrypoint:test-by-*",
									Sidecar:    "sidecar:test-by-*",
								},
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "test-bucket-by-*",
									PathStrategy: "single-by-*",
									DefaultOrg:   "org-by-*",
									DefaultRepo:  "repo-by-*",
								},
								GCSCredentialsSecret: "credentials-gcs-by-*",
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			periodic := &Periodic{JobBase: JobBase{UtilityConfig: tc.utilityConfig}}
			setPeriodicDecorationDefaults(tc.config, periodic)
			if diff := cmp.Diff(periodic.DecorationConfig, tc.expected, cmpopts.EquateEmpty()); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestDecorationRequested(t *testing.T) {
	yes := true
	no := false
	testCases := []struct {
		name        string
		decorateAll bool
		presubmits  map[string][]Presubmit
		postsubmits map[string][]Postsubmit
		periodics   []Periodic
		expected    bool
	}{
		{
			name:        "decorate_all_jobs set",
			decorateAll: true,
			presubmits: map[string][]Presubmit{
				"org/repo": {
					{JobBase: JobBase{Name: "presubmit-job"}},
				},
			},
			expected: true,
		},
		{
			name: "at-least one job is decorated",
			presubmits: map[string][]Presubmit{
				"org/repo": {
					{JobBase: JobBase{Name: "presubmit-job"}},
				},
			},
			postsubmits: map[string][]Postsubmit{
				"org/repo": {
					{JobBase: JobBase{UtilityConfig: UtilityConfig{Decorate: &yes}}},
				},
			},
			expected: true,
		},
		{
			name:        "decorate_all_jobs set, at-least one job does not opt out",
			decorateAll: true,
			presubmits: map[string][]Presubmit{
				"org/repo": {
					{JobBase: JobBase{UtilityConfig: UtilityConfig{Decorate: &no}}},
				},
			},
			postsubmits: map[string][]Postsubmit{
				"org/repo": {
					{JobBase: JobBase{UtilityConfig: UtilityConfig{Decorate: &no}}},
				},
			},
			periodics: []Periodic{
				{JobBase: JobBase{Name: "periodic-job"}},
			},
			expected: true,
		},
		{
			name: "decorate_all_jobs set, all jobs opt out",
			presubmits: map[string][]Presubmit{
				"org/repo": {
					{JobBase: JobBase{UtilityConfig: UtilityConfig{Decorate: &no}}},
				},
			},
			postsubmits: map[string][]Postsubmit{
				"org/repo": {
					{JobBase: JobBase{UtilityConfig: UtilityConfig{Decorate: &no}}},
				},
			},
			periodics: []Periodic{
				{JobBase: JobBase{UtilityConfig: UtilityConfig{Decorate: &no}}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jobConfig := &JobConfig{
				DecorateAllJobs:   tc.decorateAll,
				PresubmitsStatic:  tc.presubmits,
				PostsubmitsStatic: tc.postsubmits,
				Periodics:         tc.periodics,
			}

			if actual := jobConfig.decorationRequested(); actual != tc.expected {
				t.Errorf("expected %t got %t", tc.expected, actual)
			}
		})
	}
}

func TestInRepoConfigEnabled(t *testing.T) {
	testCases := []struct {
		name     string
		config   Config
		expected bool
	}{
		{
			name: "Exact match",
			config: Config{
				ProwConfig: ProwConfig{
					InRepoConfig: InRepoConfig{
						Enabled: map[string]*bool{
							"org/repo": utilpointer.BoolPtr(true),
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "Orgname matches",
			config: Config{
				ProwConfig: ProwConfig{
					InRepoConfig: InRepoConfig{
						Enabled: map[string]*bool{
							"org": utilpointer.BoolPtr(true),
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "Globally enabled",
			config: Config{
				ProwConfig: ProwConfig{
					InRepoConfig: InRepoConfig{
						Enabled: map[string]*bool{
							"*": utilpointer.BoolPtr(true),
						},
					},
				},
			},
			expected: true,
		},
		{
			name:     "Disabled by default",
			expected: false,
		},
	}

	for idx := range testCases {
		tc := testCases[idx]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if result := tc.config.InRepoConfigEnabled("org/repo"); result != tc.expected {
				t.Errorf("Expected %t, got %t", tc.expected, result)
			}
		})
	}
}

func TestGetProwYAMLDoesNotCallRefGettersWhenInrepoconfigIsDisabled(t *testing.T) {
	t.Parallel()

	var baseSHAGetterCalled, headSHAGetterCalled bool
	baseSHAGetter := func() (string, error) {
		baseSHAGetterCalled = true
		return "", nil
	}
	headSHAGetter := func() (string, error) {
		headSHAGetterCalled = true
		return "", nil
	}

	c := &Config{}
	if _, err := c.getProwYAML(nil, "test", baseSHAGetter, headSHAGetter); err != nil {
		t.Fatalf("error calling GetProwYAML: %v", err)
	}
	if baseSHAGetterCalled {
		t.Error("baseSHAGetter got called")
	}
	if headSHAGetterCalled {
		t.Error("headSHAGetter got called")
	}
}

func TestGetPresubmitsReturnsStaticAndInrepoconfigPresubmits(t *testing.T) {
	t.Parallel()

	org, repo := "org", "repo"
	c := &Config{
		ProwConfig: ProwConfig{
			InRepoConfig: InRepoConfig{Enabled: map[string]*bool{"*": utilpointer.BoolPtr(true)}},
		},
		JobConfig: JobConfig{
			PresubmitsStatic: map[string][]Presubmit{
				org + "/" + repo: {{
					JobBase:  JobBase{Name: "my-static-presubmit"},
					Reporter: Reporter{Context: "my-static-presubmit"},
				}},
			},
			ProwYAMLGetter: fakeProwYAMLGetterFactory(
				[]Presubmit{
					{
						JobBase: JobBase{Name: "hans"},
					},
				},
				nil,
			),
		},
	}

	presubmits, err := c.GetPresubmits(nil, org+"/"+repo, func() (string, error) { return "", nil })
	if err != nil {
		t.Fatalf("Error calling GetPresubmits: %v", err)
	}

	if n := len(presubmits); n != 2 ||
		presubmits[0].Name != "my-static-presubmit" ||
		presubmits[1].Name != "hans" {
		t.Errorf(`expected exactly two presubmits named "my-static-presubmit" and "hans", got %d (%v)`, n, presubmits)
	}
}

func TestGetPostsubmitsReturnsStaticAndInrepoconfigPostsubmits(t *testing.T) {
	t.Parallel()

	org, repo := "org", "repo"
	c := &Config{
		ProwConfig: ProwConfig{
			InRepoConfig: InRepoConfig{Enabled: map[string]*bool{"*": utilpointer.BoolPtr(true)}},
		},
		JobConfig: JobConfig{
			PostsubmitsStatic: map[string][]Postsubmit{
				org + "/" + repo: {{
					JobBase:  JobBase{Name: "my-static-postsubmits"},
					Reporter: Reporter{Context: "my-static-postsubmits"},
				}},
			},
			ProwYAMLGetter: fakeProwYAMLGetterFactory(
				nil,
				[]Postsubmit{
					{
						JobBase: JobBase{Name: "hans"},
					},
				},
			),
		},
	}

	postsubmits, err := c.GetPostsubmits(nil, org+"/"+repo, func() (string, error) { return "", nil })
	if err != nil {
		t.Fatalf("Error calling GetPostsubmits: %v", err)
	}

	if n := len(postsubmits); n != 2 ||
		postsubmits[0].Name != "my-static-postsubmits" ||
		postsubmits[1].Name != "hans" {
		t.Errorf(`expected exactly two postsubmits named "my-static-postsubmits" and "hans", got %d (%v)`, n, postsubmits)
	}
}

func TestInRepoConfigAllowsCluster(t *testing.T) {
	const clusterName = "that-cluster"

	testCases := []struct {
		name            string
		repoIdentifier  string
		allowedClusters map[string][]string

		expectedResult bool
	}{
		{
			name:           "Nothing configured, nothing allowed",
			repoIdentifier: "foo",
			expectedResult: false,
		},
		{
			name:            "Allowed on repolevel",
			repoIdentifier:  "foo/repo",
			allowedClusters: map[string][]string{"foo/repo": {clusterName}},
			expectedResult:  true,
		},
		{
			name:            "Not allowed on repolevel",
			repoIdentifier:  "foo/repo",
			allowedClusters: map[string][]string{"foo/repo": {"different-cluster"}},
			expectedResult:  false,
		},
		{
			name:            "Allowed for different repo",
			repoIdentifier:  "foo/repo",
			allowedClusters: map[string][]string{"bar/repo": {clusterName}},
			expectedResult:  false,
		},
		{
			name:            "Allowed on orglevel",
			repoIdentifier:  "foo/repo",
			allowedClusters: map[string][]string{"foo": {clusterName}},
			expectedResult:  true,
		},
		{
			name:            "Not allowed on orglevel",
			repoIdentifier:  "foo/repo",
			allowedClusters: map[string][]string{"foo": {"different-cluster"}},
			expectedResult:  false,
		},
		{
			name:            "Allowed on for different org",
			repoIdentifier:  "foo/repo",
			allowedClusters: map[string][]string{"bar": {clusterName}},
			expectedResult:  false,
		},
		{
			name:            "Allowed globally",
			repoIdentifier:  "foo/repo",
			allowedClusters: map[string][]string{"*": {clusterName}},
			expectedResult:  true,
		},
	}

	for idx := range testCases {
		tc := testCases[idx]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				ProwConfig: ProwConfig{InRepoConfig: InRepoConfig{AllowedClusters: tc.allowedClusters}},
			}

			if actual := cfg.InRepoConfigAllowsCluster(clusterName, tc.repoIdentifier); actual != tc.expectedResult {
				t.Errorf("expected result %t, got result %t", tc.expectedResult, actual)
			}
		})
	}
}

func TestGetDefaultDecorationConfigsThreadSafety(t *testing.T) {
	const repo = "repo"
	p := Plank{DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
		"*": {
			GCSConfiguration: &prowapi.GCSConfiguration{
				MediaTypes: map[string]string{"text": "text"},
			},
		},
		repo: {
			GCSConfiguration: &prowapi.GCSConfiguration{
				MediaTypes: map[string]string{"text": "text"},
			},
		},
	}}

	s1 := make(chan struct{})
	s2 := make(chan struct{})

	go func() {
		_ = p.GetDefaultDecorationConfigs(repo)
		close(s1)
	}()
	go func() {
		_ = p.GetDefaultDecorationConfigs(repo)
		close(s2)
	}()

	<-s1
	<-s2
}

func TestDefaultAndValidateReportTemplate(t *testing.T) {
	testCases := []struct {
		id          string
		controller  *Controller
		expected    *Controller
		expectedErr bool
	}{

		{
			id:         "no report_template or report_templates specified, no changes expected",
			controller: &Controller{},
			expected:   &Controller{},
		},

		{
			id:         "only report_template specified, expected report_template[*]=report_template",
			controller: &Controller{ReportTemplateString: "test template"},
			expected: &Controller{
				ReportTemplateString:  "test template",
				ReportTemplateStrings: map[string]string{"*": "test template"},
				ReportTemplates: map[string]*template.Template{
					"*": func() *template.Template {
						reportTmpl, _ := template.New("Report").Parse("test template")
						return reportTmpl
					}(),
				},
			},
		},

		{
			id:         "only report_templates specified, expected direct conversion",
			controller: &Controller{ReportTemplateStrings: map[string]string{"*": "test template"}},
			expected: &Controller{
				ReportTemplateStrings: map[string]string{"*": "test template"},
				ReportTemplates: map[string]*template.Template{
					"*": func() *template.Template {
						reportTmpl, _ := template.New("Report").Parse("test template")
						return reportTmpl
					}(),
				},
			},
		},

		{
			id:          "no '*' in report_templates specified, expected error",
			controller:  &Controller{ReportTemplateStrings: map[string]string{"org": "test template"}},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			if err := defaultAndValidateReportTemplate(tc.controller); err != nil && !tc.expectedErr {
				t.Fatalf("error not expected: %v", err)
			}

			if !reflect.DeepEqual(tc.controller, tc.expected) && !tc.expectedErr {
				t.Fatalf("\nGot: %#v\nExpected: %#v", tc.controller, tc.expected)
			}
		})
	}
}

func TestValidatePresubmits(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name          string
		presubmits    []Presubmit
		expectedError string
	}{
		{
			name: "Duplicate context causes error",
			presubmits: []Presubmit{
				{JobBase: JobBase{Name: "a"}, Reporter: Reporter{Context: "repeated"}},
				{JobBase: JobBase{Name: "b"}, Reporter: Reporter{Context: "repeated"}},
			},
			expectedError: `[jobs b and a report to the same GitHub context "repeated", jobs a and b report to the same GitHub context "repeated"]`,
		},
		{
			name: "Duplicate context on different branch doesn't cause error",
			presubmits: []Presubmit{
				{JobBase: JobBase{Name: "a"}, Reporter: Reporter{Context: "repeated"}, Brancher: Brancher{Branches: []string{"master"}}},
				{JobBase: JobBase{Name: "b"}, Reporter: Reporter{Context: "repeated"}, Brancher: Brancher{Branches: []string{"next"}}},
			},
		},
		{
			name: "Duplicate jobname causes error",
			presubmits: []Presubmit{
				{JobBase: JobBase{Name: "a"}, Reporter: Reporter{Context: "foo"}},
				{JobBase: JobBase{Name: "a"}, Reporter: Reporter{Context: "bar"}},
			},
			expectedError: "duplicated presubmit job: a",
		},
		{
			name: "Duplicate jobname on different branches doesn't cause error",
			presubmits: []Presubmit{
				{JobBase: JobBase{Name: "a"}, Reporter: Reporter{Context: "foo"}, Brancher: Brancher{Branches: []string{"master"}}},
				{JobBase: JobBase{Name: "a"}, Reporter: Reporter{Context: "foo"}, Brancher: Brancher{Branches: []string{"next"}}},
			},
		},
		{
			name:          "Invalid JobBase causes error",
			presubmits:    []Presubmit{{Reporter: Reporter{Context: "foo"}}},
			expectedError: `invalid presubmit job : name: must match regex "^[A-Za-z0-9-._]+$"`,
		},
		{
			name:          "Invalid triggering config causes error",
			presubmits:    []Presubmit{{Trigger: "some-trigger", JobBase: JobBase{Name: "my-job"}, Reporter: Reporter{Context: "foo"}}},
			expectedError: `Either both of job.Trigger and job.RerunCommand must be set, wasnt the case for job "my-job"`,
		},
		{
			name:          "Invalid reporting config causes error",
			presubmits:    []Presubmit{{JobBase: JobBase{Name: "my-job"}}},
			expectedError: "invalid presubmit job my-job: job is set to report but has no context configured",
		},
	}

	for _, tc := range testCases {
		var errMsg string
		err := validatePresubmits(tc.presubmits, "")
		if err != nil {
			errMsg = err.Error()
		}
		if errMsg != tc.expectedError {
			t.Errorf("expected error '%s', got error '%s'", tc.expectedError, errMsg)
		}
	}
}

func TestValidatePostsubmits(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name          string
		postsubmits   []Postsubmit
		expectedError string
	}{
		{
			name: "Duplicate context causes error",
			postsubmits: []Postsubmit{
				{JobBase: JobBase{Name: "a"}, Reporter: Reporter{Context: "repeated"}},
				{JobBase: JobBase{Name: "b"}, Reporter: Reporter{Context: "repeated"}},
			},
			expectedError: `[jobs b and a report to the same GitHub context "repeated", jobs a and b report to the same GitHub context "repeated"]`,
		},
		{
			name: "Duplicate context on different branch doesn't cause error",
			postsubmits: []Postsubmit{
				{JobBase: JobBase{Name: "a"}, Reporter: Reporter{Context: "repeated"}, Brancher: Brancher{Branches: []string{"master"}}},
				{JobBase: JobBase{Name: "b"}, Reporter: Reporter{Context: "repeated"}, Brancher: Brancher{Branches: []string{"next"}}},
			},
		},
		{
			name: "Duplicate jobname causes error",
			postsubmits: []Postsubmit{
				{JobBase: JobBase{Name: "a"}, Reporter: Reporter{Context: "foo"}},
				{JobBase: JobBase{Name: "a"}, Reporter: Reporter{Context: "bar"}},
			},
			expectedError: "duplicated postsubmit job: a",
		},
		{
			name:          "Invalid JobBase causes error",
			postsubmits:   []Postsubmit{{Reporter: Reporter{Context: "foo"}}},
			expectedError: `invalid postsubmit job : name: must match regex "^[A-Za-z0-9-._]+$"`,
		},
		{
			name:          "Invalid reporting config causes error",
			postsubmits:   []Postsubmit{{JobBase: JobBase{Name: "my-job"}}},
			expectedError: "invalid postsubmit job my-job: job is set to report but has no context configured",
		},
	}

	for _, tc := range testCases {
		var errMsg string
		err := validatePostsubmits(tc.postsubmits, "")
		if err != nil {
			errMsg = err.Error()
		}
		if errMsg != tc.expectedError {
			t.Errorf("expected error '%s', got error '%s'", tc.expectedError, errMsg)
		}
	}
}

func TestValidateStorageBucket(t *testing.T) {
	testCases := []struct {
		name        string
		yaml        string
		bucket      string
		expectedErr string
	}{
		{
			name:        "unspecified config means no validation",
			yaml:        ``,
			bucket:      "who-knows",
			expectedErr: "",
		},
		{
			name: "validation disabled",
			yaml: `
deck:
    skip_storage_path_validation: true`,
			bucket:      "random-unknown-bucket",
			expectedErr: "",
		},
		{
			name: "validation enabled",
			yaml: `
deck:
    skip_storage_path_validation: false`,
			bucket:      "random-unknown-bucket",
			expectedErr: "bucket \"random-unknown-bucket\" not in allowed list",
		},
		{
			name: "DecorationConfig allowed bucket",
			yaml: `
deck:
    skip_storage_path_validation: false
plank:
    default_decoration_configs:
        '*':
            gcs_configuration:
                bucket: "kubernetes-jenkins"`,
			bucket:      "kubernetes-jenkins",
			expectedErr: "",
		},
		{
			name: "custom allowed bucket",
			yaml: `
deck:
    skip_storage_path_validation: false
    additional_allowed_buckets:
    - "kubernetes-prow"`,
			bucket:      "kubernetes-prow",
			expectedErr: "",
		},
		{
			name: "unknown bucket path",
			yaml: `
deck:
    skip_storage_path_validation: false`,
			bucket:      "istio-prow",
			expectedErr: "bucket \"istio-prow\" not in allowed list",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(nested *testing.T) {
			cfg, err := loadConfigYaml(tc.yaml, nested)
			if err != nil {
				nested.Fatalf("failed to load prow config: err=%v\nYAML=%v", err, tc.yaml)
			}
			expectingErr := len(tc.expectedErr) > 0

			err = cfg.ValidateStorageBucket(tc.bucket)

			if expectingErr && err == nil {
				nested.Fatalf("no errors, but was expecting error: %v", tc.expectedErr)
			}
			if err != nil && !expectingErr {
				nested.Fatalf("expecting no errors, but got: %v", err)
			}
			if expectingErr && err != nil && !strings.Contains(err.Error(), tc.expectedErr) {
				nested.Fatalf("expecting error substring \"%v\", but got error: %v", tc.expectedErr, err)
			}
		})
	}
}

func loadConfigYaml(prowConfigYaml string, t *testing.T) (*Config, error) {
	prowConfigDir, err := ioutil.TempDir("", "prowConfig")
	if err != nil {
		t.Fatalf("fail to make tempdir: %v", err)
	}
	defer os.RemoveAll(prowConfigDir)

	prowConfig := filepath.Join(prowConfigDir, "config.yaml")
	if err := ioutil.WriteFile(prowConfig, []byte(prowConfigYaml), 0666); err != nil {
		t.Fatalf("fail to write prow config: %v", err)
	}

	return Load(prowConfig, "")
}

func TestGenYamlDocs(t *testing.T) {
	const fixtureName = "./prow-config-documented.yaml"
	inputFiles, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("filepath.Glob: %v", err)
	}
	prowapiInputFiles, err := filepath.Glob("../apis/prowjobs/v1/*.go")
	if err != nil {
		t.Fatalf("prowapi filepath.Glob: %v", err)
	}
	inputFiles = append(inputFiles, prowapiInputFiles...)

	commentMap, err := genyaml.NewCommentMap(inputFiles...)
	if err != nil {
		t.Fatalf("failed to construct commentMap: %v", err)
	}
	actualYaml, err := commentMap.GenYaml(genyaml.PopulateStruct(&ProwConfig{}))
	if err != nil {
		t.Fatalf("genyaml errored: %v", err)
	}
	if os.Getenv("UPDATE") != "" {
		if err := ioutil.WriteFile(fixtureName, []byte(actualYaml), 0644); err != nil {
			t.Fatalf("failed to write fixture: %v", err)
		}
	}
	expectedYaml, err := ioutil.ReadFile(fixtureName)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	if diff := cmp.Diff(actualYaml, string(expectedYaml)); diff != "" {
		t.Errorf("Actual result differs from expected: %s. If this is expected, re-run the tests with the UPDATE env var set to update the fixture: UPDATE=true go test ./...", diff)
	}
}
