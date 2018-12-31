package awsapi

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"io"
)

type BucketHandle struct {
	bucket string
	session *session.Session
}

type BucketWriter struct {
	handle *BucketHandle
	key string
}

func NewBucketWriter(handle *BucketHandle, key string) *BucketWriter {
	return &BucketWriter{
		handle: handle,
		key: key,
	}
}

func (writer BucketWriter) Write(p []byte) (n int, err error) {
	panic("BucketWriter.Write is not allowed, use ReadFrom instead")
}

func (writer BucketWriter) Close() error {
	panic("BucketWriter.Close is not allowed, use ReadFrom instead")
}

func Bucket(bucket string, session *session.Session) *BucketHandle {
	return &BucketHandle{
		bucket: bucket,
		session: session,
	}
}

func (w BucketWriter) ReadFrom(reader io.Reader) (written int64, err error) {
	return 0, w.S3Put(reader)
}

func (w *BucketWriter) S3Put(reader io.Reader) error {
	uploader := s3manager.NewUploader(w.handle.session)

	_, err := uploader.Upload(&s3manager.UploadInput{
		Body:   reader,
		Bucket: aws.String(w.handle.bucket),
		Key:    aws.String(w.key),
	})

	return err
}