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

package process

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"os/signal"

	"k8s.io/test-infra/kubetest2/pkg/metadata"
)

type execJunitError struct {
	error
	systemout string
}

func (e *execJunitError) SystemOut() string {
	return e.systemout
}

var _ metadata.JUnitError = &execJunitError{}

// ExecJUnit is like exec, except that it tees the output and captures it
// for returning a metadata.JUnitError if the process does not exit success
func ExecJUnit(argv0 string, args []string, env []string) error {
	// construct command from inputs
	cmd := exec.Command(argv0, args...)
	cmd.Env = env

	// inherit all standard file descriptors, as if `syscall.Exec`ed
	cmd.Stdin = os.Stdin

	var systemout bytes.Buffer
	cmd.Stdout = io.MultiWriter(&systemout, os.Stdout)
	cmd.Stderr = io.MultiWriter(&systemout, os.Stderr)

	// setup listener to forward all signals
	// TODO(bentheelder): what should this buffer size be?
	signals := make(chan os.Signal, 5)
	signal.Notify(signals)
	defer close(signals)

	// start the process
	if err := cmd.Start(); err != nil {
		return err
	}

	// set up a channel to monitor for when it exits
	wait := make(chan error, 1)
	go func() {
		wait <- cmd.Wait()
		close(wait)
	}()

	// pass all signals to the subcommand until it exits, return the result
	for {
		select {
		case sig := <-signals:
			// TODO(bentheelder): can this actually fail? should we log this?
			cmd.Process.Signal(sig)
		case err := <-wait:
			if err != nil {
				return &execJunitError{
					error:     err,
					systemout: systemout.String(),
				}
			}
			return nil
		}
	}
}
