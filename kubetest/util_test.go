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

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
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
		f, err := pushEnv(env, tc.pushed)
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

func TestXmlWrap(t *testing.T) {
	cases := []struct {
		name            string
		interrupted     bool
		shouldInterrupt bool
		err             string
		expectSkipped   bool
		expectError     bool
	}{
		{
			name: "xmlWrap can pass",
		},
		{
			name:        "xmlWrap can error",
			err:         "hello there",
			expectError: true,
		},
		{
			name:            "xmlWrap always errors on interrupt",
			err:             "",
			shouldInterrupt: true,
			expectError:     true,
		},
		{
			name:            "xmlWrap errors on interrupt",
			shouldInterrupt: true,
			err:             "the step failed",
			expectError:     true,
		},
		{
			name:          "xmlWrap skips errors when already interrupted",
			interrupted:   true,
			err:           "this failed because we interrupted the previous step",
			expectSkipped: true,
		},
		{
			name:        "xmlWrap can pass when interrupted",
			interrupted: true,
			err:         "",
		},
	}

	for _, tc := range cases {
		interrupted = tc.interrupted
		suite.Cases = suite.Cases[:0]
		suite.Failures = 6
		suite.Tests = 9
		err := xmlWrap(tc.name, func() error {
			if tc.shouldInterrupt {
				interrupted = true
			}
			if tc.err != "" {
				return errors.New(tc.err)
			}
			return nil
		})
		if tc.shouldInterrupt && tc.expectError {
			if err == nil {
				t.Fatalf("Case %s did not error", tc.name)
			}
			if tc.err == "" {
				tc.err = err.Error()
			}
		}
		if (tc.err == "") != (err == nil) {
			t.Errorf("Case %s expected err: %s != actual: %v", tc.name, tc.err, err)
		}
		if tc.shouldInterrupt && !interrupted {
			t.Errorf("Case %s did not interrupt", tc.name)
		}
		if len(suite.Cases) != 1 {
			t.Fatalf("Case %s did not result in a single suite testcase: %v", tc.name, suite.Cases)
		}
		sc := suite.Cases[0]
		if sc.Name != tc.name {
			t.Errorf("Case %s resulted in wrong test case name %s", tc.name, sc.Name)
		}
		if tc.expectError {
			if sc.Failure != tc.err {
				t.Errorf("Case %s expected error %s but got %s", tc.name, tc.err, sc.Failure)
			}
			if suite.Failures != 7 {
				t.Errorf("Case %s failed and should increase suite failures from 6 to 7, found: %d", tc.name, suite.Failures)
			}
		} else if tc.expectSkipped {
			if sc.Skipped != tc.err {
				t.Errorf("Case %s expected skipped %s but got %s", tc.name, tc.err, sc.Skipped)
			}
			if suite.Failures != 7 {
				t.Errorf("Case %s interrupted and increase suite failures from 6 to 7, found: %d", tc.name, suite.Failures)
			}
		} else {
			if suite.Failures != 6 {
				t.Errorf("Case %s passed so suite failures should remain at 6, found: %d", tc.name, suite.Failures)
			}
		}

	}
}

func TestOutput(t *testing.T) {
	cases := []struct {
		name              string
		terminated        bool
		interrupted       bool
		causeTermination  bool
		causeInterruption bool
		pass              bool
		sleep             int
		output            bool
		shouldError       bool
		shouldInterrupt   bool
		shouldTerminate   bool
	}{
		{
			name: "finishRunning can pass",
			pass: true,
		},
		{
			name:   "output can pass",
			output: true,
			pass:   true,
		},
		{
			name:        "finishRuning can fail",
			pass:        false,
			shouldError: true,
		},
		{
			name:        "output can fail",
			pass:        false,
			output:      true,
			shouldError: true,
		},
		{
			name:        "finishRunning should error when terminated",
			terminated:  true,
			pass:        true,
			shouldError: true,
		},
		{
			name:        "output should error when terminated",
			terminated:  true,
			pass:        true,
			output:      true,
			shouldError: true,
		},
		{
			name:              "finishRunning should interrupt when interrupted",
			pass:              true,
			sleep:             60,
			causeInterruption: true,
			shouldError:       true,
		},
		{
			name:              "output should interrupt when interrupted",
			pass:              true,
			sleep:             60,
			output:            true,
			causeInterruption: true,
			shouldError:       true,
		},
		{
			name:             "output should terminate when terminated",
			pass:             true,
			sleep:            60,
			output:           true,
			causeTermination: true,
			shouldError:      true,
		},
		{
			name:             "finishRunning should terminate when terminated",
			pass:             true,
			sleep:            60,
			causeTermination: true,
			shouldError:      true,
		},
	}

	clearTimers := func() {
		if !terminate.Stop() {
			<-terminate.C
		}
		if !interrupt.Stop() {
			<-interrupt.C
		}
	}

	for _, tc := range cases {
		log.Println(tc.name)
		terminated = tc.terminated
		interrupted = tc.interrupted
		interrupt = time.NewTimer(time.Duration(0))
		terminate = time.NewTimer(time.Duration(0))
		clearTimers()
		if tc.causeInterruption {
			interrupt.Reset(0)
		}
		if tc.causeTermination {
			terminate.Reset(0)
		}
		var cmd *exec.Cmd
		if !tc.pass {
			cmd = exec.Command("false")
		} else if tc.sleep == 0 {
			cmd = exec.Command("true")
		} else {
			cmd = exec.Command("sleep", strconv.Itoa(tc.sleep))
		}
		var err error
		if tc.output {
			_, err = output(cmd)
		} else {
			err = finishRunning(cmd)
		}
		if err == nil == tc.shouldError {
			t.Errorf("Step %s shouldError=%v error: %v", tc.name, tc.shouldError, err)
		}
		if tc.causeInterruption && !interrupted {
			t.Errorf("Step %s did not interrupt, err: %v", tc.name, err)
		} else if tc.causeInterruption && !terminate.Reset(0) {
			t.Errorf("Step %s did not reset the terminate timer: %v", tc.name, err)
		}
		if tc.causeTermination && !terminated {
			t.Errorf("Step %s did not terminate, err: %v", tc.name, err)
		}
	}
	terminated = false
	interrupted = false
	if !terminate.Stop() {
		<-terminate.C
	}
}

func TestFinishRunningParallel(t *testing.T) {
	cases := []struct {
		name              string
		terminated        bool
		interrupted       bool
		causeTermination  bool
		causeInterruption bool
		cmds              []*exec.Cmd
		shouldError       bool
		shouldInterrupt   bool
		shouldTerminate   bool
	}{
		{
			name: "finishRunningParallel with single command can pass",
			cmds: []*exec.Cmd{exec.Command("true")},
		},
		{
			name: "finishRunningParallel with multiple commands can pass",
			cmds: []*exec.Cmd{exec.Command("true"), exec.Command("true")},
		},
		{
			name:        "finishRunningParallel with single command can fail",
			cmds:        []*exec.Cmd{exec.Command("false")},
			shouldError: true,
		},
		{
			name:        "finishRunningParallel with multiple commands can fail",
			cmds:        []*exec.Cmd{exec.Command("true"), exec.Command("false")},
			shouldError: true,
		},
		{
			name:        "finishRunningParallel should error when terminated",
			cmds:        []*exec.Cmd{exec.Command("true"), exec.Command("true")},
			terminated:  true,
			shouldError: true,
		},
		{
			name:              "finishRunningParallel should interrupt when interrupted",
			cmds:              []*exec.Cmd{exec.Command("true"), exec.Command("sleep", "60"), exec.Command("sleep", "30")},
			causeInterruption: true,
			shouldError:       true,
		},
		{
			name:             "finishRunningParallel should terminate when terminated",
			cmds:             []*exec.Cmd{exec.Command("true"), exec.Command("sleep", "60"), exec.Command("sleep", "30")},
			causeTermination: true,
			shouldError:      true,
		},
	}

	clearTimers := func() {
		if !terminate.Stop() {
			<-terminate.C
		}
		if !interrupt.Stop() {
			<-interrupt.C
		}
	}

	for _, tc := range cases {
		log.Println(tc.name)
		terminated = tc.terminated
		interrupted = tc.interrupted
		interrupt = time.NewTimer(time.Duration(0))
		terminate = time.NewTimer(time.Duration(0))
		clearTimers()
		if tc.causeInterruption {
			interrupt.Reset(1 * time.Second)
		}
		if tc.causeTermination {
			terminate.Reset(1 * time.Second)
		}

		err := finishRunningParallel(tc.cmds...)
		if err == nil == tc.shouldError {
			t.Errorf("TC %q shouldError=%v error: %v", tc.name, tc.shouldError, err)
		}
		if tc.causeInterruption && !interrupted {
			t.Errorf("TC %q did not interrupt, err: %v", tc.name, err)
		} else if tc.causeInterruption && !terminate.Reset(0) {
			t.Errorf("TC %q did not reset the terminate timer: %v", tc.name, err)
		}
		if tc.causeTermination && !terminated {
			t.Errorf("TC %q did not terminate, err: %v", tc.name, err)
		}
	}
	terminated = false
	interrupted = false
	if !terminate.Stop() {
		<-terminate.C
	}
}

func TestOutputOutputs(t *testing.T) {
	b, err := output(exec.Command("echo", "hello world"))
	txt := string(b)
	if err != nil {
		t.Fatalf("failed to echo: %v", err)
	}
	if !strings.Contains(txt, "hello world") {
		t.Errorf("output() did not echo hello world: %v", txt)
	}
}

func TestHttpFileScheme(t *testing.T) {
	expected := "some testdata"
	tmpfile, err := ioutil.TempFile("", "test_http_file_scheme")
	if err != nil {
		t.Errorf("Error creating temporary file: %v", err)
	}
	defer os.Remove(tmpfile.Name())
	if _, err := tmpfile.WriteString(expected); err != nil {
		t.Errorf("Error writing to temporary file: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Errorf("Error closing temporary file: %v", err)
	}

	fileURL := fmt.Sprintf("file://%s", tmpfile.Name())
	buf := new(bytes.Buffer)
	if err := httpRead(fileURL, buf); err != nil {
		t.Errorf("Error reading temporary file through httpRead: %v", err)
	}

	if buf.String() != expected {
		t.Errorf("httpRead(%s): expected %v, got %v", fileURL, expected, buf)
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
			name:           "flag and no env, push overwites env",
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
		if err := migrateOptions([]migratedOption{
			{
				env:      env,
				option:   &opt,
				name:     "--random-flag",
				skipPush: !tc.push,
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
		actual := strings.Join(appendField(strings.Fields(tc.start), flag, add), " ")
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
		actual := strings.Join(setFieldDefault(strings.Fields(tc.start), flag, def), " ")
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
		f, extracted, found := extractField(strings.Fields(tc.start), tc.target)
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

func TestGetLatestClusterUpTime(t *testing.T) {
	const magicTime = "2011-11-11T11:11:11.111-11:00"
	myTime, err := time.Parse(time.RFC3339, magicTime)
	if err != nil {
		t.Fatalf("Fail parsing time: %v", err)
	}

	cases := []struct {
		name         string
		body         string
		expectedTime time.Time
		expectErr    bool
	}{
		{
			name:      "bad json",
			body:      "abc",
			expectErr: true,
		},
		{
			name:         "empty json",
			body:         "[]",
			expectedTime: time.Time{},
		},
		{
			name:         "valid json",
			body:         "[{\"name\": \"foo\", \"creationTimestamp\": \"2011-11-11T11:11:11.111-11:00\"}]",
			expectedTime: myTime,
		},
		{
			name:      "bad time format",
			body:      "[{\"name\": \"foo\", \"creationTimestamp\": \"blah-blah\"}]",
			expectErr: true,
		},
		{
			name:         "multiple entries",
			body:         "[{\"name\": \"foo\", \"creationTimestamp\": \"2011-11-11T11:11:11.111-11:00\"}, {\"name\": \"bar\", \"creationTimestamp\": \"2010-10-10T11:11:11.111-11:00\"}]",
			expectedTime: myTime,
		},
	}
	for _, tc := range cases {
		time, err := getLatestClusterUpTime(tc.body)
		if err != nil && !tc.expectErr {
			t.Errorf("%s: got unexpected error %v", tc.name, err)
		}
		if err == nil && tc.expectErr {
			t.Errorf("%s: expect error but did not get one", tc.name)
		}
		if !tc.expectErr && !time.Equal(tc.expectedTime) {
			t.Errorf("%s: expect time %v, but got %v", tc.name, tc.expectedTime, time)
		}
	}
}
