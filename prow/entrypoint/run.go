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
	"strconv"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

// InternalErrorCode is what we write to the marker file to
// indicate that we failed to start the wrapped command
const InternalErrorCode = "127"

// Run executes the process as configured, writing the output
// to the process log and the exit code to the marker file on
// exit.
func (o Options) Run() error {
	processLogFile, err := os.Create(o.ProcessLog)
	if err != nil {
		return fmt.Errorf("could not open output process logfile: %v", err)
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
		if err := ioutil.WriteFile(o.MarkerFile, []byte(InternalErrorCode), os.ModePerm); err != nil {
			return fmt.Errorf("could not write to marker file: %v", err)
		}
		return fmt.Errorf("could not start the process: %v", err)
	}

	timeout := time.Duration(o.TimeoutMinutes) * time.Minute
	var commandErr error
	done := make(chan error)
	go func() {
		done <- command.Wait()
	}()
	select {
	case err := <-done:
		commandErr = err
	case <-time.After(timeout):
		logrus.Errorf("Process did not finish before %s timeout", timeout)
		if err := command.Process.Kill(); err != nil {
			logrus.WithError(err).Error("Could not kill process after timeout")
		}
		commandErr = errors.New("process timed out")
	}

	returnCode := "1"
	if commandErr == nil {
		returnCode = "0"
	} else if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			returnCode = strconv.Itoa(status.ExitStatus())
		}
	}

	if err := ioutil.WriteFile(o.MarkerFile, []byte(returnCode), os.ModePerm); err != nil {
		return fmt.Errorf("could not write return code to marker file: %v", err)
	}
	if commandErr != nil {
		return fmt.Errorf("wrapped process failed with code %s: %v", returnCode, err)
	}
	return nil
}
