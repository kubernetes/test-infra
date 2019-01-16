package awsapi

import (
	"cloud.google.com/go/storage"
	"context"
	"google.golang.org/api/iterator"
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

func (o *ObjectHandle) NewWriter(context context.Context) *Writer2Reader {
	b := Bucket(o.b.bucket, o.b.client)

	return NewWriter2Reader(func(reader io.Reader) error {
		return S3Upload(reader, b, o.key)
	})
}
func (o *ObjectHandle) NewReader(ctx context.Context) *Reader2Writer {
	return o.NewRangeReader(ctx, 0, -1)
}

func (o *ObjectHandle) NewRangeReader(ctx context.Context, offset, length int64) *Reader2Writer {
	b := Bucket(o.b.bucket, o.b.client)

	return NewReader2Writer(func(writer io.WriterAt) (int64, error) {
		return S3Download(writer, b, o.key, offset, length)
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

//AbugovTODO
type ObjectIterator struct {
	bucket   *BucketHandle
	//query    Query
	pageInfo *iterator.PageInfo
	nextFunc func() error
	items    []*ObjectAttrs
}

func (it *ObjectIterator) Next() (*ObjectAttrs, error) {
	if err := it.nextFunc(); err != nil {
		return nil, err
	}
	item := it.items[0]
	it.items = it.items[1:]
	return item, nil
}

func (b *BucketHandle) Objects(q *Query) *ObjectIterator {
	panic("AbugovTODO")

	/*it := &ObjectIterator{
		ctx:    ctx,
		bucket: b,
	}
	it.pageInfo, it.nextFunc = iterator.NewPageInfo(
		it.fetch,
		func() int { return len(it.items) },
		func() interface{} { b := it.items; it.items = nil; return b })
	if q != nil {
		it.query = *q
	}
	return it*/
}

/*
func (it *ObjectIterator) fetch(pageSize int, pageToken string) (string, error) {

	req := it.bucket.c.raw.Objects.List(it.bucket.name)
	setClientHeader(req.Header())
	req.Projection("full")
	req.Delimiter(it.query.Delimiter)
	req.Prefix(it.query.Prefix)
	req.Versions(it.query.Versions)
	req.PageToken(pageToken)
	if it.bucket.userProject != "" {
		req.UserProject(it.bucket.userProject)
	}
	if pageSize > 0 {
		req.MaxResults(int64(pageSize))
	}
	var resp *raw.Objects
	var err error
	err = runWithRetry(it.ctx, func() error {
		resp, err = req.Context(it.ctx).Do()
		return err
	})
	if err != nil {
		if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
			err = ErrBucketNotExist
		}
		return "", err
	}
	for _, item := range resp.Items {
		it.items = append(it.items, newObject(item))
	}
	for _, prefix := range resp.Prefixes {
		it.items = append(it.items, &storage.ObjectAttrs{Prefix: prefix})
	}
	return resp.NextPageToken, nil
}
*/

type ObjectAttrs = storage.ObjectAttrs

type Query = storage.Query