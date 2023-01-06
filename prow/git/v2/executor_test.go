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

package git

import (
	"bytes"
	"errors"
	"github.com/sirupsen/logrus"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/diff"
)

func TestCensoringExecutor_Run(t *testing.T) {
	var testCases = []struct {
		name        string
		dir, git    string
		args        []string
		censor      Censor
		executeOut  []byte
		executeErr  error
		expectedOut []byte
		expectedErr bool
	}{
		{
			name: "happy path with nothing to censor returns all output",
			dir:  "/somewhere/repo",
			git:  "/usr/bin/git",
			args: []string{"status"},
			censor: func(content []byte) []byte {
				return content
			},
			executeOut:  []byte("hi"),
			executeErr:  nil,
			expectedOut: []byte("hi"),
			expectedErr: false,
		},
		{
			name: "happy path with nonstandard git binary",
			dir:  "/somewhere/repo",
			git:  "/usr/local/bin/git",
			args: []string{"status"},
			censor: func(content []byte) []byte {
				return content
			},
			executeOut:  []byte("hi"),
			executeErr:  nil,
			expectedOut: []byte("hi"),
			expectedErr: false,
		},
		{
			name: "happy path with something to censor returns altered output",
			dir:  "/somewhere/repo",
			git:  "/usr/bin/git",
			args: []string{"status"},
			censor: func(content []byte) []byte {
				return bytes.ReplaceAll(content, []byte("secret"), []byte("CENSORED"))
			},
			executeOut:  []byte("hi secret"),
			executeErr:  nil,
			expectedOut: []byte("hi CENSORED"),
			expectedErr: false,
		},
		{
			name: "error is propagated",
			dir:  "/somewhere/repo",
			git:  "/usr/bin/git",
			args: []string{"status"},
			censor: func(content []byte) []byte {
				return bytes.ReplaceAll(content, []byte("secret"), []byte("CENSORED"))
			},
			executeOut:  []byte("hi secret"),
			executeErr:  errors.New("oops"),
			expectedOut: []byte("hi CENSORED"),
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			e := censoringExecutor{
				logger: logrus.WithField("name", testCase.name),
				dir:    testCase.dir,
				git:    testCase.git,
				censor: testCase.censor,
				execute: func(dir, command string, args ...string) (bytes []byte, e error) {
					if dir != testCase.dir {
						t.Errorf("%s: got incorrect dir: %v", testCase.name, diff.StringDiff(dir, testCase.dir))
					}
					if command != testCase.git {
						t.Errorf("%s: got incorrect command: %v", testCase.name, diff.StringDiff(command, testCase.git))
					}
					if !reflect.DeepEqual(args, testCase.args) {
						t.Errorf("%s: got incorrect args: %v", testCase.name, diff.ObjectReflectDiff(args, testCase.args))
					}
					return testCase.executeOut, testCase.executeErr
				},
			}
			actual, actualErr := e.Run(testCase.args...)
			if testCase.expectedErr && actualErr == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && actualErr != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
			}
			if !reflect.DeepEqual(actual, testCase.expectedOut) {
				t.Errorf("%s: got incorrect command output: %v", testCase.name, diff.StringDiff(string(actual), string(testCase.expectedOut)))
			}
		})
	}
}
