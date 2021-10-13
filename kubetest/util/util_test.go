/*
Copyright 2017 The Kubernetes Authors.

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

package util

import (
	"os"
	"strings"
	"testing"
)

func TestPushEnv(t *testing.T) {
	env := "fake-env"
	empty := ""
	filled := "initial"
	cases := []struct {
		name    string
		initial *string
		pushed  string
	}{
		{
			name:   "initial-missing-popped-missing",
			pushed: "hello",
		},
		{
			name:    "initial-empty-popped-empty",
			initial: &empty,
			pushed:  "hello",
		},
		{
			name:    "initial-set-popped-set",
			initial: &filled,
			pushed:  "hello",
		},
	}
	for _, tc := range cases {
		if tc.initial == nil {
			if err := os.Unsetenv(env); err != nil {
				t.Fatalf("%s: could not unset %s: %v", tc.name, env, err)
			}
		} else {
			if err := os.Setenv(env, *tc.initial); err != nil {
				t.Fatalf("%s: could not set %s: %v", tc.name, env, err)
			}
		}
		f, err := PushEnv(env, tc.pushed)
		if err != nil {
			t.Errorf("%s: push error: %v", tc.name, err)
			continue
		}
		actual, present := os.LookupEnv(env)
		if !present {
			t.Errorf("%s: failed to push %s", tc.name, tc.pushed)
			continue
		}
		if actual != tc.pushed {
			t.Errorf("%s: actual %s != expected %s", tc.name, actual, tc.pushed)
			continue
		}
		if err = f(); err != nil {
			t.Errorf("%s: pop error: %v", tc.name, err)
		}
		actual, present = os.LookupEnv(env)
		if tc.initial == nil && present {
			t.Errorf("%s: env present after popping", tc.name)
			continue
		} else if tc.initial != nil && *tc.initial != actual {
			t.Errorf("%s: popped env is %s not initial %s", tc.name, actual, *tc.initial)
		}
	}

}

func TestMigrateOptions(t *testing.T) {
	ov := "option-value"
	ev := "env-value"

	cases := []struct {
		name           string
		setEnv         bool
		setOption      bool
		push           bool
		expectedEnv    *string
		expectedOption string
	}{
		{
			name: "no flag or env results in no change",
		},
		{
			name:           "flag and env, no push results in no change",
			setEnv:         true,
			setOption:      true,
			expectedEnv:    &ev,
			expectedOption: ov,
		},
		{
			name:           "flag and env, push overwrites env",
			setEnv:         true,
			setOption:      true,
			push:           true,
			expectedEnv:    &ov,
			expectedOption: ov,
		},
		{
			name:           "flag and no env, no push results in no change",
			setOption:      true,
			expectedOption: ov,
		},
		{
			name:           "flag and no env, push overwrites env",
			setOption:      true,
			push:           true,
			expectedEnv:    &ov,
			expectedOption: ov,
		},
		{
			name:           "no flag and env overwrites option",
			setEnv:         true,
			expectedEnv:    &ev,
			expectedOption: ev,
		},
	}

	env := "random-env"

	for _, tc := range cases {
		if tc.setEnv {
			if err := os.Setenv(env, ev); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
		} else if err := os.Unsetenv(env); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}

		opt := ""
		if tc.setOption {
			opt = ov
		}
		if err := MigrateOptions([]MigratedOption{
			{
				Env:      env,
				Option:   &opt,
				Name:     "--random-flag",
				SkipPush: !tc.push,
			},
		}); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}

		val, present := os.LookupEnv(env)
		if present && tc.expectedEnv == nil {
			t.Errorf("%s: env should not be set", tc.name)
		} else if tc.expectedEnv != nil && !present {
			t.Errorf("%s: env should be set", tc.name)
		} else if tc.expectedEnv != nil && val != *tc.expectedEnv {
			t.Errorf("%s: env actual %s != expected %s", tc.name, val, *tc.expectedEnv)
		}

		if tc.expectedOption != opt {
			t.Errorf("%s: option actual %s != expected %s", tc.name, opt, tc.expectedOption)
		}
	}
}

func TestAppendField(t *testing.T) {
	flag := "--target"
	add := "hello"
	cases := []struct {
		name     string
		start    string
		expected string
	}{
		{
			name:     "missing",
			start:    "--a=1 --b=2",
			expected: "--a=1 --b=2 --target=hello",
		},
		{
			name:     "empty",
			start:    "--target= --b=2",
			expected: "--b=2 --target=hello",
		},
		{
			name:     "set",
			start:    "--target=first --b=2",
			expected: "--b=2 --target=first-hello",
		},
	}

	for _, tc := range cases {
		actual := strings.Join(AppendField(strings.Fields(tc.start), flag, add), " ")
		if actual != tc.expected {
			t.Errorf("%s: actual %s != expected %s", tc.name, actual, tc.expected)
		}
	}
}

func TestSetFieldDefault(t *testing.T) {
	flag := "--target"
	def := "default-value"
	cases := []struct {
		name     string
		start    string
		expected string
	}{
		{
			name:     "missing",
			start:    "--a 1 --b 2",
			expected: "--a 1 --b 2 --target=default-value",
		},
		{
			name:     "empty",
			start:    "--target= --b=2",
			expected: "--b=2 --target=",
		},
		{
			name:     "set",
			start:    "--target=1 --b=2",
			expected: "--b=2 --target=1",
		},
	}

	for _, tc := range cases {
		actual := strings.Join(SetFieldDefault(strings.Fields(tc.start), flag, def), " ")
		if actual != tc.expected {
			t.Errorf("%s: actual %s != expected %s", tc.name, actual, tc.expected)
		}
	}
}

func TestExtractField(t *testing.T) {
	cases := []struct {
		name      string
		start     string
		target    string
		out       string
		extracted string
		found     bool
	}{
		{
			name:      "not present",
			start:     "--a=1 --b=2 --c=3",
			target:    "--missing",
			out:       "--a=1 --b=2 --c=3",
			extracted: "",
			found:     false,
		},
		{
			name:      "found filled",
			start:     "--a=1 --b=2 --c=3",
			target:    "--b",
			out:       "--a=1 --c=3",
			extracted: "2",
			found:     true,
		},
		{
			name:      "found empty",
			start:     "--a=1 --b= --c=3",
			target:    "--b",
			out:       "--a=1 --c=3",
			extracted: "",
			found:     true,
		},
		{
			name:      "found space instead of =",
			start:     "--a 1 --b 2 --c=3",
			target:    "--b",
			out:       "--a 1 --c=3",
			extracted: "2",
			found:     true,
		},
	}
	for _, tc := range cases {
		f, extracted, found := ExtractField(strings.Fields(tc.start), tc.target)
		out := strings.Join(f, " ")
		if out != tc.out {
			t.Errorf("%s: actual fields %s != expected %s", tc.name, out, tc.out)
		}
		if extracted != tc.extracted {
			t.Errorf("%s: actual extracted %s != expected %s", tc.name, extracted, tc.extracted)
		}
		if found != tc.found {
			t.Errorf("%s: actual found %t != expected %t", tc.name, found, tc.found)
		}
	}
}
