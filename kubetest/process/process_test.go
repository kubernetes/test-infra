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

package process

import (
	"errors"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"k8s.io/test-infra/kubetest/util"
)

func TestXMLWrap(t *testing.T) {
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
		interrupt := time.NewTimer(time.Duration(0))
		terminate := time.NewTimer(time.Duration(0))
		c := NewControl(time.Duration(0), interrupt, terminate, false)
		c.interrupted = tc.interrupted
		suite := util.TestSuite{
			Failures: 6,
			Tests:    9,
		}
		err := c.XMLWrap(&suite, tc.name, func() error {
			if tc.shouldInterrupt {
				c.interrupted = true
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
		if tc.shouldInterrupt && !c.interrupted {
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

	clearTimers := func(c *Control) {
		if !c.Terminate.Stop() {
			<-c.Terminate.C
		}
		if !c.Interrupt.Stop() {
			<-c.Interrupt.C
		}
	}

	for _, tc := range cases {
		log.Println(tc.name)
		interrupt := time.NewTimer(time.Duration(0))
		terminate := time.NewTimer(time.Duration(0))
		c := NewControl(time.Duration(0), interrupt, terminate, false)
		c.terminated = tc.terminated
		c.interrupted = tc.interrupted
		clearTimers(c)
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
			_, err = c.Output(cmd)
		} else {
			err = c.FinishRunning(cmd)
		}
		if err == nil == tc.shouldError {
			t.Errorf("Step %s shouldError=%v error: %v", tc.name, tc.shouldError, err)
		}
		if tc.causeInterruption && !c.interrupted {
			t.Errorf("Step %s did not interrupt, err: %v", tc.name, err)
		} else if tc.causeInterruption && !terminate.Reset(0) {
			t.Errorf("Step %s did not reset the terminate timer: %v", tc.name, err)
		}
		if tc.causeTermination && !c.terminated {
			t.Errorf("Step %s did not terminate, err: %v", tc.name, err)
		}
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

	clearTimers := func(c *Control) {
		if !c.Terminate.Stop() {
			<-c.Terminate.C
		}
		if !c.Interrupt.Stop() {
			<-c.Interrupt.C
		}
	}

	for _, tc := range cases {
		log.Println(tc.name)
		interrupt := time.NewTimer(time.Duration(0))
		terminate := time.NewTimer(time.Duration(0))
		c := NewControl(time.Duration(0), interrupt, terminate, false)
		c.terminated = tc.terminated
		c.interrupted = tc.interrupted
		clearTimers(c)
		if tc.causeInterruption {
			interrupt.Reset(1 * time.Second)
		}
		if tc.causeTermination {
			terminate.Reset(1 * time.Second)
		}

		err := c.FinishRunningParallel(tc.cmds...)
		if err == nil == tc.shouldError {
			t.Errorf("TC %q shouldError=%v error: %v", tc.name, tc.shouldError, err)
		}
		if tc.causeInterruption && !c.interrupted {
			t.Errorf("TC %q did not interrupt, err: %v", tc.name, err)
		} else if tc.causeInterruption && !terminate.Reset(0) {
			t.Errorf("TC %q did not reset the terminate timer: %v", tc.name, err)
		}
		if tc.causeTermination && !c.terminated {
			t.Errorf("TC %q did not terminate, err: %v", tc.name, err)
		}
	}
}

func TestOutputOutputs(t *testing.T) {
	interrupt := time.NewTimer(time.Duration(1) * time.Second)
	terminate := time.NewTimer(time.Duration(1) * time.Second)
	c := NewControl(time.Duration(1)*time.Second, interrupt, terminate, false)

	b, err := c.Output(exec.Command("echo", "hello world"))
	txt := string(b)
	if err != nil {
		t.Fatalf("failed to echo: %v", err)
	}
	if !strings.Contains(txt, "hello world") {
		t.Errorf("output() did not echo hello world: %v", txt)
	}
}
