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
	"context"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/deck/jobs"
	"k8s.io/test-infra/prow/pod-utils/gcs"
	"k8s.io/test-infra/prow/spyglass/lenses"
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

	config   config.Getter
	testgrid *TestGrid

	*GCSArtifactFetcher
	*PodLogArtifactFetcher
}

// LensRequest holds data sent by a view
type LensRequest struct {
	Source    string   `json:"src"`
	Artifacts []string `json:"artifacts"`
}

// New constructs a Spyglass object from a JobAgent, a config.Agent, and a storage Client.
func New(ja *jobs.JobAgent, cfg config.Getter, c *storage.Client, ctx context.Context) *Spyglass {
	return &Spyglass{
		JobAgent:              ja,
		config:                cfg,
		PodLogArtifactFetcher: NewPodLogArtifactFetcher(ja),
		GCSArtifactFetcher:    NewGCSArtifactFetcher(c),
		testgrid: &TestGrid{
			conf:   cfg,
			client: c,
			ctx:    ctx,
		},
	}
}

func (sg *Spyglass) Start() {
	sg.testgrid.Start()
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
		iconf := ls[i].Config()
		jconf := ls[j].Config()
		iname := iconf.Name
		jname := jconf.Name
		pi := iconf.Priority
		pj := jconf.Priority
		if pi == pj {
			return iname < jname
		}
		return pi < pj
	})
	return ls
}

func (s *Spyglass) ResolveSymlink(src string) (string, error) {
	keyType, key, err := splitSrc(src)
	if err != nil {
		return "", fmt.Errorf("error parsing src: %v", src)
	}
	switch keyType {
	case prowKeyType:
		return src, nil // prowjob keys cannot be symlinks.
	case gcsKeyType:
		bucketName, prefix := extractBucketPrefixPair(strings.TrimSuffix(key, "/"))
		bkt := s.client.Bucket(bucketName)
		obj := bkt.Object(prefix + ".txt")
		reader, err := obj.NewReader(context.Background())
		if err != nil {
			return src, nil
		}
		// Avoid using ReadAll here to prevent an attacker forcing us to read a giant file into memory.
		bytes := make([]byte, 4096) // assume we won't get more than 4 kB of symlink to read
		n, err := reader.Read(bytes)
		if err != nil {
			return "", fmt.Errorf("failed to read symlink file (which does seem to exist): %v", err)
		}
		if n == len(bytes) {
			return "", fmt.Errorf("symlink destination exceeds length limit of %d bytes", len(bytes) - 1)
		}
		u, err := url.Parse(string(bytes[:n]))
		if err != nil {
			return "", fmt.Errorf("failed to parse URL: %v", err)
		}
		if u.Scheme != "gs" {
			return "", fmt.Errorf("expected gs:// symlink, got '%s://'", u.Scheme)
		}
		return path.Join(gcsKeyType, u.Host, u.Path), nil
	default:
		return "", fmt.Errorf("unknown src key type %q", keyType)
	}
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
		if logType == gcs.NonPRLogs {
			return path.Dir(key), nil
		} else if logType == gcs.PRLogs {
			return path.Join(bktName, gcs.PRLogs, "directory", jobName), nil
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
		if job.Spec.Type == prowapi.PresubmitJob {
			return path.Join(bktName, gcs.PRLogs, "directory", jobName), nil
		}
		return path.Join(bktName, gcs.NonPRLogs, jobName), nil
	default:
		return "", fmt.Errorf("unrecognized key type for src: %v", src)
	}
}

// RunPath returns the path to the GCS directory for the job run specified in src.
func (s *Spyglass) RunPath(src string) (string, error) {
	src = strings.TrimSuffix(src, "/")
	keyType, key, err := splitSrc(src)
	if err != nil {
		return "", fmt.Errorf("error parsing src: %v", src)
	}
	switch keyType {
	case gcsKeyType:
		return key, nil
	case prowKeyType:
		return s.prowToGCS(key)
	default:
		return "", fmt.Errorf("unrecognized key type for src: %v", src)
	}
}

// RunToPR returns the (org, repo, pr#) tuple referenced by the provided src.
// Returns an error if src does not reference a job with an associated PR.
func (s *Spyglass) RunToPR(src string) (string, string, int, error) {
	src = strings.TrimSuffix(src, "/")
	keyType, key, err := splitSrc(src)
	if err != nil {
		return "", "", 0, fmt.Errorf("error parsing src: %v", src)
	}
	split := strings.Split(key, "/")
	if len(split) < 2 {
		return "", "", 0, fmt.Errorf("expected more URL components in %q", src)
	}
	switch keyType {
	case gcsKeyType:
		// In theory, we could derive this information without trying to parse the URL by instead fetching the
		// data from uploaded artifacts. In practice, that would not be a great solution: it would require us
		// to try pulling two different metadata files (one for bootstrap and one for podutils), then parse them
		// in unintended ways to infer the original PR. Aside from this being some work to do, it's also slow: we would
		// like to be able to always answer this request without needing to call out to GCS.
		logType := split[1]
		if logType == gcs.NonPRLogs {
			return "", "", 0, fmt.Errorf("not a PR URL: %q", key)
		} else if logType == gcs.PRLogs {
			prNum, err := strconv.Atoi(split[len(split)-3])
			if err != nil {
				return "", "", 0, fmt.Errorf("couldn't parse PR number %q in %q: %v", split[5], key, err)
			}
			// We don't actually attempt to look up the job's own configuration.
			// In practice, this shouldn't matter: we only want to read DefaultOrg and DefaultRepo, and overriding those
			// per job would probably be a bad idea (indeed, not even the tests try to do this).
			// This decision should probably be revisited if we ever want other information from it.
			if s.config().Plank.DefaultDecorationConfig == nil || s.config().Plank.DefaultDecorationConfig.GCSConfiguration == nil {
				return "", "", 0, fmt.Errorf("couldn't look up a GCS configuration")
			}
			c := s.config().Plank.DefaultDecorationConfig.GCSConfiguration
			// Assumption: we can derive the type of URL from how many components it has, without worrying much about
			// what the actual path configuration is.
			switch len(split) {
			case 7:
				// In this case we suffer an ambiguity when using 'path_strategy: legacy', and the repo
				// is in the default repo, and the repo name contains an underscore.
				// Currently this affects no actual repo. Hopefully we will soon deprecate 'legacy' and
				// ensure it never does.
				parts := strings.SplitN(split[3], "_", 2)
				if len(parts) == 1 {
					return c.DefaultOrg, parts[0], prNum, nil
				}
				return parts[0], parts[1], prNum, nil
			case 6:
				return c.DefaultOrg, c.DefaultRepo, prNum, nil
			default:
				return "", "", 0, fmt.Errorf("didn't understand the GCS URL %q", key)
			}
		} else {
			return "", "", 0, fmt.Errorf("unknown log type: %q", logType)
		}
	case prowKeyType:
		if len(split) < 2 {
			return "", "", 0, fmt.Errorf("invalid key %s: expected <job-name>/<build-id>", key)
		}
		jobName := split[0]
		buildID := split[1]
		job, err := s.jobAgent.GetProwJob(jobName, buildID)
		if err != nil {
			return "", "", 0, fmt.Errorf("failed to get prow job from src %q: %v", key, err)
		}
		if job.Spec.Refs == nil || len(job.Spec.Refs.Pulls) == 0 {
			return "", "", 0, fmt.Errorf("no PRs on job %q", job.Name)
		}
		return job.Spec.Refs.Org, job.Spec.Refs.Repo, job.Spec.Refs.Pulls[0].Number, nil
	default:
		return "", "", 0, fmt.Errorf("unrecognized key type for src: %v", src)
	}
}

// TestGridLink returns a link to a relevant TestGrid tab for the given source string.
// Because there is a one-to-many mapping from job names to TestGrid tabs, the returned tab
// link may not be deterministic.
func (sg *Spyglass) TestGridLink(src string) (string, error) {
	if !sg.testgrid.Ready() || sg.config().Deck.Spyglass.TestGridRoot == "" {
		return "", fmt.Errorf("testgrid is not configured")
	}

	src = strings.TrimSuffix(src, "/")
	split := strings.Split(src, "/")
	if len(split) < 2 {
		return "", fmt.Errorf("couldn't parse src %q", src)
	}
	jobName := split[len(split)-2]
	q, err := sg.testgrid.FindQuery(jobName)
	if err != nil {
		return "", fmt.Errorf("failed to find query: %v", err)
	}
	return sg.config().Deck.Spyglass.TestGridRoot + q, nil
}
