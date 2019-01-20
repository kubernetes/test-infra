package awsapi

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"google.golang.org/api/iterator"
)

type S3ObjectIterator struct {
	err error
	output *s3.ListObjectsV2Output
	current int
}

func NewS3ObjectIterator(b *BucketHandle, q *Query) *S3ObjectIterator {
	output, err := S3ListObjects(b, q.Prefix, q.Delimiter)

	return &S3ObjectIterator{
		err:    err,
		output: output,
	}
}

func (it *S3ObjectIterator) Next() (att *ObjectAttrs, err error) {
	if it.err != nil {
		return nil, it.err
	}

	it.current++

	if it.current >= len(it.output.Contents) {
		return nil, iterator.Done
	}

	return it.objectToAttrs(it.output.Contents[it.current]), err
}

func (it *S3ObjectIterator) objectToAttrs(o *s3.Object) *ObjectAttrs {
	if o == nil {
		return nil
	}

	return &ObjectAttrs {
		Name: aws.StringValue(o.Key),
		Prefix: aws.StringValue(it.output.Prefix),
	}
}