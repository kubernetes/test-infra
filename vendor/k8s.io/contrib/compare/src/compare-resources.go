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
	"strings"
	"text/tabwriter"

	"k8s.io/kubernetes/test/e2e"

	"github.com/daviddengcn/go-colortext"
	"github.com/golang/glog"
)

const (
	// ResourceUsageVarianceAllowedPercent specifies how much variance we allow between results.
	// Precise semantics is that we say that difference is too big if greater value is more than
	// ResourceUsageVarianceAllowedPercent bigger than the smaller one. I.e. if X < Y, then Y is too
	// big if Y > X + X*(ResourceUsageVarianceAllowedPercent/100).
	ResourceUsageVarianceAllowedPercent = 50
	// To avoid false negatives we assume that minimal CPU usage is 5% and memory 50MB
	minCPU = 0.05
	minMem = int64(50 * 1024 * 1024)
)

type percentileUsageData struct {
	// We keep percentile as a string because go JSON encoder can't handle int keys.
	percentile string
	cpuData    []float64
	memData    []int64
}

// ViolatingResourceUsageDataPair stores a pair of results which proves that the difference is too big
type ViolatingResourceUsageDataPair struct {
	percentile   string
	leftCPUData  []float64
	rightCPUData []float64
	leftMemData  []int64
	rightMemData []int64
}

// ViolatingResourceUsageData stores offending pairs keyed by the log file
// identified by the container name.
type ViolatingResourceUsageData map[string]ViolatingResourceUsageDataPair

func writeViolatingResourceData(minCPU, maxCPU, baselineMinCPU, baselineMaxCPU, allowedVariance float64, enableOutputColoring bool, writer *tabwriter.Writer) {
	fmt.Fprint(writer, "(")
	changeColorFloat64AndWrite(minCPU, baselineMinCPU, allowedVariance, enableOutputColoring, writer)
	fmt.Fprint(writer, ",")
	changeColorFloat64AndWrite(maxCPU, baselineMaxCPU, allowedVariance, enableOutputColoring, writer)
	fmt.Fprint(writer, ")")
}

// PrintToStdout prints result to Stdout assuming that the comparison logic is a standard one (comparing minimums and maximums)
// If comparison logic is changed this function should be updated as well.
func (d *ViolatingResourceUsageData) PrintToStdout(leftBuild, rightBuild int, enableOutputColoring bool) {
	writer := tabwriter.NewWriter(os.Stdout, 1, 0, 1, ' ', 0)
	fmt.Fprint(writer, "Percentile\tContainer\tCPU in ")
	if enableOutputColoring {
		ChangeColor(ct.White, writer)
		ResetColor(writer)
	}
	printBuildNumber(leftBuild, writer, enableOutputColoring)
	fmt.Fprint(writer, "\tMem in ")
	if enableOutputColoring {
		ChangeColor(ct.White, writer)
		ResetColor(writer)
	}
	printBuildNumber(leftBuild, writer, enableOutputColoring)
	fmt.Fprint(writer, "\tCPU in ")
	if enableOutputColoring {
		ChangeColor(ct.White, writer)
		ResetColor(writer)
	}
	printBuildNumber(rightBuild, writer, enableOutputColoring)
	fmt.Fprint(writer, "\tMem in ")
	if enableOutputColoring {
		ChangeColor(ct.White, writer)
		ResetColor(writer)
	}
	printBuildNumber(rightBuild, writer, enableOutputColoring)
	fmt.Fprint(writer, "\n")
	allowedVariance := float64(100+ResourceUsageVarianceAllowedPercent) / float64(100)
	for k, v := range *d {
		fmt.Fprintf(writer, "%v\t%v\t", v.percentile, k)

		leftCPUMin := v.leftCPUData[0]
		leftCPUMax := v.leftCPUData[len(v.leftCPUData)-1]
		leftMemMin := v.leftMemData[0]
		leftMemMax := v.leftMemData[len(v.leftMemData)-1]
		rightCPUMin := v.rightCPUData[0]
		rightCPUMax := v.rightCPUData[len(v.rightCPUData)-1]
		rightMemMin := v.rightMemData[0]
		rightMemMax := v.rightMemData[len(v.rightMemData)-1]

		writeViolatingResourceData(leftCPUMin, leftCPUMax, rightCPUMin, rightCPUMax, allowedVariance, enableOutputColoring, writer)
		fmt.Fprint(writer, "\t")
		writeViolatingResourceData(float64(leftMemMin), float64(leftMemMax), float64(rightMemMin), float64(rightMemMax), allowedVariance, enableOutputColoring, writer)
		fmt.Fprint(writer, "\t")
		writeViolatingResourceData(rightCPUMin, rightCPUMax, leftCPUMin, leftCPUMax, allowedVariance, enableOutputColoring, writer)
		fmt.Fprint(writer, "\t")
		writeViolatingResourceData(float64(rightMemMin), float64(rightMemMax), float64(leftMemMin), float64(leftMemMax), allowedVariance, enableOutputColoring, writer)
		fmt.Fprint(writer, "\n")
	}
	writer.Flush()
}

type int64arr []int64

func (a int64arr) Len() int           { return len(a) }
func (a int64arr) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a int64arr) Less(i, j int) bool { return a[i] < a[j] }

func max(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func min(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func getContainerKind(containerName string) string {
	return containerName[strings.LastIndex(containerName, "/")+1:]
}

// A simple comparison checking if minimum and maximums in both datasets are within allowedVariance
// If this function changes, PrintToStdout should be updated accordingly.
func isResourceUsageSimilarEnough(left, right percentileUsageData, allowedVariance float64) bool {
	if len(left.cpuData) == 0 || len(left.memData) == 0 || len(right.cpuData) == 0 || len(right.memData) == 0 {
		glog.V(4).Infof("Length of at least one data vector is zero. Returning false for the lack of data.")
		return false
	}

	sort.Float64s(left.cpuData)
	sort.Float64s(right.cpuData)
	sort.Sort(int64arr(left.memData))
	sort.Sort(int64arr(right.memData))

	leftCPUMin := math.Max(left.cpuData[0], minCPU)
	leftCPUMax := math.Max(left.cpuData[len(left.cpuData)-1], minCPU)
	leftMemMin := max(left.memData[0], minMem)
	leftMemMax := max(left.memData[len(left.memData)-1], minMem)
	rightCPUMin := math.Max(right.cpuData[0], minCPU)
	rightCPUMax := math.Max(right.cpuData[len(right.cpuData)-1], minCPU)
	rightMemMin := max(right.memData[0], minMem)
	rightMemMax := max(right.memData[len(right.memData)-1], minMem)

	return leq(leftCPUMin, allowedVariance*rightCPUMin) &&
		leq(rightCPUMin, allowedVariance*leftCPUMin) &&
		leq(leftCPUMax, allowedVariance*rightCPUMax) &&
		leq(rightCPUMax, allowedVariance*leftCPUMax) &&
		leq(float64(leftMemMin), allowedVariance*float64(rightMemMin)) &&
		leq(float64(rightMemMin), allowedVariance*float64(leftMemMin)) &&
		leq(float64(leftMemMax), allowedVariance*float64(rightMemMax)) &&
		leq(float64(rightMemMax), allowedVariance*float64(leftMemMax))
}

// Pivoting the data from percentile -> container to container_kind -> percentile
func computeResourceAggregates(data *e2e.ResourceUsageSummary) map[string][]percentileUsageData {
	aggregates := make(map[string][]percentileUsageData)
	var sortedPercentiles []string
	for percentile := range *data {
		sortedPercentiles = append(sortedPercentiles, percentile)
	}
	sort.Strings(sortedPercentiles)
	for _, percentile := range sortedPercentiles {
		for i := range (*data)[percentile] {
			name := getContainerKind((*data)[percentile][i].Name)
			aggregate, ok := aggregates[name]
			if !ok || aggregate[len(aggregate)-1].percentile != percentile {
				aggregates[name] = append(aggregates[name],
					percentileUsageData{percentile: percentile})
			}
			aggregates[name][len(aggregates[name])-1].cpuData = append(aggregates[name][len(aggregates[name])-1].cpuData, (*data)[percentile][i].Cpu)
			aggregates[name][len(aggregates[name])-1].memData = append(aggregates[name][len(aggregates[name])-1].memData, (*data)[percentile][i].Mem)
		}
	}
	return aggregates
}

// CompareResourceUsages given two summaries compares the data in them and returns set of
// offending values. Precise semantics is that for each container type, identified by the container
// name we check if minimal and maximal values for both CPU and memory usage are roughly the same.
func CompareResourceUsages(left *e2e.ResourceUsageSummary, right *e2e.ResourceUsageSummary) ViolatingResourceUsageData {
	result := make(ViolatingResourceUsageData)
	if left == nil || right == nil {
		glog.Warningf("At least one of received data is nil:\nleft: %p\nright:%p", left, right)
		return result
	}
	leftAggregates := computeResourceAggregates(left)
	rightAggregates := computeResourceAggregates(right)

	for container := range leftAggregates {
		if _, ok := rightAggregates[container]; !ok {
			glog.V(4).Infof("Missing results for container %v on right-hand side.", container)
			continue
		}
		j := 0
		for i := range leftAggregates[container] {
			// Left and right data might contain different percentiles, hence we need to look for
			// appropriate one.
			for j < len(rightAggregates[container]) && rightAggregates[container][j].percentile < leftAggregates[container][i].percentile {
				j++
			}
			if j >= len(rightAggregates[container]) || rightAggregates[container][j].percentile != leftAggregates[container][i].percentile {
				glog.V(4).Infof("Right-hand data for %v missing percentile: %v, skipping", container, leftAggregates[container][i].percentile)
				continue
			}
			if !isResourceUsageSimilarEnough(leftAggregates[container][i], rightAggregates[container][j], float64(100+ResourceUsageVarianceAllowedPercent)/float64(100)) {
				result[container] = ViolatingResourceUsageDataPair{
					percentile:   leftAggregates[container][i].percentile,
					leftCPUData:  leftAggregates[container][i].cpuData,
					rightCPUData: rightAggregates[container][i].cpuData,
					leftMemData:  leftAggregates[container][i].memData,
					rightMemData: rightAggregates[container][i].memData,
				}
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
