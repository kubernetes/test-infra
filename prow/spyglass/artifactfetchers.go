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
	"k8s.io/test-infra/prow/spyglass/viewers"
)

// JobSource gets information about a location storing the results of a single Prow job
type JobSource interface {
	// CanonicalLink returns a link to view this job's artifacts
	CanonicalLink() string
	// JobPath returns the common prefix (excluding the name of the bucket) of all artifacts produced
	// by this job
	JobPath() string
	BucketName() string
}

// ArtifactFetcher gets all information necessary to perform IO operations on artifacts from a storage provider
type ArtifactFetcher interface {
	// Lists the names of all artifacts associated with this job source.
	// If treating the source as a directory structure, names of artifacts should be returned
	// with the job path prefix removed.
	// (e.g. return names of the format artifacts/junit_01.xml instead of
	// logs/ci-example-run/1456/artifacts/junit_01.xml)
	Artifacts(src *JobSource) []string
	// Artifact constructs and returns an artifact object ready for read operations from the
	// job source and artifact name.
	Artifact(src *JobSource, name string) viewers.Artifact
}
