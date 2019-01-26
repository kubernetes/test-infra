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

	"cloud.google.com/go/storage"
)

type ByteReadCloser struct {
	io.Reader
}

func (rc *ByteReadCloser) Close() error {
	return nil
}

func (rc *ByteReadCloser) Read(p []byte) (int, error) {
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
	oAttrs   *storage.ObjectAttrs
	contents []byte
}

func (h *fakeArtifactHandle) Attrs(ctx context.Context) (*storage.ObjectAttrs, error) {
	if bytes.Equal(h.contents, []byte("no attrs")) {
		return nil, fmt.Errorf("error getting attrs")
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
	return &ByteReadCloser{bytes.NewReader(h.contents[offset : offset+toRead])}, err
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
	return &ByteReadCloser{bytes.NewReader(h.contents)}, nil
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
		artifact := NewGCSArtifact(context.Background(), &fakeArtifactHandle{
			contents: tc.contents,
			oAttrs: &storage.ObjectAttrs{
				Bucket:          "foo-bucket",
				Name:            "build-log.txt",
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
		artifact := NewGCSArtifact(context.Background(), &fakeArtifactHandle{
			contents: tc.contents,
			oAttrs: &storage.ObjectAttrs{
				Bucket:          "foo-bucket",
				Name:            "build-log.txt",
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
		name      string
		n         int64
		offset    int64
		contents  []byte
		encoding  string
		expected  []byte
		expectErr bool
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
		artifact := NewGCSArtifact(context.Background(), &fakeArtifactHandle{
			contents: tc.contents,
			oAttrs: &storage.ObjectAttrs{
				Bucket:          "foo-bucket",
				Name:            "build-log.txt",
				Size:            int64(len(tc.contents)),
				ContentEncoding: tc.encoding,
			},
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
		artifact := NewGCSArtifact(context.Background(), &fakeArtifactHandle{
			contents: tc.contents,
			oAttrs: &storage.ObjectAttrs{
				Bucket: "foo-bucket",
				Name:   "build-log.txt",
				Size:   int64(len(tc.contents)),
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
	fakeGCSBucket := fakeGCSClient.Bucket("test-bucket")
	startedContent := []byte("hi jason, im started")
	testCases := []struct {
		name      string
		handle    artifactHandle
		expected  int64
		expectErr bool
	}{
		{
			name: "Test size simple",
			handle: &fakeArtifactHandle{
				contents: startedContent,
				oAttrs: &storage.ObjectAttrs{
					Bucket: "foo-bucket",
					Name:   "started.json",
					Size:   int64(len(startedContent)),
				},
			},
			expected:  int64(len(startedContent)),
			expectErr: false,
		},
		{
			name: "Test size from attrs error",
			handle: &fakeArtifactHandle{
				contents: []byte("no attrs"),
				oAttrs: &storage.ObjectAttrs{
					Bucket: "foo-bucket",
					Name:   "started.json",
					Size:   8,
				},
			},
			expectErr: true,
		},
		{
			name:      "Size of nonexistentArtifact",
			handle:    &gcsArtifactHandle{fakeGCSBucket.Object("logs/example-ci-run/404/started.json")},
			expectErr: true,
		},
	}
	for _, tc := range testCases {
		artifact := NewGCSArtifact(context.Background(), tc.handle, "", "started.json", 500e6)
		actual, err := artifact.Size()
		if err != nil && !tc.expectErr {
			t.Fatalf("%s failed getting size for artifact %s, err: %v", tc.name, artifact.JobPath(), err)
		}
		if err == nil && tc.expectErr {
			t.Errorf("%s did not produce error when error was expected.", tc.name)
		}
		if tc.expected != actual {
			t.Errorf("Test %s failed.\nExpected:\n%d\nActual:\n%d", tc.name, tc.expected, actual)
		}
	}
}
