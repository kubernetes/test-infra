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
	"errors"
	"net/url"

	"k8s.io/test-infra/prow/spyglass/viewers"
)

var (
	// ErrCannotParseSource is thrown when an invalid source string is provided to an ArtifactFetcher
	ErrCannotParseSource = errors.New("could not create job source from provided source")
)

// ArtifactFetcher gets all information necessary to perform IO operations on artifacts from a storage provider
type ArtifactFetcher interface {
	// Lists the names of all artifacts associated with this job source.
	// If treating the source as a directory structure, names of artifacts should be returned
	// with the job path prefix removed.
	// (e.g. return names of the format artifacts/junit_01.xml instead of
	// logs/ci-example-run/1456/artifacts/3unit_01.xml)
	artifactNames(srcURL url.URL) ([]string, error)
	// Artifact constructs and returns an artifact object ready for read operations
	artifact(srcURL url.URL, name string, sizeLimit int64) (viewers.Artifact, error)
}
