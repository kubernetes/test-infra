/*
Copyright 2020 The Kubernetes Authors.

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

/*
Contains functions that prepare data for output.
*/

package summarize

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/test-infra/triage/utils"
)

// jsonOutput represents the output as it will be written to the JSON.
type jsonOutput struct {
	Clustered []jsonCluster `json:"clustered"`
	Builds    columns       `json:"builds"`
}

// render accepts a map from build paths to builds, and the global clusters, and renders them in a
// format consumable by the web page.
func render(builds map[string]build, clustered nestedFailuresGroups) jsonOutput {
	clusteredSorted := clustered.sortByMostAggregatedFailures()

	flattenedClusters := make([]flattenedGlobalCluster, len(clusteredSorted))

	for i, pair := range clusteredSorted {
		k := pair.Key
		clusters := pair.Group

		flattenedClusters[i] = flattenedGlobalCluster{
			k,
			makeNgramCountsDigest(k),
			clusters.sortByMostFailures(),
		}
	}

	return jsonOutput{
		clustersToDisplay(flattenedClusters, builds),
		buildsToColumns(builds),
	}
}

// sigLabelRE matches '[sig-x]', so long as x does not contain a closing bracket.
var sigLabelRE = regexp.MustCompile(`\[sig-([^]]*)\]`)

/*
annotateOwners assigns ownership to a cluster based on the share of hits in the last day. It modifies
the data parameter in place.

owners maps SIG names to collections of SIG-specific prefixes.
*/
func annotateOwners(data *jsonOutput, builds map[string]build, owners map[string][]string) error {
	var ownerRE *regexp.Regexp = nil
	if owners != nil {
		// Dynamically create a regular expression based on the value of owners.
		/*
			namedOwnerREs is a collection of regular expressions of the form
				(?P<signame>prefixA|prefixB|prefixC)
			where signame is the name of a SIG (such as 'sig-testing') with '-' replaced with '_' for
			compatibility with regex capture group name rules. There can be any number of prefixes
			following the capture group name.
		*/
		namedOwnerREs := make([]string, 0, len(owners))
		for sig, prefixes := range owners {
			// prefixREs is a collection of non-empty prefixes with any special regex characters quoted
			prefixREs := make([]string, 0, len(prefixes))
			for _, prefix := range prefixes {
				if prefix != "" {
					prefixREs = append(prefixREs, regexp.QuoteMeta(prefix))
				}
			}

			namedOwnerREs = append(namedOwnerREs,
				fmt.Sprintf("(?P<%s>%s)",
					strings.Replace(sig, "-", "_", -1), // Regex group names can't have '-', we'll substitute back later
					strings.Join(prefixREs, "|")))
		}

		// ownerRE is the final regex created from the values of namedOwnerREs, placed into a
		// non-capturing group
		var err error
		ownerRE, err = regexp.Compile(fmt.Sprintf(`(?:%s)`, strings.Join(namedOwnerREs, "|")))
		if err != nil {
			return fmt.Errorf("Could not compile ownerRE from provided SIG names and prefixes: %s", err)
		}
	}

	jobPaths := data.Builds.JobPaths
	yesterday := 0
	if len(data.Builds.Cols.Started) > 0 {
		yesterday = utils.Max(data.Builds.Cols.Started...) - (60 * 60 * 24)
	}

	// Determine the owner for each cluster
	for i := range data.Clustered {
		cluster := &data.Clustered[i]
		// Maps owner names to hits (I think hits yesterday and hits today, respectively)
		ownerCounts := make(map[string][]int)

		// For each test, determine the owner with the most hits
		for _, test := range cluster.Tests {
			var owner string
			if submatches := sigLabelRE.FindStringSubmatch(test.Name); submatches != nil {
				owner = submatches[1] // Get the first (and only) submatch of sigLabelRE
			} else if ownerRE != nil {
				normalizedTestName := normalizeName(test.Name)

				// Determine whether there were any named groups with matches for normalizedTestName,
				// and if so what the first named group with a match is
				namedGroupMatchExists := false
				firstMatchingGroupName := ""
				// Names of the named capturing groups, which are really the names of the owners
				groupNames := ownerRE.SubexpNames()
			outer:
				for _, submatches := range ownerRE.FindAllStringSubmatch(normalizedTestName, -1) {
					for i, submatch := range submatches {
						// If the group is named and there was a match
						if groupNames[i] != "" && submatch != "" {
							namedGroupMatchExists = true
							firstMatchingGroupName = groupNames[i]
							break outer
						}
					}
				}

				ownerIndex := ownerRE.FindStringIndex(normalizedTestName)

				if ownerIndex == nil || // If no match was found for the owner, or
					ownerIndex[0] != 0 || // the test name did not begin with the owner name, or
					!namedGroupMatchExists { // there were no named groups that matched
					continue
				}

				// Get the name of the first named group with a non-empty match, and assign it to owner
				owner = firstMatchingGroupName
			}

			owner = strings.Replace(owner, "_", "-", -1) // Substitute '_' back to '-'

			if _, ok := ownerCounts[owner]; !ok {
				ownerCounts[owner] = []int{0, 0}
			}
			counts := ownerCounts[owner]

			for _, job := range test.Jobs {
				if strings.Contains(job.Name, ":") { // non-standard CI
					continue
				}

				jobPath := jobPaths[job.Name]
				for _, build := range job.BuildNumbers {
					bucketKey := fmt.Sprintf("%s/%s", jobPath, build)
					if _, ok := builds[bucketKey]; !ok {
						continue
					} else if builds[bucketKey].Started > yesterday {
						counts[0]++
					} else {
						counts[1]++
					}
				}
			}
		}

		if len(ownerCounts) != 0 {
			// Utility function to find the owner with the most hits yesterday, then most hits today,
			// then first name alphabetically. Returns true if current owner wins, false otherwise.
			newOwnerHasMoreHits := func(topOwner string, topOwnerCounts []int, currentOwner string, currentCounts []int) bool {
				if currentCounts[0] == topOwnerCounts[0] {
					if currentCounts[1] == topOwnerCounts[1] {
						// Which has the earlier name alphabetically
						return currentOwner < topOwner
					}
					return currentCounts[1] > topOwnerCounts[1]
				}
				return currentCounts[0] > topOwnerCounts[0]
			}

			// Find the owner with the most hits
			var topOwner string
			topCounts := []int{0, 0}
			for owner, counts := range ownerCounts {
				if newOwnerHasMoreHits(topOwner, topCounts, owner, counts) {
					topOwner = owner
					topCounts = counts
				}
			}
			cluster.Owner = topOwner
		} else {
			cluster.Owner = "testing"
		}
	}
	return nil
}

// renderSlice returns clusters whose owner field is the owner parameter or whose id field has a
// prefix of the prefix parameter, and the columnar form of the jobs belonging to those clusters.
// If parameters prefix and owner are both the empty string, the function will return empty objects.
func renderSlice(data jsonOutput, builds map[string]build, prefix string, owner string) ([]jsonCluster, columns) {
	clustered := make([]jsonCluster, 0)
	// Maps build paths to builds
	buildsOut := make(map[string]build)
	jobs := make(sets.String)

	// For each cluster whose owner field is the owner parameter, or whose id field has a prefix of
	// the prefix parameter, add its tests' jobs to the jobs set.
	for _, cluster := range data.Clustered {
		if owner != "" && cluster.Owner == owner {
			clustered = append(clustered, cluster)
		} else if prefix != "" && strings.HasPrefix(cluster.ID, prefix) {
			clustered = append(clustered, cluster)
		} else {
			continue
		}

		for _, tst := range cluster.Tests {
			for _, jb := range tst.Jobs {
				jobs.Insert(jb.Name)
			}
		}
	}

	// Add builds whose job is in jobs to buildsOut
	for _, bld := range builds {
		if jobs.Has(bld.Job) {
			buildsOut[bld.Path] = bld
		}
	}

	return clustered, buildsToColumns(buildsOut)
}

// flattenedGlobalCluster is the key and value of a specific global cluster (as clusterText and
// sortedTests, respectively), plus the result of calling makeNgramCountsDigest on the key.
type flattenedGlobalCluster struct {
	clusterText       string
	ngramCountsDigest string
	sortedTests       []failuresGroupPair
}

// test represents a test name and a collection of associated jobs.
type test struct {
	Name string `json:"name"`
	Jobs []job  `json:"jobs"`
}

/*
jsonCluster represents a global cluster as it will be written to the JSON.

	key:   the cluster text
	id:    the result of calling makeNgramCountsDigest() on key
	text:  a failure text from one of the cluster's failures
	spans: common spans between all of the cluster's failure texts
	tests: the build numbers that belong to the cluster's failures as per testGroupByJob()
	owner: the SIG that owns the cluster, determined by annotateOwners()
*/
type jsonCluster struct {
	Key   string `json:"key"`
	ID    string `json:"id"`
	Text  string `json:"text"`
	Spans []int  `json:"spans"`
	Tests []test `json:"tests"`
	Owner string `json:"owner"`
}

// clustersToDisplay transposes and sorts the flattened output of clusterGlobal.
// builds maps a build path to a build object.
func clustersToDisplay(clustered []flattenedGlobalCluster, builds map[string]build) []jsonCluster {
	jsonClusters := make([]jsonCluster, 0, len(clustered))

	for _, flattened := range clustered {
		key := flattened.clusterText
		keyID := flattened.ngramCountsDigest
		clusters := flattened.sortedTests

		// Determine the number of failures across all clusters
		numClusterFailures := 0
		for _, cluster := range clusters {
			numClusterFailures += len(cluster.Failures)
		}

		if numClusterFailures > 1 {
			jCluster := jsonCluster{
				Key:   key,
				ID:    keyID,
				Text:  clusters[0].Failures[0].FailureText,
				Tests: make([]test, len(clusters)),
			}

			// Get all of the failure texts from all clusters
			clusterFailureTexts := make([]string, 0, numClusterFailures)
			for _, cluster := range clusters {
				for _, flr := range cluster.Failures {
					clusterFailureTexts = append(clusterFailureTexts, flr.FailureText)
				}
			}
			jCluster.Spans = commonSpans(clusterFailureTexts)

			// Fill out jCluster.tests
			for i, cluster := range clusters {
				jCluster.Tests[i] = test{
					Name: cluster.Key,
					Jobs: testsGroupByJob(cluster.Failures, builds),
				}
			}

			jsonClusters = append(jsonClusters, jCluster)
		}
	}

	return jsonClusters
}

// job represents a job name and a collection of associated build numbers.
type job struct {
	Name         string   `json:"name"`
	BuildNumbers []string `json:"builds"`
}

// build represents a specific instance of a build.
type build struct {
	Path        string `json:"path"`
	Started     int    `json:"started"`
	Elapsed     int    `json:"elapsed"`
	TestsRun    int    `json:"tests_run"`
	TestsFailed int    `json:"tests_failed"`
	Result      string `json:"result"`
	Executor    string `json:"executor"`
	Job         string `json:"job"`
	Number      int    `json:"number"`
	PR          string `json:"pr"`
	Key         string `json:"key"` // Often nonexistent
}

/*
testsGroupByJob takes a group of failures and a map of builds and returns the list of build numbers
that belong to each failure's job.

builds is a mapping from build paths to build objects.
*/
func testsGroupByJob(failures []failure, builds map[string]build) []job {
	// groups maps job names to sets of failures' build numbers (as strings).
	groups := make(map[string]sets.String)

	// For each failure, grab its build's job name. Map the job name to the failure's build number.
	for _, flr := range failures {
		// Try to grab the build from builds if it exists
		if bld, ok := builds[flr.Build]; ok {
			// If the JSON build's "number" field was not null
			if bld.Number != 0 {
				// Create the set if one doesn't exist for the given job
				if _, ok := groups[bld.Job]; !ok {
					groups[bld.Job] = make(sets.String, 1)
				}
				groups[bld.Job].Insert(strconv.Itoa(bld.Number))
			}
		}
	}

	// Sort groups in two stages.
	// First, sort each build number set in descending order.
	// Then, sort the jobs by the number of build numbers in each job's build number slice, descending.

	// First stage
	// sortedBuildNumbers is essentially groups, but with the build numbers sorted.
	sortedBuildNumbers := make(map[string][]string, len(groups))
	// Create the slice to hold the set elements, fill it, and sort it
	for jobName, buildNumberSet := range groups {
		// Initialize the int slice
		sortedBuildNumbers[jobName] = make([]string, len(buildNumberSet))

		// Fill it
		iter := 0
		for buildNumber := range buildNumberSet {
			sortedBuildNumbers[jobName][iter] = buildNumber
			iter++
		}

		// Sort it. Use > instead of < in less function to sort descending.
		sort.Slice(sortedBuildNumbers[jobName], func(i, j int) bool { return sortedBuildNumbers[jobName][i] > sortedBuildNumbers[jobName][j] })
	}

	// Second stage
	sortedGroups := make([]job, 0, len(groups))

	// Fill sortedGroups
	for newJobName, newBuildNumbers := range sortedBuildNumbers {
		sortedGroups = append(sortedGroups, job{newJobName, newBuildNumbers})
	}
	// Sort it
	sort.Slice(sortedGroups, func(i, j int) bool {
		iGroupLen := len(sortedGroups[i].BuildNumbers)
		jGroupLen := len(sortedGroups[j].BuildNumbers)

		// If they're the same length, sort by job name alphabetically
		if iGroupLen == jGroupLen {
			return sortedGroups[i].Name < sortedGroups[j].Name
		}

		// Use > instead of < to sort descending.
		return iGroupLen > jGroupLen
	})

	return sortedGroups
}

/*
columnarBuilds represents a collection of build objects where the i-th build's property p can be
found at p[i].

For example, the 4th (0-indexed) build's start time can be found in started[4], while its elapsed
time can be found in elapsed[4].
*/
type columnarBuilds struct {
	Started     []int    `json:"started"`
	TestsFailed []int    `json:"tests_failed"`
	Elapsed     []int    `json:"elapsed"`
	TestsRun    []int    `json:"tests_run"`
	Result      []string `json:"result"`
	Executor    []string `json:"executor"`
	PR          []string `json:"pr"`
}

// currentIndex returns the index of the next build to be stored (and, by extension, the number of
// builds currently stored).
func (cb *columnarBuilds) currentIndex() int {
	return len(cb.Started)
}

// insert adds a build into the columnarBuilds object.
func (cb *columnarBuilds) insert(b build) {
	cb.Started = append(cb.Started, b.Started)
	cb.TestsFailed = append(cb.TestsFailed, b.TestsFailed)
	cb.Elapsed = append(cb.Elapsed, b.Elapsed)
	cb.TestsRun = append(cb.TestsRun, b.TestsRun)
	cb.Result = append(cb.Result, b.Result)
	cb.Executor = append(cb.Executor, b.Executor)
	cb.PR = append(cb.PR, b.PR)
}

// newColumnarBuilds creates a columnarBuilds object with the correct number of columns. The number
// of columns is the same as the number of builds being converted to columnar form.
func newColumnarBuilds(columns int) columnarBuilds {
	// Start the length at 0 because columnarBuilds.currentIndex() relies on the length.
	return columnarBuilds{
		Started:     make([]int, 0, columns),
		TestsFailed: make([]int, 0, columns),
		Elapsed:     make([]int, 0, columns),
		TestsRun:    make([]int, 0, columns),
		Result:      make([]string, 0, columns),
		Executor:    make([]string, 0, columns),
		PR:          make([]string, 0, columns),
	}
}

/*
jobCollection represents a collection of jobs. It can either be a map[int]int (a mapping from
build numbers to indexes of builds in the columnar representation) or a []int (a condensed form
of the mapping for dense sequential mappings from builds to indexes; see buildsToColumns() comment).
This is necessary because the outputted JSON is unstructured, and has some fields that can be
either a map or a slice.
*/
type jobCollection interface{}

/*
columns represents a collection of builds in columnar form, plus the necessary maps to decode it.

jobs maps job names to their location in the columnar form.

cols is the collection of builds in columnar form.

jobPaths maps a job name to a build path, minus the last path segment.
*/
type columns struct {
	Jobs     map[string]jobCollection `json:"jobs"`
	Cols     columnarBuilds           `json:"cols"`
	JobPaths map[string]string        `json:"job_paths"`
}

// buildsToColumns converts a map (from build paths to builds) into a columnar form. This compresses
// much better with gzip. See columnarBuilds for more information on the columnar form.
func buildsToColumns(builds map[string]build) columns {
	// The function result
	// result.jobs maps job names to either map[int]int or []int. See jobCollection.
	result := columns{make(map[string]jobCollection), newColumnarBuilds(len(builds)), make(map[string]string)}

	// Sort the builds before making them columnar
	sortedBuilds := make([]build, 0, len(builds))
	// Fill the slice
	for _, bld := range builds {
		sortedBuilds = append(sortedBuilds, bld)
	}
	// Sort the slice
	sort.Slice(sortedBuilds, func(i, j int) bool {
		// Sort by job name, then by build number
		if sortedBuilds[i].Job == sortedBuilds[j].Job {
			return sortedBuilds[i].Number < sortedBuilds[j].Number
		}
		return sortedBuilds[i].Job < sortedBuilds[j].Job
	})

	// Add the builds to result.cols
	for _, bld := range sortedBuilds {
		// If there was no build number when the build was retrieved from the JSON
		if bld.Number == 0 {
			continue
		}

		// Get the index within cols's slices of the next inserted build
		index := result.Cols.currentIndex()

		// Add the build
		result.Cols.insert(bld)

		// job maps build numbers to their indexes in the columnar representation
		var job map[int]int
		if _, ok := result.Jobs[bld.Job]; !ok {
			result.Jobs[bld.Job] = make(map[int]int)
		}
		// We can safely assert map[int]int here because replacement of maps with slices only
		// happens later
		job = result.Jobs[bld.Job].(map[int]int)

		// Store the job path
		if len(job) == 0 {
			result.JobPaths[bld.Job] = bld.Path[:strings.LastIndex(bld.Path, "/")]
		}

		// Store the column number (index) so we know in which column to find which build
		job[bld.Number] = index
	}

	// Sort build numbers and compress some data
	for jobName, indexes := range result.Jobs {
		// Sort the build numbers
		sortedBuildNumbers := make([]int, 0, len(indexes.(map[int]int)))
		for key := range indexes.(map[int]int) {
			sortedBuildNumbers = append(sortedBuildNumbers, key)
		}
		sort.Ints(sortedBuildNumbers)

		base := indexes.(map[int]int)[sortedBuildNumbers[0]]
		count := len(sortedBuildNumbers)

		// Optimization: if we have a dense sequential mapping of builds=>indexes,
		// store only the first build number, the run length, and the first index number.
		allTrue := true
		for i, buildNumber := range sortedBuildNumbers {
			if indexes.(map[int]int)[buildNumber] != i+base {
				allTrue = false
				break
			}
		}
		if (sortedBuildNumbers[len(sortedBuildNumbers)-1] == sortedBuildNumbers[0]+count-1) && allTrue {
			result.Jobs[jobName] = []int{sortedBuildNumbers[0], count, base}
			for _, n := range sortedBuildNumbers {
				if !(n <= sortedBuildNumbers[0]+len(sortedBuildNumbers)) {
					klog.Fatal(jobName, n, result.Jobs[jobName], len(sortedBuildNumbers), sortedBuildNumbers)
				}
			}
		}
	}
	return result
}
