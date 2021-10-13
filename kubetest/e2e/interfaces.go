/*
Copyright 2018 The Kubernetes Authors.

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

package e2e

import (
	"k8s.io/test-infra/kubetest/process"
)

// Tester is implemented by runners that run our tests
type Tester interface {
	Run(control *process.Control, args []string) error
}

// TestBuilder is implemented by deployers that want to customize how the e2e tests are run
type TestBuilder interface {
	// BuildTester builds the appropriate Tester object for running tests
	BuildTester(options *BuildTesterOptions) (Tester, error)
}

// BuildTesterOptions is the options struct that should be passed to testBuilder::BuildTester
type BuildTesterOptions struct {
	FocusRegex            string
	SkipRegex             string
	StorageTestDriverPath string
	Parallelism           int
}
