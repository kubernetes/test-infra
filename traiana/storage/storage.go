package storage

import (
	"cloud.google.com/go/storage"
	"context"
	"google.golang.org/api/option"
)

type Client struct {
	gcs *storage.Client
}

func NewClient(ctx context.Context, opts ...option.ClientOption) (*Client, error) {
	gcs, err := storage.NewClient(ctx, opts...)

	return &Client{
		gcs: gcs,
	}, err
}

type ObjectHandle struct {
	gcs *storage.ObjectHandle
}

func (c *Client) Bucket(name string) *BucketHandle {
	return &BucketHandle{
		gcs: c.gcs.Bucket(name),
	}
}

func (o *ObjectHandle) NewWriter(ctx context.Context) *storage.Writer {
	return o.gcs.NewWriter(ctx)
}
