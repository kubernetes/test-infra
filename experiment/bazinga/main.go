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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"k8s.io/test-infra/experiment/bazinga/pkg/app"
	"k8s.io/test-infra/experiment/bazinga/pkg/config"
	"k8s.io/test-infra/experiment/bazinga/pkg/config/encoding"
)

var flags struct {
	config string
}

func init() {
	flag.StringVar(&flags.config, "config", "", "path to the bazinga config file")
}

func main() {
	var appConfig *config.App
	flag.Parse()
	if flags.config == "" {
		if flag.NArg() == 0 {
			fmt.Fprintln(os.Stderr, "usage: bazinga -- CMD [ARGS]")
			os.Exit(1)
		}
		appConfig = getAppConfigFromArgs()
	} else {
		var err error
		if appConfig, err = encoding.Load(flags.config); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	result, err := app.Run(context.Background(), *appConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	os.Exit(result.ExitCode)
}

func getAppConfigFromArgs() *config.App {
	appConfig := &config.App{
		TestSuites: []config.TestSuite{
			{
				Name: flag.Arg(0),
				TestCases: []config.TestCase{
					{
						Name:     flag.Arg(0),
						Command:  flag.Arg(0),
						Args:     flag.Args()[1:],
						EnvClean: true,
					},
				},
			},
		},
	}
	encoding.SetDefaults(appConfig)
	return appConfig
}
