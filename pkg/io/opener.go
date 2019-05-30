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
	"os"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"

	"k8s.io/test-infra/testgrid/util/gcs" // TODO(fejta): move this logic here
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
	Writer(ctx context.Context, path string) (WriteCloser, error)
}

type opener struct {
	gcs storageClient
}

// NewOpener returns an opener that can read GCS and local paths.
func NewOpener(ctx context.Context, creds string) (Opener, error) {
	var options []option.ClientOption
	if creds != "" {
		options = append(options, option.WithCredentialsFile(creds))
	}
	client, err := storage.NewClient(ctx, options...)
	if err != nil {
		if creds != "" {
			return nil, err
		}
		logrus.WithError(err).Debug("Cannot load application default gcp credentials")
		client = nil
	}
	return opener{gcs: client}, nil
}

// IsNotExist will return true if the error is because the object does not exist.
func IsNotExist(err error) bool {
	return os.IsNotExist(err) || err == storage.ErrObjectNotExist
}

// LogClose will attempt a close an log any error
func LogClose(c io.Closer) {
	if err := c.Close(); err != nil {
		logrus.WithError(err).Error("Failed to close")
	}
}

func (o opener) openGCS(path string) (*storage.ObjectHandle, error) {
	if !strings.HasPrefix(path, "gs://") {
		return nil, nil
	}
	if o.gcs == nil {
		return nil, errors.New("no gcs client configured")
	}
	var p gcs.Path
	if err := p.Set(path); err != nil {
		return nil, err
	}
	if p.Object() == "" {
		return nil, errors.New("object name is empty")
	}
	return o.gcs.Bucket(p.Bucket()).Object(p.Object()), nil
}

// Reader will open the path for reading, returning an IsNotExist() error when missing
func (o opener) Reader(ctx context.Context, path string) (io.ReadCloser, error) {
	g, err := o.openGCS(path)
	if err != nil {
		return nil, fmt.Errorf("bad gcs path: %v", err)
	}
	if g == nil {
		return os.Open(path)
	}
	return g.NewReader(ctx)
}

// Writer returns a writer that overwrites the path.
func (o opener) Writer(ctx context.Context, path string) (io.WriteCloser, error) {
	g, err := o.openGCS(path)
	if err != nil {
		return nil, fmt.Errorf("bad gcs path: %v", err)
	}
	if g == nil {
		return os.Create(path)
	}
	return g.NewWriter(ctx), nil
}
