/*
Copyright 2020 The Kubernetes Authors.

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

package spyglass

import (
	"context"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/io"
)

func TestSpyglass_ListArtifacts(t *testing.T) {
	type args struct {
		src string
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "list artifacts (old format)",
			args: args{
				src: "gcs/test-bucket/logs/example-ci-run/403",
			},
			want: []string{
				"build-log.txt",
				prowv1.FinishedStatusFile,
				"junit_01.xml",
				"long-log.txt",
				prowv1.StartedStatusFile,
			},
		},
		{
			name: "list artifacts (new format)",
			args: args{
				src: "gs/test-bucket/logs/example-ci-run/403",
			},
			want: []string{
				"build-log.txt",
				prowv1.FinishedStatusFile,
				"junit_01.xml",
				"long-log.txt",
				prowv1.StartedStatusFile,
			},
		},
		{
			name: "list artifacts without results in gs (new format)",
			args: args{
				src: "gs/test-bucket/logs/example-ci-run/404",
			},
			want: []string{
				"build-log.txt",
			},
		},
		{
			name: "list artifacts without results in gs with multiple containers",
			args: args{
				src: "gs/test-bucket/logs/multiple-container-job/123",
			},
			want: []string{
				"test-1-build-log.txt",
				"test-2-build-log.txt",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeGCSClient := fakeGCSServer.Client()
			ca := &config.Agent{}
			ca.Set(&config.Config{
				ProwConfig: config.ProwConfig{
					Deck: config.Deck{
						AllKnownStorageBuckets: sets.NewString("test-bucket"),
					},
				},
			})
			sg := New(context.Background(), fakeJa, ca.Config, io.NewGCSOpener(fakeGCSClient), false)
			got, err := sg.ListArtifacts(context.Background(), tt.args.src)
			if (err != nil) != tt.wantErr {
				t.Errorf("ListArtifacts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ListArtifacts() got = %v, want %v", got, tt.want)
			}
		})
	}
}
