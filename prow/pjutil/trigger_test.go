/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pjutil

import (
	"fmt"
	"testing"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	pjapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	prowconfig "k8s.io/test-infra/prow/config"
)

type fakeJobResult struct {
	err error
}

func Test_getJobArtifactsURL(t *testing.T) {
	org := "redhat-operator-ecosystem"
	repo := "playground"
	bucket := "origin-ci-test"
	browserPrefix := "https://gcsweb-ci.svc.ci.openshift.org/gcs/"
	jobName := "periodic-ci-redhat-operator-ecosystem-playground-cvp-ocp-4.4-cvp-common-aws"

	prowConfig := &prowconfig.Config{
		JobConfig: prowconfig.JobConfig{},
		ProwConfig: prowconfig.ProwConfig{
			Plank: config.Plank{
				Controller: prowconfig.Controller{},
				DefaultDecorationConfigs: map[string]*pjapi.DecorationConfig{
					fmt.Sprintf("%s/%s", org, repo): {GCSConfiguration: &pjapi.GCSConfiguration{Bucket: bucket}},
				},
			},
			Deck: prowconfig.Deck{
				Spyglass: prowconfig.Spyglass{GCSBrowserPrefix: browserPrefix},
			},
		},
	}
	type args struct {
		prowJob *pjapi.ProwJob
		config  *prowconfig.Config
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Returns artifacts URL when we have .Spec.Ref",
			args: args{
				prowJob: &pjapi.ProwJob{
					TypeMeta:   v1.TypeMeta{},
					ObjectMeta: v1.ObjectMeta{Name: "jobWithSpecRef"},
					Spec: pjapi.ProwJobSpec{
						ExtraRefs: nil,
						Job:       jobName,
						Refs:      &pjapi.Refs{Org: org, Repo: repo},
						Type:      "periodic",
					},
					Status: pjapi.ProwJobStatus{State: "success", BuildID: "100"},
				},
				config: prowConfig,
			},
			want: "https://gcsweb-ci.svc.ci.openshift.org/gcs/origin-ci-test/logs/periodic-ci-redhat-operator-ecosystem-playground-cvp-ocp-4.4-cvp-common-aws/100",
		},
		{
			name: "Returns artifacts URL when we have Spec.ExtraRefs",
			args: args{
				prowJob: &pjapi.ProwJob{
					TypeMeta:   v1.TypeMeta{},
					ObjectMeta: v1.ObjectMeta{Name: "jobWithExtraRef"},
					Spec: pjapi.ProwJobSpec{
						ExtraRefs: []pjapi.Refs{
							{Org: org, Repo: repo},
							{Org: "org2", Repo: "repo2"},
						},
						Job:  jobName,
						Refs: nil,
						Type: "periodic",
					},
					Status: pjapi.ProwJobStatus{State: "success", BuildID: "101"},
				},
				config: prowConfig,
			},
			want: "https://gcsweb-ci.svc.ci.openshift.org/gcs/origin-ci-test/logs/periodic-ci-redhat-operator-ecosystem-playground-cvp-ocp-4.4-cvp-common-aws/101",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getJobArtifactsURL(tt.args.prowJob, tt.args.config); got != tt.want {
				t.Errorf("getJobArtifactsURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
