/*
Copyright 2019 The Kubernetes Authors.

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

package metadata

// Started holds the started.json values of the build.
type Started struct {
	// Timestamp is UTC epoch seconds when the job started.
	Timestamp int64 `json:"timestamp"` // epoch seconds
	// Node holds the name of the machine that ran the job.
	Node string `json:"node,omitempty"`

	// Consider whether to keep the following:

	// Pull holds the PR number the primary repo is testing
	Pull string `json:"pull,omitempty"`
	// RepoVersion holds the git revision of the primary repo
	RepoVersion string `json:"repo-version,omitempty"`
	// Repos holds the RepoVersion of all commits checked out.
	Repos map[string]string `json:"repos,omitempty"` // {repo: branch_or_pull} map

	// Deprecated fields:

	// Metadata is deprecated, add to finished.json
	Metadata Metadata `json:"metadata,omitempty"` // TODO(fejta): remove

}

// Finished holds the finished.json values of the build
type Finished struct {
	// Timestamp is UTC epoch seconds when the job finished.
	// An empty value indicates an incomplete job.
	Timestamp *int64 `json:"timestamp,omitempty"`
	// Passed is true when the job completes successfully.
	Passed *bool `json:"passed"`
	// Metadata holds data computed by the job at runtime.
	// For example, the version of a binary downloaded at runtime
	Metadata Metadata `json:"metadata,omitempty"`

	// Consider whether to keep the following:

	// JobVersion provides a way for jobs to set/override the version at runtime.
	// Should take precedence over Started.RepoVersion whenever set.
	JobVersion string `json:"job-version,omitempty"`

	// Deprecated fields:

	// Result is deprecated, use Passed.
	Result string `json:"result,omitempty"` // TODO(fejta): remove
	// Revision is deprecated, use RepoVersion in started.json
	Revision string `json:"revision,omitempty"` // TODO(fejta): remove
}

// Metadata holds the finished.json values in the metadata key.
//
// Metadata values can either be string or string map of strings
//
// TODO(fejta): figure out which of these we want and document them
// Special values: infra-commit, repos, repo, repo-commit, links, others
type Metadata map[string]interface{}

// String returns the name key if its value is a string, and true if the key is present.
func (m Metadata) String(name string) (*string, bool) {
	if v, ok := m[name]; !ok {
		return nil, false
	} else if t, good := v.(string); !good {
		return nil, true
	} else {
		return &t, true
	}
}

// Meta returns the name key if its value is a child object, and true if they key is present.
func (m Metadata) Meta(name string) (*Metadata, bool) {
	if v, ok := m[name]; !ok {
		return nil, false
	} else if t, good := v.(Metadata); good {
		return &t, true
	} else if t, good := v.(map[string]interface{}); good {
		child := Metadata(t)
		return &child, true
	}
	return nil, true
}

// Keys returns an array of the keys of all valid Metadata values.
func (m Metadata) Keys() []string {
	ka := make([]string, 0, len(m))
	for k := range m {
		if _, ok := m.Meta(k); ok {
			ka = append(ka, k)
		}
	}
	return ka
}

// Strings returns the submap of values in the map that are strings.
func (m Metadata) Strings() map[string]string {
	bm := map[string]string{}
	for k, v := range m {
		if s, ok := v.(string); ok {
			bm[k] = s
		}
		// TODO(fejta): handle sub items
	}
	return bm
}
