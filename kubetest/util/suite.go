package util

import (
	"encoding/xml"
)

type TestCase struct {
	XMLName   xml.Name `xml:"testcase"`
	ClassName string   `xml:"classname,attr"`
	Name      string   `xml:"name,attr"`
	Time      float64  `xml:"time,attr"`
	Failure   string   `xml:"failure,omitempty"`
	Skipped   string   `xml:"skipped,omitempty"`
}

type TestSuite struct {
	XMLName  xml.Name `xml:"testsuite"`
	Failures int      `xml:"failures,attr"`
	Tests    int      `xml:"tests,attr"`
	Time     float64  `xml:"time,attr"`
	Cases    []TestCase
}
