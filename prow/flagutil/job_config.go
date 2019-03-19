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

// JobConfigOptions holds options for interacting with job configs.
type JobConfigOptions struct {
	JobConfigPath string
}

// AddFlags injects job config options into the given FlagSet.
func (o *JobConfigOptions) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.JobConfigPath, "job-config-path", "", "Path to prow job configs.")
}

// Validate validates job config options.
func (o *JobConfigOptions) Validate() error {
	return nil
}

// Agent takes a configPath and returns a started job config agent. The configPath
// must be a valid config path.
func (o *JobConfigOptions) Agent(configPath string) (agent *config.Agent, err error) {
	agent = &config.Agent{}
	config, err := config.Load(configPath, o.JobConfigPath)
	if err != nil {
		return nil, err
	}

	err = agent.Start(configPath, o.JobConfigPath)
	if err != nil {
		return nil, err
	}

	return agent, err
}
