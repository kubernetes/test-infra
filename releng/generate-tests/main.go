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

// generate-tests generates Prow periodic job configurations and TestGrid test
// group configurations for Kubernetes GCE COS e2e tests across all supported
// release branches. It replaces the former Python-based generate_tests.py and
// test_config.yaml system.
//
// Version information is auto-discovered from the release-branch-jobs directory.
package main

import (
	"errors"
	"flag"
	"log"

	"k8s.io/test-infra/releng/generate-tests/internal/generate"
	"k8s.io/test-infra/releng/generate-tests/internal/output"
	"k8s.io/test-infra/releng/generate-tests/internal/version"
)

var (
	errMissingReleaseBranchDir = errors.New("--release-branch-dir is required")
	errMissingOutputDir        = errors.New("--output-dir is required")
	errMissingTestgridOutput   = errors.New("--testgrid-output-path is required")
)

type options struct {
	releaseBranchDir   string
	outputDir          string
	testgridOutputPath string
}

func parseFlags() options {
	opts := options{
		releaseBranchDir:   "",
		outputDir:          "",
		testgridOutputPath: "",
	}
	flag.StringVar(&opts.releaseBranchDir, "release-branch-dir",
		"config/jobs/kubernetes/sig-release/release-branch-jobs",
		"Path to the release-branch-jobs directory")
	flag.StringVar(&opts.outputDir, "output-dir",
		"config/jobs/kubernetes/generated/",
		"Output directory for generated Prow job configs")
	flag.StringVar(&opts.testgridOutputPath, "testgrid-output-path",
		"config/testgrids/generated-test-config.yaml",
		"Output path for TestGrid test group configs")
	flag.Parse()

	return opts
}

func validateOptions(opts options) error {
	if opts.releaseBranchDir == "" {
		return errMissingReleaseBranchDir
	}

	if opts.outputDir == "" {
		return errMissingOutputDir
	}

	if opts.testgridOutputPath == "" {
		return errMissingTestgridOutput
	}

	return nil
}

func main() {
	opts := parseFlags()
	if err := validateOptions(opts); err != nil {
		log.Fatalln(err)
	}

	tiers, err := version.DiscoverVersions(opts.releaseBranchDir)
	if err != nil {
		log.Fatalf("Failed to discover versions: %v\n", err)
	}

	log.Printf("Discovered version tiers:")

	for _, tier := range tiers {
		log.Printf("  %s: %s (interval: %s)", tier.Marker, tier.Version.String(), tier.Interval)
	}

	prowConfig, tgc := generate.Jobs(tiers)

	if err := output.Write(prowConfig, tgc, opts.outputDir, opts.testgridOutputPath); err != nil {
		log.Fatalf("Failed to write output: %v\n", err)
	}

	log.Printf("Successfully generated %d jobs", len(prowConfig.Periodics))
}
