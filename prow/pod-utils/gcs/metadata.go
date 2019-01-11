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

package gcs

// Started is used to mirror the started.json artifact.
// All fields in Started beside Timestamp are deprecated.
type Started struct {
	// Timestamp is the time in epoch seconds recorded when started.json is created.
	Timestamp int64 `json:"timestamp"`
	// Node is deprecated.
	Node string `json:"node"`
	// Repos is deprecated.
	Repos map[string]string `json:"repos"`
	// Pull is deprecated.
	Pull string `json:"pull"`
	// Revision is deprecated.
	Revision string `json:"revision"`
}

// Finished is used to mirror the finished.json artifact.
type Finished struct {
	// Timestamp is the time in epoch seconds recorded when finished.json is created.
	Timestamp int64 `json:"timestamp"`
	// Version is deprecated.
	// TODO: Version should be removed in favor of Revision.
	Version string `json:"version"`
	// JobVersion is deprecated.
	// TODO: JobVersion should be removed in favor of Revision.
	JobVersion string `json:"job-version"`
	// Revision identifies the revision of the code the build tested.
	// Revision can be either a SHA or a ref.
	// TODO: resolve https://github.com/kubernetes/test-infra/issues/10359
	Revision string `json:"revision,omitempty"`
	// Passed is true when the job has passed, else false.
	Passed bool `json:"passed"`
	// Result is the result of the job ("SUCCESS", "ABORTED", or "FAILURE").
	Result string `json:"result"`
	// Metadata is any additional metadata uploaded by the job.
	// Keys typically include infra-commit, repos, repo, repo-commit.
	// Values typically have type string or map[string]string.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}
