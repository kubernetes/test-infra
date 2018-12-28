package awsupload

import (
	"cloud.google.com/go/storage"
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"
	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/pod-utils/gcs"
)

func Run(o gcsupload.Options, uploadTargets map[string]gcs.UploadFunc) error {
	if !o.DryRun {
		ctx := context.Background()
		gcsClient, err := storage.NewClient(ctx, option.WithCredentialsFile(o.GcsCredentialsFile))
		if err != nil {
			return fmt.Errorf("could not connect to GCS: %v", err)
		}

		if err := gcs.Upload(gcsClient.Bucket(o.Bucket), uploadTargets); err != nil {
			return fmt.Errorf("failed to upload to GCS: %v", err)
		}
	} else {
		for destination := range uploadTargets {
			logrus.WithField("dest", destination).Info("Would upload")
		}
	}

	logrus.Info("Finished upload to GCS")
	return nil
}