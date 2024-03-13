/*
Copyright 2024 The Kubernetes Authors.

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

package config

type Scheduler struct {
	Enabled bool `json:"enabled,omitempty"`

	// Scheduling strategies
	Failover *FailoverScheduling `json:"failover,omitempty"`
}

// FailoverScheduling is a configuration for the Failover scheduling strategy
type FailoverScheduling struct {
	// ClusterMappings maps a cluster to another one. It is used when we
	// want to schedule a ProJob to a cluster other than the one it was
	// configured to in the first place.
	ClusterMappings map[string]string `json:"mappings,omitempty"`
}
