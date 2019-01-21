package awsapi

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
)

func S3GetObject(handle *BucketHandle, key string) (*ObjectAttrs, error) {
	s33 := s3.New(handle.client.session)

	in := &s3.GetObjectInput{
		Bucket: aws.String(handle.bucket),
		Key: aws.String(key),
	}

	o, err := s33.GetObject(in)

	if err != nil {
		return nil, err
	}

	m := map[string]string{}

	for k := range o.Metadata {
		m[k] = aws.StringValue(o.Metadata[k])
	}

	return &ObjectAttrs {
		Name: key,
		Bucket: handle.bucket,
		Size: aws.Int64Value(o.ContentLength),
		ContentEncoding: aws.StringValue(o.ContentEncoding),
		ContentLanguage: aws.StringValue(o.ContentLanguage),
		ContentType: aws.StringValue(o.ContentType),
		Metadata: m,
		Updated: aws.TimeValue(o.LastModified),
	}, nil
}