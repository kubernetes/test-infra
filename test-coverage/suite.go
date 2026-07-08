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

import "fmt"

// e2eSuite describes one of the Ginkgo test suites in the kubernetes/test
// directory that this tool knows how to analyze.
type e2eSuite struct {
	// testPackage is the Go package (relative to the kubernetes repo root)
	// which contains the suite, e.g. "./test/e2e_node".
	testPackage string
	// junitClassname is the "classname" attribute that JUnit testcase
	// elements have when they were produced by this suite, e.g.
	// "E2eNode Suite". Used to recognize (and ignore) job runs that
	// executed a different suite.
	junitClassname string
	// defaultJobDir is the default value of the -job-dir flag when this
	// suite is selected, relative to the test-infra repo root.
	defaultJobDir string
}

var e2eSuites = map[string]e2eSuite{
	"e2e": {
		testPackage:    "./test/e2e",
		junitClassname: "Kubernetes e2e suite",
		defaultJobDir:  "config/jobs/kubernetes",
	},
	"e2e_node": {
		testPackage:    "./test/e2e_node",
		junitClassname: "E2eNode Suite",
		defaultJobDir:  "config/jobs/kubernetes/sig-node",
	},
}

// lookupE2ESuite returns the e2eSuite for name, or an error if name is not
// one of the supported suites.
func lookupE2ESuite(name string) (e2eSuite, error) {
	suite, ok := e2eSuites[name]
	if !ok {
		return e2eSuite{}, fmt.Errorf("unsupported -e2e-suite %q, must be one of \"e2e\" or \"e2e_node\"", name)
	}
	return suite, nil
}

// isKnownSuiteClassname reports whether classname matches the
// junitClassname of one of the known e2eSuites.
func isKnownSuiteClassname(classname string) bool {
	for _, suite := range e2eSuites {
		if suite.junitClassname == classname {
			return true
		}
	}
	return false
}

// suiteNameForClassname returns the short name ("e2e", "e2e_node") of the
// known e2eSuite whose junitClassname matches classname, or classname
// itself if it does not belong to any known suite.
func suiteNameForClassname(classname string) string {
	for name, suite := range e2eSuites {
		if suite.junitClassname == classname {
			return name
		}
	}
	return classname
}
