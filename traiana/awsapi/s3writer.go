package awsapi

import (
	"io"
	"k8s.io/test-infra/traiana/prow/awsapi"
)

// S3Writer is a wrapper around S3Put. This way we can write to S3 using io.Copy
type S3Writer struct {
	source *S3Source
}

func (w *S3Writer) Write(bytes []byte) (n int, err error) {
	// on first call to Write open a new s3Source to help channelling the next Writes into S3Put (S3Put doesn't implement io.Writer)
	if w.source == nil {
		session, err := awsapi.NewSession()

		if err != nil {
			return 0, err
		}

		bw := awsapi.NewBucketWriter(
			awsapi.Bucket("dev-okro-io", session),
			"lala")

		w.source = &S3Source {
			buffer: make(chan []byte, 1),
			error: make(chan error, 1),
		}

		go s3put(bw, w.source)
	}

	w.source.buffer <- bytes

	return len(bytes), nil
}

func (w S3Writer) Close() error {
	// send empty buffer to mark completion
	w.source.buffer <- []byte{}

	// wait for completion
	err := <- w.source.error

	return err
}

func s3put(bw *awsapi.BucketWriter, src *S3Source) {
	err := bw.S3Put(src)
	src.error <- err
}

type S3Source struct {
	buffer chan []byte
	error chan error
}

func (s S3Source) Read(buffer []byte) (n int, err error) {
	buffer = <-s.buffer

	err = nil

	if len(buffer) == 0 {
		err = io.EOF
	}

	return len(buffer), err
}