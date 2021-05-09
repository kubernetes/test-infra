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

package metadata

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

func TestValidateBase(t *testing.T) {
	var testCases = []struct {
		name        string
		flags       *Flags
		expected    *Flags
		envOwner    string
		expectedErr bool
	}{
		{
			name:     "Flag Set",
			flags:    &Flags{project: "foo"},
			expected: &Flags{project: "foo"},
		},
		{
			name:     "Flag From ENV",
			flags:    &Flags{project: ""},
			envOwner: "foo",
			expected: &Flags{project: "foo"},
		},
		{
			name:        "Flag ENV unset",
			envOwner:    "",
			flags:       &Flags{},
			expected:    &Flags{},
			expectedErr: true,
		},
	}

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print("Test")
		}}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			fakeEnv := func(a string) string {
				return testCase.envOwner
			}
			err := ValidateBase(testCase.flags, cmd, fakeEnv)
			if err == nil && testCase.expectedErr {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if err != nil && !testCase.expectedErr {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}

			if !reflect.DeepEqual(testCase.flags, testCase.expected) {
				t.Errorf("%s: expected match to %v but got %v", testCase.name, testCase.expected, testCase.flags)
			}
		})
	}
}

func TestValidateInc(t *testing.T) {
	var testCases = []struct {
		name        string
		flags       *Flags
		expected    *Flags
		expectedErr bool
	}{
		{
			name:     "Flag Set",
			flags:    &Flags{changeNum: "foo", patchSet: "bar"},
			expected: &Flags{changeNum: "foo", patchSet: "bar"},
		},
		{
			name:        "One flag unset",
			flags:       &Flags{changeNum: "foo"},
			expected:    &Flags{changeNum: "foo"},
			expectedErr: true,
		},
		{
			name:        "All flag unset",
			flags:       &Flags{},
			expected:    &Flags{},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateInc(testCase.flags)
			if err == nil && testCase.expectedErr {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if err != nil && !testCase.expectedErr {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}

			if !reflect.DeepEqual(testCase.flags, testCase.expected) {
				t.Errorf("%s: expected match to %v but got %v", testCase.name, testCase.expected, testCase.flags)
			}
		})
	}
}

func TestValidateAbs(t *testing.T) {
	var testCases = []struct {
		name        string
		flags       *Flags
		expected    *Flags
		gitFunc     gitRunner
		expectedErr bool
	}{
		{
			name:     "Flag Set",
			flags:    &Flags{commitID: "1234", ref: "bar"},
			expected: &Flags{commitID: "1234", ref: "bar"},
		},
		{
			name:     "No CommitID",
			flags:    &Flags{ref: "bar"},
			expected: &Flags{commitID: "1234", ref: "bar"},
			gitFunc:  func(a ...string) (string, error) { return "1234", nil },
		},
		{
			name:        "Error CommitID",
			flags:       &Flags{ref: "bar"},
			expected:    &Flags{ref: "bar"},
			gitFunc:     func(a ...string) (string, error) { return "", errors.New("bad") },
			expectedErr: true,
		},
		{
			name:     "No Ref",
			flags:    &Flags{commitID: "1234"},
			expected: &Flags{commitID: "1234", ref: "bar"},
			gitFunc:  func(a ...string) (string, error) { return "bar", nil },
		},
		{
			name:     "Error Ref",
			flags:    &Flags{commitID: "1234"},
			expected: &Flags{commitID: "1234", ref: "HEAD"},
			gitFunc:  func(a ...string) (string, error) { return "", errors.New("bad") },
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateAbs(testCase.flags, testCase.gitFunc)
			if err == nil && testCase.expectedErr {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if err != nil && !testCase.expectedErr {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}

			if !reflect.DeepEqual(testCase.flags, testCase.expected) {
				t.Errorf("%s: expected match to %v but got %v", testCase.name, testCase.expected, testCase.flags)
			}
		})
	}
}
