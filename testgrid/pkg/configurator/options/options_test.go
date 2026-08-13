/*
Copyright 2021 The Kubernetes Authors.

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

package options

import (
	"flag"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/prow/pkg/flagutil"
	configflagutil "sigs.k8s.io/prow/pkg/flagutil/config"
)

func Test_Options(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected *Options
	}{
		{
			name: "No options: fails",
			args: []string{},
		},
		{
			name: "Print Text",
			args: []string{"--yaml=file.yaml", "--print-text", "--oneshot"},
			expected: &Options{
				Inputs:      []string{"file.yaml"},
				DefaultYAML: "file.yaml",
				ProwConfig: configflagutil.ConfigOptions{
					ConfigPathFlagName:                    "prow-config",
					JobConfigPathFlagName:                 "prow-job-config",
					SupplementalProwConfigsFileNameSuffix: "_prowconfig.yaml",
					InRepoConfigCacheSize:                 200,
				},
				PrintText: true,
				Oneshot:   true,
			},
		},
		{
			name: "Output to Location",
			args: []string{"--yaml=file.yaml", "--output=gs://foo/bar"},
			expected: &Options{
				Inputs:      []string{"file.yaml"},
				DefaultYAML: "file.yaml",
				ProwConfig: configflagutil.ConfigOptions{
					ConfigPathFlagName:                    "prow-config",
					JobConfigPathFlagName:                 "prow-job-config",
					SupplementalProwConfigsFileNameSuffix: "_prowconfig.yaml",
					InRepoConfigCacheSize:                 200,
				},
				Output: flagutil.NewStringsBeenSet("gs://foo/bar"),
			},
		},
		{
			name: "Output to multiple Locations",
			args: []string{"--yaml=file.yaml", "--output=gs://foo/bar", "--output=./foo/bar"},
			expected: &Options{
				Inputs:      []string{"file.yaml"},
				DefaultYAML: "file.yaml",
				ProwConfig: configflagutil.ConfigOptions{
					ConfigPathFlagName:                    "prow-config",
					JobConfigPathFlagName:                 "prow-job-config",
					SupplementalProwConfigsFileNameSuffix: "_prowconfig.yaml",
					InRepoConfigCacheSize:                 200,
				},
				Output: flagutil.NewStringsBeenSet("gs://foo/bar", "./foo/bar"),
			},
		},
		{
			name: "Many files: first set as default",
			args: []string{"--yaml=first,second,third", "--validate-config-file"},
			expected: &Options{
				Inputs:      []string{"first", "second", "third"},
				DefaultYAML: "first",
				ProwConfig: configflagutil.ConfigOptions{
					ConfigPathFlagName:                    "prow-config",
					JobConfigPathFlagName:                 "prow-job-config",
					SupplementalProwConfigsFileNameSuffix: "_prowconfig.yaml",
					InRepoConfigCacheSize:                 200,
				},
				ValidateConfigFile: true,
			},
		},
		{
			name: "--validate-config-file with output: fails",
			args: []string{"--yaml=file.yaml", "--validate-config-file", "--output=/foo/bar"},
		},
		{
			name: "Prow jobs with no root config: fails",
			args: []string{"--yaml=file.yaml", "--output=/foo/bar", "--prow-job-config=/prow/jobs"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			flags := flag.NewFlagSet(test.name, flag.ContinueOnError)
			var actual Options
			err := actual.GatherOptions(flags, test.args)
			switch {
			case err == nil && test.expected == nil:
				t.Errorf("Failed to return an error")
			case err != nil && test.expected != nil:
				t.Errorf("Unexpected error: %v", err)
			case test.expected != nil && !reflect.DeepEqual(*test.expected, actual):
				t.Errorf("Mismatched Options: diff: %s", cmp.Diff(*test.expected, actual, cmp.Exporter(func(_ reflect.Type) bool { return true })))
			}
		})
	}
}
