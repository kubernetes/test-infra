/*
Copyright 2016 The Kubernetes Authors.

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
	"encoding/json"
	"fmt"
	"net/http"

	"k8s.io/kubernetes/test/e2e/perftype"
)

const (
	// TestNameSeparator is the prefix of test name.
	TestNameSeparator = "[It] "
	// BenchmarkSeparator is the suffix of benchmark test name.
	BenchmarkSeparator = " [Benchmark]"

	// Test result tags
	perfResultTag = perftype.PerfResultTag
	perfResultEnd = perftype.PerfResultEnd
	// TODO(coufon): add the tags to perftype
	timeSeriesTag = "[Result:TimeSeries]"
	timeSeriesEnd = "[Finish:TimeSeries]"
)

// PerfData is performance test result (latency, resource-usage).
// Usually there is an array of PerfData containing different data type.
type PerfData perftype.DataItem

// SeriesData is time series data, including operation and resourage time series.
// TODO(coufon): now we only record an array of timestamps when an operation (probe)
// is called. In future we can record probe name, pod UID together with the timestamp.
// TODO(coufon): rename 'operation' to 'probe'?
type SeriesData struct {
	OperationSeries map[string][]int64        `json:"op_series,omitempty"`
	ResourceSeries  map[string]ResourceSeries `json:"resource_series,omitempty"`
}

// TestData wraps up PerfData and SeriesData to simplify parser logic.
// TODO(coufon): use better json tags? need to change test code as well.
type TestData struct {
	Version       string            `json:"version"`
	Labels        map[string]string `json:"labels,omitempty"`
	PerfDataItems []PerfData        `json:"dataItems,omitempty"`
	SeriesData
}

// ResourceSeries contains time series data of CPU/memory usage.
type ResourceSeries struct {
	Timestamp            []int64           `json:"ts"`
	CPUUsageInMilliCores []int64           `json:"cpu"`
	MemoryRSSInMegaBytes []int64           `json:"memory"`
	Units                map[string]string `json:"unit"`
}

// DataPerBuild contains perf/time series data for a build.
type DataPerBuild struct {
	Perf   []PerfData   `json:"perf,omitempty"`
	Series []SeriesData `json:"series,omitempty"`
}

// AppendPerfData appends new perf data.
func (db *DataPerBuild) AppendPerfData(obj TestData) {
	db.Perf = append(db.Perf, obj.PerfDataItems...)
}

// AppendSeriesData appends new time series data.
func (db *DataPerBuild) AppendSeriesData(obj TestData) {
	db.Series = append(db.Series, obj.SeriesData)
}

// DataPerNode contains perf/time series data for a node.
type DataPerNode map[string]*DataPerBuild

// DataPerTest contains job name and a map (build to perf data).
type DataPerTest struct {
	Data    map[string]DataPerNode `json:"data"`
	Job     string                 `json:"job"`
	Version string                 `json:"version"`
}

// TestToBuildData is a map from test name to BuildData
type TestToBuildData map[string]*DataPerTest

func (b *TestToBuildData) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	data, err := json.Marshal(b)
	if err != nil {
		res.Header().Set("Content-type", "text/html")
		res.WriteHeader(http.StatusInternalServerError)
		res.Write([]byte(fmt.Sprintf("<h3>Internal Error</h3><p>%v", err)))
		return
	}
	res.Header().Set("Content-type", "application/json")
	res.WriteHeader(http.StatusOK)
	res.Write(data)
}

// TestInfoMap contains all testInfo indexed by short test name
type TestInfoMap struct {
	Info map[string]string `json:"info"`
}

var testInfoMap = TestInfoMap{
	Info: make(map[string]string),
}

func (b *TestInfoMap) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	data, err := json.Marshal(b)
	if err != nil {
		res.Header().Set("Content-type", "text/html")
		res.WriteHeader(http.StatusInternalServerError)
		res.Write([]byte(fmt.Sprintf("<h3>Internal Error</h3><p>%v", err)))
		return
	}
	res.Header().Set("Content-type", "application/json")
	res.WriteHeader(http.StatusOK)
	res.Write(data)
}
