// Package junit describes the kubernetes/test-infra definition of "junit", and provides
// utilities to parse it.
package junit

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
)

// Suites holds a <testsuites/> list of Suite results
type Suites struct {
	XMLName xml.Name `xml:"testsuites"`
	Suites  []Suite  `xml:"testsuite"`
}

// Suite holds <testsuite/> results
type Suite struct {
	XMLName  xml.Name `xml:"testsuite"`
	Name     string   `xml:"name,attr"`
	Time     float64  `xml:"time,attr"` // Seconds
	Failures int      `xml:"failures,attr"`
	Tests    int      `xml:"tests,attr"`
	Results  []Result `xml:"testcase"`
	/*
	* <properties><property name="go.version" value="go1.8.3"/></properties>
	 */
}

// Result holds <testcase/> results
type Result struct {
	Name      string  `xml:"name,attr"`
	Time      float64 `xml:"time,attr"`
	ClassName string  `xml:"classname,attr"`
	Failure   *string `xml:"failure,omitempty"`
	Output    *string `xml:"system-out,omitempty"`
	Error     *string `xml:"system-err,omitempty"`
	Skipped   *string `xml:"skipped,omitempty"`
}

func unmarshalXML(buf []byte, i interface{}) error {
	reader := bytes.NewReader(buf)
	dec := xml.NewDecoder(reader)
	dec.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		switch charset {
		case "UTF-8", "utf8", "":
			// utf8 is not recognized by golang, but our coalesce.py writes a utf8 doc, which python accepts.
			return input, nil
		default:
			return nil, fmt.Errorf("unknown charset: %s", charset)
		}
	}
	return dec.Decode(i)
}

func Parse(buf []byte) (Suites, error) {
	var suites Suites
	// Try to parse it as a <testsuites/> object
	err := unmarshalXML(buf, &suites)
	if err != nil {
		// Maybe it is a <testsuite/> object instead
		suites.Suites = append([]Suite(nil), Suite{})
		ie := unmarshalXML(buf, &suites.Suites[0])
		if ie != nil {
			// Nope, it just doesn't parse
			return suites, fmt.Errorf("not valid testsuites: %v nor testsuite: %v", err, ie)
		}
	}
	return suites, nil
}
