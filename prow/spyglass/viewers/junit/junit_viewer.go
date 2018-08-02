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
	"fmt"

	"html/template"

	junit "github.com/joshdk/go-junit"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/spyglass/viewers"
)

const (
	name     = "junit-viewer"
	title    = "JUnit"
	priority = 5
)

func init() {
	viewers.RegisterViewer(name, viewers.ViewMetadata{
		Title:    title,
		Priority: priority,
	}, ViewHandler)
}

type TestResult struct {
	Junit junit.Test
	Link  string
}

// ViewHandler creates a view for JUnit tests
func ViewHandler(artifacts []viewers.Artifact, raw string) string {
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

	var buf bytes.Buffer
	t := template.Must(template.New(fmt.Sprintf("%sTemplate", name)).Parse(tmplt))
	if err := t.Execute(&buf, jvd); err != nil {
		logrus.WithError(err).Error("Error executing template.")
	}

	return buf.String()
}
