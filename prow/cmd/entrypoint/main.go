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
	"flag"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pod-utils/wrapper"
)

type options struct {
	args    []string
	timeout time.Duration

	wrapperOptions *wrapper.Options
}

func (o *options) Validate() error {
	if len(o.args) == 0 {
		return errors.New("no process to wrap specified")
	}

	return o.wrapperOptions.Validate()
}

func (o *options) Complete(args []string) {
	o.args = args
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	o.wrapperOptions = wrapper.BindOptions(fs)
	fs.DurationVar(&o.timeout, "timeout", 5*time.Hour, "Timeout for the test command.")
	fs.Parse(os.Args[1:])
	o.Complete(fs.Args())
	return o
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "entrypoint"}),
	)

	processLogFile, err := os.Create(o.wrapperOptions.ProcessLog)
	if err != nil {
		logrus.WithError(err).Fatal("Could not open output process logfile")
	}
	output := io.MultiWriter(os.Stdout, processLogFile)
	logrus.SetOutput(output)

	executable := o.args[0]
	var arguments []string
	if len(o.args) > 1 {
		arguments = o.args[1:]
	}
	command := exec.Command(executable, arguments...)
	command.Stderr = output
	command.Stdout = output
	if err := command.Start(); err != nil {
		if err := ioutil.WriteFile(o.wrapperOptions.MarkerFile, []byte("127"), os.ModePerm); err != nil {
			logrus.WithError(err).Fatal("Could not write to marker file")
		}
		logrus.WithError(err).Fatal("Could not start the process")
	}

	var commandErr error
	done := make(chan error)
	go func() {
		done <- command.Wait()
	}()
	select {
	case err := <-done:
		commandErr = err
	case <-time.After(o.timeout):
		logrus.Errorf("Process did not finish before %s timeout", o.timeout)
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

	if err := ioutil.WriteFile(o.wrapperOptions.MarkerFile, []byte(returnCode), os.ModePerm); err != nil {
		logrus.WithError(err).Fatal("Could not write return code to marker file")
	}
	if commandErr != nil {
		logrus.WithError(err).Fatal("Wrapped process failed")
	}
}
