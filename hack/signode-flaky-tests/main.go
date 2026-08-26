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
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"sigs.k8s.io/yaml"
)

type FlakyTest struct {
	Name      string   `json:"name"`
	Issue     string   `json:"issue"`
	DateAdded string   `json:"dateAdded"`
	Suite     string   `json:"suite,omitempty"`
	Feature   string   `json:"feature,omitempty"`
	Reason    string   `json:"reason,omitempty"`
	Jobs      []string `json:"jobs,omitempty"`
}

type FlakyTestConfig struct {
	FlakyTests []FlakyTest `json:"flakyTests"`
}

func main() {
	os.Exit(run(os.Stdout, os.Stderr, os.Args[1:]))
}

func run(stdout, stderr io.Writer, args []string) int {
	fs := flag.NewFlagSet("signode-flaky-tests", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "", "Path to flaky-tests.yaml")
	mode := fs.String("mode", "validate", "Mode: validate, skip-regex")
	suite := fs.String("suite", "", "Filter by test suite: node_e2e, e2e")
	job := fs.String("job", "", "Filter by job name")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *configPath == "" {
		fmt.Fprintln(stderr, "error: --config is required")
		return 1
	}

	config, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	switch *mode {
	case "validate":
		errs := validate(config)
		if len(errs) > 0 {
			for _, e := range errs {
				fmt.Fprintf(stderr, "error: %s\n", e)
			}
			return 1
		}
		fmt.Fprintf(stdout, "ok: %d flaky test entries validated\n", len(config.FlakyTests))

	case "skip-regex":
		regex := generateSkipRegex(config, *suite, *job)
		if regex != "" {
			fmt.Fprint(stdout, regex)
		}

	default:
		fmt.Fprintf(stderr, "error: unknown mode %q (use validate or skip-regex)\n", *mode)
		return 1
	}

	return 0
}

func loadConfig(path string) (*FlakyTestConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var config FlakyTestConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &config, nil
}

func validate(config *FlakyTestConfig) []string {
	var errors []string
	seen := make(map[string]bool)

	for i, t := range config.FlakyTests {
		prefix := fmt.Sprintf("flakyTests[%d]", i)

		if t.Name == "" {
			errors = append(errors, fmt.Sprintf("%s: name is required", prefix))
		} else {
			if _, err := regexp.Compile(t.Name); err != nil {
				errors = append(errors, fmt.Sprintf("%s: name is not a valid regex: %v", prefix, err))
			}
			if seen[t.Name] {
				errors = append(errors, fmt.Sprintf("%s: duplicate name %q", prefix, t.Name))
			}
			seen[t.Name] = true
		}

		if t.Issue == "" {
			errors = append(errors, fmt.Sprintf("%s: issue is required", prefix))
		} else if !strings.HasPrefix(t.Issue, "https://github.com/") {
			errors = append(errors, fmt.Sprintf("%s: issue must be a GitHub URL, got %q", prefix, t.Issue))
		}

		if t.DateAdded == "" {
			errors = append(errors, fmt.Sprintf("%s: dateAdded is required", prefix))
		} else if _, err := time.Parse("2006-01-02", t.DateAdded); err != nil {
			errors = append(errors, fmt.Sprintf("%s: dateAdded must be YYYY-MM-DD, got %q", prefix, t.DateAdded))
		}

		if t.Suite != "" && t.Suite != "node_e2e" && t.Suite != "e2e" {
			errors = append(errors, fmt.Sprintf("%s: suite must be \"node_e2e\" or \"e2e\", got %q", prefix, t.Suite))
		}
	}

	return errors
}

func generateSkipRegex(config *FlakyTestConfig, suite string, job string) string {
	var patterns []string
	for _, t := range config.FlakyTests {
		if suite != "" && t.Suite != "" && t.Suite != suite {
			continue
		}
		if job != "" && len(t.Jobs) > 0 && !slices.Contains(t.Jobs, job) {
			continue
		}
		patterns = append(patterns, t.Name)
	}
	return strings.Join(patterns, "|")
}
