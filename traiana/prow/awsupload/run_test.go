package awsupload

import (
	"github.com/gophercloud/gophercloud/testhelper"
	"k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
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
		Bucket: "dev-okro-io",
		PathStrategy: kube.PathStrategySingle,
		DefaultOrg: "",
		DefaultRepo: "",
		PathPrefix: "",

	}

	items := []string{
		"testdata/artifact_folder",
		"testdata/artifact1",
	}

	subDir := ""

	err := Run(spec, false, conf, items, subDir)

	testhelper.AssertNoErr(t, err)
}
