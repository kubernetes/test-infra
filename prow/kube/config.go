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

package kube

import (
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func kubeConfigs(kubeconfig string) (map[string]rest.Config, string, error) {
	// Attempt to load external clusters too
	var loader clientcmd.ClientConfigLoader
	if kubeconfig != "" { // load from --kubeconfig
		loader = &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
	} else {
		loader = clientcmd.NewDefaultClientConfigLoadingRules()
	}

	cfg, err := loader.Load()
	if err != nil && kubeconfig != "" {
		return nil, "", fmt.Errorf("load: %v", err)
	}
	if err != nil {
		logrus.WithError(err).Warn("Cannot load kubecfg")
		return nil, "", nil
	}
	configs := map[string]rest.Config{}
	for context := range cfg.Contexts {
		contextCfg, err := clientcmd.NewNonInteractiveClientConfig(*cfg, context, &clientcmd.ConfigOverrides{}, loader).ClientConfig()
		if err != nil {
			return nil, "", fmt.Errorf("create %s client: %v", context, err)
		}
		configs[context] = *contextCfg
		logrus.Infof("Parsed kubeconfig context: %s", context)
	}
	return configs, cfg.CurrentContext, nil
}

func mergeConfigs(local *rest.Config, foreign map[string]rest.Config, currentContext string) (map[string]rest.Config, error) {
	ret := map[string]rest.Config{}
	for ctx, cfg := range foreign {
		ret[ctx] = cfg
	}
	if local != nil {
		ret[InClusterContext] = *local
	} else if currentContext != "" {
		ret[InClusterContext] = ret[currentContext]
	} else {
		return nil, errors.New("no prow cluster access: in-cluster current kubecfg context required")
	}
	if len(ret) == 0 {
		return nil, errors.New("no client contexts found")
	}
	if _, ok := ret[DefaultClusterAlias]; !ok {
		ret[DefaultClusterAlias] = ret[InClusterContext]
	}
	return ret, nil
}

// LoadClusterConfigs loads rest.Configs for creation of clients, by using a normal
// .kube/config file. The configs are returned in a mapping of context --> config. The default
// context is included in this mapping and specified as a return vaule. Errors are returned if
// .kube/config is specified and invalid or if no valid contexts are found.
func LoadClusterConfigs(kubeconfig, projectedTokenFile string) (map[string]rest.Config, error) {

	logrus.Infof("Loading cluster contexts...")
	// This will work if we are running inside kubernetes
	localCfg, err := rest.InClusterConfig()
	if err != nil {
		logrus.WithError(err).Warn("Could not create in-cluster config (expected when running outside the cluster).")
	}
	if localCfg != nil && projectedTokenFile != "" {
		localCfg.BearerToken = ""
		localCfg.BearerTokenFile = projectedTokenFile
		logrus.WithField("tokenfile", projectedTokenFile).Info("Using projected token file")
	}

	kubeCfgs, currentContext, err := kubeConfigs(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("kubecfg: %v", err)
	}

	return mergeConfigs(localCfg, kubeCfgs, currentContext)
}
