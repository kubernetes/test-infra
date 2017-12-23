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

package monitor

import (
	"time"

	log "github.com/golang/glog"
	v3 "google.golang.org/api/monitoring/v3"
)

// SourceConfig is the set of data required to configure a kubernetes
// data source (e.g., kubelet or kube-controller).
type SourceConfig struct {
	Zone, Project, Cluster, Host string
	Port                         uint
	Resolution                   time.Duration
}

// MetricsSource is an object that provides kubernetes metrics in
// Stackdriver format, probably from a backend like the kubelet.
type MetricsSource interface {
	GetTimeSeriesReq() (*v3.CreateTimeSeriesRequest, error)
	Name() string
	ProjectPath() string
}

// Once polls the backend and puts the data to the given service one time.
func Once(src MetricsSource, gcm *v3.Service) {
	req, err := src.GetTimeSeriesReq()
	if err != nil {
		log.Errorf("Failed to create time series request: %v", err)
		return
	}

	// Push that data to GCM's v3 API.
	createCall := gcm.Projects.TimeSeries.Create(src.ProjectPath(), req)
	if empty, err := createCall.Do(); err != nil {
		log.Errorf("Failed to write time series data, empty: %v, err: %v", empty, err)

		jsonReq, err := req.MarshalJSON()
		if err != nil {
			log.Errorf("Failed to marshal time series as JSON")
			return
		}
		log.Errorf("JSON GCM: %s", string(jsonReq[:]))
		return
	}
	log.Infof("Successfully wrote TimeSeries data for %s to GCM v3 API.", src.Name())
}
