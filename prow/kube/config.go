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
	"io/ioutil"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// LoadClusterConfigs loads rest.Configs for creation of clients, by using either a normal
// .kube/config file, a custom `Cluster` file, or both. The configs are returned in a mapping
// of context --> config. The default context is included in this mapping and specified as a
// return vaule. Errors are returned if .kube/config is specified and invalid or if no valid
// contexts are found.
func LoadClusterConfigs(kubeconfig, buildCluster string) (configurations map[string]rest.Config, defaultContext string, err error) {
	logrus.Infof("Loading cluster contexts...")

	loaders := []clusterConfigLoader{inClusterConfigLoader(), kubeconfigConfigLoader(kubeconfig)}
	if buildCluster != "" {
		loaders = append(loaders, buildClusterConfigLoader(buildCluster))
	}
	configs, defCtx, err := aggregateClusterConfigLoader(loaders...)()
	if err != nil {
		return nil, "", fmt.Errorf("failed to load cluster configs: %v", err)
	}

	if len(configs) == 0 {
		return nil, "", errors.New("no clients found")
	}
	return configs, *defCtx, nil
}

// inClusterContext is the context used to denote an in-cluster client
func inClusterContext() *string {
	return new(string)
}

type clusterConfigLoader func() (configurations map[string]rest.Config, defaultContext *string, err error)

// inClusterConfigLoader returns an optimistic loader for in-cluster config
// the config has the in-cluster alias ("") and this will never return an error
// and will not attempt to specify a default context
func inClusterConfigLoader() clusterConfigLoader {
	return pluggableInClusterConfigLoader(rest.InClusterConfig)
}

// pluggableInClusterConfigLoader returns an optimistic loader for in-cluster config
// using the given loading func. This is intended for internal use and testing _only_
func pluggableInClusterConfigLoader(loader func() (*rest.Config, error)) clusterConfigLoader {
	return func() (configurations map[string]rest.Config, defaultContext *string, err error) {
		configurations = map[string]rest.Config{}
		if localCfg, err := loader(); err != nil {
			logrus.WithError(err).Warn("failed to create in-cluster config")
		} else {
			logrus.WithField("context", "").Info("loaded in-cluster config")
			configurations[*inClusterContext()] = *localCfg
		}
		return configurations, nil, nil
	}
}

// kubeconfigConfigLoader returns a loader for build cluster configurations from a
// kube.config file. This loader will attempt to provide a default context
func kubeconfigConfigLoader(kubeconfig string) clusterConfigLoader {
	return func() (configurations map[string]rest.Config, defaultContext *string, err error) {
		configurations = map[string]rest.Config{}
		var loader clientcmd.ClientConfigLoader
		if kubeconfig != "" { // load from --kubeconfig
			loader = &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
		} else {
			loader = clientcmd.NewDefaultClientConfigLoadingRules()
		}

		cfg, err := loader.Load()
		switch {
		case err != nil && kubeconfig != "":
			return nil, nil, fmt.Errorf("error loading kubeconfig at %s: %v", kubeconfig, err)
		case err != nil:
			logrus.WithError(err).Warn("failed to load any kubeconfig files")
		default:
			_, currentContextHasConfig := cfg.Contexts[cfg.CurrentContext]
			if cfg.CurrentContext != "" && currentContextHasConfig {
				defaultContext = &cfg.CurrentContext
				logrus.WithField("context", defaultContext).Info("setting default context from current context in kubeconfig")
			}

			for context := range cfg.Contexts {
				contextCfg, err := clientcmd.NewNonInteractiveClientConfig(*cfg, context, &clientcmd.ConfigOverrides{}, loader).ClientConfig()
				if err != nil {
					return nil, nil, fmt.Errorf("failed to create client for context %s: %v", context, err)
				}
				logrus.WithField("context", context).Info("loaded build cluster config from kubeconfig")
				configurations[context] = *contextCfg
			}
		}
		return configurations, defaultContext, nil
	}
}

// buildClusterConfigLoader returns a loader for build cluster configurations from a
// clusters.yaml file, where cluster contexts are the aliases given in that file.
// This loader will not attempt to specify a default context
func buildClusterConfigLoader(buildCluster string) clusterConfigLoader {
	return func() (configurations map[string]rest.Config, defaultContext *string, err error) {
		configurations = map[string]rest.Config{}
		data, err := ioutil.ReadFile(buildCluster)
		if err != nil {
			return nil, nil, fmt.Errorf("read build clusters: %v", err)
		}
		raw, err := UnmarshalClusterMap(data)
		if err != nil {
			return nil, nil, fmt.Errorf("unmarshal build clusters: %v", err)
		}
		cfg := &clientcmdapi.Config{
			Clusters:  map[string]*clientcmdapi.Cluster{},
			AuthInfos: map[string]*clientcmdapi.AuthInfo{},
			Contexts:  map[string]*clientcmdapi.Context{},
		}
		for alias, config := range raw {
			fmt.Printf("working on alias %q\n", alias)
			cfg.Clusters[alias] = &clientcmdapi.Cluster{
				Server:                   config.Endpoint,
				CertificateAuthorityData: config.ClusterCACertificate,
			}
			cfg.AuthInfos[alias] = &clientcmdapi.AuthInfo{
				ClientCertificateData: config.ClientCertificate,
				ClientKeyData:         config.ClientKey,
			}
			cfg.Contexts[alias] = &clientcmdapi.Context{
				Cluster:  alias,
				AuthInfo: alias,
				// TODO(fejta): Namespace?
			}
		}
		for context := range cfg.Contexts {
			fmt.Printf("working on context %q\n", context)
			contextCfg, err := clientcmd.NewNonInteractiveClientConfig(*cfg, context, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create client for context %s: %v", context, err)
			}
			logrus.WithField("context", context).Info("loaded build cluster config from cluster YAML")
			configurations[context] = *contextCfg
		}
		return configurations, nil, nil
	}
}

func aggregateClusterConfigLoader(loaders ...clusterConfigLoader) clusterConfigLoader {
	return func() (configurations map[string]rest.Config, defaultContext *string, err error) {
		configurations = map[string]rest.Config{}
		for _, loader := range loaders {
			configurationSubset, localDefault, loadErr := loader()
			if loadErr != nil {
				return nil, nil, fmt.Errorf("failed to load cluster configurations: %v", loadErr)
			}
			for context, config := range configurationSubset {
				if _, alreadyPresent := configurations[context]; alreadyPresent {
					return nil, nil, fmt.Errorf("failed to load cluster configurations, context %q provided twice", context)
				}
				configurations[context] = config
			}
			if localDefault != nil {
				if defaultContext != nil {
					return nil, nil, fmt.Errorf("failed to load cluster configurations, default context provided twice (%q, %q)", *defaultContext, *localDefault)
				}
				defaultContext = localDefault
			}
		}
		// if no default context was set, we should use the in-cluster context
		if defaultContext == nil {
			if _, inClusterConfigExists := configurations[*inClusterContext()]; !inClusterConfigExists {
				return nil, nil, errors.New("failed to load cluster configurations, no default context provided and no in-cluster config loaded")
			}
			defaultContext = inClusterContext()
		}
		return configurations, defaultContext, nil
	}
}
