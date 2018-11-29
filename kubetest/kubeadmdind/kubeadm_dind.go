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

// Package kubeadmdind implements a kubetest deployer based on the scripts
// in the github.com/kubernetes-sigs/kubeadm-dind-cluster repo.
// This deployer can be used to create a multinode, containerized Kubernetes
// cluster that runs inside a Prow DinD container.
package kubeadmdind

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/test-infra/kubetest/process"
)

var (
	// Names that are fixed in the Kubeadm DinD scripts
	kubeMasterPrefix = "kube-master"
	kubeNodePrefix   = "kube-node"

	// Systemd service logs to collect on the host container
	hostServices = []string{
		"docker",
	}

	// Docker commands to run on the host container and embedded node
	// containers for log dump
	dockerCommands = []struct {
		cmd     string
		logFile string
	}{
		{"docker images", "docker_images.log"},
		{"docker ps -a", "docker_ps.log"},
	}

	// Systemd service logs to collect on the master and worker embedded
	// node containers for log dump
	systemdServices = []string{
		"kubelet",
		"docker",
	}
	masterKubePods = []string{
		"kube-apiserver",
		"kube-scheduler",
		"kube-controller-manager",
		"kube-proxy",
		"etcd",
	}
	nodeKubePods = []string{
		"kube-proxy",
		"kube-dns",
	}

	// Where to look for (nested) container log files on the node containers
	nodeLogDir = "/var/log"

	// Relative path to Kubernetes source tree
	kubeOrg  = "k8s.io"
	kubeRepo = "kubernetes"

	// Kubeadm-DinD-Cluster (kdc) repo and main script
	kdcOrg    = "github.com/kubernetes-sigs"
	kdcRepo   = "kubeadm-dind-cluster"
	kdcScript = "fixed/dind-cluster-stable.sh"

	// Number of worker nodes to create for testing
	numWorkerNodes = "2"

	// Kubeadm-DinD specific flags
	kubeadmDinDIPMode     = flag.String("kubeadm-dind-ip-mode", "ipv4", "(Kubeadm-DinD only) IP Mode. Can be 'ipv4' (default), 'ipv6', or 'dual-stack'.")
	kubeadmDinDK8sTarFile = flag.String("kubeadm-dind-k8s-tar-file", "", "(Kubeadm-DinD only) Location of tar file containing Kubernetes server binaries.")
	k8sExtractSubDir      = "kubernetes/server/bin"
	k8sTestBinSubDir      = "platforms/linux/amd64"
	testBinDir            = "/usr/bin"
	ipv6EnableCmd         = "sysctl -w net.ipv6.conf.all.disable_ipv6=0"
)

// Deployer is used to implement a kubetest deployer interface
type Deployer struct {
	ipMode     string
	k8sTarFile string
	hostCmder  execCmder
	control    *process.Control
}

// NewDeployer returns a new Kubeadm-DinD Deployer
func NewDeployer(control *process.Control) (*Deployer, error) {
	d := &Deployer{
		ipMode:     *kubeadmDinDIPMode,
		k8sTarFile: *kubeadmDinDK8sTarFile,
		hostCmder:  new(hostCmder),
		control:    control,
	}

	switch d.ipMode {
	case "ipv4":
		// Valid value
	case "ipv6", "dual-stack":
		log.Printf("Enabling IPv6")
		if err := d.run(ipv6EnableCmd); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("configured --ip-mode=%s is not supported for --deployment=kubeadmdind", d.ipMode)
	}

	return d, nil
}

// execCmd executes a command on the host container.
func (d *Deployer) execCmd(cmd string) *exec.Cmd {
	return d.hostCmder.execCmd(cmd)
}

// run runs a command on the host container, and prints any errors.
func (d *Deployer) run(cmd string) error {
	err := d.control.FinishRunning(d.execCmd(cmd))
	if err != nil {
		fmt.Printf("Error: '%v'", err)
	}
	return err
}

// getOutput runs a command on the host container, prints any errors,
// and returns command output.
func (d *Deployer) getOutput(cmd string) ([]byte, error) {
	execCmd := d.execCmd(cmd)
	o, err := d.control.Output(execCmd)
	if err != nil {
		log.Printf("Error: '%v'", err)
		return nil, err
	}
	return o, nil
}

// outputWithStderr runs a command on the host container and returns
// combined stdout and stderr.
func (d *Deployer) outputWithStderr(cmd *exec.Cmd) ([]byte, error) {
	var stdOutErr bytes.Buffer
	cmd.Stdout = &stdOutErr
	cmd.Stderr = &stdOutErr
	err := d.control.FinishRunning(cmd)
	return stdOutErr.Bytes(), err
}

// Up brings up a multinode, containerized Kubernetes cluster inside a
// Prow DinD container.
func (d *Deployer) Up() error {

	var binDir string
	if d.k8sTarFile != "" {
		// Extract Kubernetes server binaries
		cmd := fmt.Sprintf("tar -xvf %s", *kubeadmDinDK8sTarFile)
		if err := d.run(cmd); err != nil {
			return err
		}
		// Derive the location of the extracted binaries
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		binDir = filepath.Join(cwd, k8sExtractSubDir)
	} else {
		// K-D-C scripts must be run from Kubernetes source tree for
		// building binaries.
		kubeDir, err := findPath(kubeOrg, kubeRepo, "")
		if err == nil {
			err = os.Chdir(kubeDir)
		}
		if err != nil {
			return err
		}
	}

	d.setEnv(binDir)

	// Bring up a cluster inside the host Prow container
	script, err := findPath(kdcOrg, kdcRepo, kdcScript)
	if err != nil {
		return err
	}
	return d.run(script + " up")
}

// setEnv sets environment variables for building and testing
// a cluster.
func (d *Deployer) setEnv(k8sBinDir string) error {
	var doBuild string
	switch {
	case k8sBinDir == "":
		doBuild = "y"
	default:
		doBuild = "n"
	}

	// Set KUBERNETES_CONFORMANCE_TEST so that the master IP address
	// is derived from kube config rather than through gcloud.
	envMap := map[string]string{
		"NUM_NODES":                   numWorkerNodes,
		"DIND_K8S_BIN_DIR":            k8sBinDir,
		"BUILD_KUBEADM":               doBuild,
		"BUILD_HYPERKUBE":             doBuild,
		"IP_MODE":                     d.ipMode,
		"KUBERNETES_CONFORMANCE_TEST": "y",
		"NAT64_V4_SUBNET_PREFIX":      "172.20",
	}
	for env, val := range envMap {
		if err := os.Setenv(env, val); err != nil {
			return err
		}
	}
	return nil
}

// IsUp determines if a cluster is up based on whether one or more nodes
// is ready.
func (d *Deployer) IsUp() error {
	n, err := d.clusterSize()
	if err != nil {
		return err
	}
	if n <= 0 {
		return fmt.Errorf("cluster found, but %d nodes reported", n)
	}
	return nil
}

// DumpClusterLogs copies dumps docker state and service logs for:
// - Host Prow container
// - Kube master node container(s)
// - Kube worker node containers
// to a local artifacts directory.
func (d *Deployer) DumpClusterLogs(localPath, gcsPath string) error {
	// Save logs from the host container
	if err := d.saveHostLogs(localPath); err != nil {
		return err
	}

	// Save logs from master node container(s)
	if err := d.saveMasterNodeLogs(localPath); err != nil {
		return err
	}

	// Save logs from worker node containers
	return d.saveWorkerNodeLogs(localPath)
}

// TestSetup builds end-to-end test and ginkgo binaries.
func (d *Deployer) TestSetup() error {
	if d.k8sTarFile == "" {
		// Build e2e.test and ginkgo binaries
		if err := d.run("make WHAT=test/e2e/e2e.test"); err != nil {
			return err
		}
		return d.run("make WHAT=vendor/github.com/onsi/ginkgo/ginkgo")
	}
	// Copy downloaded e2e.test and ginkgo binaries
	for _, file := range []string{"e2e.test", "ginkgo"} {
		srcPath := filepath.Join(k8sTestBinSubDir, file)
		cmd := fmt.Sprintf("cp %s %s", srcPath, testBinDir)
		if err := d.run(cmd); err != nil {
			return err
		}
	}
	return nil
}

// Down brings the DinD-based cluster down and cleans up any DinD state
func (d *Deployer) Down() error {
	// Bring the cluster down and clean up kubeadm-dind-cluster state
	script, err := findPath(kdcOrg, kdcRepo, kdcScript)
	if err != nil {
		return err
	}
	clusterDownCommands := []string{
		script + " down",
		script + " clean",
	}
	for _, cmd := range clusterDownCommands {
		if err := d.run(cmd); err != nil {
			return err
		}
	}
	return nil
}

// GetClusterCreated is not yet implemented.
func (d *Deployer) GetClusterCreated(gcpProject string) (time.Time, error) {
	return time.Time{}, errors.New("not implemented")
}

func (_ *Deployer) KubectlCommand() (*exec.Cmd, error) { return nil, nil }

// findPath looks for the existence of a file or directory based on a
// a github organization, github repo, and a relative path.  It looks
// for the file/directory in this order:
//    - $WORKSPACE/<gitOrg>/<gitRepo>/<gitFile>
//    - $GOPATH/src/<gitOrg>/<gitRepo>/<gitFile>
//    - ./<gitRepo>/<gitFile>
//    - ./<gitFile>
//    - ../<gitFile>
// and returns the path for the first match or returns an error.
func findPath(gitOrg, gitRepo, gitFile string) (string, error) {
	workPath := os.Getenv("WORKSPACE")
	if workPath != "" {
		workPath = filepath.Join(workPath, gitOrg, gitRepo, gitFile)
	}
	goPath := os.Getenv("GOPATH")
	if goPath != "" {
		goPath = filepath.Join(goPath, "src", gitOrg, gitRepo, gitFile)
	}
	relPath := filepath.Join(gitRepo, gitFile)
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	parentDir := filepath.Dir(cwd)
	parentPath := filepath.Join(parentDir, gitFile)
	paths := []string{workPath, goPath, relPath, gitFile, parentPath}
	for _, path := range paths {
		_, err := os.Stat(path)
		if err == nil {
			return path, nil
		}
	}
	err = fmt.Errorf("could not locate %s/%s/%s", gitOrg, gitRepo, gitFile)
	return "", err
}

// execCmder defines an interface for providing a wrapper for processing
// command line strings before calling os/exec.Command().
// There are two implementations of this interface defined below:
// - hostCmder: For executing commands locally (e.g. in Prow container).
// - nodeCmder: For executing commands on node containers embedded
//              in the Prow container.
type execCmder interface {
	execCmd(cmd string) *exec.Cmd
}

// hostCmder implements the execCmder interface for processing commands
// locally (e.g. in Prow container).
type hostCmder struct{}

// execCmd splits a command line string into a command (first word) and
// remaining arguments in variadic form, as required by exec.Command().
func (h *hostCmder) execCmd(cmd string) *exec.Cmd {
	words := strings.Fields(cmd)
	return exec.Command(words[0], words[1:]...)
}

// nodeCmder implements the nodeExecCmder interface for processing
// commands in an embedded node container.
type nodeCmder struct {
	node string
}

func newNodeCmder(node string) *nodeCmder {
	cmder := new(nodeCmder)
	cmder.node = node
	return cmder
}

// execCmd creates an exec.Cmd structure for running a command line on a
// nested node container in the host container. It is equivalent to running
// a command via 'docker exec <node-container-name> <cmd>'.
func (n *nodeCmder) execCmd(cmd string) *exec.Cmd {
	args := strings.Fields(fmt.Sprintf("exec %s %s", n.node, cmd))
	return exec.Command("docker", args...)
}

// getNode returns the node name for a nodeExecCmder
func (n *nodeCmder) getNode() string {
	return n.node
}

// execCmdSaveLog executes a command either in the host container or
// in an embedded node container, and writes the combined stdout and
// stderr to a log file in a local artifacts directory. (Stderr is
// required because running 'docker logs ...' on nodes sometimes
// returns results as stderr).
func (d *Deployer) execCmdSaveLog(cmder execCmder, cmd string, logDir string, logFile string) error {
	execCmd := cmder.execCmd(cmd)
	o, err := d.outputWithStderr(execCmd)
	if err != nil {
		log.Printf("%v", err)
		if len(o) > 0 {
			log.Printf("%s", o)
		}
		// Ignore the command error and continue collecting logs
		return nil
	}
	logPath := filepath.Join(logDir, logFile)
	return ioutil.WriteFile(logPath, o, 0644)
}

// saveDockerState saves docker state for either a host Prow container
// or an embedded node container.
func (d *Deployer) saveDockerState(cmder execCmder, logDir string) error {
	for _, dockerCommand := range dockerCommands {
		if err := d.execCmdSaveLog(cmder, dockerCommand.cmd, logDir, dockerCommand.logFile); err != nil {
			return err
		}
	}
	return nil
}

// saveServiceLogs saves logs for a list of systemd services on either
// a host Prow container or an embedded node container.
func (d *Deployer) saveServiceLogs(cmder execCmder, services []string, logDir string) error {
	for _, svc := range services {
		cmd := fmt.Sprintf("journalctl -u %s.service", svc)
		logFile := fmt.Sprintf("%s.log", svc)
		if err := d.execCmdSaveLog(cmder, cmd, logDir, logFile); err != nil {
			return err
		}
	}
	return nil
}

// clusterSize determines the number of nodes in a cluster.
func (d *Deployer) clusterSize() (int, error) {
	o, err := d.getOutput("kubectl get nodes --no-headers")
	if err != nil {
		return -1, fmt.Errorf("kubectl get nodes failed: %s\n%s", err, string(o))
	}
	trimmed := strings.TrimSpace(string(o))
	if trimmed != "" {
		return len(strings.Split(trimmed, "\n")), nil
	}
	return 0, nil
}

// Create a local log artifacts directory
func (d *Deployer) makeLogDir(logDir string) error {
	cmd := fmt.Sprintf("mkdir -p %s", logDir)
	execCmd := d.execCmd(cmd)
	return d.control.FinishRunning(execCmd)
}

// saveHostLogs collects service logs and docker state from the host
// container, and saves the logs in a local artifacts directory.
func (d *Deployer) saveHostLogs(artifactsDir string) error {
	log.Printf("Saving logs from host container")

	// Create directory for the host container artifacts
	logDir := filepath.Join(artifactsDir, "host-container")
	if err := d.run("mkdir -p " + logDir); err != nil {
		return err
	}

	// Save docker state for the host container
	if err := d.saveDockerState(d.hostCmder, logDir); err != nil {
		return err
	}

	// Copy service logs from the node container
	return d.saveServiceLogs(d.hostCmder, hostServices, logDir)
}

// saveMasterNodeLogs collects docker state, service logs, and Kubernetes
// system pod logs from all nested master node containers that are running
// on the host container, and saves the logs in a local artifacts directory.
func (d *Deployer) saveMasterNodeLogs(artifactsDir string) error {
	masters, err := d.detectNodeContainers(kubeMasterPrefix)
	if err != nil {
		return err
	}
	for _, master := range masters {
		if err := d.saveNodeLogs(master, artifactsDir, systemdServices, masterKubePods); err != nil {
			return err
		}
	}
	return nil
}

// saveWorkerNodeLogs collects docker state, service logs, and Kubernetes
// system pod logs from all nested worker node containers that are running
// on the host container, and saves the logs in a local artifacts directory.
func (d *Deployer) saveWorkerNodeLogs(artifactsDir string) error {
	nodes, err := d.detectNodeContainers(kubeNodePrefix)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if err := d.saveNodeLogs(node, artifactsDir, systemdServices, nodeKubePods); err != nil {
			return err
		}
	}
	return nil
}

// detectNodeContainers creates a list of names for either all master or all
// worker node containers. It does this by running 'kubectl get nodes ... '
// and searching for container names that begin with a specified name prefix.
func (d *Deployer) detectNodeContainers(namePrefix string) ([]string, error) {
	log.Printf("Looking for container names beginning with '%s'", namePrefix)
	o, err := d.getOutput("kubectl get nodes --no-headers")
	if err != nil {
		return nil, err
	}
	var nodes []string
	trimmed := strings.TrimSpace(string(o))
	if trimmed != "" {
		lines := strings.Split(trimmed, "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			name := fields[0]
			if strings.Contains(name, namePrefix) {
				nodes = append(nodes, name)
			}
		}
	}
	return nodes, nil
}

// detectKubeContainers creates a list of containers (either running or
// exited) on a master or worker node whose names contain any of a list of
// Kubernetes system pod name substrings.
func (d *Deployer) detectKubeContainers(nodeCmder execCmder, node string, kubePods []string) ([]string, error) {
	// Run 'docker ps -a' on the node container
	cmd := fmt.Sprintf("docker ps -a")
	execCmd := nodeCmder.execCmd(cmd)
	o, err := d.control.Output(execCmd)
	if err != nil {
		log.Printf("Error running '%s' on %s: '%v'", cmd, node, err)
		return nil, err
	}
	// Find container names that contain any of a list of pod name substrings
	var containers []string
	if trimmed := strings.TrimSpace(string(o)); trimmed != "" {
		lines := strings.Split(trimmed, "\n")
		for _, line := range lines {
			if fields := strings.Fields(line); len(fields) > 0 {
				name := fields[len(fields)-1]
				if strings.Contains(name, "_POD_") {
					// Ignore infra containers
					continue
				}
				for _, pod := range kubePods {
					if strings.Contains(name, pod) {
						containers = append(containers, name)
						break
					}
				}
			}
		}
	}
	return containers, nil
}

// saveNodeLogs collects docker state, service logs, and Kubernetes
// system pod logs for a given node container, and saves the logs in a local
// artifacts directory.
func (d *Deployer) saveNodeLogs(node string, artifactsDir string, services []string, kubePods []string) error {
	log.Printf("Saving logs from node container %s", node)

	// Create directory for node container artifacts
	logDir := filepath.Join(artifactsDir, node)
	if err := d.run("mkdir -p " + logDir); err != nil {
		return err
	}

	cmder := newNodeCmder(node)

	// Save docker state for this node
	if err := d.saveDockerState(cmder, logDir); err != nil {
		return err
	}

	// Copy service logs from the node container
	if err := d.saveServiceLogs(cmder, services, logDir); err != nil {
		return err
	}

	// Copy log files for kube system pod containers (running or exited)
	// from this node container.
	containers, err := d.detectKubeContainers(cmder, node, kubePods)
	if err != nil {
		return err
	}
	for _, container := range containers {
		cmd := fmt.Sprintf("docker logs %s", container)
		logFile := fmt.Sprintf("%s.log", container)
		if err := d.execCmdSaveLog(cmder, cmd, logDir, logFile); err != nil {
			return err
		}
	}
	return nil
}
