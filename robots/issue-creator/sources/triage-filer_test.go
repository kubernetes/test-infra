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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/github"
	"k8s.io/test-infra/robots/issue-creator/creator"
	"k8s.io/test-infra/robots/issue-creator/testowner"
)

var (
	// json1issue2job2test is a small example of the JSON format that loadClusters reads.
	// It includes all of the different types of formatting that is accepted. Namely both types
	// of buildnum to row index mappings.
	json1issue2job2test []byte
	// buildTimes is a map containing the build times of builds found in the json1issue2job2test JSON data.
	buildTimes map[int]int64
	// sampleOwnerCSV is a small sample test owners csv file that contains both real test owner
	// data and owner/SIG info for a fake test in json1issue2job2test.
	sampleOwnerCSV []byte
	// latestBuildTime is the end time of the sliding window for these tests.
	latestBuildTime int64
)

func init() {
	latestBuildTime = int64(947462400) // Jan 10, 2000
	hourSecs := int64(60 * 60)
	dailySecs := hourSecs * 24
	buildTimes = map[int]int64{
		41:  latestBuildTime - (dailySecs * 10),           // before window start
		42:  latestBuildTime + hourSecs - (dailySecs * 5), // just inside window start
		43:  latestBuildTime + hourSecs - (dailySecs * 4),
		52:  latestBuildTime + hourSecs - (dailySecs * 2),
		142: latestBuildTime - dailySecs, // a day before window end
		144: latestBuildTime - hourSecs,  // an hour before window end
	}

	json1issue2job2test = []byte(
		`{
		"builds":
		{
			"cols":
			{
				"started":
				[
					` + strconv.FormatInt(buildTimes[41], 10) + `,
					` + strconv.FormatInt(buildTimes[42], 10) + `,
					` + strconv.FormatInt(buildTimes[43], 10) + `,
					10000000,
					10000000,
					10000000,
					10000000,
					10000000,
					10000000,
					10000000,
					10000000,
					` + strconv.FormatInt(buildTimes[52], 10) + `,
					` + strconv.FormatInt(buildTimes[142], 10) + `,
					10000000,
					` + strconv.FormatInt(buildTimes[144], 10) + `
				]
			},
			"jobs":
			{
				"jobname1": [41, 12, 0],
				"jobname2": {"142": 12, "144": 14},
				"pr:jobname3": {"200": 13}
			},
			"job_paths":
			{
				"jobname1": "path//to/jobname1",
				"jobname2": "path//to/jobname2",
				"pr:jobname3": "path//to/pr:jobname3"
			}
		},
		"clustered":
		[
			{
				"id": "key_hash",
				"key": "key_text",
				"tests": 
				[
					{
						"jobs":
						[
							{
								"builds": [42, 43, 52],
								"name": "jobname1"
							},
							{
								"builds": [144],
								"name": "jobname2"
							}
						],
						"name": "testname1"
					},
					{
						"jobs":
						[
							{
								"builds": [41, 42, 43],
								"name": "jobname1"
							},
							{
								"builds": [200],
								"name": "pr:jobname3"
							}
						],
						"name": "testname2"
					}
				],
				"text":	"issue_name"
			}
		]
	}`)

	sampleOwnerCSV = []byte(
		`name,owner,auto-assigned,sig
Sysctls should support sysctls,Random-Liu,1,node
Sysctls should support unsafe sysctls which are actually allowed,deads2k,1,node
testname1,cjwagner ,1,sigarea
testname2,spxtr,1,sigarea
ThirdParty resources Simple Third Party creating/deleting thirdparty objects works,luxas,1,api-machinery
Upgrade cluster upgrade should maintain a functioning cluster,luxas,1,cluster-lifecycle
Upgrade master upgrade should maintain a functioning cluster,xiang90,1,cluster-lifecycle
Upgrade node upgrade should maintain a functioning cluster,zmerlynn,1,cluster-lifecycle
Variable Expansion should allow composing env vars into new env vars,derekwaynecarr,0,node
Variable Expansion should allow substituting values in a container's args,dchen1107,1,node
Variable Expansion should allow substituting values in a container's command,mml,1,node
Volume Disk Format verify disk format type - eagerzeroedthick is honored for dynamically provisioned pv using storageclass,piosz,1,`)
}

// NewTestTriageFiler creates a new TriageFiler that isn't connected to an IssueCreator so that
// it can be used for testing.
func NewTestTriageFiler() *TriageFiler {
	return &TriageFiler{
		creator:          &creator.IssueCreator{},
		topClustersCount: 3,
		windowDays:       5,
	}
}

func TestTFParserSimple(t *testing.T) {
	f := NewTestTriageFiler()
	issues, err := f.loadClusters(json1issue2job2test)
	if err != nil {
		t.Fatalf("Error parsing triage data: %v\n", err)
	}

	if len(issues) != 1 {
		t.Error("Expected 1 issue, got ", len(issues))
	}
	if issues[0].Text != "issue_name" {
		t.Error("Expected Text='issue_name', got ", issues[0].Text)
	}
	if issues[0].Identifier != "key_hash" {
		t.Error("Expected Identifier='key_hash', got ", issues[0].Identifier)
	}
	// Note that 5 builds failed in json, but one is outside the time window.
	if issues[0].totalBuilds != 4 {
		t.Error("Expected totalBuilds failed = 4, got ", issues[0].totalBuilds)
	}
	// Note that 3 jobs failed in json, but one is a PR job and should be ignored.
	if issues[0].totalJobs != 2 || len(issues[0].jobs) != 2 {
		t.Error("Expected totalJobs failed = 2, got ", issues[0].totalJobs)
	}
	if issues[0].totalTests != 2 || len(issues[0].Tests) != 2 {
		t.Error("Expected totalTests failed = 2, got ", issues[0].totalTests)
	}
	if f.data.Builds.JobPaths["jobname1"] != "path//to/jobname1" ||
		f.data.Builds.JobPaths["jobname2"] != "path//to/jobname2" {
		t.Error("Invalid jobpath. got jobname1: ", f.data.Builds.JobPaths["jobname1"],
			" and jobname2: ", f.data.Builds.JobPaths["jobname2"])
	}

	checkBuildStart(t, f, "jobname1", 42, buildTimes[42])
	checkBuildStart(t, f, "jobname1", 52, buildTimes[52])
	checkBuildStart(t, f, "jobname2", 144, buildTimes[144])

	checkCluster(issues[0], t)
}

func checkBuildStart(t *testing.T, f *TriageFiler, jobName string, build int, expected int64) {
	row, err := f.data.Builds.Jobs[jobName].rowForBuild(build)
	if err != nil {
		t.Errorf("Failed to look up row index for %s:%d", jobName, build)
	}
	actual := f.data.Builds.Cols.Started[row]
	if actual != expected {
		t.Errorf("Expected build start time for build %s:%d to be %d, got %d.", jobName, build, expected, actual)
	}
}

// checkCluster checks that the properties that should be true for all clusters hold for this cluster
func checkCluster(clust *Cluster, t *testing.T) {
	if !checkTopFailingsSorted(clust) {
		t.Errorf("Top tests or jobs is improperly sorted for cluster: %s\n", clust.Identifier)
	}
	if clust.totalJobs != len(clust.jobs) {
		t.Errorf("Total job count is invalid for cluster: %s\n", clust.Identifier)
	}
	if clust.totalTests != len(clust.Tests) {
		t.Errorf("Total test count is invalid for cluster: %s\n", clust.Identifier)
	}
	title := clust.Title()
	body := clust.Body(nil)
	id := clust.ID()
	if len(title) <= 0 {
		t.Errorf("Title of cluster: %s is empty!", clust.Identifier)
	}
	if len(body) <= 0 {
		t.Errorf("Body of cluster: %s is empty!", clust.Identifier)
	}
	if len(id) <= 0 {
		t.Errorf("ID of cluster: %s is empty!", clust.Identifier)
	}
	if !strings.Contains(body, id) {
		t.Errorf("The body text for cluster: %s does not contain its ID!\n", clust.Identifier)
	}
	//ensure that 'kind/flake' is among the label set
	found := false
	for _, label := range clust.Labels() {
		if label == "kind/flake" {
			found = true
		} else {
			if label == "" {
				t.Errorf("Cluster: %s has an empty label!\n", clust.Identifier)
			}
		}
	}
	if !found {
		t.Errorf("The cluster: %s does not have the label 'kind/flake'!", clust.Identifier)
	}
}

func TestTFOwnersAndSIGs(t *testing.T) {
	// Integration test for triage-filers use of issue-creator's TestsOwners, TestsSIGs, and
	// ExplainTestAssignments. These functions in turn rely on OwnerList.
	f := NewTestTriageFiler()
	var err error
	f.creator.Collaborators = []string{"cjwagner", "spxtr"}
	f.creator.Owners, err = testowner.NewOwnerListFromCsv(bytes.NewReader(sampleOwnerCSV))
	f.creator.MaxSIGCount = 3
	f.creator.MaxAssignees = 3
	if err != nil {
		t.Fatalf("Failed to create a new OwnersList.  errmsg: %v", err)
	}

	// Check that the usernames and sig areas are as expected (no stay commas or anything like that).
	clusters, err := f.loadClusters(json1issue2job2test)
	if err != nil {
		t.Fatalf("Failed to load clusters: %v", err)
	}
	foundSIG := false
	for _, label := range clusters[0].Labels() {
		if label == "sig/sigarea" {
			foundSIG = true
			break
		}
	}
	if !foundSIG {
		t.Errorf("Failed to get the SIG for cluster: %s\n", clusters[0].Identifier)
	}

	// Check that the body contains a table that correctly explains why users and sig areas were assigned.
	body := clusters[0].Body(nil)
	if !strings.Contains(body, "| cjwagner | testname1 |") {
		t.Errorf("Body should contain a table row to explain that 'cjwagner' was assigned due to ownership of 'testname1'.")
	}
	if !strings.Contains(body, "| spxtr | testname2 |") {
		t.Errorf("Body should contain a table row to explain that 'spxtr' was assigned due to ownership of 'testname2'.")
	}
	if !strings.Contains(body, "| sig/sigarea | testname1; testname2 |") {
		t.Errorf("Body should contain a table row to explain that 'sigarea' was set as a SIG due to ownership of 'testname1' and 'testname2'.")
	}

	// Check that the body contains the assignments themselves:
	if !strings.Contains(body, "/assign @cjwagner @spxtr") && !strings.Contains(body, "/assign @spxtr @cjwagner") {
		t.Errorf("Failed to find the '/assign' command in the body of cluster: %s\n%q\n", clusters[0].Identifier, body)
	}
}

// TestTFPrevCloseInWindow checks that Cluster issues will abort issue creation by returning an empty
// body if there is a recently closed issue for the cluster.
func TestTFPrevCloseInWindow(t *testing.T) {
	f := NewTestTriageFiler()
	clusters, err := f.loadClusters(json1issue2job2test)
	if err != nil || len(clusters) == 0 {
		t.Fatalf("Error parsing triage data: %v\n", err)
	}
	clust := clusters[0]

	lastWeek := time.Unix(latestBuildTime, 0).AddDate(0, 0, -7)
	yesterday := time.Unix(latestBuildTime, 0).AddDate(0, 0, -1)
	five := 5
	// Only need to populate the Issue.ClosedAt and Issue.Number field of the MungeObject.
	prevIssues := []*github.Issue{{ClosedAt: &yesterday, Number: &five}}
	if clust.Body(prevIssues) != "" {
		t.Errorf("Cluster returned an issue body when there was a recently closed issue for the cluster.")
	}

	prevIssues = []*github.Issue{{ClosedAt: &lastWeek, Number: &five}}
	if clust.Body(prevIssues) == "" {
		t.Errorf("Cluster returned an empty issue body when it should have returned a valid body.")
	}
}

func checkTopFailingsSorted(issue *Cluster) bool {
	return checkTopJobsFailedSorted(issue) && checkTopTestsFailedSorted(issue)
}

func checkTopJobsFailedSorted(issue *Cluster) bool {
	topJobs := issue.topJobsFailed(len(issue.jobs))
	for i := 1; i < len(topJobs); i++ {
		if len(topJobs[i-1].Builds) < len(topJobs[i].Builds) {
			return false
		}
	}
	return true
}

func checkTopTestsFailedSorted(issue *Cluster) bool {
	topTests := issue.topTestsFailed(len(issue.Tests))
	for i := 1; i < len(topTests); i++ {
		if len(topTests[i-1].Jobs) < len(topTests[i].Jobs) {
			return false
		}
	}
	return true
}
