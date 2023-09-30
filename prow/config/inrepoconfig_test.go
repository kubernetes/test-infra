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

package config

import (
	"errors"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/kube"
)

var defaultBranch = localgit.DefaultBranch("")

func TestDefaultProwYAMLGetterV2(t *testing.T) {
	testDefaultProwYAMLGetter(localgit.NewV2, t)
}

func testDefaultProwYAMLGetter(clients localgit.Clients, t *testing.T) {
	org, defaultRepo := "org", "repo"
	testCases := []struct {
		name              string
		baseContent       map[string][]byte
		headContent       map[string][]byte
		config            *Config
		dontPassGitClient bool
		validate          func(*ProwYAML, error) error
		repo              string
	}{
		// presubmits
		{
			name: "Basic happy path (presubmits)",
			baseContent: map[string][]byte{
				".prow.yaml": []byte(`presubmits: [{"name": "hans", "annotations": {"foo.bar": "foobar"}, "spec": {"containers": [{}]}}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Presubmits); n != 1 || p.Presubmits[0].Name != "hans" {
					return fmt.Errorf(`expected exactly one presubmit with name "hans", got %v`, p.Presubmits)
				}
				if diff := cmp.Diff(p.Presubmits[0].Annotations, map[string]string{"foo.bar": "foobar"}); diff != "" {
					return errors.New(diff)
				}
				return nil
			},
		},
		{
			name: "Merging is executed (presubmits)",
			headContent: map[string][]byte{
				".prow.yaml": []byte(`presubmits: [{"name": "hans", "spec": {"containers": [{}]}}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Presubmits); n != 1 || p.Presubmits[0].Name != "hans" {
					return fmt.Errorf(`expected exactly one presubmit with name "hans", got %v`, p.Presubmits)
				}
				return nil
			},
		},
		{
			name: "Presubmit defaulting is executed",
			baseContent: map[string][]byte{
				".prow.yaml": []byte(`presubmits: [{"name": "hans", "spec": {"containers": [{}]}}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Presubmits); n != 1 || p.Presubmits[0].Name != "hans" {
					return fmt.Errorf(`expected exactly one presubmit with name "hans", got %v`, p.Presubmits)
				}
				if p.Presubmits[0].Context != "hans" {
					return fmt.Errorf(`expected defaulting to set context to "hans", was %q`, p.Presubmits[0].Context)
				}
				return nil
			},
		},
		{
			name: "Presubmit validation is executed",
			baseContent: map[string][]byte{
				".prow.yaml": []byte(`presubmits: [{"name": "hans", "spec": {"containers": [{}]}},{"name": "hans", "spec": {"containers": [{}]}}]`),
			},
			validate: func(_ *ProwYAML, err error) error {
				if err == nil {
					return errors.New("error is nil")
				}
				expectedErrMsg := "duplicated presubmit job: hans"
				if err.Error() != expectedErrMsg {
					return fmt.Errorf("expected error message to be %q, was %q", expectedErrMsg, err.Error())
				}
				return nil
			},
		},
		{
			name: "Presubmit validation includes static presubmits",
			baseContent: map[string][]byte{
				".prow.yaml": []byte(`presubmits: [{"name": "hans", "spec": {"containers": [{}]}}]`),
			},
			config: &Config{JobConfig: JobConfig{
				PresubmitsStatic: map[string][]Presubmit{
					org + "/" + defaultRepo: {{Reporter: Reporter{Context: "hans"}, JobBase: JobBase{Name: "hans"}}},
				},
			}},
			validate: func(_ *ProwYAML, err error) error {
				if err == nil {
					return errors.New("error is nil")
				}
				expectedErrMsg := "duplicated presubmit job: hans"
				if err.Error() != expectedErrMsg {
					return fmt.Errorf("expected error message to be %q, was %q", expectedErrMsg, err.Error())
				}
				return nil
			},
		},
		{
			name: "Branchconfig on presubmit is allowed",
			baseContent: map[string][]byte{
				".prow.yaml": []byte(`presubmits: [{"name": "hans", "spec": {"containers": [{}]}, "branches":["master"]}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Presubmits); n != 1 || p.Presubmits[0].Name != "hans" {
					return fmt.Errorf(`expected exactly one postsubmit with name "hans", got %v`, p.Presubmits)
				}
				if n := len(p.Presubmits[0].Branches); n != 1 || p.Presubmits[0].Branches[0] != "master" {
					return fmt.Errorf(`expected exactly one postsubmit branch with name "master", got %v`, p.Presubmits[0].Branches)
				}
				return nil
			},
		},
		{
			name: "Not allowed cluster is rejected (presubmits)",
			baseContent: map[string][]byte{
				".prow.yaml": []byte(`presubmits: [{"name": "hans", "cluster": "privileged", "spec": {"containers": [{}]}}]`),
			},
			validate: func(_ *ProwYAML, err error) error {
				if err == nil {
					return errors.New("error is nil")
				}
				expectedErrMsg := "cluster \"privileged\" is not allowed for repository \"org/repo\""
				if err.Error() != expectedErrMsg {
					return fmt.Errorf("expected error message to be %q, was %q", expectedErrMsg, err.Error())
				}
				return nil
			},
		},
		// postsubmits
		{
			name: "Basic happy path (postsubmits)",
			baseContent: map[string][]byte{
				".prow.yaml": []byte(`postsubmits: [{"name": "hans", "spec": {"containers": [{}]}}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Postsubmits); n != 1 || p.Postsubmits[0].Name != "hans" {
					return fmt.Errorf(`expected exactly one postsubmit with name "hans", got %v`, p.Postsubmits)
				}
				return nil
			},
		},
		{
			name: "Postsubmit defaulting is executed",
			baseContent: map[string][]byte{
				".prow.yaml": []byte(`postsubmits: [{"name": "hans", "spec": {"containers": [{}]}}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Postsubmits); n != 1 || p.Postsubmits[0].Name != "hans" {
					return fmt.Errorf(`expected exactly one postsubmit with name "hans", got %v`, p.Postsubmits)
				}
				if p.Postsubmits[0].Context != "hans" {
					return fmt.Errorf(`expected defaulting to set context to "hans", was %q`, p.Postsubmits[0].Context)
				}
				return nil
			},
		},
		{
			name: "Postsubmit validation is executed",
			baseContent: map[string][]byte{
				".prow.yaml": []byte(`postsubmits: [{"name": "hans", "spec": {"containers": [{}]}},{"name": "hans", "spec": {"containers": [{}]}}]`),
			},
			validate: func(_ *ProwYAML, err error) error {
				if err == nil {
					return errors.New("error is nil")
				}
				expectedErrMsg := "duplicated postsubmit job: hans"
				if err.Error() != expectedErrMsg {
					return fmt.Errorf("expected error message to be %q, was %q", expectedErrMsg, err.Error())
				}
				return nil
			},
		},
		{
			name: "Postsubmit validation includes static postsubmits",
			baseContent: map[string][]byte{
				".prow.yaml": []byte(`postsubmits: [{"name": "hans", "spec": {"containers": [{}]}}]`),
			},
			config: &Config{JobConfig: JobConfig{
				PostsubmitsStatic: map[string][]Postsubmit{
					org + "/" + defaultRepo: {{Reporter: Reporter{Context: "hans"}, JobBase: JobBase{Name: "hans"}}},
				},
			}},
			validate: func(_ *ProwYAML, err error) error {
				if err == nil {
					return errors.New("error is nil")
				}
				expectedErrMsg := "duplicated postsubmit job: hans"
				if err.Error() != expectedErrMsg {
					return fmt.Errorf("expected error message to be %q, was %q", expectedErrMsg, err.Error())
				}
				return nil
			},
		},
		{
			name: "Branchconfig on postsubmit is allowed",
			baseContent: map[string][]byte{
				".prow.yaml": []byte(`postsubmits: [{"name": "hans", "spec": {"containers": [{}]}, "branches":["master"]}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Postsubmits); n != 1 || p.Postsubmits[0].Name != "hans" {
					return fmt.Errorf(`expected exactly one postsubmit with name "hans", got %v`, p.Postsubmits)
				}
				if n := len(p.Postsubmits[0].Branches); n != 1 || p.Postsubmits[0].Branches[0] != "master" {
					return fmt.Errorf(`expected exactly one postsubmit branch with name "master", got %v`, p.Postsubmits[0].Branches)
				}
				return nil
			},
		},
		// prowyaml
		{
			name: "Not allowed cluster is rejected",
			baseContent: map[string][]byte{
				".prow.yaml": []byte(`postsubmits: [{"name": "hans", "cluster": "privileged", "spec": {"containers": [{}]}}]`),
			},
			validate: func(_ *ProwYAML, err error) error {
				if err == nil {
					return errors.New("error is nil")
				}
				expectedErrMsg := "cluster \"privileged\" is not allowed for repository \"org/repo\""
				if err.Error() != expectedErrMsg {
					return fmt.Errorf("expected error message to be %q, was %q", expectedErrMsg, err.Error())
				}
				return nil
			},
		},
		{
			name: "No prow.yaml, no error, no nullpointer",
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if p == nil {
					return errors.New("prowYAML is nil")
				}
				if n := len(p.Presubmits); n != 0 {
					return fmt.Errorf("expected to get zero presubmits, got %d", n)
				}
				return nil
			},
		},
		{
			name: "Yaml unmarshaling is not strict",
			baseContent: map[string][]byte{
				".prow.yaml": []byte(`postsubmits: [{"name": "hans", "undef_attr": true, "spec": {"containers": [{}]}}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Postsubmits); n != 1 || p.Postsubmits[0].Name != "hans" {
					return fmt.Errorf(`expected exactly one postsubmit with name "hans", got %v`, p.Postsubmits)
				}
				return nil
			},
		},
		// git client
		{
			name:              "No panic on nil gitClient",
			dontPassGitClient: true,
			validate: func(_ *ProwYAML, err error) error {
				if err == nil || err.Error() != "gitClient is nil" {
					return fmt.Errorf(`expected error to be "gitClient is nil", was %v`, err)
				}
				return nil
			},
		},
		// .prow directory
		{
			name: "Basic happy path (.prow directory, single file)",
			baseContent: map[string][]byte{
				".prow/base.yaml": []byte(`presubmits: [{"name": "hans", "spec": {"containers": [{}]}}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Presubmits); n != 1 || p.Presubmits[0].Name != "hans" {
					return fmt.Errorf(`expected exactly one presubmit with name "hans", got %v`, p.Presubmits)
				}
				return nil
			},
		},
		{
			name: "Prefer .prow directory over .prow.yaml file",
			baseContent: map[string][]byte{
				".prow.yaml":      []byte(`presubmits: [{"name": "hans", "spec": {"containers": [{}]}}]`),
				".prow/base.yaml": []byte(`presubmits: [{"name": "kurt", "spec": {"containers": [{}]}}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Presubmits); n != 1 || p.Presubmits[0].Name != "kurt" {
					return fmt.Errorf(`expected exactly one presubmit with name "kurt", got %v`, p.Presubmits)
				}
				return nil
			},
		},
		{
			name: "Merge presubmits under .prow directory",
			baseContent: map[string][]byte{
				".prow/one.yaml": []byte(`presubmits: [{"name": "hans", "spec": {"containers": [{}]}}]`),
				".prow/two.yaml": []byte(`presubmits: [{"name": "kurt", "spec": {"containers": [{}]}}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Presubmits); n != 2 ||
					p.Presubmits[0].Name != "hans" ||
					p.Presubmits[1].Name != "kurt" {
					return fmt.Errorf(`expected exactly two presubmit with name "hans" and "kurt", got %v`, p.Presubmits)
				}
				return nil
			},
		},
		{
			name: "Merge presubmits several levels under .prow directory",
			baseContent: map[string][]byte{
				".prow/sub1/sub2/one.yaml": []byte(`presubmits: [{"name": "hans", "spec": {"containers": [{}]}}]`),
				".prow/sub3/two.yaml":      []byte(`presubmits: [{"name": "kurt", "spec": {"containers": [{}]}}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Presubmits); n != 2 ||
					p.Presubmits[0].Name != "hans" ||
					p.Presubmits[1].Name != "kurt" {
					return fmt.Errorf(`expected exactly two presubmit with name "hans" and "kurt", got %v`, p.Presubmits)
				}
				return nil
			},
		},
		{
			name: "Merge postsubmits under .prow directory",
			baseContent: map[string][]byte{
				".prow/one.yaml": []byte(`postsubmits: [{"name": "hans", "spec": {"containers": [{}]}}]`),
				".prow/two.yaml": []byte(`postsubmits: [{"name": "kurt", "spec": {"containers": [{}]}}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Postsubmits); n != 2 ||
					p.Postsubmits[0].Name != "hans" ||
					p.Postsubmits[1].Name != "kurt" {
					return fmt.Errorf(`expected exactly two postsubmit with name "hans" and "kurt", got %v`, p.Postsubmits)
				}
				return nil
			},
		},
		{
			name: "Merge postsubmits several levels under .prow directory",
			baseContent: map[string][]byte{
				".prow/sub1/sub2/one.yaml": []byte(`postsubmits: [{"name": "hans", "spec": {"containers": [{}]}}]`),
				".prow/sub3/two.yaml":      []byte(`postsubmits: [{"name": "kurt", "spec": {"containers": [{}]}}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Postsubmits); n != 2 ||
					p.Postsubmits[0].Name != "hans" ||
					p.Postsubmits[1].Name != "kurt" {
					return fmt.Errorf(`expected exactly two postsubmit with names "hans" and "kurt", got %v`, p.Postsubmits)
				}
				return nil
			},
		},
		{
			name: "Merge presets under .prow directory",
			baseContent: map[string][]byte{
				".prow/one.yaml": []byte(`presets: [{"labels": {"hans": "hansValue"}}]`),
				".prow/two.yaml": []byte(`presets: [{"labels": {"kurt": "kurtValue"}}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Presets); n != 2 ||
					p.Presets[0].Labels["hans"] != "hansValue" ||
					p.Presets[1].Labels["kurt"] != "kurtValue" {
					return fmt.Errorf(`expected exactly two presets with labels "hans": "hansValue" and "kurt": "kurtValue", got %v`, p.Presets)
				}
				return nil
			},
		},
		{
			name: "Merge presets several levels under .prow directory",
			baseContent: map[string][]byte{
				".prow/sub1/sub2/one.yaml": []byte(`presets: [{"labels": {"hans": "hansValue"}}]`),
				".prow/sub3/two.yaml":      []byte(`presets: [{"labels": {"kurt": "kurtValue"}}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Presets); n != 2 ||
					p.Presets[0].Labels["hans"] != "hansValue" ||
					p.Presets[1].Labels["kurt"] != "kurtValue" {
					return fmt.Errorf(`expected exactly two presets with labels "hans": "hansValue" and "kurt": "kurtValue", got %v`, p.Presets)
				}
				return nil
			},
		},
		{
			name: "Merge presubmits, postsubmits and presets several levels under .prow directory",
			baseContent: map[string][]byte{
				".prow/sub1/sub2/one.yaml": []byte(`presubmits: [{"name": "hans", "spec": {"containers": [{}]}}]
postsubmits: [{"name": "karl", "spec": {"containers": [{}]}}]
presets: [{"labels": {"karl": "karlValue"}}]
`),
				".prow/sub3/two.yaml": []byte(`presubmits: [{"name": "kurt", "spec": {"containers": [{}]}}]
postsubmits: [{"name": "oli", "spec": {"containers": [{}]}}]`),
				".prow/sub4//sub5/sub6/three.yaml": []byte(`presets: [{"labels": {"henning": "henningValue"}}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Presubmits); n != 2 ||
					p.Presubmits[0].Name != "hans" ||
					p.Presubmits[1].Name != "kurt" {
					return fmt.Errorf(`expected exactly two presubmits with names "hans" and "kurt" got %v`, p.Presubmits)
				}
				if n := len(p.Postsubmits); n != 2 ||
					p.Postsubmits[0].Name != "karl" ||
					p.Postsubmits[1].Name != "oli" {
					return fmt.Errorf(`expected exactly two postsubmits with names "karl" and "oli", got %v`, p.Postsubmits)
				}
				if n := len(p.Presets); n != 2 ||
					p.Presets[0].Labels["karl"] != "karlValue" ||
					p.Presets[1].Labels["henning"] != "henningValue" {
					return fmt.Errorf(`expected exactly two presets with labels "karl": "karlValue" and "henning": "henningValue", got %v`, p.Presets)
				}
				return nil
			},
		},
		{
			name: "Non-.yaml files under .prow directory are allowed",
			baseContent: map[string][]byte{
				".prow/one.yaml":     []byte(`presubmits: [{"name": "hans", "spec": {"containers": [{}]}}]`),
				".prow/OWNERS":       []byte(`approvers: [approver1, approver2]`),
				".prow/sub/two.yaml": []byte(`presubmits: [{"name": "kurt", "spec": {"containers": [{}]}}]`),
				".prow/sub/OWNERS":   []byte(`approvers: [approver3, approver4]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Presubmits); n != 2 ||
					p.Presubmits[0].Name != "hans" ||
					p.Presubmits[1].Name != "kurt" {
					return fmt.Errorf(`expected exactly two presubmit with name "hans" and "kurt", got %v`, p.Presubmits)
				}
				return nil
			},
		},
		{
			name: "Both .yaml and .yml files are allowed under .prow directory)",
			baseContent: map[string][]byte{
				".prow/one.yaml": []byte(`presubmits: [{"name": "hans", "spec": {"containers": [{}]}}]`),
				".prow/two.yml":  []byte(`presubmits: [{"name": "kurt", "spec": {"containers": [{}]}}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Presubmits); n != 2 ||
					p.Presubmits[0].Name != "hans" ||
					p.Presubmits[1].Name != "kurt" {
					return fmt.Errorf(`expected exactly two presubmit with name "hans" and "kurt", got %v`, p.Presubmits)
				}
				return nil
			},
		},
		{
			name: "Basic happy path (presubmits, gerrit repo)",
			baseContent: map[string][]byte{
				".prow.yaml": []byte(`presubmits: [{"name": "hans", "spec": {"containers": [{}]}}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %w", err)
				}
				if n := len(p.Presubmits); n != 1 || p.Presubmits[0].Name != "hans" {
					return fmt.Errorf(`expected exactly one presubmit with name "hans", got %v`, p.Presubmits)
				}
				return nil
			},
			repo: "repo/name",
		},
	}

	for idx := range testCases {
		tc := testCases[idx]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := defaultRepo
			if len(tc.repo) > 0 {
				repo = tc.repo
			}

			lg, gc, err := clients()
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
			if tc.baseContent != nil {
				if err := lg.AddCommit(org, repo, tc.baseContent); err != nil {
					t.Fatalf("failed to commit baseContent: %v", err)
				}
			}
			if tc.headContent != nil {
				if err := lg.CheckoutNewBranch(org, repo, "can-I-haz-pulled"); err != nil {
					t.Fatalf("failed to create new branch: %v", err)
				}
				if err := lg.AddCommit(org, repo, tc.headContent); err != nil {
					t.Fatalf("failed to add head commit: %v", err)
				}
			}

			baseSHA, err := lg.RevParse(org, repo, defaultBranch)
			if err != nil {
				t.Fatalf("failed to get baseSHA: %v", err)
			}
			headSHA, err := lg.RevParse(org, repo, "HEAD")
			if err != nil {
				t.Fatalf("failed to head headSHA: %v", err)
			}

			if tc.config == nil {
				tc.config = &Config{
					ProwConfig: ProwConfig{
						InRepoConfig: InRepoConfig{
							AllowedClusters: map[string][]string{"*": {kube.DefaultClusterAlias}},
						},
					},
				}
			}
			// Validation fails when no NS is provided
			tc.config.PodNamespace = "my-ns"

			testGC := gc
			if tc.dontPassGitClient {
				testGC = nil
			}

			var p *ProwYAML
			if headSHA == baseSHA {
				p, err = prowYAMLGetterWithDefaults(tc.config, testGC, org+"/"+repo, baseSHA)
			} else {
				p, err = prowYAMLGetterWithDefaults(tc.config, testGC, org+"/"+repo, baseSHA, headSHA)
			}

			if err := tc.validate(p, err); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDefaultProwYAMLGetter_RejectsJustOrgV2(t *testing.T) {
	testDefaultProwYAMLGetter_RejectsJustOrg(localgit.NewV2, t)
}

func testDefaultProwYAMLGetter_RejectsJustOrg(clients localgit.Clients, t *testing.T) {
	lg, gc, err := clients()
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

	identifier := "my-repo"
	if err := lg.MakeFakeRepo(identifier, ""); err != nil {
		t.Fatalf("Making fake repo: %v", err)
	}
	expectedErrMsg := `didn't get two results when splitting repo identifier "my-repo"`
	if _, err := prowYAMLGetterWithDefaults(&Config{}, gc, identifier, "", ""); err == nil || err.Error() != expectedErrMsg {
		t.Errorf("Error %v does not have expected message %s", err, expectedErrMsg)
	}
}

type testClientFactory struct {
	git.ClientFactory // This will be nil during testing, we override the functions that are used.
	rcMap             map[string]git.RepoClient
	clientsCreated    int
}

func (cf *testClientFactory) ClientFor(org, repo string) (git.RepoClient, error) {
	cf.clientsCreated++
	// Returning this RepoClient ensures that only Fetch() is called and that Close() is not.

	return &fetchOnlyNoCleanRepoClient{cf.rcMap[repo]}, nil
}

func (cf *testClientFactory) ClientForWithRepoOpts(org, repo string, repoOpts git.RepoOpts) (git.RepoClient, error) {
	cf.clientsCreated++
	// Returning this RepoClient ensures that only Fetch() is called and that Close() is not.

	return &fetchOnlyNoCleanRepoClient{cf.rcMap[repo]}, nil
}

type fetchOnlyNoCleanRepoClient struct {
	git.RepoClient // This will be nil during testing, we override the functions that are allowed to be used.
}

func (rc *fetchOnlyNoCleanRepoClient) Fetch(arg ...string) error {
	return nil
}

// Override Close to make sure when Close is called it would error out
func (rc *fetchOnlyNoCleanRepoClient) Close() error {
	panic("This is not supposed to be called")
}

func TestInRepoConfigClean(t *testing.T) {
	t.Parallel()
	org, repo := "org", "repo"

	lg, c, _ := localgit.NewV2()
	rcMap := make(map[string]git.RepoClient)
	if err := lg.MakeFakeRepo(org, repo); err != nil {
		t.Fatal(err)
	}
	rc, err := c.ClientFor(org, repo)
	if err != nil {
		t.Fatal(err)
	}
	rcMap[repo] = rc

	cf := &testClientFactory{
		rcMap: rcMap,
	}

	// First time clone should work
	repoClient, err := cf.ClientFor(org, repo)
	if err != nil {
		t.Fatalf("Unexpected error getting repo client for thread 1: %v.", err)
	}

	// Now dirty the repo
	dir := repoClient.Directory()
	f := path.Join(dir, "new-file")
	if err := os.WriteFile(f, []byte("something"), 0644); err != nil {
		t.Fatal(err)
	}
	repoClient.Clean()

	// Second time should be none dirty
	repoClient, err = cf.ClientFor(org, repo)
	if err != nil {
		t.Fatalf("Unexpected error getting repo client for thread 1: %v.", err)
	}
	repoClient.Clean()

	_, err = os.Stat(f)
	if err == nil || !os.IsNotExist(err) {
		t.Fatalf("%s should have been deleted", f)
	}
}
