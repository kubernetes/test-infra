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

package sources

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	githubapi "github.com/google/go-github/github"
	"k8s.io/test-infra/robots/issue-creator/creator"
)

const (
	timeFormat = "2 Jan 2006 15:04 MST"

	// Configuration constants.
	topJobsCount   = 3
	topTestsCount  = 3
	triageURL      = "https://go.k8s.io/triage"
	clusterDataURL = "https://storage.googleapis.com/k8s-gubernator/triage/failure_data.json"
)

// TriageFiler files issues for clustered test failures.
type TriageFiler struct {
	topClustersCount int
	windowDays       int

	nextSync    time.Time
	latestStart int64

	creator *creator.IssueCreator
	data    *triageData
}

func init() {
	creator.RegisterSourceOrDie("triage-filer", &TriageFiler{})
}

// Issues is the main work function of the TriageFiler.  It fetches and parses cluster data,
// then syncs the top issues to github with the IssueCreator.
func (f *TriageFiler) Issues(c *creator.IssueCreator) ([]creator.Issue, error) {
	f.creator = c
	rawjson, err := ReadHTTP(clusterDataURL)
	if err != nil {
		return nil, err
	}
	clusters, err := f.loadClusters(rawjson)
	if err != nil {
		return nil, err
	}
	topclusters := topClusters(clusters, f.topClustersCount)
	issues := make([]creator.Issue, 0, len(topclusters))
	for _, clust := range topclusters {
		issues = append(issues, clust)
	}
	return issues, nil
}

// RegisterFlags registers options for this munger; returns any that require a restart when changed.
func (f *TriageFiler) RegisterFlags() {
	flag.IntVar(&f.topClustersCount, "triage-count", 3, "The number of clusters to sync issues for on github.")
	flag.IntVar(&f.windowDays, "triage-window", 1, "The size of the sliding time window (in days) that is used to determine which failures to consider.")
}

// triageData is a struct that represents the format of the JSON triage data and is used for parsing.
type triageData struct {
	Builds struct {
		Cols struct {
			Elapsed     []int    `json:"elapsed"`
			Executor    []string `json:"executor"`
			PR          []string `json:"pr"`
			Result      []string `json:"result"`
			Started     []int64  `json:"started"`
			TestsFailed []int    `json:"tests_failed"`
			TestsRun    []int    `json:"tests_run"`
		} `json:"cols"`
		JobsRaw  map[string]interface{} `json:"jobs"` // []int or map[string]int
		Jobs     map[string]BuildIndexer
		JobPaths map[string]string `json:"job_paths"`
	} `json:"builds"`
	Clustered []*Cluster `json:"clustered"`
}

// Cluster holds information about a failure cluster.
type Cluster struct {
	Identifier string  `json:"id"`
	Key        string  `json:"key"`
	Text       string  `json:"text"`
	Tests      []*Test `json:"tests"`

	filer       *TriageFiler
	jobs        map[string][]int
	totalBuilds int
	totalJobs   int
	totalTests  int
}

// Test holds a name and list of jobs
type Test struct {
	Name string `json:"name"`
	Jobs []*Job `json:"jobs"`
}

// Job holds a name and list of build numbers
type Job struct {
	Name   string `json:"name"`
	Builds []int  `json:"builds"`
}

// filterAndValidate removes failure data that falls outside the time window and ensures that cluster
// data is well formed. It also removes data for PR jobs so that only post-submit failures are considered.
func (f *TriageFiler) filterAndValidate(windowDays int) error {
	f.latestStart = int64(0)
	for _, start := range f.data.Builds.Cols.Started {
		if start > f.latestStart {
			f.latestStart = start
		}
	}
	cutoffTime := time.Unix(f.latestStart, 0).AddDate(0, 0, -windowDays).Unix()

	validClusts := []*Cluster{}
	for clustIndex, clust := range f.data.Clustered {
		if len(clust.Identifier) == 0 {
			return fmt.Errorf("the cluster at index %d in the triage JSON data does not specify an ID", clustIndex)
		}
		if clust.Tests == nil {
			return fmt.Errorf("cluster '%s' does not have a 'tests' key", clust.Identifier)
		}
		validTests := []*Test{}
		for _, test := range clust.Tests {
			if len(test.Name) == 0 {
				return fmt.Errorf("cluster '%s' contains a test without a name", clust.Identifier)
			}
			if test.Jobs == nil {
				return fmt.Errorf("cluster '%s' does not have a 'jobs' key", clust.Identifier)
			}
			validJobs := []*Job{}
			for _, job := range test.Jobs {
				if len(job.Name) == 0 {
					return fmt.Errorf("cluster '%s' contains a job without a name under test '%s'", clust.Identifier, test.Name)
				}
				// Filter out PR jobs
				if strings.HasPrefix(job.Name, "pr:") {
					continue
				}
				if len(job.Builds) == 0 {
					return fmt.Errorf("cluster '%s' contains job '%s' under test '%s' with no failing builds", clust.Identifier, job.Name, test.Name)
				}
				validBuilds := []int{}
				rowMap, ok := f.data.Builds.Jobs[job.Name]
				if !ok {
					return fmt.Errorf("triage json data does not contain buildnum to row index mapping for job '%s'", job.Name)
				}
				for _, buildnum := range job.Builds {
					row, err := rowMap.rowForBuild(buildnum)
					if err != nil {
						return err
					}
					if f.data.Builds.Cols.Started[row] > cutoffTime {
						validBuilds = append(validBuilds, buildnum)
					}
				}
				if len(validBuilds) > 0 {
					job.Builds = validBuilds
					validJobs = append(validJobs, job)
				}
			}
			if len(validJobs) > 0 {
				test.Jobs = validJobs
				validTests = append(validTests, test)
			}
		}
		if len(validTests) > 0 {
			clust.Tests = validTests
			validClusts = append(validClusts, clust)
		}
	}
	f.data.Clustered = validClusts
	return nil
}

// BuildIndexer is an interface that describes the buildnum to row index mapping used to retrieve data
// about individual builds from the JSON file.
// This is an interface because the JSON format describing failure clusters has 2 ways of recording the mapping info.
type BuildIndexer interface {
	rowForBuild(buildnum int) (int, error)
}

// ContigIndexer is a BuildIndexer implementation for when the buildnum to row index mapping describes
// a contiguous set of rows via 3 ints.
type ContigIndexer struct {
	startRow, startBuild, count int
}

func (rowMap ContigIndexer) rowForBuild(buildnum int) (int, error) {
	if buildnum < rowMap.startBuild || buildnum > rowMap.startBuild+rowMap.count-1 {
		return 0, fmt.Errorf("failed to find row in JSON for buildnumber: %d. Row mapping or buildnumber is invalid", buildnum)
	}
	return buildnum - rowMap.startBuild + rowMap.startRow, nil
}

// DictIndexer is a BuildIndexer implementation for when the buildnum to row index mapping is simply a dictionary.
// The value type of this dictionary is interface instead of int so that we don't have to convert the original map.
type DictIndexer map[string]interface{}

func (rowMap DictIndexer) rowForBuild(buildnum int) (int, error) {
	row, ok := rowMap[strconv.Itoa(buildnum)]
	if !ok {
		return 0, fmt.Errorf("failed to find row in JSON for buildnumber: %d. Row mapping or buildnumber is invalid", buildnum)
	}
	var irow float64
	if irow, ok = row.(float64); !ok {
		return 0, fmt.Errorf("failed to find row in JSON for buildnumber: %d. Row mapping contains invalid type", buildnum)
	}
	return int(irow), nil
}

// loadClusters parses and filters the json data, then populates every Cluster struct with
// aggregated job data and totals. The job data specifies all jobs that failed in a cluster and the
// builds that failed for each job, independent of which tests the jobs or builds failed.
func (f *TriageFiler) loadClusters(jsonIn []byte) ([]*Cluster, error) {
	var err error
	f.data, err = parseTriageData(jsonIn)
	if err != nil {
		return nil, err
	}
	if err = f.filterAndValidate(f.windowDays); err != nil {
		return nil, err
	}

	// Aggregate failing builds in each cluster by job (independent of tests).
	for _, clust := range f.data.Clustered {
		clust.filer = f
		clust.jobs = make(map[string][]int)

		for _, test := range clust.Tests {
			for _, job := range test.Jobs {
				for _, buildnum := range job.Builds {
					found := false
					for _, oldBuild := range clust.jobs[job.Name] {
						if oldBuild == buildnum {
							found = true
							break
						}
					}
					if !found {
						clust.jobs[job.Name] = append(clust.jobs[job.Name], buildnum)
					}
				}
			}
		}
		clust.totalJobs = len(clust.jobs)
		clust.totalTests = len(clust.Tests)
		clust.totalBuilds = 0
		for _, builds := range clust.jobs {
			clust.totalBuilds += len(builds)
		}
	}
	return f.data.Clustered, nil
}

// parseTriageData unmarshals raw json data into a triageData struct and creates a BuildIndexer for
// every job.
func parseTriageData(jsonIn []byte) (*triageData, error) {
	var data triageData
	if err := json.Unmarshal(jsonIn, &data); err != nil {
		return nil, err
	}

	if data.Builds.Cols.Started == nil {
		return nil, fmt.Errorf("triage data json is missing the builds.cols.started key")
	}
	if data.Builds.JobsRaw == nil {
		return nil, fmt.Errorf("triage data is missing the builds.jobs key")
	}
	if data.Builds.JobPaths == nil {
		return nil, fmt.Errorf("triage data is missing the builds.job_paths key")
	}
	if data.Clustered == nil {
		return nil, fmt.Errorf("triage data is missing the clustered key")
	}
	// Populate 'Jobs' with the BuildIndexer for each job.
	data.Builds.Jobs = make(map[string]BuildIndexer)
	for jobID, mapper := range data.Builds.JobsRaw {
		switch mapper := mapper.(type) {
		case []interface{}:
			// In this case mapper is a 3 member array. 0:first buildnum, 1:number of builds, 2:start index.
			data.Builds.Jobs[jobID] = ContigIndexer{
				startBuild: int(mapper[0].(float64)),
				count:      int(mapper[1].(float64)),
				startRow:   int(mapper[2].(float64)),
			}
		case map[string]interface{}:
			// In this case mapper is a dictionary.
			data.Builds.Jobs[jobID] = DictIndexer(mapper)
		default:
			return nil, fmt.Errorf("the build number to row index mapping for job '%s' is not an accepted type. Type is: %v", jobID, reflect.TypeOf(mapper))
		}
	}
	return &data, nil
}

// topClusters gets the 'count' most important clusters from a slice of clusters based on number of build failures.
func topClusters(clusters []*Cluster, count int) []*Cluster {
	less := func(i, j int) bool { return clusters[i].totalBuilds > clusters[j].totalBuilds }
	sort.SliceStable(clusters, less)

	if len(clusters) < count {
		count = len(clusters)
	}
	return clusters[0:count]
}

// topTestsFailing returns the top 'count' test names sorted by number of failing jobs.
func (c *Cluster) topTestsFailed(count int) []*Test {
	less := func(i, j int) bool { return len(c.Tests[i].Jobs) > len(c.Tests[j].Jobs) }
	sort.SliceStable(c.Tests, less)

	if len(c.Tests) < count {
		count = len(c.Tests)
	}
	return c.Tests[0:count]
}

// topJobsFailed returns the top 'count' job names sorted by number of failing builds.
func (c *Cluster) topJobsFailed(count int) []*Job {
	slice := make([]*Job, len(c.jobs))
	i := 0
	for jobName, builds := range c.jobs {
		slice[i] = &Job{Name: jobName, Builds: builds}
		i++
	}
	less := func(i, j int) bool { return len(slice[i].Builds) > len(slice[j].Builds) }
	sort.SliceStable(slice, less)

	if len(slice) < count {
		count = len(slice)
	}
	return slice[0:count]
}

// Title is the string to use as the github issue title.
func (c *Cluster) Title() string {
	return fmt.Sprintf("Failure cluster [%s...] failed %d builds, %d jobs, and %d tests over %d days",
		c.Identifier[0:6],
		c.totalBuilds,
		c.totalJobs,
		c.totalTests,
		c.filer.windowDays,
	)
}

// Body returns the body text of the github issue and *must* contain the output of ID().
// closedIssues is a (potentially empty) slice containing all closed issues authored by this bot
// that contain ID() in their body.
// If Body returns an empty string no issue is created.
func (c *Cluster) Body(closedIssues []*githubapi.Issue) string {
	// First check that the most recently closed issue (if any exist) was closed
	// before the start of the sliding window.
	cutoffTime := time.Unix(c.filer.latestStart, 0).AddDate(0, 0, -c.filer.windowDays)
	for _, closed := range closedIssues {
		if closed.ClosedAt.After(cutoffTime) {
			return ""
		}
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "### Failure cluster [%s](%s#%s)\n", c.ID(), triageURL, c.Identifier)
	fmt.Fprintf(&buf, "##### Error text:\n```\n%s\n```\n", c.Text)
	// cluster stats
	fmt.Fprint(&buf, "##### Failure cluster statistics:\n")
	fmt.Fprintf(&buf, "%d tests failed,    %d jobs failed,    %d builds failed.\n", c.totalTests, c.totalJobs, c.totalBuilds)
	fmt.Fprintf(&buf, "Failure stats cover %d day time range '%s' to '%s'.\n##### Top failed tests by jobs failed:\n",
		c.filer.windowDays,
		cutoffTime.Format(timeFormat),
		time.Unix(c.filer.latestStart, 0).Format(timeFormat))
	// top tests failed
	fmt.Fprint(&buf, "\n| Test Name | Jobs Failed |\n| --- | --- |\n")
	for _, test := range c.topTestsFailed(topTestsCount) {
		fmt.Fprintf(&buf, "| %s | %d |\n", test.Name, len(test.Jobs))
	}
	// top jobs failed
	fmt.Fprint(&buf, "\n##### Top failed jobs by builds failed:\n")
	fmt.Fprint(&buf, "\n| Job Name | Builds Failed | Latest Failure |\n| --- | --- | --- |\n")
	for _, job := range c.topJobsFailed(topJobsCount) {
		latest := 0
		latestTime := int64(0)
		rowMap := c.filer.data.Builds.Jobs[job.Name]
		for _, build := range job.Builds {
			row, _ := rowMap.rowForBuild(build) // Already validated start time lookup for all builds.
			buildTime := c.filer.data.Builds.Cols.Started[row]
			if buildTime > latestTime {
				latestTime = buildTime
				latest = build
			}
		}
		path := strings.TrimPrefix(c.filer.data.Builds.JobPaths[job.Name], "gs://")
		fmt.Fprintf(&buf, "| %s | %d | [%s](https://prow.k8s.io/view/gs/%s/%d) |\n", job.Name, len(job.Builds), time.Unix(latestTime, 0).Format(timeFormat), path, latest)
	}
	// previously closed issues if there are any
	if len(closedIssues) > 0 {
		fmt.Fprint(&buf, "\n##### Previously closed issues for this cluster:\n")
		for _, closed := range closedIssues {
			fmt.Fprintf(&buf, "#%d ", *closed.Number)
		}
		fmt.Fprint(&buf, "\n")
	}

	// Create /assign command.
	testNames := make([]string, 0, len(c.Tests))
	for _, test := range c.topTestsFailed(len(c.Tests)) {
		testNames = append(testNames, test.Name)
	}
	ownersMap := c.filer.creator.TestsOwners(testNames)
	if len(ownersMap) > 0 {
		fmt.Fprint(&buf, "\n/assign")
		for user := range ownersMap {
			fmt.Fprintf(&buf, " @%s", user)
		}
		fmt.Fprint(&buf, "\n")
	}

	// Explanations of assignees and sigs
	fmt.Fprint(&buf, c.filer.creator.ExplainTestAssignments(testNames))

	fmt.Fprintf(&buf, "\n[Current Status](%s#%s)", triageURL, c.Identifier)

	return buf.String()
}

// ID yields the string identifier that uniquely identifies this issue.
// This ID must appear in the body of the issue.
// DO NOT CHANGE how this ID is formatted or duplicate issues may be created on github.
func (c *Cluster) ID() string {
	return c.Identifier
}

// Labels returns the labels to apply to the issue created for this cluster on github.
func (c *Cluster) Labels() []string {
	labels := []string{"kind/flake"}

	topTests := make([]string, len(c.Tests))
	for i, test := range c.topTestsFailed(len(c.Tests)) {
		topTests[i] = test.Name
	}
	for sig := range c.filer.creator.TestsSIGs(topTests) {
		labels = append(labels, "sig/"+sig)
	}

	return labels
}

// Owners returns the list of usernames to assign to this issue on github.
func (c *Cluster) Owners() []string {
	// Assign owners by including a /assign command in the body instead of using Owners to set
	// assignees on the issue request. This lets prow do the assignee validation and will mention
	// the user we want to assign even if they can't be assigned.
	return nil
}

// Priority calculates and returns the priority of this issue.
// The returned bool indicates if the returned priority is valid and can be used.
func (c *Cluster) Priority() (string, bool) {
	// TODO implement priority calcs later.
	return "", false
}
