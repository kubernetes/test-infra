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

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/spyglass/api"
	"k8s.io/test-infra/prow/spyglass/lenses"
)

const customContainerName = "custom-container"

// fakePodLogJAgent used for pod log artifact dependency injection
type fakePodLogJAgent struct {
}

func (j *fakePodLogJAgent) GetProwJob(job, id string) (prowapi.ProwJob, error) {
	return prowapi.ProwJob{}, nil
}

func (j *fakePodLogJAgent) GetJobLog(job, id, container string) ([]byte, error) {
	if job == "BFG" && id == "435" {
		switch container {
		case kube.TestContainerName:
			return []byte("frobscottle"), nil
		case customContainerName:
			return []byte("snozzcumber"), nil
		}
	} else if job == "Fantastic Mr. Fox" && id == "4" {
		return []byte("a hundred smoked hams and fifty sides of bacon"), nil
	}
	return nil, fmt.Errorf("could not find job %s, id %s, container %s", job, id, container)
}

func TestNewPodLogArtifact(t *testing.T) {
	testCases := []struct {
		name         string
		jobName      string
		buildID      string
		container    string
		artifact     string
		sizeLimit    int64
		expectedErr  error
		expectedLink string
	}{
		{
			name:         "Create pod log with valid fields",
			jobName:      "job",
			buildID:      "123",
			container:    kube.TestContainerName,
			artifact:     singleLogName,
			sizeLimit:    500e6,
			expectedErr:  nil,
			expectedLink: fmt.Sprintf("/log?container=%s&id=123&job=job", kube.TestContainerName),
		},
		{
			name:         "Create pod log with valid fields and custom container",
			jobName:      "job",
			buildID:      "123",
			container:    customContainerName,
			artifact:     fmt.Sprintf("%s-%s", customContainerName, singleLogName),
			sizeLimit:    500e6,
			expectedErr:  nil,
			expectedLink: fmt.Sprintf("/log?container=%s&id=123&job=job", customContainerName),
		},
		{
			name:         "Create pod log with no jobName",
			jobName:      "",
			buildID:      "123",
			container:    kube.TestContainerName,
			artifact:     singleLogName,
			sizeLimit:    500e6,
			expectedErr:  errInsufficientJobInfo,
			expectedLink: "",
		},
		{
			name:         "Create pod log with no buildID",
			jobName:      "job",
			buildID:      "",
			container:    kube.TestContainerName,
			artifact:     singleLogName,
			sizeLimit:    500e6,
			expectedErr:  errInsufficientJobInfo,
			expectedLink: "",
		},
		{
			name:         "Create pod log with negative sizeLimit",
			jobName:      "job",
			buildID:      "123",
			container:    kube.TestContainerName,
			artifact:     singleLogName,
			sizeLimit:    -4,
			expectedErr:  errInvalidSizeLimit,
			expectedLink: "",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			artifact, err := NewPodLogArtifact(tc.jobName, tc.buildID, tc.artifact, tc.container, tc.sizeLimit, &fakePodLogJAgent{})
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
		container string
		artifact  *PodLogArtifact
		n         int64
		expected  []byte
		expectErr bool
	}{
		{
			name:      "Podlog ReadTail longer than contents",
			jobName:   "BFG",
			buildID:   "435",
			container: kube.TestContainerName,
			n:         50,
			expected:  []byte("frobscottle"),
		},
		{
			name:      "Podlog ReadTail shorter than contents",
			jobName:   "Fantastic Mr. Fox",
			buildID:   "4",
			container: kube.TestContainerName,
			n:         3,
			expected:  []byte("con"),
		},
		{
			name:      "Podlog ReadTail longer for different container",
			jobName:   "BFG",
			buildID:   "435",
			container: customContainerName,
			n:         50,
			expected:  []byte("snozzcumber"),
		},
		{
			name:      "Podlog ReadTail shorter for different container",
			jobName:   "BFG",
			buildID:   "435",
			container: customContainerName,
			n:         3,
			expected:  []byte("ber"),
		},
		{
			name:      "Podlog ReadTail nonexistent pod",
			jobName:   "Fax",
			buildID:   "4",
			container: kube.TestContainerName,
			n:         3,
			expectErr: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			artifact, err := NewPodLogArtifact(tc.jobName, tc.buildID, singleLogName, tc.container, 500e6, &fakePodLogJAgent{})
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
		container   string
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
			container:   kube.TestContainerName,
			offset:      3,
			expectedErr: io.EOF,
			expected:    []byte("bscottle"),
		},
		{
			name:        "Podlog ReadAt range within contents",
			n:           4,
			jobName:     "Fantastic Mr. Fox",
			buildID:     "4",
			container:   kube.TestContainerName,
			offset:      2,
			expectedErr: nil,
			expected:    []byte("hund"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			artifact, err := NewPodLogArtifact(tc.jobName, tc.buildID, singleLogName, tc.container, 500e6, &fakePodLogJAgent{})
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
		container   string
		expectedErr error
		expected    []byte
	}{
		{
			name:        "Podlog ReadAtMost longer than contents",
			jobName:     "BFG",
			buildID:     "435",
			container:   kube.TestContainerName,
			n:           100,
			expectedErr: io.EOF,
			expected:    []byte("frobscottle"),
		},
		{
			name:        "Podlog ReadAtMost shorter than contents",
			n:           3,
			jobName:     "BFG",
			buildID:     "435",
			container:   kube.TestContainerName,
			expectedErr: nil,
			expected:    []byte("fro"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			artifact, err := NewPodLogArtifact(tc.jobName, tc.buildID, singleLogName, tc.container, 500e6, &fakePodLogJAgent{})
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
		container   string
		sizeLimit   int64
		expectedErr error
		expected    []byte
	}{
		{
			name:        "Podlog readall not found",
			jobName:     "job",
			buildID:     "123",
			container:   kube.TestContainerName,
			sizeLimit:   500e6,
			expectedErr: fmt.Errorf("error getting pod log size: error getting size of pod log: could not find job job, id 123, container %s", kube.TestContainerName),
			expected:    nil,
		},
		{
			name:        "Simple \"BFG\" Podlog readall",
			jobName:     "BFG",
			buildID:     "435",
			container:   kube.TestContainerName,
			sizeLimit:   500e6,
			expectedErr: nil,
			expected:    []byte("frobscottle"),
		},
		{
			name:        "Simple \"BFG\" Podlog readall for custom container",
			jobName:     "BFG",
			buildID:     "435",
			container:   customContainerName,
			sizeLimit:   500e6,
			expectedErr: nil,
			expected:    []byte("snozzcumber"),
		},
		{
			name:        "\"Fantastic Mr. Fox\" Podlog readall",
			jobName:     "Fantastic Mr. Fox",
			buildID:     "4",
			container:   kube.TestContainerName,
			sizeLimit:   500e6,
			expectedErr: nil,
			expected:    []byte("a hundred smoked hams and fifty sides of bacon"),
		},
		{
			name:        "Podlog readall over size limit",
			jobName:     "Fantastic Mr. Fox",
			buildID:     "4",
			container:   kube.TestContainerName,
			sizeLimit:   5,
			expectedErr: lenses.ErrFileTooLarge,
			expected:    nil,
		},
	}
	for _, tc := range testCases {
		artifact, err := NewPodLogArtifact(tc.jobName, tc.buildID, singleLogName, tc.container, tc.sizeLimit, fakePodLogAgent)
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

type fakeAgent struct {
	contents string
}

func (f *fakeAgent) GetProwJob(job, id string) (prowapi.ProwJob, error) {
	return prowapi.ProwJob{}, nil
}

func (f *fakeAgent) GetJobLog(job, id, container string) ([]byte, error) {
	return []byte(f.contents), nil
}

func TestPodLogArtifact_RespectsSizeLimit(t *testing.T) {
	contents := "Supercalifragilisticexpialidocious"
	numRequestedBytes := int64(10)

	testCases := []struct {
		name      string
		expected  error
		contents  string
		skipGzip  bool
		sizeLimit int64
		action    func(api.Artifact) error
	}{
		{
			name:     "ReadAll",
			expected: lenses.ErrFileTooLarge,
			action: func(a api.Artifact) error {
				_, err := a.ReadAll()
				return err
			},
		},
		{
			name:     "ReadAt",
			expected: lenses.ErrRequestSizeTooLarge,
			action: func(a api.Artifact) error {
				buf := make([]byte, numRequestedBytes)
				_, err := a.ReadAt(buf, 3)
				return err
			},
		},
		{
			name:     "ReadAtMost",
			expected: lenses.ErrRequestSizeTooLarge,
			action: func(a api.Artifact) error {
				_, err := a.ReadAtMost(numRequestedBytes)
				return err
			},
		},
		{
			name:     "ReadTail",
			expected: lenses.ErrRequestSizeTooLarge,
			action: func(a api.Artifact) error {
				_, err := a.ReadTail(numRequestedBytes)
				return err
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name+"_NoError", func(nested *testing.T) {
			sizeLimit := int64(2 * len(contents))
			artifact, err := NewPodLogArtifact("job-name", "build-id", "log-name", "container-name", sizeLimit, &fakeAgent{contents: contents})
			if err != nil {
				nested.Fatalf("error creating test data: %s", err)
			}

			actual := tc.action(artifact)
			if actual != nil {
				nested.Fatalf("unexpected error: %s", actual)
			}
		})
		t.Run(tc.name+"_WithError", func(nested *testing.T) {
			sizeLimit := int64(5)
			artifact, err := NewPodLogArtifact("job-name", "build-id", "log-name", "container-name", sizeLimit, &fakeAgent{contents: contents})
			if err != nil {
				nested.Fatalf("error creating test data: %s", err)
			}

			actual := tc.action(artifact)
			if actual == nil {
				nested.Fatalf("expected error (%s), but got: nil", tc.expected)
			} else if tc.expected.Error() != actual.Error() {
				nested.Fatalf("expected error (%s), but got: %s", tc.expected, actual)
			}
		})
	}
}
