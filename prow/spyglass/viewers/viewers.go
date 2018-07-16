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
	ReadAtMost(n int64) ([]byte, error)
	CanonicalLink() string
	JobPath() string
	ReadAll() ([]byte, error)
	ReadTail(n int64) ([]byte, error)
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
		return errors.New("Must provide at least a Title in ViewMetadata.")
	}
	if metadata.Priority < 0 {
		return errors.New("Priority must be in range [0-INTMAX]")
	}
	viewHandlerRegistry[viewerName] = handler
	viewMetadataRegistry[viewerName] = metadata
	logrus.Infof("Registered viewer %s with title %s.", viewerName, metadata.Title)
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
	logrus.Infof("Unregistered viewer %s.", viewerName)
	return nil
}

// LastNLines reads the last n lines from an artifact.
func LastNLines(a Artifact, n int64) ([]string, error) {
	chunkSize := int64(256e3) // 256KB
	toRead := chunkSize + 1   // Add 1 for exclusive upper bound read range
	chunks := int64(1)
	var contents []byte
	var linesInContents int64
	artifactSize := a.Size()
	offset := artifactSize - chunks*chunkSize
	for linesInContents < n && offset != 0 {
		offset = artifactSize - chunks*chunkSize
		if offset < 0 {
			toRead = offset + chunkSize + 1
			offset = 0
		}
		bytesRead := make([]byte, toRead)
		_, err := a.ReadAt(bytesRead, offset)
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
