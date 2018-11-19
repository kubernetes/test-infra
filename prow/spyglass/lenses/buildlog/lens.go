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
	"regexp"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"html/template"
	"k8s.io/test-infra/prow/spyglass/lenses"
	"path/filepath"
)

const (
	name            = "buildlog"
	title           = "Build Log"
	priority        = 10
	byteChunkSize   = 1e3 // 1KB
	lineChunkSize   = 100
	neighborLines   = 5 // number of "important" lines to be displayed in either direction
	minLinesSkipped = 5
)

// Lens implements the build lens.
type Lens struct{}

// Name returns the name.
func (lens Lens) Name() string {
	return name
}

// Title returns the title.
func (lens Lens) Title() string {
	return title
}

// Priority returns the priority.
func (lens Lens) Priority() int {
	return priority
}

// Header executes the "header" section of the template.
func (lens Lens) Header(artifacts []lenses.Artifact, resourceDir string) string {
	return executeTemplate(resourceDir, "header", BuildLogsView{})
}

// Callback does nothing.
func (lens Lens) Callback(artifacts []lenses.Artifact, resourceDir string, data string) string {
	return ""
}

// errRE matches keywords and glog error messages
var errRE = regexp.MustCompile(`(?i)(\s|^)timed out\b|(\s|^)error(s)?\b|(\s|^)fail(ure|ed)?\b|(\s|^)fatal\b|(\s|^)panic\b|^E\d{4} \d\d:\d\d:\d\d\.\d\d\d]`)

func init() {
	lenses.RegisterLens(Lens{})
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

// SubLine is a part of a LogLine, used so that error terms can be highlighted.
type SubLine struct {
	Highlighted bool
	Text        string
}

// LogLine represents a line displayed in the LogArtifactView.
type LogLine struct {
	Number      int
	Highlighted bool
	Skip        bool
	SubLines    []SubLine
}

// LineGroup holds multiple lines that can be collapsed/expanded as a block
type LineGroup struct {
	Skip       bool
	Start, End int // closed, open
	LogIndex   int
	LogLines   []LogLine
}

func linesSkipped(g LineGroup) int {
	return g.End - g.Start
}

func linesID(g LineGroup) string {
	return fmt.Sprintf("lines-%d-%d-%d", g.LogIndex, g.Start, g.End)
}

func skipID(g LineGroup) string {
	return fmt.Sprintf("skip-%d-%d-%d", g.LogIndex, g.Start, g.End)
}

// LogArtifactView holds a single log file's view
type LogArtifactView struct {
	ArtifactName          string
	ArtifactLink          string
	ViewName              string
	ViewMethodDescription string
	LineGroups            []LineGroup
	ViewAll               bool
	Index                 int
}

func logID(lav LogArtifactView) string {
	return fmt.Sprintf("log-%d", lav.Index)
}

// BuildLogsView holds each log file view
type BuildLogsView struct {
	LogViews           []LogArtifactView
	GetAllRequests     map[string]LogViewerRequest
	GetMoreRequests    map[string]LogViewerRequest
	RawGetAllRequests  map[string]string
	RawGetMoreRequests map[string]string
}

// Body returns the <body> content for a build log (or multiple build logs)
func (lens Lens) Body(artifacts []lenses.Artifact, resourceDir string, data string) string {
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
	if err := json.Unmarshal([]byte(data), &viewReq); err != nil {
		logrus.WithError(err).Error("Error unmarshalling log view request data.")
	}

	getAllReq := LogViewerRequest{
		Requests: make(map[string]LogViewData),
	}

	getMoreReq := LogViewerRequest{
		Requests: make(map[string]LogViewData),
	}

	// Read log artifacts and construct template structs
	for i, a := range artifacts {
		viewData, ok := viewReq.Requests[a.JobPath()]
		if !ok {
			viewData = LogViewData{
				UseBytes:  false,
				HeadBytes: 0,
				TailLines: 0,
				Operation: "more",
			}
		}

		logView, logViewData := nextViewData(i, a, viewData)

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

	return executeTemplate(resourceDir, "body", buildLogsView)
}

// nextViewData constructs the new log artifact view and needed requests from the current
// view data and artifact
func nextViewData(logIndex int, artifact lenses.Artifact, currentViewData LogViewData) (LogArtifactView, LogViewData) {
	newLogViewData := LogViewData{
		Operation: "",
	}
	newArtifactView := LogArtifactView{
		ArtifactName: artifact.JobPath(),
		ArtifactLink: artifact.CanonicalLink(),
		ViewName:     name,
		Index:        logIndex,
	}
	lines := logLinesAll(artifact)
	newArtifactView.LineGroups = groupLines(lines, logIndex)
	newArtifactView.ViewMethodDescription = "viewing error lines"
	newArtifactView.ViewAll = true

	return newArtifactView, newLogViewData

}

// logLinesHead reads the first n bytes of an artifact and splits it into lines.
func logLinesHead(artifact lenses.Artifact, n int64) ([]string, error) {
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
	return logLines, nil
}

// logLinesAll reads all of an artifact and splits it into lines.
func logLinesAll(artifact lenses.Artifact) []string {
	read, err := artifact.ReadAll()
	if err != nil {
		if err == lenses.ErrFileTooLarge {
			logrus.WithError(err).Error("Artifact too large to read all.")
		} else {
			logrus.WithError(err).Error("Failed to read log.")
		}
		return []string{}
	}
	logLines := strings.Split(string(read), "\n")

	return logLines
}

// breaks lines into important/unimportant groups
func groupLines(lines []string, logIndex int) []LineGroup {
	// mark highlighted lines
	logLines := make([]LogLine, 0, len(lines))
	for i, text := range lines {
		subLines := []SubLine{}
		loc := errRE.FindStringIndex(text)
		for loc != nil {
			subLines = append(subLines, SubLine{false, text[:loc[0]]})
			subLines = append(subLines, SubLine{true, text[loc[0]:loc[1]]})
			text = text[loc[1]:]
			loc = errRE.FindStringIndex(text)
		}
		subLines = append(subLines, SubLine{false, text})
		logLines = append(logLines, LogLine{
			SubLines:    subLines,
			Number:      i + 1,
			Highlighted: len(subLines) > 1,
			Skip:        true,
		})
	}
	// show highlighted lines and their neighboring lines
	for i, line := range logLines {
		if line.Highlighted {
			for d := -neighborLines; d <= neighborLines; d++ {
				if i+d < 0 {
					continue
				}
				if i+d >= len(logLines) {
					break
				}
				logLines[i+d].Skip = false
			}
		}
	}
	// break into groups
	var lineGroups []LineGroup
	curGroup := LineGroup{LogIndex: logIndex}
	for i, line := range logLines {
		if line.Skip == curGroup.Skip {
			curGroup.LogLines = append(curGroup.LogLines, line)
		} else {
			curGroup.End = i
			if curGroup.Skip {
				if linesSkipped(curGroup) < minLinesSkipped {
					curGroup.Skip = false
				}
			}
			if len(curGroup.LogLines) > 0 {
				lineGroups = append(lineGroups, curGroup)
			}
			curGroup = LineGroup{
				LogIndex: logIndex,
				Skip:     line.Skip,
				Start:    i,
				LogLines: []LogLine{line},
			}
		}
	}
	curGroup.End = len(logLines)
	if curGroup.Skip {
		if linesSkipped(curGroup) < minLinesSkipped {
			curGroup.Skip = false
		}
	}
	if len(curGroup.LogLines) > 0 {
		lineGroups = append(lineGroups, curGroup)
	}
	return lineGroups
}

// LogViewTemplate executes the log viewer template ready for rendering
func executeTemplate(resourceDir, templateName string, buildLogsView BuildLogsView) string {
	t := template.New("template.html")
	t.Funcs(template.FuncMap{
		"linesSkipped": linesSkipped,
		"linesID":      linesID,
		"skipID":       skipID,
		"logID":        logID})
	_, err := t.ParseFiles(filepath.Join(resourceDir, "template.html"))
	if err != nil {
		return fmt.Sprintf("Failed to load template: %v", err)
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, templateName, buildLogsView); err != nil {
		logrus.WithError(err).Error("Error executing template.")
	}
	return buf.String()
}
