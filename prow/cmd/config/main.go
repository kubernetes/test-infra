/*
Copyright 2017 The Kubernetes Authors.

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

// Package main knows how to validate config files.
package main

import (
	"flag"
	"fmt"
	"os"

	"k8s.io/test-infra/prow/config"
	_ "k8s.io/test-infra/prow/hook"
	"k8s.io/test-infra/prow/plugins"
)

var (
	configPath   = flag.String("config-path", "", "Path to config file.")
	pluginConfig = flag.String("plugin-config", "", "Path to plugin config file.")
)

func main() {
	flag.Parse()
	var foundError bool
	if *pluginConfig != "" {
		pa := &plugins.PluginAgent{}
		if err := pa.Load(*pluginConfig); err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v.", *pluginConfig, err)
			foundError = true
		}
	}
	if *configPath != "" {
		if _, err := config.Load(*configPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v.", *configPath, err)
			foundError = true
		}
	}
	if foundError {
		os.Exit(1)
	}
}
