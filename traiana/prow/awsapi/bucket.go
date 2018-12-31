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

	//file, err := os.Open(filename)
	//////if err != nil {
	//return errors.WithMessage(err, "Failed to open file "+filename)
	//}
	//defer file.Close()

	//fileInfo, _ := file.Stat()
	//var size= fileInfo.Size()
	//buffer := make([]byte, size)
	//file.Read(buffer)

	/*output, err := s3.New(b.session).PutObject(&s3.PutObjectInput{
		Bucket:               aws.String(b.bucket),
		Key:                  aws.String("delme/yarn.lock"),
		ACL:                  aws.String("private"),
		Body:                 bytes.NewReader(buffer),
		//ContentLength:        aws.Int64(size),
		//ContentType:          aws.String(http.DetectContentType(buffer)),
		ContentDisposition:   aws.String("attachment"),
		ServerSideEncryption: aws.String("AES256"),
	})*/
	//_ = output

	return err
}