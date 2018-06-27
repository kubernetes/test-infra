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
	"log"
	"path"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

// A fetcher for a GCS client
type GCSArtifactFetcher struct {
	client *storage.Client
}

// A location in GCS where prow job-specific artifacts are stored
type GCSJobSource struct {
	linkPrefix string
	bucket     string
	jobPath    string
}

// NewGCSArtifactFetcher creates a new ArtifactFetcher with a real GCS Client
func NewGCSArtifactFetcher() *GCSArtifactFetcher {
	c, err := storage.NewClient(context.Background(), option.WithoutAuthentication())
	if err != nil {
		log.Fatal(err)
	}
	return &GCSArtifactFetcher{
		client: c,
	}
}

// NewGCSJobSource creates a new GCSJobSource from a given bucket and jobPath
func NewGCSJobSource(bucket string, jobPath string) *GCSJobSource {
	return &GCSJobSource{
		linkPrefix: "gs://",
		bucket:     bucket,
		jobPath:    jobPath,
	}
}

// NewGCSJobSource creates a new GCSJobSource from a given bucket, jobPath, and link prefix
func NewGCSJobSourceWithPrefix(linkPrefix string, bucket string, jobPath string) *GCSJobSource {
	return &GCSJobSource{
		linkPrefix: linkPrefix,
		bucket:     bucket,
		jobPath:    jobPath,
	}
}

// Artifacts gets all artifacts from a GCS job source
func (af *GCSArtifactFetcher) Artifacts(src JobSource) []viewers.Artifact {
	artifacts := []viewers.Artifact{}
	bkt := af.client.Bucket(src.BucketName())
	q := storage.Query{
		Prefix:   src.JobPath(),
		Versions: false,
	}
	objIter := bkt.Objects(context.Background(), &q)
	for {
		oAttrs, err := objIter.Next()
		if err == iterator.Done {
			break
		}
		artifacts = append(artifacts, NewGCSArtifact(bkt.Object(oAttrs.Name), src.JobPath()))

	}
	return artifacts
}

// CanonicalLink gets a link to the location of job-specific artifacts in GCS
func (src *GCSJobSource) CanonicalLink() string {
	return path.Join(src.linkPrefix, src.bucket, src.jobPath)
}

// BucketName gets the bucket name of the GCS Job Source
func (src *GCSJobSource) BucketName() string {
	return src.bucket
}

// JobPath gets the path in GCS to the job
func (src *GCSJobSource) JobPath() string {
	return src.jobPath
}
