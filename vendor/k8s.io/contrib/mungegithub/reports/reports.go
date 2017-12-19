/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package reports

import (
	"fmt"

	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

// Report is the interface which all reports must implement to register
type Report interface {
	// Take action on a specific github issue:
	Report(config *github.Config) error
	AddFlags(cmd *cobra.Command, config *github.Config)
	Name() string
}

var reportMap = map[string]Report{}
var reports = []Report{}

// GetAllReports returns a slice of all registered reports. This list is
// completely independant of the reports selected at runtime in --pr-reports.
// This is all possible reports.
func GetAllReports() []Report {
	out := []Report{}
	for _, report := range reportMap {
		out = append(out, report)
	}
	return out
}

// GetActiveReports returns a slice of all reports which both registered and
// were requested by the user
func GetActiveReports() []Report {
	return reports
}

// RegisterReport should be called in `init()` by each report to make itself
// available by name
func RegisterReport(report Report) error {
	if _, found := reportMap[report.Name()]; found {
		return fmt.Errorf("a report with that name (%s) already exists", report.Name())
	}
	reportMap[report.Name()] = report
	glog.Infof("Registered %#v at %s", report, report.Name())
	return nil
}

// RegisterReportOrDie will call RegisterReport but will be fatal on error
func RegisterReportOrDie(report Report) {
	if err := RegisterReport(report); err != nil {
		glog.Fatalf("Failed to register report: %s", err)
	}
}

// RunReports runs the specified reports.
func RunReports(cfg *github.Config, runReports ...string) error {
	for _, name := range runReports {
		report, ok := reportMap[name]
		if !ok {
			return fmt.Errorf("%v: not a valid report", name)
		}
		if err := report.Report(cfg); err != nil {
			return fmt.Errorf("Error running %v: %v", report.Name(), err)
		}
	}
	return nil
}
