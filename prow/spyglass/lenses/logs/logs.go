/*
Copyright 2021 The Kubernetes Authors.

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

package logs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/spyglass/api"
	"k8s.io/test-infra/prow/spyglass/lenses"
)

const (
	name     = "logs"
	title    = "Master and node logs"
	priority = 20
)

func init() {
	lenses.RegisterLens(Lens{})
}

// Lens prints link to master and node logs.
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
func (lens Lens) Header(artifacts []api.Artifact, resourceDir string, config json.RawMessage) string {
	output, err := renderTemplate(resourceDir, "header", nil)
	if err != nil {
		logrus.Warnf("Failed to render header: %v", err)
		return "Error: " + err.Error()
	}
	return output
}

func renderTemplate(resourceDir, block string, params interface{}) (string, error) {
	t, err := template.ParseFiles(filepath.Join(resourceDir, "template.html"))
	if err != nil {
		return "", fmt.Errorf("Failed to parse template: %v", err)
	}

	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, block, params); err != nil {
		return "", fmt.Errorf("Failed to execute template: %v", err)
	}
	return buf.String(), nil
}

// Body renders link to logs.
func (lens Lens) Body(artifacts []api.Artifact, resourceDir string, data string, config json.RawMessage) string {
	if len(artifacts) == 0 {
		return "No artifacts found"
	}
	content, err := artifacts[0].ReadAll()
	if err != nil {
		logrus.WithError(err).Warn("Failed to read artifact file.")
		return fmt.Sprintf("Failed to read artifact file: %v", err)
	}

	params := struct {
		LogsURL string
	}{
		LogsURL: strings.TrimSpace(string(content)),
	}

	output, err := renderTemplate(resourceDir, "body", params)
	if err != nil {
		logrus.Warnf("Failed to render body: %v", err)
		return "Error: " + err.Error()
	}
	return output
}

// Callback does nothing.
func (lens Lens) Callback(artifacts []api.Artifact, resourceDir string, data string, config json.RawMessage) string {
	return ""
}
