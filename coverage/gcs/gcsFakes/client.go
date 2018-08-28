package gcsFakes

import (
	"context"

	"github.com/sirupsen/logrus"
	"io"
	"k8s.io/test-infra/coverage/artifacts/artsTest"
)

type fakeStorageClient struct{}

//NewFakeStorageClient create a new fake storage client
func NewFakeStorageClient() *fakeStorageClient {
	return &fakeStorageClient{}
}

//ListGcsObjects implements StorageClientIntf and returns made-up gcs objects names in a list
func (client *fakeStorageClient) ListGcsObjects(ctx context.Context, bucketName, prefix, delim string) (objects []string) {
	logrus.Infof("fakeStorageClient.ListGcsObjects\n")
	return []string{"3", "9", "1", "5"}
}

//DoesObjectExist implements StorageClientIntf and returns true/false based on bucket name
func (client *fakeStorageClient) DoesObjectExist(ctx context.Context, bucket, object string) bool {
	logrus.Infof("running fakeStorageClient.DoesObjectExist(Ctx, bucket=%s, object=%s)\n",
		bucket, object)
	if bucket == "do-not-exist" || object == "do-not-exist" {
		return false
	}
	return true
}

//ProfileReader implements StorageClientIntf and returns a profile reader for testing purpose
func (client *fakeStorageClient) ProfileReader(ctx context.Context, bucket, object string) io.ReadCloser {
	return artsTest.LocalInputArtsForTest().ProfileReader()
}
