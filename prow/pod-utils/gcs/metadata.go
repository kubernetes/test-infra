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
type Started struct {
	Timestamp int64             `json:"timestamp"`
	Node      string            `json:"node"`
	Repos     map[string]string `json:"repos"`
	Pull      string            `json:"pull"`
	Revision  string            `json:"revision"`
}

// Finished is used to mirror the finished.json artifact.
type Finished struct {
	Timestamp  int64                  `json:"timestamp"`
	Version    string                 `json:"version"`
	JobVersion string                 `json:"job-version"`
	Passed     bool                   `json:"passed"`
	Result     string                 `json:"result"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Revision   string                 `json:"revision,omitempty"`
}
