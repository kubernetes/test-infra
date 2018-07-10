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

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/deck/jobs"
)

// PodLogArtifact holds data for reading from a specific pod log
type PodLogArtifact struct {
	name      string
	jobId     string
	ja        *jobs.JobAgent
	sizeLimit int64
}

//NewPodLogArtifact creates a new PodLogArtifact
func NewPodLogArtifact(jobName string, jobId string, ja *jobs.JobAgent) *PodLogArtifact {
	return &PodLogArtifact{
		name:      jobName,
		jobId:     jobId,
		ja:        ja,
		sizeLimit: 500e6,
	}
}

// CanonicalLink returns a link to where pod logs are streamed
func (a *PodLogArtifact) CanonicalLink() string {
	return fmt.Sprintf("/log?job=%s&id=%s", a.name, a.jobId)
}

// JobPath gets the path within the job for the pod log. Note this is a special case, trying to match
// artifacts uploaded to other storage locations with the name "pod-log" will also match this.
func (a *PodLogArtifact) JobPath() string {
	return "pod-log"
}

func (a *PodLogArtifact) ReadAt(p []byte, off int64) (n int, err error) {
	logs, err := a.ja.GetJobLog(a.name, a.jobId)
	if err != nil {
		logrus.WithError(err).Error("Could not get pod logs.")
		return 0, err
	}
	return bytes.NewReader(logs).ReadAt(p, off)
}

// ReadAll reads all available pod logs, failing if they are too large
func (a *PodLogArtifact) ReadAll() ([]byte, error) {
	if a.Size() > a.sizeLimit {
		return []byte{}, errors.New("File too large.")
	}
	return a.ja.GetJobLog(a.name, a.jobId)
}

// ReadAtMost reads at most n bytes
func (a *PodLogArtifact) ReadAtMost(n int64) ([]byte, error) {
	joblog, err := a.ja.GetJobLog(a.name, a.jobId)
	if err != nil {
		return []byte{}, err
	}
	reader := bytes.NewReader(joblog)
	var byteCount int64
	var p []byte
	for byteCount <= n {
		b, err := reader.ReadByte()
		if err != nil {
			if err == io.EOF {
				return p, nil
			} else {
				return p, err
			}
		}
		p = append(p, b)
	}
	return p, nil
}

// ReadTail reads the last n bytes of the pod log
func (a *PodLogArtifact) ReadTail(n int64) ([]byte, error) {
	logs, err := a.ja.GetJobLog(a.name, a.jobId)
	if err != nil {
		logrus.WithError(err).Error("Could not get pod logs.")
		return []byte{}, err
	}
	off := int64(len(logs)) - n - 1
	p := []byte{}
	_, err = bytes.NewReader(logs).ReadAt(p, off)
	if err != nil {
		logrus.WithError(err).Error("Failed to read pod logs.")
	}
	return p, err

}

// Size gets the size of the pod log. Note: this function makes the same network call as reading the entire file.
func (a *PodLogArtifact) Size() int64 {
	logs, err := a.ja.GetJobLog(a.name, a.jobId)
	if err != nil {
		logrus.WithError(err).Error("Could not get pod logs.")
		return -1
	}
	return int64(len(logs))

}
