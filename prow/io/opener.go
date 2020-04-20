/*
Copyright 2019 The Kubernetes Authors.

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

package io

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"
	"google.golang.org/api/option"

	"github.com/GoogleCloudPlatform/testgrid/util/gcs" // TODO(fejta): move this logic here

	"k8s.io/test-infra/prow/io/providers"
)

type storageClient interface {
	Bucket(name string) *storage.BucketHandle
}

// Aliases to types in the standard library
type (
	ReadCloser  = io.ReadCloser
	WriteCloser = io.WriteCloser
)

// Opener has methods to read and write paths
type Opener interface {
	Reader(ctx context.Context, path string) (ReadCloser, error)
	Writer(ctx context.Context, path string, opts ...WriterOptions) (WriteCloser, error)
}

type opener struct {
	gcsClient          storageClient
	s3Credentials      []byte
	cachedBuckets      map[string]*blob.Bucket
	cachedBucketsMutex sync.Mutex
}

// NewOpener returns an opener that can read GCS, S3 and local paths.
// credentialsFile may also be empty
// For local paths it has to be empty
// In all other cases gocloud auto-discovery is used to detect credentials, if credentialsFile is empty.
// For more details about the possible content of the credentialsFile see prow/io/providers.GetBucket
func NewOpener(ctx context.Context, gcsCredentialsFile, s3CredentialsFile string) (Opener, error) {
	var options []option.ClientOption
	if gcsCredentialsFile != "" {
		options = append(options, option.WithCredentialsFile(gcsCredentialsFile))
	}
	gcsClient, err := storage.NewClient(ctx, options...)
	if err != nil {
		if gcsCredentialsFile != "" {
			return nil, err
		}
		logrus.WithError(err).Debug("Cannot load application default gcp credentials")
		gcsClient = nil
	}
	var s3Credentials []byte
	if s3CredentialsFile != "" {
		s3Credentials, err = ioutil.ReadFile(s3CredentialsFile)
		if err != nil {
			return nil, err
		}
	}
	return &opener{
		gcsClient:     gcsClient,
		s3Credentials: s3Credentials,
		cachedBuckets: map[string]*blob.Bucket{},
	}, nil
}

// ErrNotFoundTest can be used for unit tests to simulate NotFound errors.
// This is required because gocloud doesn't expose its errors.
var ErrNotFoundTest = fmt.Errorf("not found error which should only be used in tests")

// IsNotExist will return true if the error shows that the object does not exist.
func IsNotExist(err error) bool {
	if os.IsNotExist(err) || err == storage.ErrObjectNotExist {
		return true
	}
	if err == ErrNotFoundTest {
		return true
	}
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	return gcerrors.Code(err) == gcerrors.NotFound
}

// LogClose will attempt a close an log any error
func LogClose(c io.Closer) {
	if err := c.Close(); err != nil {
		logrus.WithError(err).Error("Failed to close")
	}
}

func (o *opener) openGCS(path string) (*storage.ObjectHandle, error) {
	if !strings.HasPrefix(path, "gs://") {
		return nil, nil
	}
	if o.gcsClient == nil {
		return nil, errors.New("no gcs client configured")
	}
	var p gcs.Path
	if err := p.Set(path); err != nil {
		return nil, err
	}
	if p.Object() == "" {
		return nil, errors.New("object name is empty")
	}
	return o.gcsClient.Bucket(p.Bucket()).Object(p.Object()), nil
}

// getBucket opens a bucket
// The storageProvider is discovered based on the given path.
// The buckets are cached per bucket name. So we don't open a bucket multiple times in the same process
func (o *opener) getBucket(ctx context.Context, path string) (*blob.Bucket, string, error) {
	_, bucketName, relativePath, err := providers.ParseStoragePath(path)
	if err != nil {
		return nil, "", fmt.Errorf("could not get bucket: %w", err)
	}

	o.cachedBucketsMutex.Lock()
	defer o.cachedBucketsMutex.Unlock()
	if bucket, ok := o.cachedBuckets[bucketName]; ok {
		return bucket, relativePath, nil
	}

	bucket, err := providers.GetBucket(ctx, o.s3Credentials, path)
	if err != nil {
		return nil, "", err
	}
	o.cachedBuckets[bucketName] = bucket
	return bucket, relativePath, nil
}

// Reader will open the path for reading, returning an IsNotExist() error when missing
func (o *opener) Reader(ctx context.Context, path string) (io.ReadCloser, error) {
	if strings.HasPrefix(path, "gs://") {
		g, err := o.openGCS(path)
		if err != nil {
			return nil, fmt.Errorf("bad gcs path: %v", err)
		}
		return g.NewReader(ctx)
	}
	if strings.HasPrefix(path, "/") {
		return os.Open(path)
	}

	bucket, relativePath, err := o.getBucket(ctx, path)
	if err != nil {
		return nil, err
	}
	reader, err := bucket.NewReader(ctx, relativePath, nil)
	if err != nil {
		return nil, err
	}
	return reader, nil
}

// Writer returns a writer that overwrites the path.
func (o *opener) Writer(ctx context.Context, p string, opts ...WriterOptions) (io.WriteCloser, error) {
	if strings.HasPrefix(p, "gs://") {
		g, err := o.openGCS(p)
		if err != nil {
			return nil, fmt.Errorf("bad gcs path: %v", err)
		}
		writer := g.NewWriter(ctx)
		for _, opt := range opts {
			opt.Apply(writer, nil)
		}
		return writer, nil
	}
	if strings.HasPrefix(p, "/") {
		// create parent dir if doesn't exist
		dir := path.Dir(p)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create directory %q: %w", dir, err)
		}
		return os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	}

	bucket, relativePath, err := o.getBucket(ctx, p)
	if err != nil {
		return nil, err
	}
	var wOpts blob.WriterOptions
	for _, opt := range opts {
		opt.Apply(nil, &wOpts)
	}
	writer, err := bucket.NewWriter(ctx, relativePath, &wOpts)
	if err != nil {
		return nil, err
	}
	return writer, nil
}
