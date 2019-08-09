/*
Copyright 2018 The Kubernetes Authors.

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

// Package takes coverage profile file as input and produces junit xml that is consumable by TestGrid

package junit

import (
	"encoding/xml"
	"fmt"

	"golang.org/x/tools/cover"

	"k8s.io/test-infra/gopherage/pkg/cov/junit/calculation"
)

// Property defines the xml element that stores the value of code coverage
type Property struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

// Properties defines the xml element that stores the a list of properties that associated with one testcase
type Properties struct {
	PropertyList []Property `xml:"property"`
}

// TestCase defines the xml element that stores all information associated with one test case
type TestCase struct {
	ClassName    string     `xml:"class_name,attr"`
	Name         string     `xml:"name,attr"`
	Time         string     `xml:"time,attr"`
	Failure      bool       `xml:"failure,omitempty"`
	PropertyList Properties `xml:"properties"`
}

// Testsuite defines the outer-most xml element that contains all test cases
type Testsuite struct {
	XMLName   string     `xml:"testsuite"`
	Testcases []TestCase `xml:"testcase"`
}

func (ts *Testsuite) addTestCase(coverageTargetName string, ratio, threshold float32) {
	testCase := TestCase{
		ClassName: "go_coverage",
		Name:      coverageTargetName,
		Time:      "0",
		Failure:   ratio < threshold,
		PropertyList: Properties{
			PropertyList: []Property{
				{
					Name:  "coverage",
					Value: fmt.Sprintf("%.1f", ratio*100),
				},
			},
		},
	}
	ts.Testcases = append(ts.Testcases, testCase)
}

// toTestsuite populates Testsuite struct with data from CoverageList and actual file
// directories from OS
func toTestsuite(covList *calculation.CoverageList, coverageThreshold float32) Testsuite {
	ts := Testsuite{}
	ts.addTestCase("OVERALL", covList.Ratio(), coverageThreshold)

	for _, cov := range covList.Group {
		ts.addTestCase(cov.Name, cov.Ratio(), coverageThreshold)
	}

	for _, dir := range covList.ListDirectories() {
		dirCov := covList.Subset(dir)
		ts.addTestCase(dir, dirCov.Ratio(), coverageThreshold)
	}
	return ts
}

// ProfileToTestsuiteXML uses coverage profile to produce junit xml
// which serves as the input for test coverage testgrid
func ProfileToTestsuiteXML(profiles []*cover.Profile, coverageThreshold float32) ([]byte, error) {
	covList := calculation.ProduceCovList(profiles)

	ts := toTestsuite(covList, coverageThreshold)
	return xml.MarshalIndent(ts, "", "    ")
}
