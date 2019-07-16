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

package common

import (
	"fmt"
	"io/ioutil"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"

	"k8s.io/apimachinery/pkg/util/validation"
)

// ValidateConfig validates config with existing resources
// In: boskosConfig - a boskos config defining resources
// Out: nil on success, error on failure
func ValidateConfig(config *BoskosConfig) error {
	if len(config.Resources) == 0 {
		return fmt.Errorf("empty config")
	}
	resourceNames := map[string]bool{}
	resourceTypes := map[string]bool{}
	resourcesNeeds := map[string]int{}
	actualResources := map[string]int{}

	for _, e := range config.Resources {
		if e.Type == "" {
			return fmt.Errorf("empty resource type: %s", e.Type)
		}

		if resourceTypes[e.Type] {
			return fmt.Errorf("type %s already exists", e.Type)
		}

		names := e.Names
		if len(e.Names) == 0 {
			// Dynamic Resource
			if e.MaxCount == 0 {
				return fmt.Errorf("max should be > 0")
			}
			if e.MinCount > e.MaxCount {
				return fmt.Errorf("min should be <= max %v", e)
			}
			for i := 0; i < e.MaxCount; i++ {
				name := GenerateDynamicResourceName()
				names = append(names, name)
			}

			// Updating resourceNeeds
			for k, v := range e.Needs {
				resourcesNeeds[k] += v * e.MaxCount
			}

		}
		actualResources[e.Type] += len(names)
		for _, name := range names {
			errs := validation.IsQualifiedName(name)
			if len(errs) != 0 {
				return fmt.Errorf("resource name %s is not a qualified k8s object name, errs: %v", name, errs)
			}

			if _, ok := resourceNames[name]; ok {
				return fmt.Errorf("duplicated resource name: %s", name)
			}
			resourceNames[name] = true
		}
	}

	for rType, needs := range resourcesNeeds {
		actual, ok := actualResources[rType]
		if !ok {
			err := fmt.Errorf("need for resource %s that does not exist", rType)
			logrus.WithError(err).Errorf("invalid configuration")
			return err
		}
		if needs > actual {
			err := fmt.Errorf("not enough resource of type %s for provisioning", rType)
			logrus.WithError(err).Errorf("invalid configuration")
			return err
		}
	}
	return nil
}

// ParseConfig reads in configPath and returns a list of resource objects
// on success.
func ParseConfig(configPath string) (*BoskosConfig, error) {
	file, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var data BoskosConfig
	err = yaml.Unmarshal(file, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}
