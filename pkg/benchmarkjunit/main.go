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

package main

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os/exec"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type options struct {
	outputFile    string
	logFile       string
	benchRegexp   string
	extraTestArgs []string
	goBinaryPath  string
}

func main() {
	opts := &options{}
	cmd := &cobra.Command{
		Use:   "benchmarkjunit <packages>",
		Short: "Runs go benchmarks and outputs junit xml.",
		Long:  `Runs "go test -v -run='^$' -bench=. <packages>" and translates the output into JUnit XML.`,
		Run: func(cmd *cobra.Command, args []string) {
			run(opts, args)
		},
	}
	cmd.Flags().StringVarP(&opts.outputFile, "output", "o", "-", "output file")
	cmd.Flags().StringVarP(&opts.logFile, "log-file", "l", "", "optional output file for complete go test output")
	cmd.Flags().StringVar(&opts.benchRegexp, "bench", ".", "The regexp to pass to the -bench 'go test' flag to select benchmarks to run.")
	cmd.Flags().StringSliceVar(&opts.extraTestArgs, "test-arg", nil, "additional args for go test")
	cmd.Flags().StringVar(&opts.goBinaryPath, "go", "go", "The location of the go binary. This flag is primarily intended for use with bazel.")

	if err := cmd.Execute(); err != nil {
		logrus.WithError(err).Fatal("Command failed.")
	}
}

func run(opts *options, args []string) {
	testArgs := []string{
		"test", "-v", "-run='^$'", "-bench=" + opts.benchRegexp,
	}
	testArgs = append(testArgs, opts.extraTestArgs...)
	testArgs = append(testArgs, args...)
	testCmd := exec.Command(opts.goBinaryPath, testArgs...)

	logrus.Infof("Running command %q...", append([]string{opts.goBinaryPath}, testArgs...))
	testOutput, err := testCmd.CombinedOutput()
	if err != nil {
		logrus.WithError(err).Error("Error(s) executing benchmarks.")
	}
	if len(opts.logFile) > 0 {
		if err := ioutil.WriteFile(opts.logFile, testOutput, 0666); err != nil {
			logrus.WithError(err).Fatalf("Failed to write to log file %q.", opts.logFile)
		}
	}
	logrus.Info("Benchmarks completed. Generating JUnit XML...")

	// Now parse output to JUnit, marshal to XML, and output.
	junit, err := parse(testOutput)
	if err != nil {
		fmt.Printf("Error parsing go test output: %v.\nOutput:\n%s\n\n", err, string(testOutput))
		logrus.WithError(err).Fatal("Error parsing 'go test' output.")
	}
	if len(junit.Suites) == 0 {
		logrus.Warn("Warning: no test suites were found in the 'go test' output.")
	}
	junitBytes, err := xml.Marshal(junit)
	if err != nil {
		logrus.WithError(err).Fatal("Error marshaling parsed 'go test' output to XML.")
	}
	if opts.outputFile == "-" {
		fmt.Println(string(junitBytes))
	} else {
		if err := ioutil.WriteFile(opts.outputFile, junitBytes, 0666); err != nil {
			logrus.WithError(err).Fatalf("Failed to write JUnit to output file %q.", opts.outputFile)
		}
	}
	logrus.Info("Successfully generated JUnit XML for Benchmarks.")
}
