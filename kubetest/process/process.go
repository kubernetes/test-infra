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
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"k8s.io/test-infra/kubetest/util"
)

// Control can commands until a timeout is reached, at which point it signals and then terminates them.
type Control struct {
	termLock    *sync.RWMutex
	terminated  bool
	intLock     *sync.RWMutex
	interrupted bool

	Timeout   time.Duration
	Interrupt *time.Timer
	Terminate *time.Timer

	verbose bool
}

// NewControl constructs a Control with the specified arguments, instiating other necessary fields.
func NewControl(timeout time.Duration, interrupt, terminate *time.Timer, verbose bool) *Control {
	return &Control{
		termLock:    new(sync.RWMutex),
		terminated:  false,
		intLock:     new(sync.RWMutex),
		interrupted: false,
		Timeout:     timeout,
		Interrupt:   interrupt,
		Terminate:   terminate,
		verbose:     verbose,
	}
}

// WriteXML creates a util.TestCase{} junit_runner.xml file inside the dump dir.
func (c *Control) WriteXML(suite *util.TestSuite, dump string, start time.Time) {
	// Note whether timeout occurred
	tc := util.TestCase{
		Name:      "Timeout",
		ClassName: "e2e.go",
		Time:      c.Timeout.Seconds(),
	}
	if c.isInterrupted() {
		tc.Failure = "kubetest --timeout triggered"
		suite.Failures++
	}
	suite.Cases = append(suite.Cases, tc)
	// Write xml
	suite.Time = time.Since(start).Seconds()
	out, err := xml.MarshalIndent(&suite, "", "    ")
	if err != nil {
		log.Fatalf("Could not marshal XML: %s", err)
	}
	path := filepath.Join(dump, "junit_runner.xml")
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("Could not create file: %s", err)
	}
	defer f.Close()
	if _, err := f.WriteString(xml.Header); err != nil {
		log.Fatalf("Error writing XML header: %s", err)
	}
	if _, err := f.Write(out); err != nil {
		log.Fatalf("Error writing XML data: %s", err)
	}
	log.Printf("Saved XML output to %s.", path)
}

// XMLWrap returns f(), adding junit xml testcase result for name
func (c *Control) XMLWrap(suite *util.TestSuite, name string, f func() error) error {
	alreadyInterrupted := c.isInterrupted()
	start := time.Now()
	err := f()
	duration := time.Since(start)
	tc := util.TestCase{
		Name:      name,
		ClassName: "e2e.go",
		Time:      duration.Seconds(),
	}
	if err == nil && !alreadyInterrupted && c.isInterrupted() {
		err = fmt.Errorf("kubetest interrupted during step %s", name)
	}
	if err != nil {
		if !alreadyInterrupted {
			tc.Failure = err.Error()
		} else {
			tc.Skipped = err.Error()
		}
		suite.Failures++
	}

	suite.Cases = append(suite.Cases, tc)
	suite.Tests++
	return err
}

func (c *Control) isTerminated() bool {
	c.termLock.RLock()
	t := c.terminated
	c.termLock.RUnlock()
	return t
}

func (c *Control) isInterrupted() bool {
	c.intLock.RLock()
	i := c.interrupted
	c.intLock.RUnlock()
	return i
}

// FinishRunning returns cmd.Wait() and/or times out.
func (c *Control) FinishRunning(cmd *exec.Cmd) error {
	stepName := strings.Join(cmd.Args, " ")
	if c.isTerminated() {
		return fmt.Errorf("skipped %s (kubetest is terminated)", stepName)
	}
	if cmd.Stdout == nil && c.verbose {
		cmd.Stdout = os.Stdout
	}
	if cmd.Stderr == nil && c.verbose {
		cmd.Stderr = os.Stderr
	}
	log.Printf("Running: %v", stepName)
	defer func(start time.Time) {
		log.Printf("Step '%s' finished in %s", stepName, time.Since(start))
	}(time.Now())

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error starting %v: %v", stepName, err)
	}

	finished := make(chan error)

	sigChannel := make(chan os.Signal, 1)
	signal.Notify(sigChannel, os.Interrupt)

	go func() {
		finished <- cmd.Wait()
	}()

	for {
		select {
		case <-sigChannel:
			log.Printf("Killing %v(%v) after receiving signal", stepName, -cmd.Process.Pid)

			pgid := getGroupPid(cmd.Process.Pid)

			if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
				log.Printf("Failed to kill %v: %v", stepName, err)
			}

		case <-c.Terminate.C:
			c.termLock.Lock()
			c.terminated = true
			c.termLock.Unlock()
			c.Terminate.Reset(time.Duration(-1)) // Kill subsequent processes immediately.
			pgid := getGroupPid(cmd.Process.Pid)
			if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
				log.Printf("Failed to kill %v: %v", stepName, err)
			}
			if err := cmd.Process.Kill(); err != nil {
				log.Printf("Failed to terminate %s (terminated 15m after interrupt): %v", stepName, err)
			}
		case <-c.Interrupt.C:
			c.intLock.Lock()
			c.interrupted = true
			c.intLock.Unlock()
			log.Printf("Interrupt after %s timeout during %s. Will terminate in another 15m", c.Timeout, stepName)
			c.Terminate.Reset(15 * time.Minute)
			pgid := getGroupPid(cmd.Process.Pid)
			if err := syscall.Kill(-pgid, syscall.SIGINT); err != nil {
				log.Printf("Failed to interrupt %s. Will terminate immediately: %v", stepName, err)
				syscall.Kill(-pgid, syscall.SIGTERM)
				cmd.Process.Kill()
			}
		case err := <-finished:
			if err != nil {
				var suffix string
				if c.isTerminated() {
					suffix = " (terminated)"
				} else if c.isInterrupted() {
					suffix = " (interrupted)"
				}
				return fmt.Errorf("error during %s%s: %v", stepName, suffix, err)
			}
			return err
		}
	}
}

type cmdExecResult struct {
	stepName string
	output   string
	execTime time.Duration
	err      error
}

// executeParallelCommand executes a given command and send output and error via channel
func (c *Control) executeParallelCommand(cmd *exec.Cmd, resChan chan cmdExecResult, termChan, intChan chan struct{}) {
	stepName := strings.Join(cmd.Args, " ")
	stdout := bytes.Buffer{}
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout

	start := time.Now()
	log.Printf("Running: %v in parallel", stepName)

	if c.isTerminated() {
		resChan <- cmdExecResult{stepName: stepName, output: stdout.String(), execTime: time.Since(start), err: fmt.Errorf("skipped %s (kubetest is terminated)", stepName)}
		return
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		resChan <- cmdExecResult{stepName: stepName, output: stdout.String(), execTime: time.Since(start), err: fmt.Errorf("error starting %v: %v", stepName, err)}
		return
	}

	finished := make(chan error)
	go func() {
		finished <- cmd.Wait()
	}()

	for {
		select {
		case err := <-finished:
			if err != nil {
				var suffix string
				if c.isTerminated() {
					suffix = " (terminated)"
				} else if c.isInterrupted() {
					suffix = " (interrupted)"
				}
				err = fmt.Errorf("error during %s%s: %v", stepName, suffix, err)
			}
			resChan <- cmdExecResult{stepName: stepName, output: stdout.String(), execTime: time.Since(start), err: err}
			return

		case <-termChan:
			pgid := getGroupPid(cmd.Process.Pid)
			syscall.Kill(-pgid, syscall.SIGKILL)
			if err := cmd.Process.Kill(); err != nil {
				log.Printf("Failed to terminate %s (terminated 15m after interrupt): %v", strings.Join(cmd.Args, " "), err)
			}

		case <-intChan:
			log.Printf("Abort after %s timeout during %s. Will terminate in another 15m", c.Timeout, strings.Join(cmd.Args, " "))
			pgid := getGroupPid(cmd.Process.Pid)
			if err := syscall.Kill(-pgid, syscall.SIGABRT); err != nil {
				log.Printf("Failed to abort %s. Will terminate immediately: %v", strings.Join(cmd.Args, " "), err)
				syscall.Kill(-pgid, syscall.SIGTERM)
				cmd.Process.Kill()
			}
		}
	}
}

// FinishRunningParallel executes multiple commands in parallel
func (c *Control) FinishRunningParallel(cmds ...*exec.Cmd) error {
	var wg sync.WaitGroup
	resultChan := make(chan cmdExecResult, len(cmds))
	termChan := make(chan struct{}, len(cmds))
	intChan := make(chan struct{}, len(cmds))

	for _, cmd := range cmds {
		wg.Add(1)
		go func(cmd *exec.Cmd) {
			defer wg.Done()
			c.executeParallelCommand(cmd, resultChan, termChan, intChan)
		}(cmd)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	cmdFailed := false
	for {
		select {
		case <-c.Terminate.C:
			c.termLock.Lock()
			c.terminated = true
			c.termLock.Unlock()
			c.Terminate.Reset(time.Duration(0))
			select {
			case <-termChan:
			default:
				close(termChan)
			}

		case <-c.Interrupt.C:
			c.intLock.Lock()
			c.interrupted = true
			c.intLock.Unlock()
			c.Terminate.Reset(15 * time.Minute)
			close(intChan)

		case result, ok := <-resultChan:
			if !ok {
				if cmdFailed {
					return fmt.Errorf("one or more commands failed")
				}
				return nil
			}
			log.Print(result.output)
			if result.err != nil {
				cmdFailed = true
			}
			log.Printf("Step '%s' finished in %s", result.stepName, result.execTime)
		}
	}
}

// InputCommand returns exec.Command(cmd, args...) while calling .StdinPipe().WriteString(input)
func (c *Control) InputCommand(input, cmd string, args ...string) (*exec.Cmd, error) {
	command := exec.Command(cmd, args...)
	w, e := command.StdinPipe()
	if e != nil {
		return nil, e
	}
	go func() {
		if _, e = io.WriteString(w, input); e != nil {
			log.Printf("Failed to write all %d chars to %s: %v", len(input), cmd, e)
		}
		if e = w.Close(); e != nil {
			log.Printf("Failed to close stdin for %s: %v", cmd, e)
		}
	}()
	return command, nil
}

// Output returns cmd.Output(), potentially timing out in the process.
func (c *Control) Output(cmd *exec.Cmd) ([]byte, error) {
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := c.FinishRunning(cmd)
	return stdout.Bytes(), err
}

// NoOutput ignores all output from the command, potentially timing out in the process.
func (c *Control) NoOutput(cmd *exec.Cmd) error {
	var void bytes.Buffer
	cmd.Stdout = &void
	cmd.Stderr = &void
	return c.FinishRunning(cmd)
}

// getGroupPid gets the process group to kill the entire main/child process
// if Getpgid return error use the current process Pid
func getGroupPid(pid int) int {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		log.Printf("Failed to get the group process from %v: %v", pid, err)
		return pid
	}
	return pgid
}
