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

package config

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"k8s.io/contrib/kubelet-to-gcm/monitor"
)

const (
	gceMetaDataEndpoint = "http://169.254.169.254"
	gceMetaDataPrefix   = "/computeMetadata/v1"
)

// NewConfigs returns the SourceConfigs for all monitored endpoints, and
// hits the GCE Metadata server if required.
func NewConfigs(zone, projectID, cluster, host string, kubeletPort, ctrlPort uint, resolution time.Duration) (*monitor.SourceConfig, *monitor.SourceConfig, error) {
	zone, err := getZone(zone)
	if err != nil {
		return nil, nil, err
	}

	projectID, err = getProjectID(projectID)
	if err != nil {
		return nil, nil, err
	}

	cluster, err = getCluster(cluster)
	if err != nil {
		return nil, nil, err
	}

	host, err = getKubeletHost(host)
	if err != nil {
		return nil, nil, err
	}

	return &monitor.SourceConfig{
			Zone:       zone,
			Project:    projectID,
			Cluster:    cluster,
			Host:       host,
			Port:       kubeletPort,
			Resolution: resolution,
		}, &monitor.SourceConfig{
			Zone:       zone,
			Project:    projectID,
			Cluster:    cluster,
			Host:       host,
			Port:       ctrlPort,
			Resolution: resolution,
		}, nil
}

// metaDataURI returns the full URI for the desired resource
func metaDataURI(resource string) string {
	return gceMetaDataEndpoint + gceMetaDataPrefix + resource
}

// getGCEMetaData hits the instance's MD server.
func getGCEMetaData(uri string) ([]byte, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to create request %q for GCE metadata: %v", uri, err)
	}
	req.Header.Add("Metadata-Flavor", "Google")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Failed request %q for GCE metadata: %v", uri, err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Failed to read body for request %q for GCE metadata: %v", uri, err)
	}
	return body, nil
}

// getZone returns zone if it's given, or gets it from gce if asked.
func getZone(zone string) (string, error) {
	if zone == "use-gce" {
		body, err := getGCEMetaData(metaDataURI("/instance/zone"))
		if err != nil {
			return "", fmt.Errorf("Failed to get zone from GCE: %v", err)
		}
		tokens := strings.Split(string(body), "/")
		if len(tokens) < 1 {
			return "", fmt.Errorf("Failed to parse GCE response %q for instance zone.", string(body))
		}
		zone = tokens[len(tokens)-1]
	}
	return zone, nil
}

// getProjectID returns projectID if it's given, or gets it from gce if asked.
func getProjectID(projectID string) (string, error) {
	if projectID == "use-gce" {
		body, err := getGCEMetaData(metaDataURI("/project/project-id"))
		if err != nil {
			return "", fmt.Errorf("Failed to get zone from GCE: %v", err)
		}
		projectID = string(body)
	}
	return projectID, nil
}

// getCluster returns the cluster name given, or gets it from gce if asked.
func getCluster(cluster string) (string, error) {
	if cluster == "use-gce" {
		body, err := getGCEMetaData(metaDataURI("/instance/attributes/cluster-name"))
		if err != nil {
			return "", fmt.Errorf("Failed to get cluster name from GCE: %v", err)
		}
		cluster = string(body)
	}
	return cluster, nil
}

// getKubeletHost returns the kubelet host if given, or gets ip of network interface 0 from gce.
func getKubeletHost(kubeletHost string) (string, error) {
	if kubeletHost == "use-gce" {
		body, err := getGCEMetaData(metaDataURI("/instance/network-interfaces/0/ip"))
		if err != nil {
			return "", fmt.Errorf("Failed to get instance IP from GCE: %v", err)
		}
		kubeletHost = string(body)
	}
	return kubeletHost, nil
}
