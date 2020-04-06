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
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/spyglass/api"
	"k8s.io/test-infra/prow/spyglass/lenses"
)

const (
	name               = "buildlog"
	title              = "Build Log"
	priority           = 10
	neighborLines      = 5 // number of "important" lines to be displayed in either direction
	minLinesSkipped    = 5
	maxHighlightLength = 10000 // Maximum length of a line worth highlighting
)

type config struct {
	HighlightRegexes []string `json:"highlight_regexes"`
}

var _ api.Lens = Lens{}

// Lens implements the build lens.
type Lens struct{}

// Config returns the lens's configuration.
func (lens Lens) Config() lenses.LensConfig {
	return lenses.LensConfig{
		Name:     name,
		Title:    title,
		Priority: priority,
	}
}

// Header executes the "header" section of the template.
func (lens Lens) Header(artifacts []api.Artifact, resourceDir string, config json.RawMessage) string {
	return executeTemplate(resourceDir, "header", BuildLogsView{})
}

// defaultErrRE matches keywords and glog error messages.
// It is only used if higlight_regexes is not specified in the lens config.
var defaultErrRE = regexp.MustCompile(`timed out|ERROR:|(FAIL|Failure \[)\b|panic\b|^E\d{4} \d\d:\d\d:\d\d\.\d\d\d]`)

func init() {
	lenses.RegisterLens(Lens{})
}

// SubLine represents an substring within a LogLine. It it used so error terms can be highlighted.
type SubLine struct {
	Highlighted bool
	Text        string
}

// LogLine represents a line displayed in the LogArtifactView.
type LogLine struct {
	ArtifactName string
	Number       int
	Length       int
	Highlighted  bool
	Skip         bool
	SubLines     []SubLine
}

// LineGroup holds multiple lines that can be collapsed/expanded as a block
type LineGroup struct {
	Skip                   bool
	Start, End             int // closed, open
	ByteOffset, ByteLength int
	LogLines               []LogLine
}

// LineRequest represents a request for output lines from an artifact. If Offset is 0 and Length
// is -1, all lines will be fetched.
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

func getHighlightRegex(rawConfig json.RawMessage) *regexp.Regexp {
	// No config at all is fine.
	if len(rawConfig) == 0 {
		return defaultErrRE
	}

	var c config
	if err := json.Unmarshal(rawConfig, &c); err != nil {
		logrus.WithError(err).Error("Failed to decode buildlog config")
		return defaultErrRE
	}
	if len(c.HighlightRegexes) == 0 {
		return defaultErrRE
	}

	re, err := regexp.Compile(strings.Join(c.HighlightRegexes, "|"))
	if err != nil {
		logrus.WithError(err).Warnf("Couldn't compile %q", c.HighlightRegexes)
		return defaultErrRE
	}
	return re
}

// Body returns the <body> content for a build log (or multiple build logs)
func (lens Lens) Body(artifacts []api.Artifact, resourceDir string, data string, rawConfig json.RawMessage) string {
	buildLogsView := BuildLogsView{
		LogViews:           []LogArtifactView{},
		RawGetAllRequests:  make(map[string]string),
		RawGetMoreRequests: make(map[string]string),
	}

	highlightRe := getHighlightRegex(rawConfig)
	// Read log artifacts and construct template structs
	for _, a := range artifacts {
		av := LogArtifactView{
			ArtifactName: a.JobPath(),
			ArtifactLink: a.CanonicalLink(),
		}
		lines, err := logLinesAll(a)
		if err != nil {
			logrus.WithError(err).Info("Error reading log.")
			continue
		}
		av.LineGroups = groupLines(highlightLines(lines, 0, av.ArtifactName, highlightRe))
		av.ViewAll = true
		buildLogsView.LogViews = append(buildLogsView.LogViews, av)
	}

	return executeTemplate(resourceDir, "body", buildLogsView)
}

// Callback is used to retrieve new log segments
func (lens Lens) Callback(artifacts []api.Artifact, resourceDir string, data string, rawConfig json.RawMessage) string {
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

	logLines := highlightLines(lines, request.StartLine, request.Artifact, getHighlightRegex(rawConfig))
	return executeTemplate(resourceDir, "line group", logLines)
}

func artifactByName(artifacts []api.Artifact, name string) (api.Artifact, bool) {
	for _, a := range artifacts {
		if a.JobPath() == name {
			return a, true
		}
	}
	return nil, false
}

// logLinesAll reads all of an artifact and splits it into lines.
func logLinesAll(artifact api.Artifact) ([]string, error) {
	read, err := artifact.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read log %q: %v", artifact.JobPath(), err)
	}
	logLines := strings.Split(string(read), "\n")

	return logLines, nil
}

func logLines(artifact api.Artifact, offset, length int64) ([]string, error) {
	b := make([]byte, length)
	_, err := artifact.ReadAt(b, offset)
	if err != nil && err != io.EOF {
		if err != lenses.ErrGzipOffsetRead {
			return nil, fmt.Errorf("couldn't read requested bytes: %v", err)
		}
		moreBytes, err := artifact.ReadAtMost(offset + length)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("couldn't handle reading gzipped file: %v", err)
		}
		b = moreBytes[offset:]
	}
	return strings.Split(string(b), "\n"), nil
}

func highlightLines(lines []string, startLine int, artifact string, highlightRegex *regexp.Regexp) []LogLine {
	// mark highlighted lines
	logLines := make([]LogLine, 0, len(lines))
	for i, text := range lines {
		length := len(text)
		subLines := []SubLine{}
		if length <= maxHighlightLength {
			loc := highlightRegex.FindStringIndex(text)
			for loc != nil {
				subLines = append(subLines, SubLine{false, text[:loc[0]]})
				subLines = append(subLines, SubLine{true, text[loc[0]:loc[1]]})
				text = text[loc[1]:]
				loc = highlightRegex.FindStringIndex(text)
			}
		}
		subLines = append(subLines, SubLine{false, text})
		logLines = append(logLines, LogLine{
			Length:       length + 1, // counting the "\n"
			SubLines:     subLines,
			Number:       startLine + i + 1,
			Highlighted:  len(subLines) > 1,
			ArtifactName: artifact,
			Skip:         true,
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
