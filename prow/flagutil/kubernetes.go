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

	"k8s.io/test-infra/prow/kube"
)

// KubernetesOptions holds options for interacting with Kubernetes.
type KubernetesOptions struct {
	cluster    string
	kubeconfig string
	DeckURI    string
}

// AddFlags injects Kubernetes options into the given FlagSet.
func (o *KubernetesOptions) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.cluster, "cluster", "", "Path to kube.Cluster YAML file. If empty, uses the local cluster.")
	fs.StringVar(&o.kubeconfig, "kubeconfig", "", "Path to .kube/config file. If empty, uses the local cluster.")
	fs.StringVar(&o.DeckURI, "deck-url", "", "Deck URI for read-only access to the cluster.")
}

// Validate validates Kubernetes options.
func (o *KubernetesOptions) Validate(dryRun bool) error {
	if dryRun && o.DeckURI == "" {
		return errors.New("a dry-run was requested but required flag -deck-url was unset")
	}

	if o.DeckURI != "" {
		if _, err := url.ParseRequestURI(o.DeckURI); err != nil {
			return fmt.Errorf("invalid -deck-url URI: %q", o.DeckURI)
		}
	}

	return nil
}

// InjectBuildCluster is needed for backwards compatibility for Deck. Remove later.
func (o *KubernetesOptions) InjectBuildCluster(buildCluster string) {
	o.cluster = buildCluster
}

// Client returns a Kubernetes client.
func (o *KubernetesOptions) Client(namespace string, dryRun bool) (*kube.Client, error) {
	if dryRun {
		return kube.NewFakeClient(o.DeckURI), nil
	}

	if o.cluster == "" {
		return kube.NewClientInCluster(namespace)
	}

	return kube.NewClientFromFile(o.cluster, namespace)
}
