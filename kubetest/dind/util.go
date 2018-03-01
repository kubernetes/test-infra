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

// Package dind implements dind specific kubetest code.
package dind

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"k8s.io/test-infra/kubetest/util"
)

// GetDockerVersion reads the stable-status.txt file from bazel to get the docker artifact version.
func GetDockerVersion() (string, error) {
	fileName := util.K8s("kubernetes", "bazel-out", "stable-status.txt")
	file, err := os.Open(fileName)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		tokens := strings.Fields(scanner.Text())
		if len(tokens) < 2 {
			continue
		}
		if tokens[0] != "STABLE_DOCKER_TAG" {
			continue
		}
		return tokens[1], nil
	}

	if err = scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("docker tag not found in file %s", fileName)
}
