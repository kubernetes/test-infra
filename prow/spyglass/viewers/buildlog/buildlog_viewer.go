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
	"io"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

const (
	name          = "build-log-viewer"
	title         = "Build Log"
	priority      = 10
	byteChunkSize = 1e3 // 1KB
	lineChunkSize = 100
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
	Requests map[string]LogViewData `json:"requests,omitempty"`
}

// LogViewData holds the current state of a log artifact's view and the operation to be performed on it.
// Valid operations: more, all, ""
type LogViewData struct {
	HeadBytes int64 `json:"headBytes,omitempty"`
	// UseBytes is true if this log view's data cannot be accessed via Line operations like
	// viewers.LastNLines. Usually true if the data is gzipped.
	UseBytes  bool   `json:"useBytes,omitempty"`
	TailLines int64  `json:"tailLines,omitempty"`
	Operation string `json:"operation,omitempty"`
}

//LogArtifactView holds a single log file's view
type LogArtifactView struct {
	ArtifactName          string
	ArtifactLink          string
	ViewName              string
	ViewMethodDescription string
	LogLines              []string
	ViewAll               bool
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

	// Read log artifacts and construct template structs
	for _, a := range artifacts {
		viewData, ok := viewReq.Requests[a.JobPath()]
		if !ok {
			viewData = LogViewData{
				UseBytes:  false,
				HeadBytes: 0,
				TailLines: 0,
				Operation: "more",
			}
		}

		logView, logViewData := nextViewData(a, viewData)

		buildLogsView.LogViews = append(buildLogsView.LogViews, logView)
		getAllReq.Requests[a.JobPath()] = logViewData
		getMoreReq.Requests[a.JobPath()] = logViewData

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

// nextViewData constructs the new log artifact view and needed requests from the current
// view data and artifact
func nextViewData(artifact viewers.Artifact, currentViewData LogViewData) (LogArtifactView, LogViewData) {
	var logLines []string
	var err error
	newLogViewData := LogViewData{
		Operation: "",
	}
	newArtifactView := LogArtifactView{
		ArtifactName: artifact.JobPath(),
		ArtifactLink: artifact.CanonicalLink(),
		ViewName:     name,
	}
	switch currentViewData.Operation {
	case "all":
		logLines = logLinesAll(artifact)
		newArtifactView.ViewMethodDescription = fmt.Sprintf("viewing all lines")
		newArtifactView.ViewAll = true
	case "more":
		if currentViewData.UseBytes {
			nBytes := int64(currentViewData.HeadBytes + byteChunkSize)
			artifactSize, err := artifact.Size()
			if err != nil {
				logrus.WithFields(logrus.Fields{"artifact": artifact.JobPath()}).WithError(err).Error("Error getting artifact size.")
			}
			if nBytes >= artifactSize {
				logLines = logLinesAll(artifact)
				newArtifactView.ViewMethodDescription = fmt.Sprintf("viewing all lines")
				newArtifactView.ViewAll = true
			} else {
				newArtifactView.ViewMethodDescription = fmt.Sprintf("viewing first %d bytes", nBytes)
				newLogViewData.HeadBytes = nBytes
				newLogViewData.UseBytes = true
				logLines, err = logLinesHead(artifact, nBytes)
				if err != nil {
					logrus.WithError(err).Error("Reading log lines failed.")
					newArtifactView.ViewMethodDescription = fmt.Sprintf("failed to read log file - is it compressed?")
					logLines = []string{}
				}
			}
		} else {
			nLines := currentViewData.TailLines + lineChunkSize
			logLines, err = viewers.LastNLines(artifact, nLines)
			if err != nil {
				nBytes := int64(byteChunkSize)
				newLogViewData.UseBytes = true
				newLogViewData.HeadBytes = nBytes
				newArtifactView.ViewMethodDescription = fmt.Sprintf("viewing first %d bytes", nBytes)
				logLines, err = logLinesHead(artifact, nBytes)
				if err != nil {
					logrus.WithError(err).Error("Reading head of log lines failed.")
					newArtifactView.ViewMethodDescription = fmt.Sprintf("failed to read log file - is it compressed?")
					logLines = []string{}
				}
			} else {
				newLogViewData.TailLines = nLines
				newArtifactView.ViewMethodDescription = fmt.Sprintf("viewing last %d lines", nLines)
			}
		}
	case "":
	default:
	}

	newArtifactView.LogLines = logLines
	return newArtifactView, newLogViewData

}

// logLinesHead reads the first n bytes of an artifact and splits it into lines.
func logLinesHead(artifact viewers.Artifact, n int64) ([]string, error) {
	read, err := artifact.ReadAtMost(n)
	if err != nil {
		if err == io.ErrUnexpectedEOF {
			artifactSize, err := artifact.Size()
			if err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"n":        strconv.FormatInt(n, 10),
					"artifact": artifact.JobPath(),
				}).Error("Unexpected EOF, failed to get size of artifact.")
				return nil, fmt.Errorf("error getting size of artifact after unexpected EOF: %v", err)
			}
			logrus.WithError(err).WithFields(logrus.Fields{
				"n":            strconv.FormatInt(n, 10),
				"artifactSize": strconv.FormatInt(artifactSize, 10),
			}).Info("Unexpected EOF, continuing...")
		} else {
			return nil, fmt.Errorf("error reading at most n bytes from artifact: %v", err)
		}
	}
	logLines := strings.Split(string(read), "\n")
	for ix, line := range logLines {
		logLines[ix] = fmt.Sprintf("%d.\t%s", ix+1, line)
	}
	return logLines, nil
}

// logLinesAll reads all of an artifact and splits it into lines.
func logLinesAll(artifact viewers.Artifact) []string {
	read, err := artifact.ReadAll()
	if err != nil {
		if err == viewers.ErrFileTooLarge {
			logrus.WithError(err).Error("Artifact too large to read all.")
			return []string{}
		}
	}
	logLines := strings.Split(string(read), "\n")
	for ix, line := range logLines {
		logLines[ix] = fmt.Sprintf("%d.\t%s", ix+1, line)
	}
	return logLines
}

// LogViewTemplate executes the log viewer template ready for rendering
func LogViewTemplate(buildLogsView BuildLogsView) string {
	var buf bytes.Buffer
	if err := buildLogTemplate.Execute(&buf, buildLogsView); err != nil {
		logrus.WithError(err).Error("Error executing template.")
	}
	return buf.String()
}
