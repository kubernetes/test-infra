/*
Copyright 2020 The Kubernetes Authors.

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

// Package deployer implements the kubetest2 GKE deployer
package deployer

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"

	"k8s.io/test-infra/kubetest2/pkg/exec"
	"k8s.io/test-infra/kubetest2/pkg/types"
)

// Name is the name of the deployer
const Name = "gce"

type deployer struct {
	// generic parts
	commonOptions types.Options

	repoRootPath string
}

// New implements deployer.New for gce
func New(opts types.Options) (types.Deployer, *pflag.FlagSet) {
	d := &deployer{
		commonOptions: opts,
	}

	// register flags and return
	return d, bindFlags(d)
}

// assert that New implements types.NewDeployer
var _ types.NewDeployer = New

func bindFlags(d *deployer) *pflag.FlagSet {
	flags := pflag.NewFlagSet(Name, pflag.ContinueOnError)

	flags.StringVar(&d.repoRootPath, "repo-root-path", "", "The path to the local kubernetes/cloud-provider/gcp repo. Necessary to call certain scripts. Defaults to the current directory. If operating in legacy mode, this should be set to the local kubernetes/kubernetes repo.")
	return flags
}

// assert that deployer implements types.Deployer
var _ types.Deployer = &deployer{}

func (d *deployer) Provider() string {
	return "gce"
}

func (d *deployer) Build() error {
	if err := d.verifyBuildFlags(); err != nil {
		return err
	}

	err := os.Chdir(d.repoRootPath)
	if err != nil {
		return err
	}

	// this code path supports the kubernetes/cloud-provider-gcp build
	// TODO: update in future patch to support legacy (k/k) build
	cmd := exec.Command("bazel", "build", "//release:release-tars")
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("error during make step of build: %s", err)
	}

	// no untarring, uploading, etc is necessary because
	// kube-up/down use find-release-tars and upload-tars
	// which know how to find the tars, assuming KUBE_ROOT
	// is set

	return nil
}

func (d *deployer) Up() error { return nil }

func (d *deployer) IsUp() (up bool, err error) { return false, nil }

func (d *deployer) DumpClusterLogs() error { return nil }

func (d *deployer) TestSetup() error { return nil }

func (d *deployer) Kubeconfig() (string, error) { return "", nil }

func (d *deployer) Down() error { return nil }
