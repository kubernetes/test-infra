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
	"regexp"
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

// errRE matches keywords and glog error messages
var errRE = regexp.MustCompile(`(?i)(\s|^)timed out\b|(\s|^)error(s)?\b|(\s|^)fail(ure|ed)?\b|(\s|^)fatal\b|(\s|^)panic\b|^E\d{4} \d\d:\d\d:\d\d\.\d\d\d]`)

func init() {
	lenses.RegisterLens(Lens{})
}

// SubLine is a part of a LogLine, used so that error terms can be highlighted.
type SubLine struct {
	Highlighted bool
	Text        string
}

// LogLine represents a line displayed in the LogArtifactView.
type LogLine struct {
	Number      int
	Length      int
	Highlighted bool
	Skip        bool
	SubLines    []SubLine
}

// LineGroup holds multiple lines that can be collapsed/expanded as a block
type LineGroup struct {
	Skip                   bool
	Start, End             int // closed, open
	ByteOffset, ByteLength int
	LogLines               []LogLine
}

type LineRequest struct {
	Artifact  string `json:"artifact"`
	Offset    int64  `json:"offset"`
	Length    int64  `json:"length"`
	StartLine int    `json:"startLine"`
}

// LinesSkipped returns the number of lines skipped in a line group.
func (g LineGroup) LinesSkipped() int {
	return g.End - g.Start
}

// LogArtifactView holds a single log file's view
type LogArtifactView struct {
	ArtifactName string
	ArtifactLink string
	LineGroups   []LineGroup
	ViewAll      bool
}

// BuildLogsView holds each log file view
type BuildLogsView struct {
	LogViews           []LogArtifactView
	RawGetAllRequests  map[string]string
	RawGetMoreRequests map[string]string
}

// Body returns the <body> content for a build log (or multiple build logs)
func (lens Lens) Body(artifacts []lenses.Artifact, resourceDir string, data string) string {
	buildLogsView := BuildLogsView{
		LogViews:           []LogArtifactView{},
		RawGetAllRequests:  make(map[string]string),
		RawGetMoreRequests: make(map[string]string),
	}

	// Read log artifacts and construct template structs
	for _, a := range artifacts {
		av := LogArtifactView{
			ArtifactName: a.JobPath(),
			ArtifactLink: a.CanonicalLink(),
		}
		lines, err := logLinesAll(a)
		if err != nil {
			logrus.WithError(err).Error("Error reading log.")
			continue
		}
		av.LineGroups = groupLines(highlightLines(lines, 0))
		av.ViewAll = true
		buildLogsView.LogViews = append(buildLogsView.LogViews, av)
	}

	return executeTemplate(resourceDir, "body", buildLogsView)
}

// Callback is used to retrieve new log segments
func (lens Lens) Callback(artifacts []lenses.Artifact, resourceDir string, data string) string {
	var request LineRequest
	err := json.Unmarshal([]byte(data), &request)
	if err != nil {
		return "failed to unmarshal request"
	}
	artifact, ok := artifactByName(artifacts, request.Artifact)
	if !ok {
		return "no artifact named " + request.Artifact
	}

	var lines []string
	if request.Offset == 0 && request.Length == -1 {
		lines, err = logLinesAll(artifact)
	} else {
		lines, err = logLines(artifact, request.Offset, request.Length)
	}
	if err != nil {
		return fmt.Sprintf("failed to retrieve log lines: %v", err)
	}

	logLines := highlightLines(lines, request.StartLine)
	return executeTemplate(resourceDir, "line group", logLines)
}

func artifactByName(artifacts []lenses.Artifact, name string) (lenses.Artifact, bool) {
	for _, a := range artifacts {
		if a.JobPath() == name {
			return a, true
		}
	}
	return nil, false
}

// logLinesAll reads all of an artifact and splits it into lines.
func logLinesAll(artifact lenses.Artifact) ([]string, error) {
	read, err := artifact.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read log %q: %v", artifact.JobPath(), err)
	}
	logLines := strings.Split(string(read), "\n")

	return logLines, nil
}

func logLines(artifact lenses.Artifact, offset, length int64) ([]string, error) {
	b := make([]byte, length)
	_, err := artifact.ReadAt(b, offset)
	if err != nil {
		if err != lenses.ErrGzipOffsetRead {
			return nil, fmt.Errorf("couldn't read requested bytes: %v", err)
		}
		moreBytes, err := artifact.ReadAtMost(offset + length)
		if err != nil {
			return nil, fmt.Errorf("couldn't handle reading gzipped file: %v", err)
		}
		b = moreBytes[offset:]
	}
	return strings.Split(string(b), "\n"), nil
}

func highlightLines(lines []string, startLine int) []LogLine {
	// mark highlighted lines
	logLines := make([]LogLine, 0, len(lines))
	for i, text := range lines {
		length := len(text)
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
			Length:      length + 1, // counting the "\n"
			SubLines:    subLines,
			Number:      startLine + i + 1,
			Highlighted: len(subLines) > 1,
			Skip:        true,
		})
	}
	return logLines
}

// breaks lines into important/unimportant groups
func groupLines(logLines []LogLine) []LineGroup {
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
	currentOffset := 0
	previousOffset := 0
	var lineGroups []LineGroup
	curGroup := LineGroup{}
	for i, line := range logLines {
		if line.Skip == curGroup.Skip {
			curGroup.LogLines = append(curGroup.LogLines, line)
			currentOffset += line.Length
		} else {
			curGroup.End = i
			curGroup.ByteLength = currentOffset - previousOffset - 1 // -1 for trailing newline
			previousOffset = currentOffset
			if curGroup.Skip {
				if curGroup.LinesSkipped() < minLinesSkipped {
					curGroup.Skip = false
				}
			}
			if len(curGroup.LogLines) > 0 {
				lineGroups = append(lineGroups, curGroup)
			}
			curGroup = LineGroup{
				Skip:       line.Skip,
				Start:      i,
				LogLines:   []LogLine{line},
				ByteOffset: currentOffset,
			}
			currentOffset += line.Length
		}
	}
	curGroup.End = len(logLines)
	curGroup.ByteLength = currentOffset - previousOffset - 1
	if curGroup.Skip {
		if curGroup.LinesSkipped() < minLinesSkipped {
			curGroup.Skip = false
		}
	}
	if len(curGroup.LogLines) > 0 {
		lineGroups = append(lineGroups, curGroup)
	}
	return lineGroups
}

// LogViewTemplate executes the log viewer template ready for rendering
func executeTemplate(resourceDir, templateName string, data interface{}) string {
	t := template.New("template.html")
	_, err := t.ParseFiles(filepath.Join(resourceDir, "template.html"))
	if err != nil {
		return fmt.Sprintf("Failed to load template: %v", err)
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, templateName, data); err != nil {
		logrus.WithError(err).Error("Error executing template.")
	}
	return buf.String()
}
