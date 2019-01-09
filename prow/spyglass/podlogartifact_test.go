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
	"fmt"
	"io"
	"testing"

	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/spyglass/lenses"
)

// fakePodLogJAgent used for pod log artifact dependency injection
type fakePodLogJAgent struct {
}

func (j *fakePodLogJAgent) GetProwJob(job, id string) (kube.ProwJob, error) {
	return kube.ProwJob{}, nil
}

func (j *fakePodLogJAgent) GetJobLog(job, id string) ([]byte, error) {
	if job == "BFG" && id == "435" {
		return []byte("frobscottle"), nil
	} else if job == "Fantastic Mr. Fox" && id == "4" {
		return []byte("a hundred smoked hams and fifty sides of bacon"), nil
	}
	return nil, fmt.Errorf("could not find job %s, id %s", job, id)
}

func (j *fakePodLogJAgent) GetJobLogTail(job, id string, n int64) ([]byte, error) {
	log, err := j.GetJobLog(job, id)
	if err != nil {
		return nil, fmt.Errorf("error getting log tail: %v", err)
	}
	logLen := int64(len(log))
	if n > logLen {
		return log, nil
	}
	return log[logLen-n:], nil

}

func TestNewPodLogArtifact(t *testing.T) {
	testCases := []struct {
		name         string
		jobName      string
		buildID      string
		sizeLimit    int64
		expectedErr  error
		expectedLink string
	}{
		{
			name:         "Create pod log with valid fields",
			jobName:      "job",
			buildID:      "123",
			sizeLimit:    500e6,
			expectedErr:  nil,
			expectedLink: "/log?id=123&job=job",
		},
		{
			name:         "Create pod log with no jobName",
			jobName:      "",
			buildID:      "123",
			sizeLimit:    500e6,
			expectedErr:  errInsufficientJobInfo,
			expectedLink: "",
		},
		{
			name:         "Create pod log with no buildID",
			jobName:      "job",
			buildID:      "",
			sizeLimit:    500e6,
			expectedErr:  errInsufficientJobInfo,
			expectedLink: "",
		},
		{
			name:         "Create pod log with negative sizeLimit",
			jobName:      "job",
			buildID:      "123",
			sizeLimit:    -4,
			expectedErr:  errInvalidSizeLimit,
			expectedLink: "",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			artifact, err := NewPodLogArtifact(tc.jobName, tc.buildID, tc.sizeLimit, &fakePodLogJAgent{})
			if err != nil {
				if err != tc.expectedErr {
					t.Fatalf("failed creating artifact. err: %v", err)
				}
				return
			}
			link := artifact.CanonicalLink()
			if link != tc.expectedLink {
				t.Errorf("Unexpected link, expected %s, got %q", tc.expectedLink, link)
			}
		})
	}
}

func TestReadTail_PodLog(t *testing.T) {
	testCases := []struct {
		name      string
		jobName   string
		buildID   string
		artifact  *PodLogArtifact
		n         int64
		expected  []byte
		expectErr bool
	}{
		{
			name:     "Podlog ReadTail longer than contents",
			jobName:  "BFG",
			buildID:  "435",
			n:        50,
			expected: []byte("frobscottle"),
		},
		{
			name:     "Podlog ReadTail shorter than contents",
			jobName:  "Fantastic Mr. Fox",
			buildID:  "4",
			n:        3,
			expected: []byte("con"),
		},
		{
			name:      "Podlog ReadTail nonexistent pod",
			jobName:   "Fax",
			buildID:   "4",
			n:         3,
			expectErr: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			artifact, err := NewPodLogArtifact(tc.jobName, tc.buildID, 500e6, &fakePodLogJAgent{})
			if err != nil {
				t.Fatalf("Pod Log Tests failed to create pod log artifact, err %v", err)
			}
			res, err := artifact.ReadTail(tc.n)
			if err != nil && !tc.expectErr {
				t.Fatalf("failed reading bytes of log. did not expect err, got err: %v", err)
			}
			if err == nil && tc.expectErr {
				t.Errorf("expected an error, got none")
			}
			if !bytes.Equal(tc.expected, res) {
				t.Errorf("Unexpected result of reading pod logs, expected %q, got %q", tc.expected, res)
			}
		})
	}

}
func TestReadAt_PodLog(t *testing.T) {
	testCases := []struct {
		name        string
		jobName     string
		buildID     string
		n           int64
		offset      int64
		expectedErr error
		expected    []byte
	}{
		{
			name:        "Podlog ReadAt range longer than contents",
			n:           100,
			jobName:     "BFG",
			buildID:     "435",
			offset:      3,
			expectedErr: io.EOF,
			expected:    []byte("bscottle"),
		},
		{
			name:        "Podlog ReadAt range within contents",
			n:           4,
			jobName:     "Fantastic Mr. Fox",
			buildID:     "4",
			offset:      2,
			expectedErr: nil,
			expected:    []byte("hund"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			artifact, err := NewPodLogArtifact(tc.jobName, tc.buildID, 500e6, &fakePodLogJAgent{})
			if err != nil {
				t.Fatalf("Pod Log Tests failed to create pod log artifact, err %v", err)
			}
			res := make([]byte, tc.n)
			readBytes, err := artifact.ReadAt(res, tc.offset)
			if err != tc.expectedErr {
				t.Fatalf("failed reading bytes of log. err: %v, expected err: %v", err, tc.expectedErr)
			}
			if !bytes.Equal(tc.expected, res[:readBytes]) {
				t.Errorf("Unexpected result of reading pod logs, expected %q, got %q", tc.expected, res)
			}
		})
	}

}
func TestReadAtMost_PodLog(t *testing.T) {
	testCases := []struct {
		name        string
		n           int64
		jobName     string
		buildID     string
		expectedErr error
		expected    []byte
	}{
		{
			name:        "Podlog ReadAtMost longer than contents",
			jobName:     "BFG",
			buildID:     "435",
			n:           100,
			expectedErr: io.EOF,
			expected:    []byte("frobscottle"),
		},
		{
			name:        "Podlog ReadAtMost shorter than contents",
			n:           3,
			jobName:     "BFG",
			buildID:     "435",
			expectedErr: nil,
			expected:    []byte("fro"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			artifact, err := NewPodLogArtifact(tc.jobName, tc.buildID, 500e6, &fakePodLogJAgent{})
			if err != nil {
				t.Fatalf("Pod Log Tests failed to create pod log artifact, err %v", err)
			}
			res, err := artifact.ReadAtMost(tc.n)
			if err != tc.expectedErr {
				t.Fatalf("failed reading bytes of log. err: %v, expected err: %v", err, tc.expectedErr)
			}
			if !bytes.Equal(tc.expected, res) {
				t.Errorf("Unexpected result of reading pod logs, expected %q, got %q", tc.expected, res)
			}
		})
	}

}

func TestReadAll_PodLog(t *testing.T) {
	fakePodLogAgent := &fakePodLogJAgent{}
	testCases := []struct {
		name        string
		jobName     string
		buildID     string
		sizeLimit   int64
		expectedErr error
		expected    []byte
	}{
		{
			name:        "Podlog readall not found",
			jobName:     "job",
			buildID:     "123",
			sizeLimit:   500e6,
			expectedErr: fmt.Errorf("error getting pod log size: error getting size of pod log: could not find job job, id 123"),
			expected:    nil,
		},
		{
			name:        "Simple \"BFG\" Podlog readall",
			jobName:     "BFG",
			buildID:     "435",
			sizeLimit:   500e6,
			expectedErr: nil,
			expected:    []byte("frobscottle"),
		},
		{
			name:        "\"Fantastic Mr. Fox\" Podlog readall",
			jobName:     "Fantastic Mr. Fox",
			buildID:     "4",
			sizeLimit:   500e6,
			expectedErr: nil,
			expected:    []byte("a hundred smoked hams and fifty sides of bacon"),
		},
		{
			name:        "Podlog readall over size limit",
			jobName:     "Fantastic Mr. Fox",
			buildID:     "4",
			sizeLimit:   5,
			expectedErr: lenses.ErrFileTooLarge,
			expected:    nil,
		},
	}
	for _, tc := range testCases {
		artifact, err := NewPodLogArtifact(tc.jobName, tc.buildID, tc.sizeLimit, fakePodLogAgent)
		if err != nil {
			t.Fatalf("Pod Log Tests failed to create pod log artifact, err %v", err)
		}
		res, err := artifact.ReadAll()
		if err != nil && err.Error() != tc.expectedErr.Error() {
			t.Fatalf("%s failed reading bytes of log. got err: %v, expected err: %v", tc.name, err, tc.expectedErr)
		}
		if err != nil {
			continue
		}
		if !bytes.Equal(tc.expected, res) {
			t.Errorf("Unexpected result of reading pod logs, expected %q, got %q", tc.expected, res)
		}

	}

}
