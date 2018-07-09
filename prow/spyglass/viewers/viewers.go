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

// Package viewers provides interfaces and methods necessary for implementing views
package viewers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/sirupsen/logrus"
)

var (
	viewHandlerRegistry = map[string]ViewHandler{}
	viewTitleRegistry   = map[string]string{}
	ErrUnsupportedOp    = errors.New("Unsupported Operation")
)

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

// ViewHandler consumes artifacts and some possible callback json data and returns and html view
type ViewHandler func([]Artifact, *json.RawMessage) string

// View gets the updated view from an artifact viewer with the provided name
func View(name string, artifacts []Artifact, raw *json.RawMessage) (string, error) {
	handler, ok := viewHandlerRegistry[name]
	if !ok {
		logrus.Error("Invalid view name ", name)
		return "", errors.New("Invalid view name provided.")
	}
	return handler(artifacts, raw), nil

}

// Title gets the title of the view with the given name
func Title(name string) (string, error) {
	title, ok := viewTitleRegistry[name]
	if !ok {
		logrus.Error("Invalid view name ", name)
		return "", errors.New("Invalid view name provided.")
	}
	return title, nil

}

// RegisterViewer registers new viewers
func RegisterViewer(viewerName string, title string, handler ViewHandler) error {
	_, ok := viewHandlerRegistry[viewerName]
	if ok {
		return errors.New(fmt.Sprintf("Viewer already registered with name %s.", viewerName))
	}
	viewHandlerRegistry[viewerName] = handler
	viewTitleRegistry[viewerName] = title
	logrus.Infof("Registered viewer %s with title %s and a handler function", viewerName, title)
	return nil
}

// LastNLines reads the last n lines from a file in GCS
func LastNLines(a Artifact, n int64) ([]string, error) {
	chunkSize := int64(256e3) // 256KB
	toRead := chunkSize + 1   // Add 1 for exclusive upper bound read range
	chunks := int64(1)
	var contents []byte
	var linesInContents int64
	artifactSize := a.Size()
	logrus.Info("Artifact size: ", artifactSize)
	offset := artifactSize - chunks*chunkSize
	for linesInContents < n && offset != 0 {
		offset = artifactSize - chunks*chunkSize
		if offset < 0 {
			toRead = offset + chunkSize + 1
			offset = 0
		}
		logrus.Info("toRead ", toRead)
		logrus.Info("offset ", offset)
		bytesRead := make([]byte, toRead)
		numRead, err := a.ReadAt(bytesRead, offset)
		if err != nil {
			if err != io.EOF {
				logrus.WithError(err).Error("Error reading artifact ", a.JobPath())
				return []string{}, err
			}
		}
		logrus.Info("Read bytes ", numRead)
		bytesRead = bytes.Trim(bytesRead, "\x00")
		linesInContents += int64(bytes.Count(bytesRead, []byte("\n")))
		logrus.Info("lines so far ", linesInContents)
		contents = append(bytesRead, contents...)
		chunks += 1
	}

	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(contents))
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		line := scanner.Text()
		logrus.Info("read line ", line)
		lines = append(lines, line)
	}

	l := int64(len(lines))
	if l < n {
		return lines, nil
	}
	return lines[l-n:], nil

}
