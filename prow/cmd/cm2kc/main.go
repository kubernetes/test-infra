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

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	flag "github.com/spf13/pflag"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/prow/kube"
)

const (
	// defaultInput is the default input source.
	defaultInput = "/dev/stdin"
	// defaultOutput is the default output source.
	defaultOutput = "/dev/stdout"
)

// Cluster represents the information necessary to talk to a Kubernetes master endpoint.
type Cluster struct {
	// The IP address of the cluster's master endpoint.
	Endpoint string `json:"endpoint"`
	// Base64-encoded public cert used by clients to authenticate to the cluster endpoint.
	ClientCertificate []byte `json:"clientCertificate"`
	// Base64-encoded private key used by clients..
	ClientKey []byte `json:"clientKey"`
	// Base64-encoded public certificate that is the root of trust for the cluster.
	ClusterCACertificate []byte `json:"clusterCaCertificate"`
}

// options are the available command-line flags.
type options struct {
	input  string
	output string
}

// parseFlags parses the command-line flags.
func (o *options) parseFlags() {
	flag.StringVarP(&o.input, "input", "i", defaultInput, "Input cluster map file.")
	flag.StringVarP(&o.output, "output", "o", defaultOutput, "Output kubeconfig file.")

	flag.Parse()
}

// printErrAndExit prints an error message to stderr and exits with a status code.
func printErrAndExit(err error, code int) {
	_, _ = fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(code)
}

// unmarshalClusterMap reads a map[string]Cluster in yaml bytes.
func unmarshalClusterMap(data []byte) (map[string]Cluster, error) {
	var raw map[string]Cluster
	if err := yaml.Unmarshal(data, &raw); err != nil {
		// If we failed to unmarshal the multicluster format try the single Cluster format.
		var singleConfig Cluster
		if err := yaml.Unmarshal(data, &singleConfig); err != nil {
			return nil, err
		}
		raw = map[string]Cluster{kube.DefaultClusterAlias: singleConfig}
	}
	return raw, nil
}

// createKubeConfigFromClusterMap creates a standard kube config from a cluster map.
func createKubeConfigFromClusterMap(cm map[string]Cluster) ([]byte, error) {
	config := clientcmdapi.Config{
		APIVersion:     "v1",
		Kind:           "Config",
		Clusters:       []clientcmdapi.NamedCluster{},
		AuthInfos:      []clientcmdapi.NamedAuthInfo{},
		Contexts:       []clientcmdapi.NamedContext{},
		CurrentContext: kube.DefaultClusterAlias,
	}

	names := make([]string, 0, len(cm))

	for k := range cm {
		names = append(names, k)
	}

	sort.Strings(names)

	for _, name := range names {
		config.Clusters = append(config.Clusters, clientcmdapi.NamedCluster{
			Name: name,
			Cluster: clientcmdapi.Cluster{
				Server:                   cm[name].Endpoint,
				CertificateAuthorityData: cm[name].ClusterCACertificate,
			},
		})
		config.AuthInfos = append(config.AuthInfos, clientcmdapi.NamedAuthInfo{
			Name: name,
			AuthInfo: clientcmdapi.AuthInfo{
				ClientCertificateData: cm[name].ClientCertificate,
				ClientKeyData:         cm[name].ClientKey,
			},
		})
		config.Contexts = append(config.Contexts, clientcmdapi.NamedContext{
			Name: name,
			Context: clientcmdapi.Context{
				Cluster:  name,
				AuthInfo: name,
			},
		})
	}

	return yaml.Marshal(config)
}

// main entry point.
func main() {
	var o options

	o.parseFlags()

	in, err := ioutil.ReadFile(o.input)
	if err != nil {
		printErrAndExit(err, 1)
	}

	cm, err := unmarshalClusterMap(in)
	if err != nil {
		printErrAndExit(err, 1)
	}

	kc, err := createKubeConfigFromClusterMap(cm)
	if err != nil {
		printErrAndExit(err, 1)
	}

	dir := filepath.Dir(o.output)
	if err = os.MkdirAll(dir, os.ModePerm); err != nil {
		printErrAndExit(err, 1)
	}

	if err = ioutil.WriteFile(o.output, kc, 0644); err != nil {
		printErrAndExit(err, 1)
	}
}
