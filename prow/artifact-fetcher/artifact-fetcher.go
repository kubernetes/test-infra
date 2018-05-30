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

// artifact-fetcher fetches artifacts from various sources and parses them into Artifacts
package artifact-fetcher

import (
	"context"

	"cloud.google.com/go/storage"
	"k8s.io/prow/kube"
	"k8s.io/prow/spyglass"
)

var client, err := storage.NewClient(context.Background(), option.WithoutAuthentication())

// A location storing the results of prow jobs
type JobSource interface {
	Artifacts() []spyglass.Artifact
	CanonicalLink() string
}

// A location in GCS where prow job-specific artifacts are stored
type GCSJobSource struct {
	JobSource
	bucket string
	jobPath string
}

// Gets all artifacts from a GCS job source
func (src *GCSJobSource) Artifacts() []spyglass.Artifact {
	// TODO download all artfacts under the job-bucket and parse them into artifacts
	// create a bucket handle from the client, get everything with the prefix: jobPath
	// parse them into artifacts, return them
}

// Gets a link to the location of job-specific artifacts in GCS
func (src *GCSJobSource) CanonicalLink() string {
	// TODO
}
