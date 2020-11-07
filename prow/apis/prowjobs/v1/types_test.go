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

package v1

import (
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/gofuzz"
)

func TestDecorationDefaultingDoesntOverwrite(t *testing.T) {
	truth := true
	lies := false

	var testCases = []struct {
		name     string
		provided *DecorationConfig
		// Note: def is a copy of the defaults and may be modified.
		expected func(orig, def *DecorationConfig) *DecorationConfig
	}{
		{
			name:     "nothing provided",
			provided: &DecorationConfig{},
			expected: func(orig, def *DecorationConfig) *DecorationConfig {
				return def
			},
		},
		{
			name: "timeout provided",
			provided: &DecorationConfig{
				Timeout: &Duration{Duration: 10 * time.Minute},
			},
			expected: func(orig, def *DecorationConfig) *DecorationConfig {
				def.Timeout = orig.Timeout
				return def
			},
		},
		{
			name: "grace period provided",
			provided: &DecorationConfig{
				GracePeriod: &Duration{Duration: 10 * time.Hour},
			},
			expected: func(orig, def *DecorationConfig) *DecorationConfig {
				def.GracePeriod = orig.GracePeriod
				return def
			},
		},
		{
			name: "utility images provided",
			provided: &DecorationConfig{
				UtilityImages: &UtilityImages{
					CloneRefs:  "clonerefs-special",
					InitUpload: "initupload-special",
					Entrypoint: "entrypoint-special",
					Sidecar:    "sidecar-special",
				},
			},
			expected: func(orig, def *DecorationConfig) *DecorationConfig {
				def.UtilityImages = orig.UtilityImages
				return def
			},
		},
		{
			name: "gcs configuration provided",
			provided: &DecorationConfig{
				GCSConfiguration: &GCSConfiguration{
					Bucket:       "bucket-1",
					PathPrefix:   "prefix-2",
					PathStrategy: PathStrategyExplicit,
					DefaultOrg:   "org2",
					DefaultRepo:  "repo2",
				},
			},
			expected: func(orig, def *DecorationConfig) *DecorationConfig {
				def.GCSConfiguration = orig.GCSConfiguration
				return def
			},
		},
		{
			name: "gcs secret name provided",
			provided: &DecorationConfig{
				GCSCredentialsSecret: "somethingSecret",
			},
			expected: func(orig, def *DecorationConfig) *DecorationConfig {
				def.GCSCredentialsSecret = orig.GCSCredentialsSecret
				return def
			},
		},
		{
			name: "s3 secret name provided",
			provided: &DecorationConfig{
				S3CredentialsSecret: "overwritten",
			},
			expected: func(orig, def *DecorationConfig) *DecorationConfig {
				def.S3CredentialsSecret = orig.S3CredentialsSecret
				return def
			},
		},
		{
			name: "ssh secrets provided",
			provided: &DecorationConfig{
				SSHKeySecrets: []string{"my", "special"},
			},
			expected: func(orig, def *DecorationConfig) *DecorationConfig {
				def.SSHKeySecrets = orig.SSHKeySecrets
				return def
			},
		},

		{
			name: "utility images partially provided",
			provided: &DecorationConfig{
				UtilityImages: &UtilityImages{
					CloneRefs:  "clonerefs-special",
					InitUpload: "initupload-special",
				},
			},
			expected: func(orig, def *DecorationConfig) *DecorationConfig {
				def.UtilityImages.CloneRefs = orig.UtilityImages.CloneRefs
				def.UtilityImages.InitUpload = orig.UtilityImages.InitUpload
				return def
			},
		},
		{
			name: "gcs configuration partially provided",
			provided: &DecorationConfig{
				GCSConfiguration: &GCSConfiguration{
					Bucket: "bucket-1",
				},
			},
			expected: func(orig, def *DecorationConfig) *DecorationConfig {
				def.GCSConfiguration.Bucket = orig.GCSConfiguration.Bucket
				return def
			},
		},
		{
			name: "skip_cloning provided",
			provided: &DecorationConfig{
				SkipCloning: &lies,
			},
			expected: func(orig, def *DecorationConfig) *DecorationConfig {
				def.SkipCloning = orig.SkipCloning
				return def
			},
		},
		{
			name: "ssh host fingerprints provided",
			provided: &DecorationConfig{
				SSHHostFingerprints: []string{"unique", "print"},
			},
			expected: func(orig, def *DecorationConfig) *DecorationConfig {
				def.SSHHostFingerprints = orig.SSHHostFingerprints
				return def
			},
		},
	}

	for _, testCase := range testCases {
		tc := testCase
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defaults := &DecorationConfig{
				Timeout:     &Duration{Duration: 1 * time.Minute},
				GracePeriod: &Duration{Duration: 10 * time.Second},
				UtilityImages: &UtilityImages{
					CloneRefs:  "clonerefs",
					InitUpload: "initupload",
					Entrypoint: "entrypoint",
					Sidecar:    "sidecar",
				},
				GCSConfiguration: &GCSConfiguration{
					Bucket:       "bucket",
					PathPrefix:   "prefix",
					PathStrategy: PathStrategyLegacy,
					DefaultOrg:   "org",
					DefaultRepo:  "repo",
				},
				GCSCredentialsSecret: "secretName",
				S3CredentialsSecret:  "s3-secret",
				SSHKeySecrets:        []string{"first", "second"},
				SSHHostFingerprints:  []string{"primero", "segundo"},
				SkipCloning:          &truth,
			}

			expected := tc.expected(tc.provided, defaults)
			actual := tc.provided.ApplyDefault(defaults)
			if diff := cmp.Diff(actual, expected, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("expected defaulted config but got diff %v", diff)
			}
		})
	}
}

func TestApplyDefaultsAppliesDefaultsForAllFields(t *testing.T) {
	t.Parallel()
	for i := 0; i < 100; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			def := &DecorationConfig{}
			fuzz.New().Fuzz(def)

			defaulted := (&DecorationConfig{}).ApplyDefault(def)

			if diff := cmp.Diff(def, defaulted); diff != "" {
				t.Errorf("defaulted decoration config didn't get all fields defaulted: %s", diff)
			}
		})
	}
}

func TestRefsToString(t *testing.T) {
	var tests = []struct {
		name     string
		ref      Refs
		expected string
	}{
		{
			name: "Refs with Pull",
			ref: Refs{
				BaseRef: "master",
				BaseSHA: "deadbeef",
				Pulls: []Pull{
					{
						Number: 123,
						SHA:    "abcd1234",
					},
				},
			},
			expected: "master:deadbeef,123:abcd1234",
		},
		{
			name: "Refs with multiple Pulls",
			ref: Refs{
				BaseRef: "master",
				BaseSHA: "deadbeef",
				Pulls: []Pull{
					{
						Number: 123,
						SHA:    "abcd1234",
					},
					{
						Number: 456,
						SHA:    "dcba4321",
					},
				},
			},
			expected: "master:deadbeef,123:abcd1234,456:dcba4321",
		},
		{
			name: "Refs with BaseRef only",
			ref: Refs{
				BaseRef: "master",
			},
			expected: "master",
		},
		{
			name: "Refs with BaseRef and BaseSHA",
			ref: Refs{
				BaseRef: "master",
				BaseSHA: "deadbeef",
			},
			expected: "master:deadbeef",
		},
	}

	for _, test := range tests {
		actual, expected := test.ref.String(), test.expected
		if actual != expected {
			t.Errorf("%s: got ref string: %s, but expected: %s", test.name, actual, expected)
		}
	}
}

func TestRerunAuthConfigValidate(t *testing.T) {
	var testCases = []struct {
		name        string
		config      *RerunAuthConfig
		errExpected bool
	}{
		{
			name:        "disallow all",
			config:      &RerunAuthConfig{AllowAnyone: false},
			errExpected: false,
		},
		{
			name:        "no restrictions",
			config:      &RerunAuthConfig{},
			errExpected: false,
		},
		{
			name:        "allow any",
			config:      &RerunAuthConfig{AllowAnyone: true},
			errExpected: false,
		},
		{
			name:        "restrict orgs",
			config:      &RerunAuthConfig{GitHubOrgs: []string{"istio"}},
			errExpected: false,
		},
		{
			name:        "restrict orgs and users",
			config:      &RerunAuthConfig{GitHubOrgs: []string{"istio", "kubernetes"}, GitHubUsers: []string{"clarketm", "scoobydoo"}},
			errExpected: false,
		},
		{
			name:        "allow any and has restriction",
			config:      &RerunAuthConfig{AllowAnyone: true, GitHubOrgs: []string{"istio"}},
			errExpected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			if err := tc.config.Validate(); (err != nil) != tc.errExpected {
				t.Errorf("Expected error %v, got %v", tc.errExpected, err)
			}
		})
	}
}

func TestRerunAuthConfigIsAuthorized(t *testing.T) {
	var testCases = []struct {
		name       string
		user       string
		config     *RerunAuthConfig
		authorized bool
	}{
		{
			name:       "authorized - AllowAnyone is true",
			user:       "gumby",
			config:     &RerunAuthConfig{AllowAnyone: true},
			authorized: true,
		},
		{
			name:       "authorized - user in GitHubUsers",
			user:       "gumby",
			config:     &RerunAuthConfig{GitHubUsers: []string{"gumby"}},
			authorized: true,
		},
		{
			name:       "unauthorized - RerunAuthConfig is nil",
			user:       "gumby",
			config:     nil,
			authorized: false,
		},
		{
			name:       "unauthorized - cli is nil",
			user:       "gumby",
			config:     &RerunAuthConfig{},
			authorized: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			if actual, _ := tc.config.IsAuthorized(tc.user, nil); actual != tc.authorized {
				t.Errorf("Expected %v, got %v", tc.authorized, actual)
			}
		})
	}
}

func TestRerunAuthConfigIsAllowAnyone(t *testing.T) {
	var testCases = []struct {
		name     string
		config   *RerunAuthConfig
		expected bool
	}{
		{
			name:     "AllowAnyone is true",
			config:   &RerunAuthConfig{AllowAnyone: true},
			expected: true,
		},
		{
			name:     "AllowAnyone is false",
			config:   &RerunAuthConfig{AllowAnyone: false},
			expected: false,
		},
		{
			name:     "AllowAnyone is unset",
			config:   &RerunAuthConfig{},
			expected: false,
		},
		{
			name:     "RerunAuthConfig is nil",
			config:   nil,
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			if actual := tc.config.IsAllowAnyone(); actual != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, actual)
			}
		})
	}
}

func TestParsePath(t *testing.T) {
	type args struct {
		bucket string
	}
	tests := []struct {
		name                string
		args                args
		wantStorageProvider string
		wantBucket          string
		wantFullPath        string
		wantErr             string
	}{
		{
			name: "valid gcs bucket",
			args: args{
				bucket: "prow-artifacts",
			},
			wantStorageProvider: "gs",
			wantBucket:          "prow-artifacts",
			wantFullPath:        "prow-artifacts",
		},
		{
			name: "valid gcs bucket with storage provider prefix",
			args: args{
				bucket: "gs://prow-artifacts",
			},
			wantStorageProvider: "gs",
			wantBucket:          "prow-artifacts",
			wantFullPath:        "prow-artifacts",
		},
		{
			name: "valid gcs bucket with multiple separator with storage provider prefix",
			args: args{
				bucket: "gs://my-floppy-backup/a://doom2.wad.006",
			},
			wantStorageProvider: "gs",
			wantBucket:          "my-floppy-backup",
			wantFullPath:        "my-floppy-backup/a://doom2.wad.006",
		},
		{
			name: "valid s3 bucket with storage provider prefix",
			args: args{
				bucket: "s3://prow-artifacts",
			},
			wantStorageProvider: "s3",
			wantBucket:          "prow-artifacts",
			wantFullPath:        "prow-artifacts",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prowPath, err := ParsePath(tt.args.bucket)
			var gotErr string
			if err != nil {
				gotErr = err.Error()
			}
			if gotErr != tt.wantErr {
				t.Errorf("ParsePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if prowPath.StorageProvider() != tt.wantStorageProvider {
				t.Errorf("ParsePath() gotStorageProvider = %v, wantStorageProvider %v", prowPath.StorageProvider(), tt.wantStorageProvider)
			}
			if prowPath.Bucket() != tt.wantBucket {
				t.Errorf("ParsePath() gotBucket = %v, wantBucket %v", prowPath.Bucket(), tt.wantBucket)
			}
			if prowPath.FullPath() != tt.wantFullPath {
				t.Errorf("ParsePath() gotFullPath = %v, wantFullPath %v", prowPath.FullPath(), tt.wantBucket)
			}
		})
	}
}
