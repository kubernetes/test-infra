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

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/bazel-test-infra/external/go_sdk/src/html/template"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

const (
	name     = "JUnitViewer"
	title    = "JUnit"
	priority = 5
)

func init() {
	viewers.RegisterViewer(name, viewers.ViewMetadata{
		Title:    title,
		Priority: priority,
	}, ViewHandler)
}

// TestResult holds information about a test run
type TestResult struct {
	TestName string
	Passed   bool
}

// ViewHandler creates a view for JUnit tests
func ViewHandler(artifacts []viewers.Artifact, raw string) string {
	type JunitViewData struct {
		NumTests int
		Passed   []TestResult
		Failed   []TestResult
	}

	for _, a := range artifacts {
		contents, err := a.ReadAll()

	}

	var buf bytes.Buffer
	t := template.Must(template.New(fmt.Sprintf("%sTemplate", name)).Parse(tmplt))
	if err := t.Execute(&buf, junitViewData); err != nil {
		logrus.WithError(err).Error("Error executing template.")
	}

	return buf.String()
}
