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

package resultstore

import (
	"fmt"
	"net/url"
	"time"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/golang/protobuf/ptypes/wrappers"
	resultstore "google.golang.org/genproto/googleapis/devtools/resultstore/v2"
)

// Invocation represents a flatted ResultStore invocation
type Invocation struct {
	// Name of the invocation, immutable after creation.
	Name string
	// Project in GCP that owns this invocation.
	Project string

	// Details describing the invocation.
	Details string
	// Duration of the invocation
	Duration time.Duration
	// Start time of the invocation
	Start time.Time

	// Files for this invocation (InvocationLog in particular)
	Files []File
	// Properties of the invocation, currently appears to be useless.
	Properties []Property

	// Status indicating whether the invocation completed successfully.
	Status Status
	// Description of the status.
	Description string
}

func URL(resourceName string) string {
	u := url.URL{
		Scheme: "https",
		Host:   "source.cloud.google.com",
		Path:   "results/" + resourceName,
	}
	return u.String()
}

func fromInvocation(rsi *resultstore.Invocation) Invocation {
	i := Invocation{
		Name:       rsi.Name,
		Files:      fromFiles(rsi.Files),
		Properties: fromProperties(rsi.Properties),
	}
	if ia := rsi.InvocationAttributes; ia != nil {
		i.Project = ia.ProjectId
		i.Description = ia.Description
	}
	if rsi.Timing != nil {
		i.Start, i.Duration = fromTiming(rsi.Timing)
	}
	i.Status, i.Description = fromStatus(rsi.StatusAttributes)
	return i
}

// To converts the invocation into a ResultStore Invoation proto.
func (i Invocation) To() *resultstore.Invocation {
	inv := resultstore.Invocation{
		Name:             i.Name,
		Timing:           timing(i.Start, i.Duration),
		StatusAttributes: status(i.Status, i.Description),
		Files:            Files(i.Files),
		Properties:       properties(i.Properties),
	}
	if i.Project != "" || i.Details != "" {
		inv.InvocationAttributes = &resultstore.InvocationAttributes{
			ProjectId:   i.Project,
			Description: i.Details,
		}
	}
	return &inv
}

// Timing

func dur(d time.Duration) *duration.Duration {
	return &duration.Duration{
		Seconds: int64(d / time.Second),
		Nanos:   int32(d % time.Second),
	}
}

func stamp(when time.Time) *timestamp.Timestamp {
	if when.IsZero() {
		return nil
	}
	return &timestamp.Timestamp{
		Seconds: when.Unix(),
		Nanos:   int32(when.UnixNano() % int64(time.Second)),
	}
}

func fromTiming(t *resultstore.Timing) (time.Time, time.Duration) {
	var when time.Time
	var dur time.Duration
	if t == nil {
		return when, dur
	}
	if s := t.StartTime; s != nil {
		when = time.Unix(s.Seconds, int64(s.Nanos))
	}
	if d := t.Duration; d != nil {
		dur = time.Duration(d.Seconds)*time.Second + time.Duration(d.Nanos)*time.Nanosecond
	}
	return when, dur
}

func timing(when time.Time, d time.Duration) *resultstore.Timing {
	never := when.IsZero()
	if never && d == 0 {
		return nil
	}
	rst := resultstore.Timing{}
	if !never {
		rst.StartTime = stamp(when)
	}
	if d > 0 {
		rst.Duration = dur(d)
	}
	return &rst
}

// TestFailure == Failure

// Failure describes the encountered problem.
type Failure struct {
	// Message is the failure message.
	Message string
	// Type is the type/type/class of error, currently appears useless.
	Type string
	// Stack represents the call stack, separated by new lines.
	Stack string
	// Expected represents what we expected, often just one value.
	Expected []string
	// Actual represents what we actually got.
	Actual []string
}

func fromFailures(tfs []*resultstore.TestFailure) []Failure {
	var ret []Failure
	for _, tf := range tfs {
		ret = append(ret, Failure{
			Message:  tf.FailureMessage,
			Type:     tf.ExceptionType,
			Stack:    tf.StackTrace,
			Expected: tf.Expected,
			Actual:   tf.Actual,
		})
	}
	return ret
}

// To converts the failure into a ResultStore TestFailure proto
func (f Failure) To() *resultstore.TestFailure {
	return &resultstore.TestFailure{
		FailureMessage: f.Message,
		ExceptionType:  f.Type,
		StackTrace:     f.Stack,
		Expected:       f.Expected,
		Actual:         f.Actual,
	}
}

func failures(fs []Failure) []*resultstore.TestFailure {
	var rstfs []*resultstore.TestFailure
	for _, f := range fs {
		rstfs = append(rstfs, f.To())
	}
	return rstfs
}

// TestError == Error

// Error describes what prevented completion.
type Error struct {
	// Message of the error
	Message string
	// Type of error, currently useless.
	Type string
	// Stack trace, separated by new lines.
	Stack string
}

// Error returns the corresponding ResultStore TestError message
func (e Error) To() *resultstore.TestError {
	return &resultstore.TestError{
		ErrorMessage:  e.Message,
		ExceptionType: e.Type,
		StackTrace:    e.Stack,
	}
}

func fromErrors(tes []*resultstore.TestError) []Error {
	var ret []Error
	for _, te := range tes {
		ret = append(ret, Error{
			Message: te.ErrorMessage,
			Type:    te.ExceptionType,
			Stack:   te.StackTrace,
		})
	}
	return ret
}

func errors(es []Error) []*resultstore.TestError {
	var rstes []*resultstore.TestError
	for _, e := range es {
		rstes = append(rstes, e.To())
	}
	return rstes
}

// Property

// Properties converts key, value pairs into a property list.
func Properties(pairs ...string) []Property {
	if len(pairs)%2 == 1 {
		panic(fmt.Sprintf("unbalanced properties: %v", pairs))
	}
	var out []Property
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, Property{Key: pairs[i], Value: pairs[i+1]})
	}
	return out
}

// Property represents a key-value pairing.
type Property = resultstore.Property

func properties(ps []Property) []*Property {
	var out []*Property
	for _, p := range ps {
		p2 := p
		out = append(out, &p2)
	}
	return out
}

func fromProperties(ps []*Property) []Property {
	var out []Property
	for _, p := range ps {
		out = append(out, *p)
	}
	return out
}

// File

// The following logs cause ResultStore to do additional processing
const (
	// BuildLog appears in the invocation log
	BuildLog = "build.log"

	// Stdout of a build action, which isn't useful right now.
	Stdout = "stdout"
	// Stderr of a build action, which also isn't useful.
	Stderr = "stderr"

	// TestLog appears in the Target Log tab.
	TestLog = "test.log"
	// TestXml causes ResultStore to process this junit.xml to add cases automatically (we aren't using).
	TestXml = "test.xml"

	// TestCov provides line coverage, currently we're not using this.
	TestCov = "test.lcov"
	// BaselineCov provides original line coverage, currently we're not using this.
	BaselineCov = "baseline.lcov"
)

// ResultStore will display the following logs inline.
const (
	// InvocationLog is a more obvious name for the invocation log
	InvocationLog = BuildLog
	// TargetLog is a more obvious name for the target log.
	TargetLog = TestLog
)

// File represents a file stored in GCS
type File struct {
	// Unique name within the set
	ID string

	// ContentType tells the browser how to render
	ContentType string
	// Length if complete and known
	Length int64
	// URL to file in Google Cloud Storage, such as gs://bucket/path/foo
	URL string
}

func wrap64(v int64) *wrappers.Int64Value {
	if v == 0 {
		return nil
	}
	return &wrappers.Int64Value{Value: v}
}

func unwrap64(w *wrappers.Int64Value) int64 {
	if w == nil {
		return 0
	}
	return w.Value
}

// To converts the file to the corresponding ResultStore File proto.
func (f File) To() *resultstore.File {
	return &resultstore.File{
		Uid:         f.ID,
		Uri:         f.URL,
		Length:      wrap64(f.Length),
		ContentType: f.ContentType,
	}
}

// Files converts a list of files.
func Files(fs []File) []*resultstore.File {
	var rsfs []*resultstore.File
	for _, f := range fs {
		rsfs = append(rsfs, f.To())
	}
	return rsfs
}

func fromFiles(fs []*resultstore.File) []File {
	var out []File
	for _, f := range fs {
		out = append(out, File{
			ID:          f.Uid,
			URL:         f.Uri,
			Length:      unwrap64(f.Length),
			ContentType: f.ContentType,
		})
	}
	return out
}

// Case == TestCase

// Result specifies whether the test passed.
type Result = resultstore.TestCase_Result

// Common constants.
const (
	// Completed cases finished, producing failures if it failed.
	Completed = resultstore.TestCase_COMPLETED
	// Cancelled cases did not complete (should have an error).
	Cancelled = resultstore.TestCase_CANCELLED
	// Skipped cases did not run.
	Skipped = resultstore.TestCase_SKIPPED
)

// Case represents the completion of a test case/method.
type Case struct {
	// Name identifies the test within its class.
	Name string

	// Class is the container holding one or more names.
	Class string
	// Result indicates whether it ran and to completion.
	Result Result

	// Duration of the case.
	Duration time.Duration
	// Errors preventing the case from completing.
	Errors []Error
	// Failures encountered upon completion.
	Failures []Failure
	// Files specific to this case
	Files []File
	// Properties of the case
	Properties []Property
	// Start time of the case.
	Start time.Time
}

func fromCase(tc *resultstore.TestCase) Case {
	c := Case{
		Name:       tc.CaseName,
		Class:      tc.ClassName,
		Result:     tc.Result,
		Properties: fromProperties(tc.Properties),
		Errors:     fromErrors(tc.Errors),
		Failures:   fromFailures(tc.Failures),
	}
	c.Start, c.Duration = fromTiming(tc.Timing)
	return c
}

// To converts the case to the corresponding ResultStore TestCase proto.
func (c Case) To() *resultstore.TestCase {
	return &resultstore.TestCase{
		CaseName:   c.Name,
		ClassName:  c.Class,
		Errors:     errors(c.Errors),
		Failures:   failures(c.Failures),
		Result:     c.Result,
		Timing:     timing(c.Start, c.Duration),
		Properties: properties(c.Properties),
	}
}

// TestAction == Test

// Status represents the status of the action/target/invocation.
type Status = resultstore.Status

// Common statuses
const (
	// Running means incomplete.
	Running = resultstore.Status_TESTING
	// Passed means successful.
	Passed = resultstore.Status_PASSED
	// Failed means unsuccessful.
	Failed = resultstore.Status_FAILED
)

// Test represents a test action, containing action, suite and warnings.
type Test struct {
	// Action holds generic metadata about the test
	Action
	// Suite holds a variety of case and sub-suite data.
	Suite
	// Warnings, appear to be useless.
	Warnings []string
}

// To converts the test into the corresponding ResultStore Action proto
func (t Test) To() *resultstore.Action {
	a := t.Action.to()
	a.ActionType = &resultstore.Action_TestAction{
		TestAction: &resultstore.TestAction{
			Warnings:  warnings(t.Warnings),
			TestSuite: t.Suite.To(),
		},
	}
	a.Files = Files(t.Files)
	a.Properties = properties(t.Properties)
	return a
}

func fromTestAction(ta *resultstore.TestAction) (Suite, []string) {
	if ta == nil {
		return Suite{}, nil
	}
	return fromSuite(ta.TestSuite), fromWarnings(ta.Warnings)
}

func fromTest(a *resultstore.Action) Test {
	t := Test{
		Action: fromAction(a),
	}
	t.Suite, t.Warnings = fromTestAction(a.GetTestAction())
	return t
}

// Action rerepresents a step in the target, such as a container or command.
type Action struct {
	// StatusAttributes
	// Description of the status.
	Description string
	// Status indicates whether the action completed successfully.
	Status Status

	// Timing
	// Start of the action.
	Start time.Time
	// Duration of the action.
	Duration time.Duration

	// Node or machine on which the test ran.
	Node string
	// ExitCode of the command
	ExitCode int

	// TODO(fejta): deps, coverage
}

func (act Action) to() *resultstore.Action {
	return &resultstore.Action{
		StatusAttributes: status(act.Status, act.Description),
		Timing:           timing(act.Start, act.Duration),
		ActionAttributes: actionAttributes(act.Node, act.ExitCode),
	}
}

func actionAttributes(node string, exit int) *resultstore.ActionAttributes {
	if node == "" && exit == 0 {
		return nil
	}
	return &resultstore.ActionAttributes{
		Hostname: node,
		ExitCode: int32(exit),
	}
}

func fromActionAttributes(aa *resultstore.ActionAttributes) (string, int) {
	if aa == nil {
		return "", 0
	}
	return aa.Hostname, int(aa.ExitCode)
}

func fromAction(a *resultstore.Action) Action {
	var ret Action
	ret.Status, ret.Description = fromStatus(a.StatusAttributes)
	ret.Start, ret.Duration = fromTiming(a.Timing)
	ret.Node, ret.ExitCode = fromActionAttributes(a.ActionAttributes)
	return ret
}

func status(s Status, d string) *resultstore.StatusAttributes {
	return &resultstore.StatusAttributes{
		Status:      s,
		Description: d,
	}
}

func fromStatus(sa *resultstore.StatusAttributes) (Status, string) {
	if sa == nil {
		return 0, ""
	}
	return sa.Status, sa.Description
}

func warnings(ws []string) []*resultstore.TestWarning {
	var rstws []*resultstore.TestWarning
	for _, w := range ws {
		rstws = append(rstws, &resultstore.TestWarning{WarningMessage: w})
	}
	return rstws
}

func fromWarnings(ws []*resultstore.TestWarning) []string {
	var ret []string
	for _, w := range ws {
		ret = append(ret, w.WarningMessage)
	}
	return ret
}

// TestSuite == Suite

// Suite represents testing details.
type Suite struct {
	// Name of the suite, such as the tested class.
	Name string

	// Cases holds details about each case in the suite.
	Cases []Case
	// Duration of the entire suite.
	Duration time.Duration
	// Errors that prevented the suite from completing.
	Errors []Error
	// Failures detected during the suite.
	Failures []Failure
	// Files outputted by the suite.
	Files []File
	// Properties of the suite.
	Properties []Property
	// Result determines whether the suite ran and finished.
	Result Result
	// Time the suite started
	Start time.Time
	// Suites hold details about child suites.
	Suites []Suite
}

func (s Suite) tests() []*resultstore.Test {
	var ts []*resultstore.Test
	for _, suite := range s.Suites {
		ts = append(ts, &resultstore.Test{
			TestType: &resultstore.Test_TestSuite{
				TestSuite: suite.To(),
			},
		})
	}
	for _, c := range s.Cases {
		ts = append(ts, &resultstore.Test{
			TestType: &resultstore.Test_TestCase{
				TestCase: c.To(),
			},
		})
	}
	return ts
}

func (s *Suite) fromTests(tests []*resultstore.Test) {
	for _, t := range tests {
		if tc := t.GetTestCase(); tc != nil {
			s.Cases = append(s.Cases, fromCase(tc))
		}
		if ts := t.GetTestSuite(); ts != nil {
			s.Suites = append(s.Suites, fromSuite(ts))
		}
	}
}

// To converts a suite into the corresponding ResultStore TestSuite proto.
func (s Suite) To() *resultstore.TestSuite {
	return &resultstore.TestSuite{
		Errors:     errors(s.Errors),
		Failures:   failures(s.Failures),
		Properties: properties(s.Properties),
		SuiteName:  s.Name,
		Tests:      s.tests(),
		Timing:     timing(s.Start, s.Duration),
		Files:      Files(s.Files),
	}
}

func fromSuite(ts *resultstore.TestSuite) Suite {
	s := Suite{
		Errors:     fromErrors(ts.Errors),
		Failures:   fromFailures(ts.Failures),
		Properties: fromProperties(ts.Properties),
		Name:       ts.SuiteName,
		Files:      fromFiles(ts.Files),
	}
	s.fromTests(ts.Tests)
	s.Start, s.Duration = fromTiming(ts.Timing)
	return s
}

// Target represents a set of commands run inside the same pod.
type Target struct {
	// Name of the target, immutable.
	Name string

	// Start time of the target.
	Start time.Time
	// Duration the target ran.
	Duration time.Duration

	// Status specifying whether the target completed successfully.
	Status Status
	// Description of the status
	Description string

	// Tags are metadata for the target (like github labels).
	Tags []string
}

func fromTarget(t *resultstore.Target) Target {
	tgt := Target{
		Name: t.Name,
	}
	if t.TargetAttributes != nil {
		copy(tgt.Tags, t.TargetAttributes.Tags)
	}
	tgt.Start, tgt.Duration = fromTiming(t.Timing)
	tgt.Status, tgt.Description = fromStatus(t.StatusAttributes)
	return tgt
}

// To converts a target into the corresponding ResultStore Target proto.
func (t Target) To() *resultstore.Target {
	tgt := resultstore.Target{
		Timing:           timing(t.Start, t.Duration),
		StatusAttributes: status(t.Status, t.Description),
		Visible:          true,
	}
	if t.Tags != nil {
		tgt.TargetAttributes = &resultstore.TargetAttributes{
			Tags: t.Tags,
		}
	}
	return &tgt
}
