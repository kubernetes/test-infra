/*
Copyright 2017 The Kubernetes Authors.

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

package objectstorage

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/sirupsen/logrus"
	"gocloud.dev/blob"

	"k8s.io/test-infra/prow/errorutil"
)

// UploadFunc knows how to upload into an object
type UploadFunc func(obj *blob.Bucket, dest string) error

// Upload uploads all of the data in the
// uploadTargets map to object storage in parallel. The map is
// keyed on object storage path under the bucket
func Upload(bucket *blob.Bucket, uploadTargets map[string]UploadFunc) error {
	errCh := make(chan error, len(uploadTargets))
	group := &sync.WaitGroup{}
	group.Add(len(uploadTargets))
	for dest, upload := range uploadTargets {

		logrus.WithField("dest", dest).Info("Queued for upload")
		go func(f UploadFunc, bucket *blob.Bucket, name string) {
			defer group.Done()
			if err := f(bucket, name); err != nil {
				errCh <- err
			}
			logrus.WithField("dest", name).Info("Finished upload")
		}(upload, bucket, dest)
	}
	group.Wait()
	close(errCh)
	if len(errCh) != 0 {
		var uploadErrors []error
		for err := range errCh {
			uploadErrors = append(uploadErrors, err)
		}
		return fmt.Errorf("encountered errors during upload: %v", uploadErrors)
	}

	return nil
}

// FileUpload returns an UploadFunc which copies all
// data from the file on disk to the Object Storage
func FileUpload(file string) UploadFunc {
	return FileUploadWithAttributes(file, nil)
}

// FileUploadWithMetadata returns an UploadFunc which copies all
// data from the file on disk into Object Storage and also sets the provided
// metadata fields on the object.
func FileUploadWithMetadata(file string, metadata map[string]string) UploadFunc {
	return FileUploadWithAttributes(file, &blob.WriterOptions{Metadata: metadata})
}

// FileUploadWithAttributes returns an UploadFunc which copies all data
// from the file on disk into Object Storage and also sets the provided
// attributes on the object.
func FileUploadWithAttributes(file string, attrs *blob.WriterOptions) UploadFunc {
	return func(obj *blob.Bucket, dest string) error {
		reader, err := os.Open(file)
		if err != nil {
			return err
		}

		uploadErr := DataUploadWithAttributes(reader, attrs)(obj, dest)
		closeErr := reader.Close()

		return errorutil.NewAggregate(uploadErr, closeErr)
	}
}

// DataUpload returns an UploadFunc which copies all
// data from src reader into GCS.
func DataUpload(src io.Reader) UploadFunc {
	return DataUploadWithAttributes(src, nil)
}

// DataUploadWithMetadata returns an UploadFunc which copies all
// data from src reader into GCS and also sets the provided metadata
// fields onto the object.
func DataUploadWithMetadata(src io.Reader, metadata map[string]string) UploadFunc {
	return DataUploadWithAttributes(src, &blob.WriterOptions{Metadata: metadata})
}

// DataUploadWithAttributes returns an UploadFunc which copies all
// data from src reader into Object Storage and also sets the provided
// attributes on the object.
func DataUploadWithAttributes(src io.Reader, attrs *blob.WriterOptions) UploadFunc {
	return func(obj *blob.Bucket, dest string) error {

		writer, writerErr := obj.NewWriter(context.Background(), dest, attrs)
		_, copyErr := io.Copy(writer, src)
		closeErr := writer.Close()

		return errorutil.NewAggregate(writerErr, copyErr, closeErr)
	}
}
