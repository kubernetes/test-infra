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
	"math"
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

// LogViewData holds the current state of a log artifact's view and the operation to be performed on it.
// Valid operations: more, all, ""
type LogViewData struct {
	CurrentChunks int    `json:"currentChunks"`
	TotalChunks   int    `json:"totalChunks"`
	Operation     string `json:"operation"`
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
	GetAllRequests     map[string]LogViewerRequest
	GetMoreRequests    map[string]LogViewerRequest
	RawGetAllRequests  map[string]string
	RawGetMoreRequests map[string]string
}

// ViewHandler creates a view for a build log (or multiple build logs)
func ViewHandler(artifacts []viewers.Artifact, raw string) string {
	buildLogsView := BuildLogsView{
		LogViews:           []LogArtifactView{},
		GetAllRequests:     make(map[string]LogViewerRequest),
		GetMoreRequests:    make(map[string]LogViewerRequest),
		RawGetAllRequests:  make(map[string]string),
		RawGetMoreRequests: make(map[string]string),
	}
	viewReq := LogViewerRequest{
		Requests: make(map[string]LogViewData),
	}
	if err := json.Unmarshal([]byte(raw), &viewReq); err != nil {
		logrus.WithError(err).Error("Error unmarshaling log view request data.")
	}

	getAllReq := LogViewerRequest{
		Requests: make(map[string]LogViewData),
	}

	getMoreReq := LogViewerRequest{
		Requests: make(map[string]LogViewData),
	}

	var desc string
	var chunks int
	var totalChunks int
	// Read log artifacts and construct template structs
	for _, a := range artifacts {
		viewData, ok := viewReq.Requests[a.JobPath()]
		if ok {
			chunks = viewData.CurrentChunks
			totalChunks = viewData.TotalChunks
			switch viewData.Operation {
			case "more":
				logrus.Info("Requesting more for artifact ", a.JobPath())
				chunks++
			case "all":
				chunks = viewData.TotalChunks
			case "":
			default:
				logrus.Error("Invalid BuildLogViewer operation provided.")
			}
		} else {
			totalChunks = int(math.Ceil(float64(a.Size()) / float64(viewChunkSize)))
			chunks = 1
		}
		logLines, err := viewers.LastNLines(a, int64(nLines*chunks))
		desc = fmt.Sprintf("viewing last %d lines", nLines*chunks)
		if err != nil {
			logrus.WithError(err).Error("Last N lines failed.")
			desc = fmt.Sprintf("viewing first %d bytes", viewChunkSize*chunks)
			if err == viewers.ErrUnsupportedOp { // Try a different read operation
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
		getAllReq.Requests[a.JobPath()] = LogViewData{
			CurrentChunks: chunks,
			TotalChunks:   totalChunks,
			Operation:     "",
		}
		getMoreReq.Requests[a.JobPath()] = LogViewData{
			CurrentChunks: chunks,
			TotalChunks:   totalChunks,
			Operation:     "",
		}

	}

	// Build individualized requests for stateless callbacks
	for _, a := range artifacts {
		aName := a.JobPath()

		buildLogsView.GetAllRequests[aName] = getAllReq
		reqAll := buildLogsView.GetAllRequests[aName]

		reqAllData := reqAll.Requests[aName]
		reqAllData.Operation = "all"
		reqAll.Requests[aName] = reqAllData
		raw, err := json.Marshal(reqAll)
		if err != nil {
			logrus.WithError(err).Error("Failed to marshal build log more lines request object.")
		}
		buildLogsView.RawGetAllRequests[aName] = string(raw)

		buildLogsView.GetMoreRequests[aName] = getMoreReq
		reqMore := buildLogsView.GetMoreRequests[aName]

		reqMoreData := reqMore.Requests[aName]
		reqMoreData.Operation = "more"
		reqMore.Requests[aName] = reqMoreData
		raw, err = json.Marshal(reqMore)
		if err != nil {
			logrus.WithError(err).Error("Failed to marshal build log more lines request object.")
		}
		buildLogsView.RawGetMoreRequests[aName] = string(raw)
	}

	return LogViewTemplate(buildLogsView)
}

// LogViewTemplate executes the log viewer template ready for rendering
func LogViewTemplate(buildLogsView BuildLogsView) string {
	var buf bytes.Buffer
	t := template.Must(template.New(fmt.Sprintf("%sTemplate", name)).Parse(tmplt))
	if err := t.Execute(&buf, buildLogsView); err != nil {
		logrus.WithError(err).Error("Error executing template.")
	}
	return buf.String()
}
