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
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	prowconfig "k8s.io/test-infra/prow/config"
	pkgio "k8s.io/test-infra/prow/io"
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
	HighlightRegexes []string         `json:"highlight_regexes"`
	HideRawLog       bool             `json:"hide_raw_log,omitempty"`
	Highlighter      *highlightConfig `json:"highlighter,omitempty"`
}

type highlightConfig struct {
	// Endpoint specifies the URL to send highlight requests
	Endpoint string `json:"endpoint"`
	// Pin should automatically save the highlight when set.
	Pin bool `json:"pin"`
	// Overwrite should replace any existing highlight when set.
	Overwrite bool `json:"overwrite"`
	// Auto should request highlights before loading the page, implies Pin.
	Auto bool `json:"auto"`
}

type parsedConfig struct {
	highlightRegex *regexp.Regexp
	showRawLog     bool
	highlighter    *highlightConfig
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
func (lens Lens) Header(artifacts []api.Artifact, resourceDir string, config json.RawMessage, spyglassConfig prowconfig.Spyglass) string {
	return executeTemplate(resourceDir, "header", buildLogsView{})
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
	ArtifactName *string
	Number       int
	Length       int
	Highlighted  bool
	Skip         bool
	SubLines     []SubLine
	Focused      bool
	Clip         bool
}

// LineGroup holds multiple lines that can be collapsed/expanded as a block
type LineGroup struct {
	Skip                   bool
	Start, End             int // closed, open
	ByteOffset, ByteLength int64
	LogLines               []LogLine
	ArtifactName           *string
}

const moreLines = 20

func (g LineGroup) Expand() bool {
	return len(g.LogLines) >= moreLines
}

// callbackRequest represents a request for output lines from an artifact. If Offset is 0 and Length
// is -1, all lines will be fetched.
type callbackRequest struct {
	Artifact  string `json:"artifact"`
	Offset    int64  `json:"offset"`
	Length    int64  `json:"length"`
	StartLine int    `json:"startLine"`
	Top       int    `json:"top"`
	Bottom    int    `json:"bottom"`
	SaveEnd   *int   `json:"saveEnd"`
	Analyze   bool   `json:"analyze"`
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
	ShowRawLog   bool
	CanSave      bool
	CanAnalyze   bool
}

// buildLogsView holds each log file view
type buildLogsView struct {
	LogViews []LogArtifactView
}

func getConfig(rawConfig json.RawMessage) parsedConfig {
	conf := parsedConfig{
		highlightRegex: defaultErrRE,
		showRawLog:     true,
	}

	// No config at all is fine.
	if len(rawConfig) == 0 {
		return conf
	}

	var c config
	if err := json.Unmarshal(rawConfig, &c); err != nil {
		logrus.WithError(err).Error("Failed to decode buildlog config")
		return conf
	}
	conf.highlighter = c.Highlighter
	if conf.highlighter != nil && conf.highlighter.Endpoint == "" {
		conf.highlighter = nil
	}
	conf.showRawLog = !c.HideRawLog
	if len(c.HighlightRegexes) == 0 {
		return conf
	}

	re, err := regexp.Compile(strings.Join(c.HighlightRegexes, "|"))
	if err != nil {
		logrus.WithError(err).Warnf("Couldn't compile %q", c.HighlightRegexes)
		return conf
	}
	conf.highlightRegex = re
	return conf
}

// Body returns the <body> content for a build log (or multiple build logs)
func (lens Lens) Body(artifacts []api.Artifact, resourceDir string, data string, rawConfig json.RawMessage, spyglassConfig prowconfig.Spyglass) string {
	buildLogsView := buildLogsView{
		LogViews: []LogArtifactView{},
	}

	conf := getConfig(rawConfig)
	// Read log artifacts and construct template structs
	for _, a := range artifacts {
		av := LogArtifactView{
			ArtifactName: a.JobPath(),
			ArtifactLink: a.CanonicalLink(),
			ShowRawLog:   conf.showRawLog,
		}
		lines, err := logLinesAll(a)
		if err != nil {
			logrus.WithError(err).Info("Error reading log.")
			continue
		}
		artifact := av.ArtifactName
		meta, _ := a.Metadata()
		start, end := -1, -1

		for key, val := range meta {
			var targ *int
			if key == focusStart {
				targ = &start
			} else if key == focusEnd {
				targ = &end
			} else {
				continue
			}

			n, err := strconv.Atoi(val)
			if err != nil {
				continue
			}
			*targ = n
		}
		analyze := conf.highlighter != nil
		if start == -1 && analyze && conf.highlighter.Auto {
			resp, err := analyzeArtifact(a, &conf)
			if err != nil {
				logrus.WithError(err).Info("Failed to analyze artifact")
			} else {
				start, end = resp.Min, resp.Max
			}
		}
		av.LineGroups = groupLines(&artifact, start, end, highlightLines(lines, 0, &artifact, conf.highlightRegex)...)
		av.ViewAll = true
		av.CanSave = canSave(a.CanonicalLink())
		av.CanAnalyze = analyze
		buildLogsView.LogViews = append(buildLogsView.LogViews, av)
	}

	return executeTemplate(resourceDir, "body", buildLogsView)
}

func canSave(link string) bool {
	return strings.Contains(link, pkgio.GSAnonHost) || strings.Contains(link, pkgio.GSCookieHost)
}

const failedUnmarshal = "Failed to unmarshal request"
const missingArtifact = "No artifact named %s"
const focusStart = "focus-start"
const focusEnd = "focus-end"

// Callback is used to retrieve new log segments
func (lens Lens) Callback(artifacts []api.Artifact, resourceDir string, data string, rawConfig json.RawMessage, spyglassConfig prowconfig.Spyglass) string {
	var request callbackRequest
	err := json.Unmarshal([]byte(data), &request)
	if err != nil {
		return failedUnmarshal
	}
	artifact, ok := artifactByName(artifacts, request.Artifact)
	if !ok {
		return fmt.Sprintf(missingArtifact, request.Artifact)
	}
	if request.Analyze {
		conf := getConfig(rawConfig)
		hr, err := analyzeArtifact(artifact, &conf)
		if err != nil {
			hr = &highlightResponse{Error: err.Error()}
		}
		buf, err := json.Marshal(hr)
		if err != nil {
			return err.Error()
		}
		return string(buf)
	}
	if request.SaveEnd != nil {
		return storeHighlightedLines(&request, artifact)
	}
	return loadLines(&request, artifact, resourceDir, rawConfig)
}

type highlightRequest struct {
	// URL to highlight
	URL string `json:"url"`
	// Pin if the highlight should be saved
	Pin bool `json:"pin"`
	// Overwrite if an existing highlight should be replaced
	Overwrite bool `json:"overwrite"`
}

type highlightResponse struct {
	// Min line number to highlight
	Min int `json:"min"`
	// Max line number to highlight (inclusive).
	Max int `json:"max"`
	// Link to the highlighted lines
	Link string `json:"link,omitempty"`
	// Pinned if the highlight changed
	Pinned bool `json:"pinned,omitempty"`
	// Error describing the problem.
	Error string `json:"error,omitempty"`
}

var (
	errNoHighlighter = errors.New("buildlog.highlighter unconfigured")
)

func analyzeArtifact(artifact api.Artifact, conf *parsedConfig) (*highlightResponse, error) {
	if conf.highlighter == nil {
		return nil, errNoHighlighter
	}
	link := artifact.CanonicalLink()
	if !canSave(link) {
		return nil, fmt.Errorf("Unsupported artifact: %q", link)
	}
	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("parse artifact link %q: %v", link, err)
	}
	log := logrus.WithFields(logrus.Fields{
		"artifact": link,
	})

	req := highlightRequest{
		URL:       u.String(),
		Pin:       conf.highlighter.Pin || conf.highlighter.Auto,
		Overwrite: conf.highlighter.Overwrite,
	}

	buf, err := json.Marshal(req)
	if err != nil {
		log.WithError(err).Error("Failed to marshal highlight request")
		return nil, fmt.Errorf("bad request for %s", link)
	}

	resp, err := http.Post(conf.highlighter.Endpoint, "text/plain", bytes.NewBuffer(buf))
	if err != nil {
		log.WithError(err).WithField("link", link).Error("POST to highlighter failed")
		return nil, fmt.Errorf("POST %s failed", link)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.WithField("status", resp.StatusCode).Error("Response failed")
		return nil, fmt.Errorf("%s returned status code %d", link, resp.StatusCode)
	}

	dec := json.NewDecoder(resp.Body)
	var hr highlightResponse
	if err := dec.Decode(&hr); err != nil {
		log.WithError(err).Error("Failed to decode response")
		return nil, fmt.Errorf("bad response for %s", link)
	}
	return &hr, nil
}

func focusLines(artifact api.Artifact, start, end int) error {
	return artifact.UpdateMetadata(map[string]string{
		focusStart: strconv.Itoa(start),
		focusEnd:   strconv.Itoa(end),
	})
}

func storeHighlightedLines(request *callbackRequest, artifact api.Artifact) string {
	err := focusLines(artifact, request.StartLine, *request.SaveEnd)
	if err != nil {
		return err.Error()
	}
	logrus.WithFields(logrus.Fields{
		"artifact": artifact.CanonicalLink(),
		"start":    request.StartLine,
		"end":      request.SaveEnd,
	}).Info("Saved selected lines")
	return ""
}

func loadLines(request *callbackRequest, artifact api.Artifact, resourceDir string, rawConfig json.RawMessage) string {

	var err error
	var lines []string
	if request.Offset == 0 && request.Length == -1 {
		lines, err = logLinesAll(artifact)
	} else {
		lines, err = logLines(artifact, request.Offset, request.Length)
	}
	if err != nil {
		return fmt.Sprintf("Failed to retrieve log lines: %v", err)
	}

	var skipFirst bool
	var skipLines []string
	skipRequest := *request
	// Should we expand all the lines? Or just some from the top/bottom.
	if t, n := request.Top, len(lines); t > 0 && t < n {
		skipLines = lines[request.Top:]
		lines = lines[:request.Top]
		skipRequest.StartLine += t
		for _, line := range lines {
			b := int64(len(line) + 1)
			skipRequest.Offset += b
			skipRequest.Length -= b
		}
	} else if b := request.Bottom; b > 0 && b < n {
		skipLines = lines[:n-b]
		lines = lines[n-b:]
		request.StartLine += (n - b)
		for _, line := range lines {
			skipRequest.Length -= int64(len(line) + 1)
		}
		skipFirst = true
	}
	var skipGroup *LineGroup
	conf := getConfig(rawConfig)
	if len(skipLines) > 0 {
		logLines := highlightLines(skipLines, skipRequest.StartLine, &request.Artifact, conf.highlightRegex)
		skipGroup = &LineGroup{
			Skip:         true,
			Start:        skipRequest.StartLine,
			End:          skipRequest.StartLine + len(logLines),
			ByteOffset:   skipRequest.Offset,
			ByteLength:   skipRequest.Length,
			ArtifactName: &request.Artifact,
			LogLines:     logLines,
		}
	}
	groups := make([]*LineGroup, 0, 2)

	if skipGroup != nil && skipFirst {
		groups = append(groups, skipGroup)
		skipGroup = nil
	}
	logLines := highlightLines(lines, request.StartLine, &request.Artifact, conf.highlightRegex)
	groups = append(groups, &LineGroup{
		LogLines:     logLines,
		ArtifactName: &request.Artifact,
	})
	if skipGroup != nil {
		groups = append(groups, skipGroup)
	}
	return executeTemplate(resourceDir, "line groups", groups)
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
		return nil, fmt.Errorf("failed to read log %q: %w", artifact.JobPath(), err)
	}
	logLines := strings.Split(string(read), "\n")

	return logLines, nil
}

func logLines(artifact api.Artifact, offset, length int64) ([]string, error) {
	b := make([]byte, length)
	_, err := artifact.ReadAt(b, offset)
	if err != nil && err != io.EOF {
		if err != lenses.ErrGzipOffsetRead {
			return nil, fmt.Errorf("couldn't read requested bytes: %w", err)
		}
		moreBytes, err := artifact.ReadAtMost(offset + length)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("couldn't handle reading gzipped file: %w", err)
		}
		b = moreBytes[offset:]
	}
	return strings.Split(string(b), "\n"), nil
}

func highlightLines(lines []string, startLine int, artifact *string, highlightRegex *regexp.Regexp) []LogLine {
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
func groupLines(artifact *string, start, end int, logLines ...LogLine) []LineGroup {
	// show highlighted lines and their neighboring lines
	for i, line := range logLines {
		if start > 0 && end > 0 {
			switch {
			case line.Number >= start && line.Number <= end:
				logLines[i].Skip = false
				logLines[i].Focused = true
				if line.Number == start {
					logLines[i].Clip = true
				}
			case line.Number+neighborLines >= start && line.Number-neighborLines <= end:
				logLines[i].Skip = false
			}
			continue
		}
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
	var currentOffset int64
	var previousOffset int64
	var lineGroups []LineGroup
	var curGroup LineGroup
	for i, line := range logLines {
		if line.Skip == curGroup.Skip {
			curGroup.LogLines = append(curGroup.LogLines, line)
			currentOffset += int64(line.Length)
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
				Skip:         line.Skip,
				Start:        i,
				LogLines:     []LogLine{line},
				ByteOffset:   currentOffset,
				ArtifactName: artifact,
			}
			currentOffset += int64(line.Length)
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
