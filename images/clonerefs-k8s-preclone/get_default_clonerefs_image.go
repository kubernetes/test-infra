/*
Copyright 2021 The Kubernetes Authors.

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

// Small utility to dump the default clonerefs image

package main

import (
	"fmt"
	"io/ioutil"
	"log"

	prowconfig "k8s.io/test-infra/prow/config"
	"sigs.k8s.io/yaml"
)

func main() {
	config := struct {
		prowconfig.Plank `json:"plank"`
	}{}
	b, err := ioutil.ReadFile("./../../config/prow/config.yaml")
	if err != nil {
		log.Fatalf("failed to read config: %v", err)
	}
	yaml.Unmarshal(b, &config)
	fmt.Println(config.Plank.DefaultDecorationConfigs["*"].UtilityImages.CloneRefs)
}
