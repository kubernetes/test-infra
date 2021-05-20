/*
Copyright 2020 The Kubernetes Authors.

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
	"flag"
	"fmt"

	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/plugins"
)

type PluginOptions struct {
	PluginConfigPath                         string
	PluginConfigPathDefault                  string
	SupplementalPluginsConfigDirs            flagutil.Strings
	SupplementalPluginsConfigsFileNameSuffix string
	CheckUnknownPlugins                      bool
}

func (o *PluginOptions) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.PluginConfigPath, "plugin-config", o.PluginConfigPathDefault, "Path to plugin config file.")
	fs.Var(&o.SupplementalPluginsConfigDirs, "supplemental-plugin-config-dir", "An additional directory from which to load plugin configs. Can be used for config sharding but only supports a subset of the config. The flag can be passed multiple times.")
	fs.StringVar(&o.SupplementalPluginsConfigsFileNameSuffix, "supplemental-plugin-configs-filename-suffix", "_pluginconfig.yaml", "Suffix for additional plugin configs. Only files with this name will be considered")
}

func (o *PluginOptions) Validate(_ bool) error {
	return nil
}

func (o *PluginOptions) PluginAgent() (*plugins.ConfigAgent, error) {
	pluginAgent := &plugins.ConfigAgent{}
	if err := pluginAgent.Start(o.PluginConfigPath, o.SupplementalPluginsConfigDirs.Strings(), o.SupplementalPluginsConfigsFileNameSuffix, o.CheckUnknownPlugins); err != nil {
		return nil, fmt.Errorf("failed to start plugins agent: %w", err)
	}

	return pluginAgent, nil
}
