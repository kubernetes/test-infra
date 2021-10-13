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
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/test-infra/prow/version"
)

func kubeConfigs(loader clientcmd.ClientConfigLoader) (map[string]rest.Config, string, error) {
	cfg, err := loader.Load()
	if err != nil {
		return nil, "", fmt.Errorf("failed to load: %w", err)
	}
	configs := map[string]rest.Config{}
	for context := range cfg.Contexts {
		contextCfg, err := clientcmd.NewNonInteractiveClientConfig(*cfg, context, &clientcmd.ConfigOverrides{}, loader).ClientConfig()
		if err != nil {
			return nil, "", fmt.Errorf("create %s client: %w", context, err)
		}
		contextCfg.UserAgent = version.UserAgent()
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

// LoadClusterConfigs loads rest.Configs for creation of clients according to the given options.
// Errors are returned if a file/dir is specified in the options and invalid or if no valid contexts are found.
func LoadClusterConfigs(opts *Options) (map[string]rest.Config, error) {

	logrus.Infof("Loading cluster contexts...")
	// This will work if we are running inside kubernetes
	localCfg, err := rest.InClusterConfig()
	if err != nil {
		logrus.WithError(err).Warn("Could not create in-cluster config (expected when running outside the cluster).")
	} else {
		localCfg.UserAgent = version.UserAgent()
	}
	if localCfg != nil && opts.projectedTokenFile != "" {
		localCfg.BearerToken = ""
		localCfg.BearerTokenFile = opts.projectedTokenFile
		logrus.WithField("tokenfile", opts.projectedTokenFile).Info("Using projected token file")
	}

	var candidates []string
	if opts.file != "" {
		candidates = append(candidates, opts.file)
	}
	if opts.dir != "" {
		files, err := ioutil.ReadDir(opts.dir)
		if err != nil {
			return nil, fmt.Errorf("kubecfg dir: %w", err)
		}
		for _, file := range files {
			filename := file.Name()
			if file.IsDir() {
				logrus.WithField("dir", filename).Info("Ignored directory")
				continue
			}
			if strings.HasPrefix(filename, "..") {
				logrus.WithField("filename", filename).Info("Ignored file starting with double dots")
				continue
			}
			candidates = append(candidates, filepath.Join(opts.dir, filename))
		}
	}

	allKubeCfgs := map[string]rest.Config{}
	var currentContext string
	if len(candidates) == 0 {
		// loading from the defaults, e.g., ${KUBECONFIG}
		if allKubeCfgs, currentContext, err = kubeConfigs(clientcmd.NewDefaultClientConfigLoadingRules()); err != nil {
			logrus.WithError(err).Warn("Cannot load kubecfg")
		}
	} else {
		for _, candidate := range candidates {
			logrus.Infof("Loading kubeconfig from: %q", candidate)
			kubeCfgs, tempCurrentContext, err := kubeConfigs(&clientcmd.ClientConfigLoadingRules{ExplicitPath: candidate})
			if err != nil {
				return nil, fmt.Errorf("fail to load kubecfg from %q: %w", candidate, err)
			}
			currentContext = tempCurrentContext
			for c, k := range kubeCfgs {
				if _, ok := allKubeCfgs[c]; ok {
					return nil, fmt.Errorf("context %s occurred more than once in kubeconfig dir %q", c, opts.dir)
				}
				allKubeCfgs[c] = k
			}
		}
	}

	if opts.noInClusterConfig {
		return allKubeCfgs, nil
	}
	return mergeConfigs(localCfg, allKubeCfgs, currentContext)
}

// Options defines how to load kubeconfigs files
type Options struct {
	file               string
	dir                string
	projectedTokenFile string
	noInClusterConfig  bool
}

type ConfigOptions func(*Options)

// ConfigDir configures the directory containing kubeconfig files
func ConfigDir(dir string) ConfigOptions {
	return func(kc *Options) {
		kc.dir = dir
	}
}

// ConfigFile configures the path to a kubeconfig file
func ConfigFile(file string) ConfigOptions {
	return func(kc *Options) {
		kc.file = file
	}
}

// ConfigFile configures the path to a projectedToken file
func ConfigProjectedTokenFile(projectedTokenFile string) ConfigOptions {
	return func(kc *Options) {
		kc.projectedTokenFile = projectedTokenFile
	}
}

// noInClusterConfig indicates that there is no InCluster Config to load
func NoInClusterConfig(noInClusterConfig bool) ConfigOptions {
	return func(kc *Options) {
		kc.noInClusterConfig = noInClusterConfig
	}
}

// NewConfig builds Options according to the given ConfigOptions
func NewConfig(opts ...ConfigOptions) *Options {
	kc := &Options{}
	for _, opt := range opts {
		opt(kc)
	}
	return kc
}
