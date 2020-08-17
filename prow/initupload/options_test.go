/*
Copyright 2018 The Kubernetes Authors.

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

package initupload

import (
	"reflect"
	"testing"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/gcsupload"
)

func TestOptions_Validate(t *testing.T) {
	var testCases = []struct {
		name        string
		input       Options
		expectedErr bool
	}{
		{
			name: "minimal set ok",
			input: Options{
				Log: "testing",
				Options: &gcsupload.Options{
					DryRun: true,
					GCSConfiguration: &prowapi.GCSConfiguration{
						PathStrategy: prowapi.PathStrategyExplicit,
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "missing clone log",
			input: Options{
				Options: &gcsupload.Options{
					DryRun: true,
					GCSConfiguration: &prowapi.GCSConfiguration{
						PathStrategy: prowapi.PathStrategyExplicit,
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "missing path strategy",
			input: Options{
				Options: &gcsupload.Options{
					DryRun:           true,
					GCSConfiguration: &prowapi.GCSConfiguration{},
				},
			},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		err := testCase.input.Validate()
		if testCase.expectedErr && err == nil {
			t.Errorf("%s: expected an error but got none", testCase.name)
		}
		if !testCase.expectedErr && err != nil {
			t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
		}
	}
}

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
  "bucket": "prow-artifacts",
  "path_strategy": "explicit",
  "gcs_credentials_file": "/secrets/gcs/service-account.json",
  "dry_run": false,
  "log": "/logs/clone.json"
}
				`,
			},
			wantOptions: &Options{
				Options: &gcsupload.Options{
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket:       "prow-artifacts",
						PathStrategy: "explicit",
					},
					StorageClientOptions: flagutil.StorageClientOptions{
						GCSCredentialsFile: "/secrets/gcs/service-account.json",
					},
					DryRun: false,
				},
				Log: "/logs/clone.json",
			},
			wantErr: false,
		},
		{
			name: "Parse S3 storage options",
			args: args{
				config: `
{
  "bucket": "s3://prow-artifacts",
  "path_strategy": "explicit",
  "s3_credentials_file": "/secrets/s3-storage/service-account.json",
  "dry_run": false,
  "log": "/logs/clone.json"
}
`,
			},
			wantOptions: &Options{
				Options: &gcsupload.Options{
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket:       "s3://prow-artifacts",
						PathStrategy: "explicit",
					},
					StorageClientOptions: flagutil.StorageClientOptions{
						S3CredentialsFile: "/secrets/s3-storage/service-account.json",
					},
				},
				Log: "/logs/clone.json",
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
