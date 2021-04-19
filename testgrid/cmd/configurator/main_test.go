/*
Copyright 2016 The Kubernetes Authors.

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
	"context"
	"flag"
	"io/ioutil"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	configflagutil "k8s.io/test-infra/prow/flagutil/config"
)

func Test_Options(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected *options
	}{
		{
			name: "No options: fails",
			args: []string{},
		},
		{
			name: "Print Text",
			args: []string{"--yaml=file.yaml", "--print-text", "--oneshot"},
			expected: &options{
				inputs:      []string{"file.yaml"},
				defaultYAML: "file.yaml",
				prowConfig: configflagutil.ConfigOptions{
					ConfigPathFlagName:              "prow-config",
					JobConfigPathFlagName:           "prow-job-config",
					SupplementalProwConfigsFileName: "_prowconfig.yaml",
				},
				printText: true,
				oneshot:   true,
			},
		},
		{
			name: "Output to Location",
			args: []string{"--yaml=file.yaml", "--output=gs://foo/bar"},
			expected: &options{
				inputs:      []string{"file.yaml"},
				defaultYAML: "file.yaml",
				prowConfig: configflagutil.ConfigOptions{
					ConfigPathFlagName:              "prow-config",
					JobConfigPathFlagName:           "prow-job-config",
					SupplementalProwConfigsFileName: "_prowconfig.yaml",
				},
				output: "gs://foo/bar",
			},
		},
		{
			name: "Many files: first set as default",
			args: []string{"--yaml=first,second,third", "--validate-config-file"},
			expected: &options{
				inputs:      []string{"first", "second", "third"},
				defaultYAML: "first",
				prowConfig: configflagutil.ConfigOptions{
					ConfigPathFlagName:              "prow-config",
					JobConfigPathFlagName:           "prow-job-config",
					SupplementalProwConfigsFileName: "_prowconfig.yaml",
				},
				validateConfigFile: true,
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
			var actual options
			err := actual.gatherOptions(flags, test.args)
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

func Test_announceChanges(t *testing.T) {
	tests := []struct {
		name    string
		touch   bool
		delete  bool
		addFile bool
	}{
		{
			name:  "Announce on edit file",
			touch: true,
		},
		{
			name:   "Announce on delete file",
			delete: true,
		},
		{
			name:    "Announce on added file to subdirectory",
			addFile: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory, err := ioutil.TempDir("", "")
			if err != nil {
				t.Fatalf("Error in creating temporary dir: %v", err)
			}
			defer os.RemoveAll(directory)

			file, err := ioutil.TempFile(directory, "1*.yaml")
			if err != nil {
				t.Fatalf("Error in creating temporary file: %v", err)
			}

			ctx, cancelFunc := context.WithCancel(context.Background())
			resultChannel := make(chan []string)
			go announceChanges(ctx, []string{directory}, resultChannel)

			initResult := <-resultChannel
			if len(initResult) != 1 && initResult[0] != file.Name() {
				t.Errorf("Unexpected initialization announcement; got %s, expected %s", initResult, []string{file.Name()})
			}

			switch {
			case test.touch:
				if err := os.Chtimes(file.Name(), time.Now().Local(), time.Now().Local()); err != nil {
					t.Fatalf("OS error with touching file")
				}
			case test.delete:
				if err := os.Remove(file.Name()); err != nil {
					t.Fatalf("OS error with deleting file")
				}
			case test.addFile:
				if _, err := ioutil.TempFile(directory, "2*.yaml"); err != nil {
					t.Fatalf("OS error with adding new file")
				}
			}

			result := <-resultChannel
			cancelFunc()

			if len(result) != 1 {
				t.Errorf("Unexpected result: got %v, but expected only one result", result)
			}
		})
	}
}
