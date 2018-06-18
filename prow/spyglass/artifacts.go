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
	"context"
	"io/ioutil"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
)

// GCSArtifact represents some output of a prow job stored in GCS
type GCSArtifact struct {
	Artifact

	// The handle of the object in GCS
	Handle *storage.ObjectHandle

	// The direct link to the Artifact, can be used for read operations
	link string

	// The path of the Artifact within the job
	path string
}

// Size returns the size of the artifact in GCS
func (a GCSArtifact) Size() int64 {
	attrs, err := a.Handle.Attrs(context.Background())
	if err != nil {
		logrus.Errorf("Could not retrieve object attributes for artifact %s.\nErr: %s\n", a.path, err)
	}
	return attrs.Size
}

// JobPath gets the GCS path of the artifact within the current job
func (a GCSArtifact) JobPath() string {
	return a.path
}

// CanonicalLink gets the GCS web address of the artifact
func (a GCSArtifact) CanonicalLink() string {
	return a.link
}

// Read reads len(p) bytes from a file in GCS
func (a GCSArtifact) ReadAt(p []byte, off int64) (n int, err error) {
	reader, err := a.Handle.NewRangeReader(context.Background(), off, int64(len(p)))
	if err != nil {
		logrus.Errorf("There was an error getting a Reader to the desired artifact: %s", err)
	}
	return reader.Read(p)
}

// ReadAll reads all bytes from a file in GCS
func (a GCSArtifact) ReadAll() ([]byte, error) {
	reader, err := a.Handle.NewReader(context.Background())
	if err != nil {
		logrus.Errorf("There was an error getting a Reader to the desired artifact: %s", err)
	}
	return ioutil.ReadAll(reader)
}

// ReadTail reads the last n bytes from a file in GCS
func (a GCSArtifact) ReadTail(n int64) ([]byte, error) {
	offset := a.Size() - n
	reader, err := a.Handle.NewRangeReader(context.Background(), offset, -1)
	if err != nil {
		logrus.Errorf("There was an error getting a Reader to the desired artifact: %s", err)
	}
	return ioutil.ReadAll(reader)
}
