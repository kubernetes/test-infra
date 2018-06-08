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

// GCSArtifact represents some output of a prow job stored in GCS
type GCSArtifact struct {
	Artifact

	// The direct link to the Artifact, can be used for read operations
	link string

	// The path of the Artifact within the job
	path string
}

// Gets the GCS path of the artifact within the current job
func (a GCSArtifact) JobPath() string {
	return a.path
}

// Gets the GCS web address of the artifact
func (a GCSArtifact) CanonicalLink() string {
	return a.link
}

// Reads len(p) bytes from GCS bucket
func (a GCSArtifact) Read(p []byte) (n int, err error) {
	// TODO
	return
}

// Reads all bytes from a file in GCS
func (a GCSArtifact) ReadAll() string {
	// TODO
	return ""
}

// Seeks to a location in a GCS file
func Seek(offset int64, whence int) (int64, error) {
	// TODO
	return 0, nil
}
