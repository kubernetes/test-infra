package awsupload

import (
	"cloud.google.com/go/storage"
	"fmt"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/pod-utils/gcs"
	"k8s.io/test-infra/traiana/prow/awsapi"
	"k8s.io/test-infra/traiana/prow/pod-utils/aws"
)

func Run(uploadTargets map[string]gcs.UploadFunc, dryRun bool) error {

	var targets []aws.UploadTarget

	for dest, upload := range uploadTargets {
		targets = append(targets, aws.UploadTarget{
			Sourcepath: "",
			Bucket:     upload.,
			Dest:       "",
		})
	}

	if !dryRun {
		//ctx := context.Background()

		session, err := awsapi.NewSession()

		if err != nil {
			return fmt.Errorf("could not connect to AWS: %v", err)
		}

		aws.Upload(session, targets)

	} else {
		for destination := range uploadTargets {
			logrus.WithField("dest", destination).Info("Would upload")
		}
	}

	logrus.Info("Finished upload to AWS")
	return nil
}