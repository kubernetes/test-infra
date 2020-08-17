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

package flagutil

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	prow "k8s.io/test-infra/prow/client/clientset/versioned"
	prowv1 "k8s.io/test-infra/prow/client/clientset/versioned/typed/prowjobs/v1"
	"k8s.io/test-infra/prow/kube"
)

// KubernetesOptions holds options for interacting with Kubernetes.
// These options are both useful for clients interacting with ProwJobs
// and other resources on the infrastructure cluster, as well as Pods
// on build clusters.
type KubernetesOptions struct {
	buildCluster string
	kubeconfig   string

	DeckURI string

	// from resolution
	resolved                    bool
	dryRun                      bool
	prowJobClientset            prow.Interface
	clusterConfigs              map[string]rest.Config
	kubernetesClientsByContext  map[string]kubernetes.Interface
	infrastructureClusterConfig *rest.Config
}

// AddFlags injects Kubernetes options into the given FlagSet.
func (o *KubernetesOptions) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.buildCluster, "build-cluster", "", "Path to kube.Cluster YAML file. If empty, uses the local cluster. All clusters are used as build clusters. Cannot be combined with --kubeconfig.")
	fs.StringVar(&o.kubeconfig, "kubeconfig", "", "Path to .kube/config file. If empty, uses the local cluster. All contexts other than the default are used as build clusters. Cannot be combined with --build-cluster.")
	fs.StringVar(&o.DeckURI, "deck-url", "", "Deck URI for read-only access to the infrastructure cluster.")
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

	if o.kubeconfig != "" {
		if _, err := os.Stat(o.kubeconfig); err != nil {
			return fmt.Errorf("error accessing --kubeconfig: %v", err)
		}
	}

	if o.kubeconfig != "" && o.buildCluster != "" {
		return errors.New("must provide only --build-cluster OR --kubeconfig")
	}

	return nil
}

// resolve loads all of the clients we need and caches them for future calls.
func (o *KubernetesOptions) resolve(dryRun bool) error {
	if o.resolved {
		return nil
	}

	o.dryRun = dryRun
	if dryRun {
		return nil
	}

	clusterConfigs, err := kube.LoadClusterConfigs(o.kubeconfig, o.buildCluster)
	if err != nil {
		return fmt.Errorf("load --kubeconfig=%q --build-cluster=%q configs: %v", o.kubeconfig, o.buildCluster, err)
	}
	o.clusterConfigs = clusterConfigs

	clients := map[string]kubernetes.Interface{}
	for context, config := range clusterConfigs {
		client, err := kubernetes.NewForConfig(&config)
		if err != nil {
			return fmt.Errorf("create %s kubernetes client: %v", context, err)
		}
		clients[context] = client
	}

	localCfg := clusterConfigs[kube.InClusterContext]
	o.infrastructureClusterConfig = &localCfg
	pjClient, err := prow.NewForConfig(&localCfg)
	if err != nil {
		return err
	}

	o.prowJobClientset = pjClient
	o.kubernetesClientsByContext = clients
	o.resolved = true

	return nil
}

// ProwJobClientset returns a ProwJob clientset for use in informer factories.
func (o *KubernetesOptions) ProwJobClientset(namespace string, dryRun bool) (prowJobClientset prow.Interface, err error) {
	if err := o.resolve(dryRun); err != nil {
		return nil, err
	}

	if o.dryRun {
		return nil, errors.New("no dry-run prowjob clientset is supported in dry-run mode")
	}

	return o.prowJobClientset, nil
}

// ProwJobClient returns a ProwJob client.
func (o *KubernetesOptions) ProwJobClient(namespace string, dryRun bool) (prowJobClient prowv1.ProwJobInterface, err error) {
	if err := o.resolve(dryRun); err != nil {
		return nil, err
	}

	if o.dryRun {
		return kube.NewDryRunProwJobClient(o.DeckURI), nil
	}

	return o.prowJobClientset.ProwV1().ProwJobs(namespace), nil
}

// InfrastructureClusterConfig returns the *rest.Config for the infrastructure cluster
func (o *KubernetesOptions) InfrastructureClusterConfig(dryRun bool) (*rest.Config, error) {
	if err := o.resolve(dryRun); err != nil {
		return nil, err
	}

	return o.infrastructureClusterConfig, nil
}

// InfrastructureClusterClient returns a Kubernetes client for the infrastructure cluster.
func (o *KubernetesOptions) InfrastructureClusterClient(dryRun bool) (kubernetesClient kubernetes.Interface, err error) {
	return o.ClusterClientForContext(kube.InClusterContext, dryRun)
}

// ClusterClientForContext returns a Kubernetes client for the given context name.
func (o *KubernetesOptions) ClusterClientForContext(context string, dryRun bool) (kubernetesClient kubernetes.Interface, err error) {
	if err := o.resolve(dryRun); err != nil {
		return nil, err
	}

	if o.dryRun {
		return nil, errors.New("no dry-run kubernetes client is supported in dry-run mode")
	}

	client, exists := o.kubernetesClientsByContext[context]
	if !exists {
		return nil, fmt.Errorf("context %q does not exist in the provided config", context)
	}
	return client, nil
}

// BuildClusterClients returns Pod clients for build clusters.
func (o *KubernetesOptions) BuildClusterClients(namespace string, dryRun bool) (buildClusterClients map[string]corev1.PodInterface, err error) {
	if err := o.resolve(dryRun); err != nil {
		return nil, err
	}

	if o.dryRun {
		return nil, errors.New("no dry-run pod client is supported for build clusters in dry-run mode")
	}

	buildClients := map[string]corev1.PodInterface{}
	for context, client := range o.kubernetesClientsByContext {
		buildClients[context] = client.CoreV1().Pods(namespace)
	}
	return buildClients, nil
}

// BuildClusterCoreV1Clients returns core v1 clients for build clusters.
func (o *KubernetesOptions) BuildClusterCoreV1Clients(dryRun bool) (v1Clients map[string]corev1.CoreV1Interface, err error) {
	if err := o.resolve(dryRun); err != nil {
		return nil, err
	}

	if o.dryRun {
		return nil, errors.New("no dry-run pod client is supported for build clusters in dry-run mode")
	}

	clients := map[string]corev1.CoreV1Interface{}
	for context, client := range o.kubernetesClientsByContext {
		clients[context] = client.CoreV1()
	}
	return clients, nil
}

// BuildClusterManagers returns a manager per buildCluster.
// Per default, LeaderElection and the metrics listener are disabled, as we assume
// that there is another manager for ProwJobs that handles that.
func (o *KubernetesOptions) BuildClusterManagers(dryRun bool, opts ...func(*manager.Options)) (map[string]manager.Manager, error) {
	if err := o.resolve(dryRun); err != nil {
		return nil, err
	}
	if o.dryRun {
		// TODO: Can be supported after bumping c-r to 0.6.0, ref:
		// https://github.com/kubernetes-sigs/controller-runtime/pull/839
		return nil, errors.New("dry-run is currently not supported")
	}

	options := manager.Options{
		LeaderElection:     false,
		MetricsBindAddress: "0",
	}
	for _, opt := range opts {
		opt(&options)
	}

	res := map[string]manager.Manager{}
	var errs []error
	for buildCluserName, buildClusterConfig := range o.clusterConfigs {
		// We pass a pointer, need to capture it here. Dragons will fall if this is changed.
		cfg := buildClusterConfig
		mgr, err := manager.New(&cfg, options)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to construct manager for cluster %s: %w", buildCluserName, err))
		}
		res[buildCluserName] = mgr
	}

	return res, utilerrors.NewAggregate(errs)
}

// BuildClusterUncachedRuntimeClients returns ctrlruntimeclients for the build cluster in a non-caching implementation.
func (o *KubernetesOptions) BuildClusterUncachedRuntimeClients(dryRun bool) (map[string]ctrlruntimeclient.Client, error) {
	if err := o.resolve(dryRun); err != nil {
		return nil, err
	}

	if o.dryRun {
		return nil, errors.New("no dry-run pod client is supported for build clusters in dry-run mode")
	}

	clients := map[string]ctrlruntimeclient.Client{}
	for name := range o.clusterConfigs {
		cfg := o.clusterConfigs[name]
		client, err := ctrlruntimeclient.New(&cfg, ctrlruntimeclient.Options{})
		if err != nil {
			return nil, fmt.Errorf("failed to construct client for cluster %q: %v", name, err)
		}
		clients[name] = client
	}

	return clients, nil
}
