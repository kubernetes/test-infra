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

package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"flag"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"k8s.io/test-infra/prow/flagutil"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	"k8s.io/test-infra/prow/webhook"
)

func TestOptions(t *testing.T) {

	var defaultGitHubOptions flagutil.GitHubOptions
	defaultGitHubOptions.AddFlags(flag.NewFlagSet("", flag.ContinueOnError))

	cases := []struct {
		name     string
		args     []string
		expected *options
	}{
		//General
		{
			name: "no args, reject",
			args: []string{},
		},
		{
			name: "config-path is empty string, reject",
			args: []string{"--pubsub-workers=1", "--config-path="},
		},
		//Gerrit Reporter
		{
			name: "gerrit supports multiple workers",
			args: []string{"--gerrit-workers=99", "--cookiefile=foobar", "--config-path=foo"},
			expected: &options{
				gerritWorkers:  99,
				cookiefilePath: "foobar",
				config: configflagutil.ConfigOptions{
					ConfigPathFlagName:                    "config-path",
					JobConfigPathFlagName:                 "job-config-path",
					ConfigPath:                            "foo",
					SupplementalProwConfigsFileNameSuffix: "_prowconfig.yaml",
					InRepoConfigCacheSize:                 100,
					InRepoConfigCacheCopies:               1,
				},
				github:                 defaultGitHubOptions,
				k8sReportFraction:      1.0,
				instrumentationOptions: prowflagutil.DefaultInstrumentationOptions(),
			},
		},
		{
			name: "gerrit missing --cookiefile",
			args: []string{"--gerrit-workers=5", "--config-path=foo"},
			expected: &options{
				gerritWorkers: 5,
				config: configflagutil.ConfigOptions{
					ConfigPathFlagName:                    "config-path",
					JobConfigPathFlagName:                 "job-config-path",
					ConfigPath:                            "foo",
					SupplementalProwConfigsFileNameSuffix: "_prowconfig.yaml",
					InRepoConfigCacheSize:                 100,
					InRepoConfigCacheCopies:               1,
				},
				github:                 defaultGitHubOptions,
				k8sReportFraction:      1.0,
				instrumentationOptions: prowflagutil.DefaultInstrumentationOptions(),
			},
		},
		//PubSub Reporter
		{
			name: "pubsub workers, sets workers",
			args: []string{"--pubsub-workers=7", "--config-path=baz"},
			expected: &options{
				config: configflagutil.ConfigOptions{
					ConfigPathFlagName:                    "config-path",
					JobConfigPathFlagName:                 "job-config-path",
					ConfigPath:                            "baz",
					SupplementalProwConfigsFileNameSuffix: "_prowconfig.yaml",
					InRepoConfigCacheSize:                 100,
					InRepoConfigCacheCopies:               1,
				},
				pubsubWorkers:          7,
				github:                 defaultGitHubOptions,
				k8sReportFraction:      1.0,
				instrumentationOptions: prowflagutil.DefaultInstrumentationOptions(),
			},
		},
		{
			name: "pubsub workers set to negative, rejects",
			args: []string{"--pubsub-workers=-3", "--config-path=foo"},
		},
		//Slack Reporter
		{
			name: "slack workers, sets workers",
			args: []string{"--slack-workers=13", "--slack-token-file=/bar/baz", "--config-path=foo"},
			expected: &options{
				slackWorkers:   13,
				slackTokenFile: "/bar/baz",
				config: configflagutil.ConfigOptions{
					ConfigPathFlagName:                    "config-path",
					JobConfigPathFlagName:                 "job-config-path",
					ConfigPath:                            "foo",
					SupplementalProwConfigsFileNameSuffix: "_prowconfig.yaml",
					InRepoConfigCacheSize:                 100,
					InRepoConfigCacheCopies:               1,
				},
				github:                 defaultGitHubOptions,
				k8sReportFraction:      1.0,
				instrumentationOptions: prowflagutil.DefaultInstrumentationOptions(),
			},
		},
		{
			name: "slack missing --slack-token, rejects",
			args: []string{"--slack-workers=1", "--config-path=foo"},
		},
		{
			name: "slack with --dry-run, sets",
			args: []string{"--slack-workers=13", "--slack-token-file=/bar/baz", "--config-path=foo", "--dry-run"},
			expected: &options{
				slackWorkers:   13,
				slackTokenFile: "/bar/baz",
				config: configflagutil.ConfigOptions{
					ConfigPathFlagName:                    "config-path",
					JobConfigPathFlagName:                 "job-config-path",
					ConfigPath:                            "foo",
					SupplementalProwConfigsFileNameSuffix: "_prowconfig.yaml",
					InRepoConfigCacheSize:                 100,
					InRepoConfigCacheCopies:               1,
				},
				dryrun:                 true,
				github:                 defaultGitHubOptions,
				k8sReportFraction:      1.0,
				instrumentationOptions: prowflagutil.DefaultInstrumentationOptions(),
			},
		},
		{
			name: "k8s-gcs enables k8s-gcs",
			args: []string{"--kubernetes-blob-storage-workers=3", "--config-path=foo"},
			expected: &options{
				k8sBlobStorageWorkers: 3,
				config: configflagutil.ConfigOptions{
					ConfigPathFlagName:                    "config-path",
					JobConfigPathFlagName:                 "job-config-path",
					ConfigPath:                            "foo",
					SupplementalProwConfigsFileNameSuffix: "_prowconfig.yaml",
					InRepoConfigCacheSize:                 100,
					InRepoConfigCacheCopies:               1,
				},
				github:                 defaultGitHubOptions,
				k8sReportFraction:      1.0,
				instrumentationOptions: prowflagutil.DefaultInstrumentationOptions(),
			},
		},
		{
			name: "k8s-gcs with report fraction sets report fraction",
			args: []string{"--kubernetes-blob-storage-workers=3", "--config-path=foo", "--kubernetes-report-fraction=0.5"},
			expected: &options{
				k8sBlobStorageWorkers: 3,
				config: configflagutil.ConfigOptions{
					ConfigPathFlagName:                    "config-path",
					JobConfigPathFlagName:                 "job-config-path",
					ConfigPath:                            "foo",
					SupplementalProwConfigsFileNameSuffix: "_prowconfig.yaml",
					InRepoConfigCacheSize:                 100,
					InRepoConfigCacheCopies:               1,
				},
				github:                 defaultGitHubOptions,
				k8sReportFraction:      0.5,
				instrumentationOptions: prowflagutil.DefaultInstrumentationOptions(),
			},
		},
		{
			name: "k8s-gcs with too large report fraction rejects",
			args: []string{"--kubernetes-blob-storage-workers=3", "--config-path=foo", "--kubernetes-report-fraction=1.5"},
		},
		{
			name: "k8s-gcs with negative report fraction rejects",
			args: []string{"--kubernetes-blob-storage-workers=3", "--config-path=foo", "--kubernetes-report-fraction=-1.2"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flags := flag.NewFlagSet(tc.name, flag.ContinueOnError)
			var actual options
			err := actual.parseArgs(flags, tc.args)
			switch {
			case err == nil && tc.expected == nil:
				t.Fatalf("%s: failed to return an error", tc.name)
			case err != nil && tc.expected != nil:
				t.Fatalf("%s: unexpected error: %v", tc.name, err)
			}

			if tc.expected == nil {
				return
			}
			if diff := cmp.Diff(actual, *tc.expected, cmp.Exporter(func(_ reflect.Type) bool { return true })); diff != "" {
				t.Errorf("Result differs from expected: %s", diff)
			}

		})
	}
}

/*
The GitHubOptions object has several private fields and objects
This unit testing covers only the public portions
*/
func TestGitHubOptions(t *testing.T) {
	cases := []struct {
		name              string
		args              []string
		expectedWorkers   int
		expectedTokenPath string
	}{
		{
			name:              "github workers, only support single worker",
			args:              []string{"--github-workers=5", "--github-token-path=tkpath", "--config-path=foo"},
			expectedWorkers:   5,
			expectedTokenPath: "tkpath",
		},
	}

	for _, tc := range cases {
		flags := flag.NewFlagSet(tc.name, flag.ContinueOnError)
		actual := options{}
		err := actual.parseArgs(flags, tc.args)

		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		}
		if actual.githubWorkers != tc.expectedWorkers {
			t.Errorf("%s: worker mismatch: actual %d != expected %d",
				tc.name, actual.githubWorkers, tc.expectedWorkers)
		}
		if actual.github.TokenPath != tc.expectedTokenPath {
			t.Errorf("%s: path mismatch: actual %s != expected %s",
				tc.name, actual.github.TokenPath, tc.expectedTokenPath)
		}
	}
}

// TestWebhook tests the integration of the webhook logic with Crier's
// ed25519-based signature token. The signture-verification handler to
// `httptest.NewServer` can be used in webhook target servers.
func TestWebhook(t *testing.T) {
	for _, tc := range [...]struct {
		name                      string
		privKey                   string
		pubKey                    []byte
		message                   any
		expectedSendError         string
		expectedVerificationError bool
	}{
		{
			name:    "happy path",
			privKey: "rfA2rImIhzsKSRGqAOzyCu722K8CJb0uRR94dac2BY7I1unzumhVcshbfon6QD6mGzIMW26pMHek+i5A5crbbw==",
			pubKey:  []byte{200, 214, 233, 243, 186, 104, 85, 114, 200, 91, 126, 137, 250, 64, 62, 166, 27, 50, 12, 91, 110, 169, 48, 119, 164, 250, 46, 64, 229, 202, 219, 111},
			message: "This is a message",
		},
		{
			name:              "incorrect key length",
			privKey:           "rfA2rImIhzsKSRGqAOzyCu722K8CJb0uRR94dac2BY7I1vO6aFVyyFt+ifpAPqYbMgxbbqkwd6T6LkDlyttv",
			expectedSendError: "webhook signing key length mismatch",
		},
		{
			name:    "complex message",
			privKey: "rfA2rImIhzsKSRGqAOzyCu722K8CJb0uRR94dac2BY7I1unzumhVcshbfon6QD6mGzIMW26pMHek+i5A5crbbw==",
			pubKey:  []byte{200, 214, 233, 243, 186, 104, 85, 114, 200, 91, 126, 137, 250, 64, 62, 166, 27, 50, 12, 91, 110, 169, 48, 119, 164, 250, 46, 64, 229, 202, 219, 111},
			message: struct {
				FieldOne   float64
				FieldTwo   string
				FieldThree struct{ FieldFour []byte }
			}{
				FieldOne:   12345.45672,
				FieldTwo:   "🕬" + `";>`,
				FieldThree: struct{ FieldFour []byte }{FieldFour: []byte{7, 77, 0, 0, 0, 0, 0, 0, 0, 123, 123, 123}},
			},
		},
		{
			name:                      "key mismatch",
			privKey:                   "rfA2rImIhzsKSRGqAOzyCu722K8CJb0uRR94dac2BY7I1unzumhVcshbfon6QD6mGzIMW26pMHek+i5A5crbbw==",
			pubKey:                    []byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			message:                   "This is a message",
			expectedVerificationError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var (
				tokenFunc = webhookSigningFunc(
					func() []byte { return []byte(tc.privKey) },
				)
				client       = webhook.NewClient(tokenFunc)
				callReceived = false

				serverURL string
			)

			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
				callReceived = true
				timestamp := req.Header.Get("x-prow-timestamp")

				// Verify the timestamp
				{
					ts, err := time.Parse(time.RFC3339, timestamp)
					if err != nil {
						t.Fatalf("the x-prow-timestamp header is not parseable as time.RFC3339")
					}

					tolerance := 5 * time.Second
					if skew := time.Since(ts); skew > tolerance || -skew > tolerance {
						t.Errorf("unacceptable time skew with x-prow-timestamp: %v", skew)
					}
				}

				// Compute the digest
				var digest []byte
				{
					var buf bytes.Buffer
					buf.WriteString(serverURL)
					buf.WriteByte(0)
					buf.WriteString(timestamp)
					buf.WriteByte(0)
					_, err := buf.ReadFrom(req.Body)
					if err != nil {
						t.Fatalf("unexpected error reading from the request body: %v", err)
					}
					digest = buf.Bytes()
				}

				// Verify the signature
				{
					signature, err := base64.StdEncoding.DecodeString(req.Header.Get("x-prow-token"))
					if err != nil {
						t.Fatalf("unexpected error decoding x-prow-token from base64: %v", err)
					}

					if ok := ed25519.Verify(tc.pubKey, digest, signature); ok == tc.expectedVerificationError {
						if ok {
							t.Errorf("signature verification unexpectedly succeeded")
						} else {
							t.Errorf("signature verification failed")
						}
					}
				}
			}))
			defer server.Close()
			serverURL = server.URL

			if err := client.Send(context.Background(), serverURL, tc.message); err != nil {
				if tc.expectedSendError == "" {
					t.Errorf("unexpected error while sending the webhook: %v", err)
				} else {
					if !strings.Contains(err.Error(), tc.expectedSendError) {
						t.Errorf("expected error to contain %q, found instead: %v", tc.expectedSendError, err)
					}
				}
			} else {
				if tc.expectedSendError != "" {
					t.Errorf("expected error containing %q, fuond nil", tc.expectedSendError)
				}
			}

			if !callReceived && tc.expectedSendError == "" {
				t.Errorf("expected call to the target server, not received")
			}
		})
	}
}
