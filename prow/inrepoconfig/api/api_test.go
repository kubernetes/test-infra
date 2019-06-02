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

package api

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/localgit"
)

// TestDefaultAndValidateInRepoConfigPresubmits tests the defaulting
// and validation in defaultAndValidateInRepoConfig
func TestDefaultAndValidateInRepoConfigPresubmits(t *testing.T) {
	tests := []struct {
		name         string
		inrepoconfig *InRepoConfig
		configModify func(*config.Config)
		verifyFunc   func(config.Presubmit) error
		errExpected  bool
	}{
		{
			name: "Verify DefaultPresubmitFields is executed",
			inrepoconfig: &InRepoConfig{Presubmits: []config.Presubmit{{
				JobBase: config.JobBase{
					Name: "test-job",
				},
			}}},
			verifyFunc: func(ps config.Presubmit) error {
				if ps.Context != "test-job" {
					return fmt.Errorf(`Expected ps.Context to be "test-job", was %q`, ps.Context)
				}
				if ps.Agent != "kubernetes" {
					return fmt.Errorf(`Expected agent to be "kubernetes", was %q`, ps.Agent)
				}
				return nil
			},
		},
		{
			name: "Verify decoration config gets set",
			inrepoconfig: &InRepoConfig{Presubmits: []config.Presubmit{{
				JobBase: config.JobBase{
					Name:          "test-job",
					UtilityConfig: config.UtilityConfig{Decorate: true},
				},
			}}},
			configModify: func(c *config.Config) {
				c.Plank.DefaultDecorationConfig = &prowapi.DecorationConfig{
					CookiefileSecret: "my-secret",
					UtilityImages: &prowapi.UtilityImages{
						CloneRefs:  "a",
						InitUpload: "b",
						Entrypoint: "c",
						Sidecar:    "d",
					},
					GCSConfiguration: &prowapi.GCSConfiguration{
						PathStrategy: "explicit",
					},
					GCSCredentialsSecret: "e",
				}
			},
			verifyFunc: func(ps config.Presubmit) error {
				if ps.DecorationConfig == nil || ps.DecorationConfig.CookiefileSecret == "" {
					return errors.New("Expected decoration config to get set")
				}
				return nil
			},
		},
		{
			name: "Verify Presets get set",
			inrepoconfig: &InRepoConfig{Presubmits: []config.Presubmit{{
				JobBase: config.JobBase{
					Name:   "test-job",
					Labels: map[string]string{"my-ps": "true"},
				},
			}}},
			configModify: func(c *config.Config) {
				c.Presets = []config.Preset{
					{
						Labels: map[string]string{"my-ps": "true"},
						Env:    []corev1.EnvVar{{Name: "my-env-var"}},
					},
				}
			},
			verifyFunc: func(ps config.Presubmit) error {
				for _, container := range ps.Spec.Containers {
					for _, envVar := range container.Env {
						if envVar.Name == "my-env-var" {
							return nil
						}
					}
				}
				return errors.New("Did not find env var on container")
			},
		},
		{
			name: "Verify checkconfigapis ValidatePresubmitJob is executed",
			inrepoconfig: &InRepoConfig{Presubmits: []config.Presubmit{{
				JobBase: config.JobBase{
					Name: "Very long job name because short ones can be done by everyone!!!",
				},
			}}},
			errExpected: true,
		},
		{
			name: "Verify regexes get set",
			inrepoconfig: &InRepoConfig{Presubmits: []config.Presubmit{{
				JobBase: config.JobBase{
					Name: "test-job",
				},
			}}},
			verifyFunc: func(ps config.Presubmit) error {
				// The config.Presubmit.re field is unexported, so we have to create
				// an identical config.Presubmit, run it through SetPresubmitRegexes and
				// use reflect.DeepEqual
				s := ""
				presubmitToCompare := config.Presubmit{
					JobBase: config.JobBase{
						Name:      "test-job",
						Namespace: &s,
						Agent:     "kubernetes",
						Cluster:   "default",
					},
					Trigger:      "(?m)^/test( | .* )test-job,?($|\\s.*)",
					RerunCommand: "/test test-job",
					Reporter: config.Reporter{
						Context: "test-job",
					},
				}
				if err := config.SetPresubmitRegexes([]config.Presubmit{presubmitToCompare}); err != nil {
					return fmt.Errorf("failed to call SetPresubmitRegexes: %v", err)
				}
				if equal := reflect.DeepEqual(presubmitToCompare, ps); equal {
					return fmt.Errorf("Presubmits are not equal")
				}
				return nil
			},
		},
		{
			name: "Verify duplicate job config check is executed",
			inrepoconfig: &InRepoConfig{Presubmits: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "test-job",
					},
				},
				{
					JobBase: config.JobBase{
						Name: "test-job",
					},
				},
			},
			},
			errExpected: true,
		},
		{
			name: "Verify validateJobBase is executed",
			inrepoconfig: &InRepoConfig{Presubmits: []config.Presubmit{{
				JobBase: config.JobBase{
					Name: "test-job",
					// ValidateJobBase checks that jobs with the Kubernetes
					// agent contain exactly one container in their PodSpec
					// so this is expected to fail
					Spec: &corev1.PodSpec{},
				},
			}}},
			errExpected: true,
		},
		{
			name: "Verify validateTriggering is executed",
			inrepoconfig: &InRepoConfig{Presubmits: []config.Presubmit{{
				JobBase: config.JobBase{
					Name: "test-job",
				},
				AlwaysRun: true,
				RegexpChangeMatcher: config.RegexpChangeMatcher{
					RunIfChanged: "some-file",
				},
			}}},
			errExpected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{
				ProwConfig: config.ProwConfig{
					PodNamespace: "my-pod-ns",
				},
			}
			if tc.configModify != nil {
				tc.configModify(cfg)
			}

			if tc.errExpected && tc.verifyFunc != nil {
				t.Fatal("Can not use verifyFunc when an error is expected")
			}

			// Validation defaults the job agent to Kubernetes and if the
			// job agent is kubernetes and the spec is a nil pointer, it fails
			for i := range tc.inrepoconfig.Presubmits {
				if tc.inrepoconfig.Presubmits[i].Spec == nil {
					tc.inrepoconfig.Presubmits[i].Spec = &corev1.PodSpec{
						// Validation tests that the spec contains exactly one container
						// DecorationConfigValidation tests that the containre has an command or arg
						Containers: []corev1.Container{{Command: []string{"cmd"}}},
					}
				}
			}

			if err := tc.inrepoconfig.defaultAndValidateInRepoConfig(cfg, "org/repo"); err != nil {
				if !tc.errExpected {
					t.Fatalf("error when calling defaultAndValidateInRepoConfig: %v", err)
				}
				return
			}
			if tc.errExpected {
				t.Fatal("Expected to get an error calling defaultAndValidateInRepoConfig but did not happen")
			}

			for _, presubmit := range tc.inrepoconfig.Presubmits {
				if err := tc.verifyFunc(presubmit); err != nil {
					t.Errorf("Verification failed: %v", err)
				}
			}

		})
	}
}

// TestNew tests the New func
func TestNew(t *testing.T) {
	testCases := []struct {
		name        string
		baseContent map[string][]byte
		headContent map[string][]byte
		errExpected bool
		verifyFunc  func(*InRepoConfig) error
	}{
		{
			name: "Verify getting jobs from base",
			baseContent: map[string][]byte{
				"prow.yaml": []byte(`
presubmits:
- name: my-ps
  spec:
    containers:
    - name: my-container`,
				),
			},
			headContent: map[string][]byte{"some-file": []byte("some-content")},
			verifyFunc: func(irc *InRepoConfig) error {
				if len(irc.Presubmits) == 1 && irc.Presubmits[0].Name == "my-ps" {
					return nil
				}
				return errors.New(`Expected to find presubmit "my-ps" but wasn't there :(`)
			},
		},
		{
			name:        "Verify getting jobs from head",
			baseContent: map[string][]byte{"some-file": []byte("some-content")},
			headContent: map[string][]byte{
				"prow.yaml": []byte(`
presubmits:
- name: my-ps
  spec:
    containers:
    - name: my-container`,
				),
			},
			verifyFunc: func(irc *InRepoConfig) error {
				if len(irc.Presubmits) == 1 && irc.Presubmits[0].Name == "my-ps" {
					return nil
				}
				return errors.New(`Expected to find presubmit "my-ps" but wasn't there :(`)
			},
		},
		{
			name:        "Verify no prow.yaml doesn't result in an error",
			baseContent: map[string][]byte{"some-file": []byte("some-content")},
			headContent: map[string][]byte{"another-file": []byte("some-content")},
			verifyFunc:  func(_ *InRepoConfig) error { return nil },
		},
		{
			name:        "Verify faulty prow.yaml results in an error",
			baseContent: map[string][]byte{"prow.yaml": []byte("some_random_key: 2")},
			headContent: map[string][]byte{"another-file": []byte("some-content")},
			errExpected: true,
		},
	}

	const org, repo = "org", "repo"
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			if tc.errExpected && tc.verifyFunc != nil {
				t.Fatal("Can not use verifyFunc when an error is expected")
			}

			lg, gc, err := localgit.New()
			if err != nil {
				t.Fatalf("Making local git repo: %v", err)
			}
			defer func() {
				if err := lg.Clean(); err != nil {
					t.Errorf("Error cleaning LocalGit: %v", err)
				}
				if err := gc.Clean(); err != nil {
					t.Errorf("Error cleaning Client: %v", err)
				}
			}()
			if err := lg.MakeFakeRepo(org, repo); err != nil {
				t.Fatalf("Making fake repo: %v", err)
			}

			if err := lg.AddCommit(org, repo, tc.baseContent); err != nil {
				t.Fatalf("failed to add commit to base: %v", err)
			}
			if err := lg.CheckoutNewBranch(org, repo, "can-I-haz-pulled"); err != nil {
				t.Fatalf("failed to create new branch: %v", err)
			}
			if err := lg.AddCommit(org, repo, tc.headContent); err != nil {
				t.Fatalf("failed to add head commit: %v", err)
			}
			baseSHA, err := lg.RevParse(org, repo, "master")
			if err != nil {
				t.Fatalf("failed to get baseSHA: %v", err)
			}
			headSHA, err := lg.RevParse(org, repo, "HEAD")
			if err != nil {
				t.Fatalf("failed to head headSHA: %v", err)
			}

			cfg := &config.Config{
				ProwConfig: config.ProwConfig{
					PodNamespace: "my-pod-ns",
				},
			}
			logger := logrus.WithField("testcase", tc.name)
			irc, err := New(logger, cfg, gc, org, repo, baseSHA, []string{headSHA})
			if err != nil {
				if !tc.errExpected {
					t.Fatalf("Unexpected error: %v", err)
				}
				return
			}
			if tc.errExpected {
				t.Fatalf("Expected error but didn't get it")
			}

			if err := tc.verifyFunc(irc); err != nil {
				t.Fatalf("verification failed: %v", err)
			}
		})

	}
}
