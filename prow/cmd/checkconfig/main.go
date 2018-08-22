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

// checkconfig loads configuration for Prow to validate it
package main

import (
	"errors"
	"flag"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	_ "k8s.io/test-infra/prow/hook"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/plugins"
)

type options struct {
	configPath    string
	jobConfigPath string
	pluginConfig  string
}

func (o *options) Validate() error {
	if o.configPath == "" {
		return errors.New("required flag --config-path was unset")
	}
	if o.pluginConfig == "" {
		return errors.New("required flag --plugin-config was unset")
	}
	return nil
}

func gatherOptions() options {
	o := options{}
	flag.StringVar(&o.configPath, "config-path", "", "Path to config.yaml.")
	flag.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	flag.StringVar(&o.pluginConfig, "plugin-config", "", "Path to plugin config file.")
	flag.Parse()
	return o
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "checkconfig"}),
	)

	configAgent := config.Agent{}
	if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
		logrus.WithError(err).Fatal("Error loading Prow config.")
	}

	pluginAgent := plugins.PluginAgent{}
	if err := pluginAgent.Load(o.pluginConfig); err != nil {
		logrus.WithError(err).Fatal("Error loading Prow plugin config.")
	}
}
