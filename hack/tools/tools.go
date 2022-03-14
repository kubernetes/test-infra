//go:build tools
// +build tools

/*
Copyright 2021 The Kubernetes Authors.

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

/*
Package tools is used to track binary dependencies with go modules
https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module
*/
package tools

import (
	// linter(s)
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"

	// kubernetes code generators
	_ "github.com/go-bindata/go-bindata/v3"
	_ "k8s.io/code-generator/cmd/client-gen"
	_ "k8s.io/code-generator/cmd/deepcopy-gen"
	_ "k8s.io/code-generator/cmd/informer-gen"
	_ "k8s.io/code-generator/cmd/lister-gen"
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"

	// proto generator
	_ "github.com/golang/protobuf/protoc-gen-go"

	// test runner
	_ "gotest.tools/gotestsum"

	// bazel-related tools
	_ "github.com/bazelbuild/buildtools/buildozer"

	_ "github.com/client9/misspell/cmd/misspell"

	// image builder
	_ "github.com/google/ko"

	// caching
	_ "github.com/sethvargo/gcs-cacher"
)
