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

package ginkgo

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"k8s.io/test-infra/kubetest2/pkg/app/testers"
	"k8s.io/test-infra/kubetest2/pkg/app/testers/standard/ginkgo/kubectl"
	"k8s.io/test-infra/kubetest2/pkg/exec"
	"k8s.io/test-infra/kubetest2/pkg/types"

	"github.com/spf13/pflag"
)

const (
	binary = "ginkgo" // TODO(RonWeber): Actually find these binaries.
)

var (
	kubeRoot    string
	e2eTestPath = filepath.Join("kubernetes", "test", "bin", "e2e.test")
)

const usage = `--flake-attempts Make up to this many attempts to run each spec.
--parallel Run this many tests in parallel at once.  Defaults to 1 (Not parallel)
--skip Regular expression of jobs to skip.`

func init() {
	testers.Register("ginkgo", usage, NewTester)
}

// Tester implements a kubetest2 types.Tester that exec's it's arguments
type Tester struct {
	conformanceTest bool

	flakeAttempts string
	parallel      string
	skipRegex     string
	focusRegex    string

	host           string
	kubeconfigPath string
	provider       string

	deployer types.Deployer
}

// NewTester creates a new Tester
func NewTester(common types.Options, testArgs []string, deployer types.Deployer) (types.Tester, error) {
	wd, _ := os.Getwd()
	kubeRoot = filepath.Join(wd, "kubernetes")

	t := Tester{}
	t.deployer = deployer

	flags := bindFlags(&t)
	flags.Parse(testArgs)

	t.conformanceTest = true //TODO(RonWeber): Better logic here.

	return &t, nil
}

func bindFlags(t *Tester) *pflag.FlagSet {
	flags := pflag.NewFlagSet("ginkgo tester", pflag.ContinueOnError)
	flags.StringVar(&t.flakeAttempts, "flake-attempts", "1", "Make up to this many attempts to run each spec.")
	flags.StringVar(&t.parallel, "parallel", "1", "Run this many tests in parallel at once.  Defaults to 1 (Not parallel)")
	flags.StringVar(&t.skipRegex, "skip", "", "Regular expression of jobs to skip.") //TODO(RonWeber): Some of that can be detected automatically.
	flags.StringVar(&t.focusRegex, "focus", "", "Regular expression of jobs to focus on.")
	return flags
}

// Test runs the test
func (t *Tester) Test() error {
	if err := t.pretestSetup(); err != nil {
		return err
	}

	e2eTestArgs := []string{
		"--host=" + t.host,
		"--provider=" + t.provider,
		"--kubeconfig=" + t.kubeconfigPath,
		"--ginkgo.flakeAttempts=" + t.flakeAttempts,
		"--ginkgo.skip=" + t.skipRegex,
		"--ginkgo.focus=" + t.focusRegex,
	}
	ginkgoArgs := append([]string{
		"--nodes=" + t.parallel,
		e2eTestPath,
		"--"}, e2eTestArgs...)

	log.Printf("Running ginkgo test as %s %+v", binary, ginkgoArgs)
	cmd := exec.Command(binary, ginkgoArgs...)
	exec.InheritOutput(cmd)
	return cmd.Run()
}

func (t *Tester) pretestSetup() error {
	host, err := kubectl.APIServerURL()
	if err != nil {
		return fmt.Errorf("Could not get master URL: %v", err)
	}
	t.host = host

	t.provider = "skeleton"
	dp, ok := t.deployer.(types.DeployerWithProvider)
	if !t.conformanceTest && ok {
		t.provider = dp.Provider()
	}

	dk, ok := t.deployer.(types.DeployerWithKubeconfig)
	if ok { // Call Kubeconfig() if possible
		d, err := dk.Kubeconfig()
		if err != nil {
			return err
		}
		t.kubeconfigPath = d
	} else { // Otherwise fall back to $KUBECONFIG
		t.kubeconfigPath = os.Getenv("KUBECONFIG")
	}
	log.Printf("Using kubeconfig at %s", t.kubeconfigPath)

	return nil
}
