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
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

type options struct {
	owners            []string
	endpoint          string
	githubTokenPath   string
	usedDeprecatedOrg bool
}

func parseOptions() (*options, error) {
	var (
		ownersFlag string
		orgsFlag   string
		endpoint   string
		tokenPath  string
	)
	flag.StringVar(&ownersFlag, "owner", "", "Comma-separated list of GitHub owners (organizations or users) to scan.")
	flag.StringVar(&orgsFlag, "org", "", "Deprecated: comma-separated list of GitHub owners (organizations or users) to scan.")
	flag.StringVar(&endpoint, "endpoint", "https://api.github.com/", "GitHub API endpoint to use.")
	flag.StringVar(&tokenPath, "github-token-path", "", "Path to a file containing the GitHub token to use.")
	flag.Parse()

	owners, err := parseCommaSeparatedValues(ownersFlag)
	if err != nil {
		return nil, fmt.Errorf("flag --owner: %w", err)
	}

	deprecatedOwners, err := parseCommaSeparatedValues(orgsFlag)
	if err != nil {
		return nil, fmt.Errorf("flag --org: %w", err)
	}
	owners = append(owners, deprecatedOwners...)

	if len(owners) == 0 {
		return nil, fmt.Errorf("required flag --owner or --org was unset")
	}

	return &options{
		owners:            owners,
		endpoint:          endpoint,
		githubTokenPath:   tokenPath,
		usedDeprecatedOrg: len(deprecatedOwners) > 0,
	}, nil
}

func parseCommaSeparatedValues(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("contains an empty entry")
		}
		values = append(values, part)
	}
	return values, nil
}

func loadGitHubToken(opts *options) (string, error) {
	if strings.TrimSpace(opts.githubTokenPath) != "" {
		content, err := os.ReadFile(opts.githubTokenPath)
		if err != nil {
			return "", fmt.Errorf("read --github-token-path %q: %w", opts.githubTokenPath, err)
		}

		token := strings.TrimSpace(string(content))
		if token == "" {
			return "", fmt.Errorf("--github-token-path %q was empty", opts.githubTokenPath)
		}
		return token, nil
	}

	token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	if token == "" {
		return "", fmt.Errorf("required flag --github-token-path or environment variable GITHUB_TOKEN was unset")
	}
	return token, nil
}

func main() {
	log.SetPrefix("github-actions-policy-scanner: ")

	opts, err := parseOptions()
	if err != nil {
		log.Fatalf("invalid options: %v", err)
	}

	token, err := loadGitHubToken(opts)
	if err != nil {
		log.Fatalf("invalid GitHub token configuration: %v", err)
	}

	if opts.usedDeprecatedOrg {
		log.Printf("warning: --org is deprecated; use --owner")
	}

	s, err := newScanner(token, opts.endpoint)
	if err != nil {
		log.Fatalf("invalid scanner configuration: %v", err)
	}

	summary, err := s.run(context.Background(), opts.owners)
	if err != nil {
		log.Fatalf("scan failed: %v", err)
	}

	log.Printf(
		"summary owners=%d repos_seen=%d repos_scanned=%d repos_skipped=%d files_scanned=%d findings=%d scan_errors=%d",
		summary.OwnersScanned,
		summary.ReposSeen,
		summary.ReposScanned,
		summary.ReposSkipped,
		summary.FilesScanned,
		summary.Findings,
		summary.ScanErrors,
	)
}
