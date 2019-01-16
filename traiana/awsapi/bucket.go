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

func (h *ObjectHandle) NewWriter(context context.Context) *Writer2Reader {
	b := Bucket(h.b.bucket, h.b.client)

	return NewWriter2Reader(func(reader io.Reader) error {
		return S3Upload(reader, b, h.key)
	})
}
func (o *ObjectHandle) NewReader(ctx context.Context) (*Reader2Writer, error) {
	return o.NewRangeReader(ctx, 0, -1)
}

func (o *ObjectHandle) NewRangeReader(ctx context.Context, offset, length int64) (r *Reader2Writer, err error) {
	//d := S3Download()
	panic("AbugovTODO")

	/*
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/storage.Object.NewRangeReader")
	defer func() { trace.EndSpan(ctx, err) }()

	if err := o.validate(); err != nil {
		return nil, err
	}
	if offset < 0 {
		return nil, fmt.Errorf("storage: invalid offset %d < 0", offset)
	}
	if o.conds != nil {
		if err := o.conds.validate("NewRangeReader"); err != nil {
			return nil, err
		}
	}
	u := &url.URL{
		Scheme:   "https",
		Host:     "storage.googleapis.com",
		Path:     fmt.Sprintf("/%s/%s", o.bucket, o.object),
		RawQuery: conditionsQuery(o.gen, o.conds),
	}
	verb := "GET"
	if length == 0 {
		verb = "HEAD"
	}
	req, err := http.NewRequest(verb, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req = withContext(req, ctx)
	if o.userProject != "" {
		req.Header.Set("X-Goog-User-Project", o.userProject)
	}
	if o.readCompressed {
		req.Header.Set("Accept-Encoding", "gzip")
	}
	if err := setEncryptionHeaders(req.Header, o.encryptionKey, false); err != nil {
		return nil, err
	}

	// Define a function that initiates a Read with offset and length, assuming we
	// have already read seen bytes.
	reopen := func(seen int64) (*http.Response, error) {
		start := offset + seen
		if length < 0 && start > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
		} else if length > 0 {
			// The end character isn't affected by how many bytes we've seen.
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, offset+length-1))
		}
		var res *http.Response
		err = runWithRetry(ctx, func() error {
			res, err = o.c.hc.Do(req)
			if err != nil {
				return err
			}
			if res.StatusCode == http.StatusNotFound {
				res.Body.Close()
				return ErrObjectNotExist
			}
			if res.StatusCode < 200 || res.StatusCode > 299 {
				body, _ := ioutil.ReadAll(res.Body)
				res.Body.Close()
				return &googleapi.Error{
					Code:   res.StatusCode,
					Header: res.Header,
					Body:   string(body),
				}
			}
			if start > 0 && length != 0 && res.StatusCode != http.StatusPartialContent {
				res.Body.Close()
				return errors.New("storage: partial request not satisfied")
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return res, nil
	}

	res, err := reopen(0)
	if err != nil {
		return nil, err
	}
	var size int64 // total size of object, even if a range was requested.
	if res.StatusCode == http.StatusPartialContent {
		cr := strings.TrimSpace(res.Header.Get("Content-Range"))
		if !strings.HasPrefix(cr, "bytes ") || !strings.Contains(cr, "/") {

			return nil, fmt.Errorf("storage: invalid Content-Range %q", cr)
		}
		size, err = strconv.ParseInt(cr[strings.LastIndex(cr, "/")+1:], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("storage: invalid Content-Range %q", cr)
		}
	} else {
		size = res.ContentLength
	}

	remain := res.ContentLength
	body := res.Body
	if length == 0 {
		remain = 0
		body.Close()
		body = emptyBody
	}
	var (
		checkCRC bool
		crc      uint32
	)
	// Even if there is a CRC header, we can't compute the hash on partial data.
	if remain == size {
		crc, checkCRC = parseCRC32c(res)
	}
	return &Reader{
		body:            body,
		size:            size,
		offset:          remain,
		contentType:     res.Header.Get("Content-Type"),
		contentEncoding: res.Header.Get("Content-Encoding"),
		cacheControl:    res.Header.Get("Cache-Control"),
		wantCRC:         crc,
		checkCRC:        checkCRC,
		reopen:          reopen,
	}, nil
	 */
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