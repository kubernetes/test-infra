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

// Package viewers provides interfaces and methods necessary for implementing custom artifact viewers
package viewers

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/sirupsen/logrus"
)

var (
	viewHandlerRegistry  = map[string]ViewHandler{}
	viewMetadataRegistry = map[string]ViewMetadata{}
	ErrUnsupportedOp     = errors.New("unsupported operation")
	ErrInvalidViewName   = errors.New("invalid view name")
	ErrFileTooLarge      = errors.New("file size over specified limit")
)

// ViewMetadata represents some metadata associated with rendering views
type ViewMetadata struct {
	// The title of the view
	Title string

	// Defines the order of views on the page. Lower priority values will be rendered higher up.
	// Views with identical priorities will be rendered in alphabetical order by title.
	// Valid: [0-INTMAX].
	Priority int
}

// Artifact represents some output of a prow job
type Artifact interface {
	io.ReaderAt
	// ReadAtMost reads at most n bytes from the beginning of the artifact
	ReadAtMost(n int64) ([]byte, error)
	// CanonicalLink gets a link to viewing this artifact in storage
	CanonicalLink() string
	// JobPath is the path to the artifact within the job (i.e. without the job prefix)
	JobPath() string
	// ReadAll reads all bytes from the artifact up to a limit specified by the artifact
	ReadAll() ([]byte, error)
	// ReadTail reads the last n bytes from the artifact (unsupported on some compressed files)
	ReadTail(n int64) ([]byte, error)
	// Size gets the size of the artifact in bytes
	Size() int64
}

// ViewHandler consumes artifacts and some possible callback json data and returns an html view.
// Use the javascript function refreshView(viewName, viewData) to allow your viewer to call back to itself
// (request more data, update the view, etc.). ViewData is a json blob that will be passed back to the
// handler function for your view as the string
type ViewHandler func([]Artifact, string) string

// View gets the updated view from an artifact viewer with the provided name
func View(name string, artifacts []Artifact, raw string) (string, error) {
	handler, ok := viewHandlerRegistry[name]
	if !ok {
		return "", ErrInvalidViewName
	}
	return handler(artifacts, raw), nil

}

// Title gets the title of the view with the given name
func Title(name string) (string, error) {
	m, ok := viewMetadataRegistry[name]
	if !ok {
		return "", ErrInvalidViewName
	}
	return m.Title, nil

}

// Priority gets the priority of the view with the given name
func Priority(name string) (int, error) {
	m, ok := viewMetadataRegistry[name]
	if !ok {
		return -1, ErrInvalidViewName
	}
	return m.Priority, nil

}

// RegisterViewer registers new viewers
func RegisterViewer(viewerName string, metadata ViewMetadata, handler ViewHandler) error {
	_, ok := viewHandlerRegistry[viewerName]
	if ok {
		return errors.New(fmt.Sprintf("Viewer already registered with name %s.", viewerName))
	}

	if metadata.Title == "" {
		return errors.New("empty title field in view metadata")
	}
	if metadata.Priority < 0 {
		return errors.New("priority less than 0")
	}
	viewHandlerRegistry[viewerName] = handler
	viewMetadataRegistry[viewerName] = metadata
	logrus.Infof("Spyglass registered viewer %s with title %s.", viewerName, metadata.Title)
	return nil
}

// UnregisterViewer unregisters viewers
func UnregisterViewer(viewerName string) error {
	_, ok := viewHandlerRegistry[viewerName]
	if !ok {
		return ErrInvalidViewName
	}
	delete(viewHandlerRegistry, viewerName)
	delete(viewMetadataRegistry, viewerName)
	logrus.Infof("Spyglass unregistered viewer %s.", viewerName)
	return nil
}

// LastNLines reads the last n lines from an artifact.
func LastNLines(a Artifact, n int64) ([]string, error) {
	return LastNLinesChunked(a, n, 128e3)
}

// LastNLinesChunked reads the last n lines from an artifact by reading chunks of size chunkSize
// from the end of the artifact.
func LastNLinesChunked(a Artifact, n, chunkSize int64) ([]string, error) {
	toRead := chunkSize + 1 // Add 1 for exclusive upper bound read range
	chunks := int64(1)
	var contents []byte
	var linesInContents int64
	artifactSize := a.Size()
	offset := artifactSize - chunks*chunkSize
	lastOffset := offset
	var lastRead int64
	for linesInContents <= n && offset != 0 {
		offset = lastOffset - lastRead
		if offset < 0 {
			toRead = offset + chunkSize + 1
			offset = 0
		}
		bytesRead := make([]byte, toRead)
		numBytesRead, err := a.ReadAt(bytesRead, offset)
		lastRead = int64(numBytesRead)
		lastOffset = offset
		if err != nil {
			if err != io.EOF {
				logrus.WithError(err).Error("Error reading artifact ", a.JobPath())
				return []string{}, err
			}
		}
		bytesRead = bytes.Trim(bytesRead, "\x00")
		linesInContents += int64(bytes.Count(bytesRead, []byte("\n")))
		contents = append(bytesRead, contents...)
		chunks += 1
	}

	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(contents))
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
	}
	l := int64(len(lines))
	if l < n {
		return lines, nil
	}
	return lines[l-n:], nil
}
