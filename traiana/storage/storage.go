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

func NewClient(ctx context.Context, opt option.ClientOption) (*Client, error) {
	if traiana.Aws {
		aws, err := awsapi.NewClient(opt.Aws)

		return &Client{
			aws: aws,
		}, err
	} else {
		gcs, err := storage.NewClient(ctx, opt.Gcs)

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
	gcs      *storage.Writer
	aws      *awsapi.S3Writer
	Metadata map[string]string // You must call SetMetadata() after setting this field
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
func (sw *StorageWriter) SetMetadata() {
	if !traiana.Aws {
		sw.gcs.Metadata = sw.Metadata
	}
}

func (o *ObjectHandle) NewWriter(ctx context.Context) *StorageWriter {
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
