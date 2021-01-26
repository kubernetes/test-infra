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

package util

import (
	"encoding/xml"
)

// TestCase holds the result of a test/step/command.
//
// This will become a row in testgrid.
type TestCase struct {
	XMLName   xml.Name `xml:"testcase"`
	ClassName string   `xml:"classname,attr"`
	Name      string   `xml:"name,attr"`
	Time      float64  `xml:"time,attr"`
	Failure   string   `xml:"failure,omitempty"`
	Skipped   string   `xml:"skipped,omitempty"`
}

// TestSuite holds a slice of TestCase and other summary metadata.
//
// A build (column in testgrid) is composed of one or more TestSuites.
type TestSuite struct {
	XMLName  xml.Name `xml:"testsuite"`
	Name     string   `xml:"name,arr"`
	Time     float64  `xml:"time,attr"` // seconds
	Failures int      `xml:"failures,attr"`
	Tests    int      `xml:"tests,attr"`
	Cases    []TestCase
}
