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

// Package spyglass creates views for Prow job artifacts.
package spyglass

import (
	"cloud.google.com/go/storage"
	"fmt"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/deck/jobs"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/spyglass/lenses"
	"path"
	"sort"
	"strings"
)

// Key types specify the way Spyglass will fetch artifact handles
const (
	gcsKeyType  = "gcs"
	prowKeyType = "prowjob"
)

// Spyglass records which sets of artifacts need views for a Prow job. The metaphor
// can be understood as follows: A spyglass receives light from a source through
// an eyepiece, which has a lens that ultimately presents a view of the light source
// to the observer. Spyglass receives light (artifacts) via a
// source (src) through the eyepiece (Eyepiece) and presents the view (what you see
// in your browser) via a lens (Lens).
type Spyglass struct {
	// JobAgent contains information about the current jobs in deck
	JobAgent *jobs.JobAgent

	// ConfigAgent contains information about the prow configuration
	ConfigAgent configAgent

	*GCSArtifactFetcher
	*PodLogArtifactFetcher
}

// This interface matches config.Agent and exists for the purpose of unit tests.
type configAgent interface {
	Config() *config.Config
}

// LensRequest holds data sent by a view
type LensRequest struct {
	Source    string   `json:"src"`
	Artifacts []string `json:"artifacts"`
}

// New constructs a Spyglass object from a JobAgent, a config.Agent, and a storage Client.
func New(ja *jobs.JobAgent, conf configAgent, c *storage.Client) *Spyglass {
	return &Spyglass{
		JobAgent:              ja,
		ConfigAgent:           conf,
		PodLogArtifactFetcher: NewPodLogArtifactFetcher(ja),
		GCSArtifactFetcher:    NewGCSArtifactFetcher(c),
	}
}

// Lenses gets all views of all artifact files matching each regexp with a registered lens
func (s *Spyglass) Lenses(matchCache map[string][]string) []lenses.Lens {
	ls := []lenses.Lens{}
	for lensName, matches := range matchCache {
		if len(matches) == 0 {
			continue
		}
		lens, err := lenses.GetLens(lensName)
		if err != nil {
			logrus.WithField("lensName", lens).WithError(err).Error("Could not find artifact lens")
		} else {
			ls = append(ls, lens)
		}
	}
	// Make sure lenses are rendered in order by ascending priority
	sort.Slice(ls, func(i, j int) bool {
		iname := ls[i].Name()
		jname := ls[j].Name()
		pi := ls[i].Priority()
		pj := ls[j].Priority()
		if pi == pj {
			return iname < jname
		}
		return pi < pj
	})
	return ls
}

// JobPath returns a link to the GCS directory for the job specified in src
func (s *Spyglass) JobPath(src string) (string, error) {
	src = strings.TrimSuffix(src, "/")
	keyType, key, err := splitSrc(src)
	if err != nil {
		return "", fmt.Errorf("error parsing src: %v", src)
	}
	split := strings.Split(key, "/")
	switch keyType {
	case gcsKeyType:
		if len(split) < 4 {
			return "", fmt.Errorf("invalid key %s: expected <bucket-name>/<log-type>/.../<job-name>/<build-id>", key)
		}
		// see https://github.com/kubernetes/test-infra/tree/master/gubernator
		bktName := split[0]
		logType := split[1]
		jobName := split[len(split)-2]
		if logType == "logs" {
			return path.Dir(key), nil
		} else if logType == "pr-logs" {
			return path.Join(bktName, "pr-logs/directory", jobName), nil
		}
		return "", fmt.Errorf("unrecognized GCS key: %s", key)
	case prowKeyType:
		if len(split) < 2 {
			return "", fmt.Errorf("invalid key %s: expected <job-name>/<build-id>", key)
		}
		jobName := split[0]
		buildID := split[1]
		job, err := s.jobAgent.GetProwJob(jobName, buildID)
		if err != nil {
			return "", fmt.Errorf("failed to get prow job from src %q: %v", key, err)
		}
		if job.Spec.DecorationConfig == nil {
			return "", fmt.Errorf("failed to locate GCS upload bucket for %s: job is undecorated", jobName)
		}
		if job.Spec.DecorationConfig.GCSConfiguration == nil {
			return "", fmt.Errorf("failed to locate GCS upload bucket for %s: missing GCS configuration", jobName)
		}
		bktName := job.Spec.DecorationConfig.GCSConfiguration.Bucket
		if job.Spec.Type == kube.PresubmitJob {
			return path.Join(bktName, "pr-logs/directory", jobName), nil
		}
		return path.Join(bktName, "logs", jobName), nil
	default:
		return "", fmt.Errorf("unrecognized key type for src: %v", src)
	}
}
