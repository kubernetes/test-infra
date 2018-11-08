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
		value = buildDefault
	}
	switch value {
	case "bazel", "e2e", "host-go", "quick", "release":
		*b = buildStrategy(value)
		return nil
	}
	return fmt.Errorf("bad build strategy: %v (use: bazel, e2e, host-go, quick, release)", value)
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
	default:
		return fmt.Errorf("Unknown build strategy: %v", b)
	}

	// TODO(fejta): FIX ME
	// The build-release script needs stdin to ask the user whether
	// it's OK to download the docker image.
	return control.FinishRunning(exec.Command("make", "-C", util.K8s("kubernetes"), target))
}

type buildFederationStrategy struct {
	buildStrategy
}

type buildIngressGCEStrategy struct {
	buildStrategy
}

func (b *buildFederationStrategy) Type() string {
	return "buildFederationStrategy"
}

func (b *buildIngressGCEStrategy) Type() string {
	return "buildIngressGCEStrategy"
}

// Build federation according to specified strategy.
// This may be a bazel, quick or full release build depending on --build=B.
func (b *buildFederationStrategy) Build() error {
	var target string
	switch b.String() {
	case "bazel":
		target = "bazel-release"
	case "quick":
		target = "quick-release"
	case "release":
		target = "release"
	default:
		return fmt.Errorf("unknown federation build strategy: %v", b)
	}

	return control.FinishRunning(exec.Command("make", "-C", util.K8s("federation"), target))
}
