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

package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"regexp/syntax"
	"syscall"
	"time"

	"k8s.io/test-infra/experiment/bazinga/pkg/config"
	"k8s.io/test-infra/experiment/bazinga/pkg/junit"
)

// Result includes the exit code if any of the test cases was non-zero as
// well as the produced JUnit document.
type Result struct {
	ExitCode int
	JUnitDoc junit.Document
}

// Run starts the application with the provided configuration. The given ctx
// can be used to cancel an executing test case.
func Run(ctx context.Context, appConfig config.App) (*Result, error) {
	result := &Result{}

	connectStdin := len(appConfig.TestSuites) == 1
	otherFailConditions, err := getFailConditions(appConfig.FailureConditions)
	if err != nil {
		return nil, err
	}

	for _, testSuiteConfig := range appConfig.TestSuites {
		junitTestSuite, err := processTestSuite(
			ctx,
			testSuiteConfig,
			connectStdin,
			appConfig.SendSignals,
			otherFailConditions)
		if err != nil {
			return nil, err
		}
		if result.ExitCode == 0 && junitTestSuite.exitCode > 0 {
			result.ExitCode = junitTestSuite.exitCode
		}
		result.JUnitDoc.TestSuites = append(
			result.JUnitDoc.TestSuites, junitTestSuite.TestSuite)
	}

	f, err := os.Create(appConfig.Output)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if err := result.JUnitDoc.Write(f); err != nil {
		return nil, err
	}

	return result, nil
}

func processTestSuite(
	ctx context.Context,
	testSuiteConfig config.TestSuite,
	connectStdin, sendSignals bool,
	otherFailConditions []failCond) (*junitTestSuiteWrapper, error) {

	failConditions, err := getFailConditions(testSuiteConfig.FailureConditions)
	if err != nil {
		return nil, err
	}
	failConditions = append(failConditions, otherFailConditions...)

	junitTestSuite := &junitTestSuiteWrapper{}
	for _, testCaseConfig := range testSuiteConfig.TestCases {
		junitTestCase, err := processTestCase(
			ctx,
			testCaseConfig,
			connectStdin && len(testSuiteConfig.TestCases) == 1,
			sendSignals,
			failConditions)
		if err != nil {
			return nil, err
		}
		if junitTestSuite.exitCode == 0 && junitTestCase.exitCode > 0 {
			junitTestSuite.exitCode = junitTestCase.exitCode
		}
		junitTestSuite.TestCases = append(
			junitTestSuite.TestCases, junitTestCase.TestCase)
	}

	return junitTestSuite, nil
}

func processTestCase(
	ctx context.Context,
	testCaseConfig config.TestCase,
	connectStdin, sendSignals bool,
	otherFailConditions []failCond) (*junitTestCaseWrapper, error) {

	failConditions, err := getFailConditions(testCaseConfig.FailureConditions)
	if err != nil {
		return nil, err
	}
	failConditions = append(failConditions, otherFailConditions...)

	junitTestCase := &junitTestCaseWrapper{}
	junitTestCase.Name = testCaseConfig.Name

	cmd := exec.CommandContext(
		ctx, testCaseConfig.Command, testCaseConfig.Args...)

	if connectStdin {
		cmd.Stdin = os.Stdin
	}

	// Create a temporary file to contain the output of the command
	// in case it needs to be added to the test case as failure content.
	tmpOut, err := ioutil.TempFile("", "")
	if err != nil {
		return nil, err
	}
	defer func() {
		tmpOut.Close()
		os.RemoveAll(tmpOut.Name())
	}()

	// Define a list of writers to which the test case's
	// stdout and stderr will be written.
	stdout := []io.Writer{os.Stdout, tmpOut}
	stderr := []io.Writer{os.Stderr, tmpOut}

	// If there are failure conditions then scan the output.
	if len(failConditions) > 0 {
		// Create a pipe and scanner to efficiently parse both stdout
		// and stderr as they're being written.
		ppr, ppw := io.Pipe()
		scn := bufio.NewScanner(ppr)

		// Create a goroutine to monitor the test case's output
		// for a line that matches the configured failure pattern.
		go func() {
			for scn.Scan() {
				line := scn.Bytes()
				for _, fc := range failConditions {
					if fc.regexp.Match(line) {
						junitTestCase.Failures = append(
							junitTestCase.Failures,
							junit.Failure{
								Type:    fc.category,
								Message: fc.message,
								Text:    line,
							})
					}
				}
			}
		}()

		// Add the pipe's writer sides to the list of pipes given
		// to the test case's process.
		stdout = append(stdout, ppw)
		stderr = append(stderr, ppw)
	}

	// The command's stdout and stderr pipes write to three locations:
	// 1. The parent process's corresponding pipe
	// 2. A temporary file for the combined output
	// 3. A bufio.Scanner that searches for lines that match the
	//    failure regexp.
	cmd.Stdout = io.MultiWriter(stdout...)
	cmd.Stderr = io.MultiWriter(stderr...)

	// Configure the test case's environment.
	if testCaseConfig.EnvClean {
		cmd.Env = []string{}
	} else {
		cmd.Env = os.Environ()
	}
	for _, ev := range testCaseConfig.Env {
		cmd.Env = append(cmd.Env, ev)
	}

	// Record the start time and start the test.
	startTime := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Set up a channel that's close when the process is complete.
	chanDone := make(chan struct{})

	// If the app is forwarding signals to the test case processes
	// then set up a goroutine to do so that returns when the test
	// case process exits.
	if sendSignals {
		chanSigs := make(chan os.Signal, 1)
		go func() {
			for {
				select {
				case <-chanDone:
					// The process is complete, exit the goroutine.
					return
				case sig := <-chanSigs:
					// Forward the signal.
					cmd.Process.Signal(sig)
				default:
					// Don't wait on the above channels
				}
			}
		}()
	}

	// Wait for the command to exit.
	cmdErr := cmd.Wait()

	// Record the end time.
	endTime := time.Now()

	// Indicate that the command has completed.
	close(chanDone)

	if cmdErr != nil {
		if err, ok := cmdErr.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				failed := true
				exitCode := status.ExitStatus()
				successExitCodes := testCaseConfig.SuccessExitCodes
				if len(successExitCodes) == 0 {
					successExitCodes = []int{0}
				}
				for _, successCode := range successExitCodes {
					if exitCode == successCode {
						failed = false
						break
					}
				}
				if failed {
					junitTestCase.exitCode = exitCode
					if len(junitTestCase.Failures) == 0 {
						tmpOut.Seek(0, 0)
						buf, readOutErr := ioutil.ReadAll(tmpOut)
						if readOutErr != nil {
							return nil, readOutErr
						}
						junitTestCase.Failures = append(
							junitTestCase.Failures,
							junit.Failure{
								Message: fmt.Sprintf("%d", exitCode),
								Type:    "exitcode",
								Text:    buf,
							})
					}
				}
			}
		}
	}

	junitTestCase.Time = endTime.Sub(startTime).Seconds()
	if junitTestCase.exitCode == 0 && len(junitTestCase.Failures) > 0 {
		junitTestCase.exitCode = 1
	}

	return junitTestCase, nil
}

type junitTestSuiteWrapper struct {
	junit.TestSuite
	exitCode int
}

type junitTestCaseWrapper struct {
	junit.TestCase
	exitCode int
}

type failCond struct {
	category string
	message  string
	regexp   *regexp.Regexp
}

func getFailConditions(in []config.FailureCondition) ([]failCond, error) {
	out := make([]failCond, len(in))
	for i, fc := range in {
		regexpSyntax, err := syntax.Parse(fc.Pattern, fc.Flags)
		if err != nil {
			return nil, err
		}
		regexpPatt, err := regexp.Compile(regexpSyntax.String())
		if err != nil {
			return nil, err
		}
		out[i].category = fc.Category
		out[i].message = fc.Message
		out[i].regexp = regexpPatt
	}
	return out, nil
}
