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

package restcoverage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"path/filepath"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/spyglass/api"
	"k8s.io/test-infra/prow/spyglass/lenses"
)

const (
	// DefaultWarningThreshold returns default threshold for warning class
	DefaultWarningThreshold = 40.0
	// DefaultErrorThreshold returns default threshold for error class
	DefaultErrorThreshold = 10.0
)

type Lens struct{}

// Coverage represents a REST API statistics
type Coverage struct {
	UniqueHits         int                             `json:"uniqueHits"`
	ExpectedUniqueHits int                             `json:"expectedUniqueHits"`
	Percent            float64                         `json:"percent"`
	Endpoints          map[string]map[string]*Endpoint `json:"endpoints"`
	*Thresholds
}

// Endpoint represents a basic statistics structure which is used to calculate REST API coverage
type Endpoint struct {
	Params             `json:"params"`
	UniqueHits         int     `json:"uniqueHits"`
	ExpectedUniqueHits int     `json:"expectedUniqueHits"`
	Percent            float64 `json:"percent"`
	MethodCalled       bool    `json:"methodCalled"`
}

// Params represents body and query parameters
type Params struct {
	Body  Trie `json:"body"`
	Query Trie `json:"query"`
}

// Trie represents a coverage data
type Trie struct {
	Root               Node `json:"root"`
	UniqueHits         int  `json:"uniqueHits"`
	ExpectedUniqueHits int  `json:"expectedUniqueHits"`
	Size               int  `json:"size"`
	Height             int  `json:"height"`
}

// Node represents a single data unit for coverage report
type Node struct {
	Hits  int              `json:"hits"`
	Items map[string]*Node `json:"items,omitempty"`
}

// Thresholds sets color (yellow or red) to highlight coverage percent
type Thresholds struct {
	Warning float64 `json:"threshold_warning"`
	Error   float64 `json:"threshold_error"`
}

func init() {
	lenses.RegisterLens(Lens{})
}

// Config returns the lens's configuration.
func (lens Lens) Config() lenses.LensConfig {
	return lenses.LensConfig{
		Title:    "REST API coverage report",
		Name:     "restcoverage",
		Priority: 0,
	}
}

// Header returns the content of <head>
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

func (lens Lens) Callback(artifacts []api.Artifact, resourceDir string, data string, config json.RawMessage, spyglassConfig config.Spyglass) string {
	return ""
}

// Body returns the displayed HTML for the <body>
func (lens Lens) Body(artifacts []api.Artifact, resourceDir string, data string, config json.RawMessage, spyglassConfig config.Spyglass) string {
	var (
		cov Coverage
		err error
	)

	cov.Thresholds, err = getThresholds(config)
	if err != nil {
		logrus.Errorf("Invalid config: %v", err)
		return fmt.Sprintf("Invalid config: %v", err)
	}

	if len(artifacts) != 1 {
		logrus.Errorf("Invalid artifacts: %v", artifacts)
		return "Either artifact is not passed or it is too many of them! Let's call it an error :)"
	}

	covJSON, err := artifacts[0].ReadAll()
	if err != nil {
		logrus.Errorf("Failed to read artifact: %v", err)
		return fmt.Sprintf("Failed to read artifact: %v", err)
	}

	err = json.Unmarshal(covJSON, &cov)
	if err != nil {
		logrus.Errorf("Failed to unmarshall coverage report: %v", err)
		return fmt.Sprintf("Failed to unmarshall coverage report: %v", err)
	}

	restcoverageTemplate, err := template.ParseFiles(filepath.Join(resourceDir, "template.html"))
	if err != nil {
		logrus.WithError(err).Error("Error executing template.")
		return fmt.Sprintf("Failed to load template file: %v", err)
	}

	var buf bytes.Buffer
	if err := restcoverageTemplate.ExecuteTemplate(&buf, "body", cov); err != nil {
		logrus.WithError(err).Error("Error executing template.")
	}

	return buf.String()
}

func getThresholds(config json.RawMessage) (*Thresholds, error) {
	var thresholds Thresholds

	if len(config) == 0 {
		thresholds.Error = DefaultErrorThreshold
		thresholds.Warning = DefaultWarningThreshold
		return &thresholds, nil
	}

	if err := json.Unmarshal(config, &thresholds); err != nil {
		return nil, err
	}

	if thresholds.Error > thresholds.Warning {
		return nil, fmt.Errorf("errorThreshold %.2f is bigger than warningThreshold %.2f", thresholds.Error, thresholds.Warning)
	}

	return &thresholds, nil
}
