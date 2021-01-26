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

package spyglass

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/spyglass/lenses"
)

type jobAgent interface {
	GetProwJob(job string, id string) (prowapi.ProwJob, error)
	GetJobLog(job string, id string, container string) ([]byte, error)
}

// PodLogArtifact holds data for reading from a specific pod log
type PodLogArtifact struct {
	name         string
	buildID      string
	artifactName string
	container    string
	sizeLimit    int64
	jobAgent
}

var (
	errInsufficientJobInfo = errors.New("insufficient job information provided")
	errInvalidSizeLimit    = errors.New("sizeLimit must be a 64-bit integer greater than 0")
)

// NewPodLogArtifact creates a new PodLogArtifact
func NewPodLogArtifact(jobName string, buildID string, artifactName string, container string, sizeLimit int64, ja jobAgent) (*PodLogArtifact, error) {
	if jobName == "" {
		return nil, errInsufficientJobInfo
	}
	if buildID == "" {
		return nil, errInsufficientJobInfo
	}
	if artifactName == "" {
		return nil, errInsufficientJobInfo
	}
	if sizeLimit < 0 {
		return nil, errInvalidSizeLimit
	}
	return &PodLogArtifact{
		name:         jobName,
		buildID:      buildID,
		artifactName: artifactName,
		container:    container,
		sizeLimit:    sizeLimit,
		jobAgent:     ja,
	}, nil
}

// CanonicalLink returns a link to where pod logs are streamed
func (a *PodLogArtifact) CanonicalLink() string {
	q := url.Values{
		"job":       []string{a.name},
		"id":        []string{a.buildID},
		"container": []string{a.container},
	}
	u := url.URL{
		Path:     "/log",
		RawQuery: q.Encode(),
	}
	return u.String()
}

// JobPath gets the path within the job for the pod log. Always returns build-log.txt if we have only 1 test container
// in the ProwJob. Returns <containerName>-build-log.txt if we have multiple containers in the ProwJob.
// This is because the pod log becomes the build log after the job artifact uploads
// are complete, which should be used instead of the pod log.
func (a *PodLogArtifact) JobPath() string {
	return a.artifactName
}

// ReadAt implements reading a range of bytes from the pod logs endpoint
func (a *PodLogArtifact) ReadAt(p []byte, off int64) (n int, err error) {
	if int64(len(p)) > a.sizeLimit {
		return 0, lenses.ErrRequestSizeTooLarge
	}
	logs, err := a.jobAgent.GetJobLog(a.name, a.buildID, a.container)
	if err != nil {
		return 0, fmt.Errorf("error getting pod log: %v", err)
	}
	r := bytes.NewReader(logs)
	readBytes, err := r.ReadAt(p, off)
	if err == io.EOF {
		return readBytes, io.EOF
	}
	if err != nil {
		return 0, fmt.Errorf("error reading pod logs: %v", err)
	}
	return readBytes, nil
}

// ReadAll reads all available pod logs, failing if they are too large
func (a *PodLogArtifact) ReadAll() ([]byte, error) {
	size, err := a.Size()
	if err != nil {
		return nil, fmt.Errorf("error getting pod log size: %v", err)
	}
	if size > a.sizeLimit {
		return nil, lenses.ErrFileTooLarge
	}
	logs, err := a.jobAgent.GetJobLog(a.name, a.buildID, a.container)
	if err != nil {
		return nil, fmt.Errorf("error getting pod log: %v", err)
	}
	return logs, nil
}

// ReadAtMost reads at most n bytes
func (a *PodLogArtifact) ReadAtMost(n int64) ([]byte, error) {
	if n > a.sizeLimit {
		return nil, lenses.ErrRequestSizeTooLarge
	}
	logs, err := a.jobAgent.GetJobLog(a.name, a.buildID, a.container)
	if err != nil {
		return nil, fmt.Errorf("error getting pod log: %v", err)
	}
	reader := bytes.NewReader(logs)
	var byteCount int64
	var p []byte
	for byteCount < n {
		b, err := reader.ReadByte()
		if err == io.EOF {
			return p, io.EOF
		}
		if err != nil {
			return nil, fmt.Errorf("error reading pod log: %v", err)
		}
		p = append(p, b)
		byteCount++
	}
	return p, nil
}

// ReadTail reads the last n bytes of the pod log
func (a *PodLogArtifact) ReadTail(n int64) ([]byte, error) {
	if n > a.sizeLimit {
		return nil, lenses.ErrRequestSizeTooLarge
	}
	logs, err := a.jobAgent.GetJobLog(a.name, a.buildID, a.container)
	if err != nil {
		return nil, fmt.Errorf("error getting pod log tail: %v", err)
	}
	size := int64(len(logs))
	var off int64
	if n > size {
		off = 0
	} else {
		off = size - n
	}
	p := make([]byte, n)
	readBytes, err := bytes.NewReader(logs).ReadAt(p, off)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("error reading pod log tail: %v", err)
	}
	return p[:readBytes], nil
}

// Size gets the size of the pod log. Note: this function makes the same network call as reading the entire file.
func (a *PodLogArtifact) Size() (int64, error) {
	logs, err := a.jobAgent.GetJobLog(a.name, a.buildID, a.container)
	if err != nil {
		return 0, fmt.Errorf("error getting size of pod log: %v", err)
	}
	return int64(len(logs)), nil

}

// isProwJobSource returns true if the provided string is a valid Prowjob source and false otherwise
func isProwJobSource(src string) bool {
	return strings.HasPrefix(src, "prowjob/")
}
