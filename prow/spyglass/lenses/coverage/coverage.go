/*
Copyright 2019 The Kubernetes Authors.

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

// Package coverage provides a coverage viewer for Spyglass
package coverage

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"path/filepath"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/spyglass/lenses"
)

const (
	name     = "coverage"
	title    = "Coverage"
	priority = 7
)

func init() {
	lenses.RegisterLens(Lens{})
}

// Lens is the implementation of a coverage-rendering Spyglass lens.
type Lens struct{}

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

// Body renders the <body>
func (lens Lens) Body(artifacts []lenses.Artifact, resourceDir string, data string, config json.RawMessage) string {
	if len(artifacts) == 0 {
		logrus.Error("coverage Body() called with no artifacts, which should never happen.")
		return "Why am I here? There is no coverage file."
	}

	content, err := artifacts[0].ReadAll()
	if err != nil {
		logrus.WithError(err).Warn("Couldn't read a coverage file that should exist.")
		return fmt.Sprintf("Faiiled to read the coverage file: %v", err)
	}

	coverageTemplate, err := template.ParseFiles(filepath.Join(resourceDir, "template.html"))
	if err != nil {
		logrus.WithError(err).Error("Error executing template.")
		return fmt.Sprintf("Failed to load template file: %v", err)
	}

	w := &bytes.Buffer{}
	g := gzip.NewWriter(w)
	_, err = g.Write(content)
	if err != nil {
		logrus.WithError(err).Warn("Failed to compress coverage file")
		return fmt.Sprintf("Failed to compress coverage file: %v", err)
	}
	if err := g.Close(); err != nil {
		logrus.WithError(err).Warn("Failed to close gzip for coverage file")
		return fmt.Sprintf("Failed to close gzip for coverage file: %v", err)
	}
	result := base64.StdEncoding.EncodeToString(w.Bytes())

	t := struct {
		CoverageContent string
	}{
		CoverageContent: result,
	}
	var buf bytes.Buffer
	if err := coverageTemplate.ExecuteTemplate(&buf, "body", t); err != nil {
		logrus.WithError(err).Error("Error executing template.")
	}

	return buf.String()
}
