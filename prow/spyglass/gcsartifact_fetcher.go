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
	"path"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"k8s.io/test-infra/prow/spyglass/viewers"
)

// A fetcher for a GCS client
type GCSArtifactFetcher struct {
	Client *storage.Client
}

// A location in GCS where prow job-specific artifacts are stored
type GCSJobSource struct {
	source     string
	linkPrefix string
	bucket     string
	jobPath    string
	jobName    string
	jobId      string
}

// NewGCSArtifactFetcher creates a new ArtifactFetcher with a real GCS Client
func NewGCSArtifactFetcher() *GCSArtifactFetcher {
	c, err := storage.NewClient(context.Background(), option.WithoutAuthentication())
	if err != nil {
		log.Fatal(err)
	}
	return &GCSArtifactFetcher{
		Client: c,
	}
}

// NewGCSJobSource creates a new GCSJobSource from a given bucket and jobPath
func NewGCSJobSource(src string) *GCSJobSource {
	linkPrefix := "gs://"
	noPrefixSrc := strings.TrimPrefix(src, linkPrefix)
	tokens := strings.FieldsFunc(noPrefixSrc, func(c rune) bool { return c == '/' })
	bucket := tokens[0]
	jobId := tokens[len(tokens)-1]
	name := tokens[len(tokens)-2]
	jobPath := strings.TrimPrefix(noPrefixSrc, bucket+"/")
	return &GCSJobSource{
		source:     src,
		linkPrefix: linkPrefix,
		bucket:     bucket,
		jobPath:    jobPath,
		jobName:    name,
		jobId:      jobId,
	}
}

// isGCSSource recognizes whether a source string references a GCS bucket
func isGCSSource(src string) bool {
	return strings.HasPrefix(src, "gs://")
}

// Artifacts gets all artifact names from a GCS job source
func (af *GCSArtifactFetcher) Artifacts(src JobSource) []string {
	artifacts := []string{}

	bkt := af.Client.Bucket(src.BucketName())

	q := storage.Query{
		Prefix:   src.JobPath(),
		Versions: false,
	}
	objIter := bkt.Objects(context.Background(), &q)
	for {
		oAttrs, err := objIter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			logrus.WithError(err).Error("Error accessing GCS object.")
		}

		artifacts = append(artifacts, strings.TrimPrefix(strings.TrimPrefix(oAttrs.Name, src.JobPath()), "/"))

	}
	return artifacts
}

// Artifact contructs a GCS artifact from the given GCS bucket and key
func (af *GCSArtifactFetcher) Artifact(src JobSource, artifactName string) viewers.Artifact {
	bkt := af.Client.Bucket(src.BucketName())
	obj := bkt.Object(path.Join(src.JobPath(), artifactName))
	link := fmt.Sprintf("http://gcsweb.k8s.io/gcs/%s/%s/%s", src.BucketName(), src.JobPath(), artifactName)
	return NewGCSArtifact(obj, link, artifactName)
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

// JobName gets the name of the job
func (src *GCSJobSource) JobName() string {
	return src.jobName
}

// JobId gets the id of the job
func (src *GCSJobSource) JobId() string {
	return src.jobId
}
