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

package jobs

// Started is used to mirror the started.json artifact
type Started struct {
	Timestamp int64             `json:"timestamp"`
	Node      string            `json:"node"`
	Repos     map[string]string `json:"repos"`
	Pull      string            `json:"pull"`
}

type Metadata struct {
	InfraCommit string            `json:"infra-commit"`
	Pod         string            `json:"pod"`
	Repo        string            `json:"repo"`
	RepoCommit  string            `json:"repo-commit"`
	Repos       map[string]string `json:"repos"`
}

// Finished is used to mirror the finished.json artifact
type Finished struct {
	Timestamp  int64  `json:"timestamp"`
	Version    string `json:"version"`
	JobVersion string `json:"job-version"`
	Passed     bool   `json:"passed"`
	Result     string `json:"result"`
	Metadata   `json:"metadata"`
}
