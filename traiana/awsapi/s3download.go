package awsapi

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"io"
)

func S3Download(writer io.WriterAt, handle *BucketHandle, key string, offset, length int64) (int64, error) {
	downloader := s3manager.NewDownloader(handle.client.session)

	var rng string

	if length != -1 {
		rng = fmt.Sprintf("bytes=%d-%d" , offset, offset + length - 1)
	}

	in := &s3.GetObjectInput{
		Bucket: aws.String(handle.bucket),
		Key:    aws.String(key),
		Range: aws.String(rng),

	}

	return downloader.Download(writer, in)
	//downloader.DownloadWithIterator
}