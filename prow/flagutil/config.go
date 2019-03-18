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

	"k8s.io/test-infra/prow/config"
)

// ConfigOptions holds options for interacting with Configs.
type ConfigOptions struct {
	ConfigPath       string
	JobConfigPath    string
	PluginConfigPath string

	// ConfigPathNotEmpty is a boolean that enables validation of ConfigPath string.
	ConfigPathNotEmpty bool
	// JobConfigPathNotEmpty is a boolean that enables validation of JobConfigPath string.
	JobConfigPath bool
	// PluginConfigPathNotEmpty is a boolean that enables validation of PluginConfigPath string.
	PluginConfigPath bool
}

// AddFlags injects config options into the given FlagSet.
func (o *ConfigOptions) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.ConfigPath, "config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	fs.StringVar(&o.JobConfigPath, "job-config-path", "", "Path to prow job configs.")
	fs.StringVar(&o.PluginConfigPath, "plugin-config-path", "/etc/plugins/plugins.yaml", "Path to prow plugin configs.")
}

// Validate validates config options.
func (o *ConfigOptions) Validate(dryRun bool) error {
	if o.ConfigPathNotEmpty {
		if o.ConfigPath == "" {
			return fmt.Errorf("--config-path should not be empty, please set a valid config path.")
		}
	}
	if o.JobConfigPathNotEmpty {
		if o.JobConfigPath == "" {
			return fmt.Errorf("--job-config-path should not be empty, please set a valid job config path.")
		}
	}
	if o.PluginConfigPathNotEmpty {
		if o.PluginConfigPath == "" {
			return fmt.Errorf("--plugin-config-path should not be empty, please set a valid plugin config path.")
		}
	}
	return nil
}

// Agent returns a started config agent.
func (o *ConfigOptions) Agent() (agent *config.Agent, err error) {
	agent = &config.Agent{}
	config, err := config.Load(o.ConfigPath, o.jobConfigPath)
	if err != nil {
		return nil, err
	}
	agent.Set(config)

	err = agent.Start(o.ConfigPath, o.jobConfigPath)
	if err != nil {
		return nil, err
	}

	return agent, err
}
