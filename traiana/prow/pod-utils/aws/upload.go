package aws


import (
	"bytes"
	"cloud.google.com/go/storage"
	"context"
	"fmt"
	sdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io"
	"k8s.io/test-infra/prow/errorutil"
	"net/http"
	"os"
	"sync"
)

// UploadFunc knows how to upload into an object
type UploadFunc func(obj *storage.ObjectHandle) error

// Upload uploads all of the data in the
// uploadTargets map to GCS in parallel. The map is
// keyed on GCS path under the bucket
func Upload(session *session.Session, uploadTargets map[string]UploadFunc) error {

	bucket111 := "dev-okro-io"
	filename := "/Users/Traiana/alexa/go/src/k8s.io/test-infra/yarn.lock"

	file, err := os.Open(filename)
	if err != nil {
		return errors.WithMessage(err, "Failed to open file " + filename)
	}
	defer file.Close()

	fileInfo, _ := file.Stat()
	var size = fileInfo.Size()
	buffer := make([]byte, size)
	file.Read(buffer)

	output, err := s3.New(session).PutObject(&s3.PutObjectInput{
		Bucket:               sdk.String(bucket111),
		Key:                  sdk.String("delme/yarn.lock"),
		ACL:                  sdk.String("private"),
		Body:                 bytes.NewReader(buffer),
		ContentLength:        sdk.Int64(size),
		ContentType:          sdk.String(http.DetectContentType(buffer)),
		ContentDisposition:   sdk.String("attachment"),
		ServerSideEncryption: sdk.String("AES256"),
	})

	if err != nil {
		return fmt.Errorf("failed to upload to AWS: %v", err)
	}

	//TODO: print output based on verbosity
	_ = output


	if false {
		errCh := make(chan error, len(uploadTargets))
		group := &sync.WaitGroup{}
		group.Add(len(uploadTargets))
		for dest, upload := range uploadTargets {

			//bucket *storage.BucketHandle
			//obj := bucket.Object(dest)
			var obj *storage.ObjectHandle = nil

			logrus.WithField("dest", dest).Info("Queued for upload")
			go func(f UploadFunc, obj *storage.ObjectHandle, name string) {
				defer group.Done()
				if err := f(obj); err != nil {
					errCh <- err
				}
				logrus.WithField("dest", name).Info("Finished upload")
			}(upload, obj, dest)
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
	}

	return nil
}

// FileUpload returns an UploadFunc which copies all
// data from the file on disk to the GCS object
func FileUpload(file string) UploadFunc {
	return func(obj *storage.ObjectHandle) error {
		reader, err := os.Open(file)
		if err != nil {
			return err
		}

		uploadErr := DataUpload(reader)(obj)
		closeErr := reader.Close()

		return errorutil.NewAggregate(uploadErr, closeErr)
	}
}

// DataUpload returns an UploadFunc which copies all
// data from src reader into GCS
func DataUpload(src io.Reader) UploadFunc {
	return func(obj *storage.ObjectHandle) error {
		writer := obj.NewWriter(context.Background())
		_, copyErr := io.Copy(writer, src)
		closeErr := writer.Close()

		return errorutil.NewAggregate(copyErr, closeErr)
	}
}

// DataUploadWithMetadata returns an UploadFunc which copies all
// data from src reader into GCS and also sets the provided metadata
// fields onto the object.
func DataUploadWithMetadata(src io.Reader, metadata map[string]string) UploadFunc {
	return func(obj *storage.ObjectHandle) error {
		writer := obj.NewWriter(context.Background())
		writer.Metadata = metadata
		_, copyErr := io.Copy(writer, src)
		closeErr := writer.Close()

		return errorutil.NewAggregate(copyErr, closeErr)
	}
}
