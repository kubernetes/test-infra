/*
Copyright 2019 The Kubernetes Authors.

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

package gencred

import (
	"os"
	"testing"

	"github.com/spf13/pflag"
)

func TestValidateFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		errExpected bool
	}{
		{
			name:        "valid",
			args:        []string{"--context=test-context", "--name=test-name"},
			errExpected: false,
		},
		{
			name:        "name is required",
			args:        []string{"--context=test-context", "--name="},
			errExpected: true,
		},
		{
			name:        "context is required",
			args:        []string{"--context=", "--name=test-name"},
			errExpected: true,
		},
		{
			name:        "certificate and serviceaccount are mutually exclusive",
			args:        []string{"--certificate", "--serviceaccount"},
			errExpected: true,
		},
		{
			name:        "output must be a valid path",
			args:        []string{"--output=/dev/null"},
			errExpected: true,
		},
		{
			name:        "output must be a file",
			args:        []string{"--output=/tmp"},
			errExpected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var o options

			os.Args = []string{"gencred"}
			pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
			os.Args = append(os.Args, test.args...)
			o.parseFlags()

			if hasErr := o.validateFlags() != nil; hasErr != test.errExpected {
				t.Errorf("expected err: %t but was %t", test.errExpected, hasErr)
			}
		})
	}
}
