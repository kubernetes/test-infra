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

package config

import (
	"regexp/syntax"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// App is the bazinga application configuration.
type App struct {
	// TypeMeta representing the type of the object and its API schema version.
	metav1.TypeMeta `json:",inline"`

	// Output is the path to which the test suite results will be written.
	//
	// Relative paths will be resolved with respect to the working directory.
	//
	// If Output is an empty string then the result file is written in the
	// working directory to a file named "junit_1.xml".
	Output string `json:"output,omitempty"`

	// SendSignals is a flag that indicates whether or not to relay the
	// parent process's signals to the test case processes.
	SendSignals bool `json:"sendSignals,omitempty"`

	// FailureConditions is a list of conditions by which a test is considered
	// in error.
	FailureConditions []FailureCondition `json:"failureConditions,omitempty"`

	// TestSuites are the test suites to execute.
	TestSuites []TestSuite `json:"testSuites,omitempty"`
}

// TestSuite contains a bazinga test suite configuration.
type TestSuite struct {
	// Name is the name of the test suite.
	Name string `json:"name,omitempty"`

	// FailureConditions is a list of conditions by which a test is considered
	// in error.
	FailureConditions []FailureCondition `json:"failureConditions,omitempty"`

	// TestCases is the list of this test suite's test cases.
	TestCases []TestCase `json:"testCases,omitempty"`
}

// TestCase contains settings for a a test case in the bazinga test suite.
type TestCase struct {
	// Class is the name of the class associated with the test case.
	Class string `json:"class,omitempty"`

	// Name is the name of the test case.
	Name string `json:"name,omitempty"`

	// Command is the command to execute. The path to Command follows the same rules as an os.Cmd.
	Command string `json:"command,omitempty"`

	// Args is the list of arguments provided to Command.
	Args []string `json:"args,omitempty"`

	// Env is the environment to provide to the Command. If omitted then
	// the process's environment is used, otherwise this value is appended
	// to the parent process's environment.
	Env []string `json:"env,omitempty"`

	// EnvClean is a flag that indicates whether or not to execute the
	// Command with a clean environment. If true only Env will be used,
	// and the parent process's environment is ignored.
	EnvClean bool `json:"envClean,omitempty"`

	// FailureConditions is a list of conditions by which a test is considered
	// in error.
	FailureConditions []FailureCondition `json:"failureConditions,omitempty"`

	// SuccessExitCodes is a list of exit codes considered successful. If
	// omitted then a zero exit code is successful and all others are considered
	// in error.
	SuccessExitCodes []int
}

// FailureCondition is a condition by which a test is considered in error.
type FailureCondition struct {
	// Category is the failure category.
	Category string `json:"category,omitempty"`

	// Message is an optional message to associate with the failure condition.
	Message string `json:"message,omitempty"`

	// Pattern is the regular expression pattern against which lines from the
	// test case's output are matched to determine if the test is in error.
	Pattern string `json:"pattern,omitempty"`

	// Flags are flags used with the Pattern regexp.
	Flags syntax.Flags `json:"flags"`
}
