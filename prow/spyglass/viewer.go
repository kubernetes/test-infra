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

package spyglass

import (
	"bytes"
	"html/template"

	"github.com/sirupsen/logrus"
)

// An artifact viewer for JUnit tests
type JUnitViewer struct {
	title string
}

// An artifact viewer for build logs
type BuildLogViewer struct {
	title string
}

// An artifact viewer for prow job metadata
type MetadataViewer struct {
	title string
}

// Title gets the title of the JUnit View
func (v *JUnitViewer) Title() string {
	return v.title
}

// Title gets the title of the JUnit View
func (v *BuildLogViewer) Title() string {
	return v.title
}

// Title gets the title of the Metadata View
func (v *MetadataViewer) Title() string {
	return v.title
}

// View creates a view for a build log (or multiple build logs)
func (v *BuildLogViewer) View(artifacts []Artifact) string {
	logViewTmpl := `
<div>
	{{range .LogViews}}<div>
		{{.LogLines}}
	</div>{{end}}
</div>`
	var buf bytes.Buffer
	type LogFileView struct {
		// requestMore string TODO
		LogLines string
	}
	type BuildLogsView struct {
		LogViews []LogFileView
	}
	var buildLogsView BuildLogsView
	for _, a := range artifacts {
		buildLogsView.LogViews = append(buildLogsView.LogViews, LogFileView{LogLines: LastNLines(a, 100)})
	}
	t := template.Must(template.New("BuildLogView").Parse(logViewTmpl))
	err := t.Execute(&buf, buildLogsView)
	if err != nil {
		logrus.Errorf("Template failed with error: %s", err)
	}
	return buf.String()
}

// View creates a view for JUnit tests
func (v *JUnitViewer) View(artifacts []Artifact) string {
	//TODO
	return ""
}

// View creates a view for prow job metadata
func (v *MetadataViewer) View(artifacts []Artifact) string {
	//TODO
	return ""
}
