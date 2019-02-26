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
	"os"
	"os/exec"
	"os/signal"
)

// Exec generally mimics syscall.Exec behavior, but using a child process
// isntead to make testing etc. easier
func Exec(argv0 string, args []string, env []string) error {
	// construct command from inputs
	cmd := exec.Command(argv0, args...)
	cmd.Env = env

	// inherit some standard file descriptors, as if `syscall.Exec`ed
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return execCmdWithSignals(cmd)
}

func execCmdWithSignals(cmd *exec.Cmd) error {
	// setup listener to forward all signals
	// TODO(bentheelder): what should this buffer size be?
	signals := make(chan os.Signal, 5)
	signal.Notify(signals)
	defer signal.Stop(signals)

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
			return err
		}
	}
}
