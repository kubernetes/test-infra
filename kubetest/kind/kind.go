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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"k8s.io/test-infra/kubetest/process"
)

const (
	// note: this is under the user's home
	kindBinarySubDir    = ".kubetest/kind"
	kindNodeImageLatest = "kindest/node:latest"

	kindBinaryBuild  = "build"
	kindBinaryStable = "stable"

	// If a new version of kind is released this value has to be updated.
	kindBinaryStableTag = "v0.7.0"

	kindClusterNameDefault = "kind-kubetest"

	flagLogLevel = "--verbosity=9"
)

var (
	kindConfigPath = flag.String("kind-config-path", "",
		"(kind only) Path to the kind configuration file.")
	kindKubeconfigPath = flag.String("kind-kubeconfig-path", "",
		"(kind only) Path to the kubeconfig file for kind create cluster command.")
	kindBaseImage = flag.String("kind-base-image", "",
		"(kind only) name:tag of the base image to use for building the node image for kind.")
	kindBinaryVersion = flag.String("kind-binary-version", kindBinaryStable,
		fmt.Sprintf("(kind only) This flag can be either %q (build from source) "+
			"or %q (download a stable binary).", kindBinaryBuild, kindBinaryStable))
	kindClusterName = flag.String("kind-cluster-name", kindClusterNameDefault,
		"(kind only) Name of the kind cluster.")
	kindNodeImage = flag.String("kind-node-image", "", "(kind only) name:tag of the node image to start the cluster. If build is enabled, this is ignored and built image is used.")
)

var (
	kindBinaryStableHashes = map[string]string{
		"kind-linux-amd64":   "9a64f1774cdf24dad5f92e1299058b371c4e3f09d2f9eb281e91ed0777bd1e13",
		"kind-darwin-amd64":  "b6a8fe2b3b53930a1afa4f91b033cdc24b0f6c628d993abaa9e40b57d261162a",
		"kind-windows-amd64": "df327d1e7f8bb41dfd5b1a69c5bc7a8d4bad95bb933562ca367a3a45b6c6ca04",
	}
)

// Deployer is an object the satisfies the kubetest main deployer interface.
type Deployer struct {
	control            *process.Control
	buildType          string
	configPath         string
	importPathK8s      string
	importPathKind     string
	kindBinaryDir      string
	kindBinaryVersion  string
	kindBinaryPath     string
	kindKubeconfigPath string
	kindNodeImage      string
	kindBaseImage      string
	kindClusterName    string
}

type kindReleaseAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

type kindRelease struct {
	Tag    string             `json:"tag_name"`
	Assets []kindReleaseAsset `json:"assets"`
}

// NewDeployer creates a new kind deployer.
func NewDeployer(ctl *process.Control, buildType string) (*Deployer, error) {
	k, err := initializeDeployer(ctl, buildType)
	if err != nil {
		return nil, err
	}
	return k, nil
}

// initializeDeployer initializers the kind deployer flags.
func initializeDeployer(ctl *process.Control, buildType string) (*Deployer, error) {
	if ctl == nil {
		return nil, fmt.Errorf("kind deployer received nil Control")
	}
	// get the user's HOME
	kindBinaryDir := filepath.Join(os.Getenv("HOME"), kindBinarySubDir)

	// Ensure the kind binary dir is added in $PATH.
	err := os.MkdirAll(kindBinaryDir, 0770)
	if err != nil {
		return nil, err
	}
	path := os.Getenv("PATH")
	if !strings.Contains(path, kindBinaryDir) {
		if err := os.Setenv("PATH", kindBinaryDir+":"+path); err != nil {
			return nil, err
		}
	}

	kubeconfigPath := *kindKubeconfigPath
	if kubeconfigPath == "" {
		// Create directory for the cluster kube config
		kindClusterDir := filepath.Join(kindBinaryDir, *kindClusterName)
		if err := os.MkdirAll(kindClusterDir, 0770); err != nil {
			return nil, err
		}
		kubeconfigPath = filepath.Join(kindClusterDir, "kubeconfig")
	}

	d := &Deployer{
		control:            ctl,
		buildType:          buildType,
		configPath:         *kindConfigPath,
		kindBinaryDir:      kindBinaryDir,
		kindBinaryPath:     filepath.Join(kindBinaryDir, "kind"),
		kindBinaryVersion:  *kindBinaryVersion,
		kindKubeconfigPath: kubeconfigPath,
		kindNodeImage:      *kindNodeImage,
		kindClusterName:    *kindClusterName,
	}
	// Obtain the import paths for k8s and kind
	d.importPathK8s, err = d.getImportPath("k8s.io/kubernetes")
	if err != nil {
		return nil, err
	}
	d.importPathKind, err = d.getImportPath("sigs.k8s.io/kind")
	if err != nil {
		return nil, err
	}

	if kindBaseImage != nil {
		d.kindBaseImage = *kindBaseImage
	}
	// ensure we have the kind binary
	if err := d.prepareKindBinary(); err != nil {
		return nil, err
	}
	return d, nil
}

// getImportPath does a naive concat between GOPATH, "src" and a user provided path.
func (d *Deployer) getImportPath(path string) (string, error) {
	o, err := d.control.Output(exec.Command("go", "env", "GOPATH"))
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSuffix(string(o), "\n")
	log.Printf("kind.go:getImportPath(): %s", trimmed)
	return filepath.Join(trimmed, "src", path), nil
}

// setKubeConfigEnv sets the KUBECONFIG environment variable.
func (d *Deployer) setKubeConfigEnv() error {
	log.Println("kind.go:setKubeConfigEnv()")
	return os.Setenv("KUBECONFIG", d.kindKubeconfigPath)
}

// prepareKindBinary either builds kind from source or pulls a binary from GitHub.
func (d *Deployer) prepareKindBinary() error {
	log.Println("kind.go:prepareKindBinary()")
	switch d.kindBinaryVersion {
	case kindBinaryBuild:
		log.Println("Building a kind binary from source.")
		// Build the kind binary.
		cmd := exec.Command("make", "install", "INSTALL_DIR="+d.kindBinaryDir)
		cmd.Dir = d.importPathKind
		if err := d.control.FinishRunning(cmd); err != nil {
			return err
		}
	case kindBinaryStable:
		// ensure a stable kind binary.
		kindPlatformBinary := fmt.Sprintf("kind-%s-%s", runtime.GOOS, runtime.GOARCH)
		if haveStableBinary(d.kindBinaryPath, kindPlatformBinary) {
			log.Printf("Found stable kind binary at %q", d.kindBinaryPath)
			return nil
		}
		// we don't have it, so download it
		binary := fmt.Sprintf("kind-%s-%s", runtime.GOOS, runtime.GOARCH)
		url := fmt.Sprintf("https://github.com/kubernetes-sigs/kind/releases/download/%s/%s", kindBinaryStableTag, binary)
		log.Printf("Downloading a stable kind binary from GitHub: %s, tag: %s", binary, kindBinaryStableTag)
		f, err := os.OpenFile(d.kindBinaryPath, os.O_RDWR|os.O_CREATE, 0770)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := downloadFromURL(url, f); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown kind binary version value: %s", d.kindBinaryVersion)
	}
	return nil
}

// Build handles building kubernetes / kubectl / the node image.
func (d *Deployer) Build() error {
	log.Println("kind.go:Build()")
	// Adapt the build type if needed.
	var buildType string
	var buildNodeImage string
	switch d.buildType {
	case "":
		// The default option is to use a pre-build image.
		log.Println("Skipping the kind node image build.")
		return nil
	case "quick":
		// This is the default build type in kind.
		buildType = "docker"
		buildNodeImage = kindNodeImageLatest
	default:
		// Other types and 'bazel' are handled transparently here.
		buildType = d.buildType
		buildNodeImage = kindNodeImageLatest
	}

	args := []string{"build", "node-image", "--type=" + buildType, flagLogLevel, "--kube-root=" + d.importPathK8s}
	if buildNodeImage != "" {
		args = append(args, "--image="+buildNodeImage)
		// override user-specified node image
		d.kindNodeImage = buildNodeImage
	}
	if d.kindBaseImage != "" {
		args = append(args, "--base-image="+d.kindBaseImage)
	}

	// Build the node image (including kubernetes)
	cmd := exec.Command("kind", args...)
	if err := d.control.FinishRunning(cmd); err != nil {
		return err
	}

	// Build binaries for the host, including kubectl, ginkgo, e2e.test
	if d.buildType != "bazel" {
		cmd := exec.Command(
			"make", "all",
			"WHAT=cmd/kubectl test/e2e/e2e.test vendor/github.com/onsi/ginkgo/ginkgo",
		)
		cmd.Dir = d.importPathK8s
		if err := d.control.FinishRunning(cmd); err != nil {
			return err
		}
		// Copy kubectl to the kind binary path.
		cmd = exec.Command("cp", "-f", "./_output/local/go/bin/kubectl", d.kindBinaryDir)
		cmd.Dir = d.importPathK8s
		if err := d.control.FinishRunning(cmd); err != nil {
			return err
		}
	} else {
		// make build
		cmd := exec.Command(
			"bazel", "build",
			"//cmd/kubectl", "//test/e2e:e2e.test",
			"//vendor/github.com/onsi/ginkgo/ginkgo",
		)
		cmd.Dir = d.importPathK8s
		if err := d.control.FinishRunning(cmd); err != nil {
			return err
		}
		// Copy kubectl to the kind binary path.
		kubectlPath := fmt.Sprintf(
			"./bazel-bin/cmd/kubectl/%s_%s_pure_stripped/kubectl",
			runtime.GOOS, runtime.GOARCH,
		)
		cmd = exec.Command("cp", "-f", kubectlPath, d.kindBinaryDir)
		cmd.Dir = d.importPathK8s
		if err := d.control.FinishRunning(cmd); err != nil {
			return err
		}
	}

	return nil
}

// Up creates a kind cluster. Allows passing node image and config.
func (d *Deployer) Up() error {
	log.Println("kind.go:Up()")
	args := []string{"create", "cluster", "--retain", "--wait=1m", flagLogLevel}

	// Handle the config flag.
	if d.configPath != "" {
		args = append(args, "--config="+d.configPath)
	}

	// Handle the node image flag if we built a new node image.
	if d.kindNodeImage != "" {
		args = append(args, "--image="+d.kindNodeImage)
	}

	// Use a specific cluster name.
	if d.kindClusterName != "" {
		args = append(args, "--name="+d.kindClusterName)
	}

	// Use specific path for the kubeconfig
	if d.kindKubeconfigPath != "" {
		args = append(args, "--kubeconfig="+d.kindKubeconfigPath)
	}

	// Build the kind cluster.
	cmd := exec.Command("kind", args...)
	if err := d.control.FinishRunning(cmd); err != nil {
		return err
	}
	log.Println("*************************************************************************************************")
	log.Println("Cluster is UP")
	log.Printf("Run: \"export KUBECONFIG=%s\" to access to it\n", d.kindKubeconfigPath)
	log.Println("*************************************************************************************************")
	return nil
}

// IsUp verifies if the cluster created by Up() is functional.
func (d *Deployer) IsUp() error {
	log.Println("kind.go:IsUp()")

	// Check if kubectl reports nodes.
	cmd, err := d.KubectlCommand()
	if err != nil {
		return err
	}
	cmd.Args = append(cmd.Args, []string{"get", "nodes", "--no-headers"}...)
	o, err := d.control.Output(cmd)
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
func (d *Deployer) DumpClusterLogs(localPath, gcsPath string) error {
	log.Println("kind.go:DumpClusterLogs()")
	args := []string{"export", "logs", localPath, flagLogLevel}

	// Use a specific cluster name.
	if d.kindClusterName != "" {
		args = append(args, "--name="+d.kindClusterName)
	}

	cmd := exec.Command("kind", args...)
	if err := d.control.FinishRunning(cmd); err != nil {
		log.Printf("kind.go:DumpClusterLogs(): ignoring error: %v", err)
	}
	return nil
}

// TestSetup is a NO-OP in this deployer.
func (d *Deployer) TestSetup() error {
	log.Println("kind.go:TestSetup()")

	// set conformance env so ginkgo.sh etc won't try to do provider setup
	if err := os.Setenv("KUBERNETES_CONFORMANCE_TEST", "y"); err != nil {
		return err
	}

	// Proceed only if a cluster exists.
	exists, err := d.clusterExists()
	if err != nil {
		return err
	}
	if !exists {
		log.Printf("kind.go:TestSetup(): no such cluster %q; skipping the setup of KUBECONFIG!", d.kindClusterName)
		return nil
	}

	// set KUBECONFIG
	if err = d.setKubeConfigEnv(); err != nil {
		return err
	}

	return nil
}

// clusterExists checks if a kind cluster with 'name' exists
func (d *Deployer) clusterExists() (bool, error) {
	log.Println("kind.go:clusterExists()")

	cmd := exec.Command("kind")
	cmd.Args = append(cmd.Args, []string{"get", "clusters"}...)
	out, err := d.control.Output(cmd)
	if err != nil {
		return false, err
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if line == d.kindClusterName {
			log.Printf("kind.go:clusterExists(): found %q", d.kindClusterName)
			return true, nil
		}
	}
	return false, nil
}

// Down tears down the cluster.
func (d *Deployer) Down() error {
	log.Println("kind.go:Down()")

	// Proceed only if a cluster exists.
	exists, err := d.clusterExists()
	if err != nil {
		return err
	}
	if !exists {
		log.Printf("kind.go:Down(): no such cluster %q; skipping 'delete'!", d.kindClusterName)
		return nil
	}

	log.Printf("kind.go:Down(): deleting cluster: %s", d.kindClusterName)
	args := []string{"delete", "cluster", flagLogLevel}

	// Use a specific cluster name.
	if d.kindClusterName != "" {
		args = append(args, "--name="+d.kindClusterName)
	}

	// Delete the cluster.
	cmd := exec.Command("kind", args...)
	if err := d.control.FinishRunning(cmd); err != nil {
		return err
	}

	if d.kindClusterName != "" {
		kindClusterDir := filepath.Join(d.kindBinaryDir, d.kindClusterName)
		if _, err := os.Stat(kindClusterDir); !os.IsNotExist(err) {
			if err := os.RemoveAll(kindClusterDir); err != nil {
				return err
			}
		}
	}
	return nil
}

// GetClusterCreated is unimplemented.GetClusterCreated
func (d *Deployer) GetClusterCreated(gcpProject string) (time.Time, error) {
	log.Println("kind.go:GetClusterCreated()")
	return time.Time{}, errors.New("not implemented")
}

// KubectlCommand returns the exec.Cmd command for kubectl.
func (d *Deployer) KubectlCommand() (*exec.Cmd, error) {
	log.Println("kind.go:KubectlCommand()")
	if err := d.setKubeConfigEnv(); err != nil {
		return nil, err
	}
	// Avoid using ./cluster/kubectl.sh
	// TODO(bentheelder): cache this
	return exec.Command("kubectl"), nil
}

// downloadFromURL downloads from a url to f
func downloadFromURL(url string, f *os.File) error {
	log.Printf("kind.go:downloadFromURL(): %s", url)
	// TODO(bentheelder): is this long enough?
	timeout := time.Duration(60 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	defer f.Sync()
	_, err = io.Copy(f, resp.Body)
	return err
}

// returns true if the binary at expected path exists and
// matches the expected hash of kindPlatformBinary
func haveStableBinary(expectedPath, kindPlatformBinary string) bool {
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		log.Printf("kind binary not present at %s", expectedPath)
		return false
	}
	expectedHash, ok := kindBinaryStableHashes[kindPlatformBinary]
	if !ok {
		return false
	}
	hash, err := hashFile(expectedPath)
	if err != nil {
		return false
	}
	hashMatches := expectedHash == hash
	if !hashMatches {
		log.Printf("kind binary present with hash %q at %q, but expected hash %q", hash, expectedPath, expectedHash)
	}
	return hashMatches
}

// computes the sha256sum of the file at path
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// getKindRelease accepts either a kind release tag or 'kindBinaryLatest'.
// UNUSED: we might end up using this one day.
func getKindRelease(tag string) (*kindRelease, error) {
	b, err := getFromURL("https://api.github.com/repos/kubernetes-sigs/kind/releases")
	if err != nil {
		return nil, err
	}

	var releases []kindRelease
	err = json.Unmarshal(b, &releases)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, errors.New("could not obtain a list of releases from GitHub")
	}

	switch tag {
	case kindBinaryBuild:
		return &releases[0], nil
	default:
		for _, r := range releases {
			if r.Tag == tag {
				return &r, nil
			}
		}
		return nil, fmt.Errorf("could not find a release tagged as %q", tag)
	}
}

// getKindBinaryFromRelease downloads a kind binary based on arch/platform (assetName) and a kindRelease.
// UNUSED: we might end up using this one day.
func getKindBinaryFromRelease(release *kindRelease, assetName string) ([]byte, error) {
	if release == nil {
		return nil, errors.New("getKindBinaryFromRelease() received nil value for 'release'")
	}
	if len(release.Assets) == 0 {
		return nil, fmt.Errorf("no assets defined for release %q", release.Tag)
	}
	for _, a := range release.Assets {
		if strings.Contains(a.Name, assetName) {
			log.Printf("Downloading asset name %q for kind release tag %q", assetName, release.Tag)
			b, err := getFromURL(a.DownloadURL)
			if err != nil {
				return nil, err
			}
			return b, nil
		}
	}
	return nil, fmt.Errorf("no matching asset name %q", assetName)
}

// getFromURL downloads raw bytes from a URL.
func getFromURL(url string) ([]byte, error) {
	timeout := time.Duration(60 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return b, nil
}
