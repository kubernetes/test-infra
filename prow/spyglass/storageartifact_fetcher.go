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

	"k8s.io/test-infra/prow/config"
	pkgio "k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/spyglass/api"
)

var (
	// ErrCannotParseSource is returned by newStorageJobSource when an incorrectly formatted source string is passed
	ErrCannotParseSource = errors.New("could not create job source from provided source")
)

// StorageArtifactFetcher contains information used for fetching artifacts from GCS
type StorageArtifactFetcher struct {
	opener        pkgio.Opener
	cfg           config.Getter
	useCookieAuth bool
}

// storageJobSource is a location in GCS where Prow job-specific artifacts are stored. This implementation assumes
// Prow's native GCS upload format (treating GCS keys as a directory structure), and is not
// intended to support arbitrary GCS bucket upload formats.
type storageJobSource struct {
	source     string
	linkPrefix string
	bucket     string
	jobPrefix  string
	jobName    string
	buildID    string
}

// NewStorageArtifactFetcher creates a new ArtifactFetcher with a real GCS Client
func NewStorageArtifactFetcher(opener pkgio.Opener, cfg config.Getter, useCookieAuth bool) *StorageArtifactFetcher {
	return &StorageArtifactFetcher{
		opener:        opener,
		cfg:           cfg,
		useCookieAuth: useCookieAuth,
	}
}

// parseStorageURL parses and validates the storage path.
// If no scheme is given we assume Google Cloud Storage ("gs"). For example:
// * test-bucket/logs/sig-flexing/example-ci-run/403 or
// * gs://test-bucket/logs/sig-flexing/example-ci-run/403
func (af *StorageArtifactFetcher) parseStorageURL(storagePath string) (*url.URL, error) {
	if !strings.Contains(storagePath, "://") {
		storagePath = "gs://" + storagePath
	}
	storageURL, err := url.Parse(storagePath)
	if err != nil {
		return nil, ErrCannotParseSource
	}
	if err := af.cfg().ValidateStorageBucket(storageURL.Host); err != nil {
		return nil, err
	}
	return storageURL, nil
}

func fieldsForJob(src *storageJobSource) logrus.Fields {
	return logrus.Fields{
		"jobPrefix": src.jobPath(),
	}
}

// newStorageJobSource creates a new storageJobSource from a given storage URL.
// If no scheme is given we assume Google Cloud Storage ("gs"). For example:
// * test-bucket/logs/sig-flexing/example-ci-run/403 or
// * gs://test-bucket/logs/sig-flexing/example-ci-run/403
func (af *StorageArtifactFetcher) newStorageJobSource(storagePath string) (*storageJobSource, error) {
	storageURL, err := af.parseStorageURL(storagePath)
	if err != nil {
		return &storageJobSource{}, err
	}
	var object string
	if storageURL.Path == "" {
		object = storageURL.Path
	} else {
		object = storageURL.Path[1:]
	}

	tokens := strings.FieldsFunc(object, func(c rune) bool { return c == '/' })
	if len(tokens) < 2 {
		return &storageJobSource{}, ErrCannotParseSource
	}
	buildID := tokens[len(tokens)-1]
	name := tokens[len(tokens)-2]
	return &storageJobSource{
		source:     storageURL.String(),
		linkPrefix: storageURL.Scheme + "://",
		bucket:     storageURL.Host,
		jobPrefix:  path.Clean(object) + "/",
		jobName:    name,
		buildID:    buildID,
	}, nil
}

// Artifacts lists all artifacts available for the given job source
// If no scheme is given we assume GS, e.g.:
// * test-bucket/logs/sig-flexing/example-ci-run/403 or
// * gs://test-bucket/logs/sig-flexing/example-ci-run/403
func (af *StorageArtifactFetcher) artifacts(ctx context.Context, key string) ([]string, error) {
	src, err := af.newStorageJobSource(key)
	if err != nil {
		return nil, fmt.Errorf("Failed to get GCS job source from %s: %w", key, err)
	}

	listStart := time.Now()
	_, prefix := extractBucketPrefixPair(src.jobPath())
	artifacts := []string{}

	it, err := af.opener.Iterator(ctx, src.source, "")
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
			if err == context.Canceled {
				return nil, err
			}
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

func (af *StorageArtifactFetcher) signURL(ctx context.Context, key string) (string, error) {
	return af.opener.SignedURL(ctx, key, pkgio.SignedURLOptions{
		UseGSCookieAuth: af.useCookieAuth,
	})
}

type storageArtifactHandle struct {
	pkgio.Opener
	Name string
}

func (h *storageArtifactHandle) NewReader(ctx context.Context) (io.ReadCloser, error) {
	return h.Opener.Reader(ctx, h.Name)
}

func (h *storageArtifactHandle) NewRangeReader(ctx context.Context, offset, length int64) (io.ReadCloser, error) {
	return h.Opener.RangeReader(ctx, h.Name, offset, length)
}

func (h *storageArtifactHandle) Attrs(ctx context.Context) (pkgio.Attributes, error) {
	return h.Opener.Attributes(ctx, h.Name)
}

// Artifact constructs a GCS artifact from the given GCS bucket and key. Uses the golang GCS library
// to get read handles. If the artifactName is not a valid key in the bucket a handle will still be
// constructed and returned, but all read operations will fail (dictated by behavior of golang GCS lib).
// If no scheme is given we assume GS, e.g.:
// * test-bucket/logs/sig-flexing/example-ci-run/403 or
// * gs://test-bucket/logs/sig-flexing/example-ci-run/403
func (af *StorageArtifactFetcher) Artifact(ctx context.Context, key string, artifactName string, sizeLimit int64) (api.Artifact, error) {
	src, err := af.newStorageJobSource(key)
	if err != nil {
		return nil, fmt.Errorf("failed to get GCS job source from %s: %w", key, err)
	}

	_, prefix := extractBucketPrefixPair(src.jobPath())
	objName := path.Join(prefix, artifactName)
	obj := &storageArtifactHandle{Opener: af.opener, Name: fmt.Sprintf("%s%s/%s", src.linkPrefix, src.bucket, objName)}
	signedURL, err := af.signURL(ctx, fmt.Sprintf("%s%s/%s", src.linkPrefix, src.bucket, objName))
	if err != nil {
		return nil, err
	}
	return NewStorageArtifact(context.Background(), obj, signedURL, artifactName, sizeLimit), nil
}

func extractBucketPrefixPair(storagePath string) (string, string) {
	split := strings.SplitN(storagePath, "/", 2)
	return split[0], split[1]
}

// CanonicalLink gets a link to the location of job-specific artifacts in GCS
func (src *storageJobSource) canonicalLink() string {
	return path.Join(src.linkPrefix, src.bucket, src.jobPrefix)
}

// JobPath gets the prefix to all artifacts in GCS in the job
func (src *storageJobSource) jobPath() string {
	return fmt.Sprintf("%s/%s", src.bucket, src.jobPrefix)
}
