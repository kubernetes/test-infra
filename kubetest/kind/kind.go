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

package kind

import (
	"errors"
	"flag"
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/test-infra/kubetest/process"
)

const (
	kindBinaryDir = "$HOME/.kubetest/kind"
	kindNodeImage = "kindest/node:latest"
)

var (
	kindConfigPath = flag.String("kind-config-path", "",
		"(kind only) Path to the kind configuration file.")
)

// Kind is an object the satisfies the deployer interface.
type Kind struct {
	control        *process.Control
	configPath     string
	importPathKind string
	importPathK8s  string
	kindBinaryPath string
}

// NewKind creates a new kind deployer.
func NewKind(ctl *process.Control) (*Kind, error) {
	k, err := initializeKind(ctl)
	if err != nil {
		return nil, err
	}
	return k, nil
}

func getImportPath(path string) (string, error) {
	// look up the source the way go build would
	pkg, err := build.Default.Import(path, ".", build.FindOnly)
	if err == nil && pkg.Dir != "" {
		return pkg.Dir, nil
	}
	return "", fmt.Errorf("could not find the source directory for package: %s", path)
}

// initializeKind initializers the kind deployer flags.
func initializeKind(ctl *process.Control) (*Kind, error) {
	if ctl == nil {
		return nil, fmt.Errorf("kind deployer received nil Control")
	}
	// Ensure the kind binary dir is added in $PATH.
	err := os.MkdirAll(kindBinaryDir, 0770)
	if err != nil {
		return nil, err
	}
	path := os.Getenv("PATH")
	if !strings.Contains(path, kindBinaryDir) {
		if err := os.Setenv("PATH", path+":"+kindBinaryDir); err != nil {
			return nil, err
		}
	}
	// Obtain the import paths for k8s and kind.
	importPathK8s, err := getImportPath("sigs.k8s.io/kubernetes")
	if err != nil {
		return nil, err
	}
	importPathKind, err := getImportPath("sigs.k8s.io/kind")
	if err != nil {
		return nil, err
	}
	k := &Kind{
		control:        ctl,
		configPath:     *kindConfigPath,
		importPathK8s:  importPathK8s,
		importPathKind: importPathKind,
		kindBinaryPath: filepath.Join(kindBinaryDir, "kind"),
	}
	return k, nil
}

func (k *Kind) getKubeConfigPath() (string, error) {
	o, err := k.control.Output(exec.Command("kind", "get", "kubeconfig-path"))
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(o), "\n"), nil
}

func (k *Kind) setKubeConfigEnv() error {
	path, err := k.getKubeConfigPath()
	if err != nil {
		return err
	}
	if err = os.Setenv("KUBECONFIG", path); err != nil {
		return err
	}
	return nil
}

// Up builds kind and the node image and then deploys a cluster based on the kind config.
func (k *Kind) Up() error {
	// TODO(neolit123): fetch a stable release binary and stash it in a kubetest specific dir.
	// This should be the default option instead of building kind from source.

	// TODO(neolit123): let --deployer=kind hook the kubetest --build process to instead do
	// the appropriate --type of kind build node-image and a kubectl build for the host.

	// Build the kind binary.
	cmd := exec.Command("go", "build")
	cmd.Dir = k.importPathKind
	if err := k.control.FinishRunning(cmd); err != nil {
		return err
	}
	// Copy the kind binary to the kind binary path.
	cmd = exec.Command("cp", "./kind", k.kindBinaryPath)
	cmd.Dir = k.importPathKind
	if err := k.control.FinishRunning(cmd); err != nil {
		return err
	}
	// Build the node image.
	cmd = exec.Command("kind", "build", "node-image", "--image="+kindNodeImage)
	if err := k.control.FinishRunning(cmd); err != nil {
		return err
	}
	// Build kubectl.
	cmd = exec.Command("make", "all", "WHAT=cmd/kubectl")
	cmd.Dir = k.importPathK8s
	if err := k.control.FinishRunning(cmd); err != nil {
		return err
	}
	// Copy kubectl to the kind binary path.
	cmd = exec.Command("cp", "./_output/local/go/bin/kubectl", kindBinaryDir)
	cmd.Dir = k.importPathK8s
	if err := k.control.FinishRunning(cmd); err != nil {
		return err
	}
	// Create the kind cluster.
	useConfig := ""
	if k.configPath != "" {
		useConfig = "--config=" + k.configPath
	}
	cmd = exec.Command("kind", "create", "cluster", "--image="+kindNodeImage,
		"--retain", "--wait=1m", "--loglevel=debug",
		useConfig)
	if err := k.control.FinishRunning(cmd); err != nil {
		return err
	}
	return nil
}

// IsUp verifies if the cluster created by Up() is functional.
func (k *Kind) IsUp() error {
	cmd, err := k.KubectlCommand()
	if err != nil {
		return err
	}
	cmd.Args = []string{"get", "nodes", "--no-headers"}
	o, err := k.control.Output(cmd)
	if err != nil {
		return err
	}
	trimmed := strings.TrimSpace(string(o))
	n := 0
	if trimmed != "" {
		n = len(strings.Split(trimmed, "\n"))
	}
	if n <= 0 {
		return fmt.Errorf("cluster found, but %d nodes reported", n)
	}
	return nil
}

// DumpClusterLogs dumps the logs for this cluster in localPath.
func (k *Kind) DumpClusterLogs(localPath, gcsPath string) error {
	cmd := exec.Command("kind", "export", "logs", localPath)
	if err := k.control.FinishRunning(cmd); err != nil {
		return err
	}
	return nil
}

// TestSetup is a NO-OP in this deployer.
func (k *Kind) TestSetup() error {
	// set KUBECONFIG
	if err := k.setKubeConfigEnv(); err != nil {
		return err
	}
	return nil
}

// Down tears down the cluster.
func (k *Kind) Down() error {
	cmd := exec.Command("kind", "delete", "cluster")
	if err := k.control.FinishRunning(cmd); err != nil {
		return err
	}
	return nil
}

// GetClusterCreated is unimplemented.GetClusterCreated
func (k *Kind) GetClusterCreated(gcpProject string) (time.Time, error) {
	return time.Time{}, errors.New("not implemented")
}

// KubectlCommand returns the exec.Cmd command for kubectl.
func (k *Kind) KubectlCommand() (*exec.Cmd, error) {
	// Avoid using ./cluster/kubectl.sh
	kubeConfigPath, err := k.getKubeConfigPath()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("kubectl")
	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeConfigPath))
	return cmd, nil
}
