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
	"testing"

	"k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/kube"
)

var defaultBranch = localgit.DefaultBranch("")

func TestDefaultProwYAMLGetter(t *testing.T) {
	testDefaultProwYAMLGetter(localgit.New, t)
}

func TestDefaultProwYAMLGetterV2(t *testing.T) {
	testDefaultProwYAMLGetter(localgit.NewV2, t)
}

func testDefaultProwYAMLGetter(clients localgit.Clients, t *testing.T) {
	org, repo := "org", "repo"
	testCases := []struct {
		name              string
		baseContent       map[string][]byte
		headContent       map[string][]byte
		config            *Config
		dontPassGitClient bool
		validate          func(*ProwYAML, error) error
	}{
		// presubmits
		{
			name: "Basic happy path (presubmits)",
			baseContent: map[string][]byte{
				".prow.yaml": []byte(`presubmits: [{"name": "hans", "spec": {"containers": [{}]}}]`),
			},
			validate: func(p *ProwYAML, err error) error {
				if err != nil {
					return fmt.Errorf("unexpected error: %v", err)
				}
				if n := len(p.Presubmits); n != 1 || p.Presubmits[0].Name != "hans" {
					return fmt.Errorf(`expected exactly one presubmit with name "hans", got %v`, p.Presubmits)
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
					return fmt.Errorf("unexpected error: %v", err)
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
					return fmt.Errorf("unexpected error: %v", err)
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
					org + "/" + repo: {{Reporter: Reporter{Context: "hans"}, JobBase: JobBase{Name: "hans"}}},
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
					return fmt.Errorf("unexpected error: %v", err)
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
					return fmt.Errorf("unexpected error: %v", err)
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
					return fmt.Errorf("unexpected error: %v", err)
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
					org + "/" + repo: {{Reporter: Reporter{Context: "hans"}, JobBase: JobBase{Name: "hans"}}},
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
					return fmt.Errorf("unexpected error: %v", err)
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
					return fmt.Errorf("unexpected error: %v", err)
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
					return fmt.Errorf("unexpected error: %v", err)
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
	}

	for idx := range testCases {
		tc := testCases[idx]
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

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
				p, err = defaultProwYAMLGetter(tc.config, testGC, org+"/"+repo, baseSHA)
			} else {
				p, err = defaultProwYAMLGetter(tc.config, testGC, org+"/"+repo, baseSHA, headSHA)
			}

			if err := tc.validate(p, err); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDefaultProwYAMLGetter_RejectsNonGitHubRepo(t *testing.T) {
	testDefaultProwYAMLGetter_RejectsNonGitHubRepo(localgit.New, t)
}

func TestDefaultProwYAMLGetter_RejectsNonGitHubRepoV2(t *testing.T) {
	testDefaultProwYAMLGetter_RejectsNonGitHubRepo(localgit.NewV2, t)
}

func testDefaultProwYAMLGetter_RejectsNonGitHubRepo(clients localgit.Clients, t *testing.T) {
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
	if _, err := defaultProwYAMLGetter(&Config{}, gc, identifier, ""); err == nil || err.Error() != expectedErrMsg {
		t.Errorf("Error %v does not have expected message %s", err, expectedErrMsg)
	}
}
