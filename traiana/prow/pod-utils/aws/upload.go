package aws

import (
	"fmt"
	"k8s.io/test-infra/traiana/prow/awsapi"
	"github.com/sirupsen/logrus"
	"io"
	"k8s.io/test-infra/prow/errorutil"
	"os"
	"sync"
)

type UploadFunc func(obj *awsapi.BucketWriter) error

func Upload(handle *awsapi.BucketHandle, uploadTargets map[string]UploadFunc) error {
	errCh := make(chan error, len(uploadTargets))
	group := &sync.WaitGroup{}
	group.Add(len(uploadTargets))
	for dest, upload := range uploadTargets {

		var writer *awsapi.BucketWriter = awsapi.NewBucketWriter(handle, dest)

		logrus.WithField("dest", dest).Info("Queued for upload")
		go func(f UploadFunc, obj *awsapi.BucketWriter, name string) {
			defer group.Done()
			if err := f(obj); err != nil {
				errCh <- err
			}
			logrus.WithField("dest", name).Info("Finished upload")
		}(upload, writer, dest)
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
// data from the file on disk to S3
func FileUpload(file string) UploadFunc {
	return func(writer *awsapi.BucketWriter) error {
		reader, err := os.Open(file)
		if err != nil {
			return err
		}

		uploadErr := DataUpload(reader)(writer)
		closeErr := reader.Close()

		return errorutil.NewAggregate(uploadErr, closeErr)
	}
}

// DataUpload returns an UploadFunc which copies all
// data from src reader into S3
func DataUpload(src io.Reader) UploadFunc {
	return func(writer *awsapi.BucketWriter) error {
		_, copyErr := writer.ReadFrom(src)
		return copyErr
	}
}
