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
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type testCase struct {
	XMLName   xml.Name `xml:"testcase"`
	ClassName string   `xml:"classname,attr"`
	Name      string   `xml:"name,attr"`
	Time      float64  `xml:"time,attr"`
	Failure   string   `xml:"failure,omitempty"`
}

type testSuite struct {
	XMLName  xml.Name `xml:"testsuite"`
	Failures int      `xml:"failures,attr"`
	Tests    int      `xml:"tests,attr"`
	Time     float64  `xml:"time,attr"`
	Cases    []testCase
}

var suite testSuite

func writeXML(start time.Time) {
	suite.Time = time.Since(start).Seconds()
	out, err := xml.MarshalIndent(&suite, "", "    ")
	if err != nil {
		log.Fatalf("Could not marshal XML: %s", err)
	}
	path := filepath.Join(*dump, "junit_runner.xml")
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

// return f(), adding junit xml testcase result for name
func xmlWrap(name string, f func() error) error {
	start := time.Now()
	err := f()
	duration := time.Since(start)
	c := testCase{
		Name:      name,
		ClassName: "e2e.go",
		Time:      duration.Seconds(),
	}
	if err != nil {
		c.Failure = err.Error()
		suite.Failures++
	}
	suite.Cases = append(suite.Cases, c)
	suite.Tests++
	return err
}

// return cmd.Wait() and/or timing out.
func finishRunning(cmd *exec.Cmd) error {
	stepName := strings.Join(cmd.Args, " ")
	if *verbose {
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
			terminate.Reset(time.Duration(0)) // Kill subsequent processes immediately.
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			cmd.Process.Kill()
			return fmt.Errorf("Terminate testing after 15m after %s timeout during %s", timeout, stepName)
		case <-interrupt.C:
			log.Printf("Interrupt testing after %s timeout. Will terminate in another 15m", timeout)
			terminate.Reset(15 * time.Minute)
			if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT); err != nil {
				log.Printf("Failed to interrupt %v. Will terminate immediately: %v", stepName, err)
				syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
				cmd.Process.Kill()
			}
		case err := <-finished:
			return err
		}
	}
}
