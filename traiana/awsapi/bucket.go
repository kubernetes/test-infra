package awsapi

import (
	"context"
	"io"
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

func (h ObjectHandle) NewWriter(context context.Context) *Writer2Reader {
	b := Bucket(h.b.bucket, h.b.client)

	return NewWriter2Reader(func(reader io.Reader) error {
		return S3Put(reader, b, h.key)
	})
}

func (b *BucketHandle) Object(name string) *ObjectHandle {
	return &ObjectHandle{
		b: b,
		key: name,
	}
}