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
	"flag"
	"fmt"

	"k8s.io/test-infra/prow/plugins"
)

// PluginConfigOptions holds options for interacting with Plugin Configs.
type PluginConfigOptions struct {
	PluginConfigPath string
}

// AddFlags injects plugin config options into the given FlagSet.
func (o *PluginConfigOptions) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.PluginConfigPath, "plugin-config-path", "/etc/plugins/plugins.yaml", "Path to prow plugin configs.")
}

// Validate validates plugin config options.
func (o *PluginConfigOptions) Validate(dryRun bool) error {
	return nil
}

// Agent returns a started plugin agent.
func (o *PluginConfigOptions) Agent() (agent *plugins.Agent, err error) {
	agent = &plugins.Agent{}
	config, err := plugins.Load(o.PluginConfigPath)
	if err != nil {
		return nil, err
	}
	agent.Set(config)

	err = agent.Start(o.PluginConfigPath)
	if err != nil {
		return nil, err
	}

	return agent, err
}
