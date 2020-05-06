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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"k8s.io/test-infra/prow/spyglass/api"
)

const (
	httpScheme  = "http"
	httpsScheme = "https"
)

var (
	// ErrCannotParseSource is returned by newGCSJobSource when an incorrectly formatted source string is passed
	ErrCannotParseSource = errors.New("could not create job source from provided source")
)

// GCSArtifactFetcher contains information used for fetching artifacts from GCS
type GCSArtifactFetcher struct {
	client        *storage.Client
	gcsCredsFile  string
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
func NewGCSArtifactFetcher(c *storage.Client, gcsCredsFile string, useCookieAuth bool) *GCSArtifactFetcher {
	return &GCSArtifactFetcher{
		client:        c,
		gcsCredsFile:  gcsCredsFile,
		useCookieAuth: useCookieAuth,
	}
}

func fieldsForJob(src *gcsJobSource) logrus.Fields {
	return logrus.Fields{
		"jobPrefix": src.jobPath(),
	}
}

// newGCSJobSource creates a new gcsJobSource from a given bucket and jobPrefix
func newGCSJobSource(src string) (*gcsJobSource, error) {
	gcsURL, err := url.Parse(fmt.Sprintf("gs://%s", src))
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
		jobPrefix:  path.Clean(gcsPath.Object()) + "/",
		jobName:    name,
		buildID:    buildID,
	}, nil
}

// Artifacts lists all artifacts available for the given job source
func (af *GCSArtifactFetcher) artifacts(key string) ([]string, error) {
	src, err := newGCSJobSource(key)
	if err != nil {
		return nil, fmt.Errorf("Failed to get GCS job source from %s: %v", key, err)
	}

	listStart := time.Now()
	bucketName, prefix := extractBucketPrefixPair(src.jobPath())
	artifacts := []string{}
	bkt := af.client.Bucket(bucketName)
	q := storage.Query{
		Prefix:   prefix,
		Versions: false,
	}
	objIter := bkt.Objects(context.Background(), &q)
	wait := []time.Duration{16, 32, 64, 128, 256, 256, 512, 512}
	for i := 0; ; {
		oAttrs, err := objIter.Next()
		if err == iterator.Done {
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

const (
	anonHost   = "storage.googleapis.com"
	cookieHost = "storage.cloud.google.com"
)

func (af *GCSArtifactFetcher) signURL(bucket, obj string) (string, error) {
	// We specifically want to use cookie auth, see:
	// https://cloud.google.com/storage/docs/access-control/cookie-based-authentication
	if af.useCookieAuth {
		artifactLink := &url.URL{
			Scheme: httpsScheme,
			Host:   cookieHost,
			Path:   path.Join(bucket, obj),
		}
		return artifactLink.String(), nil
	}

	// If we're anonymous we can just return a plain URL.
	if af.gcsCredsFile == "" {
		artifactLink := &url.URL{
			Scheme: httpsScheme,
			Host:   anonHost,
			Path:   path.Join(bucket, obj),
		}
		return artifactLink.String(), nil
	}

	// TODO(fejta): do not require the json file https://github.com/kubernetes/test-infra/issues/16489
	// As far as I can tell, there is no sane way to get these values other than just
	// reading them out of the JSON file ourselves.
	f, err := os.Open(af.gcsCredsFile)
	if err != nil {
		return "", err
	}
	defer f.Close()
	auth := struct {
		Type        string `json:"type"`
		PrivateKey  string `json:"private_key"`
		ClientEmail string `json:"client_email"`
	}{}
	if err := json.NewDecoder(f).Decode(&auth); err != nil {
		return "", err
	}
	if auth.Type != "service_account" {
		return "", fmt.Errorf("only service_account GCS auth is supported, got %q", auth.Type)
	}
	return storage.SignedURL(bucket, obj, &storage.SignedURLOptions{
		Method:         "GET",
		Expires:        time.Now().Add(10 * time.Minute),
		GoogleAccessID: auth.ClientEmail,
		PrivateKey:     []byte(auth.PrivateKey),
	})
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

// Artifact constructs a GCS artifact from the given GCS bucket and key. Uses the golang GCS library
// to get read handles. If the artifactName is not a valid key in the bucket a handle will still be
// constructed and returned, but all read operations will fail (dictated by behavior of golang GCS lib).
func (af *GCSArtifactFetcher) Artifact(key string, artifactName string, sizeLimit int64) (api.Artifact, error) {
	src, err := newGCSJobSource(key)
	if err != nil {
		return nil, fmt.Errorf("Failed to get GCS job source from %s: %v", key, err)
	}

	bucketName, prefix := extractBucketPrefixPair(src.jobPath())
	bkt := af.client.Bucket(bucketName)
	objName := path.Join(prefix, artifactName)
	obj := &gcsArtifactHandle{bkt.Object(objName)}
	signedURL, err := af.signURL(bucketName, objName)
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
