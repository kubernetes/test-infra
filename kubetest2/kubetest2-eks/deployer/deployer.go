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

// Package deployer implements the kubetest2 EKS deployer
package deployer

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"
	"k8s.io/test-infra/kubetest2/pkg/exec"
	"k8s.io/test-infra/kubetest2/pkg/metadata"
	"k8s.io/test-infra/kubetest2/pkg/process"
	"k8s.io/test-infra/kubetest2/pkg/types"
	utilpath "k8s.io/utils/path"
)

// Name is the name of the deployer
const Name = "eks"

// deployer implements EKS deployer interface using "aws-k8s-tester" binary.
//
// Note that you can customize configuration via environment variables, e.g.
//
//  export AWS_K8S_TESTER_EKS_NAME=kubetest2
//  export AWS_K8S_TESTER_EKS_ADD_ON_NLB_HELLO_WORLD_ENABLE="false"
//  export AWS_K8S_TESTER_EKS_ADD_ON_MANAGED_NODE_GROUPS_MNGS='{"aws-k8s-tester-kubetest2-mng":{"name":"aws-k8s-tester-kubetest2-mng","ami-type":"AL2_x86_64","asg-min-size":3,"asg-max-size":3,"asg-desired-capacity":3,"instance-types":["a1.xlarge"],"volume-size":40}}'
//
// Refer to https://github.com/aws/aws-k8s-tester for more information.
type deployer struct {
	// generic parts
	commonOptions types.Options
	// eks specific
	configPath string
}

// assert that deployer implements types.Deployer
var _ types.Deployer = &deployer{}

// New implements deployer.New for gke
func New(opts types.Options) (types.Deployer, *pflag.FlagSet) {
	// create a deployer object and set fields that are not flag controlled
	d := &deployer{
		commonOptions: opts,
	}

	// register flags and return
	return d, bindFlags(d)
}

// assert that New implements types.NewDeployer
var _ types.NewDeployer = New

// verifyFlags validates that required flags are set
func (d *deployer) verifyFlags() error {
	if d.configPath == "" {
		return fmt.Errorf("--config must not be empty")
	}
	return nil
}

func defaultConfigPath() string {
	configPath, set := os.LookupEnv("AWS_K8S_TESTER_EKS_CONFIG_PATH")
	if set {
		return configPath
	}
	return "/tmp/kubetest2.eks.config"
}

func bindFlags(d *deployer) *pflag.FlagSet {
	flags := pflag.NewFlagSet(Name, pflag.ContinueOnError)
	flags.StringVar(
		&d.configPath, "config", defaultConfigPath(), "Configuration file for aws-k8s-tester, defaulting to ${AWS_K8S_TESTER_EKS_CONFIG_PATH:-/tmp/kubetest2.eks.config}.",
	)
	return flags
}

func (d *deployer) Provider() string {
	return Name
}

func (d *deployer) Build() error {
	return nil
}

// Deployer implementation methods below
func (d *deployer) Up() error {
	if err := d.verifyFlags(); err != nil {
		return err
	}

	// populate default configurations if config file is not pre-created
	if exists, err := utilpath.Exists(utilpath.CheckFollowSymlink, d.configPath); err != nil {
		return err
	} else if !exists {
		args := []string{
			"eks", "create", "config", "--path", d.configPath,
		}
		if err := process.Exec("aws-k8s-tester", args, os.Environ()); err != nil {
			return err
		}
	}

	args := []string{
		"eks", "create", "cluster",
		"--path", d.configPath,
	}

	println("Up(): creating eks cluster with aws-k8s-tester...\n")
	return process.ExecJUnit("aws-k8s-tester", args, os.Environ())
}

func (d *deployer) IsUp() (up bool, err error) {
	// naively assume that if the api server reports nodes, the cluster is up
	lines, err := exec.CombinedOutputLines(
		exec.Command("kubectl", "get", "nodes", "-o=name"),
	)
	if err != nil {
		return false, metadata.NewJUnitError(err, strings.Join(lines, "\n"))
	}
	return len(lines) > 0, nil
}

func (d *deployer) Down() error {
	if err := d.verifyFlags(); err != nil {
		return err
	}

	args := []string{
		"eks", "delete", "cluster",
		"--path", d.configPath,
	}

	println("Down(): deleting eks cluster with aws-k8s-tester...\n")
	return process.ExecJUnit("aws-k8s-tester", args, os.Environ())
}

func (d *deployer) DumpClusterLogs() error {
	// TODO
	return nil
}
