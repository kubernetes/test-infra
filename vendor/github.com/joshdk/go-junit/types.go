package junit

import (
	"time"
)

type status string

const (
	// StatusPassed represents a JUnit testcase that was run, and did not
	// result in an error or a failure.
	StatusPassed status = "passed"

	// StatusSkipped represents a JUnit testcase that was intentionally
	// skipped.
	StatusSkipped status = "skipped"

	// StatusFailed represents a JUnit testcase that was run, but resulted in
	// a failure. Failures are violations of declared test expectations,
	// such as a failed assertion.
	StatusFailed status = "failed"

	// StatusError represents a JUnit testcase that was run, but resulted in
	// an error. Errors are unexpected violations of the test itself, such as
	// an uncaught exception.
	StatusError status = "error"
)

// Totals contains aggregated results across a set of test runs. Is usually
// calculated as a sum of all given test runs, and overrides whatever was given
// at the suite level.
//
// The following relation should hold true.
//   Tests == (Passed + Skipped + Failed + Error)
type Totals struct {
	// Tests is the total number of tests run.
	Tests int `json:"tests" yaml:"tests"`

	// Passed is the total number of tests that passed successfully.
	Passed int `json:"passed" yaml:"passed"`

	// Skipped is the total number of tests that were skipped.
	Skipped int `json:"skipped" yaml:"skipped"`

	// Failed is the total number of tests that resulted in a failure.
	Failed int `json:"failed" yaml:"failed"`

	// Error is the total number of tests that resulted in an error.
	Error int `json:"error" yaml:"error"`

	// Duration is the total time taken to run all tests.
	Duration time.Duration `json:"duration" yaml:"duration"`
}

// Suite represents a logical grouping (suite) of tests.
type Suite struct {
	// Name is a descriptor given to the suite.
	Name string `json:"name" yaml:"name"`

	// Package is an additional descriptor for the hierarchy of the suite.
	Package string `json:"package" yaml:"package"`

	// Properties is a mapping of key-value pairs that were available when the
	// tests were run.
	Properties map[string]string `json:"properties,omitempty" yaml:"properties"`

	// Tests is an ordered collection of tests with associated results.
	Tests []Test `json:"tests,omitempty" yaml:"tests"`

	// SystemOut is textual test output for the suite. Usually output that is
	// written to stdout.
	SystemOut string `json:"stdout,omitempty"`

	// SystemErr is textual test error output for the suite. Usually output that is
	// written to stderr.
	SystemErr string `json:"stderr,omitempty"`

	// Totals is the aggregated results of all tests.
	Totals Totals `json:"totals" yaml:"totals"`
}

// Aggregate calculates result sums across all tests.
func (s *Suite) Aggregate() {
	totals := Totals{Tests: len(s.Tests)}

	for _, test := range s.Tests {
		totals.Duration += test.Duration
		switch test.Status {
		case StatusPassed:
			totals.Passed++
		case StatusSkipped:
			totals.Skipped++
		case StatusFailed:
			totals.Failed++
		case StatusError:
			totals.Error++
		}
	}

	s.Totals = totals
}

// Test represents the results of a single test run.
type Test struct {
	// Name is a descriptor given to the test.
	Name string `json:"name" yaml:"name"`

	// Classname is an additional descriptor for the hierarchy of the test.
	Classname string `json:"classname" yaml:"classname"`

	// Duration is the total time taken to run the tests.
	Duration time.Duration `json:"duration" yaml:"duration"`

	// Status is the result of the test. Status values are passed, skipped,
	// failure, & error.
	Status status

	// Error is a record of the failure or error of a test, if applicable.
	//
	// The following relations should hold true.
	//   Error == nil && (Status == Passed || Status == Skipped)
	//   Error != nil && (Status == Failed || Status == Error)
	Error error
}

// Error represents an erroneous test result.
type Error struct {
	// Message is a descriptor given to the error. Purpose and values differ by
	// environment.
	Message string `json:"message,omitempty" yaml:"message"`

	// Type is a descriptor given to the error. Purpose and values differ by
	// framework. Value is typically an exception class, such as an assertion.
	Type string `json:"type,omitempty" yaml:"type"`

	// Body is extended text for the error. Purpose and values differ by
	// framework. Value is typically a stacktrace.
	Body string `json:"body,omitempty" yaml:"body"`
}

// Error returns a textual description of the test error.
func (err Error) Error() string {
	return err.Body
}
