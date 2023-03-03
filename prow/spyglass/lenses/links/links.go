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

package links

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/spyglass/api"
	"k8s.io/test-infra/prow/spyglass/lenses"
)

const (
	name     = "links"
	title    = "Debugging links"
	priority = 20

	bytesLimit = 20 * 1024
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
func (lens Lens) Header(artifacts []api.Artifact, resourceDir string, config json.RawMessage, spyglassConfig config.Spyglass) string {
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
		return "", fmt.Errorf("Failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, block, params); err != nil {
		return "", fmt.Errorf("Failed to execute template: %w", err)
	}
	return buf.String(), nil
}

// humanReadableName translates a fileName to human readable name, e.g.:
// * master-and-node-logs.txt -> "Master and node logs"
// * dashboard.link.txt -> "Dashboard"
func humanReadableName(name string) string {
	name = strings.TrimSuffix(name, ".link.txt")
	name = strings.TrimSuffix(name, ".txt")
	words := strings.Split(name, "-")
	if len(words) > 0 {
		words[0] = cases.Title(language.English).String(words[0])
	}
	return strings.Join(words, " ")
}

func clickableLink(link string, spyglassConfig config.Spyglass) string {
	if strings.HasPrefix(link, "gs://") {
		return spyglassConfig.GCSBrowserPrefix + strings.TrimPrefix(link, "gs://")
	}
	if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") {
		// Use https://google.com/url?q=[url] redirect to avoid leaking referer.
		url := url.URL{
			Scheme: "https",
			Host:   "google.com",
			Path:   "/url",
		}
		q := url.Query()
		q.Add("q", link)
		url.RawQuery = q.Encode()
		return url.String()
	}
	return ""
}

type link struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Link string `json:"-"`
}

type linkGroup struct {
	Title string `json:"title"`
	Links []link `json:"links"`
}

func toLink(jobPath string, content []byte, spyglassConfig config.Spyglass) link {
	url := strings.TrimSpace(string(content))
	return link{
		Name: humanReadableName(filepath.Base(jobPath)),
		URL:  url,
		Link: clickableLink(url, spyglassConfig),
	}
}

func parseLinkFile(jobPath string, content []byte, spyglassConfig config.Spyglass) (linkGroup, error) {
	if extension := filepath.Ext(jobPath); extension == ".json" {
		group := linkGroup{}
		if err := json.Unmarshal(content, &group); err != nil {
			return group, err
		}
		for i := range group.Links {
			group.Links[i].Link = clickableLink(group.Links[i].URL, spyglassConfig)
		}
		return group, nil
	}
	// Wrap a single link inside a linkGroup.
	wrappedLink := toLink(jobPath, content, spyglassConfig)
	return linkGroup{
		Title: wrappedLink.Name,
		Links: []link{
			wrappedLink,
		},
	}, nil
}

// Body renders link to logs.
func (lens Lens) Body(artifacts []api.Artifact, resourceDir string, data string, config json.RawMessage, spyglassConfig config.Spyglass) string {
	var linkGroups []linkGroup
	for _, artifact := range artifacts {
		jobPath := artifact.JobPath()
		content, err := artifact.ReadAtMost(bytesLimit)
		if err != nil && err != io.EOF {
			logrus.WithError(err).Warnf("Failed to read artifact file: %q", jobPath)
			continue
		}
		group, err := parseLinkFile(jobPath, content, spyglassConfig)
		if err != nil {
			logrus.WithError(err).Warnf("Failed to parse link file: %q", jobPath)
			continue
		}
		linkGroups = append(linkGroups, group)
	}

	sort.Slice(linkGroups, func(i, j int) bool { return linkGroups[i].Title < linkGroups[j].Title })

	params := struct {
		LinkGroups []linkGroup
	}{
		LinkGroups: linkGroups,
	}

	output, err := renderTemplate(resourceDir, "body", params)
	if err != nil {
		logrus.Warnf("Failed to render body: %v", err)
		return "Error: " + err.Error()
	}
	return output
}

// Callback does nothing.
func (lens Lens) Callback(artifacts []api.Artifact, resourceDir string, data string, config json.RawMessage, spyglassConfig config.Spyglass) string {
	return ""
}
