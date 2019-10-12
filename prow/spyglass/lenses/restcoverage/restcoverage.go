package restcoverage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"path/filepath"

	"k8s.io/test-infra/prow/spyglass/lenses"

	"github.com/sirupsen/logrus"
)

type Lens struct{}

// Coverage represents a REST API statistics
type Coverage struct {
	UniqueHits         int                             `json:"uniqueHits"`
	ExpectedUniqueHits int                             `json:"expectedUniqueHits"`
	Percent            float64                         `json:"percent"`
	Endpoints          map[string]map[string]*Endpoint `json:"endpoints"`
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
	Body  *Trie `json:"body"`
	Query *Trie `json:"query"`
}

// Trie represents a coverage data
type Trie struct {
	Root               *Node `json:"root"`
	UniqueHits         int   `json:"uniqueHits"`
	ExpectedUniqueHits int   `json:"expectedUniqueHits"`
	Size               int   `json:"size"`
	Height             int   `json:"height"`
}

// Node represents a single data unit for coverage report
type Node struct {
	Hits  int              `json:"hits"`
	Items map[string]*Node `json:"items,omitempty"`
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

func (lens Lens) Callback(artifacts []lenses.Artifact, resourceDir string, data string, config json.RawMessage) string {
	return ""
}

// Body returns the displayed HTML for the <body>
func (lens Lens) Body(artifacts []lenses.Artifact, resourceDir string, data string, config json.RawMessage) string {
	var cov Coverage

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
