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

package kubelet

import (
	"fmt"
	"time"

	v3 "google.golang.org/api/monitoring/v3"
	"k8s.io/contrib/kubelet-to-gcm/monitor"
	"k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/stats"
)

var (
	reservedCoresMD = &metricMetadata{
		MetricKind: "GAUGE",
		ValueType:  "DOUBLE",
		Name:       "container.googleapis.com/container/cpu/reserved_cores",
	}
	usageTimeMD = &metricMetadata{
		MetricKind: "CUMULATIVE",
		ValueType:  "DOUBLE",
		Name:       "container.googleapis.com/container/cpu/usage_time",
	}
	// utilizationMD is currently not pushed, but computed. This will be used
	// when (hopefully not if) that changes.
	utilizationMD = &metricMetadata{
		MetricKind: "GAUGE",
		ValueType:  "DOUBLE",
		Name:       "container.googleapis.com/container/cpu/utilization",
	}
	diskTotalMD = &metricMetadata{
		MetricKind: "GAUGE",
		ValueType:  "INT64",
		Name:       "container.googleapis.com/container/disk/bytes_total",
	}
	diskUsedMD = &metricMetadata{
		MetricKind: "GAUGE",
		ValueType:  "INT64",
		Name:       "container.googleapis.com/container/disk/bytes_used",
	}
	memTotalMD = &metricMetadata{
		MetricKind: "GAUGE",
		ValueType:  "INT64",
		Name:       "container.googleapis.com/container/memory/bytes_total",
	}
	memUsedMD = &metricMetadata{
		MetricKind: "GAUGE",
		ValueType:  "INT64",
		Name:       "container.googleapis.com/container/memory/bytes_used",
	}
	pageFaultsMD = &metricMetadata{
		MetricKind: "CUMULATIVE",
		ValueType:  "INT64",
		Name:       "container.googleapis.com/container/memory/page_fault_count",
	}
	uptimeMD = &metricMetadata{
		MetricKind: "CUMULATIVE",
		ValueType:  "DOUBLE",
		Name:       "container.googleapis.com/container/uptime",
	}

	memUsedNonEvictableLabels = map[string]string{"memory_type": "non-evictable"}
	memUsedEvictableLabels    = map[string]string{"memory_type": "evictable"}
	minorPageFaultLabels      = map[string]string{"fault_type": "minor"}
	majorPageFaultLabels      = map[string]string{"fault_type": "major"}
	noLabels                  = map[string]string{}
)

type metricMetadata struct {
	MetricKind, ValueType, Name string
}

// Translator contains the required information to perform translations from
// kubelet summarys to GCM's GKE metrics.
type Translator struct {
	zone, project, cluster, instanceID string
	resolution                         time.Duration
}

// NewTranslator creates a new Translator with the given fields.
func NewTranslator(zone, project, cluster, instanceID string, resolution time.Duration) *Translator {
	return &Translator{
		zone:       zone,
		project:    project,
		cluster:    cluster,
		instanceID: instanceID,
		resolution: resolution,
	}
}

// Translate translates a summary to its TimeSeries.
func (t *Translator) Translate(summary *stats.Summary) (*v3.CreateTimeSeriesRequest, error) {
	var ts []*v3.TimeSeries
	nodeTs, err := t.translateNode(summary.Node)
	if err != nil {
		return nil, err
	}
	podsTs, err := t.translateContainers(summary.Pods)
	if err != nil {
		return nil, err
	}
	ts = append(ts, nodeTs...)
	ts = append(ts, podsTs...)
	return &v3.CreateTimeSeriesRequest{TimeSeries: ts}, nil
}

func (t *Translator) translateNode(node stats.NodeStats) ([]*v3.TimeSeries, error) {
	var timeSeries []*v3.TimeSeries

	monitoredLabels := map[string]string{
		"project_id":     t.project,
		"cluster_name":   t.cluster,
		"zone":           t.zone,
		"instance_id":    t.instanceID,
		"namespace_id":   "",
		"pod_id":         "machine",
		"container_name": "",
	}
	tsFactory := newTimeSeriesFactory(monitoredLabels, t.resolution)

	// Uptime. This is embedded: there's no nil check.
	now := time.Now()
	uptimePoint := &v3.Point{
		Interval: &v3.TimeInterval{
			EndTime:   now.Format(time.RFC3339),
			StartTime: now.Add(-1 * t.resolution).Format(time.RFC3339),
		},
		Value: &v3.TypedValue{
			DoubleValue: monitor.Float64Ptr(float64(time.Since(node.StartTime.Time).Seconds())),
		},
	}
	timeSeries = append(timeSeries, tsFactory.newTimeSeries(noLabels, uptimeMD, uptimePoint))

	// Memory stats.
	memTS, err := translateMemory(node.Memory, tsFactory)
	if err != nil {
		return nil, err
	}
	timeSeries = append(timeSeries, memTS...)

	// File-system stats.
	fsTS, err := translateFS("/", node.Fs, tsFactory)
	if err != nil {
		return nil, err
	}
	timeSeries = append(timeSeries, fsTS...)

	// CPU stats.
	cpuTS, err := translateCPU(node.CPU, tsFactory)
	if err != nil {
		return nil, err
	}
	timeSeries = append(timeSeries, cpuTS...)

	return timeSeries, nil
}

func (t *Translator) translateContainers(pods []stats.PodStats) ([]*v3.TimeSeries, error) {
	var timeSeries []*v3.TimeSeries
	metricsSeen := make(map[string]time.Time)
	metrics := make(map[string][]*v3.TimeSeries)

	for _, pod := range pods {
		namespace := pod.PodRef.Namespace
		podID := pod.PodRef.Name
		// There can be duplicate data points for containers, so only
		// take the latest one.
		for _, container := range pod.Containers {
			containerName := container.Name
			// Check for duplicates
			if container.StartTime.Time.Before(metricsSeen[containerName]) || container.StartTime.Time.Equal(metricsSeen[containerName]) {
				continue
			}
			metricsSeen[containerName] = container.StartTime.Time
			var containerSeries []*v3.TimeSeries

			monitoredLabels := map[string]string{
				"project_id":     t.project,
				"cluster_name":   t.cluster,
				"zone":           t.zone,
				"instance_id":    t.instanceID,
				"namespace_id":   namespace,
				"pod_id":         podID,
				"container_name": containerName,
			}
			tsFactory := newTimeSeriesFactory(monitoredLabels, t.resolution)

			// Uptime. This is embedded: there's no nil check.
			now := time.Now()
			uptimePoint := &v3.Point{
				Interval: &v3.TimeInterval{
					EndTime:   now.Format(time.RFC3339),
					StartTime: now.Add(-1 * t.resolution).Format(time.RFC3339),
				},
				Value: &v3.TypedValue{
					DoubleValue:     monitor.Float64Ptr(float64(time.Since(container.StartTime.Time).Seconds())),
					ForceSendFields: []string{"DoubleValue"},
				},
			}
			containerSeries = append(containerSeries, tsFactory.newTimeSeries(noLabels, uptimeMD, uptimePoint))

			// Memory stats.
			memTS, err := translateMemory(container.Memory, tsFactory)
			if err != nil {
				return nil, err
			}
			containerSeries = append(containerSeries, memTS...)

			// File-system stats.
			rootfsTS, err := translateFS("/", container.Rootfs, tsFactory)
			if err != nil {
				return nil, err
			}
			containerSeries = append(containerSeries, rootfsTS...)

			logfsTS, err := translateFS("logs", container.Logs, tsFactory)
			if err != nil {
				return nil, err
			}
			containerSeries = append(containerSeries, logfsTS...)

			// CPU stats.
			cpuTS, err := translateCPU(container.CPU, tsFactory)
			if err != nil {
				return nil, err
			}
			containerSeries = append(containerSeries, cpuTS...)

			metrics[containerName] = containerSeries
		}
	}

	// Flatten the deduplicated metrics.
	for _, containerSeries := range metrics {
		timeSeries = append(timeSeries, containerSeries...)
	}
	return timeSeries, nil
}

// translateCPU creates all the TimeSeries for a give CPUStat.
func translateCPU(cpu *stats.CPUStats, tsFactory *timeSeriesFactory) ([]*v3.TimeSeries, error) {
	var timeSeries []*v3.TimeSeries

	// First check that all required information is present.
	if cpu == nil {
		return nil, fmt.Errorf("CPU information missing.")
	}
	if cpu.UsageNanoCores == nil {
		return nil, fmt.Errorf("UsageNanoCores missing from CPUStats %v", cpu)
	}
	if cpu.UsageCoreNanoSeconds == nil {
		return nil, fmt.Errorf("UsageCoreNanoSeconds missing from CPUStats %v", cpu)
	}

	// Total CPU utilization for all time. Convert from nanosec to sec.
	cpuTotalPoint := tsFactory.newPoint(&v3.TypedValue{
		DoubleValue:     monitor.Float64Ptr(float64(*cpu.UsageCoreNanoSeconds) / float64(1000*1000*1000)),
		ForceSendFields: []string{"DoubleValue"},
	}, cpu.Time.Time, usageTimeMD.MetricKind)
	timeSeries = append(timeSeries, tsFactory.newTimeSeries(noLabels, usageTimeMD, cpuTotalPoint))
	return timeSeries, nil
}

// translateFS creates all the TimeSeries for a given FsStats and volume name.
func translateFS(volume string, fs *stats.FsStats, tsFactory *timeSeriesFactory) ([]*v3.TimeSeries, error) {
	var timeSeries []*v3.TimeSeries

	// First, check that we've been given all the data we need.
	if fs == nil {
		return nil, fmt.Errorf("File-system information missing.")
	}
	if fs.CapacityBytes == nil {
		return nil, fmt.Errorf("CapacityBytes is missing from FsStats %v", fs)
	}
	if fs.UsedBytes == nil {
		return nil, fmt.Errorf("UsedBytes is missing from FsStats %v", fs)
	}

	// For some reason the Kubelet doesn't return when this sample is from,
	// so we'll use now.
	now := time.Now()

	resourceLabels := map[string]string{"device_name": volume}
	// Total disk available.
	diskTotalPoint := tsFactory.newPoint(&v3.TypedValue{
		Int64Value:      monitor.Int64Ptr(int64(*fs.CapacityBytes)),
		ForceSendFields: []string{"Int64Value"},
	}, now, diskTotalMD.MetricKind)
	timeSeries = append(timeSeries, tsFactory.newTimeSeries(resourceLabels, diskTotalMD, diskTotalPoint))

	// Total disk used.
	diskUsedPoint := tsFactory.newPoint(&v3.TypedValue{
		Int64Value:      monitor.Int64Ptr(int64(*fs.UsedBytes)),
		ForceSendFields: []string{"Int64Value"},
	}, now, diskUsedMD.MetricKind)
	timeSeries = append(timeSeries, tsFactory.newTimeSeries(resourceLabels, diskUsedMD, diskUsedPoint))
	return timeSeries, nil
}

// translateMemory creates all the TimeSeries for a given MemoryStats.
func translateMemory(memory *stats.MemoryStats, tsFactory *timeSeriesFactory) ([]*v3.TimeSeries, error) {
	var timeSeries []*v3.TimeSeries

	// First, check that we've been given all the data we need.
	if memory == nil {
		return nil, fmt.Errorf("Memory information missing.")
	}
	if memory.MajorPageFaults == nil {
		return nil, fmt.Errorf("MajorPageFaults missing in MemoryStats %v", memory)
	}
	if memory.PageFaults == nil {
		return nil, fmt.Errorf("PageFaults missing in MemoryStats %v", memory)
	}
	if memory.WorkingSetBytes == nil {
		return nil, fmt.Errorf("WorkingSetBytes information missing in MemoryStats %v", memory)
	}
	if memory.UsageBytes == nil {
		return nil, fmt.Errorf("UsageBytes information missing in MemoryStats %v", memory)
	}

	// Major page faults.
	majorPFPoint := tsFactory.newPoint(&v3.TypedValue{
		Int64Value:      monitor.Int64Ptr(int64(*memory.MajorPageFaults)),
		ForceSendFields: []string{"Int64Value"},
	}, memory.Time.Time, pageFaultsMD.MetricKind)
	timeSeries = append(timeSeries, tsFactory.newTimeSeries(majorPageFaultLabels, pageFaultsMD, majorPFPoint))
	// Minor page faults.
	minorPFPoint := tsFactory.newPoint(&v3.TypedValue{
		Int64Value:      monitor.Int64Ptr(int64(*memory.PageFaults - *memory.MajorPageFaults)),
		ForceSendFields: []string{"Int64Value"},
	}, memory.Time.Time, pageFaultsMD.MetricKind)
	timeSeries = append(timeSeries, tsFactory.newTimeSeries(minorPageFaultLabels, pageFaultsMD, minorPFPoint))

	// Non-evictable memory.
	nonEvictMemPoint := tsFactory.newPoint(&v3.TypedValue{
		Int64Value:      monitor.Int64Ptr(int64(*memory.WorkingSetBytes)),
		ForceSendFields: []string{"Int64Value"},
	}, memory.Time.Time, memUsedMD.MetricKind)
	timeSeries = append(timeSeries, tsFactory.newTimeSeries(memUsedNonEvictableLabels, memUsedMD, nonEvictMemPoint))
	// Evictable memory.
	evictMemPoint := tsFactory.newPoint(&v3.TypedValue{
		Int64Value:      monitor.Int64Ptr(int64(*memory.UsageBytes - *memory.WorkingSetBytes)),
		ForceSendFields: []string{"Int64Value"},
	}, memory.Time.Time, memUsedMD.MetricKind)
	timeSeries = append(timeSeries, tsFactory.newTimeSeries(memUsedEvictableLabels, memUsedMD, evictMemPoint))

	// Available memory. This may or may not be present, so don't fail if it's absent.
	if memory.AvailableBytes != nil {
		availableMemPoint := tsFactory.newPoint(&v3.TypedValue{
			Int64Value:      monitor.Int64Ptr(int64(*memory.AvailableBytes)),
			ForceSendFields: []string{"Int64Value"},
		}, memory.Time.Time, memTotalMD.MetricKind)
		timeSeries = append(timeSeries, tsFactory.newTimeSeries(noLabels, memTotalMD, availableMemPoint))
	}
	return timeSeries, nil
}

type timeSeriesFactory struct {
	resolution      time.Duration
	monitoredLabels map[string]string
}

func newTimeSeriesFactory(monitoredLabels map[string]string, resolution time.Duration) *timeSeriesFactory {
	return &timeSeriesFactory{
		resolution:      resolution,
		monitoredLabels: monitoredLabels,
	}
}

func (t *timeSeriesFactory) newPoint(val *v3.TypedValue, sampleTime time.Time, metricKind string) *v3.Point {
	start := sampleTime.Add(-1 * t.resolution)
	if metricKind == "GAUGE" {
		start = sampleTime
	}
	return &v3.Point{
		Interval: &v3.TimeInterval{
			EndTime:   sampleTime.Format(time.RFC3339),
			StartTime: start.Format(time.RFC3339),
		},
		Value: val,
	}
}

func (t *timeSeriesFactory) newTimeSeries(metricLabels map[string]string, metadata *metricMetadata, point *v3.Point) *v3.TimeSeries {
	return &v3.TimeSeries{
		Metric: &v3.Metric{
			Labels: metricLabels,
			Type:   metadata.Name,
		},
		MetricKind: metadata.MetricKind,
		ValueType:  metadata.ValueType,
		Resource: &v3.MonitoredResource{
			Labels: t.monitoredLabels,
			Type:   "gke_container",
		},
		Points: []*v3.Point{point},
	}
}
