package storage

import (
	"cloud.google.com/go/storage"
	"context"
	"k8s.io/test-infra/traiana"
	"k8s.io/test-infra/traiana/awsapi"
)

// Bucket wrapper - AWS or GCS

type BucketHandle struct {
	aws *awsapi.BucketHandle
	gcs *storage.BucketHandle
}

func (b *BucketHandle) initIfNeeded() {
	if traiana.Aws {
		if b.aws == nil {
			b.aws = &awsapi.BucketHandle{}
		}
	} else {
		if b.gcs == nil {
			b.gcs = &storage.BucketHandle{}
		}
	}
}

func (b *BucketHandle) Object(name string) *ObjectHandle {
	b.initIfNeeded()

	if traiana.Aws {
		return &ObjectHandle{
			aws: b.aws.Object(name),
		}
	} else {
		return &ObjectHandle{
			gcs: b.gcs.Object(name),
		}
	}
}
func (b *BucketHandle) Objects(ctx context.Context, q *Query) *ObjectIterator {
	if traiana.Aws {
		return &ObjectIterator{
			aws: b.aws.Objects(q.Delimiter, q.Prefix, q.Versions),
		}
	} else {
		return &ObjectIterator{
			gcs: b.gcs.Objects(ctx, &storage.Query{
				Delimiter: q.Delimiter,
				Prefix:    q.Prefix,
				Versions:  q.Versions,
			}),
		}
	}
}

type ObjectIterator struct {
	aws *awsapi.ObjectIterator
	gcs *storage.ObjectIterator
}

func (i ObjectIterator) Next() (*ObjectAttrs, error) {
	if traiana.Aws {
		return nil,nil //AbugovTODO
	} else {
		n, err := i.gcs.Next()

		return &ObjectAttrs{
			Name: n.Name,
		}, err
	}
}