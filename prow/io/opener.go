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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"
	"google.golang.org/api/option"

	"github.com/GoogleCloudPlatform/testgrid/util/gcs" // TODO(fejta): move this logic here

	"k8s.io/test-infra/prow/io/providers"
)

const (
	httpsScheme = "https"
)

type storageClient interface {
	Bucket(name string) *storage.BucketHandle
}

// Aliases to types in the standard library
type (
	ReadCloser  = io.ReadCloser
	WriteCloser = io.WriteCloser
)

type Attributes struct {
	// ContentEncoding specifies the encoding used for the blob's content, if any.
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Encoding
	ContentEncoding string
	// Size is the size of the blob's content in bytes.
	Size int64
}

// Opener has methods to read and write paths
type Opener interface {
	Reader(ctx context.Context, path string) (ReadCloser, error)
	RangeReader(ctx context.Context, path string, offset, length int64) (io.ReadCloser, error)
	Writer(ctx context.Context, path string, opts ...WriterOptions) (WriteCloser, error)
	Attributes(ctx context.Context, path string) (Attributes, error)
	SignedURL(ctx context.Context, path string, opts SignedURLOptions) (string, error)
	Iterator(ctx context.Context, prefix, delimiter string) (ObjectIterator, error)
}

type opener struct {
	gcsCredentialsFile string
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
	gcsClient, err := createGCSClient(ctx, gcsCredentialsFile)
	if err != nil {
		return nil, err
	}
	var s3Credentials []byte
	if s3CredentialsFile != "" {
		s3Credentials, err = ioutil.ReadFile(s3CredentialsFile)
		if err != nil {
			return nil, err
		}
	}
	return &opener{
		gcsClient:          gcsClient,
		gcsCredentialsFile: gcsCredentialsFile,
		s3Credentials:      s3Credentials,
		cachedBuckets:      map[string]*blob.Bucket{},
	}, nil
}

// NewGCSOpener can be used for testing against a fakeGCSClient
func NewGCSOpener(gcsClient *storage.Client) Opener {
	return &opener{
		gcsClient:     gcsClient,
		cachedBuckets: map[string]*blob.Bucket{},
	}
}

func createGCSClient(ctx context.Context, gcsCredentialsFile string) (storageClient, error) {
	// if gcsCredentialsFile is set, we have to be able to create storage.Client withCredentialsFile
	if gcsCredentialsFile != "" {
		return storage.NewClient(ctx, option.WithCredentialsFile(gcsCredentialsFile))
	}

	// if gcsCredentialsFile is unset, first try to use the default credentials
	gcsClient, err := storage.NewClient(ctx)
	if err == nil {
		return gcsClient, nil
	}
	logrus.WithError(err).Debug("Cannot load application default gcp credentials, falling back to anonymous client")

	// if default credentials don't work, use an anonymous client, this should always work
	return storage.NewClient(ctx, option.WithoutAuthentication())
}

// ErrNotFoundTest can be used for unit tests to simulate NotFound errors.
// This is required because gocloud doesn't expose its errors.
var ErrNotFoundTest = fmt.Errorf("not found error which should only be used in tests")

// IsNotExist will return true if the error shows that the object does not exist.
func IsNotExist(err error) bool {
	if os.IsNotExist(err) {
		return true
	}
	if errors.Is(err, ErrNotFoundTest) {
		return true
	}
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	if errors.Is(err, storage.ErrObjectNotExist) {
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
	if !strings.HasPrefix(path, providers.GS+"://") {
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
	if strings.HasPrefix(path, providers.GS+"://") {
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

func (o *opener) RangeReader(ctx context.Context, path string, offset, length int64) (io.ReadCloser, error) {
	if strings.HasPrefix(path, providers.GS+"://") {
		g, err := o.openGCS(path)
		if err != nil {
			return nil, fmt.Errorf("bad gcs path: %v", err)
		}
		return g.NewRangeReader(ctx, offset, length)
	}

	bucket, relativePath, err := o.getBucket(ctx, path)
	if err != nil {
		return nil, err
	}
	reader, err := bucket.NewRangeReader(ctx, relativePath, offset, length, nil)
	if err != nil {
		return nil, err
	}
	return reader, nil
}

var PreconditionFailedObjectAlreadyExists = fmt.Errorf("object already exists")

// Writer returns a writer that overwrites the path.
func (o *opener) Writer(ctx context.Context, p string, opts ...WriterOptions) (io.WriteCloser, error) {
	options := &WriterOptions{}
	for _, opt := range opts {
		opt.Apply(options)
	}
	if strings.HasPrefix(p, providers.GS+"://") {
		g, err := o.openGCS(p)
		if err != nil {
			return nil, fmt.Errorf("bad gcs path: %v", err)
		}
		if options.PreconditionDoesNotExist != nil && *options.PreconditionDoesNotExist {
			g = g.If(storage.Conditions{DoesNotExist: true})
		}

		writer := g.NewWriter(ctx)
		options.apply(writer, nil)
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
	options.apply(nil, &wOpts)

	if options.PreconditionDoesNotExist != nil && *options.PreconditionDoesNotExist {
		wOpts.BeforeWrite = func(asFunc func(interface{}) bool) error {
			_, err := o.Reader(ctx, p)
			if err != nil {
				// we got an error, but not object not exists
				if !IsNotExist(err) {
					return err
				}
				// Precondition fulfilled, return nil
				return nil
			}
			// Precondition failed, we got no err because object already exists
			return PreconditionFailedObjectAlreadyExists
		}
	}

	writer, err := bucket.NewWriter(ctx, relativePath, &wOpts)
	if err != nil {
		return nil, err
	}
	return writer, nil
}

func (o *opener) Attributes(ctx context.Context, path string) (Attributes, error) {
	if strings.HasPrefix(path, providers.GS+"://") {
		g, err := o.openGCS(path)
		if err != nil {
			return Attributes{}, fmt.Errorf("bad gcs path: %v", err)
		}
		attr, err := g.Attrs(ctx)
		if err != nil {
			return Attributes{}, err
		}
		return Attributes{
			ContentEncoding: attr.ContentEncoding,
			Size:            attr.Size,
		}, nil
	}

	bucket, relativePath, err := o.getBucket(ctx, path)
	if err != nil {
		return Attributes{}, err
	}

	attr, err := bucket.Attributes(ctx, relativePath)
	if err != nil {
		return Attributes{}, err
	}
	return Attributes{
		ContentEncoding: attr.ContentEncoding,
		Size:            attr.Size,
	}, nil
}

const (
	GSAnonHost   = "storage.googleapis.com"
	GSCookieHost = "storage.cloud.google.com"
)

func (o *opener) SignedURL(ctx context.Context, p string, opts SignedURLOptions) (string, error) {
	_, bucketName, relativePath, err := providers.ParseStoragePath(p)
	if err != nil {
		return "", fmt.Errorf("could not get bucket: %w", err)
	}
	if strings.HasPrefix(p, providers.GS+"://") {
		// We specifically want to use cookie auth, see:
		// https://cloud.google.com/storage/docs/access-control/cookie-based-authentication
		if opts.UseGSCookieAuth {
			artifactLink := &url.URL{
				Scheme: httpsScheme,
				Host:   GSCookieHost,
				Path:   path.Join(bucketName, relativePath),
			}
			return artifactLink.String(), nil
		}

		// If we're anonymous we can just return a plain URL.
		if o.gcsCredentialsFile == "" {
			artifactLink := &url.URL{
				Scheme: httpsScheme,
				Host:   GSAnonHost,
				Path:   path.Join(bucketName, relativePath),
			}
			return artifactLink.String(), nil
		}

		// TODO(fejta): do not require the json file https://github.com/kubernetes/test-infra/issues/16489
		// As far as I can tell, there is no sane way to get these values other than just
		// reading them out of the JSON file ourselves.
		f, err := os.Open(o.gcsCredentialsFile)
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
		return storage.SignedURL(bucketName, relativePath, &storage.SignedURLOptions{
			Method:         "GET",
			Expires:        time.Now().Add(10 * time.Minute),
			GoogleAccessID: auth.ClientEmail,
			PrivateKey:     []byte(auth.PrivateKey),
		})
	}

	bucket, relativePath, err := o.getBucket(ctx, p)
	if err != nil {
		return "", err
	}
	return bucket.SignedURL(ctx, relativePath, &blob.SignedURLOptions{
		Method: "GET",
		Expiry: 10 * time.Minute,
	})
}

func (o *opener) Iterator(ctx context.Context, prefix, delimiter string) (ObjectIterator, error) {
	storageProvider, bucketName, relativePath, err := providers.ParseStoragePath(prefix)
	if err != nil {
		return nil, fmt.Errorf("could not get bucket: %w", err)
	}

	if storageProvider == providers.GS {
		if o.gcsClient == nil {
			return nil, errors.New("no gcs client configured")
		}
		bkt := o.gcsClient.Bucket(bucketName)
		return gcsObjectIterator{
			Iterator: bkt.Objects(ctx, &storage.Query{
				Prefix:    relativePath,
				Delimiter: delimiter,
				Versions:  false,
			}),
		}, nil
	}

	bucket, relativePath, err := o.getBucket(ctx, prefix)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(relativePath, "/") {
		relativePath += "/"
	}
	return openerObjectIterator{
		Iterator: bucket.List(&blob.ListOptions{
			Prefix:    relativePath,
			Delimiter: delimiter,
		}),
	}, nil
}
