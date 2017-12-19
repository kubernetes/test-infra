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

	"k8s.io/kubernetes/pkg/metrics"
	"k8s.io/kubernetes/test/e2e"

	"github.com/daviddengcn/go-colortext"
	"github.com/golang/glog"
	"github.com/prometheus/common/model"
)

const (
	// MetricsVarianceAllowedPercent specifies how much variance we allow between results.
	// Precise semantics is that we say that difference is too big if greater value is more than
	// MetricsVarianceAllowedPercent bigger than the smaller one. I.e. if X < Y, then Y is too
	// big if Y > X + X*(MetricsVarianceAllowedPercent/100).
	MetricsVarianceAllowedPercent = 50
)

type flattenedSample map[string]map[string]float64

type kubeletSample struct {
	node  string
	value float64
}

type kubeletSampleArr []kubeletSample

func (a kubeletSampleArr) Len() int           { return len(a) }
func (a kubeletSampleArr) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a kubeletSampleArr) Less(i, j int) bool { return a[i].value < a[j].value }

type aggregatedKubeletSamples map[string]map[string]kubeletSampleArr

// ViolatingMetric stores a pair of metrics which proves that the difference between them is too big
// Contains an info about the source component for those metrics, and all their labels.
type ViolatingMetric struct {
	labels      string
	component   string
	left, right float64
}

// ViolatingMetricsArr stores offending metrics keyed by the metric name.
type ViolatingMetricsArr map[string][]ViolatingMetric

// PrintToStdout prints offending data to the Stdout in a human readable format.
func (d *ViolatingMetricsArr) PrintToStdout(leftBuild, rightBuild int, enableOutputColoring bool) {
	writer := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	allowedVariance := float64(100+MetricsVarianceAllowedPercent) / float64(100)
	for metric, arr := range *d {
		if enableOutputColoring {
			ChangeColor(ct.Green, writer)
		}
		fmt.Fprintf(writer, "%v", metric)
		if enableOutputColoring {
			ResetColor(writer)
		}
		fmt.Fprint(writer, "\nBucket\tComponent\t")
		printBuildNumber(leftBuild, writer, enableOutputColoring)
		fmt.Fprint(writer, "\t")
		printBuildNumber(rightBuild, writer, enableOutputColoring)
		fmt.Fprint(writer, "\n")
		for _, data := range arr {
			fmt.Fprintf(writer, "%v\t%v\t", data.labels, data.component)
			changeColorFloat64AndWrite(data.left, data.right, allowedVariance, enableOutputColoring, writer)
			fmt.Fprint(writer, "\t")
			changeColorFloat64AndWrite(data.right, data.left, allowedVariance, enableOutputColoring, writer)
			fmt.Fprint(writer, "\t\n")
		}
		fmt.Fprint(writer, "\n")
	}
	writer.Flush()
}

func uniformizeMetric(metric model.Metric) string {
	var result []string
	var sortedKeys []string
	for k := range metric {
		sortedKeys = append(sortedKeys, string(k))
	}
	sort.Strings(sortedKeys)
	for _, k := range sortedKeys {
		value := metric[model.LabelName(k)]
		switch k {
		case "__name__":
			continue
		case "client":
			client := value[0:strings.Index(string(value), "/")]
			if client == "kube-controller-manager" {
				client += value[strings.LastIndex(string(value), "/"):]
			}
			result = append(result, fmt.Sprintf("%v=%v", k, client))
		default:
			result = append(result, fmt.Sprintf("%v=%v", k, value))
		}
	}
	return strings.Join(result, ", ")
}

func flattenSamples(data *model.Samples) *flattenedSample {
	if data == nil {
		return nil
	}
	result := make(flattenedSample)
	for i := range *data {
		metricName := (*data)[i].Metric[model.MetricNameLabel]
		if _, ok := result[string(metricName)]; !ok {
			result[string(metricName)] = make(map[string]float64)
		}
		result[string(metricName)][uniformizeMetric((*data)[i].Metric)] = float64((*data)[i].Value)
	}
	return &result
}

func computeKubeletAggregates(data map[string]metrics.KubeletMetrics) *aggregatedKubeletSamples {
	result := make(aggregatedKubeletSamples)
	for name := range data {
		for metricName := range data[name] {
			for i := range data[name][metricName] {
				if _, ok := result[metricName]; !ok {
					result[metricName] = make(map[string]kubeletSampleArr)
				}
				metric := data[name][metricName][i].Metric
				result[metricName][uniformizeMetric(metric)] =
					append(result[metricName][uniformizeMetric(metric)],
						kubeletSample{node: name, value: float64(data[name][metricName][i].Value)})
			}
		}
	}
	return &result
}

func isSimpleMetricSimilarEnough(left float64, right float64, allowedVariance float64) bool {
	// To avoid problems with 0 we assume all values are at least 1
	left = math.Max(left, 1)
	right = math.Max(right, 1)
	// For very small values (1, 2) we always allow double
	if left < 3 || right < 3 {
		allowedVariance = math.Max(allowedVariance, 2)
	}
	return leq(left, right*allowedVariance) && leq(right, left*allowedVariance)
}

// Assumes that left and right are sorted
func isKubeletMetricSimilarEnough(left kubeletSampleArr, right kubeletSampleArr, allowedVariance float64) bool {
	// To avoid problems with 0 we assume all values are at least 1
	leftMin := math.Max(left[0].value, 1)
	leftMax := math.Max(left[len(left)-1].value, 1)
	rightMin := math.Max(right[0].value, 1)
	rightMax := math.Max(right[len(right)-1].value, 1)

	// For very small values (1, 2) we always allow double
	if leftMin < 3 || rightMin < 3 {
		allowedVariance = math.Max(allowedVariance, 2)
	}

	return leq(leftMin, rightMin*allowedVariance) &&
		leq(rightMin, leftMin*allowedVariance) &&
		leq(leftMax, rightMax*allowedVariance) &&
		leq(rightMax, leftMax*allowedVariance)
}

func compareSamples(left *model.Samples, right *model.Samples, component string) ViolatingMetricsArr {
	leftAggregate := flattenSamples(left)
	rightAggregate := flattenSamples(right)
	violatingMetrics := make(ViolatingMetricsArr)
	for metric, la := range *leftAggregate {
		if ra, ok := (*rightAggregate)[metric]; !ok {
			glog.V(4).Infof("Missing metric %v on right-hand side.", metric)
			continue
		} else {
			for bucket, lv := range la {
				if rv, ok := ra[bucket]; !ok {
					glog.V(4).Infof("Missing group \"%v\" for metric %v on right-hand side.", bucket, metric)
					continue
				} else {
					if !isSimpleMetricSimilarEnough(lv, rv, float64(100+MetricsVarianceAllowedPercent)/float64(100)) {
						violatingMetrics[metric] = append(violatingMetrics[metric], ViolatingMetric{
							labels:    bucket,
							component: component,
							left:      lv,
							right:     rv,
						})
					}
				}
			}
		}
	}

	if len(violatingMetrics) == 0 {
		return nil
	}
	return violatingMetrics
}

func compareKubeletMetrics(left map[string]metrics.KubeletMetrics, right map[string]metrics.KubeletMetrics, violating *ViolatingMetricsArr) {
	leftAggregate := computeKubeletAggregates(left)
	rightAggregate := computeKubeletAggregates(right)
	for metric, la := range *leftAggregate {
		if ra, ok := (*rightAggregate)[metric]; !ok {
			glog.V(4).Infof("Missing metric %v on right-hand side.", metric)
			continue
		} else {
			for bucket, lv := range la {
				if rv, ok := ra[bucket]; !ok {
					glog.V(4).Infof("Missing metric %v for group %v on right-hand side.", metric, bucket)
					continue
				} else {
					sort.Sort(lv)
					sort.Sort(rv)
					if !isKubeletMetricSimilarEnough(lv, rv, float64(100+MetricsVarianceAllowedPercent)/float64(100)) {
						(*violating)[metric] = append((*violating)[metric],
							ViolatingMetric{
								labels:    bucket,
								component: fmt.Sprintf("%v#%v#MIN", lv[0].node, rv[0].node),
								left:      lv[0].value,
								right:     rv[0].value,
							},
							ViolatingMetric{
								labels:    bucket,
								component: fmt.Sprintf("%v#%v#MAX", lv[len(lv)-1].node, rv[len(rv)-1].node),
								left:      lv[len(lv)-1].value,
								right:     rv[len(rv)-1].value,
							})
					}
				}
			}
		}
	}
}

func compareSimpleMetrics(left *metrics.Metrics, right *metrics.Metrics, violating *ViolatingMetricsArr, component string) {
	for k := range *left {
		if _, ok := (*right)[k]; !ok {
			glog.V(4).Infof("Missing metric %v on right-hand side.", k)
			continue
		}
		leftCopy := (*left)[k]
		rightCopy := (*right)[k]
		for k, v := range compareSamples(&leftCopy, &rightCopy, component) {
			(*violating)[k] = append((*violating)[k], v...)
		}
	}
}

// CompareMetrics given two summaries compares the data in them and returns set of
// offending values. Precise semantics is that for each metric (identified as a name and set of labels) we check
// if values are roughly the same. For kubelet metrics we check minimal and maximal values over all Kubelets.
func CompareMetrics(left *e2e.MetricsForE2E, right *e2e.MetricsForE2E) ViolatingMetricsArr {
	violatingMetrics := make(ViolatingMetricsArr)
	if left == nil || right == nil {
		glog.Warningf("At least one of received data is nil:\nleft: %p\nright:%p", left, right)
		return ViolatingMetricsArr{}
	}
	compareSimpleMetrics((*metrics.Metrics)(&left.ApiServerMetrics), (*metrics.Metrics)(&right.ApiServerMetrics), &violatingMetrics, "ApiServer")
	compareSimpleMetrics((*metrics.Metrics)(&left.ControllerManagerMetrics), (*metrics.Metrics)(&right.ControllerManagerMetrics), &violatingMetrics, "ControllerManager")
	compareSimpleMetrics((*metrics.Metrics)(&left.SchedulerMetrics), (*metrics.Metrics)(&right.SchedulerMetrics), &violatingMetrics, "Scheduler")
	compareKubeletMetrics(left.KubeletMetrics, right.KubeletMetrics, &violatingMetrics)
	return violatingMetrics
}
