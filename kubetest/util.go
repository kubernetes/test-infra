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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	termLock    = new(sync.RWMutex)
	terminated  = false
	intLock     = new(sync.RWMutex)
	interrupted = false
)

func isTerminated() bool {
	termLock.RLock()
	t := terminated
	termLock.RUnlock()
	return t
}

func isInterrupted() bool {
	intLock.RLock()
	i := interrupted
	intLock.RUnlock()
	return i
}

var httpTransport *http.Transport

func init() {
	httpTransport = new(http.Transport)
	httpTransport.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
}

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
	c := &http.Client{Transport: httpTransport}
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
	alreadyInterrupted := isInterrupted()
	start := time.Now()
	err := f()
	duration := time.Since(start)
	c := testCase{
		Name:      name,
		ClassName: "e2e.go",
		Time:      duration.Seconds(),
	}
	if err == nil && !alreadyInterrupted && isInterrupted() {
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
	if isTerminated() {
		return fmt.Errorf("skipped %s (kubetest is terminated)", stepName)
	}
	if cmd.Stdout == nil && verbose {
		cmd.Stdout = os.Stdout
	}
	if cmd.Stderr == nil && verbose {
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
			if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
				log.Printf("Failed to kill %v: %v", stepName, err)
			}
		case <-terminate.C:
			termLock.Lock()
			terminated = true
			termLock.Unlock()
			terminate.Reset(time.Duration(0)) // Kill subsequent processes immediately.
			if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
				log.Printf("Failed to kill %v: %v", stepName, err)
			}
			if err := cmd.Process.Kill(); err != nil {
				log.Printf("Failed to terminate %s (terminated 15m after interrupt): %v", stepName, err)
			}
		case <-interrupt.C:
			intLock.Lock()
			interrupted = true
			intLock.Unlock()
			log.Printf("Interrupt after %s timeout during %s. Will terminate in another 15m", timeout, stepName)
			terminate.Reset(15 * time.Minute)
			if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT); err != nil {
				log.Printf("Failed to interrupt %s. Will terminate immediately: %v", stepName, err)
				syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
				cmd.Process.Kill()
			}
		case err := <-finished:
			if err != nil {
				var suffix string
				if isTerminated() {
					suffix = " (terminated)"
				} else if isInterrupted() {
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

// execute a given command and send output and error via channel
func executeParallelCommand(cmd *exec.Cmd, resChan chan cmdExecResult, termChan, intChan chan struct{}) {
	stepName := strings.Join(cmd.Args, " ")
	stdout := bytes.Buffer{}
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout

	start := time.Now()
	log.Printf("Running: %v in parallel", stepName)

	if isTerminated() {
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
				if isTerminated() {
					suffix = " (terminated)"
				} else if isInterrupted() {
					suffix = " (interrupted)"
				}
				err = fmt.Errorf("error during %s%s: %v", stepName, suffix, err)
			}
			resChan <- cmdExecResult{stepName: stepName, output: stdout.String(), execTime: time.Since(start), err: err}
			return

		case <-termChan:
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			if err := cmd.Process.Kill(); err != nil {
				log.Printf("Failed to terminate %s (terminated 15m after interrupt): %v", strings.Join(cmd.Args, " "), err)
			}

		case <-intChan:
			log.Printf("Interrupt after %s timeout during %s. Will terminate in another 15m", timeout, strings.Join(cmd.Args, " "))
			if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT); err != nil {
				log.Printf("Failed to interrupt %s. Will terminate immediately: %v", strings.Join(cmd.Args, " "), err)
				syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
				cmd.Process.Kill()
			}
		}
	}
}

// execute multiple commands in parallel
func finishRunningParallel(cmds ...*exec.Cmd) error {
	var wg sync.WaitGroup
	resultChan := make(chan cmdExecResult, len(cmds))
	termChan := make(chan struct{}, len(cmds))
	intChan := make(chan struct{}, len(cmds))

	for _, cmd := range cmds {
		wg.Add(1)
		go func(cmd *exec.Cmd) {
			defer wg.Done()
			executeParallelCommand(cmd, resultChan, termChan, intChan)
		}(cmd)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	cmdFailed := false
	for {
		select {
		case <-terminate.C:
			termLock.Lock()
			terminated = true
			termLock.Unlock()
			terminate.Reset(time.Duration(0))
			select {
			case <-termChan:
			default:
				close(termChan)
			}

		case <-interrupt.C:
			intLock.Lock()
			interrupted = true
			intLock.Unlock()
			terminate.Reset(15 * time.Minute)
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
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := finishRunning(cmd)
	return stdout.Bytes(), err

}

// gs://foo and bar becomes gs://foo/bar
func joinURL(urlPath, path string) (string, error) {
	u, err := url.Parse(urlPath)
	if err != nil {
		return "", err
	}
	u.Path = filepath.Join(u.Path, path)
	return u.String(), nil
}

// Chdir() to dir and return a function to cd back to Getwd()
func pushd(dir string) (func() error, error) {
	old, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to os.Getwd(): %v", err)
	}
	if err = os.Chdir(dir); err != nil {
		return nil, err
	}
	return func() error {
		return os.Chdir(old)
	}, nil
}

// Push env=value and return a function that resets env
func pushEnv(env, value string) (func() error, error) {
	prev, present := os.LookupEnv(env)
	if err := os.Setenv(env, value); err != nil {
		return nil, fmt.Errorf("could not set %s: %v", env, err)
	}
	return func() error {
		if present {
			return os.Setenv(env, prev)
		}
		return os.Unsetenv(env)
	}, nil
}

// Option that was an ENV that is now a --flag
type migratedOption struct {
	env      string  // env associated with --flag
	option   *string // Value of --flag
	name     string  // --flag name
	skipPush bool    // Push option to env if false
}

// Read value from ENV if --flag unset, optionally pushing to ENV
func migrateOptions(m []migratedOption) error {
	for _, s := range m {
		if *s.option == "" {
			// Jobs may not be using --foo instead of FOO just yet, so ease the transition
			// TODO(fejta): require --foo instead of FOO
			v := os.Getenv(s.env) // expected Getenv
			if v != "" {
				// Tell people to use --foo=blah instead of FOO=blah
				log.Printf("Please use kubetest %s=%s (instead of deprecated %s=%s)", s.name, v, s.env, v)
				*s.option = v
			}
		}
		if s.skipPush {
			continue
		}
		// Script called by kubetest may expect these values to be set, so set them
		// TODO(fejta): refactor the scripts below kubetest to use explicit config
		if *s.option == "" {
			continue
		}
		if err := os.Setenv(s.env, *s.option); err != nil {
			return fmt.Errorf("could not set %s=%s: %v", s.env, *s.option, err)
		}
	}
	return nil
}

func appendField(fields []string, flag, prefix string) []string {
	fields, cur, _ := extractField(fields, flag)
	if len(cur) == 0 {
		cur = prefix
	} else {
		cur = cur + "-" + prefix
	}
	return append(fields, flag+"="+cur)
}

func setFieldDefault(fields []string, flag, val string) []string {
	fields, cur, present := extractField(fields, flag)
	if !present {
		cur = val
	}
	return append(fields, flag+"="+cur)
}

// extractField("--a=this --b=that --c=other", "--b") returns [--a=this, --c=other"], "that"
func extractField(fields []string, target string) ([]string, string, bool) {
	f := []string{}
	prefix := target + "="
	consumeNext := false
	done := false
	r := ""
	for _, field := range fields {
		switch {
		case done:
			f = append(f, field)
		case consumeNext:
			r = field
			done = true
		case field == target:
			consumeNext = true
		case strings.HasPrefix(field, prefix):
			r = strings.SplitN(field, "=", 2)[1]
			done = true
		default:
			f = append(f, field)
		}
	}
	return f, r, done
}

// execError returns a string format of err including stderr if the
// err is an ExitError, useful for errors from e.g. exec.Cmd.Output().
func execError(err error) string {
	if ee, ok := err.(*exec.ExitError); ok {
		return fmt.Sprintf("%v (output: %q)", err, string(ee.Stderr))
	}
	return err.Error()
}

// ensureExecutable sets the executable file mode bits, for all users, to ensure that we can execute a file
func ensureExecutable(p string) error {
	s, err := os.Stat(p)
	if err != nil {
		return fmt.Errorf("error doing stat on %q: %v", p, err)
	}
	if err := os.Chmod(p, s.Mode()|0111); err != nil {
		return fmt.Errorf("error doing chmod on %q: %v", p, err)
	}
	return nil
}

type instanceGroup struct {
	Name              string `json:"name"`
	CreationTimestamp string `json:"creationTimestamp"`
}

// getLatestClusterUpTime returns latest created instanceGroup timestamp from gcloud parsing results
func getLatestClusterUpTime(gcloudJSON string) (time.Time, error) {
	igs := []instanceGroup{}
	if err := json.Unmarshal([]byte(gcloudJSON), &igs); err != nil {
		return time.Time{}, fmt.Errorf("error when unmarshal json: %v", err)
	}

	latest := time.Time{}

	for _, ig := range igs {
		created, err := time.Parse(time.RFC3339, ig.CreationTimestamp)
		if err != nil {
			return time.Time{}, fmt.Errorf("error when parse time from %s: %v", ig.CreationTimestamp, err)
		}

		if created.After(latest) {
			latest = created
		}
	}

	// this returns time.Time{} if no ig exists, which will always force a new cluster
	return latest, nil
}

// jsonForDebug returns a json representation of the value, or a string representation of an error
// It is useful for an easy implementation of fmt.Stringer
func jsonForDebug(o interface{}) string {
	if o == nil {
		return "nil"
	}
	v, err := json.Marshal(o)
	if err != nil {
		return fmt.Sprintf("error[%v]", err)
	}
	return string(v)
}

// flushMem will try to reduce the memory usage of the container it is running in
// run this after a build
func flushMem() {
	log.Println("Flushing memory.")
	// it's ok if these fail
	// flush memory buffers
	err := exec.Command("sync").Run()
	if err != nil {
		log.Printf("flushMem error (sync): %v", err)
	}
	// clear page cache
	err = exec.Command("bash", "-c", "echo 1 > /proc/sys/vm/drop_caches").Run()
	if err != nil {
		log.Printf("flushMem error (page cache): %v", err)
	}
}
