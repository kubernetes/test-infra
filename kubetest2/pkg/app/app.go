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
	// Now for the core kubetest2 logic:
	// - build
	// - cluster up
	// - test
	// - cluster down
	// TODO(bentheelder): write out structured metadata
	// TODO(bentheelder): signal handling & timeoutf

	// build if specified
	if opts.ShouldBuild() {
		build := d.GetBuilder()
		if build == nil {
			build = defaultBuild
		}
		// TODO(bentheelder): this should write out to JUnit
		if err := build(); err != nil {
			// we do not continue to up / test etc. if build fails
			return err
		}
	}

	// up a cluster
	if opts.ShouldUp() {
		// TODO(bentheelder): this should write out to JUnit
		if err := d.Up(); err != nil {
			// we do not continue to test if build fails
			return err
		}
	}

	// ensure tearing down the cluster happens last
	defer func() {
		if opts.ShouldDown() {
			// TODO(bentheelder): this should write out to JUnit
			d.Down()
		}
	}()

	// and finally test, if a test was specified
	if opts.ShouldTest() {
		// TODO(bentheelder): this should write out to JUnit
		tester.Test()
	}

	return nil
}

func defaultBuild() error {
	panic("unimplemented")
}
