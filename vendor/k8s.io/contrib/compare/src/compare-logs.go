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

package src

import (
	"fmt"
	"math"
	"os"
	"sort"
	"text/tabwriter"

	"k8s.io/kubernetes/test/e2e"

	"github.com/daviddengcn/go-colortext"
	"github.com/golang/glog"
)

const (
	// LogsGenerationVarianceAllowedPercent specifies how much variance we allow between results.
	// Precise semantics is that we say that difference is too big if greater value is more than
	// LogsGenerationVarianceAllowedPercent bigger than the smaller one. I.e. if X < Y, then Y is too
	// big if Y > X + X*(LogsGenerationVarianceAllowedPercent/100).
	LogsGenerationVarianceAllowedPercent = 50
	// To avoid false negatives we assume that minimal log generation rate to 10
	minLogGeneration = 10
)

type logsDataOnNode struct {
	node           string
	generationRate float64
}

type logsDataArray []logsDataOnNode

func (a logsDataArray) Len() int           { return len(a) }
func (a logsDataArray) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a logsDataArray) Less(i, j int) bool { return a[i].generationRate < a[j].generationRate }

// ViolatingLogGenerationPair stores a pair of results which proves that the difference is too big
type ViolatingLogGenerationPair struct {
	left, right logsDataArray
}

// ViolatingLogGenerationData stores offending pairs keyed by the log file identified by the path.
type ViolatingLogGenerationData map[string]ViolatingLogGenerationPair

func writeViolatingLogsData(data, baseline logsDataArray, allowedVariance float64, enableOutputColoring bool, writer *tabwriter.Writer) {
	dataMin := data[0]
	dataMax := data[len(data)-1]
	baselineMin := baseline[0]
	baselineMax := baseline[len(baseline)-1]
	fmt.Fprintf(writer, "(%v: ", dataMin.node)
	changeColorFloat64AndWrite(dataMin.generationRate, baselineMin.generationRate, allowedVariance, enableOutputColoring, writer)
	fmt.Fprintf(writer, ", %v: ", dataMax.node)
	changeColorFloat64AndWrite(dataMax.generationRate, baselineMax.generationRate, allowedVariance, enableOutputColoring, writer)
	fmt.Fprint(writer, ")")
	return
}

// PrintToStdout prints offending data to the Stdout in a human readable format.
func (d *ViolatingLogGenerationData) PrintToStdout(leftBuild, rightBuild int, enableOutputColoring bool) {
	writer := tabwriter.NewWriter(os.Stdout, 1, 0, 1, ' ', 0)
	fmt.Fprint(writer, "File\t")
	// to adjust number of spaces...
	if enableOutputColoring {
		ChangeColor(ct.White, writer)
		ResetColor(writer)
	}
	printBuildNumber(leftBuild, writer, enableOutputColoring)
	fmt.Fprint(writer, "\t")
	if enableOutputColoring {
		ChangeColor(ct.White, writer)
		ResetColor(writer)
	}
	printBuildNumber(rightBuild, writer, enableOutputColoring)
	fmt.Fprint(writer, "\n")
	allowedVariance := float64(100+LogsGenerationVarianceAllowedPercent) / float64(100)
	for k, v := range *d {
		fmt.Fprintf(writer, "%v\t", k)
		writeViolatingLogsData(v.left, v.right, allowedVariance, enableOutputColoring, writer)
		fmt.Fprint(writer, "\t")
		writeViolatingLogsData(v.right, v.left, allowedVariance, enableOutputColoring, writer)
		fmt.Fprint(writer, "\n")
	}
	writer.Flush()
}

func computeLogsAggregates(data *e2e.LogsSizeDataSummary) map[string]logsDataArray {
	result := make(map[string]logsDataArray)
	for node, nodeData := range *data {
		for file, summary := range nodeData {
			result[file] = append(result[file], logsDataOnNode{node: node, generationRate: float64(summary.AverageGenerationRate)})
		}
	}
	for k := range result {
		sort.Sort(result[k])
	}
	return result
}

func isLogGenerationSimilarEnough(left logsDataArray, right logsDataArray, allowedVariance float64) bool {
	leftMin := math.Max(left[0].generationRate, minLogGeneration)
	leftMax := math.Max(left[len(left)-1].generationRate, minLogGeneration)
	rightMin := math.Max(right[0].generationRate, minLogGeneration)
	rightMax := math.Max(right[len(right)-1].generationRate, minLogGeneration)

	return leq(leftMin, rightMin*allowedVariance) &&
		leq(rightMin, leftMin*allowedVariance) &&
		leq(leftMax, rightMax*allowedVariance) &&
		leq(rightMax, leftMax*allowedVariance)
}

// CompareLogGenerationSpeed given two summaries compares the data in them and returns set of
// offending values. Precise semantics is that for each file (e.g. /var/log/kubelet.log) we check
// if minimal and maximal values are roughly the same.
func CompareLogGenerationSpeed(left *e2e.LogsSizeDataSummary, right *e2e.LogsSizeDataSummary) ViolatingLogGenerationData {
	result := make(ViolatingLogGenerationData)
	if left == nil || right == nil {
		glog.Warningf("At least one of received data is nil:\nleft: %p\nright:%p", left, right)
		return result
	}
	leftAggregates := computeLogsAggregates(left)
	rightAggregates := computeLogsAggregates(right)
	if len(leftAggregates) == 0 || len(rightAggregates) == 0 {
		glog.Warningf("No data found in at leaste one input\nleft: %v\nright: %v", leftAggregates, rightAggregates)
		return result
	}

	for path := range leftAggregates {
		if _, ok := rightAggregates[path]; !ok {
			glog.V(4).Infof("Missing results for file %v on right-hand side.", path)
			continue
		}
		if !isLogGenerationSimilarEnough(leftAggregates[path], rightAggregates[path], float64(100+LogsGenerationVarianceAllowedPercent)/float64(100)) {
			result[path] = ViolatingLogGenerationPair{
				left:  leftAggregates[path],
				right: rightAggregates[path],
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
