/*
Copyright 2019 The Kubernetes Authors.

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

package kubeconfig

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/yaml"
)

// NewKubeClient creates a new kube client for interacting with the cluster
func NewKubeClient(contextName string) (*kubernetes.Clientset, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{CurrentContext: contextName}
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeconfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

// getServerAddress gets the address of the kubernetes cluster.
func getServerAddress(clientset kubernetes.Interface) string {
	url := clientset.Discovery().RESTClient().Get().URL()
	return fmt.Sprintf("%s://%s", url.Scheme, url.Host)
}

// CreateKubeConfig creates a standard kube config.
func CreateKubeConfig(clientset kubernetes.Interface, name string, caPEM []byte, authInfo clientcmdapi.AuthInfo) ([]byte, error) {
	config := clientcmdapi.Config{
		APIVersion: "v1",
		Kind:       "Config",
		Clusters: []clientcmdapi.NamedCluster{
			{
				Name: name,
				Cluster: clientcmdapi.Cluster{
					Server:                   getServerAddress(clientset),
					CertificateAuthorityData: caPEM,
				},
			},
		},
		AuthInfos: []clientcmdapi.NamedAuthInfo{
			{
				Name:     name,
				AuthInfo: authInfo,
			},
		},
		Contexts: []clientcmdapi.NamedContext{
			{
				Name: name,
				Context: clientcmdapi.Context{
					Cluster:  name,
					AuthInfo: name,
				},
			},
		},
		CurrentContext: name,
	}

	configYaml, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}

	return configYaml, nil
}

// MergeKubeConfig merges two kube configs into a standard kube config.
func MergeKubeConfig(a, b clientcmdapi.Config) clientcmdapi.Config {
	var res clientcmdapi.Config
	res = a
	res.Clusters = append(res.Clusters, b.Clusters...)
	res.AuthInfos = append(res.AuthInfos, b.AuthInfos...)
	res.Contexts = append(res.Contexts, b.Contexts...)

	return res
}
