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
	"io"
	"net/url"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/deck/jobs"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

// PodLogArtifact holds data for reading from a specific pod log
type PodLogArtifact struct {
	name      string
	buildID   string
	podName   string
	sizeLimit int64
	ja        *jobs.JobAgent
}

// NewPodLogArtifact creates a new PodLogArtifact
func NewPodLogArtifact(jobName string, buildID string, podName string, sizeLimit int64, ja *jobs.JobAgent) *PodLogArtifact {
	if jobName == "" {
		logrus.Error("Must specify non-empty jobName")
	}
	if buildID == "" {
		logrus.Error("Must specify non-empty buildID")
	}
	return &PodLogArtifact{
		name:      jobName,
		buildID:   buildID,
		podName:   podName,
		sizeLimit: sizeLimit,
		ja:        ja,
	}
}

// CanonicalLink returns a link to where pod logs are streamed
func (a *PodLogArtifact) CanonicalLink() string {
	if a.name != "" && a.buildID != "" {
		q := url.Values{
			"job": []string{a.name},
			"id":  []string{a.buildID},
		}
		u := url.URL{
			Path:     "/log",
			RawQuery: q.Encode(),
		}
		return u.String()
	}
	logrus.Error("Error getting link to pod log, empty job name or build ID")
	return ""
}

// JobPath gets the path within the job for the pod log. Always returns build-log.txt.
// This is because the pod log becomes the build log after the job artifact uploads
// are complete, which should be used instead of the pod log.
func (a *PodLogArtifact) JobPath() string {
	return "build-log.txt"
}

// ReadAt implements reading a range of bytes from the pod logs endpoint
func (a *PodLogArtifact) ReadAt(p []byte, off int64) (n int, err error) {
	logs := a.fetchLogs()
	return bytes.NewReader(logs).ReadAt(p, off)
}

// ReadAll reads all available pod logs, failing if they are too large
func (a *PodLogArtifact) ReadAll() ([]byte, error) {
	if a.Size() > a.sizeLimit {
		return []byte{}, viewers.ErrFileTooLarge
	}
	return a.fetchLogs(), nil
}

// ReadAtMost reads at most n bytes
func (a *PodLogArtifact) ReadAtMost(n int64) ([]byte, error) {
	logs := a.fetchLogs()
	reader := bytes.NewReader(logs)
	var byteCount int64
	var p []byte
	for byteCount <= n {
		b, err := reader.ReadByte()
		if err != nil {
			if err == io.EOF {
				return p, nil
			}
			return p, err
		}
		p = append(p, b)
		byteCount++
	}
	return p, nil
}

// ReadTail reads the last n bytes of the pod log
func (a *PodLogArtifact) ReadTail(n int64) ([]byte, error) {
	logs, err := a.ja.GetJobLogTail(a.name, a.buildID, n)
	if err != nil {
		logrus.WithField("artifactName", a.name).WithError(err).Error("Error getting pod logs tail")
	}
	off := int64(len(logs)) - n - 1
	p := []byte{}
	_, err = bytes.NewReader(logs).ReadAt(p, off)
	if err != nil {
		logrus.WithError(err).WithField("artifactName", a.name).Error("Failed to read pod logs.")
	}
	return p, err
}

// Size gets the size of the pod log. Note: this function makes the same network call as reading the entire file.
func (a *PodLogArtifact) Size() int64 {
	logs := a.fetchLogs()
	return int64(len(logs))

}

// fetchLogs is a wrapper method for handling errors from fetching pod logs
func (a *PodLogArtifact) fetchLogs() []byte {
	var logs []byte
	var err error
	// logs, err = a.ja.GetJobLogByPodName(a.podName) TODO I'd like to support this eventually
	logs, err = a.ja.GetJobLog(a.name, a.buildID)
	if err != nil {
		logrus.WithField("artifactName", a.name).WithError(err).Error("Error getting pod logs")
	}
	return logs
}

// isProwJobSource returns true if the provided string is a valid Prowjob source and false otherwise
func isProwJobSource(src string) bool {
	return strings.HasPrefix(src, "prowjob/")
}
