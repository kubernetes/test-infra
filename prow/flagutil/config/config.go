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

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
)

const (
	defaultConfigPathFlagName = "config-path"
	defaultJobConfigPathFlag  = "job-config-path"
)

type ConfigOptions struct {
	ConfigPath    string
	JobConfigPath string
	// ConfigPathFlagName allows to override the flag name for the prow config. Defaults
	// to 'config-path'.
	ConfigPathFlagName string
	// JobConfigPathFlagName allows to override the flag name for the job config. Defaults
	// to 'job-config-path'.
	JobConfigPathFlagName                 string
	SupplementalProwConfigDirs            flagutil.Strings
	SupplementalProwConfigsFileNameSuffix string
}

func (o *ConfigOptions) AddFlags(fs *flag.FlagSet) {
	if o.ConfigPathFlagName == "" {
		o.ConfigPathFlagName = defaultConfigPathFlagName
	}
	if o.JobConfigPathFlagName == "" {
		o.JobConfigPathFlagName = defaultJobConfigPathFlag
	}
	fs.StringVar(&o.ConfigPath, o.ConfigPathFlagName, o.ConfigPath, "Path to the prowconfig")
	fs.StringVar(&o.JobConfigPath, o.JobConfigPathFlagName, o.JobConfigPath, "Path to the job config")
	fs.Var(&o.SupplementalProwConfigDirs, "supplemental-prow-config-dir", "An additional directory from which to load prow configs. Can be used for config sharding but only supports a subset of the config. The flag can be passed multiple times.")
	fs.StringVar(&o.SupplementalProwConfigsFileNameSuffix, "supplemental-prow-configs-filename", "_prowconfig.yaml", "Suffix for additional prow configs. Only files with this name will be considered. Deprecated and mutually exclusive with --supplemental-prow-configs-filename-suffix")
	fs.StringVar(&o.SupplementalProwConfigsFileNameSuffix, "supplemental-prow-configs-filename-suffix", "_prowconfig.yaml", "Suffix for additional prow configs. Only files with this name will be considered")
}

func (o *ConfigOptions) Validate(_ bool) error {
	if o.ConfigPath == "" {
		return fmt.Errorf("--%s is mandatory", o.ConfigPathFlagName)
	}
	return nil
}

func (o *ConfigOptions) ValidateConfigOptional() error {
	if o.JobConfigPath != "" && o.ConfigPath == "" {
		return fmt.Errorf("if --%s is given, --%s must be given as well", o.JobConfigPathFlagName, o.ConfigPathFlagName)
	}
	return nil
}

func (o *ConfigOptions) ConfigAgent(reuse ...*config.Agent) (*config.Agent, error) {
	var ca *config.Agent
	if n := len(reuse); n > 1 {
		return nil, fmt.Errorf("got more than one (%d) config agents to re-use", n)
	} else if n == 1 {
		ca = reuse[0]
	} else {
		ca = &config.Agent{}
	}
	return o.ConfigAgentWithAdditionals(ca, nil)
}

func (o *ConfigOptions) ConfigAgentWithAdditionals(ca *config.Agent, additionals []func(*config.Config) error) (*config.Agent, error) {
	return ca, ca.Start(o.ConfigPath, o.JobConfigPath, o.SupplementalProwConfigDirs.Strings(), o.SupplementalProwConfigsFileNameSuffix, additionals...)
}
