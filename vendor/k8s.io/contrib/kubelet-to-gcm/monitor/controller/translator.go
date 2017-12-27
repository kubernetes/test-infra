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

package controller

import (
	"time"

	v3 "google.golang.org/api/monitoring/v3"
	"k8s.io/contrib/kubelet-to-gcm/monitor"
)

var (
	noLabels = map[string]string{}
)

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

// Translate a summary to its TimeSeries.
func (t *Translator) Translate(metrics *Metrics) (*v3.CreateTimeSeriesRequest, error) {
	var ts []*v3.TimeSeries
	evictionTS := t.translateEviction(metrics)
	ts = append(ts, evictionTS)
	return &v3.CreateTimeSeriesRequest{TimeSeries: ts}, nil
}

// translateEviction give a GCM v3 TimeSeries for the node_eviction_count metric.
func (t *Translator) translateEviction(metrics *Metrics) *v3.TimeSeries {
	monitoredLabels := map[string]string{
		"project_id":     t.project,
		"cluster_name":   t.cluster,
		"zone":           t.zone,
		"instance_id":    t.instanceID,
		"namespace_id":   "",
		"pod_id":         "machine",
		"container_name": "",
	}

	now := time.Now().Format(time.RFC3339)
	createTime := time.Unix(metrics.CreateTime, 0).Format(time.RFC3339)

	point := &v3.Point{
		Interval: &v3.TimeInterval{
			StartTime: createTime,
			EndTime:   now,
		},
		Value: &v3.TypedValue{
			Int64Value:      monitor.Int64Ptr(metrics.NodeEvictions),
			ForceSendFields: []string{"Int64Value"},
		},
	}

	return &v3.TimeSeries{
		Metric: &v3.Metric{
			Labels: noLabels,
			Type:   "container.googleapis.com/master/node_controller/node_eviction_count",
		},
		MetricKind: "CUMULATIVE",
		ValueType:  "INT64",
		Resource: &v3.MonitoredResource{
			Labels: monitoredLabels,
			Type:   "gke_container",
		},
		Points: []*v3.Point{point},
	}
}
