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
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/GoogleCloudPlatform/testgrid/metadata"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/deck/jobs"
	pkgio "k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/io/providers"
	"k8s.io/test-infra/prow/pod-utils/gcs"
	"k8s.io/test-infra/prow/spyglass/api"
	"k8s.io/test-infra/prow/spyglass/lenses"
)

// Key types specify the way Spyglass will fetch artifact handles
const (
	gcsKeyType  = api.GCSKeyType
	prowKeyType = api.ProwKeyType
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

	*StorageArtifactFetcher
	*PodLogArtifactFetcher
}

// LensRequest holds data sent by a view
type LensRequest struct {
	Source    string   `json:"src"`
	Index     int      `json:"index"`
	Artifacts []string `json:"artifacts"`
}

// ExtraLink represents an extra link to be added to the Spyglass page.
type ExtraLink struct {
	Name        string
	Description string
	URL         string
}

// New constructs a Spyglass object from a JobAgent, a config.Agent, and a storage Client.
func New(ctx context.Context, ja *jobs.JobAgent, cfg config.Getter, opener pkgio.Opener, useCookieAuth bool) *Spyglass {
	return &Spyglass{
		JobAgent:               ja,
		config:                 cfg,
		PodLogArtifactFetcher:  NewPodLogArtifactFetcher(ja),
		StorageArtifactFetcher: NewStorageArtifactFetcher(opener, cfg, useCookieAuth),
		testgrid: &TestGrid{
			conf:   cfg,
			opener: opener,
			ctx:    ctx,
		},
	}
}

func (sg *Spyglass) Start() {
	sg.testgrid.Start()
}

type LensConfig interface {
	Config() lenses.LensConfig
}

// Lenses gets all views of all artifact files matching each regexp with a registered lens
func (sg *Spyglass) Lenses(lensConfigIndexes []int) (orderedIndexes []int, lensMap map[int]LensConfig) {
	type ld struct {
		lens  LensConfig
		index int
	}
	var ls []ld
	for _, lensIndex := range lensConfigIndexes {
		lfc := sg.config().Deck.Spyglass.Lenses[lensIndex]
		lens, err := getLensConfig(lfc)
		if err != nil {
			logrus.WithField("lensName", lfc.Lens.Name).WithError(err).Error("Could not find artifact lens")
		} else {
			ls = append(ls, ld{lens, lensIndex})
		}
	}
	// Make sure lenses are rendered in order by ascending priority
	sort.Slice(ls, func(i, j int) bool {
		iconf := ls[i].lens.Config()
		jconf := ls[j].lens.Config()
		iname := iconf.Name
		jname := jconf.Name
		pi := iconf.Priority
		pj := jconf.Priority
		if pi == pj {
			return iname < jname
		}
		return pi < pj
	})

	lensMap = map[int]LensConfig{}
	for _, l := range ls {
		orderedIndexes = append(orderedIndexes, l.index)
		lensMap[l.index] = l.lens
	}

	return orderedIndexes, lensMap
}

type lensConfigWrapper struct {
	lensConfig lenses.LensConfig
}

func (l lensConfigWrapper) Config() lenses.LensConfig {
	return l.lensConfig
}

func getLensConfig(lensFileConfig config.LensFileConfig) (LensConfig, error) {
	lens, err := lenses.GetLens(lensFileConfig.Lens.Name)
	if err != nil && err != lenses.ErrInvalidLensName {
		return nil, err
	}
	if err == nil {
		return lens, nil
	}
	// we couldn't find a local lens (err==lenses.ErrInvalidLensName) so let's search for a remote lens
	if lensFileConfig.RemoteConfig != nil {
		lc := lenses.LensConfig{
			Name:  lensFileConfig.Lens.Name,
			Title: lensFileConfig.RemoteConfig.Title,
		}
		if lensFileConfig.RemoteConfig.Priority != nil {
			lc.Priority = *lensFileConfig.RemoteConfig.Priority
		}
		if lensFileConfig.RemoteConfig.HideTitle != nil {
			lc.HideTitle = *lensFileConfig.RemoteConfig.HideTitle
		}
		return lensConfigWrapper{lc}, nil
	}
	return nil, fmt.Errorf("could not find lens")
}

func (sg *Spyglass) ResolveSymlink(src string) (string, error) {
	src = strings.TrimSuffix(src, "/")
	keyType, key, err := splitSrc(src)
	if err != nil {
		return "", fmt.Errorf("error parsing src: %v", src)
	}
	switch keyType {
	case prowKeyType:
		return src, nil // prowjob keys cannot be symlinks.
	default:
		if keyType == api.GCSKeyType {
			keyType = providers.GS
		}
		reader, err := sg.opener.Reader(context.TODO(), fmt.Sprintf("%s://%s.txt", keyType, key))
		if err != nil {
			if pkgio.IsNotExist(err) {
				return fmt.Sprintf("%s/%s", keyType, key), nil
			}
			return "", err
		}
		// Avoid using ReadAll here to prevent an attacker forcing us to read a giant file into memory.
		bytes := make([]byte, 4096) // assume we won't get more than 4 kB of symlink to read
		n, err := reader.Read(bytes)
		if err != nil && err != io.EOF {
			return "", fmt.Errorf("failed to read symlink file (which does seem to exist): %v", err)
		}
		if n == len(bytes) {
			return "", fmt.Errorf("symlink destination exceeds length limit of %d bytes", len(bytes)-1)
		}
		u, err := url.Parse(string(bytes[:n]))
		if err != nil {
			return "", fmt.Errorf("failed to parse URL: %v", err)
		}
		return path.Join(keyType, u.Host, u.Path), nil
	}
}

// JobPath returns a link to the directory for the job specified in src
func (sg *Spyglass) JobPath(src string) (string, error) {
	src = strings.TrimSuffix(src, "/")
	keyType, key, err := splitSrc(src)
	if err != nil {
		return "", fmt.Errorf("error parsing src: %v", src)
	}
	split := strings.Split(key, "/")
	switch keyType {
	case prowKeyType:
		if len(split) < 2 {
			return "", fmt.Errorf("invalid key %s: expected <job-name>/<build-id>", key)
		}
		jobName := split[0]
		buildID := split[1]
		job, err := sg.jobAgent.GetProwJob(jobName, buildID)
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
		if strings.Contains(bktName, "://") {
			// handle :// in bucket name if necessary
			bktName = strings.Replace(bktName, "://", "/", 1)
		} else {
			// fallback to gs/ if bucket name is given without storage type
			bktName = fmt.Sprintf("%s/%s", providers.GS, bktName)
		}
		if job.Spec.Type == prowv1.PresubmitJob {
			return path.Join(bktName, gcs.PRLogs, "directory", jobName), nil
		}
		return path.Join(bktName, gcs.NonPRLogs, jobName), nil
	default:
		if keyType == gcsKeyType {
			keyType = providers.GS
		}
		if len(split) < 4 {
			return "", fmt.Errorf("invalid key %s: expected <bucket-name>/<log-type>/.../<job-name>/<build-id>", key)
		}
		// see https://github.com/kubernetes/test-infra/tree/master/gubernator
		bktName := split[0]
		logType := split[1]
		jobName := split[len(split)-2]
		if logType == gcs.NonPRLogs {
			return path.Join(keyType, path.Dir(key)), nil
		} else if logType == gcs.PRLogs {
			return path.Join(keyType, bktName, gcs.PRLogs, "directory", jobName), nil
		}
		return "", fmt.Errorf("unrecognized GCS key: %s", key)
	}
}

// ProwJobName returns a link to the YAML for the job specified in src.
// If no job is found, it returns an empty string and nil error.
func (sg *Spyglass) ProwJobName(src string) (string, error) {
	src = strings.TrimSuffix(src, "/")
	keyType, key, err := splitSrc(src)
	if err != nil {
		return "", fmt.Errorf("error parsing src: %v", src)
	}
	split := strings.Split(key, "/")
	var jobName string
	var buildID string
	switch keyType {
	case prowKeyType:
		if len(split) < 2 {
			return "", fmt.Errorf("invalid key %s: expected <job-name>/<build-id>", key)
		}
		jobName = split[0]
		buildID = split[1]
	default:
		if len(split) < 4 {
			return "", fmt.Errorf("invalid key %s: expected <bucket-name>/<log-type>/.../<job-name>/<build-id>", key)
		}
		jobName = split[len(split)-2]
		buildID = split[len(split)-1]
	}
	job, err := sg.jobAgent.GetProwJob(jobName, buildID)
	if err != nil {
		if jobs.IsErrProwJobNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return job.Name, nil
}

// RunPath returns the path to the directory for the job run specified in src.
func (sg *Spyglass) RunPath(src string) (string, error) {
	src = strings.TrimSuffix(src, "/")
	keyType, key, err := splitSrc(src)
	if err != nil {
		return "", fmt.Errorf("error parsing src: %v", src)
	}
	switch keyType {
	case prowKeyType:
		_, gcsKey, err := sg.prowToGCS(key)
		if err != nil {
			return "", err
		}
		return gcsKey, nil
	default:
		return key, nil
	}
}

// RunToPR returns the (org, repo, pr#) tuple referenced by the provided src.
// Returns an error if src does not reference a job with an associated PR.
func (sg *Spyglass) RunToPR(src string) (string, string, int, error) {
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
	case prowKeyType:
		if len(split) < 2 {
			return "", "", 0, fmt.Errorf("invalid key %s: expected <job-name>/<build-id>", key)
		}
		jobName := split[0]
		buildID := split[1]
		job, err := sg.jobAgent.GetProwJob(jobName, buildID)
		if err != nil {
			return "", "", 0, fmt.Errorf("failed to get prow job from src %q: %v", key, err)
		}
		if job.Spec.Refs == nil || len(job.Spec.Refs.Pulls) == 0 {
			return "", "", 0, fmt.Errorf("no PRs on job %q", job.Name)
		}
		return job.Spec.Refs.Org, job.Spec.Refs.Repo, job.Spec.Refs.Pulls[0].Number, nil
	default:
		// In theory, we could derive this information without trying to parse the URL by instead fetching the
		// data from uploaded artifacts. In practice, that would not be a great solution: it would require us
		// to try pulling two different metadata files (one for bootstrap and one for podutils), then parse them
		// in unintended ways to infer the original PR. Aside from this being some work to do, it's also slow: we would
		// like to be able to always answer this request without needing to call out to GCS.
		logType := split[1]
		if logType == gcs.NonPRLogs {
			return "", "", 0, fmt.Errorf("not a PR URL: %q", key)
		} else if logType == gcs.PRLogs {
			if len(split) < 3 {
				return "", "", 0, fmt.Errorf("malformed %s key %q should have at least three components", gcs.PRLogs, key)
			}
			prNumStr := split[len(split)-3]
			prNum, err := strconv.Atoi(prNumStr)
			if err != nil {
				return "", "", 0, fmt.Errorf("couldn't parse PR number %q in %q: %v", prNumStr, key, err)
			}
			// We don't actually attempt to look up the job's own configuration.
			// In practice, this shouldn't matter: we only want to read DefaultOrg and DefaultRepo, and overriding those
			// per job would probably be a bad idea (indeed, not even the tests try to do this).
			// This decision should probably be revisited if we ever want other information from it.
			// TODO (droslean): we should get the default decoration config depending on the org/repo.
			if sg.config().Plank.DefaultDecorationConfigs["*"] == nil || sg.config().Plank.DefaultDecorationConfigs["*"].GCSConfiguration == nil {
				return "", "", 0, fmt.Errorf("couldn't look up a GCS configuration")
			}
			c := sg.config().Plank.DefaultDecorationConfigs["*"].GCSConfiguration
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
	}
}

// ExtraLinks fetches started.json and extracts links from metadata.links.
func (sg *Spyglass) ExtraLinks(ctx context.Context, src string) ([]ExtraLink, error) {
	artifacts, err := sg.FetchArtifacts(ctx, src, "", 1000000, []string{prowv1.StartedStatusFile})
	// Failing to parse src, that's an error.
	if err != nil {
		return nil, err
	}

	// Failing to find started.json is okay, just return nothing quietly.
	if len(artifacts) == 0 {
		logrus.Debugf("Failed to find started.json while looking for extra links.")
		return nil, nil
	}
	// Failing to read an artifact we already know to exist shouldn't happen, so that's an error.
	content, err := artifacts[0].ReadAll()
	if err != nil {
		return nil, err
	}
	// Being unable to parse a successfully fetched started.json correctly is also an error.
	started := metadata.Started{}
	if err := json.Unmarshal(content, &started); err != nil {
		return nil, err
	}
	// Not having any links is fine.
	links, ok := started.Metadata.Meta("links")
	if !ok {
		return nil, nil
	}
	extraLinks := make([]ExtraLink, 0, len(*links))
	for _, name := range links.Keys() {
		m, ok := links.Meta(name)
		if !ok {
			// This should never happen, because Keys() should only return valid Metas.
			logrus.Debugf("Got bad link key %q from %s, but that should be impossible.", name, artifacts[0].CanonicalLink())
			continue
		}
		s := m.Strings()
		link := ExtraLink{
			Name:        name,
			URL:         s["url"],
			Description: s["description"],
		}
		if link.URL == "" || link.Name == "" {
			continue
		}
		extraLinks = append(extraLinks, link)
	}
	return extraLinks, nil
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
