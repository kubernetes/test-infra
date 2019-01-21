package awsapi

import (
	"cloud.google.com/go/storage"
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

func (h *ObjectHandle) Attrs(ctx context.Context) (*ObjectAttrs, error) {
	panic("AbugovTODO")

	/*
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/storage.Object.Attrs")
	defer func() { trace.EndSpan(ctx, err) }()

	if err := o.validate(); err != nil {
		return nil, err
	}
	call := o.c.raw.Objects.Get(o.bucket, o.object).Projection("full").Context(ctx)
	if err := applyConds("Attrs", o.gen, o.conds, call); err != nil {
		return nil, err
	}
	if o.userProject != "" {
		call.UserProject(o.userProject)
	}
	if err := setEncryptionHeaders(call.Header(), o.encryptionKey, false); err != nil {
		return nil, err
	}
	var obj *raw.Object
	setClientHeader(call.Header())
	err = runWithRetry(ctx, func() error { obj, err = call.Do(); return err })
	if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
		return nil, ErrObjectNotExist
	}
	if err != nil {
		return nil, err
	}
	return newObject(obj), nil
	 */
}

func (b *BucketHandle) Object(name string) *ObjectHandle {
	return &ObjectHandle{
		b: b,
		key: name,
	}
}

func (b *BucketHandle) Objects(q *Query) *S3ObjectIterator {
	return NewS3ObjectIterator(b, q)
}

type ObjectAttrs = storage.ObjectAttrs

type Query = storage.Query