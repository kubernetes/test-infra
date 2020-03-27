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

// KubernetesClientOptions holds options for interacting with Kubernetes.
type KubernetesClientOptions struct {
	MasterURL  string
	KubeConfig string
}

// AddFlags injects Kubernetes options into the given FlagSet.
func (o *KubernetesClientOptions) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.MasterURL, "masterurl", "", "URL to k8s master")
	fs.StringVar(&o.KubeConfig, "kubeconfig", "", "Cluster config for the cluster you want to connect to")
}

// Validate validates Kubernetes options.
func (o *KubernetesClientOptions) Validate(dryRun bool) error {
	if dryRun && o.MasterURL == "" {
		return errors.New("a dry-run was requested but required flag -masterurl was unset")
	}

	if o.MasterURL != "" {
		if _, err := url.ParseRequestURI(o.MasterURL); err != nil {
			return fmt.Errorf("invalid -masterurl URI: %q", o.MasterURL)
		}
	}
	if o.KubeConfig != "" {
		if _, err := os.Stat(o.KubeConfig); err != nil {
			return err
		}
	}

	return nil
}

// KubeClient returns a Kubernetes client.
func (o *KubernetesClientOptions) KubeClient() (kubernetes.Interface, error) {
	return kube.GetKubernetesClient(o.MasterURL, o.KubeConfig)
}

// ProwJobClient returns a Kubernetes client.
func (o *KubernetesClientOptions) ProwJobClient() (versioned.Interface, error) {
	return kube.GetProwJobClient(o.MasterURL, o.KubeConfig)
}
