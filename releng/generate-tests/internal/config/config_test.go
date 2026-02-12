/*
Copyright 2026 The Kubernetes Authors.

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

package config_test

import (
	"testing"

	"k8s.io/test-infra/releng/generate-tests/internal/config"
)

func TestCloudProviderArgs(t *testing.T) {
	t.Parallel()

	args := config.CloudProviderArgs()

	expected := []string{
		"--check-leaked-resources",
		"--provider=gce",
		"--gcp-region=us-central1",
	}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d", len(expected), len(args))
	}

	for i, exp := range expected {
		if args[i] != exp {
			t.Errorf("arg[%d]: got %q, want %q", i, args[i], exp)
		}
	}
}

func TestImageArgs(t *testing.T) {
	t.Parallel()

	args := config.ImageArgs()
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(args))
	}

	if args[0] != "--gcp-node-image=gci" {
		t.Errorf("got %q, want --gcp-node-image=gci", args[0])
	}
}

func TestTestSuitesCompleteness(t *testing.T) {
	t.Parallel()

	suites := config.TestSuites()

	expectedSuites := []string{
		"alphafeatures-eventedpleg",
		"default",
		"reboot",
		"serial",
		"slow",
	}

	if len(suites) != len(expectedSuites) {
		t.Fatalf("expected %d suites, got %d", len(expectedSuites), len(suites))
	}

	for _, name := range expectedSuites {
		suite, ok := suites[name]
		if !ok {
			t.Errorf("missing suite %q", name)

			continue
		}

		if suite.Timeout <= 0 {
			t.Errorf("suite %q: timeout must be positive, got %d", name, suite.Timeout)
		}

		if suite.Cluster == "" {
			t.Errorf("suite %q: cluster must be non-empty", name)
		}

		if len(suite.Args) == 0 {
			t.Errorf("suite %q: must have at least one arg", name)
		}

		if suite.Resources.CPU == "" || suite.Resources.Memory == "" {
			t.Errorf("suite %q: resources must be non-empty", name)
		}
	}
}

func TestJobsReferenceValidSuites(t *testing.T) {
	t.Parallel()

	suites := config.TestSuites()

	for _, jobCfg := range config.Jobs() {
		suiteKey := jobCfg.Suite
		if jobCfg.TestSuiteOverride != "" {
			suiteKey = jobCfg.TestSuiteOverride
		}

		if _, ok := suites[suiteKey]; !ok {
			t.Errorf(
				"job suite %q (override: %q) not found in TestSuites()",
				jobCfg.Suite,
				jobCfg.TestSuiteOverride,
			)
		}
	}
}

func TestJobsCompleteness(t *testing.T) {
	t.Parallel()

	jcs := config.Jobs()
	if len(jcs) != 5 {
		t.Fatalf("expected 5 job configs, got %d", len(jcs))
	}

	var hasBlocking, hasInforming bool

	for _, jobCfg := range jcs {
		if jobCfg.Suite == "" {
			t.Error("job config has empty Suite")
		}

		if jobCfg.ReleaseBlocking {
			hasBlocking = true
		}

		if jobCfg.ReleaseInforming {
			hasInforming = true
		}

		if jobCfg.ReleaseBlocking && jobCfg.ReleaseInforming {
			t.Errorf("job %q: cannot be both blocking and informing", jobCfg.Suite)
		}
	}

	if !hasBlocking {
		t.Error("expected at least one blocking job")
	}

	if !hasInforming {
		t.Error("expected at least one informing job")
	}
}

func TestDefaultResources(t *testing.T) {
	t.Parallel()

	defaultResources := config.GetDefaultResources()

	if defaultResources.CPU != "1000m" {
		t.Errorf("default CPU: got %q, want 1000m", defaultResources.CPU)
	}

	if defaultResources.Memory != "3Gi" {
		t.Errorf("default memory: got %q, want 3Gi", defaultResources.Memory)
	}
}

func TestAllSuitesUseCorrectCluster(t *testing.T) {
	t.Parallel()

	for name, suite := range config.TestSuites() {
		if suite.Cluster != config.Cluster {
			t.Errorf("suite %q: cluster = %q, want %q", name, suite.Cluster, config.Cluster)
		}
	}
}

func TestConstants(t *testing.T) {
	t.Parallel()

	if config.DefaultImage == "" {
		t.Error("DefaultImage must not be empty")
	}

	if config.GCSLogPrefix == "" {
		t.Error("GCSLogPrefix must not be empty")
	}

	if config.Comment == "" {
		t.Error("Comment must not be empty")
	}

	if config.Cluster == "" {
		t.Error("Cluster must not be empty")
	}
}
