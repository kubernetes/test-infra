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

	"k8s.io/test-infra/prow/spyglass/viewers"
)

var (
	// ErrCannotParseSource is thrown when an invalid source string is provided to an ArtifactFetcher
	ErrCannotParseSource = errors.New("could not create job source from provided source")
)

// jobSource gets information about a location storing the results of a single Prow job
type jobSource interface {
	// CanonicalLink returns a link to the storage location of this job's artifacts.
	canonicalLink() string
	// JobPath returns the common prefix of all artifacts produced by this job
	jobPath() string
}

// ArtifactFetcher gets all information necessary to perform IO operations on artifacts from a storage provider
type ArtifactFetcher interface {
	// Lists the names of all artifacts associated with this job source.
	// If treating the source as a directory structure, names of artifacts should be returned
	// with the job path prefix removed.
	// (e.g. return names of the format artifacts/junit_01.xml instead of
	// logs/ci-example-run/1456/artifacts/junit_01.xml)
	artifacts(src jobSource) []string
	// Artifact constructs and returns an artifact object ready for read operations from the
	// job source and artifact name.
	artifact(src jobSource, name string, sizeLimit int64) viewers.Artifact
	// createJobSource tries to create a usable job source from the provided source string for this fetcher
	createJobSource(src string) (jobSource, error)
}
