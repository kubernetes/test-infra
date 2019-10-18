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
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/yaml"
)

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
