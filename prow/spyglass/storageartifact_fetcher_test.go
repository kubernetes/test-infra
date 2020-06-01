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

package spyglass

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/io"
)

func TestNewGCSJobSource(t *testing.T) {
	testCases := []struct {
		name         string
		src          string
		exJobPrefix  string
		exBucket     string
		exName       string
		exBuildID    string
		exLinkPrefix string
		expectedErr  error
	}{
		{
			name:         "Test standard GCS link (old format)",
			src:          "test-bucket/logs/example-ci-run/403",
			exBucket:     "test-bucket",
			exJobPrefix:  "logs/example-ci-run/403/",
			exName:       "example-ci-run",
			exBuildID:    "403",
			exLinkPrefix: "gs://",
			expectedErr:  nil,
		},
		{
			name:         "Test GCS link with trailing / (old format)",
			src:          "test-bucket/logs/example-ci-run/403/",
			exBucket:     "test-bucket",
			exJobPrefix:  "logs/example-ci-run/403/",
			exName:       "example-ci-run",
			exBuildID:    "403",
			exLinkPrefix: "gs://",
			expectedErr:  nil,
		},
		{
			name:         "Test GCS link with org name (old format)",
			src:          "test-bucket/logs/sig-flexing/example-ci-run/403",
			exBucket:     "test-bucket",
			exJobPrefix:  "logs/sig-flexing/example-ci-run/403/",
			exName:       "example-ci-run",
			exBuildID:    "403",
			exLinkPrefix: "gs://",
			expectedErr:  nil,
		},
		{
			name:         "Test standard GCS link (new format)",
			src:          "gs://test-bucket/logs/example-ci-run/403",
			exBucket:     "test-bucket",
			exJobPrefix:  "logs/example-ci-run/403/",
			exName:       "example-ci-run",
			exBuildID:    "403",
			exLinkPrefix: "gs://",
			expectedErr:  nil,
		},
		{
			name:         "Test GCS link with trailing / (new format)",
			src:          "gs://test-bucket/logs/example-ci-run/403/",
			exBucket:     "test-bucket",
			exJobPrefix:  "logs/example-ci-run/403/",
			exName:       "example-ci-run",
			exBuildID:    "403",
			exLinkPrefix: "gs://",
			expectedErr:  nil,
		},
		{
			name:         "Test GCS link with org name (new format)",
			src:          "gs://test-bucket/logs/sig-flexing/example-ci-run/403",
			exBucket:     "test-bucket",
			exJobPrefix:  "logs/sig-flexing/example-ci-run/403/",
			exName:       "example-ci-run",
			exBuildID:    "403",
			exLinkPrefix: "gs://",
			expectedErr:  nil,
		},
		{
			name:         "Test standard S3 link",
			src:          "s3://test-bucket/logs/example-ci-run/403",
			exBucket:     "test-bucket",
			exJobPrefix:  "logs/example-ci-run/403/",
			exName:       "example-ci-run",
			exBuildID:    "403",
			exLinkPrefix: "s3://",
			expectedErr:  nil,
		},
		{
			name:         "Test S3 link with trailing /",
			src:          "s3://test-bucket/logs/example-ci-run/403/",
			exBucket:     "test-bucket",
			exJobPrefix:  "logs/example-ci-run/403/",
			exName:       "example-ci-run",
			exBuildID:    "403",
			exLinkPrefix: "s3://",
			expectedErr:  nil,
		},
		{
			name:         "Test S3 link with org name",
			src:          "s3://test-bucket/logs/sig-flexing/example-ci-run/403",
			exBucket:     "test-bucket",
			exJobPrefix:  "logs/sig-flexing/example-ci-run/403/",
			exName:       "example-ci-run",
			exBuildID:    "403",
			exLinkPrefix: "s3://",
			expectedErr:  nil,
		},
		{
			name:        "Test S3 link which cannot be parsed",
			src:         "s3;://test-bucket/logs/sig-flexing/example-ci-run/403",
			expectedErr: ErrCannotParseSource,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jobSource, err := newStorageJobSource(tc.src)
			if err != tc.expectedErr {
				t.Errorf("Expected err: %v, got err: %v", tc.expectedErr, err)
			}
			if tc.exBucket != jobSource.bucket {
				t.Errorf("Expected bucket %s, got %s", tc.exBucket, jobSource.bucket)
			}
			if tc.exName != jobSource.jobName {
				t.Errorf("Expected jobName %s, got %s", tc.exName, jobSource.jobName)
			}
			if tc.exJobPrefix != jobSource.jobPrefix {
				t.Errorf("Expected jobPrefix %s, got %s", tc.exJobPrefix, jobSource.jobPrefix)
			}
			if tc.exLinkPrefix != jobSource.linkPrefix {
				t.Errorf("Expected linkPrefix %s, got %s", tc.exLinkPrefix, jobSource.linkPrefix)
			}
		})
	}
}

// Tests listing objects associated with the current job in GCS
func TestArtifacts_ListGCS(t *testing.T) {
	fakeGCSClient := fakeGCSServer.Client()
	testAf := NewStorageArtifactFetcher(io.NewGCSOpener(fakeGCSClient), false)
	testCases := []struct {
		name              string
		handle            artifactHandle
		source            string
		expectedArtifacts []string
	}{
		{
			name:   "Test ArtifactFetcher simple list artifacts (old format)",
			source: "test-bucket/logs/example-ci-run/403",
			expectedArtifacts: []string{
				"build-log.txt",
				prowv1.StartedStatusFile,
				prowv1.FinishedStatusFile,
				"junit_01.xml",
				"long-log.txt",
			},
		},
		{
			name:              "Test ArtifactFetcher list artifacts on source with no artifacts (old format)",
			source:            "test-bucket/logs/example-ci/404",
			expectedArtifacts: []string{},
		},
		{
			name:   "Test ArtifactFetcher simple list artifacts (new format)",
			source: "gs://test-bucket/logs/example-ci-run/403",
			expectedArtifacts: []string{
				"build-log.txt",
				prowv1.StartedStatusFile,
				prowv1.FinishedStatusFile,
				"junit_01.xml",
				"long-log.txt",
			},
		},
		{
			name:              "Test ArtifactFetcher list artifacts on source with no artifacts (new format)",
			source:            "gs://test-bucket/logs/example-ci/404",
			expectedArtifacts: []string{},
		},
	}

	for _, tc := range testCases {
		actualArtifacts, err := testAf.artifacts(context.Background(), tc.source)
		if err != nil {
			t.Errorf("Failed to get artifact names: %v", err)
		}
		for _, ea := range tc.expectedArtifacts {
			found := false
			for _, aa := range actualArtifacts {
				if ea == aa {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Case %s failed to retrieve the following artifact: %s\nRetrieved: %s.", tc.name, ea, actualArtifacts)
			}

		}
		if len(tc.expectedArtifacts) != len(actualArtifacts) {
			t.Errorf("Case %s produced more artifacts than expected. Expected: %s\nActual: %s.", tc.name, tc.expectedArtifacts, actualArtifacts)
		}
	}
}

// Tests getting handles to objects associated with the current job in GCS
func TestFetchArtifacts_GCS(t *testing.T) {
	fakeGCSClient := fakeGCSServer.Client()
	testAf := NewStorageArtifactFetcher(io.NewGCSOpener(fakeGCSClient), false)
	maxSize := int64(500e6)
	testCases := []struct {
		name         string
		artifactName string
		source       string
		expectedSize int64
		expectErr    bool
	}{
		{
			name:         "Fetch build-log.txt from valid source",
			artifactName: "build-log.txt",
			source:       "test-bucket/logs/example-ci-run/403",
			expectedSize: 25,
		},
		{
			name:         "Fetch build-log.txt from invalid source",
			artifactName: "build-log.txt",
			source:       "test-bucket/logs/example-ci-run/404",
			expectErr:    true,
		},
		{
			name:         "Fetch build-log.txt from valid source",
			artifactName: "build-log.txt",
			source:       "gs://test-bucket/logs/example-ci-run/403",
			expectedSize: 25,
		},
		{
			name:         "Fetch build-log.txt from invalid source",
			artifactName: "build-log.txt",
			source:       "gs://test-bucket/logs/example-ci-run/404",
			expectErr:    true,
		},
	}

	for _, tc := range testCases {
		artifact, err := testAf.Artifact(context.Background(), tc.source, tc.artifactName, maxSize)
		if err != nil {
			t.Errorf("Failed to get artifacts: %v", err)
		}
		size, err := artifact.Size()
		if err != nil && !tc.expectErr {
			t.Fatalf("%s failed getting size for artifact %s, err: %v", tc.name, artifact.JobPath(), err)
		}
		if err == nil && tc.expectErr {
			t.Errorf("%s expected error, got no error", tc.name)
		}

		if size != tc.expectedSize {
			t.Errorf("%s expected artifact with size %d but got %d", tc.name, tc.expectedSize, size)
		}
	}
}

func TestSignURL(t *testing.T) {
	// This fake key is revoked and thus worthless but still make its contents less obvious
	fakeKeyBuf, err := base64.StdEncoding.DecodeString(`
LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tXG5NSUlFdlFJQkFEQU5CZ2txaGtpRzl3MEJBUUVG
QUFTQ0JLY3dnZ1NqQWdFQUFvSUJBUUN4MEF2aW1yMjcwZDdaXG5pamw3b1FRUW1oZTFOb3dpeWMy
UStuQW95aFE1YkQvUW1jb01zcWg2YldneVI0UU90aXVBbHM2VWhJenF4Q25pXG5PazRmbWJqVnhp
STl1Ri9EVTV6ZE5wM0dkQWFiUlVPNW5yWkpMelN0VXhudFBEcjZvK281RHM5YWJJWkNYYUVTXG5o
UWxOdTBrUm5HbHZGUHNkV1JYMmtSN01Yb3pkcXczcHZZRXZyaGlhRStYZnRhUzhKdmZEc0NPT2RQ
OWp5TzNTXG5aR2lkaU5hRmhYK2xnZEcrdHdqOUE3UDFlb1NMbTZCdXVhcjRDOGhlOEVkVGVEbXVk
a1BPeWwvb2tHWU5tSzJkXG5yUkQ0WHBhcy93VGxsTXBLRUZxWllZeVdkRnJvVWQwMFVhQnhHV0cz
UlZ2TWZoRk80QUhrSkNwZlE1U00rSElmXG5VN2lkRjAyYkFnTUJBQUVDZ2dFQURIaVhoTTZ1bFFB
OHZZdzB5T2Q3cGdCd3ZqeHpxckwxc0gvb0l1dzlhK09jXG5QREMxRzV2aU5pZjdRVitEc3haeXlh
T0tISitKVktQcWZodnh3OFNmMHBxQlowdkpwNlR6SVE3R0ZSZXBLUFc4XG5NTVloYWRPZVFiUE00
emN3dWNpS1VuTW45dU1hcllmc2xxUnZDUjBrSEZDWWtucHB2RjYxckNQMGdZZjJJRXZUXG5qNVlV
QWFrNDlVRDQyaUdEZnh2OGUzMGlMTmRRWE1iMHE3V2dyRGdxL0ttUHM2Q2dOaGRzME1uSlRFbUE5
YlFtXG52MHV0K2hUYWpXalcxVWNyUTBnM2JjNng1VWN2V1VjK1ZndUllVmxVcEgvM2dJNXVYZkxn
bTVQNThNa0s4UlhTXG5YYW92Rk05VkNNRFhTK25PWk1uSXoyNVd5QmhkNmdpVWs5UkJhc05Tb1FL
QmdRRGFxUXpyYWJUZEZNY1hwVlNnXG41TUpuNEcvSFVPWUxveVM5cE9UZi9qbFN1ZUYrNkt6RGJV
N1F6TC9wT1JtYjJldVdxdmpmZDVBaU1oUnY2Snk1XG41ZVNpa3dYRDZJeS9sZGh3QUdtMUZrZ1ZX
TXJ3ZHlqYjJpV2I2Um4rNXRBYjgwdzNEN2ZTWWhEWkxUOWJCNjdCXG4ybGxiOGFycEJRcndZUFFB
U2pUVUVYQnVJUUtCZ1FEUUxVemkrd0tHNEhLeko1NE1sQjFoR3cwSFZlWEV4T0pmXG53bS9IVjhl
aThDeHZLMTRoRXpCT3JXQi9aNlo4VFFxWnA0eENnYkNiY0hwY3pLRUxvcDA2K2hqa1N3ZkR2TUJZ
XG5mNnN6U2RSenNYVTI1NndmcG1hRjJ0TlJZZFpVblh2QWc5MFIrb1BFSjhrRHd4cjdiMGZmL3lu
b0UrWUx0ckowXG53dklad3Joc093S0JnQWVPbWlTMHRZeUNnRkwvNHNuZ3ZodEs5WElGQ0w1VU9C
dlp6Qk0xdlJOdjJ5eEFyRi9nXG5zajJqSmVyUWoyTUVpQkRmL2RQelZPYnBwaTByOCthMDNFOEdG
OGZxakpxK2VnbDg2aXBaQjhxOUU5NTFyOUxSXG5Xa1ZtTEFEVVIxTC8rSjFhakxiWHJzOWlzZkxh
ZEI2OUJpT1lXWmpPRk0reitocmNkYkR5blZraEFvR0FJbW42XG50ZU1zN2NNWTh3anZsY0MrZ3Br
SU5GZzgzYVIyajhJQzNIOWtYMGs0N3ovS0ZjbW9TTGxjcEhNc0VJeGozamJXXG5kd0FkZy9TNkpi
RW1SbGdoaWVoaVNRc21RM05ta0xxNlFJWkorcjR4VkZ4RUZnOWFEM0szVUZMT0xickRCSFpJXG5D
M3JRWVpMNkpnY1E1TlBtbTk4QXZIN2RucjRiRGpaVDgzSS9McFVDZ1lFQWttNXlvVUtZY0tXMVQz
R1hadUNIXG40SDNWVGVzZDZyb3pKWUhmTWVkNE9jQ3l1bnBIVmZmSmFCMFIxRjZ2MjFQaitCVWlW
WjBzU010RjEvTE1uQkc4XG5TQVlQUnVxOHVNUUdNQTFpdE1Hc2VhMmg1V2RhbXNGODhXRFd4VEoy
QXVnblJHNERsdmJLUDhPQmVLUFFKeDhEXG5RMzJ2SVpNUVkyV1hVMVhwUkMrNWs5RT1cbi0tLS0t
RU5EIFBSSVZBVEUgS0VZLS0tLS1cbgo=`)
	if err != nil {
		t.Fatalf("Failed to decode fake key: %v", err)
	}
	fakePrivateKey := strings.TrimSpace(string(fakeKeyBuf))
	cases := []struct {
		name      string
		fakeCreds string
		useCookie bool
		expected  string
		contains  []string
		err       string
	}{
		{
			name:     "anon auth works",
			expected: fmt.Sprintf("https://%s/foo/bar/stuff", io.GSAnonHost),
		},
		{
			name:      "cookie auth works",
			useCookie: true,
			expected:  fmt.Sprintf("https://%s/foo/bar/stuff", io.GSCookieHost),
		},
		{
			name:      "invalid json file errors",
			fakeCreds: "yaml: 123",
			err:       "dialing: invalid character 'y' looking for beginning of value",
		},
		{
			name: "bad private key errors",
			fakeCreds: `{
			  "type": "service_account",
			  "private_key": "-----BEGIN PRIVATE KEY-----\nMIIE==\n-----END PRIVATE KEY-----\n",
			  "client_email": "fake-user@k8s.io"
			}`,
			err: "asn1: structure error: tags don't match (16 vs {class:0 tag:13 length:45 isCompound:true}) {optional:false explicit:false application:false private:false defaultValue:<nil> tag:<nil> stringType:0 timeType:0 set:false omitEmpty:false} pkcs1PrivateKey @2",
		},
		{
			name: "bad type errors",
			fakeCreds: `{
			  "type": "user",
			  "private_key": "` + fakePrivateKey + `",
			  "client_email": "fake-user@k8s.io"
			}`,
			err: "dialing: unknown credential type: \"user\"",
		},
		{
			name: "signed URLs work",
			fakeCreds: `{
			  "type": "service_account",
			  "private_key": "` + fakePrivateKey + `",
			  "client_email": "fake-user@k8s.io"
			}`,
			contains: []string{
				"https://storage.googleapis.com/foo/bar/stuff?",
				"GoogleAccessId=fake-user%40k8s.io",
				"Signature=", // Do not particularly care about the Signature contents
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var path string
			if tc.fakeCreds != "" {
				fp, err := ioutil.TempFile("", "fake-creds")
				if err != nil {
					t.Fatalf("Failed to create fake creds: %v", err)
				}

				path = fp.Name()
				defer os.Remove(path)
				if _, err := fp.Write([]byte(tc.fakeCreds)); err != nil {
					t.Fatalf("Failed to write fake creds %s: %v", path, err)
				}

				if err := fp.Close(); err != nil {
					t.Fatalf("Failed to close fake creds %s: %v", path, err)
				}
			}
			// We're testing the combination of NewOpener and signURL here
			// to make sure that the behaviour is more or less the same as before
			// we moved the signURL code to the io package.
			// The errors which were previously tested on the signURL method are now
			// already returned by newOpener.
			// Before, these error should have already lead to errors on gcs client creation, so signURL probably was never able to produce these errors during runtime.
			// (because deck crashed on gcsClient creation)
			var actual string
			opener, err := io.NewOpener(context.Background(), path, "")
			if err == nil {
				af := NewStorageArtifactFetcher(opener, tc.useCookie)
				actual, err = af.signURL(context.Background(), "gs://foo/bar/stuff")
			}
			switch {
			case err != nil:
				if tc.err != err.Error() {
					t.Errorf("expected error: %v, got: %v", tc.err, err)
				}
			case tc.err != "":
				t.Errorf("Failed to receive an expected error, got %q", actual)
			case len(tc.contains) == 0 && actual != tc.expected:
				t.Errorf("signURL(): got %q, want %q", actual, tc.expected)
			default:
				for _, part := range tc.contains {
					if !strings.Contains(actual, part) {
						t.Errorf("signURL(): got %q, does not contain %q", actual, part)
					}
				}
			}
		})
	}
}
