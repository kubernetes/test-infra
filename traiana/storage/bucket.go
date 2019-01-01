package storage

import
(
	"cloud.google.com/go/storage"
)

type BucketHandle struct {
	gcs *storage.BucketHandle
}

func (b *BucketHandle) initIfNeeded() {
	if b.gcs == nil {
		b.gcs = &storage.BucketHandle{}
	}
}

func (b *BucketHandle) Object(name string) *ObjectHandle {
	b.initIfNeeded()

	return &ObjectHandle{
		gcs: b.gcs.Object(name),
	}

}