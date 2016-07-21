/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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

// Package cmdlog is just a few useful functions for running os commands and
// logging their output.
package cmdlog

import (
	"bufio"
	"log"
	"os/exec"
	"strings"
)

// LogCommand just logs the name and arguments of the command.
func LogCommand(cmd *exec.Cmd) {
	log.Printf("Running: %s", strings.Join(cmd.Args, " "))
}

// RunWithLogs prints the command, sets it to output to log, then runs it.
func RunWithLogs(cmd *exec.Cmd) error {
	if err := CommandLogger(cmd); err != nil {
		return err
	}
	LogCommand(cmd)
	return cmd.Run()
}

// CommandLogger streams the command's output to the log.
func CommandLogger(cmd *exec.Cmd) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	outScanner := bufio.NewScanner(stdout)
	errScanner := bufio.NewScanner(stderr)

	go func() {
		for outScanner.Scan() {
			if text := outScanner.Text(); len(text) != 0 {
				log.Printf(text)
			}
		}
		if err := outScanner.Err(); err != nil {
			log.Printf("Unexpected error scanning stdout: %s", err)
		}
	}()

	go func() {
		for errScanner.Scan() {
			if text := errScanner.Text(); len(text) != 0 {
				log.Printf(text)
			}
		}
		if err := errScanner.Err(); err != nil {
			log.Printf("Unexpected error scanning stderr: %s", err)
		}
	}()

	return nil
}
