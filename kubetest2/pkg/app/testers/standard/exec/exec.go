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

// Package exec implements a kubetest2 tester that simply executes the arguments
// as a subprocess
package exec

import (
	"os"

	"k8s.io/test-infra/kubetest2/pkg/app/testers"
	"k8s.io/test-infra/kubetest2/pkg/process"
	"k8s.io/test-infra/kubetest2/pkg/types"
)

const usage = `  [TestCommand] [TestArgs]

  TestCommand: the command to invoke for testing
  TestArgs:    arguments passed to test command
`

func init() {
	testers.Register("exec", usage, NewTester)
}

// Tester implements a kubetest2 types.Tester that exec's it's arguments
type Tester struct {
	argv []string
}

// NewTester creates a new Tester
func NewTester(common types.Options, testArgs []string, deployer types.Deployer) (types.Tester, error) {
	if len(testArgs) < 1 {
		return nil, types.NewIncorrectUsage("Error(exec): a TestCommand is required")
	}
	return &Tester{
		argv: testArgs,
	}, nil
}

// Test runs the "test" (executes the arguments)
func (t *Tester) Test() error {
	return process.ExecJUnit(t.argv[0], t.argv[1:], os.Environ())
}
