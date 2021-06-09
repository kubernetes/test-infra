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
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/io/providers"
	"k8s.io/test-infra/prow/spyglass/api"
	"k8s.io/test-infra/prow/spyglass/lenses/common"
)

// ListArtifacts gets the names of all artifacts available from the given source
func (s *Spyglass) ListArtifacts(ctx context.Context, src string) ([]string, error) {
	keyType, key, err := splitSrc(src)
	if err != nil {
		return []string{}, fmt.Errorf("error parsing src: %w", err)
	}
	gcsKey := ""
	switch keyType {
	case prowKeyType:
		storageProvider, key, err := s.prowToGCS(key)
		if err != nil {
			logrus.Warningf("Failed to get gcs source for prow job: %v", err)
		}
		gcsKey = fmt.Sprintf("%s://%s", storageProvider, key)
	default:
		if keyType == gcsKeyType {
			keyType = providers.GS
		}
		gcsKey = fmt.Sprintf("%s://%s", keyType, key)
	}

	artifactNames, err := s.StorageArtifactFetcher.artifacts(ctx, gcsKey)
	// Don't care errors that are not supposed logged as http errors, for example
	// context cancelled error due to user cancelled request.
	if err != nil && err != context.Canceled {
		if config.IsNotAllowedBucketError(err) {
			logrus.WithError(err).Debug("error retrieving artifact names from gcs storage")
		} else {
			logrus.WithError(err).Warn("error retrieving artifact names from gcs storage")
		}
	}

	artifactNamesSet := sets.NewString(artifactNames...)

	jobName, buildID, err := common.KeyToJob(src)
	if err != nil {
		return artifactNamesSet.List(), fmt.Errorf("error parsing src: %w", err)
	}

	job, err := s.jobAgent.GetProwJob(jobName, buildID)
	if err != nil {
		// we don't return the error because we assume that if we cannot get the prowjob from the jobAgent,
		// then we must already have all the build-logs in gcs
		logrus.Infof("unable to get prowjob from Pod: %v", err)
		return artifactNamesSet.List(), nil
	}

	jobContainers := job.Spec.PodSpec.Containers

	for _, container := range jobContainers {
		logName := singleLogName
		if len(jobContainers) > 1 {
			logName = fmt.Sprintf("%s-%s", container.Name, singleLogName)
		}
		artifactNamesSet.Insert(logName)
	}

	return artifactNamesSet.List(), nil
}

// prowToGCS returns the GCS key corresponding to the given prow key
func (s *Spyglass) prowToGCS(prowKey string) (string, string, error) {
	return common.ProwToGCS(s.JobAgent, s.config, prowKey)
}

// FetchArtifacts constructs and returns Artifact objects for each artifact name in the list.
// This includes getting any handles needed for read write operations, direct artifact links, etc.
func (s *Spyglass) FetchArtifacts(ctx context.Context, src string, podName string, sizeLimit int64, artifactNames []string) ([]api.Artifact, error) {
	return common.FetchArtifacts(ctx, s.JobAgent, s.config, s.StorageArtifactFetcher, s.PodLogArtifactFetcher, src, podName, sizeLimit, artifactNames)
}

func splitSrc(src string) (keyType, key string, err error) {
	split := strings.SplitN(src, "/", 2)
	if len(split) < 2 {
		err = fmt.Errorf("invalid src %s: expected <key-type>/<key>", src)
		return
	}
	keyType = split[0]
	key = split[1]
	return
}
