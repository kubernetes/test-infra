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

// Package lenses provides interfaces and methods necessary for implementing custom artifact viewers
package lenses

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/spyglass/api"
)

var (
	lensReg = map[string]Lens{}

	// ErrGzipOffsetRead will be thrown when an offset read is attempted on a gzip-compressed object
	ErrGzipOffsetRead = errors.New("offset read on gzipped files unsupported")
	// ErrInvalidLensName will be thrown when a viewer method is called on a view name that has not
	// been registered. Ensure your viewer is registered using RegisterViewer and that you are
	// providing the correct viewer name.
	ErrInvalidLensName = errors.New("invalid lens name")
	// ErrFileTooLarge will be thrown when a size-limited operation (ex. ReadAll) is called on an
	// artifact whose size exceeds the configured limit.
	ErrFileTooLarge = errors.New("file size over specified limit")
	// ErrRequestSizeTooLarge will be thrown when any operation is called whose size exceeds the configured limit.
	ErrRequestSizeTooLarge = errors.New("request size over specified limit")
	// ErrContextUnsupported is thrown when attempting to use a context with an artifact that
	// does not support context operations (cancel, withtimeout, etc.)
	ErrContextUnsupported = errors.New("artifact does not support context operations")
)

type LensConfig struct {
	// Name is the name of the lens. It must match the package name.
	Name string
	// Title is a human-readable title for the lens.
	Title string
	// Priority is used to determine where to position the lens. Higher is better.
	Priority uint
	// HideTitle will hide the lens title after loading if set to true.
	HideTitle bool
}

// Lens defines the interface that lenses are required to implement in order to be used by Spyglass.
type Lens interface {
	// Config returns a LensConfig that describes the lens.
	Config() LensConfig
	// Header returns a a string that is injected into the rendered lens's <head>
	Header(artifacts []api.Artifact, resourceDir string, config json.RawMessage) string
	// Body returns a string that is initially injected into the rendered lens's <body>.
	// The lens's front-end code may call back to Body again, passing in some data string of its choosing.
	Body(artifacts []api.Artifact, resourceDir string, data string, config json.RawMessage) string
	// Callback receives a string sent by the lens's front-end code and returns another string to be returned
	// to that frontend code.
	Callback(artifacts []api.Artifact, resourceDir string, data string, config json.RawMessage) string
}

// ResourceDirForLens returns the path to a lens's public resource directory.
func ResourceDirForLens(baseDir, name string) string {
	return filepath.Join(baseDir, name)
}

// RegisterLens registers new viewers
func RegisterLens(lens Lens) error {
	config := lens.Config()
	_, ok := lensReg[config.Name]
	if ok {
		return fmt.Errorf("viewer already registered with name %s", config.Name)
	}

	if config.Title == "" {
		return errors.New("empty title field in view metadata")
	}
	lensReg[config.Name] = lens
	logrus.Infof("Spyglass registered viewer %s with title %s.", config.Name, config.Title)
	return nil
}

// GetLens returns a Lens or a remoteLens  by name, if it exists; otherwise it returns an error.
func GetLens(name string) (Lens, error) {
	lens, ok := lensReg[name]
	if !ok {
		return nil, ErrInvalidLensName
	}
	return lens, nil
}

// UnregisterLens unregisters lenses
func UnregisterLens(viewerName string) {
	delete(lensReg, viewerName)
	logrus.Infof("Spyglass unregistered viewer %s.", viewerName)
}

// LastNLines reads the last n lines from an artifact.
func LastNLines(a api.Artifact, n int64) ([]string, error) {
	// 300B, a reasonable log line length, probably a bit more scalable than a hard-coded value
	return LastNLinesChunked(a, n, 300*n+1)
}

// LastNLinesChunked reads the last n lines from an artifact by reading chunks of size chunkSize
// from the end of the artifact. Best performance is achieved by:
// argmin 0<chunkSize<INTMAX, f(chunkSize) = chunkSize - n * avgLineLength
func LastNLinesChunked(a api.Artifact, n, chunkSize int64) ([]string, error) {
	toRead := chunkSize + 1 // Add 1 for exclusive upper bound read range
	chunks := int64(1)
	var contents []byte
	var linesInContents int64
	artifactSize, err := a.Size()
	if err != nil {
		return nil, fmt.Errorf("error getting artifact size: %v", err)
	}
	offset := artifactSize - chunks*chunkSize
	lastOffset := offset
	var lastRead int64
	for linesInContents < n && offset != 0 {
		offset = lastOffset - lastRead
		if offset < 0 {
			toRead = offset + chunkSize + 1
			offset = 0
		}
		bytesRead := make([]byte, toRead)
		numBytesRead, err := a.ReadAt(bytesRead, offset)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("error reading artifact: %v", err)
		}
		lastRead = int64(numBytesRead)
		lastOffset = offset
		bytesRead = bytes.Trim(bytesRead, "\x00")
		linesInContents += int64(bytes.Count(bytesRead, []byte("\n")))
		contents = append(bytesRead, contents...)
		chunks++
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
