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
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/spyglass/viewers"
)

// A fetcher for a GCS client
type GCSArtifactFetcher struct {
	client      *storage.Client
	xmlEndpoint string
	withTLS     bool
}

// A location in GCS where Prow job-specific artifacts are stored. This implementation assumes
// Prow's native GCS upload format (treating GCS keys as a directory structure), and is not
// intended to support arbitrary GCS bucket upload formats.
type GCSJobSource struct {
	source     string
	linkPrefix string
	bucket     string
	jobPath    string
	jobName    string
	buildID    string
}

// NewGCSArtifactFetcher creates a new ArtifactFetcher with a real GCS Client
func NewGCSArtifactFetcher(c *storage.Client, xmlEndpoint string, tls bool) *GCSArtifactFetcher {
	return &GCSArtifactFetcher{
		client:      c,
		xmlEndpoint: xmlEndpoint,
		withTLS:     tls,
	}
}

// NewGCSJobSource creates a new GCSJobSource from a given bucket and jobPath
func NewGCSJobSource(src string) *GCSJobSource {
	linkPrefix := "gs://"
	noPrefixSrc := strings.TrimPrefix(src, linkPrefix)
	if !strings.HasSuffix(noPrefixSrc, "/") { // Cleaning up path
		noPrefixSrc += "/"
	}
	tokens := strings.FieldsFunc(noPrefixSrc, func(c rune) bool { return c == '/' })
	bucket := tokens[0]
	buildID := tokens[len(tokens)-1]
	name := tokens[len(tokens)-2]
	jobPath := strings.TrimPrefix(noPrefixSrc, bucket+"/") // Extra / is not part of prefix, only necessary for URI
	return &GCSJobSource{
		source:     src,
		linkPrefix: linkPrefix,
		bucket:     bucket,
		jobPath:    jobPath,
		jobName:    name,
		buildID:    buildID,
	}
}

// isGCSSource recognizes whether a source string references a GCS bucket
func isGCSSource(src string) bool {
	return strings.HasPrefix(src, "gs://")
}

// GCSMarker holds the starting point for the next paginated GCS query
type GCSMarker struct {
	XMLName xml.Name `xml:"ListBucketResult"`
	Marker  string   `xml:"NextMarker"`
}

// Contents is a single entry returned by the GCS XML API
type Contents struct {
	Key string
}

// GCSReq contains the contents of a GCS XML API list response
type GCSReq struct {
	XMLName  xml.Name `xml:"ListBucketResult"`
	Contents []Contents
}

// names is a helper function for extracting artifact names in parallel from a GCS XML API response
func names(content []byte, names chan string, wg *sync.WaitGroup) {
	defer wg.Done()
	extracted := GCSReq{
		Contents: []Contents{},
	}
	err := xml.Unmarshal(content, &extracted)
	if err != nil {
		logrus.WithError(err).Error("Error unmarshaling artifact names from XML")
	}
	for _, c := range extracted.Contents {
		names <- c.Key
	}
}

// Artifacts lists all artifacts available for the given job source.
// Uses the GCS XML API because it is ~2x faster than the golang GCS library
// for large number of artifacts. It should also be S3 compatible according to
// the GCS api docs.
// TODO: Look at pushing this upstream to golang GCS client
func (af *GCSArtifactFetcher) Artifacts(src JobSource) []string {
	var wg sync.WaitGroup
	artStart := time.Now()
	artifacts := []string{}
	prefix := src.JobPath()
	maxResults := 1000
	bodies := [][]byte{}
	marker := GCSMarker{}
	c := http.Client{}
	scheme := "https"
	if !af.withTLS {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		c = http.Client{Transport: tr}
		scheme = "http"
	}
	for {
		params := url.Values{}
		params.Add("prefix", prefix)
		if marker.Marker != "" {
			params.Add("marker", marker.Marker)
		}
		req := url.URL{
			Scheme:   scheme,
			Host:     af.xmlEndpoint,
			Path:     src.BucketName(),
			RawQuery: params.Encode(),
		}
		resp, err := c.Get(req.String())
		if err != nil {
			logrus.WithError(err).Error("Error in GCS XML API GET request")
		}
		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			logrus.WithError(err).Error("Error reading body of GCS XML API response")
			continue
		}
		bodies = append(bodies, body)

		marker = GCSMarker{}
		err = xml.Unmarshal(body, &marker)
		if err != nil {
			logrus.WithError(err).Error("Error unmarshaling body of GCS XML API response")
		}
		if marker.Marker == "" {
			break
		}
	}

	namesChan := make(chan string, maxResults*len(bodies))
	for _, body := range bodies {
		wg.Add(1)
		go names(body, namesChan, &wg)
	}

	wg.Wait()
	close(namesChan)
	for name := range namesChan {
		aName := strings.TrimPrefix(name, src.JobPath())
		artifacts = append(artifacts, aName)
	}
	artElapsed := time.Since(artStart)
	logrus.Infof("Listed %d GCS artifacts in %s", len(artifacts), artElapsed)
	return artifacts
}

// Artifact contructs a GCS artifact from the given GCS bucket and key. Uses the golang GCS library
// to get read handles. If the artifactName is not a valid key in the bucket a handle will still be
// constructed and returned, but all read operations will fail (dictated by behavior of golang GCS lib).
func (af *GCSArtifactFetcher) Artifact(src JobSource, artifactName string, sizeLimit int64) viewers.Artifact {
	bkt := af.client.Bucket(src.BucketName())
	obj := bkt.Object(path.Join(src.JobPath(), artifactName))
	link := fmt.Sprintf("https://storage.googleapis.com/%s/%s/%s", src.BucketName(), src.JobPath(), artifactName)
	return NewGCSArtifact(obj, link, artifactName, sizeLimit)
}

// CreateJobSource tries to create a GCS job source from the provided string
func (af *GCSArtifactFetcher) CreateJobSource(src string) (JobSource, error) {
	if isGCSSource(src) {
		return NewGCSJobSource(src), nil
	}
	return &GCSJobSource{}, ErrCannotParseSource
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

// BuildID gets the id of the job
func (src *GCSJobSource) BuildID() string {
	return src.buildID
}
