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
	"compress/gzip"
	"context"
	"io/ioutil"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/spyglass/viewers"
)

// GCSArtifact represents some output of a prow job stored in GCS
type GCSArtifact struct {
	// The handle of the object in GCS
	handle *storage.ObjectHandle

	// The direct link to the Artifact, can be used for read operations
	link string

	// The path of the Artifact within the job
	path string

	// Max size to read before failing
	sizeLimit int64
}

// NewGCSArtifact returns a new GCSArtifact with a given handle, canonical link, and path within the job
func NewGCSArtifact(handle *storage.ObjectHandle, link string, path string, sizeLimit int64) *GCSArtifact {
	return &GCSArtifact{
		handle:    handle,
		link:      link,
		path:      path,
		sizeLimit: sizeLimit,
	}
}

// Size returns the size of the artifact in GCS
func (a *GCSArtifact) Size() int64 {
	attrs, err := a.handle.Attrs(context.Background())
	if err != nil {
		logrus.WithError(err).Error("Could not retrieve object attributes")
	}
	return attrs.Size
}

// JobPath gets the GCS path of the artifact within the current job
func (a *GCSArtifact) JobPath() string {
	return a.path
}

// CanonicalLink gets the GCS web address of the artifact
func (a *GCSArtifact) CanonicalLink() string {
	return a.link
}

// Read reads len(p) bytes from a file in GCS
func (a *GCSArtifact) ReadAt(p []byte, off int64) (n int, err error) {
	if a.gzipped() {
		return 0, viewers.ErrUnsupportedOp
	}
	reader, err := a.handle.NewRangeReader(context.Background(), off, int64(len(p)))
	defer reader.Close()
	if err != nil {
		return 0, err
	}
	n, err = reader.Read(p)
	return n, err
}

// ReadAtMost reads at most n bytes from a file in GCS. If the file is compressed (gzip) in GCS, n bytes
// of gzipped content will be downloaded and decompressed into potentially GREATER than n bytes of content.
func (a *GCSArtifact) ReadAtMost(n int64) ([]byte, error) {
	reader, err := a.handle.NewRangeReader(context.Background(), 0, n+1)
	defer reader.Close()
	if err != nil {
		return []byte{}, err
	}
	var p []byte
	var e error
	if a.gzipped() {
		gReader, err := gzip.NewReader(reader)
		if err != nil {
			return []byte{}, err
		}
		p, e = ioutil.ReadAll(gReader)
	} else {
		p, e = ioutil.ReadAll(reader)
	}
	return p, e
}

// ReadAll will either read the entire file or throw an error if file size is too big
func (a *GCSArtifact) ReadAll() ([]byte, error) {
	if a.Size() > a.sizeLimit {
		return []byte{}, viewers.ErrFileTooLarge
	}
	reader, err := a.handle.NewReader(context.Background())
	defer reader.Close()
	if err != nil {
		return []byte{}, err
	}
	p, err := ioutil.ReadAll(reader)
	return p, err
}

// ReadTail reads the last n bytes from a file in GCS
func (a *GCSArtifact) ReadTail(n int64) ([]byte, error) {
	if a.gzipped() {
		return []byte{}, viewers.ErrUnsupportedOp
	}
	offset := a.Size() - n
	reader, err := a.handle.NewRangeReader(context.Background(), offset, -1)
	defer reader.Close()
	if err != nil {
		logrus.WithField("artifact", a.JobPath()).WithError(err).Error("Error getting range reader to artifact")
		return []byte{}, err
	}
	read, err := ioutil.ReadAll(reader)
	return read, err
}

// gzipped returns whether the file is gzip-encoded in GCS
func (a *GCSArtifact) gzipped() bool {
	attrs, err := a.handle.Attrs(context.Background())
	if err != nil {
		logrus.WithError(err).Error("Failed to get GCS object attributes.")
	}
	return attrs.ContentEncoding == "gzip"
}
