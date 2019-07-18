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

package gcs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"sync"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/errorutil"
)

// UploadFunc knows how to upload into an object
type UploadFunc func(writer dataWriter) error
type destToWriter func(dest string) dataWriter

// Upload uploads all of the data in the
// uploadTargets map to GCS in parallel. The map is
// keyed on GCS path under the bucket
func Upload(bucket *storage.BucketHandle, uploadTargets map[string]UploadFunc) error {
	dtw := func(dest string) dataWriter {
		return gcsObjectWriter{bucket.Object(dest).NewWriter(context.Background())}
	}
	return upload(dtw, uploadTargets)
}

// LocalExport copies all of the data in the uploadTargets map to local files in parallel. The map
// is keyed on file path under the exportDir.
func LocalExport(exportDir string, uploadTargets map[string]UploadFunc) error {
	dtw := func(dest string) dataWriter {
		return &localFileWriter{
			filePath: path.Join(exportDir, dest),
		}
	}
	return upload(dtw, uploadTargets)
}

func upload(dtw destToWriter, uploadTargets map[string]UploadFunc) error {
	errCh := make(chan error, len(uploadTargets))
	group := &sync.WaitGroup{}
	group.Add(len(uploadTargets))
	for dest, upload := range uploadTargets {
		log := logrus.WithField("dest", dest)
		log.Info("Queued for upload")
		go func(f UploadFunc, writer dataWriter, log *logrus.Entry) {
			defer group.Done()
			if err := f(writer); err != nil {
				errCh <- err
			} else {
				log.Info("Finished upload")
			}
		}(upload, dtw(dest), log)
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
// data from the file on disk to the GCS object
func FileUpload(file string) UploadFunc {
	return FileUploadWithAttributes(file, nil)
}

// FileUploadWithMetadata returns an UploadFunc which copies all
// data from the file on disk into GCS object and also sets the provided
// metadata fields on the object.
func FileUploadWithMetadata(file string, metadata map[string]string) UploadFunc {
	return FileUploadWithAttributes(file, &storage.ObjectAttrs{Metadata: metadata})
}

// FileUploadWithAttributes returns an UploadFunc which copies all data
// from the file on disk into GCS object and also sets the provided
// attributes on the object.
func FileUploadWithAttributes(file string, attrs *storage.ObjectAttrs) UploadFunc {
	return func(writer dataWriter) error {
		reader, err := os.Open(file)
		if err != nil {
			return err
		}

		uploadErr := DataUploadWithAttributes(reader, attrs)(writer)
		if uploadErr != nil {
			uploadErr = fmt.Errorf("upload error: %v", uploadErr)
		}
		closeErr := reader.Close()
		if closeErr != nil {
			closeErr = fmt.Errorf("reader close error: %v", closeErr)
		}

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
	return DataUploadWithAttributes(src, &storage.ObjectAttrs{Metadata: metadata})
}

// DataUploadWithAttributes returns an UploadFunc which copies all data
// from src reader into GCS and also sets the provided attributes on
// the object.
func DataUploadWithAttributes(src io.Reader, attrs *storage.ObjectAttrs) UploadFunc {
	return func(writer dataWriter) error {
		writer.ApplyAttributes(attrs)
		_, copyErr := io.Copy(writer, src)
		if copyErr != nil {
			copyErr = fmt.Errorf("copy error: %v", copyErr)
		}
		closeErr := writer.Close()
		if closeErr != nil {
			closeErr = fmt.Errorf("writer close error: %v", closeErr)
		}
		return errorutil.NewAggregate(copyErr, closeErr)
	}
}

type dataWriter interface {
	io.WriteCloser
	ApplyAttributes(*storage.ObjectAttrs)
}

type gcsObjectWriter struct {
	*storage.Writer
}

func (w gcsObjectWriter) ApplyAttributes(attrs *storage.ObjectAttrs) {
	if attrs == nil {
		return
	}
	attrs.Name = w.Writer.ObjectAttrs.Name
	w.Writer.ObjectAttrs = *attrs
}

type localFileWriter struct {
	filePath string
	file     *os.File
}

func (w *localFileWriter) Write(b []byte) (int, error) {
	if w.file == nil {
		var err error
		w.file, err = os.OpenFile(w.filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
		if err != nil {
			return 0, fmt.Errorf("error opening %q for writing: %v", w.filePath, err)
		}
	}
	n, err := w.file.Write(b)
	if err != nil {
		return n, fmt.Errorf("error writing to %q: %v", w.filePath, err)
	}
	return n, nil
}

func (w *localFileWriter) Close() error {
	return w.file.Close()
}

// Ignore attributes when copying files locally.
func (w *localFileWriter) ApplyAttributes(_ *storage.ObjectAttrs) {}
