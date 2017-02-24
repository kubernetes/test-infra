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
		value = buildDefault
	}
	switch value {
	case "bazel", "quick", "release":
		*b = buildStrategy(value)
		return nil
	}
	return fmt.Errorf("Bad build strategy: %v (use: bash, quick, release)", value)
}

// True when this kubetest invocation wants to build a release
func (b *buildStrategy) Enabled() bool {
	return *b != ""
}

// Build kubernetes according to specified strategy.
// This may be a bazel, quick or full release build depending on --build=B.
func (b *buildStrategy) Build() error {
	var target string
	switch *b {
	case "bazel":
		target = "bazel-build"
	case "quick":
		target = "quick-release"
	case "release":
		target = "release"
	default:
		return fmt.Errorf("Unknown build strategy: %v", b)
	}

	// TODO(fejta): FIX ME
	// The build-release script needs stdin to ask the user whether
	// it's OK to download the docker image.
	return finishRunning(exec.Command("make", target))
}
