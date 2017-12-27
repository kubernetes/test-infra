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
	"fmt"
	"net/http"

	"k8s.io/contrib/kubelet-to-gcm/monitor"

	v3 "google.golang.org/api/monitoring/v3"
)

// Source pulls data from the controller source, and translates it for GCMv3.
type Source struct {
	translator  *Translator
	client      *Client
	projectPath string
}

// NewSource creates a new Source for a kube-controller.
func NewSource(cfg *monitor.SourceConfig) (*Source, error) {
	// Create objects for controller monitoring.
	trans := NewTranslator(cfg.Zone, cfg.Project, cfg.Cluster, cfg.Host, cfg.Resolution)

	// NewClient validates its own inputs.
	client, err := NewClient(cfg.Host, cfg.Port, &http.Client{})
	if err != nil {
		return nil, fmt.Errorf("Failed to create a controller client with config %v: %v", cfg, err)
	}

	return &Source{
		translator:  trans,
		client:      client,
		projectPath: fmt.Sprintf("projects/%s", cfg.Project),
	}, nil
}

// GetTimeSeriesReq returns the GCM v3 TimeSeries data.
func (s *Source) GetTimeSeriesReq() (*v3.CreateTimeSeriesRequest, error) {
	// Get the latest summary.
	metrics, err := s.client.GetMetrics()
	if err != nil {
		return nil, fmt.Errorf("Failed to get metrics from controller: %v", err)
	}

	// Translate kubelet's data to GCM v3's format.
	tsReq, err := s.translator.Translate(metrics)
	if err != nil {
		return nil, fmt.Errorf("Failed to translate data from controller metrics %v: %v", metrics, err)
	}

	return tsReq, nil
}

// Name returns the name of the component being monitored.
func (s *Source) Name() string {
	return "kube-controller-manager"
}

// ProjectPath returns the project's path in a way Stackdriver understands.
func (s *Source) ProjectPath() string {
	return s.projectPath
}
