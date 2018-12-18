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

package flagutil

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/test-infra/prow/client/clientset/versioned"
	"k8s.io/test-infra/prow/kube"
)

// KubernetesOptions holds options for interacting with Kubernetes.
type KubernetesClientOptions struct {
	masterURL  string
	kubeConfig string
}

// AddFlags injects Kubernetes options into the given FlagSet.

func (o *KubernetesClientOptions) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.masterURL, "masterurl", "", "URL to k8s master")
	fs.StringVar(&o.kubeConfig, "kubeconfig", "", "Cluster config for the cluster you want to connect to")
}

// Validate validates Kubernetes options.
func (o *KubernetesClientOptions) Validate(dryRun bool) error {
	if dryRun && o.masterURL == "" {
		return errors.New("a dry-run was requested but required flag -masterurl was unset")
	}

	if o.masterURL != "" {
		if _, err := url.ParseRequestURI(o.masterURL); err != nil {
			return fmt.Errorf("invalid -masterurl URI: %q", o.masterURL)
		}
	}
	if o.kubeConfig != "" {
		if _, err := os.Stat(o.kubeConfig); err != nil {
			return err
		}
	}

	return nil
}

// Client returns a Kubernetes client.
func (o *KubernetesClientOptions) KubeClient() (kubernetes.Interface, error) {
	return kube.GetKubernetesClient(o.masterURL, o.kubeConfig)
}

// Client returns a Kubernetes client.
func (o *KubernetesClientOptions) ProwJobClient() (versioned.Interface, error) {
	return kube.GetProwJobClient(o.masterURL, o.kubeConfig)
}
