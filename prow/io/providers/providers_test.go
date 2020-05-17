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

package providers_test

import (
	"testing"

	"k8s.io/test-infra/prow/io/providers"
)

func TestHasStorageProviderPrefix(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "gs prefix",
			path: "gs/kubernetes-jenkins",
			want: true,
		},
		{
			name: "s3 prefix",
			path: "gs/kubernetes-jenkins",
			want: true,
		},
		{
			name: "no prefix",
			path: "kubernetes-jenkins",
			want: false,
		},
		{
			name: "no prefix bucket starts with storage provider id",
			path: "gs-bucket",
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := providers.HasStorageProviderPrefix(tt.path); got != tt.want {
				t.Errorf("HasStorageProviderPrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseStoragePath(t *testing.T) {
	type args struct {
		storagePath string
	}
	tests := []struct {
		name                string
		args                args
		wantStorageProvider string
		wantBucket          string
		wantRelativePath    string
		wantErr             bool
	}{
		{
			name:                "parse s3 path",
			args:                args{storagePath: "s3://prow-artifacts/test"},
			wantStorageProvider: providers.S3,
			wantBucket:          "prow-artifacts",
			wantRelativePath:    "test",
			wantErr:             false,
		},
		{
			name:                "parse s3 deep path",
			args:                args{storagePath: "s3://prow-artifacts/pr-logs/test"},
			wantStorageProvider: providers.S3,
			wantBucket:          "prow-artifacts",
			wantRelativePath:    "pr-logs/test",
			wantErr:             false,
		},
		{
			name:                "parse gs path",
			args:                args{storagePath: "gs://prow-artifacts/pr-logs/bazel-build/test.log"},
			wantStorageProvider: providers.GS,
			wantBucket:          "prow-artifacts",
			wantRelativePath:    "pr-logs/bazel-build/test.log",
			wantErr:             false,
		},
		{
			name:                "parse gs short path",
			args:                args{storagePath: "gs://prow-artifacts"},
			wantStorageProvider: providers.GS,
			wantBucket:          "prow-artifacts",
			wantRelativePath:    "",
			wantErr:             false,
		},
		{
			name:    "parse gs to short path fails",
			args:    args{storagePath: "gs://"},
			wantErr: true,
		},
		{
			name:                "parse unknown prefix path",
			args:                args{storagePath: "s4://prow-artifacts/pr-logs/bazel-build/test.log"},
			wantStorageProvider: "s4",
			wantBucket:          "prow-artifacts",
			wantRelativePath:    "pr-logs/bazel-build/test.log",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStorageProvider, gotBucket, gotRelativePath, err := providers.ParseStoragePath(tt.args.storagePath)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseStoragePath() error = %v, wantErr %v", err, tt.wantErr)
			}
			if gotStorageProvider != tt.wantStorageProvider {
				t.Errorf("ParseStoragePath() gotStorageProvider = %v, want %v", gotStorageProvider, tt.wantStorageProvider)
			}
			if gotBucket != tt.wantBucket {
				t.Errorf("ParseStoragePath() gotBucket = %v, want %v", gotBucket, tt.wantBucket)
			}
			if gotRelativePath != tt.wantRelativePath {
				t.Errorf("ParseStoragePath() gotRelativePath = %v, want %v", gotRelativePath, tt.wantRelativePath)
			}
		})
	}
}
