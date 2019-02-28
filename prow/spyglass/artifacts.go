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
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/spyglass/lenses"
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
	jobName, buildID, err := s.KeyToJob(prowKey)
	if err != nil {
		return "", fmt.Errorf("could not get GCS src: %v", err)
	}

	job, err := s.jobAgent.GetProwJob(jobName, buildID)
	if err != nil {
		return "", fmt.Errorf("Failed to get prow job from src %q: %v", prowKey, err)
	}

	url := job.Status.URL
	prefix := s.config().Plank.JobURLPrefix
	if !strings.HasPrefix(url, prefix) {
		return "", fmt.Errorf("unexpected job URL %q when finding GCS path: expected something starting with %q", url, prefix)
	}
	return url[len(prefix):], nil
}

// FetchArtifacts constructs and returns Artifact objects for each artifact name in the list.
// This includes getting any handles needed for read write operations, direct artifact links, etc.
func (s *Spyglass) FetchArtifacts(src string, podName string, sizeLimit int64, artifactNames []string) ([]lenses.Artifact, error) {
	artStart := time.Now()
	arts := []lenses.Artifact{}
	keyType, key, err := splitSrc(src)
	if err != nil {
		return arts, fmt.Errorf("error parsing src: %v", err)
	}
	jobName, buildID, err := s.KeyToJob(src)
	if err != nil {
		return arts, fmt.Errorf("could not derive job: %v", err)
	}
	gcsKey := ""
	switch keyType {
	case gcsKeyType:
		gcsKey = strings.TrimSuffix(key, "/")
	case prowKeyType:
		if gcsKey, err = s.prowToGCS(key); err != nil {
			logrus.Warningln(err)
		}
	default:
		return nil, fmt.Errorf("invalid src: %v", src)
	}

	podLogNeeded := false
	for _, name := range artifactNames {
		art, err := s.GCSArtifactFetcher.artifact(gcsKey, name, sizeLimit)
		if err == nil {
			// Actually try making a request, because calling GCSArtifactFetcher.artifact does no I/O.
			// (these files are being explicitly requested and so will presumably soon be accessed, so
			// the extra network I/O should not be too problematic).
			_, err = art.Size()
		}
		if err != nil {
			if name == "build-log.txt" {
				podLogNeeded = true
			} else {
				logrus.Errorf("Failed to fetch artifact %s: %v", name, err)
			}
			continue
		}
		arts = append(arts, art)
	}

	if podLogNeeded {
		art, err := s.PodLogArtifactFetcher.artifact(jobName, buildID, sizeLimit)
		if err != nil {
			logrus.Errorf("Failed to fetch pod log: %v", err)
		} else {
			arts = append(arts, art)
		}
	}

	logrus.WithField("duration", time.Since(artStart)).Infof("Retrieved artifacts for %v", src)
	return arts, nil
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
