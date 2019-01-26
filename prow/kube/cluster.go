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

package kube

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	prowjobclientset "k8s.io/test-infra/prow/client/clientset/versioned"

	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

// loadClusterConfig loads connection configuration
// for the cluster we're deploying to. We prefer to
// use in-cluster configuration if possible, but will
// fall back to using default rules otherwise.
func loadClusterConfig(masterURL, kubeConfig string) (*rest.Config, error) {
	clusterConfig, err := clientcmd.BuildConfigFromFlags(masterURL, kubeConfig)
	if err == nil {
		return clusterConfig, nil
	}

	credentials, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		return nil, fmt.Errorf("could not load credentials from config: %v", err)
	}

	clusterConfig, err = clientcmd.NewDefaultClientConfig(*credentials, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("could not load client configuration: %v", err)
	}
	return clusterConfig, nil
}

// GetKubernetesClient retrieves the Kubernetes cluster
// client from within the cluster
func GetKubernetesClient(masterURL, kubeConfig string) (kubernetes.Interface, error) {
	config, err := loadClusterConfig(masterURL, kubeConfig)
	if err != nil {
		return nil, err
	}

	// generate the client based off of the config
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	logrus.Info("Successfully constructed k8s client")
	return client, nil
}

// GetKubernetesClient retrieves the Kubernetes cluster
// client from within the cluster
func GetProwJobClient(masterURL, kubeConfig string) (prowjobclientset.Interface, error) {
	config, err := loadClusterConfig(masterURL, kubeConfig)
	if err != nil {
		return nil, err
	}

	prowjobClient, err := prowjobclientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	logrus.Info("Successfully constructed k8s client")
	return prowjobClient, nil
}
