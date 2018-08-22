package gcsFakes

import (
	"context"
	"log"

	"cloud.google.com/go/storage"

	"io"
	"k8s.io/test-infra/coverage/artifacts/artsTest"
)

type fakeStorageClient struct{}

func NewFakeStorageClient() *fakeStorageClient {
	return &fakeStorageClient{}
}

func (client *fakeStorageClient) Bucket(bucketName string) *storage.BucketHandle {
	return nil
}

func (client *fakeStorageClient) ListGcsObjects(ctx context.Context, bucketName, prefix, delim string) (objects []string) {
	log.Printf("fakeStorageClient.ListGcsObjects\n")
	return []string{"3", "9", "1", "5"}
}

func (client *fakeStorageClient) DoesObjectExist(ctx context.Context, bucket, object string) bool {
	log.Printf("running fakeStorageClient.DoesObjectExist(Ctx, bucket=%s, object=%s)\n",
		bucket, object)
	if bucket == "do-not-exist" || object == "do-not-exist" {
		return false
	}
	return true
}

func (client *fakeStorageClient) ProfileReader(ctx context.Context, bucket, object string) io.ReadCloser {
	return artsTest.LocalInputArtsForTest().ProfileReader()
}
