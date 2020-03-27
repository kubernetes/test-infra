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

// Package junit provides a junit viewer for Spyglass
package junit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/GoogleCloudPlatform/testgrid/metadata/junit"
	"k8s.io/test-infra/prow/spyglass/lenses"
)

const (
	name     = "junit"
	title    = "JUnit"
	priority = 5
)

func init() {
	lenses.RegisterLens(Lens{})
}

// Lens is the implementation of a JUnit-rendering Spyglass lens.
type Lens struct{}

type JVD struct {
	NumTests int
	Passed   []TestResult
	Failed   []TestResult
	Skipped  []TestResult
	Flaky    []TestResult
}

// Config returns the lens's configuration.
func (lens Lens) Config() lenses.LensConfig {
	return lenses.LensConfig{
		Name:     name,
		Title:    title,
		Priority: priority,
	}
}

// Header renders the content of <head> from template.html.
func (lens Lens) Header(artifacts []lenses.Artifact, resourceDir string, config json.RawMessage) string {
	t, err := template.ParseFiles(filepath.Join(resourceDir, "template.html"))
	if err != nil {
		return fmt.Sprintf("<!-- FAILED LOADING HEADER: %v -->", err)
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "header", nil); err != nil {
		return fmt.Sprintf("<!-- FAILED EXECUTING HEADER TEMPLATE: %v -->", err)
	}
	return buf.String()
}

// Callback does nothing.
func (lens Lens) Callback(artifacts []lenses.Artifact, resourceDir string, data string, config json.RawMessage) string {
	return ""
}

type JunitResult struct {
	junit.Result
}

func (jr JunitResult) Duration() time.Duration {
	return time.Duration(jr.Time * float64(time.Second)).Round(time.Second)
}

// TestResult holds data about a test extracted from junit output
type TestResult struct {
	Junit JunitResult
	Link  string
}

// Body renders the <body> for JUnit tests
func (lens Lens) Body(artifacts []lenses.Artifact, resourceDir string, data string, config json.RawMessage) string {
	jvd := lens.getJvd(artifacts)

	junitTemplate, err := template.ParseFiles(filepath.Join(resourceDir, "template.html"))
	if err != nil {
		logrus.WithError(err).Error("Error executing template.")
		return fmt.Sprintf("Failed to load template file: %v", err)
	}

	var buf bytes.Buffer
	if err := junitTemplate.ExecuteTemplate(&buf, "body", jvd); err != nil {
		logrus.WithError(err).Error("Error executing template.")
	}

	return buf.String()
}

func (lens Lens) getJvd(artifacts []lenses.Artifact) JVD {
	type testResults struct {
		// Group results based on their full path name
		junit [][]junit.Result
		link  string
		path  string
		err   error
	}
	resultChan := make(chan testResults)
	for _, artifact := range artifacts {
		go func(artifact lenses.Artifact) {
			groups := make(map[string][]junit.Result)
			result := testResults{
				link: artifact.CanonicalLink(),
				path: artifact.JobPath(),
			}
			var contents []byte
			contents, result.err = artifact.ReadAll()
			if result.err != nil {
				logrus.WithError(result.err).WithField("artifact", artifact.CanonicalLink()).Warn("Error reading artifact")
				resultChan <- result
				return
			}
			var suites junit.Suites
			suites, result.err = junit.Parse(contents)
			if result.err != nil {
				logrus.WithError(result.err).WithField("artifact", artifact.CanonicalLink()).Info("Error parsing junit file.")
				resultChan <- result
				return
			}
			var record func(suite junit.Suite)
			record = func(suite junit.Suite) {
				for _, subSuite := range suite.Suites {
					record(subSuite)
				}

				for _, test := range suite.Results {
					test := test
					// There are cases where multiple entries of exactly the same
					// testcase in a single junit result file, this could result
					// from reruns of test cases by `go test --count=N` where N>1.
					// Deduplicate them here in this case, and classify a test as being
					// flaky if it both succeeded and failed
					testFullName := fmt.Sprintf("%s...%s...%s", suite.Name, test.ClassName, test.Name)
					if _, ok := groups[testFullName]; !ok {
						groups[testFullName] = make([]junit.Result, 0)
					}
					groups[testFullName] = append(groups[testFullName], test)
				}
			}
			for _, suite := range suites.Suites {
				record(suite)
			}
			for _, results := range groups {
				result.junit = append(result.junit, results)
			}
			resultChan <- result
		}(artifact)
	}
	results := make([]testResults, 0, len(artifacts))
	for range artifacts {
		results = append(results, <-resultChan)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].path < results[j].path })

	jvd := JVD{}

	for _, result := range results {
		if result.err != nil {
			continue
		}
		for _, tests := range result.junit {
			var skipped, passed, failed, flaky bool
			var combinedTest junit.Result
			var cumFailure []string
			for _, test := range tests {
				// skipped test has no reason to rerun, so no deduplication
				if test.Skipped != nil {
					skipped = true
					combinedTest = test
				} else if test.Failure != nil {
					if passed {
						passed = false
						failed = false
						flaky = true
					}
					if !flaky {
						failed = true
					}
					cumFailure = append(cumFailure, *test.Failure)
					combinedTest = test
				} else {
					if failed {
						passed = false
						failed = false
						flaky = true
					}
					if !flaky {
						passed = true
						combinedTest = test
					}
				}
			}

			if len(cumFailure) > 0 {
				failure := strings.Join(cumFailure,
					"\n\n---Separation line for tests that failed reruns---\n\n")
				combinedTest.Failure = &failure
			}

			if skipped {
				jvd.Skipped = append(jvd.Skipped, TestResult{
					Junit: JunitResult{combinedTest},
					Link:  result.link,
				})
			} else if failed {
				jvd.Failed = append(jvd.Failed, TestResult{
					Junit: JunitResult{combinedTest},
					Link:  result.link,
				})
			} else if flaky {
				jvd.Flaky = append(jvd.Flaky, TestResult{
					Junit: JunitResult{combinedTest},
					Link:  result.link,
				})
			} else {
				jvd.Passed = append(jvd.Passed, TestResult{
					Junit: JunitResult{combinedTest},
					Link:  result.link,
				})
			}

		}
	}

	jvd.NumTests = len(jvd.Passed) + len(jvd.Failed) + len(jvd.Skipped)
	return jvd
}
