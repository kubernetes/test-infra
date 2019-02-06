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
	"go/build"
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
	kindBinaryStableTag = "0.1.0"
)

var (
	kindConfigPath = flag.String("kind-config-path", "",
		"(kind only) Path to the kind configuration file.")
	kindBinaryVersion = flag.String("kind-binary-version", kindBinaryStable,
		fmt.Sprintf("(kind only) This flag can be either %q (build from source) "+
			"or %q (download a stable binary).", kindBinaryBuild, kindBinaryStable))
)

var (
	kindBinaryStableHashes = map[string]string{
		"kind-linux-amd64":   "7566c0117d824731be5caee10fef0a88fb65e3508ee22a305dc17507ee87d874",
		"kind-darwin-amd64":  "ce85d3ed3d03702af0e9c617098249aff2e0811e1202036b260b23df4551f3ad",
		"kind-windows-amd64": "376862a3f6c449d91fccabfbae27a991e75177ad1111adbf2839a98f991eeef6",
	}
)

// Deployer is an object the satisfies the kubetest main deployer interface.
type Deployer struct {
	control           *process.Control
	buildType         string
	configPath        string
	importPathK8s     string
	kindBinaryDir     string
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

// NewDeployer creates a new kind deployer.
func NewDeployer(ctl *process.Control, buildType string) (*Deployer, error) {
	k, err := initializeDeployer(ctl, buildType)
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
	// Obtain the import paths for k8s
	importPathK8s, err := getImportPath("k8s.io/kubernetes")
	if err != nil {
		return nil, err
	}
	d := &Deployer{
		control:           ctl,
		buildType:         buildType,
		configPath:        *kindConfigPath,
		importPathK8s:     importPathK8s,
		kindBinaryDir:     kindBinaryDir,
		kindBinaryPath:    filepath.Join(kindBinaryDir, "kind"),
		kindBinaryVersion: *kindBinaryVersion,
		kindNodeImage:     kindNodeImageLatest,
	}
	// ensure we have the kind binary
	if err := d.prepareKindBinary(); err != nil {
		return nil, err
	}
	return d, nil
}

// getKubeConfigPath returns the path to the kubeconfig file.
func (d *Deployer) getKubeConfigPath() (string, error) {
	o, err := d.control.Output(exec.Command("kind", "get", "kubeconfig-path"))
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(o), "\n"), nil
}

// setKubeConfigEnv sets the KUBECONFIG environment variable.
func (d *Deployer) setKubeConfigEnv() error {
	path, err := d.getKubeConfigPath()
	if err != nil {
		return err
	}
	if err = os.Setenv("KUBECONFIG", path); err != nil {
		return err
	}
	return nil
}

// prepareKindBinary either builds kind from source or pulls a binary from GitHub.
func (d *Deployer) prepareKindBinary() error {
	switch d.kindBinaryVersion {
	case kindBinaryBuild:
		importPathKind, err := getImportPath("sigs.k8s.io/kind")
		if err != nil {
			return err
		}
		log.Println("Building a kind binary from source.")
		// Build the kind binary.
		cmd := exec.Command("go", "build", "-o", filepath.Join(d.kindBinaryPath, "kind"))
		cmd.Dir = importPathKind
		if err := d.control.FinishRunning(cmd); err != nil {
			return err
		}
	case kindBinaryStable:
		// ensure a stable kind binary.
		kindPlatformBinary := fmt.Sprintf("kind-%s-%s", runtime.GOOS, runtime.GOARCH)
		if haveStableBinary(d.kindBinaryPath, kindPlatformBinary) {
			log.Printf("Found stable kind binary at %s", d.kindBinaryPath)
			return nil
		}
		// we don't have it, so download it
		binary := fmt.Sprintf("kind-%s-%s", runtime.GOOS, runtime.GOARCH)
		url := fmt.Sprintf("https://github.com/kubernetes-sigs/kind/releases/download/%s/%s", kindBinaryStableTag, binary)
		log.Printf("Downloading a stable kind binary from GitHub: %s, tag: %s\n", binary, kindBinaryStableTag)
		f, err := os.OpenFile(d.kindBinaryPath, os.O_RDWR|os.O_CREATE, 0770)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := downloadFromURL(url, f); err != nil {
			return err
		}
	}
	return nil
}

// Build handles building kubernetes / kubectl / the node image.
func (d *Deployer) Build() error {
	// Adapt the build type if needed.
	var buildType string
	switch d.buildType {
	case "":
		// The default option is to use a pre-build image.
		log.Println("Skipping the kind node image build.")
		d.kindNodeImage = ""
		return nil
	case "quick":
		// This is the default build type in kind.
		buildType = "docker"
	default:
		// Other types and 'bazel' are handled transparently here.
		buildType = d.buildType
	}

	// Build the node image (including kubernetes)
	cmd := exec.Command("kind", "build", "node-image", "--image="+d.kindNodeImage, "--type="+buildType)
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
		/*
			e2e.test does not show up in a path with platform in it and will
			not be found by kube::util::find-binary, so we will copy it to an
			acceptable location until this is fixed upstream
			https://github.com/kubernetes/kubernetes/issues/68306
		*/
		cmd = exec.Command("cp", "-r", "bazel-bin/test/e2e/e2e.test", "./_output/bin/")
		cmd.Dir = d.importPathK8s
		if err := d.control.FinishRunning(cmd); err != nil {
			return err
		}
	}

	return nil
}

// Up builds kind and the node image and then deploys a cluster based on the kind config.
func (d *Deployer) Up() error {
	// Handle the config flag.
	configFlag := ""
	if d.configPath != "" {
		configFlag = "--config=" + d.configPath
	}

	// Handle the node image flag if we built a new node image.
	nodeImageFlag := ""
	if d.kindNodeImage != "" {
		nodeImageFlag = "--image=" + d.kindNodeImage
	}

	// Build the kind cluster.
	cmd := exec.Command(
		"kind", "create", "cluster",
		nodeImageFlag, configFlag,
		"--retain", "--wait=1m", "--loglevel=info",
	)
	if err := d.control.FinishRunning(cmd); err != nil {
		return err
	}
	return nil
}

// IsUp verifies if the cluster created by Up() is functional.
func (d *Deployer) IsUp() error {
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
	cmd := exec.Command("kind", "export", "logs", localPath)
	if err := d.control.FinishRunning(cmd); err != nil {
		return err
	}
	return nil
}

// TestSetup is a NO-OP in this deployer.
func (d *Deployer) TestSetup() error {
	// set KUBECONFIG
	if err := d.setKubeConfigEnv(); err != nil {
		return err
	}
	// set conformance env so ginkgo.sh etc won't try to do provider setup
	os.Setenv("KUBERNETES_CONFORMANCE_TEST", "y")
	return nil
}

// Down tears down the cluster.
func (d *Deployer) Down() error {
	cmd := exec.Command("kind", "delete", "cluster")
	if err := d.control.FinishRunning(cmd); err != nil {
		return err
	}
	return nil
}

// GetClusterCreated is unimplemented.GetClusterCreated
func (d *Deployer) GetClusterCreated(gcpProject string) (time.Time, error) {
	return time.Time{}, errors.New("not implemented")
}

// KubectlCommand returns the exec.Cmd command for kubectl.
func (d *Deployer) KubectlCommand() (*exec.Cmd, error) {
	// Avoid using ./cluster/kubectl.sh
	// TODO(bentheelder): cache this
	kubeConfigPath, err := d.getKubeConfigPath()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("kubectl")
	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeConfigPath))
	return cmd, nil
}

// downloadFromURL downloads from a url to f
func downloadFromURL(url string, f *os.File) error {
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
		log.Printf("kind binary present with hash: %s at: %s", hash, expectedPath)
		log.Printf("... but expected hash: %s", expectedHash)
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
