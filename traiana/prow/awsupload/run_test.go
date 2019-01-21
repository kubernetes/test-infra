package awsupload

import (
	"context"
	"github.com/gophercloud/gophercloud/testhelper"
	"github.com/stretchr/testify/assert"
	"k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/gcs"
	"k8s.io/test-infra/traiana/storage"
	"k8s.io/test-infra/traiana/storage/option"
	"strings"
	"testing"
)

func Test_Run(t *testing.T) {
	spec := &downwardapi.JobSpec{
		Type: kube.PresubmitJob,
		Refs: &kube.Refs {
			Org: "Traiana",
			Repo: "prow",
			Pulls: []v1.Pull{
				{
					Number: 1,
					Author: "zelda",
				},
			},
		},
		Job: "job1",
		BuildID: "build1",
	}

	conf := &kube.GCSConfiguration {
		Bucket: "okro-prow-test",
		PathStrategy: kube.PathStrategySingle,
		DefaultOrg: "",
		DefaultRepo: "",
		PathPrefix: "",

	}

	items := []string{
		"testdata/artifact_folder",
		"testdata/artifact1",
	}

	opt := gcsupload.Options {
		GCSConfiguration: conf,
		Items: items,
		SubDir: "",
		DryRun: false,
	}

	err := opt.Run(spec, nil)

	testhelper.AssertNoErr(t, err)
}

func Test_RealUpload(t *testing.T) {
	client, err := storage.NewClient(context.Background(), option.WithCredentialsFile("/users/Traiana/alexa/.aws/credentials"))
	assert.NoError(t, err)

	b := client.Bucket("okro-prow-test")

	targets := map[string]gcs.UploadFunc{}

	targets["/users/Traiana/alexa/Downloads/d.txt"] = gcs.DataUploadWithMetadata(strings.NewReader("dd"), map[string]string{})

	err = gcs.Upload(b, targets)
	assert.NoError(t, err)
}
