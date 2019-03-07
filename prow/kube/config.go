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
	configs := map[string]rest.Config{}
	var defCtx *string
	// This will work if we are running inside kubernetes
	if localCfg, err := rest.InClusterConfig(); err != nil {
		logrus.Warnf("Failed to create in-cluster config: %v", err)
	} else {
		defCtx = new(string)
		logrus.Info("* in-cluster")
		configs[*defCtx] = *localCfg
	}

	// Attempt to load external clusters too
	var loader clientcmd.ClientConfigLoader
	if kubeconfig != "" { // load from --kubeconfig
		loader = &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
	} else {
		loader = clientcmd.NewDefaultClientConfigLoadingRules()
	}

	cfg, err := loader.Load()
	switch {
	case err != nil && kubeconfig != "":
		return nil, "", fmt.Errorf("load %s kubecfg: %v", kubeconfig, err)
	case err != nil:
		logrus.Warnf("failed to load any kubecfg files: %v", err)
	default:
		// normally defCtx is in cluster (""), but we may be a dev running on their workstation
		// in which case rest.InClusterConfig() will fail, so use the current context as default
		// (which is where we look for prowjobs)
		if defCtx == nil && cfg.CurrentContext != "" {
			defCtx = &cfg.CurrentContext
		}

		for context := range cfg.Contexts {
			logrus.Infof("* %s", context)
			contextCfg, err := clientcmd.NewNonInteractiveClientConfig(*cfg, context, &clientcmd.ConfigOverrides{}, loader).ClientConfig()
			if err != nil {
				return nil, "", fmt.Errorf("create %s client: %v", context, err)
			}
			configs[context] = *contextCfg
		}
	}

	if buildCluster != "" { // load from --build-cluster
		data, err := ioutil.ReadFile(buildCluster)
		if err != nil {
			return nil, "", fmt.Errorf("read build clusters: %v", err)
		}
		raw, err := UnmarshalClusterMap(data)
		if err != nil {
			return nil, "", fmt.Errorf("unmarshal build clusters: %v", err)
		}
		cfg = &clientcmdapi.Config{
			Clusters:  map[string]*clientcmdapi.Cluster{},
			AuthInfos: map[string]*clientcmdapi.AuthInfo{},
			Contexts:  map[string]*clientcmdapi.Context{},
		}
		for alias, config := range raw {
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
			logrus.Infof("* %s", context)
			contextCfg, err := clientcmd.NewNonInteractiveClientConfig(*cfg, context, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
			if err != nil {
				return nil, "", fmt.Errorf("create %s client: %v", context, err)
			}
			configs[context] = *contextCfg
		}
	}

	if len(configs) == 0 {
		return nil, "", errors.New("no clients found")
	}
	return configs, *defCtx, nil
}
