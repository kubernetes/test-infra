/*
Copyright 2017 The Kubernetes Authors.

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

package main

import (
	"fmt"
	"os/exec"
	"runtime"

	"k8s.io/test-infra/kubetest/util"
)

const (
	buildDefault = "quick"
)

type buildStrategy string

// Support both --build and --build=foo
func (b *buildStrategy) IsBoolFlag() bool {
	return true
}

// Return b as a string
func (b *buildStrategy) String() string {
	return string(*b)
}

// Set to --build=B or buildDefault if just --build
func (b *buildStrategy) Set(value string) error {
	if value == "true" { // just --build, choose default
		if runtime.GOARCH == "amd64" {
			value = buildDefault
		} else {
			value = "host-go"
		}
	}
	switch value {
	case "bazel", "e2e", "host-go", "quick", "release", "gce-windows-bazel", "none":
		*b = buildStrategy(value)
		return nil
	}
	return fmt.Errorf("bad build strategy: %v (use: bazel, e2e, host-go, quick, release, none)", value)
}

func (b *buildStrategy) Type() string {
	return "buildStrategy"
}

// True when this kubetest invocation wants to build a release
func (b *buildStrategy) Enabled() bool {
	return *b != ""
}

// Build kubernetes according to specified strategy.
// This may be a bazel, host-go, quick or full release build depending on --build=B.
func (b *buildStrategy) Build() error {
	var target string
	switch *b {
	case "bazel":
		target = "bazel-release"
	case "e2e":
		//TODO(Q-Lee): we should have a better way of build just the e2e tests
		target = "bazel-release"
	// you really should use "bazel" or "quick" in most cases, but in CI
	// we are mimicking these in our job container without an extra level
	// of sandboxing in some cases
	case "host-go":
		target = "all"
	case "quick":
		target = "quick-release"
	case "release":
		target = "release"
	case "gce-windows-bazel":
		// bazel doesn't support building multiple platforms simultaneously
		// yet. We add custom logic here to build both Windows and Linux
		// release tars. https://github.com/kubernetes/kubernetes/issues/76470
		// TODO: remove this after bazel supports the feature.
	case "none":
		return nil
	default:
		return fmt.Errorf("Unknown build strategy: %v", b)
	}

	if *b == "gce-windows-bazel" {
		// Build Linux aritifacts
		cmd := exec.Command("bazel", "build", "--config=cross:linux_amd64", "//build/release-tars")
		cmd.Dir = util.K8s("kubernetes")
		err := control.FinishRunning(cmd)
		if err != nil {
			return err
		}
		// Build windows aritifacts
		cmd = exec.Command("bazel", "build", "--config=cross:windows_amd64", "//build/release-tars")
		cmd.Dir = util.K8s("kubernetes")
		return control.FinishRunning(cmd)
	}

	// TODO(fejta): FIX ME
	// The build-release script needs stdin to ask the user whether
	// it's OK to download the docker image.
	return control.FinishRunning(exec.Command("make", "-C", util.K8s("kubernetes"), target))
}
