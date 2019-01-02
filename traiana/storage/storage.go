package storage

import (
	"cloud.google.com/go/storage"
	"context"
	"google.golang.org/api/option"
	"k8s.io/test-infra/traiana"
	"k8s.io/test-infra/traiana/awsapi"
)

// Client wrapper - AWS or GCS

type Client struct {
	gcs *storage.Client
	aws *awsapi.Client
}

func NewClient(ctx context.Context, opts ...option.ClientOption) (*Client, error) {
	gcs, err := storage.NewClient(ctx, opts...)

	return &Client{
		gcs: gcs,
	}, err
}

type ObjectHandle struct {
	gcs *storage.ObjectHandle
	aws *awsapi.ObjectHandle
}

func (c *Client) Bucket(name string) *BucketHandle {
	if traiana.Traiana {
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
	gcs      *storage.Writer
	aws      *awsapi.S3Writer
}

func (sw *StorageWriter) Write(p []byte) (n int, err error) {
	if traiana.Traiana {
		return sw.aws.Write(p)
	} else {
		return sw.gcs.Write(p)
	}
}
func (sw *StorageWriter) Close() error {
	if traiana.Traiana {
		return sw.aws.Close()
	} else {
		return sw.gcs.Close()

	}
}
func (sw *StorageWriter) SetMetadata(metadata map[string]string) {
	if traiana.Traiana {
	} else {
		sw.gcs.Metadata = metadata
	}
}

func (o *ObjectHandle) NewWriter(ctx context.Context) *StorageWriter {
	if traiana.Traiana {
		return &StorageWriter{
			aws: o.aws.NewWriter(ctx),
		}
	} else {
		return &StorageWriter{
			gcs: o.gcs.NewWriter(ctx),
		}
	}
}
