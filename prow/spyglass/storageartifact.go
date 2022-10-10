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
	"sync"

	pkgio "k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/spyglass/lenses"
)

// StorageArtifact represents some output of a prow job stored in GCS
type StorageArtifact struct {
	// The handle of the object in GCS
	handle artifactHandle

	// The link to the Artifact in GCS
	link string

	// The path of the Artifact within the job
	path string

	// sizeLimit is the max size to read before failing
	sizeLimit int64

	// ctx provides context for cancellation and timeout. Embedded in struct to preserve
	// conformance with io.ReaderAt
	ctx context.Context

	attrs *pkgio.Attributes

	lock sync.RWMutex
}

type artifactHandle interface {
	Attrs(ctx context.Context) (pkgio.Attributes, error)
	NewRangeReader(ctx context.Context, offset, length int64) (io.ReadCloser, error)
	NewReader(ctx context.Context) (io.ReadCloser, error)
	UpdateAttrs(context.Context, pkgio.ObjectAttrsToUpdate) (*pkgio.Attributes, error)
}

// NewStorageArtifact returns a new StorageArtifact with a given handle, canonical link, and path within the job
func NewStorageArtifact(ctx context.Context, handle artifactHandle, link string, path string, sizeLimit int64) *StorageArtifact {
	return &StorageArtifact{
		handle:    handle,
		link:      link,
		path:      path,
		sizeLimit: sizeLimit,
		ctx:       ctx,
	}
}

func (a *StorageArtifact) fetchAttrs() (*pkgio.Attributes, error) {
	a.lock.RLock()
	attrs := a.attrs
	a.lock.RUnlock()
	if attrs != nil {
		return attrs, nil
	}
	if a.attrs != nil {
		return a.attrs, nil
	}
	{
		attrs, err := a.handle.Attrs(a.ctx)
		if err != nil {
			return nil, err
		}
		a.lock.Lock()
		defer a.lock.Unlock()
		a.attrs = &attrs
	}
	return a.attrs, nil
}

// Size returns the size of the artifact in GCS
func (a *StorageArtifact) Size() (int64, error) {
	attrs, err := a.fetchAttrs()
	if err != nil {
		return 0, fmt.Errorf("error getting gcs attributes for artifact: %w", err)
	}
	return attrs.Size, nil
}

func (a *StorageArtifact) Metadata() (map[string]string, error) {
	attrs, err := a.fetchAttrs()
	if err != nil {
		return nil, fmt.Errorf("fetch attributes: %w", err)
	}
	return attrs.Metadata, nil
}

func (a *StorageArtifact) UpdateMetadata(meta map[string]string) error {
	attrs, err := a.handle.UpdateAttrs(a.ctx, pkgio.ObjectAttrsToUpdate{
		Metadata: meta,
	})
	if err != nil {
		return err
	}
	a.lock.Lock()
	a.attrs = attrs
	a.lock.Unlock()
	return nil
}

// JobPath gets the GCS path of the artifact within the current job
func (a *StorageArtifact) JobPath() string {
	return a.path
}

// CanonicalLink gets the GCS web address of the artifact
func (a *StorageArtifact) CanonicalLink() string {
	return a.link
}

// ReadAt reads len(p) bytes from a file in GCS at offset off
func (a *StorageArtifact) ReadAt(p []byte, off int64) (n int, err error) {
	if int64(len(p)) > a.sizeLimit {
		return 0, lenses.ErrRequestSizeTooLarge
	}
	gzipped, err := a.gzipped()
	if err != nil {
		return 0, fmt.Errorf("error checking artifact for gzip compression: %w", err)
	}
	if gzipped {
		return 0, lenses.ErrGzipOffsetRead
	}
	artifactSize, err := a.Size()
	if err != nil {
		return 0, fmt.Errorf("error getting artifact size: %w", err)
	}
	if off >= artifactSize {
		return 0, fmt.Errorf("offset must be less than artifact size")
	}
	var gotEOF bool
	toRead := int64(len(p))
	if toRead+off > artifactSize {
		return 0, fmt.Errorf("read range exceeds artifact contents")
	} else if toRead+off == artifactSize {
		gotEOF = true
	}
	reader, err := a.handle.NewRangeReader(a.ctx, off, toRead)
	if err != nil {
		return 0, fmt.Errorf("error getting artifact reader: %w", err)
	}
	defer reader.Close()
	// We need to keep reading until we fill the buffer or hit EOF.
	offset := 0
	for offset < len(p) {
		n, err = reader.Read(p[offset:])
		offset += n
		if err != nil {
			if err == io.EOF && gotEOF {
				break
			}
			return 0, fmt.Errorf("error reading from artifact: %w", err)
		}
	}
	if gotEOF {
		return offset, io.EOF
	}
	return offset, nil
}

// ReadAtMost reads at most n bytes from a file in GCS. If the file is compressed (gzip) in GCS, n bytes
// of gzipped content will be downloaded and decompressed into potentially GREATER than n bytes of content.
func (a *StorageArtifact) ReadAtMost(n int64) ([]byte, error) {
	if n > a.sizeLimit {
		return nil, lenses.ErrRequestSizeTooLarge
	}
	var reader io.ReadCloser
	var p []byte
	gzipped, err := a.gzipped()
	if err != nil {
		return nil, fmt.Errorf("error checking artifact for gzip compression: %w", err)
	}
	if gzipped {
		reader, err = a.handle.NewReader(a.ctx)
		if err != nil {
			return nil, fmt.Errorf("error getting artifact reader: %w", err)
		}
		defer reader.Close()
		p, err = io.ReadAll(reader) // Must readall for gzipped files
		if err != nil {
			return nil, fmt.Errorf("error reading all from artifact: %w", err)
		}
		artifactSize := int64(len(p))
		readRange := n
		if n > artifactSize {
			readRange = artifactSize
			return p[:readRange], io.EOF
		}
		return p[:readRange], nil

	}
	artifactSize, err := a.Size()
	if err != nil {
		return nil, fmt.Errorf("error getting artifact size: %w", err)
	}
	readRange := n
	var gotEOF bool
	if n > artifactSize {
		gotEOF = true
		readRange = artifactSize
	}
	reader, err = a.handle.NewRangeReader(a.ctx, 0, readRange)
	if err != nil {
		return nil, fmt.Errorf("error getting artifact reader: %w", err)
	}
	defer reader.Close()
	p, err = io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("error reading all from artifact: %w", err)
	}
	if gotEOF {
		return p, io.EOF
	}
	return p, nil
}

// ReadAll will either read the entire file or throw an error if file size is too big
func (a *StorageArtifact) ReadAll() ([]byte, error) {
	size, err := a.Size()
	if err != nil {
		return nil, fmt.Errorf("error getting artifact size: %w", err)
	}
	if size > a.sizeLimit {
		return nil, lenses.ErrFileTooLarge
	}
	reader, err := a.handle.NewReader(a.ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting artifact reader: %w", err)
	}
	defer reader.Close()
	p, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("error reading all from artifact: %w", err)
	}
	return p, nil
}

// ReadTail reads the last n bytes from a file in GCS
func (a *StorageArtifact) ReadTail(n int64) ([]byte, error) {
	if n > a.sizeLimit {
		return nil, lenses.ErrRequestSizeTooLarge
	}
	gzipped, err := a.gzipped()
	if err != nil {
		return nil, fmt.Errorf("error checking artifact for gzip compression: %w", err)
	}
	if gzipped {
		return nil, lenses.ErrGzipOffsetRead
	}
	size, err := a.Size()
	if err != nil {
		return nil, fmt.Errorf("error getting artifact size: %w", err)
	}
	var offset int64
	if n >= size {
		offset = 0
	} else {
		offset = size - n
	}
	reader, err := a.handle.NewRangeReader(a.ctx, offset, -1)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("error getting artifact reader: %w", err)
	}
	defer reader.Close()
	read, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("error reading all from artiact: %w", err)
	}
	return read, nil
}

// gzipped returns whether the file is gzip-encoded in GCS
func (a *StorageArtifact) gzipped() (bool, error) {
	attrs, err := a.handle.Attrs(a.ctx)
	if err != nil {
		return false, fmt.Errorf("error getting gcs attributes for artifact: %w", err)
	}
	return attrs.ContentEncoding == "gzip", nil
}
