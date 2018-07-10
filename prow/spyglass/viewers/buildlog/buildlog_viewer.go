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

// Package buildlog provides a build log viewer for Spyglass
package buildlog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

const (
	name          = "BuildLogViewer"
	title         = "Build Log"
	priority      = 10
	viewChunkSize = 1e3 // 1KB
	nLines        = 100
)

func init() {
	viewers.RegisterViewer(name, viewers.ViewMetadata{
		Title:    title,
		Priority: priority,
	}, ViewHandler)
}

// LogViewerRequest contains the operations to perform on each log artifact
type LogViewerRequest struct {
	// Requests is a map of log artifact names to operations to perform on that log artifact
	Requests map[string]LogViewData `json:"requests"`
}

// LogViewData holds the current state of a log artifact's view and the operations to be performed on it
type LogViewData struct {
	CurrentChunks int  `json:"currentChunks"`
	More          bool `json:"more"`
}

//LogArtifactView holds a single log file's view
type LogArtifactView struct {
	ArtifactName          string
	ArtifactLink          string
	ViewName              string
	ViewMethodDescription string
	LogLines              []string
}

// BuildLogsView holds each log file view
type BuildLogsView struct {
	LogViews           []LogArtifactView
	RefreshRequests    map[string]LogViewerRequest
	RawRefreshRequests map[string]string
}

// ViewHandler creates a view for a build log (or multiple build logs)
func ViewHandler(artifacts []viewers.Artifact, raw string) string {
	buildLogsView := BuildLogsView{
		LogViews:           []LogArtifactView{},
		RefreshRequests:    make(map[string]LogViewerRequest),
		RawRefreshRequests: make(map[string]string),
	}
	viewReq := LogViewerRequest{
		Requests: make(map[string]LogViewData),
	}
	if err := json.Unmarshal([]byte(raw), &viewReq); err != nil {
		logrus.WithError(err).Error("Error unmarshaling log view request data.")
	}

	updatedViewReq := LogViewerRequest{
		Requests: make(map[string]LogViewData),
	}

	var desc string
	var chunks int
	// Read log artifacts and construct template structs
	for _, a := range artifacts {
		viewData, ok := viewReq.Requests[a.JobPath()]
		if ok {
			logrus.Info("Found artifact in refresh request")
			chunks = viewData.CurrentChunks
			if viewData.More {
				logrus.Info("Requesting more for artifact ", a.JobPath())
				chunks += 1
			}
		} else {
			logrus.Info("Did not find artifact in refresh request")
			chunks = 1
		}
		logLines, err := viewers.LastNLines(a, int64(nLines*chunks))
		desc = fmt.Sprintf("viewing last %d lines", nLines*chunks)
		if err != nil {
			logrus.WithError(err).Error("Last N lines failed.")
			desc = fmt.Sprintf("viewing first %d bytes", viewChunkSize*chunks)
			if err == viewers.ErrUnsupportedOp {
				read, err := a.ReadAtMost(int64(viewChunkSize * chunks))
				if err != nil {
					if err != io.EOF {
						logrus.WithError(err).Error("Failed reading lines")
					}
				}
				logLines = strings.Split(string(read), "\n")
				for ix, line := range logLines {
					logLines[ix] = fmt.Sprintf("%d.\t%s", ix+1, line)
				}
			}
		}
		buildLogsView.LogViews = append(buildLogsView.LogViews, LogArtifactView{
			ArtifactName:          a.JobPath(),
			ArtifactLink:          a.CanonicalLink(),
			ViewName:              name,
			ViewMethodDescription: desc,
			LogLines:              logLines,
		})
		updatedViewReq.Requests[a.JobPath()] = LogViewData{
			CurrentChunks: chunks,
			More:          false,
		}

	}

	// Build individualized requests for stateless callbacks
	for _, a := range artifacts {
		aName := a.JobPath()
		buildLogsView.RefreshRequests[aName] = updatedViewReq
		refreshReq := buildLogsView.RefreshRequests[aName]
		reqData := refreshReq.Requests[aName]
		reqData.More = true
		refreshReq.Requests[aName] = reqData
		buildLogsView.RefreshRequests[aName] = refreshReq
		raw, err := json.Marshal(refreshReq)
		if err != nil {
			logrus.WithError(err).Error("Failed to marshal build log more lines request object.")
		}
		buildLogsView.RawRefreshRequests[aName] = string(raw)
	}

	logrus.Info("BuildLogsView struct refresh requests: ", buildLogsView.RefreshRequests)
	return LogViewTemplate(buildLogsView)
}

// Executes a log view template ready for rendering
func LogViewTemplate(buildLogsView BuildLogsView) string {
	logViewTmpl := `
<div style="font-family:monospace;">
	{{range .LogViews}}<h4><a href="{{.ArtifactLink}}">{{.ArtifactName}}</a> - {{.ViewMethodDescription}}</h4>
	<ul style="list-style-type:none;padding:0;margin:0;line-height:1.4;color:black;">
		{{range $ix, $e := .LogLines}}
			<li>{{$e}}</li>
		{{end}}
	</ul>
	<button onclick="refreshView({{.ViewName}}, '{{index $.RawRefreshRequests .ArtifactName}}')" class="mdl-button mdl-js-button mdl-button--primary">More Lines Please</button>{{end}}
</div>`
	var buf bytes.Buffer

	t := template.Must(template.New(name).Parse(logViewTmpl))
	err := t.Execute(&buf, buildLogsView)
	if err != nil {
		logrus.WithError(err).Error("Template failed.")
	}
	return buf.String()
}
