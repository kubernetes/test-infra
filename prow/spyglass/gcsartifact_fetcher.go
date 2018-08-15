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
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	"k8s.io/test-infra/prow/spyglass/viewers"
	"k8s.io/test-infra/testgrid/util/gcs"
)

const (
	httpScheme  = "http"
	httpsScheme = "https"
)

// GCSArtifactFetcher contains information used for fetching artifacts from GCS
type GCSArtifactFetcher struct {
	client *storage.Client
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
func NewGCSArtifactFetcher(c *storage.Client) *GCSArtifactFetcher {
	return &GCSArtifactFetcher{
		client: c,
	}
}

func fieldsForJob(src jobSource) logrus.Fields {
	return logrus.Fields{
		"jobPrefix": src.jobPath(),
	}
}

// newGCSJobSource creates a new gcsJobSource from a given bucket and jobPrefix
func newGCSJobSource(src string) (*gcsJobSource, error) {
	gcsURL, err := url.Parse(src)
	if err != nil {
		return &gcsJobSource{}, ErrCannotParseSource
	}
	gcsPath := &gcs.Path{}
	err = gcsPath.SetURL(gcsURL)
	if err != nil {
		return &gcsJobSource{}, ErrCannotParseSource
	}

	tokens := strings.FieldsFunc(gcsPath.Object(), func(c rune) bool { return c == '/' })
	if len(tokens) < 2 {
		return &gcsJobSource{}, ErrCannotParseSource
	}
	buildID := tokens[len(tokens)-1]
	name := tokens[len(tokens)-2]
	return &gcsJobSource{
		source:     src,
		linkPrefix: "gs://",
		bucket:     gcsPath.Bucket(),
		jobPrefix:  gcsPath.Object() + "/",
		jobName:    name,
		buildID:    buildID,
	}, nil
}

// Artifacts lists all artifacts available for the given job source
func (af *GCSArtifactFetcher) artifacts(src jobSource) []string {
	listStart := time.Now()
	bucketName, prefix := extractBucketPrefixPair(src.jobPath())
	artifacts := []string{}
	bkt := af.client.Bucket(bucketName)
	q := storage.Query{
		Prefix:   prefix,
		Versions: false,
	}
	objIter := bkt.Objects(context.Background(), &q)
	for {
		oAttrs, err := objIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			logrus.WithFields(fieldsForJob(src)).WithError(err).Error("Error accessing GCS artifact.")
			continue
		}
		artifacts = append(artifacts, strings.TrimPrefix(oAttrs.Name, prefix))

	}
	listElapsed := time.Since(listStart)
	logrus.WithField("duration", listElapsed).Infof("Listed %d artifacts.", len(artifacts))
	return artifacts
}

type gcsArtifactHandle struct {
	*storage.ObjectHandle
}

func (h *gcsArtifactHandle) NewReader(ctx context.Context) (io.ReadCloser, error) {
	return h.ObjectHandle.NewReader(ctx)
}

func (h *gcsArtifactHandle) NewRangeReader(ctx context.Context, offset, length int64) (io.ReadCloser, error) {
	return h.ObjectHandle.NewRangeReader(ctx, offset, length)
}

// Artifact contructs a GCS artifact from the given GCS bucket and key. Uses the golang GCS library
// to get read handles. If the artifactName is not a valid key in the bucket a handle will still be
// constructed and returned, but all read operations will fail (dictated by behavior of golang GCS lib).
func (af *GCSArtifactFetcher) artifact(src jobSource, artifactName string, sizeLimit int64) viewers.Artifact {
	bucketName, prefix := extractBucketPrefixPair(src.jobPath())
	bkt := af.client.Bucket(bucketName)
	obj := &gcsArtifactHandle{bkt.Object(path.Join(prefix, artifactName))}
	artifactLink := &url.URL{
		Scheme: httpsScheme,
		Host:   "storage.googleapis.com",
		Path:   path.Join(src.jobPath(), artifactName),
	}
	return NewGCSArtifact(context.Background(), obj, artifactLink.String(), artifactName, sizeLimit)
}

func extractBucketPrefixPair(gcsPath string) (string, string) {
	split := strings.SplitN(gcsPath, "/", 2)
	return split[0], split[1]
}

// createJobSource tries to create a GCS job source from the provided string
func (af *GCSArtifactFetcher) createJobSource(src string) (jobSource, error) {
	jobSource, err := newGCSJobSource(src)
	if err != nil {
		return &gcsJobSource{}, err
	}
	return jobSource, nil
}

// CanonicalLink gets a link to the location of job-specific artifacts in GCS
func (src *gcsJobSource) canonicalLink() string {
	return path.Join(src.linkPrefix, src.bucket, src.jobPrefix)
}

// JobPath gets the prefix to all artifacts in GCS in the job
func (src *gcsJobSource) jobPath() string {
	return fmt.Sprintf("%s/%s", src.bucket, src.jobPrefix)
}
