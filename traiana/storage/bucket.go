package storage

import
(
	"cloud.google.com/go/storage"
	"k8s.io/test-infra/traiana"
	"k8s.io/test-infra/traiana/awsapi"
)

// Bucket wrapper - AWS or GCS

type BucketHandle struct {
	gcs *storage.BucketHandle
	aws *awsapi.BucketHandle
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