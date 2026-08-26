/*
Copyright The Kubernetes Authors.

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
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name       string
		config     FlakyTestConfig
		wantErrors int
		wantSubstr []string
	}{
		{
			name:       "empty config is valid",
			config:     FlakyTestConfig{},
			wantErrors: 0,
		},
		{
			name: "valid entry with required fields only",
			config: FlakyTestConfig{
				FlakyTests: []FlakyTest{{
					Name:      "should do something",
					Issue:     "https://github.com/kubernetes/kubernetes/issues/12345",
					DateAdded: "2026-08-26",
				}},
			},
			wantErrors: 0,
		},
		{
			name: "valid entry with all fields",
			config: FlakyTestConfig{
				FlakyTests: []FlakyTest{{
					Name:      "should do something",
					Issue:     "https://github.com/kubernetes/kubernetes/issues/12345",
					DateAdded: "2026-08-26",
					Suite:     "node_e2e",
					Feature:   "SidecarContainers",
					Reason:    "Flakes on COS",
					Jobs:      []string{"ci-cos-containerd-node-e2e-serial"},
				}},
			},
			wantErrors: 0,
		},
		{
			name: "valid regex pattern as name",
			config: FlakyTestConfig{
				FlakyTests: []FlakyTest{{
					Name:      `should (create|delete) pods in \d+ seconds`,
					Issue:     "https://github.com/kubernetes/kubernetes/issues/12345",
					DateAdded: "2026-08-26",
				}},
			},
			wantErrors: 0,
		},
		{
			name: "valid issue from non-k/k repo",
			config: FlakyTestConfig{
				FlakyTests: []FlakyTest{{
					Name:      "should work",
					Issue:     "https://github.com/kubernetes/test-infra/issues/99999",
					DateAdded: "2026-08-26",
				}},
			},
			wantErrors: 0,
		},
		{
			name: "missing name",
			config: FlakyTestConfig{
				FlakyTests: []FlakyTest{{
					Issue:     "https://github.com/kubernetes/kubernetes/issues/12345",
					DateAdded: "2026-08-26",
				}},
			},
			wantErrors: 1,
			wantSubstr: []string{"name is required"},
		},
		{
			name: "missing issue",
			config: FlakyTestConfig{
				FlakyTests: []FlakyTest{{
					Name:      "should do something",
					DateAdded: "2026-08-26",
				}},
			},
			wantErrors: 1,
			wantSubstr: []string{"issue is required"},
		},
		{
			name: "missing dateAdded",
			config: FlakyTestConfig{
				FlakyTests: []FlakyTest{{
					Name:  "should do something",
					Issue: "https://github.com/kubernetes/kubernetes/issues/12345",
				}},
			},
			wantErrors: 1,
			wantSubstr: []string{"dateAdded is required"},
		},
		{
			name: "all required fields missing",
			config: FlakyTestConfig{
				FlakyTests: []FlakyTest{{}},
			},
			wantErrors: 3,
			wantSubstr: []string{"name is required", "issue is required", "dateAdded is required"},
		},
		{
			name: "invalid issue URL",
			config: FlakyTestConfig{
				FlakyTests: []FlakyTest{{
					Name:      "should do something",
					Issue:     "not-a-github-url",
					DateAdded: "2026-08-26",
				}},
			},
			wantErrors: 1,
			wantSubstr: []string{"must be a GitHub URL"},
		},
		{
			name: "invalid dateAdded format",
			config: FlakyTestConfig{
				FlakyTests: []FlakyTest{{
					Name:      "should do something",
					Issue:     "https://github.com/kubernetes/kubernetes/issues/12345",
					DateAdded: "yesterday",
				}},
			},
			wantErrors: 1,
			wantSubstr: []string{"dateAdded must be YYYY-MM-DD"},
		},
		{
			name: "invalid suite value",
			config: FlakyTestConfig{
				FlakyTests: []FlakyTest{{
					Name:      "should do something",
					Issue:     "https://github.com/kubernetes/kubernetes/issues/12345",
					DateAdded: "2026-08-26",
					Suite:     "integration",
				}},
			},
			wantErrors: 1,
			wantSubstr: []string{`suite must be`},
		},
		{
			name: "valid with feature gate",
			config: FlakyTestConfig{
				FlakyTests: []FlakyTest{{
					Name:      "should do something",
					Issue:     "https://github.com/kubernetes/kubernetes/issues/12345",
					DateAdded: "2026-08-26",
					Feature:   "SidecarContainers",
				}},
			},
			wantErrors: 0,
		},
		{
			name: "invalid regex in name",
			config: FlakyTestConfig{
				FlakyTests: []FlakyTest{{
					Name:      "[invalid",
					Issue:     "https://github.com/kubernetes/kubernetes/issues/12345",
					DateAdded: "2026-08-26",
				}},
			},
			wantErrors: 1,
			wantSubstr: []string{"not a valid regex"},
		},
		{
			name: "duplicate names",
			config: FlakyTestConfig{
				FlakyTests: []FlakyTest{
					{Name: "same name", Issue: "https://github.com/kubernetes/kubernetes/issues/1", DateAdded: "2026-08-26"},
					{Name: "same name", Issue: "https://github.com/kubernetes/kubernetes/issues/2", DateAdded: "2026-08-26"},
				},
			},
			wantErrors: 1,
			wantSubstr: []string{"duplicate name"},
		},
		{
			name: "errors in multiple entries are all reported",
			config: FlakyTestConfig{
				FlakyTests: []FlakyTest{
					{Name: "valid", Issue: "https://github.com/kubernetes/kubernetes/issues/1", DateAdded: "2026-08-26"},
					{Name: "", Issue: "https://github.com/kubernetes/kubernetes/issues/2", DateAdded: "2026-08-26"},
					{Name: "also valid", Issue: "", DateAdded: "2026-08-26"},
				},
			},
			wantErrors: 2,
			wantSubstr: []string{"flakyTests[1]", "flakyTests[2]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validate(&tt.config)
			if len(errs) != tt.wantErrors {
				t.Errorf("got %d errors, want %d: %v", len(errs), tt.wantErrors, errs)
			}
			for _, substr := range tt.wantSubstr {
				found := false
				for _, e := range errs {
					if strings.Contains(e, substr) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error containing %q, got: %v", substr, errs)
				}
			}
		})
	}
}

func TestGenerateSkipRegex(t *testing.T) {
	config := &FlakyTestConfig{
		FlakyTests: []FlakyTest{
			{Name: "unscoped", Issue: "https://github.com/kubernetes/kubernetes/issues/1", DateAdded: "2026-08-26"},
			{Name: "node only", Issue: "https://github.com/kubernetes/kubernetes/issues/2", DateAdded: "2026-08-26", Suite: "node_e2e"},
			{Name: "job scoped", Issue: "https://github.com/kubernetes/kubernetes/issues/3", DateAdded: "2026-08-26", Jobs: []string{"job-A"}},
			{Name: "e2e only", Issue: "https://github.com/kubernetes/kubernetes/issues/4", DateAdded: "2026-08-26", Suite: "e2e"},
			{Name: "node+job", Issue: "https://github.com/kubernetes/kubernetes/issues/5", DateAdded: "2026-08-26", Suite: "node_e2e", Jobs: []string{"job-A", "job-B"}},
		},
	}

	tests := []struct {
		name      string
		suite     string
		job       string
		wantParts []string
	}{
		{"no filters", "", "", []string{"unscoped", "node only", "job scoped", "e2e only", "node+job"}},
		{"suite=e2e", "e2e", "", []string{"unscoped", "job scoped", "e2e only"}},
		{"suite=node_e2e", "node_e2e", "", []string{"unscoped", "node only", "job scoped", "node+job"}},
		{"matching job", "", "job-A", []string{"unscoped", "node only", "job scoped", "e2e only", "node+job"}},
		{"non-matching job", "", "job-X", []string{"unscoped", "node only", "e2e only"}},
		{"second of multi-job", "", "job-B", []string{"unscoped", "node only", "e2e only", "node+job"}},
		{"suite+job combined", "node_e2e", "job-A", []string{"unscoped", "node only", "job scoped", "node+job"}},
		{"all filtered out", "e2e", "job-X", []string{"unscoped", "e2e only"}},
		{"empty config", "", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config
			if tt.name == "empty config" {
				cfg = &FlakyTestConfig{}
			}
			got := generateSkipRegex(cfg, tt.suite, tt.job)
			gotParts := splitNonEmpty(got)

			if len(gotParts) != len(tt.wantParts) {
				t.Fatalf("got %d patterns %v, want %d patterns %v", len(gotParts), gotParts, len(tt.wantParts), tt.wantParts)
			}
			for i, want := range tt.wantParts {
				if gotParts[i] != want {
					t.Errorf("pattern[%d] = %q, want %q", i, gotParts[i], want)
				}
			}
		})
	}
}

func TestGenerateSkipRegex_OutputIsValidRegex(t *testing.T) {
	config := &FlakyTestConfig{
		FlakyTests: []FlakyTest{
			{Name: "should handle pod (creation|deletion)", Issue: "https://github.com/kubernetes/kubernetes/issues/1", DateAdded: "2026-08-26"},
			{Name: `should timeout after \d+ seconds`, Issue: "https://github.com/kubernetes/kubernetes/issues/2", DateAdded: "2026-08-26"},
		},
	}
	got := generateSkipRegex(config, "", "")
	if _, err := regexp.Compile(got); err != nil {
		t.Errorf("generated regex %q is not valid: %v", got, err)
	}
}

func TestLoadConfig(t *testing.T) {
	config := loadConfigFromString(t, `
flakyTests:
  - name: "should do something"
    issue: "https://github.com/kubernetes/kubernetes/issues/12345"
    dateAdded: "2026-08-26"
    suite: "node_e2e"
    reason: "Flakes on COS"
    jobs:
      - ci-cos-containerd-node-e2e-serial
`)
	if len(config.FlakyTests) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(config.FlakyTests))
	}

	e := config.FlakyTests[0]
	if e.Name != "should do something" {
		t.Errorf("name = %q", e.Name)
	}
	if e.Issue != "https://github.com/kubernetes/kubernetes/issues/12345" {
		t.Errorf("issue = %q", e.Issue)
	}
	if e.DateAdded != "2026-08-26" {
		t.Errorf("dateAdded = %q", e.DateAdded)
	}
	if e.Suite != "node_e2e" {
		t.Errorf("suite = %q", e.Suite)
	}
	if e.Reason != "Flakes on COS" {
		t.Errorf("reason = %q", e.Reason)
	}
	if len(e.Jobs) != 1 || e.Jobs[0] != "ci-cos-containerd-node-e2e-serial" {
		t.Errorf("jobs = %v", e.Jobs)
	}
}

func TestLoadConfig_Errors(t *testing.T) {
	if _, err := loadConfig("/nonexistent/path/flaky-tests.yaml"); err == nil {
		t.Fatal("expected error for missing file")
	}

	dir := t.TempDir()
	badPath := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(badPath, []byte("not: valid: yaml: [[["), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(badPath); err == nil {
		t.Fatal("expected error for invalid YAML")
	}

	emptyPath := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(emptyPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	config, err := loadConfig(emptyPath)
	if err != nil {
		t.Fatalf("empty file should parse without error: %v", err)
	}
	if len(config.FlakyTests) != 0 {
		t.Errorf("expected 0 entries from empty file, got %d", len(config.FlakyTests))
	}
}

func TestGenerateSkipRegex_MatchesRealTestNames(t *testing.T) {
	config := &FlakyTestConfig{
		FlakyTests: []FlakyTest{
			{Name: "should not allow privilege escalation when false", Issue: "https://github.com/kubernetes/kubernetes/issues/1", DateAdded: "2026-08-26"},
			{Name: "should support pod level resources", Issue: "https://github.com/kubernetes/kubernetes/issues/2", DateAdded: "2026-08-26"},
		},
	}

	re := regexp.MustCompile(generateSkipRegex(config, "", ""))

	for _, name := range []string{
		"[sig-node] Security Context should not allow privilege escalation when false [NodeConformance]",
		"[sig-node] Pods should support pod level resources [Feature:PodLevelResources]",
	} {
		if !re.MatchString(name) {
			t.Errorf("should match %q", name)
		}
	}
	for _, name := range []string{
		"[sig-node] Security Context should allow privilege escalation when true [NodeConformance]",
		"[sig-node] Pods should create a pod",
	} {
		if re.MatchString(name) {
			t.Errorf("should NOT match %q", name)
		}
	}
}

func TestGenerateSkipRegex_CombinedWithExistingSkip(t *testing.T) {
	config := &FlakyTestConfig{
		FlakyTests: []FlakyTest{{
			Name: "should handle eviction", Issue: "https://github.com/kubernetes/kubernetes/issues/1", DateAdded: "2026-08-26",
		}},
	}

	combined := `\[Flaky\]|\[Slow\]` + "|" + generateSkipRegex(config, "", "")
	re := regexp.MustCompile(combined)

	if !re.MatchString("[Flaky] some test") {
		t.Error("should match [Flaky]")
	}
	if !re.MatchString("kubelet should handle eviction properly") {
		t.Error("should match flaky test")
	}
	if re.MatchString("should create pods normally") {
		t.Error("should not match unrelated test")
	}
}

func TestValidateRealConfig(t *testing.T) {
	configPath := filepath.Join("flaky-tests.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Skip("flaky-tests.yaml not found")
	}

	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load real config: %v", err)
	}
	for _, e := range validate(config) {
		t.Errorf("validation error: %s", e)
	}
}

func TestRun_Validate(t *testing.T) {
	validPath := writeTestConfig(t, `
flakyTests:
  - name: "test one"
    issue: "https://github.com/kubernetes/kubernetes/issues/1"
    dateAdded: "2026-08-26"
  - name: "test two"
    issue: "https://github.com/kubernetes/kubernetes/issues/2"
    dateAdded: "2026-08-26"
`)
	invalidPath := writeTestConfig(t, `
flakyTests:
  - name: ""
    issue: ""
    dateAdded: ""
`)

	t.Run("success", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run(&stdout, &stderr, []string{"--config=" + validPath, "--mode=validate"}); code != 0 {
			t.Fatalf("exit %d; stderr: %s", code, stderr.String())
		}
		if got := stdout.String(); got != "ok: 2 flaky test entries validated\n" {
			t.Errorf("stdout = %q", got)
		}
	})

	t.Run("failure", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run(&stdout, &stderr, []string{"--config=" + invalidPath, "--mode=validate"}); code != 1 {
			t.Fatalf("expected exit 1, got %d", code)
		}
		if stdout.Len() != 0 {
			t.Errorf("expected no stdout, got: %s", stdout.String())
		}
		for _, want := range []string{"name is required", "issue is required", "dateAdded is required"} {
			if !strings.Contains(stderr.String(), want) {
				t.Errorf("stderr missing %q", want)
			}
		}
	})

	t.Run("default mode is validate", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run(&stdout, &stderr, []string{"--config=" + validPath}); code != 0 {
			t.Fatalf("exit %d; stderr: %s", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "ok: 2 flaky test entries validated") {
			t.Errorf("stdout = %q", stdout.String())
		}
	})
}

func TestRun_SkipRegex(t *testing.T) {
	path := writeTestConfig(t, `
flakyTests:
  - name: "job-scoped"
    issue: "https://github.com/kubernetes/kubernetes/issues/1"
    dateAdded: "2026-08-26"
    jobs:
      - job-A
  - name: "node only"
    issue: "https://github.com/kubernetes/kubernetes/issues/2"
    dateAdded: "2026-08-26"
    suite: "node_e2e"
  - name: "e2e only"
    issue: "https://github.com/kubernetes/kubernetes/issues/3"
    dateAdded: "2026-08-26"
    suite: "e2e"
`)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unfiltered", nil, "job-scoped|node only|e2e only"},
		{"suite=e2e", []string{"--suite=e2e"}, "job-scoped|e2e only"},
		{"suite=node_e2e", []string{"--suite=node_e2e"}, "job-scoped|node only"},
		{"matching job", []string{"--job=job-A"}, "job-scoped|node only|e2e only"},
		{"non-matching job", []string{"--job=job-X"}, "node only|e2e only"},
		{"all filtered out", []string{"--suite=e2e", "--job=job-X"}, "e2e only"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := append([]string{"--config=" + path, "--mode=skip-regex"}, tt.args...)
			if code := run(&stdout, &stderr, args); code != 0 {
				t.Fatalf("exit %d; stderr: %s", code, stderr.String())
			}
			if got := stdout.String(); got != tt.want {
				t.Errorf("stdout = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRun_Errors(t *testing.T) {
	validPath := writeTestConfig(t, `flakyTests: []`)

	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{"missing config", []string{"--mode=validate"}, "--config is required"},
		{"nonexistent config", []string{"--config=/nonexistent.yaml"}, "reading config"},
		{"unknown mode", []string{"--config=" + validPath, "--mode=bad"}, "unknown mode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(&stdout, &stderr, tt.args); code != 1 {
				t.Fatalf("expected exit 1, got %d", code)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want substring %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

// helpers

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "flaky-tests.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func loadConfigFromString(t *testing.T, content string) *FlakyTestConfig {
	t.Helper()
	path := writeTestConfig(t, content)
	config, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	return config
}

func splitNonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "|")
}
