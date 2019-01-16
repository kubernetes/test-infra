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
	storage.ObjectAttrs

	gcs *storage.Writer
	aws *awsapi.Writer2Reader

	// You must call CopyFields() after setting the following fields:
	SendCRC32C   bool
	ProgressFunc func(int64)
}

func (sw *StorageWriter) Write(p []byte) (n int, err error) {
	if traiana.Aws {
		return sw.aws.Write(p)
	} else {
		return sw.gcs.Write(p)
	}
}
func (sw *StorageWriter) Close() error {
	if traiana.Aws {
		return sw.aws.Close()
	} else {
		return sw.gcs.Close()
	}
}
func (sw *StorageWriter) CopyFields() {
	if !traiana.Aws {
		sw.gcs.Metadata = sw.Metadata
		sw.gcs.SendCRC32C = sw.SendCRC32C
		sw.gcs.ProgressFunc = sw.ProgressFunc
		sw.gcs.ObjectAttrs = sw.ObjectAttrs
	}
}

func (o *ObjectHandle) NewWriter(ctx context.Context) *StorageWriter {
	 g := o.gcs.NewWriter(ctx)
	w := &StorageWriter{}
	w.ObjectAttrs = g.ObjectAttrs

	if traiana.Aws {
		return &StorageWriter{
			aws: o.aws.NewWriter(ctx),
		}
	} else {
		return &StorageWriter{
			gcs: o.gcs.NewWriter(ctx),
		}
	}
}

type StorageReader struct {
	gcs      *storage.Reader
	aws      *awsapi.Reader
}

func (sr *StorageReader) Read(p []byte) (n int, err error) {
	if traiana.Aws {
		return sr.aws.Read(p)
	} else {
		return sr.gcs.Read(p)
	}
}

func (sr *StorageReader) Close() error {
	if traiana.Aws {
		return sr.aws.Close()
	} else {
		return sr.gcs.Close()
	}
}

func (o *ObjectHandle) NewReader(ctx context.Context) (r *StorageReader, err error) {
	if traiana.Aws {
		r.aws, err = o.aws.NewReader(ctx)
	} else {
		r.gcs, err = o.gcs.NewReader(ctx)
	}

	return r, err
}

func (o *ObjectHandle) NewRangeReader(ctx context.Context, offset, length int64) (r *StorageReader, err error) {
	if traiana.Aws {
		r.aws, err = o.aws.NewRangeReader(ctx, offset, length)
	} else {
		r.gcs, err = o.gcs.NewRangeReader(ctx, offset, length)
	}

	return r, err
}

func (o *ObjectHandle) Attrs(ctx context.Context) (*ObjectAttrs, error) {
	panic("implement me")
}

type Query = storage.Query

//AbugovTODO
type ObjectAttrs = storage.ObjectAttrs