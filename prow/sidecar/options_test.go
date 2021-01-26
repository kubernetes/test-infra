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

package sidecar

import (
	"reflect"
	"testing"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/pod-utils/wrapper"
)

func TestOptions_LoadConfig(t *testing.T) {
	type args struct {
		config string
	}
	tests := []struct {
		name        string
		args        args
		wantOptions *Options
		wantErr     bool
	}{
		{
			name: "Parse GCS options",
			args: args{
				config: `
{
  "gcs_options": {
	"items": [
      "/logs/artifacts"
    ],
    "bucket": "prow-artifacts",
    "path_strategy": "explicit",
    "gcs_credentials_file": "/secrets/gcs/service-account.json",
    "dry_run": false
  },
  "entries": [
    {
      "args": [
        "sh",
        "-c",
        "echo test"
      ],
      "process_log": "/logs/process-log.txt",
      "marker_file": "/logs/marker-file.txt",
      "metadata_file": "/logs/artifacts/metadata.json"
    }
  ]
}
				`,
			},
			wantOptions: &Options{
				GcsOptions: &gcsupload.Options{
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket:       "prow-artifacts",
						PathStrategy: "explicit",
					},
					StorageClientOptions: flagutil.StorageClientOptions{
						GCSCredentialsFile: "/secrets/gcs/service-account.json",
					},
					DryRun: false,
					Items:  []string{"/logs/artifacts"},
				},
				Entries: []wrapper.Options{
					{
						Args:         []string{"sh", "-c", "echo test"},
						ProcessLog:   "/logs/process-log.txt",
						MarkerFile:   "/logs/marker-file.txt",
						MetadataFile: "/logs/artifacts/metadata.json",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Parse S3 storage options",
			args: args{
				config: `
{
  "gcs_options": {
	"items": [
      "/logs/artifacts"
    ],
    "bucket": "s3://prow-artifacts",
    "path_strategy": "explicit",
    "s3_credentials_file": "/secrets/s3-storage/service-account.json",
    "dry_run": false
  },
  "entries": [
    {
      "args": [
        "sh",
        "-c",
        "echo test"
      ],
      "process_log": "/logs/process-log.txt",
      "marker_file": "/logs/marker-file.txt",
      "metadata_file": "/logs/artifacts/metadata.json"
    }
  ]
}
`,
			},
			wantOptions: &Options{
				GcsOptions: &gcsupload.Options{
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket:       "s3://prow-artifacts",
						PathStrategy: "explicit",
					},
					StorageClientOptions: flagutil.StorageClientOptions{
						S3CredentialsFile: "/secrets/s3-storage/service-account.json",
					},
					DryRun: false,
					Items:  []string{"/logs/artifacts"},
				},
				Entries: []wrapper.Options{
					{
						Args:         []string{"sh", "-c", "echo test"},
						ProcessLog:   "/logs/process-log.txt",
						MarkerFile:   "/logs/marker-file.txt",
						MetadataFile: "/logs/artifacts/metadata.json",
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOptions := NewOptions()
			if err := gotOptions.LoadConfig(tt.args.config); (err != nil) != tt.wantErr {
				t.Errorf("LoadConfig() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !reflect.DeepEqual(tt.wantOptions, gotOptions) {
				t.Errorf("%s: expected options to equal: %#v, but got: %#v", tt.name, tt.wantOptions, gotOptions)
			}
		})
	}
}
