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

// prepare-release-branch orchestrates the creation of Kubernetes release branch
// job configurations. It rotates existing branch tiers and forks a new branch
// config using the config-rotator and config-forker libraries.
//
// Usage: prepare-release-branch
//
// The binary expects to be run from the repository root.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"k8s.io/test-infra/releng/prepare-release-branch/internal/release"
	"k8s.io/test-infra/releng/prepare-release-branch/internal/run"
)

const (
	branchJobDir = "config/jobs/kubernetes/sig-release/release-branch-jobs"
	jobConfigDir = "config/jobs"
)

var errBazelWorkspace = errors.New("please run via make rule: make -C releng prepare-release-branch")

func execute(ctx context.Context) error {
	version, err := release.CheckVersion(branchJobDir)
	if err != nil {
		return fmt.Errorf("checking version: %w", err)
	}

	log.Printf("Current version: %s", version.String())

	next := release.Version{Major: version.Major, Minor: version.Minor + 1}

	goVersion, err := run.FetchGoVersion(ctx, run.GoVersionURL(version))
	if errors.Is(err, run.ErrBranchNotFound) {
		log.Printf("Release branch for %s does not exist yet, nothing to do", next.String())

		return nil
	}

	if err != nil {
		return fmt.Errorf("fetching Go version: %w", err)
	}

	log.Printf("Next version: %s (Go %s)", next.String(), goVersion)

	log.Println("Rotating files...")

	if err := run.RotateFiles(branchJobDir, version); err != nil {
		return fmt.Errorf("rotating files: %w", err)
	}

	log.Println("Forking new file...")

	if err := run.ForkNewFile(ctx, branchJobDir, jobConfigDir, version, goVersion); err != nil {
		return fmt.Errorf("forking new file: %w", err)
	}

	log.Println("Done!")

	return nil
}

func main() {
	if os.Getenv("BUILD_WORKSPACE_DIRECTORY") != "" {
		log.Fatalln(errBazelWorkspace)
	}

	if err := execute(context.Background()); err != nil {
		log.Fatalln(err)
	}
}
