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

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/spf13/pflag"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // Enable all auth provider plugins
)

func TestValidateFlags(t *testing.T) {
	tmpFileInfo, err := ioutil.TempFile(".", "mergecred-tmp-file")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile := tmpFileInfo.Name()
	t.Cleanup(func() {
		os.Remove(tmpFile)
	})
	tests := []struct {
		name        string
		args        []string
		errExpected bool
	}{
		{
			name: "valid",
			args: []string{
				"--project=project",
				"--context=test-context",
				"--name=test-name",
				fmt.Sprintf("--kubeconfig-to-merge=%s", tmpFile),
			},
			errExpected: false,
		}, {
			name: "project is required",
			args: []string{
				"--context=test-context",
				"--name=test-name",
				fmt.Sprintf("--kubeconfig-to-merge=%s", tmpFile),
			},
			errExpected: true,
		}, {
			name: "context is required",
			args: []string{
				"--project=project",
				"--name=test-name",
				fmt.Sprintf("--kubeconfig-to-merge=%s", tmpFile),
			},
			errExpected: true,
		}, {
			name: "kubeconfig-to-merge must be a valid path",
			args: []string{
				"--project=project",
				"--context=test-context",
				"--name=test-name",
				"--kubeconfig-to-merge=/dev/null",
			},
			errExpected: true,
		}, {
			name: "kubeconfig-to-merge must be a file",
			args: []string{
				"--project=project",
				"--context=test-context",
				"--name=test-name",
				"--kubeconfig-to-merge=tmp",
			},
			errExpected: true,
		}, {
			name: "auto and dst-key exclusive",
			args: []string{
				"--project=project",
				"--context=test-context",
				"--name=test-name",
				fmt.Sprintf("--kubeconfig-to-merge=%s", tmpFile),
				"--dst-key=dst-key",
			},
			errExpected: true,
		}, {
			name: "src-key is required when auto is off",
			args: []string{
				"--project=project",
				"--context=test-context",
				"--name=test-name",
				fmt.Sprintf("--kubeconfig-to-merge=%s", tmpFile),
				"--auto=false",
				"--dst-key=dst-key",
			},
			errExpected: true,
		}, {
			name: "dst-key is required when auto is off",
			args: []string{
				"--project=project",
				"--context=test-context",
				"--name=test-name",
				fmt.Sprintf("--kubeconfig-to-merge=%s", tmpFile),
				"--auto=false",
				"--src-key=src-key",
			},
			errExpected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var o options

			os.Args = []string{"mergecred"}
			pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
			os.Args = append(os.Args, test.args...)
			o.parseFlags()

			if hasErr := o.validateFlags() != nil; hasErr != test.errExpected {
				t.Errorf("expected err: %t but was %t", test.errExpected, hasErr)
			}
		})
	}
}

func TestMergeConfigs(t *testing.T) {
}

func TestGetKeys(t *testing.T) {

}

func TestBackupSecret(t *testing.T) {

}

// process merges secret into a new secret for write.
func TestProcess(t *testing.T) {

}
