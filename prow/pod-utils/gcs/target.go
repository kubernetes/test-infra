/*
Copyright 2017 The Kubernetes Authors.

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

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
)

const (
	// PRLogs is the name of the directory we put PR logs in.
	PRLogs = "pr-logs"
	// NonPRLogs is the name of directory we put logs that are not PR logs in.
	NonPRLogs = "logs"
)

// PathForSpec determines the GCS path prefix for files uploaded
// for a specific job spec
func PathForSpec(spec *downwardapi.JobSpec, pathSegment RepoPathBuilder) string {
	switch spec.Type {
	case prowapi.PeriodicJob, prowapi.PostsubmitJob:
		return path.Join(NonPRLogs, spec.Job, spec.BuildID)
	case prowapi.PresubmitJob:
		return path.Join(PRLogs, "pull", pathSegment(spec.Refs.Org, spec.Refs.Repo), strconv.Itoa(spec.Refs.Pulls[0].Number), spec.Job, spec.BuildID)
	case prowapi.BatchJob:
		return path.Join(PRLogs, "pull", "batch", spec.Job, spec.BuildID)
	default:
		logrus.Fatalf("unknown job spec type: %v", spec.Type)
	}
	return ""
}

// AliasForSpec determines the GCS path aliases for a job spec
func AliasForSpec(spec *downwardapi.JobSpec) string {
	switch spec.Type {
	case prowapi.PeriodicJob, prowapi.PostsubmitJob, prowapi.BatchJob:
		return ""
	case prowapi.PresubmitJob:
		return path.Join(PRLogs, "directory", spec.Job, fmt.Sprintf("%s.txt", spec.BuildID))
	default:
		logrus.Fatalf("unknown job spec type: %v", spec.Type)
	}
	return ""
}

// LatestBuildForSpec determines the GCS path for storing the latest
// build id for a job. pathSegment can be nil so callers of this
// helper are not required to choose a path strategy but can still
// get back a result.
func LatestBuildForSpec(spec *downwardapi.JobSpec, pathSegment RepoPathBuilder) []string {
	var latestBuilds []string
	switch spec.Type {
	case prowapi.PeriodicJob, prowapi.PostsubmitJob:
		latestBuilds = append(latestBuilds, path.Join(NonPRLogs, spec.Job, "latest-build.txt"))
	case prowapi.PresubmitJob:
		latestBuilds = append(latestBuilds, path.Join(PRLogs, "directory", spec.Job, "latest-build.txt"))
		// Gubernator expects presubmit tests to upload latest-build.txt
		// under the PR-specific directory too.
		if pathSegment != nil {
			latestBuilds = append(latestBuilds, path.Join(PRLogs, "pull", pathSegment(spec.Refs.Org, spec.Refs.Repo), strconv.Itoa(spec.Refs.Pulls[0].Number), spec.Job, "latest-build.txt"))
		}
	case prowapi.BatchJob:
		latestBuilds = append(latestBuilds, path.Join(PRLogs, "directory", spec.Job, "latest-build.txt"))
	default:
		logrus.Errorf("unknown job spec type: %v", spec.Type)
		return nil
	}
	return latestBuilds
}

// RootForSpec determines the root GCS path for storing artifacts about
// the provided job.
func RootForSpec(spec *downwardapi.JobSpec) string {
	switch spec.Type {
	case prowapi.PeriodicJob, prowapi.PostsubmitJob:
		return path.Join(NonPRLogs, spec.Job)
	case prowapi.PresubmitJob, prowapi.BatchJob:
		return path.Join(PRLogs, "directory", spec.Job)
	default:
		logrus.Errorf("unknown job spec type: %v", spec.Type)
	}
	return ""
}

// RepoPathBuilder builds GCS path segments and embeds defaulting behavior
type RepoPathBuilder func(org, repo string) string

// NewLegacyRepoPathBuilder returns a builder that handles the legacy path
// encoding where a path will only contain an org or repo if they are non-default
func NewLegacyRepoPathBuilder(defaultOrg, defaultRepo string) RepoPathBuilder {
	return func(org, repo string) string {
		if org == defaultOrg {
			if repo == defaultRepo {
				return ""
			}
			return repo
		}
		// handle gerrit repo
		repo = strings.Replace(repo, "/", "_", -1)
		return fmt.Sprintf("%s_%s", org, repo)
	}
}

// NewSingleDefaultRepoPathBuilder returns a builder that handles the legacy path
// encoding where a path will contain org and repo for all but one default repo
func NewSingleDefaultRepoPathBuilder(defaultOrg, defaultRepo string) RepoPathBuilder {
	return func(org, repo string) string {
		if org == defaultOrg && repo == defaultRepo {
			return ""
		}
		// handle gerrit repo
		repo = strings.Replace(repo, "/", "_", -1)
		return fmt.Sprintf("%s_%s", org, repo)
	}
}

// NewExplicitRepoPathBuilder returns a builder that handles the path encoding
// where a path will always have an explicit "org_repo" path segment
func NewExplicitRepoPathBuilder() RepoPathBuilder {
	return func(org, repo string) string {
		// handle gerrit repo
		repo = strings.Replace(repo, "/", "_", -1)
		return fmt.Sprintf("%s_%s", org, repo)
	}
}
