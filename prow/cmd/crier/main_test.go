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
	"flag"
	"reflect"
	"testing"

	"k8s.io/test-infra/prow/flagutil"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
)

func TestOptions(t *testing.T) {

	var defaultGitHubOptions flagutil.GitHubOptions
	defaultGitHubOptions.AddFlags(flag.NewFlagSet("", flag.ContinueOnError))

	defaultGerritProjects := make(map[string][]string)

	defaultInstrumentationOptions := flagutil.InstrumentationOptions{
		MetricsPort: prowflagutil.DefaultMetricsPort,
		PProfPort:   prowflagutil.DefaultPProfPort,
	}
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
			name: "gerrit only support one worker",
			args: []string{"--gerrit-workers=99", "--gerrit-projects=foo=bar", "--cookiefile=foobar", "--config-path=foo"},
			expected: &options{
				gerritWorkers:  1,
				cookiefilePath: "foobar",
				gerritProjects: map[string][]string{
					"foo": {"bar"},
				},
				configPath:             "foo",
				github:                 defaultGitHubOptions,
				k8sReportFraction:      1.0,
				instrumentationOptions: defaultInstrumentationOptions,
			},
		},
		{
			name: "gerrit missing --gerrit-projects, reject",
			args: []string{"--gerrit-workers=5", "--cookiefile=foobar", "--config-path=foo"},
		},
		{
			name: "gerrit missing --cookiefile",
			args: []string{"--gerrit-workers=5", "--gerrit-projects=foo=bar", "--config-path=foo"},
			expected: &options{
				gerritWorkers: 1,
				gerritProjects: map[string][]string{
					"foo": {"bar"},
				},
				configPath:             "foo",
				github:                 defaultGitHubOptions,
				k8sReportFraction:      1.0,
				instrumentationOptions: defaultInstrumentationOptions,
			},
		},
		//PubSub Reporter
		{
			name: "pubsub workers, sets workers",
			args: []string{"--pubsub-workers=7", "--config-path=baz"},
			expected: &options{
				pubsubWorkers:          7,
				configPath:             "baz",
				github:                 defaultGitHubOptions,
				gerritProjects:         defaultGerritProjects,
				k8sReportFraction:      1.0,
				instrumentationOptions: defaultInstrumentationOptions,
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
				slackWorkers:           13,
				slackTokenFile:         "/bar/baz",
				configPath:             "foo",
				github:                 defaultGitHubOptions,
				gerritProjects:         defaultGerritProjects,
				k8sReportFraction:      1.0,
				instrumentationOptions: defaultInstrumentationOptions,
			},
		},
		{
			name: "slack missing --slack-token, rejects",
			args: []string{"--slack-workers=1", "--config-path=foo"},
		},
		{
			name: "slack with --dry-run, sets",
			args: []string{"--slack-workers=13", "--slack-token-file=/bar/baz", "--config-path=foo", "--dry-run", "--deck-url=http://www.example.com"},
			expected: &options{
				slackWorkers:   13,
				slackTokenFile: "/bar/baz",
				configPath:     "foo",
				dryrun:         true,
				client: prowflagutil.KubernetesOptions{
					DeckURI: "http://www.example.com",
				},
				github:                 defaultGitHubOptions,
				gerritProjects:         defaultGerritProjects,
				k8sReportFraction:      1.0,
				instrumentationOptions: defaultInstrumentationOptions,
			},
		},
		{
			name: "Dry run with no --deck-url, rejects",
			args: []string{"--slack-workers=13", "--slack-token-file=/bar/baz", "--config-path=foo", "--dry-run"},
		},
		{
			name: "k8s-gcs enables k8s-gcs",
			args: []string{"--kubernetes-blob-storage-workers=3", "--config-path=foo"},
			expected: &options{
				k8sBlobStorageWorkers:  3,
				configPath:             "foo",
				github:                 defaultGitHubOptions,
				gerritProjects:         defaultGerritProjects,
				k8sReportFraction:      1.0,
				instrumentationOptions: defaultInstrumentationOptions,
			},
		},
		{
			name: "k8s-gcs with report fraction sets report fraction",
			args: []string{"--kubernetes-blob-storage-workers=3", "--config-path=foo", "--kubernetes-report-fraction=0.5"},
			expected: &options{
				k8sBlobStorageWorkers:  3,
				configPath:             "foo",
				github:                 defaultGitHubOptions,
				gerritProjects:         defaultGerritProjects,
				k8sReportFraction:      0.5,
				instrumentationOptions: defaultInstrumentationOptions,
			},
		},
		{
			name: "k8s-gcs with too large report fraction rejects",
			args: []string{"--kubernetes-gcs-workers=3", "--config-path=foo", "--kubernetes-report-fraction=1.5"},
		},
		{
			name: "k8s-gcs with negative report fraction rejects",
			args: []string{"--kubernetes-gcs-workers=3", "--config-path=foo", "--kubernetes-report-fraction=-1.2"},
		},
	}

	for _, tc := range cases {
		flags := flag.NewFlagSet(tc.name, flag.ContinueOnError)
		var actual options
		err := actual.parseArgs(flags, tc.args)
		switch {
		case err == nil && tc.expected == nil:
			t.Errorf("%s: failed to return an error", tc.name)
		case err != nil && tc.expected != nil:
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		case tc.expected != nil && !reflect.DeepEqual(*tc.expected, actual):
			t.Errorf("%s: actual %v != expected %v", tc.name, actual, *tc.expected)
		}
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
		{
			name:              "github missing --github-token-path, uses default",
			args:              []string{"--github-workers=5", "--config-path=foo"},
			expectedWorkers:   5,
			expectedTokenPath: "/etc/github/oauth",
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
