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

package app

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"k8s.io/test-infra/kubetest2/pkg/metadata"
	"k8s.io/test-infra/kubetest2/pkg/types"
)

// Main implements the kubetest2 deployer binary entrypoint
// Each deployer binary should invoke this, in addition to loading deployers
func Main(deployerName string, newDeployer types.NewDeployer) {
	// see cmd.go for the rest of the CLI boilerplate
	if err := Run(deployerName, newDeployer); err != nil {
		os.Exit(1)
	}
}

// RealMain contains nearly all of the application logic / control flow
// beyond the command line boilerplate
func RealMain(opts types.Options, d types.Deployer, tester types.Tester) error {
	/*
		Now for the core kubetest2 logic:
		 - build
		 - cluster up
		 - test
		 - cluster down
		Throughout this, collecting metadata and writing it out on exit
	*/
	// TODO(bentheelder): signal handling & timeout

	// setup the metadata writer
	junitRunner, err := os.Create(
		filepath.Join(opts.ArtifactsDir(), "junit_runner.xml"),
	)
	if err != nil {
		return errors.Wrap(err, "could not create runner output")
	}
	writer := metadata.NewWriter(junitRunner)
	// NOTE: defer is LIFO, so this should actually be the finish time
	defer func() {
		writer.Finish()
		junitRunner.Sync()
		junitRunner.Close()
	}()

	// build if specified
	if opts.ShouldBuild() {
		if err := writer.WrapStep("Build", d.Build); err != nil {
			// we do not continue to up / test etc. if build fails
			return err
		}
	}

	// up a cluster
	if opts.ShouldUp() {
		// TODO(bentheelder): this should write out to JUnit
		if err := writer.WrapStep("Up", d.Up); err != nil {
			// we do not continue to test if build fails
			return err
		}
	}

	// ensure tearing down the cluster happens last
	defer func() {
		if opts.ShouldDown() {
			writer.WrapStep("Down", d.Down)
		}
	}()

	// and finally test, if a test was specified
	if opts.ShouldTest() {
		writer.WrapStep("Test", tester.Test)
	}

	return nil
}
