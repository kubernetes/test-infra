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
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/spyglass/api"
	"k8s.io/test-infra/prow/spyglass/lenses/common"
)

// ListArtifacts gets the names of all artifacts available from the given source
func (s *Spyglass) ListArtifacts(src string) ([]string, error) {
	keyType, key, err := splitSrc(src)
	if err != nil {
		return []string{}, fmt.Errorf("error parsing src: %v", err)
	}
	gcsKey := ""
	switch keyType {
	case gcsKeyType:
		gcsKey = key
	case prowKeyType:
		if gcsKey, err = s.prowToGCS(key); err != nil {
			logrus.Warningf("Failed to get gcs source for prow job: %v", err)
		}
	default:
		return nil, fmt.Errorf("Unrecognized key type for src: %v", src)
	}

	artifactNames, err := s.GCSArtifactFetcher.artifacts(gcsKey)
	logFound := false
	for _, name := range artifactNames {
		if name == "build-log.txt" {
			logFound = true
			break
		}
	}
	if err != nil || !logFound {
		artifactNames = append(artifactNames, "build-log.txt")
	}
	return artifactNames, nil
}

// KeyToJob takes a spyglass URL and returns the jobName and buildID.
func (*Spyglass) KeyToJob(src string) (jobName string, buildID string, err error) {
	src = strings.Trim(src, "/")
	parsed := strings.Split(src, "/")
	if len(parsed) < 2 {
		return "", "", fmt.Errorf("expected at least two path components in %q", src)
	}
	jobName = parsed[len(parsed)-2]
	buildID = parsed[len(parsed)-1]
	return jobName, buildID, nil
}

// prowToGCS returns the GCS key corresponding to the given prow key
func (s *Spyglass) prowToGCS(prowKey string) (string, error) {
	return common.ProwToGCS(s.JobAgent, s.config, prowKey)
}

// FetchArtifacts constructs and returns Artifact objects for each artifact name in the list.
// This includes getting any handles needed for read write operations, direct artifact links, etc.
func (s *Spyglass) FetchArtifacts(src string, podName string, sizeLimit int64, artifactNames []string) ([]api.Artifact, error) {
	return common.FetchArtifacts(s.JobAgent, s.config, s.GCSArtifactFetcher, s.PodLogArtifactFetcher, src, podName, sizeLimit, artifactNames)
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
