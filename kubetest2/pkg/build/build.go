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

// Package build implements a common system for building kubernetes for deployers to use.
package build

import (
	"fmt"
	"go/build"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
)

const (
	target = "bazel-release"
)

var (
	// This will need changed to support other platforms.
	tarballsToExtract = []string{
		"kubernetes.tar.gz",
		"kubernetes-test-linux-amd64.tar.gz",
		"kubernetes-test-portable.tar.gz",
		"kubernetes-client-linux-amd64.tar.gz",
	}
)

// Build builds kubernetes with the bazel-release target
func Build() error {
	// TODO(RonWeber): This needs options
	src, err := K8sDir("kubernetes")
	if err != nil {
		return err
	}
	c := inheritOutput(exec.Command("make", "-C", src, target))
	if err = c.Run(); err != nil {
		return err
	}
	return extractBuiltTars()
}

// Stage stages the build to GCS.
// Essentially release/push-build.sh --bucket=B --ci --gcs-suffix=S --noupdatelatest
func Stage(location string) error {
	re := regexp.MustCompile(`^gs://([\w-]+)/(devel|ci)(/.*)?`)
	mat := re.FindStringSubmatch(location)
	if mat == nil {
		return fmt.Errorf("Invalid stage location: %v. Use gs://bucket/ci/optional-suffix", location)
	}
	bucket := mat[1]
	ci := mat[2] == "ci"
	gcsSuffix := mat[3]

	args := []string{
		"--nomock",
		"--verbose",
		"--noupdatelatest",
		fmt.Sprintf("--bucket=%v", bucket),
	}
	if len(gcsSuffix) > 0 {
		args = append(args, fmt.Sprintf("--gcs-suffix=%v", gcsSuffix))
	}
	if ci {
		args = append(args, "--ci")
	}

	name, err := K8sDir("release", "push-build.sh")
	if err != nil {
		return err
	}
	cmd := inheritOutput(exec.Command(name, args...)) //Not using kubetest2/pkg/exec because we need to set cmd.Dir.
	cmd.Dir, err = K8sDir("kubernetes")
	if err != nil {
		return err
	}
	return cmd.Run()

}

// K8sDir returns $GOPATH/src/k8s.io/...
func K8sDir(topdir string, parts ...string) (string, error) {
	gopathList := filepath.SplitList(build.Default.GOPATH)
	for _, gopath := range gopathList {
		kubedir := filepath.Join(gopath, "src", "k8s.io", topdir)
		if _, err := os.Stat(kubedir); !os.IsNotExist(err) {
			p := []string{kubedir}
			p = append(p, parts...)
			return filepath.Join(p...), nil
		}
	}
	return "", fmt.Errorf("could not find directory src/k8s.io/%s in GOPATH", topdir)
}

// TODO(RonWeber): This whole untarring and cd-ing logic is out of
// scope for Build().  It needs a better home.
func extractBuiltTars() error {
	allBuilds, err := K8sDir("kubernetes", "_output", "gcs-stage")
	if err != nil {
		return err
	}

	shouldExtract := make(map[string]bool)
	for _, name := range tarballsToExtract {
		shouldExtract[name] = true
	}

	err = filepath.Walk(allBuilds, func(path string, info os.FileInfo, err error) error { //Untar anything with the filename we want.
		if err != nil {
			return err
		}
		if shouldExtract[info.Name()] {
			log.Printf("Extracting %s into current directory", path)
			//Extract it into current directory.
			if err := inheritOutput(exec.Command("tar", "-xzf", path)).Run(); err != nil {
				return fmt.Errorf("Could not extract built tar archive: %v", err)
			}
			shouldExtract[info.Name()] = false
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("could not untar built archive: %v", err)
	}
	for n, s := range shouldExtract { // Make sure we found all the archives we were expecting.
		if s {
			return fmt.Errorf("expected built tarball was not present: %s", n)
		}
	}
	return nil
}

func inheritOutput(cmd *exec.Cmd) *exec.Cmd {
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd
}
