package awsapi

import (
	"context"

	"cloud.google.com/go/storage"
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

func (o *ObjectHandle) NewWriter(context context.Context) *Writer2Reader {
	b := Bucket(o.b.bucket, o.b.client)

	return NewWriter2Reader(func(wr *Writer2Reader) error {
		return S3Upload(wr, b, o.key)
	})
}

func (o *ObjectHandle) NewReader(ctx context.Context) *Writer2Reader {
	return o.NewRangeReader(ctx, 0, -1)
}

func (o *ObjectHandle) NewRangeReader(ctx context.Context, offset, length int64) *Writer2Reader {
	b := Bucket(o.b.bucket, o.b.client)

	return NewWriter2Reader(func(wr *Writer2Reader) error {
		_, err := S3Download(wr, b, o.key, offset, length)
		return err
	})
}

func (o *ObjectHandle) Attrs() (*ObjectAttrs, error) {
	return S3GetObject(o.b, o.key)
}

func (b *BucketHandle) Object(name string) *ObjectHandle {
	return &ObjectHandle{
		b:   b,
		key: name,
	}
}

func (b *BucketHandle) Objects(q *Query) *S3ObjectIterator {
	return NewS3ObjectIterator(b, q)
}

type ObjectAttrs = storage.ObjectAttrs

type Query = storage.Query
