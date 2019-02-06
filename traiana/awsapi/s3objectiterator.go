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

	//AbugovTODO: might need to add the output.CommonPrefixes to the result in case prow expects folder names too

	return &S3ObjectIterator{
		err:    err,
		output: output,
	}
}

func (it *S3ObjectIterator) Next() (*ObjectAttrs, error) {
	if it.err != nil {
		return nil, it.err
	}

	if it.current == len(it.output.Contents) {
		return nil, iterator.Done
	}

	att := it.objectToAttrs(it.output.Contents[it.current])
	it.current++

	return att, nil
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