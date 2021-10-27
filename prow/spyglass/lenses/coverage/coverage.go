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
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/spyglass/api"
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
func (lens Lens) Header(artifacts []api.Artifact, resourceDir string, config json.RawMessage, spyglassConfig config.Spyglass) string {
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
func (lens Lens) Callback(artifacts []api.Artifact, resourceDir string, data string, config json.RawMessage, spyglassConfig config.Spyglass) string {
	return ""
}

// Body renders the <body>
func (lens Lens) Body(artifacts []api.Artifact, resourceDir string, data string, config json.RawMessage, spyglassConfig config.Spyglass) string {
	if len(artifacts) == 0 {
		logrus.Error("coverage Body() called with no artifacts, which should never happen.")
		return "Why am I here? There is no coverage file."
	}

	profileArtifact := artifacts[0]
	var htmlArtifact api.Artifact
	if len(artifacts) > 1 {
		if len(artifacts) > 2 {
			return "Too many files - expected one coverage file and one optional HTML file"
		}
		if strings.HasSuffix(artifacts[0].JobPath(), ".html") {
			htmlArtifact = artifacts[0]
			profileArtifact = artifacts[1]
		} else if strings.HasSuffix(artifacts[1].JobPath(), ".html") {
			htmlArtifact = artifacts[1]
			profileArtifact = artifacts[0]
		} else {
			return "Multiple input files, but none had a .html extension."
		}
	}

	content, err := profileArtifact.ReadAll()
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

	renderedCoverageURL := ""
	if htmlArtifact != nil {
		renderedCoverageURL = htmlArtifact.CanonicalLink()
	}
	t := struct {
		CoverageContent  string
		RenderedCoverage string
	}{
		CoverageContent:  result,
		RenderedCoverage: renderedCoverageURL,
	}
	var buf bytes.Buffer
	if err := coverageTemplate.ExecuteTemplate(&buf, "body", t); err != nil {
		logrus.WithError(err).Error("Error executing template.")
	}

	return buf.String()
}
