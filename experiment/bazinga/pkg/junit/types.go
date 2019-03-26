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

package junit

import (
	"encoding/xml"
	"io"
)

const xmlHeader = `<?xml version="1.0" encoding="UTF-8"?>`

// Document represents a JUnit test suites file.
type Document struct {
	XMLName    xml.Name `xml:"testsuites"`
	TestSuites []TestSuite
}

// TestSuite represents a JUnit test suite.
type TestSuite struct {
	XMLName   xml.Name `xml:"testsuite"`
	Failures  int      `xml:"failures,attr"`
	Tests     int      `xml:"tests,attr"`
	Time      float64  `xml:"time,attr"`
	TestCases []TestCase
}

// TestCase represents a JUnit test case.
type TestCase struct {
	XMLName  xml.Name `xml:"testcase"`
	Name     string   `xml:"name,attr"`
	Time     float64  `xml:"time,attr"`
	Failures []Failure
}

// Failure represents a JUnit test case failure.
type Failure struct {
	XMLName xml.Name `xml:"failure"`
	Message string   `xml:"message,attr"`
	Type    string   `xml:"type,attr"`
	Text    []byte   `xml:",cdata"`
}

// Write writes the document to the given writer.
func (d *Document) Write(w io.Writer) error {
	d.Sync()
	if _, err := w.Write([]byte(xmlHeader)); err != nil {
		return err
	}
	if _, err := w.Write([]byte{'\n'}); err != nil {
		return err
	}
	e := xml.NewEncoder(w)
	e.Indent("", "  ")
	if err := e.Encode(d); err != nil {
		return err
	}
	if _, err := w.Write([]byte{'\n'}); err != nil {
		return err
	}
	return nil
}

// Sync updates the doc's test suites' totals based on the suites' test cases.
func (d *Document) Sync() {
	for i := range d.TestSuites {
		d.TestSuites[i].Sync()
	}
}

// Sync updates the test suite's totals based on the suite's test cases.
func (t *TestSuite) Sync() {
	t.Time = 0
	t.Tests = 0
	t.Failures = 0
	for _, tc := range t.TestCases {
		t.Failures = t.Failures + len(tc.Failures)
		t.Tests++
		t.Time = t.Time + tc.Time
	}
}
