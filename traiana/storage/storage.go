package storage

import (
	"cloud.google.com/go/storage"
	"context"
	"k8s.io/test-infra/traiana"
	"k8s.io/test-infra/traiana/awsapi"
	"k8s.io/test-infra/traiana/storage/option"
)

// Client wrapper - AWS or GCS
type Client struct {
	gcs *storage.Client
	aws *awsapi.Client
}

func NewClient(ctx context.Context, opt ...option.ClientOption) (*Client, error) {
	if traiana.Aws {
		aws, err := awsapi.NewClient(option.GetAws(opt))

		return &Client{
			aws: aws,
		}, err
	} else {
		gcs, err := storage.NewClient(ctx, option.GetGcs(opt)...)

		return &Client{
			gcs: gcs,
		}, err
	}
}

type ObjectHandle struct {
	gcs *storage.ObjectHandle
	aws *awsapi.ObjectHandle
}

func (c *Client) Bucket(name string) *BucketHandle {
	if traiana.Aws {
		return &BucketHandle{
			aws: c.aws.Bucket(name),
		}
	} else {
		return &BucketHandle{
			gcs: c.gcs.Bucket(name),
		}
	}
}

type StorageWriter struct {
	*storage.Writer

	aws *awsapi.Writer2Reader
}

func (sw *StorageWriter) Write(p []byte) (n int, err error) {
	if traiana.Aws {
		return sw.aws.Write(p)
	} else {
		return sw.Writer.Write(p)
	}
}
func (sw *StorageWriter) Close() error {
	if traiana.Aws {
		return sw.aws.Close()
	} else {
		return sw.Writer.Close()
	}
}

func (o *ObjectHandle) NewWriter(ctx context.Context) *StorageWriter {
	if traiana.Aws {
		return &StorageWriter{
			aws: o.aws.NewWriter(ctx),
		}
	} else {
		return &StorageWriter{
			Writer: o.gcs.NewWriter(ctx),
		}
	}
}

type StorageReader struct {
	*storage.Reader
	aws      *awsapi.Writer2Reader
}

func (sr *StorageReader) Read(p []byte) (n int, err error) {
	if traiana.Aws {
		return sr.aws.Read(p)
	} else {
		return sr.Reader.Read(p)
	}
}

func (sr *StorageReader) Close() error {
	if traiana.Aws {
		return sr.aws.Close()
	} else {
		return sr.Reader.Close()
	}
}

func (o *ObjectHandle) NewReader(ctx context.Context) (r *StorageReader, err error) {
	r = &StorageReader{}

	if traiana.Aws {
		r.aws = o.aws.NewReader(ctx)
	} else {
		r.Reader, err = o.gcs.NewReader(ctx)
	}

	return r, err
}

func (o *ObjectHandle) NewRangeReader(ctx context.Context, offset, length int64) (r *StorageReader, err error) {
	r = &StorageReader{}

	if traiana.Aws {
		r.aws = o.aws.NewRangeReader(ctx, offset, length)
	} else {
		r.Reader, err = o.gcs.NewRangeReader(ctx, offset, length)
	}

	return r, err
}

func (o *ObjectHandle) Attrs(ctx context.Context) (attrs *ObjectAttrs, err error) {
	if traiana.Aws {
		return o.aws.Attrs(ctx)
	} else {
		return o.gcs.Attrs(ctx)
	}
}


type Query = storage.Query

type ObjectAttrs = storage.ObjectAttrs