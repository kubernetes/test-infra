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

package kube

const (
	// CreatedByProw is added on resources created by prow.
	// Since resources often live in another cluster/namespace,
	// the k8s garbage collector would immediately delete these
	// resources
	// TODO: Namespace this label.
	CreatedByProw = "created-by-prow"
	// ProwJobTypeLabel is added in resources created by prow and
	// carries the job type (presubmit, postsubmit, periodic, batch)
	// that the pod is running.
	ProwJobTypeLabel = "prow.k8s.io/type"
	// ProwJobIDLabel is added in resources created by prow and
	// carries the ID of the ProwJob that the pod is fulfilling.
	// We also name resources after the ProwJob that spawned them but
	// this allows for multiple resources to be linked to one
	// ProwJob.
	ProwJobIDLabel = "prow.k8s.io/id"
	// ProwJobAnnotation is added in resources created by prow and
	// carries the name of the job that the pod is running. Since
	// job names can be arbitrarily long, this is added as
	// an annotation instead of a label.
	ProwJobAnnotation = "prow.k8s.io/job"
	// OrgLabel is added in resources created by prow and
	// carries the org associated with the job, eg kubernetes-sigs.
	OrgLabel = "prow.k8s.io/refs.org"
	// RepoLabel is added in resources created by prow and
	// carries the repo associated with the job, eg test-infra
	RepoLabel = "prow.k8s.io/refs.repo"
	// PullLabel is added in resources created by prow and
	// carries the PR number associated with the job, eg 321.
	PullLabel = "prow.k8s.io/refs.pull"
)
