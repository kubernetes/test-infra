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

// Package deployer implements the kubetest2 kind deployer
package deployer

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/pflag"

	"k8s.io/test-infra/kubetest2/pkg/exec"
	"k8s.io/test-infra/kubetest2/pkg/metadata"
	"k8s.io/test-infra/kubetest2/pkg/process"
	"k8s.io/test-infra/kubetest2/pkg/types"
)

// Name is the name of the deployer
const Name = "kind"

// New implements deployer.New for kind
func New(opts types.Options) (types.Deployer, *pflag.FlagSet) {
	// create a deployer object and set fields that are not flag controlled
	d := &deployer{
		commonOptions: opts,
		logsDir:       filepath.Join(opts.ArtifactsDir(), "logs"),
	}
	// register flags and return
	return d, bindFlags(d)
}

// assert that New implements types.NewDeployer
var _ types.NewDeployer = New

// TODO(bentheelder): finish implementing this stubbed-out deployer
type deployer struct {
	// generic parts
	commonOptions types.Options
	// kind specific details
	nodeImage      string // name of the node image built / deployed
	clusterName    string // --name flag value for kind
	logLevel       string // log level for kind commands
	logsDir        string // dir to export logs to
	buildType      string // --type flag to kind build node-image
	configPath     string // --config flag for kind create cluster
	kubeconfigPath string // --kubeconfig flag for kind create cluster
	verbosity      int    // --verbosity for kind
}

// helper used to create & bind a flagset to the deployer
func bindFlags(d *deployer) *pflag.FlagSet {
	flags := pflag.NewFlagSet(Name, pflag.ContinueOnError)
	flags.StringVar(
		&d.clusterName, "cluster-name", "kind-kubetest2", "the kind cluster --name",
	)
	flags.StringVar(
		&d.logLevel, "loglevel", "", "--loglevel for kind commands",
	)
	flags.StringVar(
		&d.nodeImage, "image-name", "", "the image name to use for build and up",
	)
	flags.StringVar(
		&d.nodeImage, "build-type", "", "--type for kind build node-image",
	)
	flags.StringVar(
		&d.configPath, "config", "", "--config for kind create cluster",
	)
	flags.StringVar(
		&d.kubeconfigPath, "kubeconfig", "", "--kubeconfig flag for kind create cluster",
	)
	flags.IntVar(
		&d.verbosity, "verbosity", 0, "--verbosity flag for kind",
	)
	return flags
}

// assert that deployer implements types.Deployer
var _ types.Deployer = &deployer{}

// Deployer implementation methods below

func (d *deployer) Up() error {
	args := []string{
		"create", "cluster",
		"--name", d.clusterName,
	}
	if d.logLevel != "" {
		args = append(args, "--loglevel", d.logLevel)
	}
	// set the explicitly specified image name if set
	if d.nodeImage != "" {
		args = append(args, "--image", d.nodeImage)
	} else if d.commonOptions.ShouldBuild() {
		// otherwise if we just built an image, use that
		// NOTE: this is safe in the face of upstream changes, because
		// we use the same logic / constant for Build()
		args = append(args, "--image", kindDefaultBuiltImageName)
	}
	if d.configPath != "" {
		args = append(args, "--config", d.configPath)
	}
	if d.kubeconfigPath != "" {
		args = append(args, "--kubeconfig", d.kubeconfigPath)
	}
	if d.verbosity > 0 {
		args = append(args, "--verbosity", strconv.Itoa(d.verbosity))
	}

	println("Up(): creating kind cluster...\n")
	// we want to see the output so use process.ExecJUnit
	return process.ExecJUnit("kind", args, os.Environ())
}

func (d *deployer) Down() error {
	args := []string{
		"delete", "cluster",
		"--name", d.clusterName,
	}
	if d.logLevel != "" {
		args = append(args, "--loglevel", d.logLevel)
	}

	println("Down(): deleting kind cluster...\n")
	// we want to see the output so use process.ExecJUnit
	return process.ExecJUnit("kind", args, os.Environ())
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

func (d *deployer) DumpClusterLogs() error {
	args := []string{
		"export", "logs",
		"--name", d.clusterName,
		d.logsDir,
	}
	if d.logLevel != "" {
		args = append(args, "--loglevel", d.logLevel)
	}

	println("DumpClusterLogs(): exporting kind cluster logs...\n")
	// we want to see the output so use process.ExecJUnit
	return process.ExecJUnit("kind", args, os.Environ())
}

func (d *deployer) Build() error {
	// TODO(bentheelder): build type should be configurable
	args := []string{
		"build", "node-image",
	}
	if d.logLevel != "" {
		args = append(args, "--loglevel", d.logLevel)
	}
	if d.buildType != "" {
		args = append(args, "--type", d.buildType)
	}
	// set the explicitly specified image name if set
	if d.nodeImage != "" {
		args = append(args, "--image", d.nodeImage)
	} else if d.commonOptions.ShouldBuild() {
		// otherwise if we just built an image, use that
		args = append(args, "--image", kindDefaultBuiltImageName)
	}

	println("Build(): building kind node image...\n")
	// we want to see the output so use process.ExecJUnit
	return process.ExecJUnit("kind", args, os.Environ())
}

// well-known kind related constants
const kindDefaultBuiltImageName = "kindest/node:latest"
