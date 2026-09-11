/*
Copyright The Kubernetes Authors.

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
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
)

// listTestNameRE matches one line of "go test -args --list-tests" output,
// for example:
//
//	../kubernetes/test/e2e_node/apparmor_test.go:88: [sig-node] AppArmor when running with AppArmor should enforce a permissive profile with annotations [NodeConformance]
//
// Capture group 1 is the test name without the "<file>:<line>: " prefix.
var listTestNameRE = regexp.MustCompile(`^\s*\S+:\d+:\s(.+)$`)

// listGinkgoTests runs the Ginkgo suite in testPackage (a package path
// relative to kubernetesRepoDir, e.g. "./test/e2e_node") with --list-tests
// and returns the sorted, de-duplicated set of test names that it prints.
func listGinkgoTests(kubernetesRepoDir, testPackage string) ([]string, error) {
	cmd := exec.Command("go", "test", "-v", testPackage, "-args", "--list-tests")
	cmd.Dir = kubernetesRepoDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("running %q in %s: %w\n%s", cmd.Args, kubernetesRepoDir, err, stderr.String())
	}

	seen := make(map[string]bool)
	var tests []string
	for _, line := range bytes.Split(stdout.Bytes(), []byte("\n")) {
		m := listTestNameRE.FindSubmatch(line)
		if m == nil {
			continue
		}
		name := string(m[1])
		if seen[name] {
			continue
		}
		seen[name] = true
		tests = append(tests, name)
	}
	if len(tests) == 0 {
		return nil, fmt.Errorf("no tests found in --list-tests output of %q", cmd.Args)
	}
	sort.Strings(tests)
	return tests, nil
}
