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
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Returns $GOPATH/src/k8s.io/...
func k8s(parts ...string) string {
	p := []string{os.Getenv("GOPATH"), "src", "k8s.io"}
	for _, a := range parts {
		p = append(p, a)
	}
	return filepath.Join(p...)
}

// append(errs, err) if err != nil
func appendError(errs []error, err error) []error {
	if err != nil {
		return append(errs, err)
	}
	return errs
}

// Returns $HOME/part/part/part
func home(parts ...string) string {
	p := []string{os.Getenv("HOME")}
	for _, a := range parts {
		p = append(p, a)
	}
	return filepath.Join(p...)
}

// export PATH=path:$PATH
func insertPath(path string) error {
	return os.Setenv("PATH", fmt.Sprintf("%v:%v", path, os.Getenv("PATH")))
}

// Essentially curl url | writer
func httpRead(url string, writer io.Writer) error {
	log.Printf("curl %s", url)
	// TODO: we should probably create only one Transport and reuse it for all calls
	t := &http.Transport{}
	t.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
	c := &http.Client{Transport: t}
	r, err := c.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if r.StatusCode >= 400 {
		return fmt.Errorf("%v returned %d", url, r.StatusCode)
	}
	_, err = io.Copy(writer, r.Body)
	if err != nil {
		return err
	}
	return nil
}

// return f(), adding junit xml testcase result for name
func xmlWrap(name string, f func() error) error {
	alreadyInterrupted := interrupted
	start := time.Now()
	err := f()
	duration := time.Since(start)
	c := testCase{
		Name:      name,
		ClassName: "e2e.go",
		Time:      duration.Seconds(),
	}
	if err == nil && !alreadyInterrupted && interrupted {
		err = fmt.Errorf("kubetest interrupted during step %s", name)
	}
	if err != nil {
		if !alreadyInterrupted {
			c.Failure = err.Error()
		} else {
			c.Skipped = err.Error()
		}
		suite.Failures++
	}

	suite.Cases = append(suite.Cases, c)
	suite.Tests++
	return err
}

// return cmd.Wait() and/or timing out.
func finishRunning(cmd *exec.Cmd) error {
	stepName := strings.Join(cmd.Args, " ")
	if terminated {
		return fmt.Errorf("kubetest terminated before starting %s", stepName)
	}
	if verbose {
		cmd.Stdout = os.Stdout
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

	go func() {
		finished <- cmd.Wait()
	}()

	for {
		select {
		case <-terminate.C:
			terminated = true
			terminate.Reset(time.Duration(0)) // Kill subsequent processes immediately.
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			cmd.Process.Kill()
			return fmt.Errorf("Terminate testing after 15m after %s timeout during %s", timeout, stepName)
		case <-interrupt.C:
			interrupted = true
			log.Printf("Interrupt testing after %s timeout. Will terminate in another 15m", timeout)
			terminate.Reset(15 * time.Minute)
			if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT); err != nil {
				log.Printf("Failed to interrupt %v. Will terminate immediately: %v", stepName, err)
				syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
				cmd.Process.Kill()
			}
		case err := <-finished:
			if err != nil {
				return fmt.Errorf("error during %s: %v", stepName, err)
			}
			return err
		}
	}
}

// return exec.Command(cmd, args...) while calling .StdinPipe().WriteString(input)
func inputCommand(input, cmd string, args ...string) (*exec.Cmd, error) {
	c := exec.Command(cmd, args...)
	w, e := c.StdinPipe()
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
	return c, nil
}

// return cmd.Output(), potentially timing out in the process.
func output(cmd *exec.Cmd) ([]byte, error) {
	stepName := strings.Join(cmd.Args, " ")
	if terminated {
		return []byte{}, fmt.Errorf("kubetest terminated before starting %s", stepName)
	}
	if verbose {
		cmd.Stderr = os.Stderr
	}
	log.Printf("Running: %v", stepName)
	defer func(start time.Time) {
		log.Printf("Step '%s' finished in %s", stepName, time.Since(start))
	}(time.Now())

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	type result struct {
		bytes []byte
		err   error
	}
	finished := make(chan result)
	lock := sync.Mutex{}
	started := false
	go func() {
		lock.Lock()
		if !terminated {
			started = true
		}
		lock.Unlock()
		if !started {
			return
		}
		b, err := cmd.Output()
		finished <- result{b, err}
	}()
	for {
		select {
		case <-terminate.C:
			terminate.Reset(time.Duration(0)) // Kill subsequent processes immediately.
			lock.Lock()
			if !started {
				terminated = true
			}
			lock.Unlock()
			if started {
				terminated = true
				for cmd.Process == nil {
					time.Sleep(50 * time.Millisecond)
				}
				syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				cmd.Process.Kill()
			}
			return nil, fmt.Errorf("Terminate testing after 15m after %s timeout during %s", timeout, stepName)
		case <-interrupt.C:
			interrupted = true
			log.Printf("Interrupt testing after %s timeout. Will terminate in another 15m", timeout)
			terminate.Reset(15 * time.Minute)
			for cmd.Process == nil {
				time.Sleep(50 * time.Millisecond)
			}
			if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT); err != nil {
				log.Printf("Failed to interrupt %v. Will terminate immediately: %v", stepName, err)
				syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
				cmd.Process.Kill()
			}
		case fin := <-finished:
			if fin.err != nil {
				return fin.bytes, fmt.Errorf("error during %s: %v", stepName, fin.err)
			}
			return fin.bytes, fin.err
		}
	}
}

// gs://foo and bar becomes gs://foo/bar
func joinUrl(urlPath, path string) (string, error) {
	u, err := url.Parse(urlPath)
	if err != nil {
		return "", err
	}
	u.Path = filepath.Join(u.Path, path)
	return u.String(), nil
}
