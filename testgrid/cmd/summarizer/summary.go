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

package summarizer

import (
	configpb "k8s.io/test-infra/testgrid/config"
	summarypb "k8s.io/test-infra/testgrid/summary"
)

// Response has all the fields expected by TestGrid's javascript client.
// It represents a grid of test results, with additional annotations for metadata.
type Response struct {
	TestGroupName string      `json:"test-group-name"`
	QueryParam    string      `json:"query"`
	Status        string      `json:"status"`
	PhaseTimer    interface{} `json:"phase-timer"`
	Cached        bool        `json:"cached"` // whether this was loaded from a cache
	Summary       string      `json:"summary"`

	Bugs              map[string]string `json:"bugs"`
	BuildIds          []string          `json:"build-ids"`
	ColumnIDs         []string          `json:"column_ids"`
	CustomColumns     [][]string        `json:"custom-columns"`
	ColumnHeaderNames []string          `json:"column-header-names"`
	Groups            []string          `json:"groups"`
	Metrics           []string          `json:"metrics"`
	Tests             []*Row            `json:"tests"`
	RowIDs            []*string         `json:"row_ids"` // client wants nullable strings here
	Timestamps        []int             `json:"timestamps"`

	// Lookup map for shortening long test IDs in each row.
	TestIDMap map[int]string `json:"test_id_map"`

	TestMetadata map[string]testMetadata `json:"test-metadata"`

	StaleTestThreshold int32  `json:"stale-test-threshold"`
	NumStaleTests      int    `json:"num-stale-tests"`
	Alerts             string `json:"alerts,omitempty"`

	AddTabularNamesOption     bool     `json:"add-tabular-names-option"`
	ShowTabularNames          bool     `json:"show-tabular-names"`
	TabularNamesColumnHeaders []string `json:"tabular-names-column-headers,omitempty"`

	Description           string       `json:"description"`
	BugComponent          int32        `json:"bug-component"`
	CodeSearchPath        string       `json:"code-search-path"`
	OpenTestTemplate      linkTemplate `json:"open-test-template"`
	FileBugTemplate       linkTemplate `json:"file-bug-template"`
	AttachBugTemplate     linkTemplate `json:"attach-bug-template"`
	ResultsURLTemplate    linkTemplate `json:"results-url-template"`
	CodeSearchURLTemplate linkTemplate `json:"code-search-url-template"`
	AboutDashboardURL     string       `json:"about-dashboard-url"`
	OpenBugTemplate       linkTemplate `json:"open-bug-template"`

	ResultsText string `json:"results-text"`
	LatestGreen string `json:"latest-green"`

	TriageEnabled bool           `json:"triage-enabled"`
	Notifications []notification `json:"notifications"`

	TestGroup     *configpb.TestGroup                     `json:"-"`
	DashboardTab  *configpb.DashboardTab                  `json:"-"`
	OverallStatus summarypb.DashboardTabSummary_TabStatus `json:"overall-status"`
}

type linkTemplate struct {
	URL     string            `json:"url"`
	Options map[string]string `json:"options"`
}

type testMetadata struct {
	BugComponent int32    `json:"bug-component"`
	Owner        string   `json:"owner,omitempty"`
	Cc           []string `json:"cc,omitempty"`
}

type notification struct {
	Summary     string `json:"summary"`
	ContextLink string `json:"context_link,omitempty"`
}

// Row describes a test row.
type Row struct {
	Name              string      `json:"name"`
	OriginalName      string      `json:"original-name"`
	Alert             *TestAlert  `json:"alert"`
	LinkedBugs        []string    `json:"linked_bugs"`
	Messages          []string    `json:"messages"`
	ShortTexts        []string    `json:"short_texts"`
	TestIDs           []string    `json:"test_ids,omitempty"`
	ShortTestIDs      []int       `json:"short_test_ids,omitempty"`
	Statuses          []rleStatus `json:"statuses"`
	Target            string      `json:"target"`
	Tests             []*Row      `json:"tests,omitempty"`
	TabularNameGroups []string    `json:"tabular-name-groups,omitempty"`
	MetricInfo        []rawMetric `json:"-"`
	Graphs            []*graph    `json:"graphs,omitempty"`
}

// TestAlert describes a consistently-failing test.
type TestAlert struct {
	FailBuildId    string `json:"fail-build-id"`
	FailCount      int    `json:"fail-count"`
	FailTime       int    `json:"fail-time"`
	Text           string `json:"text"`
	Message        string `json:"message"`
	LinkText       string `json:"link-text"`
	Link           string `json:"link"`
	URLText        string `json:"url-text"`
	TestID         string `json:"test-id"`
	PassBuildId    string `json:"pass-build-id"`
	PassCount      int    `json:"pass-count"`
	PassTime       int    `json:"pass-time"`
	CodeSearchPath string `json:"code-search-path"`
	TestName       string `json:"test-name"`
}

// rleStatus represents a run-length encoded test status:
// a run of Count cells with status Value.
type rleStatus struct {
	Count int32 `json:"count"`
	Value int32 `json:"value"`
}

// rawMetric holds a metric and its values for each test cycle.
// This is the compressed form from the protobuf.
type rawMetric struct {
	ID string // Name.

	// A sparse encoding of densely stored values. The layout encodes the cycle
	// indices of the values in the value field below. The layout contains indices
	// followed by counts. The indices specify test cycles where contiguous
	// sequences of values start. See graph_test.go for example.
	Layout []int32

	// Non-empty values for each test result.
	Value []float64
}

type graph struct {
	metric string
	names  []string
	Labels []string     `json:"metric"`
	Values [][]*float64 `json:"values"`
}
