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

// Package types defines the common types / interfaces for kubetest2 deployer
// and tester implementations
package types

// IncorrectUsage is an error with an addition HelpText() method
// NewDeployer and NewTester implementations should return a type meeting this
// interface if they want to display usage to the user when incorrect arguments
// or flags are supplied
type IncorrectUsage interface {
	error
	HelpText() string
}

// NewDeployer should process & store deployerArgs and the common Options
// kubetest2 will call this once at startup
// common will provide access to options defined by common flags and kubetest2
// logic, while deployerArgs provides all unknown arguments passed to kubetest2
// before the first bare `--` if any.
// When incorrect arguments or flags are supplied, the IncorrectUsage superset
// of error can be returned. kubetest2 will display the HelpText() output
type NewDeployer func(common Options, deployerArgs []string) (Deployer, error)

// Options is an interface to get common options supplied by kubetest2
// to all implementations
type Options interface {
	// TODO(BenTheElder): provide getters to more common options
	// if this returns true, help text will be shown to the user after instancing
	// the deployer and tester
	HelpRequested() bool
	ShouldBuild() bool
	ShouldUp() bool
	ShouldDown() bool
	ShouldTest() bool
}

// Deployer defines the interface between kubetest and a deployer
type Deployer interface {
	// Up should provision a new cluster for testing
	Up() error
	// Down should tear down the test cluster if any
	Down() error
	// IsUp should return true if a test cluster is successfully provisioned
	IsUp() (up bool, err error)
	// DumpClusterLogs should export logs from the cluster. It may be called
	// multiple times. Options for this should come from New(...)
	DumpClusterLogs() error
	// GetBuilder should return a custom build instance for the implementation
	// It may return a custom impelementation, a common implementation provided
	// by the kubetest2 packages, or nil. If nil is returned a default builder
	// will be used
	GetBuilder() Build
}

// Build should build kubernetes and package it in whatever format
// the deployer consumes
type Build func() error

// NewTester should process & store deployerArgs and the common Options
// kubetest2 will call this once at startup
// common will provide access to options defined by common flags and kubetest2
// logic, while testArgs provides all arguments passed to kubetest2
// after the first bare `--` if any.
// When incorrect arguments or flags are supplied, the IncorrectUsage superset
// of error can be returned. kubetest2 will display the HelpText() output
type NewTester func(common Options, testArgs []string, deployer Deployer) (Tester, error)

// Tester defines the interface between kubetest2 and a tester
type Tester interface {
	Test() error
}
