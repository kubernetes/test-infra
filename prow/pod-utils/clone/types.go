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

package clone

import (
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

// Record is a trace of what the desired
// git state was, what steps we took to get there,
// what our final state ended up being, and
// whether or not we were successful.
type Record struct {
	Refs     prowapi.Refs `json:"refs"`
	Commands []Command    `json:"commands"`
	Failed   bool         `json:"failed"`

	// FinalSHA is the SHA from ultimate state of a cloned ref
	// This is used to populate RepoCommit in started.json properly
	FinalSHA string `json:"final_sha,omitempty"`
}

// Command is a trace of a command executed
// while achieving the desired git state.
type Command struct {
	Command string `json:"command"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}
