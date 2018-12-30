package awsupload

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/pod-utils/gcs"
	"k8s.io/test-infra/traiana/prow/awsapi"
	"k8s.io/test-infra/traiana/prow/pod-utils/aws"
)

func Run(uploadTargets map[string]gcs.UploadFunc, dryRun bool) error {
	if !dryRun {
		//ctx := context.Background()

		session, err := awsapi.NewSession()

		if err != nil {
			return fmt.Errorf("could not connect to AWS: %v", err)
		}

		aws.Upload(session, nil)

	} else {
		for destination := range uploadTargets {
			logrus.WithField("dest", destination).Info("Would upload")
		}
	}

	logrus.Info("Finished upload to AWS")
	return nil
}