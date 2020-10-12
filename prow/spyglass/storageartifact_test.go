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
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"testing"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	pkgio "k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/spyglass/api"
	"k8s.io/test-infra/prow/spyglass/lenses"
)

type ByteReadCloser struct {
	io.Reader
	incompleteRead bool
}

func (rc *ByteReadCloser) Close() error {
	return nil
}

func (rc *ByteReadCloser) Read(p []byte) (int, error) {
	if rc.incompleteRead {
		p = p[:len(p)/2]
		rc.incompleteRead = false
	}
	read, err := rc.Reader.Read(p)
	if err != nil {
		return 0, err
	}
	if bytes.Equal(p[:read], []byte("deeper unreadable contents")) {
		return 0, fmt.Errorf("it's just turtes all the way down")
	}
	return read, nil
}

type fakeArtifactHandle struct {
	oAttrs         pkgio.Attributes
	contents       []byte
	incompleteRead bool
}

func (h *fakeArtifactHandle) Attrs(ctx context.Context) (pkgio.Attributes, error) {
	if bytes.Equal(h.contents, []byte("no attrs")) {
		return pkgio.Attributes{}, fmt.Errorf("error getting attrs")
	}
	return h.oAttrs, nil
}

func (h *fakeArtifactHandle) NewRangeReader(ctx context.Context, offset, length int64) (io.ReadCloser, error) {
	if bytes.Equal(h.contents, []byte("unreadable contents")) {
		return nil, fmt.Errorf("cannot read unreadable contents")
	}
	lenContents := int64(len(h.contents))
	var err error
	var toRead int64
	if length < 0 {
		toRead = lenContents - offset
		err = io.EOF
	} else {
		toRead = length
		if offset+length > lenContents {
			toRead = lenContents - offset
			err = io.EOF
		}
	}
	return &ByteReadCloser{bytes.NewReader(h.contents[offset : offset+toRead]), h.incompleteRead}, err
}

func (h *fakeArtifactHandle) NewReader(ctx context.Context) (io.ReadCloser, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write([]byte("unreadable contents"))
	if err != nil {
		return nil, fmt.Errorf("Failed to gzip log text, err: %v", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("Failed to close gzip writer, err: %v", err)
	}
	if bytes.Equal(h.contents, buf.Bytes()) {
		return nil, fmt.Errorf("cannot read unreadable contents, even if they're gzipped")
	}
	if bytes.Equal(h.contents, []byte("unreadable contents")) {
		return nil, fmt.Errorf("cannot read unreadable contents")
	}
	return &ByteReadCloser{bytes.NewReader(h.contents), false}, nil
}

// Tests reading the tail n bytes of data from an artifact
func TestReadTail(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write([]byte("Oh wow\nlogs\nthis is\ncrazy"))
	if err != nil {
		t.Fatalf("Failed to gzip log text, err: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Failed to close gzip writer, err: %v", err)
	}
	gzippedLog := buf.Bytes()
	testCases := []struct {
		name      string
		n         int64
		contents  []byte
		encoding  string
		expected  []byte
		expectErr bool
	}{
		{
			name:      "ReadTail example build log",
			n:         4,
			contents:  []byte("Oh wow\nlogs\nthis is\ncrazy"),
			expected:  []byte("razy"),
			expectErr: false,
		},
		{
			name:      "ReadTail build log, gzipped",
			n:         23,
			contents:  gzippedLog,
			encoding:  "gzip",
			expectErr: true,
		},
		{
			name:      "ReadTail build log, claimed gzipped but not actually gzipped",
			n:         2333,
			contents:  []byte("Oh wow\nlogs\nthis is\ncrazy"),
			encoding:  "gzip",
			expectErr: true,
		},
		{
			name:      "ReadTail N>size of build log",
			n:         2222,
			contents:  []byte("Oh wow\nlogs\nthis is\ncrazy"),
			expected:  []byte("Oh wow\nlogs\nthis is\ncrazy"),
			expectErr: false,
		},
	}
	for _, tc := range testCases {
		artifact := NewStorageArtifact(context.Background(), &fakeArtifactHandle{
			contents: tc.contents,
			oAttrs: pkgio.Attributes{
				Size:            int64(len(tc.contents)),
				ContentEncoding: tc.encoding,
			},
		}, "", "build-log.txt", 500e6)
		actualBytes, err := artifact.ReadTail(tc.n)
		if err != nil && !tc.expectErr {
			t.Fatalf("Test %s failed with err: %v", tc.name, err)
		}
		if err == nil && tc.expectErr {
			t.Errorf("Test %s did not produce error when expected", tc.name)
		}
		if !bytes.Equal(actualBytes, tc.expected) {
			t.Errorf("Test %s failed.\nExpected: %s\nActual: %s", tc.name, tc.expected, actualBytes)
		}
	}
}

// Tests reading at most n bytes of data from files in GCS
func TestReadAtMost(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write([]byte("Oh wow\nlogs\nthis is\ncrazy"))
	if err != nil {
		t.Fatalf("Failed to gzip log text, err: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Failed to close gzip writer, err: %v", err)
	}
	testCases := []struct {
		name      string
		n         int64
		contents  []byte
		encoding  string
		expected  []byte
		expectErr bool
		expectEOF bool
	}{
		{
			name:      "ReadAtMost example build log",
			n:         4,
			contents:  []byte("Oh wow\nlogs\nthis is\ncrazy"),
			expected:  []byte("Oh w"),
			expectErr: false,
		},
		{
			name:      "ReadAtMost build log, transparently gzipped",
			n:         8,
			contents:  []byte("Oh wow\nlogs\nthis is\ncrazy"),
			expected:  []byte("Oh wow\nl"),
			encoding:  "gzip",
			expectErr: false,
		},
		{
			name:      "ReadAtMost unreadable contents",
			n:         2,
			contents:  []byte("unreadable contents"),
			expectErr: true,
		},
		{
			name:      "ReadAtMost unreadable contents",
			n:         45,
			contents:  []byte("deeper unreadable contents"),
			expectErr: true,
		},
		{
			name:      "ReadAtMost N>size of build log",
			n:         2222,
			contents:  []byte("Oh wow\nlogs\nthis is\ncrazy"),
			expected:  []byte("Oh wow\nlogs\nthis is\ncrazy"),
			expectErr: true,
			expectEOF: true,
		},
	}
	for _, tc := range testCases {
		artifact := NewStorageArtifact(context.Background(), &fakeArtifactHandle{
			contents: tc.contents,
			oAttrs: pkgio.Attributes{
				Size:            int64(len(tc.contents)),
				ContentEncoding: tc.encoding,
			},
		}, "", "build-log.txt", 500e6)
		actualBytes, err := artifact.ReadAtMost(tc.n)
		if err != nil && !tc.expectErr {
			if tc.expectEOF && err != io.EOF {
				t.Fatalf("Test %s failed with err: %v, expected EOF", tc.name, err)
			}
			t.Fatalf("Test %s failed with err: %v", tc.name, err)
		}
		if err != nil && tc.expectEOF && err != io.EOF {
			t.Fatalf("Test %s failed with err: %v, expected EOF", tc.name, err)
		}
		if err == nil && tc.expectErr {
			t.Errorf("Test %s did not produce error when expected", tc.name)
		}
		if !bytes.Equal(actualBytes, tc.expected) {
			t.Errorf("Test %s failed.\nExpected: %s\nActual: %s", tc.name, tc.expected, actualBytes)
		}
	}
}

// Tests reading at offset from files in GCS
func TestReadAt(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write([]byte("Oh wow\nlogs\nthis is\ncrazy"))
	if err != nil {
		t.Fatalf("Failed to gzip log text, err: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Failed to close gzip writer, err: %v", err)
	}
	gzippedLog := buf.Bytes()
	testCases := []struct {
		name           string
		n              int64
		offset         int64
		contents       []byte
		encoding       string
		expected       []byte
		expectErr      bool
		incompleteRead bool
	}{
		{
			name:      "ReadAt example build log",
			n:         4,
			offset:    6,
			contents:  []byte("Oh wow\nlogs\nthis is\ncrazy"),
			expected:  []byte("\nlog"),
			expectErr: false,
		},
		{
			name:      "ReadAt offset past file size",
			n:         4,
			offset:    400,
			contents:  []byte("Oh wow\nlogs\nthis is\ncrazy"),
			expectErr: true,
		},
		{
			name:           "ReadAt needs multiple internal Reads",
			n:              4,
			offset:         6,
			contents:       []byte("Oh wow\nlogs\nthis is\ncrazy"),
			expected:       []byte("\nlog"),
			incompleteRead: true,
		},
		{
			name:      "ReadAt build log, gzipped",
			n:         23,
			contents:  gzippedLog,
			encoding:  "gzip",
			expectErr: true,
		},
		{
			name:      "ReadAt, claimed gzipped but not actually gzipped",
			n:         2333,
			contents:  []byte("Oh wow\nlogs\nthis is\ncrazy"),
			encoding:  "gzip",
			expectErr: true,
		},
		{
			name:      "ReadAt offset negative",
			offset:    -3,
			n:         32,
			contents:  []byte("Oh wow\nlogs\nthis is\ncrazy"),
			expectErr: true,
		},
	}
	for _, tc := range testCases {
		artifact := NewStorageArtifact(context.Background(), &fakeArtifactHandle{
			contents: tc.contents,
			oAttrs: pkgio.Attributes{
				Size:            int64(len(tc.contents)),
				ContentEncoding: tc.encoding,
			},
			incompleteRead: tc.incompleteRead,
		}, "", "build-log.txt", 500e6)
		p := make([]byte, tc.n)
		bytesRead, err := artifact.ReadAt(p, tc.offset)
		if err != nil && !tc.expectErr {
			t.Fatalf("Test %s failed with err: %v", tc.name, err)
		}
		if err == nil && tc.expectErr {
			t.Errorf("Test %s did not produce error when expected", tc.name)
		}
		readBytes := p[:bytesRead]
		if !bytes.Equal(readBytes, tc.expected) {
			t.Errorf("Test %s failed.\nExpected: %s\nActual: %s", tc.name, tc.expected, readBytes)
		}
	}

}

// Tests reading all data from files in GCS
func TestReadAll(t *testing.T) {
	testCases := []struct {
		name      string
		sizeLimit int64
		contents  []byte
		expectErr bool
		expected  []byte
	}{
		{
			name:      "ReadAll example build log",
			contents:  []byte("Oh wow\nlogs\nthis is\ncrazy"),
			sizeLimit: 500e6,
			expected:  []byte("Oh wow\nlogs\nthis is\ncrazy"),
		},
		{
			name:      "ReadAll example too large build log",
			sizeLimit: 20,
			contents:  []byte("Oh wow\nlogs\nthis is\ncrazy"),
			expectErr: true,
			expected:  nil,
		},
		{
			name:      "ReadAll unable to get reader",
			sizeLimit: 500e6,
			contents:  []byte("unreadable contents"),
			expectErr: true,
			expected:  nil,
		},
		{
			name:      "ReadAll unable to read contents",
			sizeLimit: 500e6,
			contents:  []byte("deeper unreadable contents"),
			expectErr: true,
			expected:  nil,
		},
	}
	for _, tc := range testCases {
		artifact := NewStorageArtifact(context.Background(), &fakeArtifactHandle{
			contents: tc.contents,
			oAttrs: pkgio.Attributes{
				Size: int64(len(tc.contents)),
			},
		}, "", "build-log.txt", tc.sizeLimit)

		actualBytes, err := artifact.ReadAll()
		if err != nil && !tc.expectErr {
			t.Fatalf("Test %s failed with err: %v", tc.name, err)
		}
		if err == nil && tc.expectErr {
			t.Errorf("Test %s did not produce error when expected", tc.name)
		}
		if !bytes.Equal(actualBytes, tc.expected) {
			t.Errorf("Test %s failed.\nExpected: %s\nActual: %s", tc.name, tc.expected, actualBytes)
		}
	}
}

func TestSize_GCS(t *testing.T) {
	fakeGCSClient := fakeGCSServer.Client()
	fakeOpener := pkgio.NewGCSOpener(fakeGCSClient)
	startedContent := []byte("hi jason, im started")
	testCases := []struct {
		name      string
		handle    artifactHandle
		expected  int64
		expectErr string
	}{
		{
			name: "Test size simple",
			handle: &fakeArtifactHandle{
				contents: startedContent,
				oAttrs: pkgio.Attributes{
					Size: int64(len(startedContent)),
				},
			},
			expected: int64(len(startedContent)),
		},
		{
			name: "Test size from attrs error",
			handle: &fakeArtifactHandle{
				contents: []byte("no attrs"),
				oAttrs: pkgio.Attributes{
					Size: 8,
				},
			},
			expectErr: "error getting gcs attributes for artifact: error getting attrs",
		},
		{
			name: "Size of nonexistentArtifact",
			handle: &storageArtifactHandle{
				Opener: fakeOpener,
				Name:   "gs://test-bucket/logs/example-ci-run/404/started.json",
			},
			expectErr: "error getting gcs attributes for artifact: storage: object doesn't exist",
		},
	}
	for _, tc := range testCases {
		artifact := NewStorageArtifact(context.Background(), tc.handle, "", prowv1.StartedStatusFile, 500e6)
		actual, err := artifact.Size()
		var actualErr string
		if err != nil {
			actualErr = err.Error()
		}
		if actualErr != tc.expectErr {
			t.Fatalf("%s failed getting size for artifact %s, error = %v, expectErr %v", tc.name, artifact.JobPath(), actualErr, tc.expectErr)
		}
		if tc.expected != actual {
			t.Errorf("Test %s failed.\nExpected:\n%d\nActual:\n%d", tc.name, tc.expected, actual)
		}
	}
}

func TestStorageArtifact_RespectsSizeLimit(t *testing.T) {
	contents := "Supercalifragilisticexpialidocious"
	numRequestedBytes := int64(10)

	testCases := []struct {
		name     string
		expected error
		skipGzip bool
		action   func(api.Artifact) error
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
			skipGzip: true, // `offset read on gzipped files unsupported`
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
			skipGzip: true, // `offset read on gzipped files unsupported`
			action: func(a api.Artifact) error {
				_, err := a.ReadTail(numRequestedBytes)
				return err
			},
		},
	}
	for _, tc := range testCases {
		for _, encoding := range []string{"", "gzip"} {
			if encoding == "gzip" && tc.skipGzip {
				continue
			}
			t.Run(tc.name+encoding+"_NoErrors", func(nested *testing.T) {
				sizeLimit := int64(2 * len(contents))
				artifact := NewStorageArtifact(context.Background(), &fakeArtifactHandle{
					contents: []byte(contents),
					oAttrs: pkgio.Attributes{
						Size:            int64(len(contents)),
						ContentEncoding: encoding,
					},
				}, "some-link-path", "build-log.txt", sizeLimit)
				actual := tc.action(artifact)
				if actual != nil {
					nested.Fatalf("unexpected error: %s", actual)
				}
			})
			t.Run(tc.name+encoding+"_WithErrors", func(nested *testing.T) {
				sizeLimit := int64(5)
				artifact := NewStorageArtifact(context.Background(), &fakeArtifactHandle{
					contents: []byte(contents),
					oAttrs: pkgio.Attributes{
						Size:            int64(len(contents)),
						ContentEncoding: encoding,
					},
				}, "some-link-path", "build-log.txt", sizeLimit)
				actual := tc.action(artifact)
				if actual == nil {
					nested.Fatalf("expected error (%s), but got: nil", tc.expected)
				} else if tc.expected.Error() != actual.Error() {
					nested.Fatalf("expected error (%s), but got: %s", tc.expected, actual)
				}
			})
		}
	}
}
