package awsapi

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
)

func S3ListObjects(handle *BucketHandle, prefix string, delimiter string) (*s3.ListObjectsV2Output, error) {
	s33 := s3.New(handle.client.session)

	in := &s3.ListObjectsV2Input{
		Bucket: aws.String(handle.bucket),
		Prefix: aws.String(prefix),
		Delimiter: aws.String(delimiter),
	}

	return s33.ListObjectsV2(in)
}