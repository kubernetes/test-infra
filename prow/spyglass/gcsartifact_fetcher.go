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
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	pkgio "k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/spyglass/api"
)

var (
	// ErrCannotParseSource is returned by newGCSJobSource when an incorrectly formatted source string is passed
	ErrCannotParseSource = errors.New("could not create job source from provided source")
)

// GCSArtifactFetcher contains information used for fetching artifacts from GCS
type GCSArtifactFetcher struct {
	opener        pkgio.Opener
	useCookieAuth bool
}

// gcsJobSource is a location in GCS where Prow job-specific artifacts are stored. This implementation assumes
// Prow's native GCS upload format (treating GCS keys as a directory structure), and is not
// intended to support arbitrary GCS bucket upload formats.
type gcsJobSource struct {
	source     string
	linkPrefix string
	bucket     string
	jobPrefix  string
	jobName    string
	buildID    string
}

// NewGCSArtifactFetcher creates a new ArtifactFetcher with a real GCS Client
func NewGCSArtifactFetcher(opener pkgio.Opener, useCookieAuth bool) *GCSArtifactFetcher {
	return &GCSArtifactFetcher{
		opener:        opener,
		useCookieAuth: useCookieAuth,
	}
}

func fieldsForJob(src *gcsJobSource) logrus.Fields {
	return logrus.Fields{
		"jobPrefix": src.jobPath(),
	}
}

// newGCSJobSource creates a new gcsJobSource from a given storage provider bucket and jobPrefix. If no scheme is given we assume GS, e.g.:
// * test-bucket/logs/sig-flexing/example-ci-run/403 or
// * gs://test-bucket/logs/sig-flexing/example-ci-run/403
func newGCSJobSource(src string) (*gcsJobSource, error) {
	if !strings.Contains(src, "://") {
		src = "gs://" + src
	}
	gcsURL, err := url.Parse(src)
	if err != nil {
		return &gcsJobSource{}, ErrCannotParseSource
	}
	var object string
	if gcsURL.Path == "" {
		object = gcsURL.Path
	} else {
		object = gcsURL.Path[1:]
	}

	tokens := strings.FieldsFunc(object, func(c rune) bool { return c == '/' })
	if len(tokens) < 2 {
		return &gcsJobSource{}, ErrCannotParseSource
	}
	buildID := tokens[len(tokens)-1]
	name := tokens[len(tokens)-2]
	return &gcsJobSource{
		source:     src,
		linkPrefix: gcsURL.Scheme + "://",
		bucket:     gcsURL.Host,
		jobPrefix:  path.Clean(object) + "/",
		jobName:    name,
		buildID:    buildID,
	}, nil
}

// Artifacts lists all artifacts available for the given job source
// If no scheme is given we assume GS, e.g.:
// * test-bucket/logs/sig-flexing/example-ci-run/403 or
// * gs://test-bucket/logs/sig-flexing/example-ci-run/403
func (af *GCSArtifactFetcher) artifacts(ctx context.Context, key string) ([]string, error) {
	if !strings.Contains(key, "://") {
		key = "gs://" + key
	}
	src, err := newGCSJobSource(key)
	if err != nil {
		return nil, fmt.Errorf("Failed to get GCS job source from %s: %v", key, err)
	}

	listStart := time.Now()
	_, prefix := extractBucketPrefixPair(src.jobPath())
	artifacts := []string{}

	it, err := af.opener.Iterator(ctx, key, "")
	if err != nil {
		return artifacts, err
	}

	wait := []time.Duration{16, 32, 64, 128, 256, 256, 512, 512}
	for i := 0; ; {
		oAttrs, err := it.Next(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			logrus.WithFields(fieldsForJob(src)).WithError(err).Error("Error accessing GCS artifact.")
			if i >= len(wait) {
				return artifacts, fmt.Errorf("timed out: error accessing GCS artifact: %v", err)
			}
			time.Sleep((wait[i] + time.Duration(rand.Intn(10))) * time.Millisecond)
			i++
			continue
		}
		artifacts = append(artifacts, strings.TrimPrefix(oAttrs.Name, prefix))
		i = 0
	}
	logrus.WithField("duration", time.Since(listStart).String()).Infof("Listed %d artifacts.", len(artifacts))
	return artifacts, nil
}

func (af *GCSArtifactFetcher) signURL(ctx context.Context, key string) (string, error) {
	return af.opener.SignedURL(ctx, key, pkgio.SignedURLOptions{
		UseGSCookieAuth: af.useCookieAuth,
	})
}

type gcsArtifactHandle struct {
	pkgio.Opener
	Name string
}

func (h *gcsArtifactHandle) NewReader(ctx context.Context) (io.ReadCloser, error) {
	return h.Opener.Reader(ctx, h.Name)
}

func (h *gcsArtifactHandle) NewRangeReader(ctx context.Context, offset, length int64) (io.ReadCloser, error) {
	return h.Opener.RangeReader(ctx, h.Name, offset, length)
}

func (h *gcsArtifactHandle) Attrs(ctx context.Context) (pkgio.Attributes, error) {
	return h.Opener.Attributes(ctx, h.Name)
}

// Artifact constructs a GCS artifact from the given GCS bucket and key. Uses the golang GCS library
// to get read handles. If the artifactName is not a valid key in the bucket a handle will still be
// constructed and returned, but all read operations will fail (dictated by behavior of golang GCS lib).
// If no scheme is given we assume GS, e.g.:
// * test-bucket/logs/sig-flexing/example-ci-run/403 or
// * gs://test-bucket/logs/sig-flexing/example-ci-run/403
func (af *GCSArtifactFetcher) Artifact(ctx context.Context, key string, artifactName string, sizeLimit int64) (api.Artifact, error) {
	if !strings.Contains(key, "://") {
		key = "gs://" + key
	}
	src, err := newGCSJobSource(key)
	if err != nil {
		return nil, fmt.Errorf("failed to get GCS job source from %s: %v", key, err)
	}

	_, prefix := extractBucketPrefixPair(src.jobPath())
	objName := path.Join(prefix, artifactName)
	obj := &gcsArtifactHandle{Opener: af.opener, Name: fmt.Sprintf("%s%s/%s", src.linkPrefix, src.bucket, objName)}
	signedURL, err := af.signURL(ctx, fmt.Sprintf("%s%s/%s", src.linkPrefix, src.bucket, objName))
	if err != nil {
		return nil, err
	}
	return NewGCSArtifact(context.Background(), obj, signedURL, artifactName, sizeLimit), nil
}

func extractBucketPrefixPair(gcsPath string) (string, string) {
	split := strings.SplitN(gcsPath, "/", 2)
	return split[0], split[1]
}

// CanonicalLink gets a link to the location of job-specific artifacts in GCS
func (src *gcsJobSource) canonicalLink() string {
	return path.Join(src.linkPrefix, src.bucket, src.jobPrefix)
}

// JobPath gets the prefix to all artifacts in GCS in the job
func (src *gcsJobSource) jobPath() string {
	return fmt.Sprintf("%s/%s", src.bucket, src.jobPrefix)
}
