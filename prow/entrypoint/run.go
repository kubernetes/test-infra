/*
Copyright 2018 The Kubernetes Authors.

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

package entrypoint

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"k8s.io/test-infra/prow/errorutil"

	"github.com/sirupsen/logrus"
)

const (
	// InternalErrorCode is what we write to the marker file to
	// indicate that we failed to start the wrapped command
	InternalErrorCode = "127"

	// DefaultTimeout is the default timeout for the test
	// process before SIGINT is sent
	DefaultTimeout = 120 * time.Minute

	// DefaultGracePeriod is the default timeout for the test
	// process after SIGINT is sent before SIGKILL is sent
	DefaultGracePeriod = 15 * time.Second
)

var (
	// errTimedOut is used as the command's error when the command
	// is terminated after the timeout is reached
	errTimedOut = errors.New("process timed out")
)

// Run creates the artifact directory then executes the process as configured,
// writing the output to the process log and the exit code to the marker file
// on exit.
func (o Options) Run() error {
	if o.ArtifactDir != "" {
		if err := os.MkdirAll(o.ArtifactDir, os.ModePerm); err != nil {
			return errorutil.NewAggregate(
				fmt.Errorf("could not create artifact directory(%s): %v", o.ArtifactDir, err),
				o.mark(InternalErrorCode),
			)
		}
	}
	processLogFile, err := os.Create(o.ProcessLog)
	if err != nil {
		return errorutil.NewAggregate(
			fmt.Errorf("could not create process logfile(%s): %v", o.ProcessLog, err),
			o.mark(InternalErrorCode),
		)
	}

	output := io.MultiWriter(os.Stdout, processLogFile)
	logrus.SetOutput(output)

	executable := o.Args[0]
	var arguments []string
	if len(o.Args) > 1 {
		arguments = o.Args[1:]
	}
	command := exec.Command(executable, arguments...)
	command.Stderr = output
	command.Stdout = output
	if err := command.Start(); err != nil {
		return errorutil.NewAggregate(
			fmt.Errorf("could not start the process: %v", err),
			o.mark(InternalErrorCode),
		)
	}

	// if we get asked to terminate we need to forward
	// that to the wrapped process as if it timed out
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	timeout := optionOrDefault(o.Timeout, DefaultTimeout)
	gracePeriod := optionOrDefault(o.GracePeriod, DefaultGracePeriod)
	var commandErr error
	cancelled := false
	done := make(chan error)
	go func() {
		done <- command.Wait()
	}()
	select {
	case err := <-done:
		commandErr = err
	case <-time.After(timeout):
		logrus.Errorf("Process did not finish before %s timeout", timeout)
		cancelled = true
		gracefullyTerminate(command, done, gracePeriod)
	case s := <-interrupt:
		logrus.Errorf("Entrypoint received interrupt: %v", s)
		cancelled = true
		gracefullyTerminate(command, done, gracePeriod)
	}
	// Close the process logfile before writing the marker file to avoid racing
	// with the sidecar container.
	processLogFile.Close()

	var returnCode string
	if cancelled {
		returnCode = InternalErrorCode
		commandErr = errTimedOut
	} else {
		if status, ok := command.ProcessState.Sys().(syscall.WaitStatus); ok {
			returnCode = strconv.Itoa(status.ExitStatus())
		} else if commandErr == nil {
			returnCode = "0"
		} else {
			returnCode = "1"
			commandErr = fmt.Errorf("wrapped process failed: %v", commandErr)
		}
	}
	return errorutil.NewAggregate(commandErr, o.mark(returnCode))
}

func (o *Options) mark(exitCode string) error {
	if err := ioutil.WriteFile(o.MarkerFile, []byte(exitCode), os.ModePerm); err != nil {
		return fmt.Errorf("could not write to marker file(%s): %v", o.MarkerFile, err)
	}
	return nil
}

// optionOrDefault defaults to a value if option
// is the zero value
func optionOrDefault(option, defaultValue time.Duration) time.Duration {
	if option == 0 {
		return defaultValue
	}

	return option
}

func gracefullyTerminate(command *exec.Cmd, done <-chan error, gracePeriod time.Duration) {
	if err := command.Process.Signal(os.Interrupt); err != nil {
		logrus.WithError(err).Error("Could not interrupt process after timeout")
	}
	select {
	case <-done:
		logrus.Errorf("Process gracefully exited before %s grace period", gracePeriod)
		// but we ignore the output error as we will want errTimedOut
	case <-time.After(gracePeriod):
		logrus.Errorf("Process did not exit before %s grace period", gracePeriod)
		if err := command.Process.Kill(); err != nil {
			logrus.WithError(err).Error("Could not kill process after grace period")
		}
	}
}
