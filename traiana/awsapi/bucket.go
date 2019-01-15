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
func (h ObjectHandle) NewReader(ctx context.Context) interface{} {
	panic("AbugovTODO")
}

func (h ObjectHandle) NewRangeReader(ctx context.Context, i int64, i2 int64) *ObjectHandle {
	panic("AbugovTODO")
}

func (b *BucketHandle) Object(name string) *ObjectHandle {
	return &ObjectHandle{
		b: b,
		key: name,
	}
}

//AbugovTODO
type ObjectIterator struct {
	//bucket   *BucketHandle
	//query    Query
	//pageInfo *iterator.PageInfo
	//nextFunc func() error
	//items    []*ObjectAttrs
}

func (b *BucketHandle) Objects(delimiter string, prefix string, versions bool) *ObjectIterator {
	panic("AbugovTODO")
}