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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/build"
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
	kindBinaryDir       = "$HOME/.kubetest/kind"
	kindNodeImageLatest = "kindest/node:latest"

	kindBinaryBuild  = "build"
	kindBinaryStable = "stable"

	// If a new version of kind is released this value has to be updated.
	kindBinaryStableTag = "0.1.0"
)

var (
	kindConfigPath = flag.String("kind-config-path", "",
		"(kind only) Path to the kind configuration file.")
	kindBinaryVersion = flag.String("kind-binary-version", kindBinaryStable,
		fmt.Sprintf("(kind only) This flag can be either %q (build from source) "+
			"or %q (download a stable binary).", kindBinaryBuild, kindBinaryStable))
)

// Kind is an object the satisfies the deployer interface.
type Kind struct {
	control           *process.Control
	buildType         string
	configPath        string
	importPathKind    string
	importPathK8s     string
	kindBinaryVersion string
	kindBinaryPath    string
	kindNodeImage     string
}

type kindReleaseAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

type kindRelease struct {
	Tag    string             `json:"tag_name"`
	Assets []kindReleaseAsset `json:"assets"`
}

// NewKind creates a new kind deployer.
func NewKind(ctl *process.Control, buildType string) (*Kind, error) {
	k, err := initializeKind(ctl, buildType)
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
func initializeKind(ctl *process.Control, buildType string) (*Kind, error) {
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
		control:           ctl,
		buildType:         buildType,
		configPath:        *kindConfigPath,
		importPathK8s:     importPathK8s,
		importPathKind:    importPathKind,
		kindBinaryPath:    filepath.Join(kindBinaryDir, "kind"),
		kindBinaryVersion: *kindBinaryVersion,
		kindNodeImage:     kindNodeImageLatest,
	}
	return k, nil
}

// getKubeConfigPath returns the path to the kubeconfig file.
func (k *Kind) getKubeConfigPath() (string, error) {
	o, err := k.control.Output(exec.Command("kind", "get", "kubeconfig-path"))
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(o), "\n"), nil
}

// setKubeConfigEnv sets the KUBECONFIG environment variable.
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

// prepareKindBinary either builds kind from source or pulls a binary from GitHub.
func (k *Kind) prepareKindBinary() error {
	switch k.kindBinaryVersion {
	case kindBinaryBuild:
		log.Println("Building a kind binary from source.")
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
	case kindBinaryStable:
		// Download a stable kind binary.
		binary := fmt.Sprintf("kind-%s-%s", runtime.GOOS, runtime.GOARCH)
		url := fmt.Sprintf("https://github.com/kubernetes-sigs/kind/releases/download/%s/%s", kindBinaryStableTag, binary)
		log.Printf("Downloading a stable kind binary from GitHub: %s, tag: %s\n", binary, kindBinaryStableTag)
		new, err := getFromURL(url)
		if err != nil {
			return err
		}
		const writeMode = 0770
		// If the old binary is missing write the new binary.
		if _, err := os.Stat(k.kindBinaryPath); os.IsNotExist(err) {
			if err := ioutil.WriteFile(k.kindBinaryPath, new, writeMode); err != nil {
				return err
			}
			return nil
		}
		// Read the old binary and compare its checksum to the checksum of the new one.
		// Only write the new binary if the checksums differ.
		old, err := ioutil.ReadFile(k.kindBinaryPath)
		if err != nil {
			return err
		}
		if checksumIsEqual(new, old) {
			log.Printf("The same version of kind is already installed: %s", k.kindBinaryPath)
			return nil
		}
		log.Printf("Installing a new kind binary: %s", k.kindBinaryPath)
		if err := ioutil.WriteFile(k.kindBinaryPath, new, writeMode); err != nil {
			return err
		}
	}
	return nil
}

// prepareNodeImage handles building the node image.
func (k *Kind) prepareNodeImage() error {
	// Adapt the build type if needed.
	var buildType string
	switch k.buildType {
	case "":
		// The default option is to use a pre-build image.
		log.Println("Skipping the kind node image build.")
		k.kindNodeImage = ""
		return nil
	case "quick":
		// This is the default build type in kind.
		buildType = "docker"
	default:
		// Other types and 'bazel' are handled transparently here.
		buildType = k.buildType
	}

	// Build the node image.
	cmd := exec.Command("kind", "build", "node-image", "--image="+k.kindNodeImage, "--type="+buildType)
	if err := k.control.FinishRunning(cmd); err != nil {
		return err
	}
	return nil
}

// Up builds kind and the node image and then deploys a cluster based on the kind config.
func (k *Kind) Up() error {
	if err := k.prepareKindBinary(); err != nil {
		return err
	}
	if err := k.prepareNodeImage(); err != nil {
		return err
	}

	// Build kubectl.
	cmd := exec.Command("make", "all", "WHAT=cmd/kubectl")
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

	// Handle the config flag.
	configFlag := ""
	if k.configPath != "" {
		configFlag = "--config=" + k.configPath
	}

	// Handle the node image flag if we built a new node image.
	nodeImageFlag := ""
	if k.kindNodeImage != "" {
		nodeImageFlag = "--image=" + k.kindNodeImage
	}

	// Build the kind cluster.
	cmd = exec.Command("kind", "create", "cluster", nodeImageFlag, configFlag,
		"--retain", "--wait=1m", "--loglevel=debug")
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

// checksumIsEqual takes a couple of byte slices and returns 'true' if their
// checksum matches.
func checksumIsEqual(new, old []byte) bool {
	shaNew := sha256.Sum256(new)
	shaOld := sha256.Sum256(old)
	for i := 0; i < sha256.Size; i++ {
		if shaNew[i] != shaOld[i] {
			return false
		}
	}
	return true
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
			log.Printf("Downloading asset name %q for kind release tag %q\n", assetName, release.Tag)
			b, err := getFromURL(a.DownloadURL)
			if err != nil {
				return nil, err
			}
			return b, nil
		}
	}
	return nil, fmt.Errorf("no matching asset name %q", assetName)
}
