// package gcs stores functions that relates to GCS operations,
// without dependency on the package calc

package gcs

import (
	"context"
	"log"
	"path"
	"strconv"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"

	"github.com/sirupsen/logrus"
	"io"
	"k8s.io/test-infra/coverage/artifacts"
	"k8s.io/test-infra/coverage/logUtil"
)

const (
	gcsUrlHost = "storage.cloud.google.com/"
)

// DoesObjectExist checks whether an object exists in GCS bucket
func (client storageClient) DoesObjectExist(ctx context.Context, bucket, object string) bool {
	_, err := client.Bucket(bucket).Object(object).Attrs(ctx)
	if err != nil {
		logrus.Infof("Error getting attrs from object '%s': %v", object, err)
		return false
	}
	return true
}

//StorageClientIntf collects methods depending on storage client. It needs to be implemented by fake
// struct as well.
type StorageClientIntf interface {
	ListGcsObjects(ctx context.Context, bucketName, prefix, delim string) (
		objects []string)
	ProfileReader(ctx context.Context, bucket, object string) io.ReadCloser
	DoesObjectExist(ctx context.Context, bucket, object string) bool
}

type storageClient struct {
	storage.Client
}

func NewStorageClient(ctx context.Context) *storageClient {
	client, err := storage.NewClient(ctx)

	if err != nil {
		logUtil.LogFatalf("Failed to create client: %v", err)
	}
	return &storageClient{*client}
}

func (client *storageClient) ListGcsObjects(ctx context.Context, bucketName,
	prefix, delim string) (objects []string) {
	it := client.Bucket(bucketName).Objects(ctx, &storage.Query{
		Prefix:    prefix,
		Delimiter: delim,
	})

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("Error iterating: %v", err)
		}

		if attrs.Prefix != "" {
			objects = append(objects, path.Base(attrs.Prefix))
		}
	}
	logrus.Info("end of ListGcsObjects(...)")
	return
}

func (client storageClient) ProfileReader(ctx context.Context, bucket,
	object string) io.ReadCloser {
	logrus.Infof("Running ProfileReader on bucket '%s', object='%s'\n",
		bucket, object)

	o := client.Bucket(bucket).Object(object)
	reader, err := o.NewReader(ctx)
	if err != nil {
		logUtil.LogFatalf("o.NewReader(Ctx) error: %v", err)
	}
	return reader
}

type GcsBuild struct {
	StorageClient StorageClientIntf
	Bucket        string
	Job           string
	Build         int
	CovThreshold  int
}

func (b *GcsBuild) BuildStr() string {
	return strconv.Itoa(b.Build)
}

type GcsArtifacts struct {
	artifacts.Artifacts
	Ctx    context.Context
	Client StorageClientIntf
	Bucket string
}

func newGcsArtifacts(ctx context.Context, client StorageClientIntf,
	bucket string, baseArtifacts artifacts.Artifacts) *GcsArtifacts {
	return &GcsArtifacts{baseArtifacts, ctx, client, bucket}
}

func (arts *GcsArtifacts) ProfileReader() io.ReadCloser {
	return arts.Client.ProfileReader(arts.Ctx, arts.Bucket, arts.ProfilePath())
}
