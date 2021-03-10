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
	"flag"
	"fmt"
	"io/ioutil"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/cmd/generic-autobumper/bumper"

	"gopkg.in/yaml.v2"
)

func parseOptions() (*bumper.Options, error) {
	var config string

	flag.StringVar(&config, "config", "", "The path to the config file for the autobumber.")
	flag.Parse()

	var o bumper.Options
	data, err := ioutil.ReadFile(config)
	if err != nil {
		return nil, fmt.Errorf("Failed to read in config file, %s", config)
	}

	err = yaml.UnmarshalStrict(data, &o)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse yaml file, %s", err)
	}

	return &o, nil
}

func main() {
	o, err := parseOptions()
	if err != nil {
		logrus.WithError(err).Fatalf("Failed to run the bumper tool")
	}

	if err := bumper.Run(o); err != nil {
		logrus.WithError(err).Fatalf("failed to run the bumper tool")
	}
}
