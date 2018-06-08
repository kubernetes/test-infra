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
	"log"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// Gets artfiacts from a storage provider
type ArtifactFetcher interface {
	NewFetcher()
	Artifacts(src *JobSource) []Artifact
}

// A fetcher for a GCS client
type GCSArtifactFetcher struct {
	ArtifactFetcher
	client *storage.Client
}

// A location storing the results of prow jobs
type JobSource interface {
	CanonicalLink() string
	JobPath() string
	BucketName() string
}

// A location in GCS where prow job-specific artifacts are stored
type GCSJobSource struct {
	JobSource
	bucket  string
	jobPath string
}

// Gets a new ArtifactFetcher with a real GCS Client
func (af *GCSArtifactFetcher) NewFetcher() {
	c, err := storage.NewClient(context.Background(), option.WithoutAuthentication())
	if err != nil {
		log.Fatal(err)
	}
	af.client = c
}

// Gets all artifacts from a GCS job source
func (af *GCSArtifactFetcher) Artifacts(src JobSource) []Artifact {
	artifacts := []Artifact{}
	bkt := af.client.Bucket(src.BucketName())
	q := storage.Query{
		Prefix:   src.JobPath(),
		Versions: false,
	}
	objIter := bkt.Objects(context.Background(), &q)
	for oAttrs, err := objIter.Next(); err != iterator.Done; {
		if err != nil {
			break
		}

		artifacts = append(artifacts, GCSArtifact{
			link: oAttrs.MediaLink,
			path: strings.TrimPrefix(oAttrs.Name, src.JobPath()),
		})
	}
	return artifacts
}

// Gets a link to the location of job-specific artifacts in GCS
func (src *GCSJobSource) CanonicalLink() string {
	return fmt.Sprintf("https://storage.googleapis.com/%s/%s", src.bucket, src.jobPath)
}

// Gets the bucket name of the GCS Job Source
func (src *GCSJobSource) BucketName() string {
	return src.bucket
}

func (src *GCSJobSource) JobPath() string {
	return src.jobPath
}
