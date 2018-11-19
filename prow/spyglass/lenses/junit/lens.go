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

	junit "github.com/joshdk/go-junit"
	"github.com/sirupsen/logrus"

	"fmt"
	"html/template"
	"k8s.io/test-infra/prow/spyglass/lenses"
	"path/filepath"
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

// Name returns the name.
func (lens Lens) Name() string {
	return name
}

// Title returns the title.
func (lens Lens) Title() string {
	return title
}

// Priority returns the priority.
func (lens Lens) Priority() int {
	return priority
}

// Header renders the content of <head> from template.html.
func (lens Lens) Header(artifacts []lenses.Artifact, resourceDir string) string {
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
func (lens Lens) Callback(artifacts []lenses.Artifact, resourceDir string, data string) string {
	return ""
}

// TestResult holds data about a test extracted from junit output
type TestResult struct {
	Junit junit.Test
	Link  string
}

// Body renders the <body> for JUnit tests
func (lens Lens) Body(artifacts []lenses.Artifact, resourceDir string, data string) string {
	type JunitViewData struct {
		NumTests int
		Passed   []TestResult
		Failed   []TestResult
		Skipped  []TestResult
	}

	jvd := JunitViewData{
		Passed:   []TestResult{},
		Failed:   []TestResult{},
		Skipped:  []TestResult{},
		NumTests: 0,
	}

	for _, a := range artifacts {
		contents, err := a.ReadAll()
		suites, err := junit.Ingest(contents)
		if err != nil {
			logrus.WithError(err).Error("Error parsing junit file.")
		}
		for _, suite := range suites {
			for _, test := range suite.Tests {
				if test.Status == "failed" {
					jvd.Failed = append(jvd.Failed, TestResult{
						Junit: test,
						Link:  a.CanonicalLink(),
					})
				} else if test.Status == "skipped" {
					jvd.Skipped = append(jvd.Skipped, TestResult{
						Junit: test,
						Link:  a.CanonicalLink(),
					})
				} else if test.Status == "passed" {
					jvd.Passed = append(jvd.Passed, TestResult{
						Junit: test,
						Link:  a.CanonicalLink(),
					})
				} else {
					logrus.Error("Invalid test status string: ", test.Status)
				}
			}
		}
		jvd.NumTests = len(jvd.Passed) + len(jvd.Failed) + len(jvd.Skipped)

	}

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
