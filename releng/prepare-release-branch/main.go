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
// job configurations. It calls config-rotator, config-forker, and generate-tests
// as subprocesses to rotate existing branch tiers, fork a new branch config,
// and regenerate test definitions.
//
// Usage: prepare-release-branch <config-rotator> <config-forker> <generate-tests>
//
// The binary expects to be run from the repository root (set by run.sh).
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
	requiredArgs = 4 // program name + 3 tool binaries
)

var (
	errBazelWorkspace = errors.New("please run via make rule: make -C releng prepare-release-branch")
	errUsage          = errors.New("invalid arguments")
)

type options struct {
	rotatorBin       string
	forkerBin        string
	generateTestsBin string
}

func parseArgs(args []string) (options, error) {
	if len(args) != requiredArgs {
		name := "prepare-release-branch"
		if len(args) > 0 {
			name = args[0]
		}

		return options{}, fmt.Errorf(
			"%w: usage: %s <config-rotator> <config-forker> <generate-tests>", errUsage, name,
		)
	}

	return options{
		rotatorBin:       args[1],
		forkerBin:        args[2],
		generateTestsBin: args[3],
	}, nil
}

func execute(ctx context.Context) error {
	opts, err := parseArgs(os.Args)
	if err != nil {
		return err
	}

	version, err := release.CheckVersion(branchJobDir)
	if err != nil {
		return fmt.Errorf("checking version: %w", err)
	}

	log.Printf("Current version: %s", version.String())

	goVersion, err := run.FetchGoVersion(ctx, run.GoVersionURL)
	if err != nil {
		return fmt.Errorf("fetching Go version: %w", err)
	}

	log.Printf("Current Go version: %s", goVersion)

	commander := &run.ExecCommander{Stdout: os.Stdout, Stderr: os.Stderr}

	log.Println("Rotating files...")

	if err := run.RotateFiles(ctx, commander, opts.rotatorBin, branchJobDir, version); err != nil {
		return fmt.Errorf("rotating files: %w", err)
	}

	log.Println("Forking new file...")

	if err := run.ForkNewFile(
		ctx, commander, opts.forkerBin, branchJobDir, jobConfigDir, version, goVersion,
	); err != nil {
		return fmt.Errorf("forking new file: %w", err)
	}

	log.Println("Regenerating files...")

	if err := run.RegenerateFiles(ctx, commander, opts.generateTestsBin, branchJobDir); err != nil {
		return fmt.Errorf("regenerating files: %w", err)
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
