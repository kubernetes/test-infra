package awsapi

import (
	"context"
)

type BucketHandle struct {
	bucket string
	client *Client
}

func Bucket(bucket string, client *Client) *BucketHandle {
	return &BucketHandle{
		bucket: bucket,
		client: client,
	}
}

type ObjectHandle struct {
	b   *BucketHandle
	key string
}

func (h ObjectHandle) NewWriter(context context.Context) *S3Writer {
	return &S3Writer{
		handle: Bucket(h.b.bucket, h.b.client),
		key: h.key,
	}
}

func (b *BucketHandle) Object(name string) *ObjectHandle {
	return &ObjectHandle{
		b: b,
		key: name,
	}
}