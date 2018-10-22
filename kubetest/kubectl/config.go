/*
Copyright 2018 The Kubernetes Authors.

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

package kubectl

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"

	"k8s.io/test-infra/kubetest/process"
)

// GetConfig gets the current kubectl configuration, parsing the output into a kubeconfig object
func GetConfig(control *process.Control, kubeconfig string) (*Config, error) {
	cmd := "kubectl"
	args := []string{}
	if kubeconfig != "" {
		args = append(args, "--kubeconfig="+kubeconfig)
	}
	args = append(args, "config", "view", "--flatten", "-ojson")

	o, err := control.Output(exec.Command(cmd, args...))
	if err != nil {
		log.Printf("kubectl config view failed: %s\n%s", err, string(o))
		return nil, err
	}

	return parseConfig(o)
}

func parseConfig(b []byte) (*Config, error) {
	k := &Config{}
	if err := json.Unmarshal(b, k); err != nil {
		// The config usually contains credentials, so we don't print it
		return nil, fmt.Errorf("error parsing kubectl config view output: %v", err)
	}

	return k, nil
}

// Config is a simplified version of the v1.Config type
type Config struct {
	Clusters       []ConfigCluster `json:"clusters,omitempty"`
	Contexts       []ConfigContext `json:"contexts,omitempty"`
	CurrentContext string          `json:"current-context"`
	// And more fields that we haven't needed yet
}

// ConfigCluster holds information on a single cluster
type ConfigCluster struct {
	Name string            `json:"name"`
	Info ConfigClusterInfo `json:"cluster"`
}

// ConfigClusterInfo holds detailed information on a cluster
type ConfigClusterInfo struct {
	Server string `json:"server"`
	// And more fields that we haven't needed yet
}

// ConfigContext holds information on a single context
type ConfigContext struct {
	Name string            `json:"name"`
	Info ConfigContextInfo `json:"context"`
}

// ConfigContextInfo holds detailed information on a context
type ConfigContextInfo struct {
	Cluster   string `json:"cluster"`
	User      string `json:"user"`
	Namespace string `json:"namespace"`
}

// CurrentServer returns the server URL from the current context, of (false, nil) if it isn't configured
func (k *Config) CurrentServer() (string, bool) {
	context, found := k.Context(k.CurrentContext)
	if !found {
		return "", false
	}

	cluster, found := k.Cluster(context.Info.Cluster)
	if !found {
		return "", false
	}

	if cluster.Info.Server == "" {
		return "", false
	}

	return cluster.Info.Server, true
}

// Context returns the context with the matching name, or (nil,false) if not found
func (k *Config) Context(name string) (*ConfigContext, bool) {
	for i := range k.Contexts {
		if k.Contexts[i].Name == name {
			return &k.Contexts[i], true
		}
	}
	return nil, false
}

// Cluster returns the cluster with the matching name, or (nil,false) if not found
func (k *Config) Cluster(name string) (*ConfigCluster, bool) {
	for i := range k.Clusters {
		if k.Clusters[i].Name == name {
			return &k.Clusters[i], true
		}
	}
	return nil, false
}
